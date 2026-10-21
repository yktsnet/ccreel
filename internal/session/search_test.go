package session

import (
	"testing"
)

func TestSnippet(t *testing.T) {
	cases := []struct {
		text, q string
		ctx     int
		want    string
		ok      bool
	}{
		{"The PARSER drops tokens", "parser", 4, "The PARSER dro…", true},
		{"0123456789 needle 0123456789", "needle", 3, "…89 needle 01…", true},
		{"日本語の文章で検索する", "文章", 2, "…語の文章で検…", true},
		{"no match here", "absent", 5, "", false},
	}
	for _, c := range cases {
		got, ok := snippet(c.text, c.q, c.ctx)
		if got != c.want || ok != c.ok {
			t.Errorf("snippet(%q, %q) = %q, %v; want %q, %v", c.text, c.q, got, ok, c.want, c.ok)
		}
	}
}

func TestCandidates(t *testing.T) {
	paths, err := Candidates(fixtureDir, "LOGROTATE")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 {
		t.Errorf("got %v, want only the other session", paths)
	}
	// サブエージェント側にしか無い語では当たらない
	paths, err = Candidates(fixtureDir, "callers of Parse")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 0 {
		t.Errorf("got %v, want none (subagents are not searched)", paths)
	}
}

func TestSearch(t *testing.T) {
	paths, _ := Discover(fixtureDir)

	hits, _ := Search(paths, "parser", SearchOptions{Context: 10})
	kinds := []string{}
	for _, h := range hits {
		kinds = append(kinds, h.Kind)
	}
	// 既定ではツールの入出力を探さない
	if len(hits) != 2 || kinds[0] != "prompt" || kinds[1] != "text" {
		t.Errorf("default: got %v", kinds)
	}

	hits, _ = Search(paths, "parser", SearchOptions{Context: 10, Tools: true})
	if len(hits) != 4 {
		t.Errorf("with tools: got %d hits, want 4", len(hits))
	}

	hits, _ = Search(paths, "parser", SearchOptions{Context: 10, Tools: true, PerSession: 1})
	if len(hits) != 1 {
		t.Errorf("per-session 1: got %d hits", len(hits))
	}

	hits, _ = Search(paths, "parser", SearchOptions{Context: 10, Filter: func(m Meta) bool { return m.Repo == "other" }})
	if len(hits) != 0 {
		t.Errorf("filtered out: got %d hits", len(hits))
	}
}
