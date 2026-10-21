package finder

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	demo  = "11111111-1111-4111-8111-111111111111"
	other = "22222222-2222-4222-8222-222222222222"
)

var (
	fixture = filepath.Join("..", "..", "testdata")
	now     = time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC)
)

func find(t *testing.T, words ...string) Result {
	t.Helper()
	res, err := Find(ParseQuery(words), Options{ConfigDirs: []string{fixture}, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func ids(r Result) string {
	var out []string
	for _, c := range r.Candidates {
		out = append(out, c.ID[:8])
	}
	return fmt.Sprint(out)
}

func TestParseQuery(t *testing.T) {
	q := ParseQuery([]string{"@demo", "#12 parser"})
	if q.Repo != "demo" || q.Issue != "12" || fmt.Sprint(q.Terms) != "[12 parser]" {
		t.Errorf("ParseQuery = %+v", q)
	}
}

func TestFindByTitleAndBody(t *testing.T) {
	r := find(t, "parser")
	if ids(r) != "[11111111]" || !r.Decided {
		t.Fatalf("got %s decided=%v", ids(r), r.Decided)
	}
	if fmt.Sprint(r.Candidates[0].Reasons) != "[title:parser body:parser×8]" {
		t.Errorf("reasons = %v", r.Candidates[0].Reasons)
	}
}

func TestFindByIssue(t *testing.T) {
	r := find(t, "#12")
	if ids(r) != "[11111111]" || r.Candidates[0].Reasons[0] != "issue #12" {
		t.Errorf("got %s %v", ids(r), r.Candidates)
	}
}

func TestFindByRepoOnly(t *testing.T) {
	r := find(t, "@other")
	if ids(r) != "[22222222]" {
		t.Errorf("got %s", ids(r))
	}
}

func TestTitleOutranksBody(t *testing.T) {
	// demo は題に parser、other は本文に logs。どちらも片方の語にしか当たらない
	r := find(t, "logs", "parser")
	if ids(r) != "[11111111 22222222]" {
		t.Errorf("got %s", ids(r))
	}
	if r.Candidates[0].Score <= r.Candidates[1].Score {
		t.Errorf("title hit should outrank a body hit: %d vs %d", r.Candidates[0].Score, r.Candidates[1].Score)
	}
}

func TestExcludeCurrent(t *testing.T) {
	res, err := Find(ParseQuery([]string{"parser"}), Options{ConfigDirs: []string{fixture}, Exclude: demo})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Candidates) != 0 {
		t.Errorf("current session was not excluded: %s", ids(res))
	}
}

func TestEmptyQuery(t *testing.T) {
	if _, err := Find(Query{}, Options{ConfigDirs: []string{fixture}}); err != ErrEmpty {
		t.Errorf("err = %v", err)
	}
}

func TestDecided(t *testing.T) {
	c := func(scores ...int) []Candidate {
		var cs []Candidate
		for _, s := range scores {
			cs = append(cs, Candidate{Score: s})
		}
		return cs
	}
	cases := []struct {
		cs   []Candidate
		want bool
	}{
		{c(), false},
		{c(5), true},
		{c(65, 25), true},
		{c(130, 91), false}, // 1.5倍に届かない
		{c(40, 25), false},  // 1.5倍だが差が20に届かない
		{c(65, 65), false},
	}
	for _, tc := range cases {
		if got := decided(tc.cs); got != tc.want {
			t.Errorf("decided(%v) = %v", tc.cs, got)
		}
	}
}

// rg が無い環境でも、同じ数を返す
func TestCountWithoutRg(t *testing.T) {
	root := filepath.Join(fixture, "projects")
	withRg, err := count(root, "parser", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "")
	withoutRg, err := count(root, "parser", true)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(withRg) != fmt.Sprint(withoutRg) {
		t.Errorf("rg %v != go %v", withRg, withoutRg)
	}
}

// writeSession は1本のセッションを作る。body の語を n 行くり返して本文の当たり数を作る。
func writeSession(t *testing.T, dir, id, branch, title, body string, n int) {
	t.Helper()
	p := filepath.Join(dir, "projects", "-home-alice-src-app", id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, `{"type":"user","message":{"role":"user","content":"start"},"timestamp":"2026-03-01T09:00:00Z","cwd":"/home/alice/src/app","gitBranch":%q}`+"\n", branch)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":%q}]},"timestamp":"2026-03-01T09:01:00Z"}`+"\n", body)
	}
	if title != "" {
		fmt.Fprintf(&b, `{"type":"ai-title","aiTitle":%q}`+"\n", title)
	}
	if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRanking(t *testing.T) {
	dir := t.TempDir()
	const (
		onBranch = "aaaaaaaa-0000-4000-8000-000000000001"
		mentions = "aaaaaaaa-0000-4000-8000-000000000002"
		titled   = "aaaaaaaa-0000-4000-8000-000000000003"
		wordy    = "aaaaaaaa-0000-4000-8000-000000000004"
	)
	// Issue の作業をしたセッションは、番号を本文で何度も口にしただけのセッションより上
	writeSession(t, dir, onBranch, "task/42-fix-login", "", "done", 1)
	writeSession(t, dir, mentions, "main", "", "see issue 42 again", 40)
	// 題に語があるセッションは、本文で語をくり返しただけのセッションより上
	writeSession(t, dir, titled, "main", "Router rewrite", "router", 1)
	writeSession(t, dir, wordy, "main", "", "router router", 40)

	o := Options{ConfigDirs: []string{dir}, Now: now}
	r, err := Find(ParseQuery([]string{"#42"}), o)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Candidates) < 2 || r.Candidates[0].ID != onBranch {
		t.Errorf("#42: got %s", ids(r))
	}
	r, err = Find(ParseQuery([]string{"router"}), o)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Candidates) < 2 || r.Candidates[0].ID != titled {
		t.Errorf("router: got %s", ids(r))
	}
}
