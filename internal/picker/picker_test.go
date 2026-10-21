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
	m := session.Meta{ID: "11111111-aaaa", Repo: "demo", FirstPrompt: "fix\nthe  parser", Updated: time.Date(2026, 3, 1, 9, 0, 0, 0, time.Local)}
	got := Line(m)
	cols := strings.Split(got, "\t")
	if len(cols) != 4 || cols[0] != "11111111-aaaa" || cols[1] != "03-01 09:00" || strings.TrimSpace(cols[2]) != "demo" || cols[3] != "fix the parser" {
		t.Errorf("Line = %q", got)
	}
	m.Title = "Fix parser"
	if !strings.HasSuffix(Line(m), "\tFix parser") {
		t.Errorf("title not preferred: %q", Line(m))
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
