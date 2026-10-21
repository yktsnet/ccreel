package session

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Hit は検索で当たった1か所。Snippet は当たった語の前後だけを切り出したもの。
type Hit struct {
	ID      string `json:"id"`
	Repo    string `json:"repo"`
	Title   string `json:"title"`
	Updated string `json:"updated"`
	Turn    int    `json:"turn"`
	Kind    string `json:"kind"`
	Snippet string `json:"snippet"`
}

type SearchOptions struct {
	Tools      bool // tool_use と tool_result も対象にする
	Context    int  // 当たった語の前後に残す文字数
	Limit      int  // 全体の件数の上限
	PerSession int  // 1セッションから出す件数の上限
	Filter     func(Meta) bool
}

// Candidates は query を含むセッションを返す。全文を decode する前に、rg で生の jsonl から絞る。
// rg が無ければ同じ絞り込みを Go で行う。
func Candidates(configDir, query string) ([]string, error) {
	root := filepath.Join(configDir, "projects")
	if _, err := exec.LookPath("rg"); err == nil {
		out, err := exec.Command("rg", "-l", "-F", "-i", "--max-depth", "2", "--glob", "*.jsonl", "--", query, root).Output()
		var ee *exec.ExitError
		// rg は1件も当たらないと終了コード1を返す
		if errors.As(err, &ee) && ee.ExitCode() == 1 {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return strings.Split(strings.TrimSpace(string(out)), "\n"), nil
	}
	paths, err := Discover(configDir)
	if err != nil {
		return nil, err
	}
	lq := bytes.ToLower([]byte(query))
	var hits []string
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err == nil && bytes.Contains(bytes.ToLower(b), lq) {
			hits = append(hits, p)
		}
	}
	return hits, nil
}

// Search は候補を新しい順に読み、発言と応答の中で query に当たった箇所を返す。
// jsonl の生の文字列で当たっても、エスケープやツールの出力にしか無ければ Hit にならない。
func Search(paths []string, query string, opt SearchOptions) ([]Hit, error) {
	lq := strings.ToLower(query)
	var hits []Hit
	// 件数の上限で早く止められるよう、全件を Scan せず mtime の新しい順に1本ずつ読む
	for _, p := range byMtime(paths) {
		m, err := Scan(p)
		if err != nil || m.Prompts == 0 || (opt.Filter != nil && !opt.Filter(m)) {
			continue
		}
		turns, err := ReadTurns(m.Path)
		if err != nil {
			continue
		}
		n := 0
		emit := func(turn int, kind, text string) bool {
			snip, ok := snippet(text, lq, opt.Context)
			if !ok {
				return true
			}
			hits = append(hits, Hit{
				ID: m.ID, Repo: m.Repo, Title: m.Title, Updated: m.Updated.Format("2006-01-02T15:04:05Z07:00"),
				Turn: turn, Kind: kind, Snippet: snip,
			})
			n++
			return (opt.PerSession <= 0 || n < opt.PerSession) && (opt.Limit <= 0 || len(hits) < opt.Limit)
		}
	turns:
		for _, t := range turns {
			if !emit(t.Index, "prompt", t.Prompt) {
				break
			}
			for _, msg := range t.Messages {
				if msg.Kind != "text" && !opt.Tools {
					continue
				}
				if !emit(t.Index, msg.Kind, msg.Text) {
					break turns
				}
			}
		}
		if opt.Limit > 0 && len(hits) >= opt.Limit {
			break
		}
	}
	return hits, nil
}

func byMtime(paths []string) []string {
	type pm struct {
		path string
		mod  int64
	}
	ps := make([]pm, 0, len(paths))
	for _, p := range paths {
		if fi, err := os.Stat(p); err == nil {
			ps = append(ps, pm{p, fi.ModTime().UnixNano()})
		}
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i].mod > ps[j].mod })
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.path
	}
	return out
}

// snippet は当たった語の前後 ctx 文字を切り出す。大文字小文字は区別しない。
func snippet(text, lq string, ctx int) (string, bool) {
	lower := strings.ToLower(text)
	i := strings.Index(lower, lq)
	if i < 0 {
		return "", false
	}
	// ToLower でバイト長が変わる文字を含むときは、位置を合わせられないので小文字のまま切り出す
	src := text
	if len(lower) != len(text) {
		src = lower
	}
	head := []rune(src[:i])
	tail := []rune(src[i:])
	start := max(len(head)-ctx, 0)
	end := min(len([]rune(lq))+ctx, len(tail))
	s := string(head[start:]) + string(tail[:end])
	s = strings.Join(strings.Fields(s), " ")
	if start > 0 {
		s = "…" + s
	}
	if end < len(tail) {
		s += "…"
	}
	return s, true
}
