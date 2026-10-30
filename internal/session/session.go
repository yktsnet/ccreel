package session

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
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
	// claude -p で動かしたセッション。人が会話したものではないので一覧に出さない
	Headless bool `json:"-"`
	// ScanAllTerms に渡した語ごとの、語を含む発言と文章の応答の数。ツールの入出力は数えない
	Hits map[string]int `json:"-"`
}

// Discover は projects 直下のセッションだけを返す。<uuid>/subagents などの付属物は一覧に出さない。
func Discover(configDir string) ([]string, error) {
	return filepath.Glob(filepath.Join(configDir, "projects", "*", "*.jsonl"))
}

// ErrNotFound は、ID に当たるセッションが無いとき。
var ErrNotFound = errors.New("no session matches")

// Find は UUID の先頭の数文字からセッションを引く。候補が複数なら誤って開かないよう失敗させる。
// 同じ UUID が複数のプロジェクトにあるのは、途中で別のリポへ移ったセッションで、更新の新しい方を返す。
func Find(configDir, prefix string) (string, error) {
	if prefix == "" || strings.Trim(prefix, "0123456789abcdef-") != "" {
		return "", fmt.Errorf("invalid session id %q", prefix)
	}
	paths, err := filepath.Glob(filepath.Join(configDir, "projects", "*", prefix+"*.jsonl"))
	if err != nil {
		return "", err
	}
	if len(paths) == 0 {
		return "", fmt.Errorf("%w %q in %s", ErrNotFound, prefix, configDir)
	}
	var ids []string
	for _, p := range paths {
		if id := sessionID(p); !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	if len(ids) > 1 {
		return "", fmt.Errorf("%q matches %d sessions: %s", prefix, len(ids), strings.Join(ids, ", "))
	}
	return newest(paths), nil
}

func sessionID(path string) string {
	return strings.TrimSuffix(filepath.Base(path), ".jsonl")
}

func newest(paths []string) string {
	var best string
	var bt time.Time
	for _, p := range paths {
		if fi, err := os.Stat(p); err == nil && (best == "" || fi.ModTime().After(bt)) {
			best, bt = p, fi.ModTime()
		}
	}
	if best == "" {
		return paths[0]
	}
	return best
}

type record struct {
	Type        string          `json:"type"`
	IsMeta      bool            `json:"isMeta"`
	IsSidechain bool            `json:"isSidechain"`
	CWD         string          `json:"cwd"`
	GitBranch   string          `json:"gitBranch"`
	Entrypoint  string          `json:"entrypoint"`
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
	return scan(path, nil)
}

// scan は terms があれば、同じ読み出しの中で Meta.Hits を数える。数えるために読み直すと find が遅くなる
func scan(path string, terms []string) (Meta, error) {
	f, err := os.Open(path)
	if err != nil {
		return Meta{}, err
	}
	defer f.Close()

	m := Meta{
		ID:      sessionID(path),
		Path:    path,
		Project: filepath.Base(filepath.Dir(path)),
	}
	var firstCWD string
	var branch, cwd string
	lower := make([]string, len(terms))
	for i, t := range terms {
		lower[i] = strings.ToLower(t)
	}
	counting := len(terms) > 0
	if counting {
		m.Hits = map[string]int{}
	}
	hit := func(text string) {
		text = strings.ToLower(text)
		for i, t := range lower {
			if strings.Contains(text, t) {
				m.Hits[terms[i]]++
			}
		}
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
			m.CWD = expandHome(r.CWD)
			if firstCWD == "" {
				firstCWD = m.CWD
			}
		}
		if r.Entrypoint == "sdk-cli" {
			m.Headless = true
		}
		if r.GitBranch != "" {
			m.Branch = r.GitBranch
		}
		// PR を出して worktree を消すと最後は main に戻るので、途中で現れた番号を残す
		if m.Branch != branch || m.CWD != cwd {
			branch, cwd = m.Branch, m.CWD
			if n := issueNumber(branch, cwd); n != "" {
				m.Issue = n
			}
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
				if counting {
					hit(text)
				}
			}
		case "assistant":
			// ReadTurns と同じく、最初の発言より前の応答とサブエージェント側の応答は数えない
			if counting && m.Prompts > 0 && !r.IsSidechain {
				for _, b := range contentBlocks(r.Message) {
					if b.Type == "text" {
						hit(b.Text)
					}
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
	m.Repo = repoName(repoCWD(firstCWD, m.CWD))
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
		// 画像を添えた発言は image ブロックと text ブロックが並ぶ。tool_result を含むものだけが発言でない
		for _, b := range blocks {
			switch b.Type {
			case "text":
				text += b.Text
			case "tool_result":
				return "", false
			}
		}
	}
	text = commandText(strings.TrimSpace(text))
	for _, p := range notPrompts {
		if strings.HasPrefix(text, p) {
			return "", false
		}
	}
	return text, text != ""
}

// 人の発言と同じ形で記録されるが、人が打ったものではない
var notPrompts = []string{
	// compact のあとに自動で入る要約
	"This session is being continued from a previous conversation",
	// 応答を止めたときの印。`[Request interrupted by user for tool use]` もある
	"[Request interrupted by user",
}

// スラッシュコマンドは <command-name> と <command-args> のタグで記録されるので、打った形に戻す。
// <local-command-stdout> などコマンドの出力は発言ではないので落とす。
func commandText(s string) string {
	// 貼り付けた文章はタグで包まれるが、人の発言である
	if !strings.HasPrefix(s, "<") || strings.HasPrefix(s, "<pasted_content") {
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

// worktree の中で起動したセッションも、元のリポの名前で一覧に出す。worktree の置き場は、
// リポの中の .claude/worktrees/<名前> と、リポの隣の <リポ>.wt/<名前> の2つの形を扱う。
// cwd が置き場そのもの（.../.claude/worktrees や .../<リポ>.wt）のこともある。相手の端末にしか無い worktree を
// 手元にある祖先へ書き換えた記録がこの形になる。
func repoName(cwd string) string {
	if cwd == "" {
		return ""
	}
	const wt = "/.claude/worktrees"
	if i := strings.Index(cwd+"/", wt+"/"); i >= 0 {
		cwd = cwd[:i]
	}
	if dir, _, ok := siblingWorktree(cwd); ok {
		return strings.TrimSuffix(filepath.Base(dir), siblingSuffix)
	}
	return filepath.Base(cwd)
}

const siblingSuffix = ".wt"

// siblingWorktree は cwd が <リポ>.wt か <リポ>.wt/<名前>（その下も含む）にあるとき、
// <リポ>.wt のパスと worktree の名前を返す。
func siblingWorktree(cwd string) (dir, name string, ok bool) {
	parts := strings.Split(filepath.Clean(cwd), "/")
	for i, p := range parts {
		if strings.HasSuffix(p, siblingSuffix) && len(p) > len(siblingSuffix) {
			if i+1 < len(parts) {
				name = parts[i+1]
			}
			return strings.Join(parts[:i+1], "/"), name, true
		}
	}
	return "", "", false
}

// ハブの jsonl は cwd の先頭のホームを "~" にして持つ。ハブを直に読むときも、pull した写しと同じ cwd にする
func expandHome(cwd string) string {
	if cwd != "~" && !strings.HasPrefix(cwd, "~/") {
		return cwd
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return cwd
	}
	return strings.TrimRight(home, "/") + cwd[1:]
}

// リポは起動した場所で決める。途中で別のリポへ cd しても、会話の主題は起動したリポにある。
// ホームで起動してからリポへ移ったときは、ホームでは何も分からないので最後の場所を使う。
func repoCWD(first, last string) string {
	home, _ := os.UserHomeDir()
	if first == "" || (home != "" && filepath.Clean(first) == filepath.Clean(home)) {
		return last
	}
	return first
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
	if _, wt, ok := siblingWorktree(cwd); ok {
		if m := issueRe.FindStringSubmatch(wt); m != nil {
			return m[1]
		}
	}
	return ""
}

// ScanAll は並列に読み、新しい順に返す。読めなかったファイルと、人の発言が無いセッションは飛ばす。
// 同じ UUID が複数のプロジェクトにあれば、更新の新しい方だけを残す。
func ScanAll(paths []string) []Meta {
	return ScanAllTerms(paths, nil)
}

// ScanAllTerms は ScanAll と同じ一覧を返し、各行の Hits に語ごとの当たり数を入れる。
func ScanAllTerms(paths []string, terms []string) []Meta {
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
			m, err := scan(p, terms)
			if err == nil && m.Prompts > 0 && !m.Headless {
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
	sort.SliceStable(metas, func(i, j int) bool { return metas[i].Updated.After(metas[j].Updated) })
	seen := map[string]bool{}
	uniq := metas[:0]
	for _, m := range metas {
		if !seen[m.ID] {
			seen[m.ID] = true
			uniq = append(uniq, m)
		}
	}
	return uniq
}
