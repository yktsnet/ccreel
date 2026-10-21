package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/yktsnet/ccreel/internal/session"
)

func TestSelectTurns(t *testing.T) {
	var turns []session.Turn
	for i := 1; i <= 6; i++ {
		turns = append(turns, session.Turn{Index: i})
	}
	idx := func(ts []session.Turn) []int {
		var out []int
		for _, t := range ts {
			out = append(out, t.Index)
		}
		return out
	}
	cases := []struct {
		spec string
		last int
		want string
	}{
		{"", 0, "[1 2 3 4 5 6]"},
		{"", 2, "[5 6]"},
		{"3", 0, "[3]"},
		{"2-4", 0, "[2 3 4]"},
		{"2-5", 2, "[4 5]"},
	}
	for _, c := range cases {
		got, err := selectTurns(turns, c.spec, c.last)
		if err != nil {
			t.Fatal(err)
		}
		if s := fmt.Sprint(idx(got)); s != c.want {
			t.Errorf("selectTurns(%q, %d) = %s, want %s", c.spec, c.last, s, c.want)
		}
	}
	for _, bad := range []string{"x", "5-2", "1-"} {
		if _, err := selectTurns(turns, bad, 0); err == nil {
			t.Errorf("selectTurns(%q) succeeded", bad)
		}
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("あいうえお", 3); got != "あいう… [truncated 2 chars]" {
		t.Errorf("truncate = %q", got)
	}
	if got := truncate("short", 0); got != "short" {
		t.Errorf("truncate with 0 = %q", got)
	}
}

func TestShowFlagsAfterID(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "../../testdata")
	var out, errw bytes.Buffer
	if err := runShow([]string{"1111", "--final", "--json", "--last", "1"}, &out, &errw); err != nil {
		t.Fatal(err)
	}
	var turn session.Turn
	if err := json.Unmarshal(out.Bytes(), &turn); err != nil {
		t.Fatalf("output is not one JSON object: %q", out.String())
	}
	if turn.Index != 2 || len(turn.Messages) != 1 || turn.Messages[0].Text != "Fixed. Added TestParseEmpty." {
		t.Errorf("got %+v", turn)
	}
}

func TestShowMaxTotal(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "../../testdata")
	var out, errw bytes.Buffer
	if err := runShow([]string{"1111", "--max-total", "10"}, &out, &errw); err != nil {
		t.Fatal(err)
	}
	// 1ターン目は上限を超えても出し、残りは止めたことを stderr で知らせる
	if !strings.Contains(out.String(), "## turn 1") || strings.Contains(out.String(), "## turn 2") {
		t.Errorf("stdout = %q", out.String())
	}
	if !strings.Contains(errw.String(), "1 more turn(s) from turn 2") {
		t.Errorf("stderr = %q", errw.String())
	}
}

func TestSplitPassthrough(t *testing.T) {
	own, rest := splitPassthrough([]string{"fzf", "--no-pull", "--", "--model", "x"})
	if fmt.Sprint(own) != "[fzf --no-pull]" || fmt.Sprint(rest) != "[--model x]" {
		t.Errorf("got %v %v", own, rest)
	}
}

func TestClaudeEnvReplacesConfigDir(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "/old")
	n := 0
	for _, kv := range claudeEnv("/new") {
		if strings.HasPrefix(kv, "CLAUDE_CONFIG_DIR=") {
			n++
			if kv != "CLAUDE_CONFIG_DIR=/new" {
				t.Errorf("got %q", kv)
			}
		}
	}
	if n != 1 {
		t.Errorf("CLAUDE_CONFIG_DIR appears %d times", n)
	}
}

func TestListSkipsCurrentSession(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "../../testdata")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "22222222-2222-4222-8222-222222222222")
	var out bytes.Buffer
	if err := runList(nil, &out, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "22222222") || !strings.Contains(out.String(), "11111111") {
		t.Errorf("default list = %q", out.String())
	}
	out.Reset()
	if err := runList([]string{"--include-current"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "22222222") {
		t.Errorf("--include-current list = %q", out.String())
	}
}

func TestListRepoIsExact(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "../../testdata")
	for repo, want := range map[string]bool{"demo": true, "dem": false} {
		var out bytes.Buffer
		if err := runList([]string{"--repo", repo}, &out, &out); err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(out.String(), "11111111"); got != want {
			t.Errorf("--repo %s: got %q", repo, out.String())
		}
	}
}
