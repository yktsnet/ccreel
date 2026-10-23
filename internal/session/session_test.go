package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const fixtureDir = "../../testdata"

func TestDiscoverSkipsAttachments(t *testing.T) {
	paths, err := Discover(fixtureDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 3 {
		t.Fatalf("got %d paths, want 3 (subagents excluded): %v", len(paths), paths)
	}
}

func TestScan(t *testing.T) {
	m, err := Scan(filepath.Join(fixtureDir, "projects", "-home-alice-src-demo", "11111111-1111-4111-8111-111111111111.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	checks := []struct{ name, got, want string }{
		{"ID", m.ID, "11111111-1111-4111-8111-111111111111"},
		{"Project", m.Project, "-home-alice-src-demo"},
		{"Title", m.Title, "Fix parser trailing token bug"},
		{"FirstPrompt", m.FirstPrompt, "/review src/parser.go"},
		{"CWD", m.CWD, "/home/alice/src/demo/.claude/worktrees/fix-parser"},
		{"Repo", m.Repo, "demo"},
		{"Branch", m.Branch, "task/12-fix-parser"},
		{"Issue", m.Issue, "12"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
	// isMeta・tool_result・サブエージェントの発言は数えない
	if m.Prompts != 2 {
		t.Errorf("Prompts = %d, want 2", m.Prompts)
	}
	if want := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC); !m.Started.Equal(want) {
		t.Errorf("Started = %v, want %v", m.Started, want)
	}
	if want := time.Date(2026, 3, 1, 9, 6, 0, 0, time.UTC); !m.Updated.Equal(want) {
		t.Errorf("Updated = %v, want %v", m.Updated, want)
	}
}

func TestScanAllOrderAndEmpty(t *testing.T) {
	paths, _ := Discover(fixtureDir)
	metas := ScanAll(paths)
	var ids []string
	for _, m := range metas {
		ids = append(ids, m.ID[:8])
	}
	// 発言の無い 33333333 は落ち、新しい順に並ぶ
	if len(ids) != 2 || ids[0] != "22222222" || ids[1] != "11111111" {
		t.Errorf("got %v, want [22222222 11111111]", ids)
	}
}

func TestRepoNameAndIssue(t *testing.T) {
	cases := []struct{ branch, cwd, repo, issue string }{
		{"task/417-s3-journal-store", "/home/a/src/invoice/.claude/worktrees/417-s3-journal-store", "invoice", "417"},
		// 祖先へ書き換えられた記録は worktree の置き場そのものを指す
		{"task/749-month-picker", "/home/a/src/invoice/.claude/worktrees", "invoice", "749"},
		{"feat/picker", "/home/a/src/ccreel/.claude/worktrees/882-manual-run", "ccreel", "882"},
		{"day/2026-10-08-accdb-extract", "/home/a/src/invoice", "invoice", ""},
		{"main", "/home/a/src/demo", "demo", ""},
		// リポの隣の <リポ>.wt/<名前> に作る worktree
		{"claude/61-works-tag", "/home/a/src/site.wt/61-works-tag", "site", "61"},
		{"", "/home/a/src/site.wt/61-works-tag/web", "site", "61"},
		{"", "/home/a/src/site.wt", "site", ""},
		// .wt だけの名前や、途中に .wt を含むだけの名前はそのまま
		{"main", "/home/a/src/.wt", ".wt", ""},
		{"main", "/home/a/src/my.wtf", "my.wtf", ""},
		{"", "", "", ""},
	}
	for _, c := range cases {
		if got := repoName(c.cwd); got != c.repo {
			t.Errorf("repoName(%q) = %q, want %q", c.cwd, got, c.repo)
		}
		if got := issueNumber(c.branch, c.cwd); got != c.issue {
			t.Errorf("issueNumber(%q, %q) = %q, want %q", c.branch, c.cwd, got, c.issue)
		}
	}
}

// writeSession は架空のセッションを projects/<project>/<id>.jsonl に書き、更新日時を mtime に揃える。
func writeSession(t *testing.T, dir, project, id string, mtime time.Time, lines ...string) string {
	t.Helper()
	p := filepath.Join(dir, "projects", project, id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, mtime, mtime); err != nil {
		t.Fatal(err)
	}
	return p
}

func userLine(cwd, ts, text, extra string) string {
	return `{"type":"user","cwd":"` + cwd + `","timestamp":"` + ts + `",` + extra + `"message":{"role":"user","content":"` + text + `"}}`
}

func TestScanAllSkipsHeadless(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	writeSession(t, dir, "-src-a", "aaaaaaaa-0000-4000-8000-000000000001", now,
		userLine("/src/a", "2026-03-01T09:00:00Z", "hello", `"entrypoint":"cli",`))
	writeSession(t, dir, "-src-a", "aaaaaaaa-0000-4000-8000-000000000002", now,
		userLine("/src/a", "2026-03-01T09:00:00Z", "Reply with just: smoke", `"entrypoint":"sdk-cli",`))
	paths, _ := Discover(dir)
	metas := ScanAll(paths)
	if len(metas) != 1 || metas[0].ID != "aaaaaaaa-0000-4000-8000-000000000001" {
		t.Fatalf("got %d sessions, want only the cli one: %+v", len(metas), metas)
	}
}

func TestSameUUIDInTwoProjects(t *testing.T) {
	dir := t.TempDir()
	id := "bbbbbbbb-0000-4000-8000-000000000001"
	old := time.Now().Add(-time.Hour)
	writeSession(t, dir, "-src-a", id, old,
		userLine("/src/a", "2026-03-01T09:00:00Z", "first", ""))
	newer := writeSession(t, dir, "-src-a--claude-worktrees-x", id, time.Now(),
		userLine("/src/a", "2026-03-01T09:00:00Z", "first", ""),
		userLine("/src/a/.claude/worktrees/x", "2026-03-01T10:00:00Z", "second", ""))

	paths, _ := Discover(dir)
	metas := ScanAll(paths)
	if len(metas) != 1 || metas[0].Path != newer {
		t.Fatalf("ScanAll: got %+v, want one row for %s", metas, newer)
	}
	p, err := Find(dir, "bbbb")
	if err != nil || p != newer {
		t.Errorf("Find = %q, %v; want %q", p, err, newer)
	}
}

func TestRepoComesFromWhereTheSessionStarted(t *testing.T) {
	dir := t.TempDir()
	home, _ := os.UserHomeDir()
	moved := writeSession(t, dir, "-src-work", "cccccccc-0000-4000-8000-000000000001", time.Now(),
		userLine("/src/work", "2026-03-01T09:00:00Z", "start in work", ""),
		userLine("/src/dotfiles", "2026-03-01T10:00:00Z", "then cd to dotfiles", ""))
	fromHome := writeSession(t, dir, "-home", "cccccccc-0000-4000-8000-000000000002", time.Now(),
		userLine(home, "2026-03-01T09:00:00Z", "start at home", ""),
		userLine("/src/demo", "2026-03-01T10:00:00Z", "then cd to demo", ""))
	cases := []struct{ path, repo, cwd string }{
		{moved, "work", "/src/dotfiles"},
		// ホームで起動したものは、移った先のリポを使う
		{fromHome, "demo", "/src/demo"},
	}
	for _, c := range cases {
		m, err := Scan(c.path)
		if err != nil {
			t.Fatal(err)
		}
		if m.Repo != c.repo || m.CWD != c.cwd {
			t.Errorf("%s: Repo = %q, CWD = %q; want %q, %q", filepath.Base(c.path), m.Repo, m.CWD, c.repo, c.cwd)
		}
	}
}
