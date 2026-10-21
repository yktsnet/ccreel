package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var demoPath = filepath.Join(fixtureDir, "projects", "-home-alice-src-demo", "11111111-1111-4111-8111-111111111111.jsonl")

func TestReadTurns(t *testing.T) {
	turns, err := ReadTurns(demoPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 {
		t.Fatalf("got %d turns, want 2", len(turns))
	}
	got := turns[0]
	if got.Index != 1 || got.Prompt != "/review src/parser.go" {
		t.Errorf("turn 1 = %d %q", got.Index, got.Prompt)
	}
	want := []Message{
		{Kind: "tool_use", Tool: "Read", Text: "/home/alice/src/demo/src/parser.go"},
		{Kind: "tool_result", Tool: "Read", Text: "package parser"},
		{Kind: "text", Text: "The parser drops the last token when the input has no trailing newline."},
	}
	if len(got.Messages) != len(want) {
		t.Fatalf("turn 1 messages = %+v", got.Messages)
	}
	for i := range want {
		if got.Messages[i] != want[i] {
			t.Errorf("message %d = %+v, want %+v", i, got.Messages[i], want[i])
		}
	}
	if f := turns[1].FinalText(); f != "Fixed. Added TestParseEmpty." {
		t.Errorf("turn 2 final = %q", f)
	}
}

func TestFind(t *testing.T) {
	if p, err := Find(fixtureDir, "1111"); err != nil || p != demoPath {
		t.Errorf("Find(1111) = %q, %v", p, err)
	}
	for _, bad := range []string{"", "*", "../x", "9999"} {
		if _, err := Find(fixtureDir, bad); err == nil {
			t.Errorf("Find(%q) succeeded, want error", bad)
		}
	}

	dir := t.TempDir()
	proj := filepath.Join(dir, "projects", "-p")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"abcd0000-0000", "abcd1111-1111"} {
		if err := os.WriteFile(filepath.Join(proj, id+".jsonl"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_, err := Find(dir, "abcd")
	if err == nil || !strings.Contains(err.Error(), "matches 2 sessions") {
		t.Errorf("ambiguous prefix: err = %v", err)
	}
}

func TestPromptTextSkipsCompactSummary(t *testing.T) {
	r := record{Type: "user", Message: []byte(`{"role":"user","content":"This session is being continued from a previous conversation that ran out of context."}`)}
	if _, ok := promptText(r); ok {
		t.Error("compact summary counted as a prompt")
	}
}
