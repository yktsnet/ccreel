package session

import (
	"path/filepath"
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
