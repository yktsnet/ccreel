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
	// 題を先頭に、@リポ #番号 と日は後ろに薄く出す
	if len(cols) != 4 || cols[0] != "11111111-aaaa" || cols[1] != "fix the parser" ||
		cols[2] != dim+"@demo #417"+reset || cols[3] != dim+"TDY"+reset {
		t.Errorf("Line = %q", cols)
	}
	m.Title = "Fix parser"
	if cols := strings.Split(Line(m, now), "\t"); cols[1] != "Fix parser" {
		t.Errorf("title not preferred: %q", cols)
	}
}

func TestAgo(t *testing.T) {
	now := time.Date(2026, 3, 10, 0, 30, 0, 0, time.Local)
	cases := []struct {
		t    time.Time
		want string
	}{
		{now.Add(-20 * time.Minute), "TDY"},
		// 時間ではなく暦の日で数える。日付をまたいだ1時間前は昨日
		{now.Add(-time.Hour), "YDY"},
		{time.Date(2026, 3, 7, 23, 0, 0, 0, time.Local), "3d"},
		{time.Date(2026, 1, 5, 9, 0, 0, 0, time.Local), "01-05"},
	}
	for _, c := range cases {
		if got := Ago(c.t, now); got != c.want {
			t.Errorf("Ago(%v) = %q, want %q", c.t, got, c.want)
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
