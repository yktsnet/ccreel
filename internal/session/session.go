package session

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Meta は一覧の1行に要る情報。本文は持たない。
type Meta struct {
	ID          string    `json:"id"`
	Path        string    `json:"path"`
	Project     string    `json:"project"`
	CWD         string    `json:"cwd"`
	Repo        string    `json:"repo"`
	Branch      string    `json:"branch,omitempty"`
	Issue       string    `json:"issue,omitempty"`
	Title       string    `json:"title"`
	FirstPrompt string    `json:"first_prompt"`
	Prompts     int       `json:"prompts"`
	Started     time.Time `json:"started"`
	Updated     time.Time `json:"updated"`
}

// Discover は projects 直下のセッションだけを返す。<uuid>/subagents などの付属物は一覧に出さない。
func Discover(configDir string) ([]string, error) {
	return filepath.Glob(filepath.Join(configDir, "projects", "*", "*.jsonl"))
}

// Find は UUID の先頭の数文字からセッションを引く。候補が複数なら誤って開かないよう失敗させる。
func Find(configDir, prefix string) (string, error) {
	if prefix == "" || strings.Trim(prefix, "0123456789abcdef-") != "" {
		return "", fmt.Errorf("invalid session id %q", prefix)
	}
	paths, err := filepath.Glob(filepath.Join(configDir, "projects", "*", prefix+"*.jsonl"))
	if err != nil {
		return "", err
	}
	switch len(paths) {
	case 0:
		return "", fmt.Errorf("no session matches %q in %s", prefix, configDir)
	case 1:
		return paths[0], nil
	}
	ids := make([]string, len(paths))
	for i, p := range paths {
		ids[i] = strings.TrimSuffix(filepath.Base(p), ".jsonl")
	}
	return "", fmt.Errorf("%q matches %d sessions: %s", prefix, len(paths), strings.Join(ids, ", "))
}

type record struct {
	Type        string          `json:"type"`
	IsMeta      bool            `json:"isMeta"`
	IsSidechain bool            `json:"isSidechain"`
	CWD         string          `json:"cwd"`
	GitBranch   string          `json:"gitBranch"`
	Timestamp   time.Time       `json:"timestamp"`
	AITitle     string          `json:"aiTitle"`
	Message     json.RawMessage `json:"message"`
}

type message struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// 本文の読み出しに関係ない行（API の生データなど）は数 MB になるので、decode の前に型で振り分ける。
var wanted = [][]byte{
	[]byte(`"type":"user"`),
	[]byte(`"type":"assistant"`),
	[]byte(`"type":"ai-title"`),
}

func relevant(line []byte) bool {
	for _, w := range wanted {
		if bytes.Contains(line, w) {
			return true
		}
	}
	return false
}

// Scan はセッション1本を読み、一覧用の情報を組み立てる。
func Scan(path string) (Meta, error) {
	f, err := os.Open(path)
	if err != nil {
		return Meta{}, err
	}
	defer f.Close()

	m := Meta{
		ID:      strings.TrimSuffix(filepath.Base(path), ".jsonl"),
		Path:    path,
		Project: filepath.Base(filepath.Dir(path)),
	}
	err = eachRecord(f, func(r record) {
		if !r.Timestamp.IsZero() {
			if m.Started.IsZero() || r.Timestamp.Before(m.Started) {
				m.Started = r.Timestamp
			}
			if r.Timestamp.After(m.Updated) {
				m.Updated = r.Timestamp
			}
		}
		if r.CWD != "" {
			m.CWD = r.CWD
		}
		if r.GitBranch != "" {
			m.Branch = r.GitBranch
		}
		switch r.Type {
		case "ai-title":
			// 会話が進むと付け直されるので、最後のものを使う
			m.Title = r.AITitle
		case "user":
			if text, ok := promptText(r); ok {
				m.Prompts++
				if m.FirstPrompt == "" {
					m.FirstPrompt = text
				}
			}
		}
	})
	if err != nil {
		return m, err
	}
	if m.Updated.IsZero() {
		if fi, err := os.Stat(path); err == nil {
			m.Updated = fi.ModTime()
		}
	}
	m.Repo = repoName(m.CWD)
	m.Issue = issueNumber(m.Branch, m.CWD)
	return m, nil
}

