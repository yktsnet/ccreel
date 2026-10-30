package picker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yktsnet/ccreel/internal/session"
)

func TestLine(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.Local)
	m := session.Meta{ID: "11111111-aaaa", Repo: "demo", Issue: "417", FirstPrompt: "fix\nthe  parser", Updated: now.Add(-time.Hour)}
	cols := strings.Split(Line(m, now), "\t")
	// タイトルを先頭に、@リポ #番号 と日は後ろに薄く出す
	if len(cols) != 5 || cols[0] != "11111111-aaaa" || cols[1] != "fix the parser" ||
		cols[2] != dim+"@demo #417"+reset || cols[3] != dim+"*0"+reset || cols[4] != dim+"1h"+reset {
		t.Errorf("Line = %q", cols)
	}
	m.Title = "Fix parser"
	if cols := strings.Split(Line(m, now), "\t"); cols[1] != "Fix parser" {
		t.Errorf("title not preferred: %q", cols)
	}
}

func TestAgo(t *testing.T) {
	// 深夜0時の直後。暦の日なら1時間前は昨日になる
	now := time.Date(2026, 3, 10, 0, 30, 0, 0, time.Local)
	cases := []struct {
		t    time.Time
		want string
	}{
		{now.Add(-30 * time.Second), "*0"},
		// 日付をまたいでも、24時間以内は *0
		{now.Add(-time.Hour), "*0"},
		{now.Add(-23*time.Hour - 59*time.Minute), "*0"},
		// 24時間ちょうどが *1 との境目
		{now.Add(-24 * time.Hour), "*1"},
		{now.Add(-47 * time.Hour), "*1"},
		{now.Add(-3*24*time.Hour - time.Hour), "*3"},
		{now.Add(-9*24*time.Hour - 23*time.Hour), "*9"},
		// 10日より前は * を付けず週で出す。*1 が *10 以降に当たらないように
		{now.Add(-10 * 24 * time.Hour), "1w"},
		{now.Add(-14 * 24 * time.Hour), "2w"},
		{now.Add(-63 * 24 * time.Hour), "9w"},
		// 時計のずれで未来の時刻が来ても *0 にする
		{now.Add(time.Hour), "*0"},
	}
	for _, c := range cases {
		if got := Ago(c.t, now); got != c.want {
			t.Errorf("Ago(%v) = %q, want %q", c.t, got, c.want)
		}
	}
}

func TestElapsed(t *testing.T) {
	now := time.Date(2026, 3, 10, 0, 30, 0, 0, time.Local)
	cases := []struct {
		t    time.Time
		want string
	}{
		{now.Add(-30 * time.Second), "now"},
		{now.Add(-12 * time.Minute), "12m"},
		{now.Add(-59 * time.Minute), "59m"},
		// 日付をまたいだ24時間以内にも出す
		{now.Add(-time.Hour), "1h"},
		{now.Add(-3*time.Hour - 40*time.Minute), "3h"},
		{now.Add(-23*time.Hour - 59*time.Minute), "23h"},
		// 時計のずれで未来の時刻が来ても now にする
		{now.Add(time.Minute), "now"},
		// 24時間を過ぎたら出さない
		{now.Add(-24 * time.Hour), ""},
		{now.Add(-47 * time.Hour), ""},
		{now.Add(-10 * 24 * time.Hour), ""},
	}
	for _, c := range cases {
		if got := Elapsed(c.t, now); got != c.want {
			t.Errorf("Elapsed(%v) = %q, want %q", c.t, got, c.want)
		}
	}
}

func TestResumeDir(t *testing.T) {
	tmp := t.TempDir()
	repo := filepath.Join(tmp, "src", "demo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(tmp, "home", "alice")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ cwd, want string }{
		{repo, repo},
		// 相手の端末にしか無い worktree は、手元にある祖先で開く
		{filepath.Join(repo, ".claude", "worktrees", "fix"), repo},
		// 祖先がホームより上にしか無ければホームで開く
		{filepath.Join(tmp, "home", "bob", "src"), home},
		{filepath.Join(home, "gone"), home},
		{home, home},
		{"", home},
	}
	for _, c := range cases {
		if got := ResumeDir(c.cwd, home); got != c.want {
			t.Errorf("ResumeDir(%q) = %q, want %q", c.cwd, got, c.want)
		}
	}
}

func TestSelectedIDs(t *testing.T) {
	out := "aaaa\tfirst\t@demo\t*0\nbbbb\tsecond\t@demo\t*1\n"
	got := selectedIDs(out)
	if len(got) != 2 || got[0] != "aaaa" || got[1] != "bbbb" {
		t.Errorf("selectedIDs = %q, want [aaaa bbbb]", got)
	}
	if got := selectedIDs("\n"); len(got) != 0 {
		t.Errorf("selectedIDs of empty output = %q, want none", got)
	}
}
