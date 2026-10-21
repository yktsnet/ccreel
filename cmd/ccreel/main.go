package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/yktsnet/ccreel/internal/session"
)

const usage = `Usage: ccreel <command> [flags]

Commands:
  list              List sessions, newest first
  show <id>         Print the turns of a session
  outline <id>      Print one line per turn of a session
  search <query>    Find where a phrase appears across sessions

<id> may be any unique prefix of the session UUID.
Run 'ccreel <command> -h' for flags.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	cmds := map[string]func([]string, io.Writer, io.Writer) error{
		"list":    runList,
		"show":    runShow,
		"outline": runOutline,
		"search":  runSearch,
	}
	name := os.Args[1]
	if name == "-h" || name == "--help" || name == "help" {
		fmt.Print(usage)
		return
	}
	run, ok := cmds[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "ccreel: unknown command %q\n\n%s", name, usage)
		os.Exit(2)
	}
	if err := run(os.Args[2:], os.Stdout, os.Stderr); err != nil {
		if err == flag.ErrHelp {
			return
		}
		fmt.Fprintf(os.Stderr, "ccreel: %v\n", err)
		os.Exit(1)
	}
}

// Claude Code と同じく CLAUDE_CONFIG_DIR を優先し、無ければ ~/.claude を見る。
func defaultConfigDir() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

type common struct {
	configDir string
	asJSON    bool
}

func newFlags(name string, c *common) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.StringVar(&c.configDir, "config-dir", defaultConfigDir(), "Claude Code config dir")
	fs.BoolVar(&c.asJSON, "json", false, "print JSON Lines")
	return fs
}

// flag パッケージは最初の位置引数で解析を止めるので、`show <id> --json` の順でも読めるよう位置引数を前から抜く。
func parseWithArg(fs *flag.FlagSet, args []string, what string) (string, error) {
	var pos []string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			return "", err
		}
		args = fs.Args()
		if len(args) > 0 {
			pos = append(pos, args[0])
			args = args[1:]
		}
	}
	if len(pos) != 1 {
		return "", fmt.Errorf("%s: expected exactly one %s", fs.Name(), what)
	}
	return pos[0], nil
}

type listFilter struct {
	since string
	repo  string
}

func (lf *listFilter) register(fs *flag.FlagSet) {
	fs.StringVar(&lf.since, "since", "", "only sessions updated within this period (e.g. 12h, 7d)")
	fs.StringVar(&lf.repo, "repo", "", "only sessions whose repo name contains this")
}

func (lf *listFilter) build() (func(session.Meta) bool, error) {
	var cutoff time.Time
	if lf.since != "" {
		d, err := parseSince(lf.since)
		if err != nil {
			return nil, err
		}
		cutoff = time.Now().Add(-d)
	}
	return func(m session.Meta) bool {
		if !cutoff.IsZero() && m.Updated.Before(cutoff) {
			return false
		}
		return lf.repo == "" || strings.Contains(m.Repo, lf.repo)
	}, nil
}

func runList(args []string, w, _ io.Writer) error {
	var c common
	var lf listFilter
	fs := newFlags("list", &c)
	lf.register(fs)
	limit := fs.Int("limit", 50, "max sessions to print (0 = no limit)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	keep, err := lf.build()
	if err != nil {
		return err
	}
	paths, err := session.Discover(c.configDir)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	n := 0
	for _, m := range session.ScanAll(paths) {
		if !keep(m) {
			continue
		}
		if *limit > 0 && n >= *limit {
			break
		}
		n++
		if c.asJSON {
			if err := enc.Encode(m); err != nil {
				return err
			}
			continue
		}
		title := m.Title
		if title == "" {
			title = oneLine(m.FirstPrompt, 80)
		}
		fmt.Fprintf(w, "%s  %s  %-20s  %s\n", shortID(m.ID), m.Updated.Local().Format("2006-01-02 15:04"), m.Repo, title)
	}
	return nil
}

func runShow(args []string, w, errw io.Writer) error {
	var c common
	fs := newFlags("show", &c)
	last := fs.Int("last", 0, "only the last N turns")
	turnsSpec := fs.String("turns", "", "only these turns, e.g. 3 or 3-7")
	final := fs.Bool("final", false, "only the prompt and the final reply of each turn")
	tools := fs.Bool("tools", false, "include tool calls and tool output")
	maxChars := fs.Int("max-chars", 2000, "truncate each message to this many characters (0 = no limit)")
	maxTotal := fs.Int("max-total", 40000, "stop after this many characters in total (0 = no limit)")
	id, err := parseWithArg(fs, args, "session id")
	if err != nil {
		return err
	}
	path, err := session.Find(c.configDir, id)
	if err != nil {
		return err
	}
	turns, err := session.ReadTurns(path)
	if err != nil {
		return err
	}
	if turns, err = selectTurns(turns, *turnsSpec, *last); err != nil {
		return err
	}

	enc := json.NewEncoder(w)
	total := 0
	for i, t := range turns {
		t = shapeTurn(t, *final, *tools, *maxChars)
		size := len([]rune(t.Prompt))
		for _, m := range t.Messages {
			size += len([]rune(m.Text))
		}
		// 1ターン目は上限を超えても出す。何も出ないと、上限を上げればよいのかが分からない
		if *maxTotal > 0 && i > 0 && total+size > *maxTotal {
			fmt.Fprintf(errw, "ccreel: stopped at --max-total %d; %d more turn(s) from turn %d (use --turns or raise --max-total)\n",
				*maxTotal, len(turns)-i, t.Index)
			break
		}
		total += size
		if c.asJSON {
			if err := enc.Encode(t); err != nil {
				return err
			}
			continue
		}
		fmt.Fprintf(w, "## turn %d  %s\n> %s\n", t.Index, t.Time.Local().Format("2006-01-02 15:04"), strings.ReplaceAll(t.Prompt, "\n", "\n> "))
		for _, m := range t.Messages {
			if m.Kind == "text" {
				fmt.Fprintf(w, "\n%s\n", m.Text)
			} else {
				fmt.Fprintf(w, "\n[%s %s] %s\n", m.Kind, m.Tool, m.Text)
			}
		}
		fmt.Fprintln(w)
	}
	return nil
}

func shapeTurn(t session.Turn, final, tools bool, maxChars int) session.Turn {
	t.Prompt = truncate(t.Prompt, maxChars)
	var msgs []session.Message
	if final {
		if f := t.FinalText(); f != "" {
			msgs = []session.Message{{Kind: "text", Text: f}}
		}
	} else {
		for _, m := range t.Messages {
			if m.Kind != "text" && !tools {
				continue
			}
			msgs = append(msgs, m)
		}
	}
	for i := range msgs {
		msgs[i].Text = truncate(msgs[i].Text, maxChars)
	}
	t.Messages = msgs
	return t
}

func selectTurns(turns []session.Turn, spec string, last int) ([]session.Turn, error) {
	if spec != "" {
		from, to, err := parseRange(spec)
		if err != nil {
			return nil, err
		}
		var out []session.Turn
		for _, t := range turns {
			if t.Index >= from && t.Index <= to {
				out = append(out, t)
			}
		}
		turns = out
	}
	if last > 0 && len(turns) > last {
		turns = turns[len(turns)-last:]
	}
	return turns, nil
}

func parseRange(spec string) (int, int, error) {
	a, b, isRange := strings.Cut(spec, "-")
	from, err := strconv.Atoi(a)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid --turns %q", spec)
	}
	if !isRange {
		return from, from, nil
	}
	to, err := strconv.Atoi(b)
	if err != nil || to < from {
		return 0, 0, fmt.Errorf("invalid --turns %q", spec)
	}
	return from, to, nil
}

type outlineRow struct {
	Turn   int       `json:"turn"`
	Time   time.Time `json:"time"`
	Prompt string    `json:"prompt"`
	Reply  string    `json:"reply"`
	Tools  int       `json:"tools"`
}

func runOutline(args []string, w, _ io.Writer) error {
	var c common
	fs := newFlags("outline", &c)
	width := fs.Int("width", 100, "characters kept from each prompt and reply")
	id, err := parseWithArg(fs, args, "session id")
	if err != nil {
		return err
	}
	path, err := session.Find(c.configDir, id)
	if err != nil {
		return err
	}
	turns, err := session.ReadTurns(path)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	for _, t := range turns {
		row := outlineRow{Turn: t.Index, Time: t.Time, Prompt: oneLine(t.Prompt, *width), Reply: oneLine(t.FinalText(), *width)}
		for _, m := range t.Messages {
			if m.Kind == "tool_use" {
				row.Tools++
			}
		}
		if c.asJSON {
			if err := enc.Encode(row); err != nil {
				return err
			}
			continue
		}
		fmt.Fprintf(w, "%3d  %s  %s\n     → %s\n", row.Turn, row.Time.Local().Format("01-02 15:04"), row.Prompt, row.Reply)
	}
	return nil
}

func runSearch(args []string, w, errw io.Writer) error {
	var c common
	var lf listFilter
	fs := newFlags("search", &c)
	lf.register(fs)
	limit := fs.Int("limit", 20, "max hits in total (0 = no limit)")
	perSession := fs.Int("per-session", 3, "max hits from one session (0 = no limit)")
	ctx := fs.Int("context", 80, "characters kept on each side of a hit")
	tools := fs.Bool("tools", false, "also search tool calls and tool output")
	query, err := parseWithArg(fs, args, "query")
	if err != nil {
		return err
	}
	keep, err := lf.build()
	if err != nil {
		return err
	}
	paths, err := session.Candidates(c.configDir, query)
	if err != nil {
		return err
	}
	hits, err := session.Search(paths, query, session.SearchOptions{
		Tools: *tools, Context: *ctx, Limit: *limit, PerSession: *perSession, Filter: keep,
	})
	if err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	for _, h := range hits {
		if c.asJSON {
			if err := enc.Encode(h); err != nil {
				return err
			}
			continue
		}
		fmt.Fprintf(w, "%s  turn %-3d %-11s %s\n    %s\n", shortID(h.ID), h.Turn, h.Kind, h.Repo, h.Snippet)
	}
	if len(hits) == 0 {
		fmt.Fprintf(errw, "ccreel: no hits for %q\n", query)
	}
	if *limit > 0 && len(hits) >= *limit {
		fmt.Fprintf(errw, "ccreel: stopped at --limit %d; narrow with --since/--repo or raise --limit\n", *limit)
	}
	return nil
}

// time.ParseDuration は日の単位を持たないので、末尾の d だけ自前で読む。
func parseSince(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, fmt.Errorf("invalid --since %q", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid --since %q", s)
	}
	return d, nil
}

func truncate(s string, max int) string {
	r := []rune(s)
	if max <= 0 || len(r) <= max {
		return s
	}
	return string(r[:max]) + fmt.Sprintf("… [truncated %d chars]", len(r)-max)
}

func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > max {
		return string(r[:max-1]) + "…"
	}
	return s
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