func eachRecord(r io.Reader, fn func(record)) error {
	br := bufio.NewReaderSize(r, 1<<20)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 && relevant(line) {
			var rec record
			if json.Unmarshal(line, &rec) == nil {
				fn(rec)
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// promptText は人が打った発言だけを取り出す。tool_result・isMeta・サブエージェント側の発言は除く。
func promptText(r record) (string, bool) {
	if r.IsMeta || r.IsSidechain || len(r.Message) == 0 {
		return "", false
	}
	var msg message
	if json.Unmarshal(r.Message, &msg) != nil {
		return "", false
	}
	var text string
	if json.Unmarshal(msg.Content, &text) != nil {
		var blocks []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal(msg.Content, &blocks) != nil {
			return "", false
		}
		for _, b := range blocks {
			if b.Type != "text" {
				return "", false
			}
			text += b.Text
		}
	}
	text = commandText(strings.TrimSpace(text))
	// compact のあとに自動で入る要約は、人の発言ではない
	if strings.HasPrefix(text, "This session is being continued from a previous conversation") {
		return "", false
	}
	return text, text != ""
}

// スラッシュコマンドは <command-name> と <command-args> のタグで記録されるので、打った形に戻す。
// <local-command-stdout> などコマンドの出力は発言ではないので落とす。
func commandText(s string) string {
	if !strings.HasPrefix(s, "<") {
		return s
	}
	name := between(s, "<command-name>", "</command-name>")
	if name == "" {
		return ""
	}
	if args := between(s, "<command-args>", "</command-args>"); args != "" {
		return name + " " + args
	}
	return name
}

func between(s, open, close string) string {
	i := strings.Index(s, open)
	if i < 0 {
		return ""
	}
	s = s[i+len(open):]
	j := strings.Index(s, close)
	if j < 0 {
		return ""
	}
	return strings.TrimSpace(s[:j])
}

// worktree の中で起動したセッションも、元のリポの名前で一覧に出す。
// cwd が worktree の置き場そのもの（.../.claude/worktrees）のこともある。相手の端末にしか無い worktree を
// 手元にある祖先へ書き換えた記録がこの形になる。
func repoName(cwd string) string {
	if cwd == "" {
		return ""
	}
	const wt = "/.claude/worktrees"
	if i := strings.Index(cwd+"/", wt+"/"); i >= 0 {
		cwd = cwd[:i]
	}
	return filepath.Base(cwd)
}

// Issue 番号は、task/417-s3-journal-store のようなブランチ名か、worktree の名前の先頭の数字から取る。
// 数字の直後が "-<英字>" のときだけ番号とみなし、day/2026-10-08 のような日付は拾わない。
var issueRe = regexp.MustCompile(`(?:^|/)([0-9]+)-[A-Za-z]`)

func issueNumber(branch, cwd string) string {
	if m := issueRe.FindStringSubmatch(branch); m != nil {
		return m[1]
	}
	if _, wt, ok := strings.Cut(cwd, "/.claude/worktrees/"); ok {
		if m := issueRe.FindStringSubmatch(wt); m != nil {
			return m[1]
		}
	}
	return ""
}

// ScanAll は並列に読み、新しい順に返す。読めなかったファイルは飛ばす。
func ScanAll(paths []string) []Meta {
	out := make([]Meta, len(paths))
	ok := make([]bool, len(paths))
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i, p := range paths {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			m, err := Scan(p)
			if err == nil && m.Prompts > 0 {
				out[i], ok[i] = m, true
			}
		}()
	}
	wg.Wait()
	metas := out[:0]
	for i, m := range out {
		if ok[i] {
			metas = append(metas, m)
		}
	}
	sort.Slice(metas, func(i, j int) bool { return metas[i].Updated.After(metas[j].Updated) })
	return metas
}
