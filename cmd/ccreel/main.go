package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/yktsnet/ccreel/internal/finder"
	"github.com/yktsnet/ccreel/internal/hubsync"
	"github.com/yktsnet/ccreel/internal/picker"
	"github.com/yktsnet/ccreel/internal/session"
)

const usage = `Usage: ccreel [command] [flags]

Commands:
  pick [query]      Pull, choose a session in fzf, and resume it (default command)
  resume <id>       Resume a session in its recorded directory
  list              List sessions, newest first
  show <id>         Print the turns of a session
  outline <id>      Print one line per turn of a session
  preview <id>      Print a session summary in Markdown (used by the picker)
  find <words>      Rank sessions by #issue, @repo and words; says whether one stands out
  search <query>    Find where a phrase appears across sessions
  push              Send local sessions to the hub (the first push sends all of them)
  pull              Fetch recent sessions from the hub into the local config dir

<id> may be any unique prefix of the session UUID.
The hub is an rsync destination such as host:path, given by --hub or CCREEL_HUB.
Arguments after -- in pick and resume are passed to claude.
CCREEL_CONFIG_DIRS (colon-separated) adds config dirs that every command except push and pull also looks in.
list and pick hide sessions with a single prompt; --all shows them.
Run 'ccreel <command> -h' for flags.
`

func main() {
	args := os.Args[1:]
	if len(args) == 0 || strings.HasPrefix(args[0], "-") && args[0] != "-h" && args[0] != "--help" {
		args = append([]string{"pick"}, args...)
	}
	cmds := map[string]func([]string, io.Writer, io.Writer) error{
		"pick":    runPick,
		"resume":  runResume,
		"list":    runList,
		"show":    runShow,
		"outline": runOutline,
		"preview": runPreview,
		"search":  runSearch,
		"find":    runFind,
		"push":    runPush,
		"pull":    runPull,
	}
	name := args[0]
	if name == "-h" || name == "--help" || name == "help" {
		fmt.Print(usage)
		return
	}
	run, ok := cmds[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "ccreel: unknown command %q\n\n%s", name, usage)
		os.Exit(2)
	}
	if err := run(args[1:], os.Stdout, os.Stderr); err != nil {
		if err == flag.ErrHelp || err == picker.ErrCancelled {
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
	since          string
	repo           string
	issue          string
	includeCurrent bool
	all            bool // list と pick だけが立てる。立てないと発言が1回のセッションを外す
	oneShotFlag    bool // --all を受け付けるか
}

func (lf *listFilter) register(fs *flag.FlagSet) {
	fs.StringVar(&lf.since, "since", "", "only sessions updated within this period (e.g. 12h, 7d)")
	fs.StringVar(&lf.repo, "repo", "", "only sessions in this repo (exact name; worktrees count as their repo)")
	fs.StringVar(&lf.issue, "issue", "", "only sessions for this issue number (taken from the branch or worktree name)")
	fs.BoolVar(&lf.includeCurrent, "include-current", false, "keep the session running this command (CLAUDE_CODE_SESSION_ID)")
}

// registerAll は list と pick に --all を足す。発言が1回で終わったセッションは、試しに打っただけのものが多く、
// 一覧で探すときには邪魔になる。言葉で探す find と search は中身で当てるので、外さない
func (lf *listFilter) registerAll(fs *flag.FlagSet) {
	lf.oneShotFlag = true
	fs.BoolVar(&lf.all, "all", false, "also show sessions with a single prompt")
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
	// Claude Code の中から呼ばれると、いま話しているセッションが一番新しい行になる。
	// 「直前のセッション」を引くときに自分自身を返さないよう、既定で外す
	current := ""
	if !lf.includeCurrent {
		current = os.Getenv("CLAUDE_CODE_SESSION_ID")
	}
	hideOneShot := lf.oneShotFlag && !lf.all
	return func(m session.Meta) bool {
		if hideOneShot && m.Prompts < 2 {
			return false
		}
		if current != "" && m.ID == current {
			return false
		}
		if !cutoff.IsZero() && m.Updated.Before(cutoff) {
			return false
		}
		if lf.issue != "" && m.Issue != strings.TrimPrefix(lf.issue, "#") {
			return false
		}
		return lf.repo == "" || m.Repo == lf.repo
	}, nil
}

func runList(args []string, w, _ io.Writer) error {
	var c common
	var lf listFilter
	fs := newFlags("list", &c)
	lf.register(fs)
	lf.registerAll(fs)
	limit := fs.Int("limit", 50, "max sessions to print (0 = no limit)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	keep, err := lf.build()
	if err != nil {
		return err
	}
	paths, err := discoverAll(configDirs(c.configDir))
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
		if m.Issue != "" {
			title = "#" + m.Issue + " " + title
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
	path, _, err := findSession(c.configDir, id)
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
	path, _, err := findSession(c.configDir, id)
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
	var paths []string
	for _, d := range configDirs(c.configDir) {
		p, err := session.Candidates(d, query)
		if err != nil {
			return err
		}
		paths = append(paths, p...)
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

func syncFlags(name string) (*flag.FlagSet, func() (hubsync.Options, error)) {
	var c common
	fs := newFlags(name, &c)
	hub := fs.String("hub", os.Getenv("CCREEL_HUB"), "rsync destination of the hub, e.g. host:path")
	mirror := fs.String("mirror", "", "local copy in hub form (default: $XDG_CACHE_HOME/ccreel/<config dir name>)")
	pullDays := fs.Int("pull-days", 30, "pull sessions updated within this many days")
	keepDays := fs.Int("keep-days", 40, "delete sessions older than this from the hub and the mirror")
	return fs, func() (hubsync.Options, error) {
		h, err := hubsync.ParseHub(*hub)
		if err != nil {
			return hubsync.Options{}, err
		}
		if *keepDays <= *pullDays {
			return hubsync.Options{}, fmt.Errorf("--keep-days (%d) must be greater than --pull-days (%d)", *keepDays, *pullDays)
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return hubsync.Options{}, err
		}
		m := *mirror
		if m == "" {
			cache, err := os.UserCacheDir()
			if err != nil {
				return hubsync.Options{}, err
			}
			m = filepath.Join(cache, "ccreel", filepath.Base(filepath.Clean(c.configDir)))
		}
		return hubsync.Options{ConfigDir: c.configDir, Home: home, Mirror: m, Hub: h, PullDays: *pullDays, KeepDays: *keepDays}, nil
	}
}

// タイマーから毎分呼ばれる前提で、送るものが無い回と、別の push・pull と重なった回は何も出さずに終える。
func runPush(args []string, w, _ io.Writer) error {
	fs, opts := syncFlags("push")
	force := fs.Bool("force", false, "send the mirror to the hub even if nothing changed")
	if err := fs.Parse(args); err != nil {
		return err
	}
	o, err := opts()
	if err != nil {
		return err
	}
	o.Force = *force
	st, err := hubsync.Push(o)
	if err == hubsync.ErrBusy {
		return nil
	}
	if err != nil {
		return err
	}
	if st.Sent {
		fmt.Fprintf(w, "pushed to %s (%d files updated)\n", o.Hub, st.Staged)
	}
	return nil
}

func runPull(args []string, w, _ io.Writer) error {
	fs, opts := syncFlags("pull")
	if err := fs.Parse(args); err != nil {
		return err
	}
	o, err := opts()
	if err != nil {
		return err
	}
	st, err := hubsync.Pull(o)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "pulled from %s (%d files updated)\n", o.Hub, st.Materialize)
	return nil
}

// splitPassthrough は -- より後ろを claude へ渡す引数として切り離す。
func splitPassthrough(args []string) (own, rest []string) {
	for i, a := range args {
		if a == "--" {
			return args[:i], args[i+1:]
		}
	}
	return args, nil
}

func runPick(args []string, _, errw io.Writer) error {
	args, extra := splitPassthrough(args)
	fs, opts := syncFlags("pick")
	noPull := fs.Bool("no-pull", false, "skip pulling from the hub")
	var lf listFilter
	lf.register(fs)
	lf.registerAll(fs)
	var words []string
	for len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		words, args = append(words, args[0]), args[1:]
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	configDir := fs.Lookup("config-dir").Value.String()
	// CCREEL_CONFIG_DIRS の config dir もまとめて一覧に出す。どれで開くかは resume がセッションの置き場から決める
	dirs := configDirs(configDir)
	o, err := opts()
	// ハブが未設定でも、手元の履歴だけで選べるようにする
	if err == nil && !*noPull {
		for _, d := range dirs {
			if _, err := hubsync.Pull(withConfigDir(o, configDir, d)); err != nil {
				fmt.Fprintf(errw, "ccreel: pull of %s failed, showing local sessions only: %v\n", d, err)
			}
		}
	}
	keep, err := lf.build()
	if err != nil {
		return err
	}

	var metas []session.Meta
	if len(words) > 0 {
		// 言葉があれば find と同じ順位で並べる。fzf に出る最初の数行が、Claude が find で選ぶ候補と同じになる
		res, err := finder.Find(finder.ParseQuery(words), finder.Options{
			ConfigDirs: dirs, CurrentRepo: currentRepo(), Exclude: os.Getenv("CLAUDE_CODE_SESSION_ID"),
		})
		if err != nil {
			return err
		}
		for _, c := range res.Candidates {
			if keep(c.Meta) {
				metas = append(metas, c.Meta)
			}
		}
	} else {
		paths, err := discoverAll(dirs)
		if err != nil {
			return err
		}
		for _, m := range session.ScanAll(paths) {
			if keep(m) {
				metas = append(metas, m)
			}
		}
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	ids, err := picker.Pick(metas, picker.Options{
		Self: self, ConfigDir: configDir, Extra: strings.Fields(os.Getenv("CCREEL_FZF_OPTS")),
	})
	if err != nil {
		return err
	}
	// 2本目からは tmux の別のウィンドウで開き、1本目はこのペインで開く。resume は exec で戻らないので先に開いておく
	if len(ids) > 1 {
		if os.Getenv("TMUX") == "" {
			fmt.Fprintf(errw, "ccreel: opening several sessions needs tmux; resuming only the first\n")
		} else {
			for _, id := range ids[1:] {
				if err := openWindow(configDir, id, extra); err != nil {
					fmt.Fprintf(errw, "ccreel: %s: %v\n", id, err)
				}
			}
		}
	}
	return resume(configDir, ids[0], extra, errw)
}

// withConfigDir は primary 向けに組んだ同期の設定を、別の config dir 向けに直す。
// mirror は config dir の名前で分けているので、既定の置き場なら名前だけ差し替える。
func withConfigDir(o hubsync.Options, primary, dir string) hubsync.Options {
	if filepath.Clean(dir) == filepath.Clean(primary) {
		return o
	}
	if filepath.Base(o.Mirror) == filepath.Base(filepath.Clean(primary)) {
		o.Mirror = filepath.Join(filepath.Dir(o.Mirror), filepath.Base(filepath.Clean(dir)))
	}
	o.ConfigDir = dir
	return o
}

func runResume(args []string, _, errw io.Writer) error {
	args, extra := splitPassthrough(args)
	var c common
	fs := newFlags("resume", &c)
	id, err := parseWithArg(fs, args, "session id")
	if err != nil {
		return err
	}
	return resume(c.configDir, id, extra, errw)
}

func resume(configDir, id string, extra []string, errw io.Writer) error {
	path, configDir, err := findSession(configDir, id)
	if err != nil {
		return err
	}
	m, err := session.Scan(path)
	if err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	dir := picker.ResumeDir(m.CWD, home)
	if dir != m.CWD {
		fmt.Fprintf(errw, "ccreel: %s is not here; resuming in %s\n", m.CWD, dir)
	}
	return picker.Resume(claudeCommand(), m.ID, dir, extra, claudeEnv(configDir))
}

func openWindow(configDir, id string, extra []string) error {
	path, configDir, err := findSession(configDir, id)
	if err != nil {
		return err
	}
	m, err := session.Scan(path)
	if err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	var setEnv []string
	for _, kv := range claudeEnv(configDir) {
		if strings.HasPrefix(kv, "CLAUDE_CONFIG_DIR=") {
			setEnv = append(setEnv, kv)
		}
	}
	return picker.OpenWindow(claudeCommand(), m.ID, picker.ResumeDir(m.CWD, home), extra, setEnv)
}

func claudeCommand() string {
	if c := os.Getenv("CCREEL_CLAUDE"); c != "" {
		return c
	}
	return "claude"
}

// claude には選んだのと同じ config dir を見せる。既定の ~/.claude のときに CLAUDE_CONFIG_DIR を新たに
// 立てると、macOS では認証情報の置き場の名前が変わるので、既定のときは環境をそのまま渡す。
func claudeEnv(configDir string) []string {
	env := os.Environ()
	home, _ := os.UserHomeDir()
	if os.Getenv("CLAUDE_CONFIG_DIR") == "" && filepath.Clean(configDir) == filepath.Join(home, ".claude") {
		return env
	}
	// 同じ名前が2つあると先のものが読まれるので、元の値を除いてから足す
	out := env[:0:0]
	for _, kv := range env {
		if !strings.HasPrefix(kv, "CLAUDE_CONFIG_DIR=") {
			out = append(out, kv)
		}
	}
	return append(out, "CLAUDE_CONFIG_DIR="+configDir)
}

// runPreview は picker の右のペインに出す Markdown を書く。上にセッションの素性をまとめ、
// その下に新しいターンから順に、発言と最終応答を並べる。どこまで進んだかを先に見せるため
func runPreview(args []string, w, _ io.Writer) error {
	var c common
	fs := newFlags("preview", &c)
	turns := fs.Int("turns", 5, "number of recent turns to show")
	id, err := parseWithArg(fs, args, "session id")
	if err != nil {
		return err
	}
	path, _, err := findSession(c.configDir, id)
	if err != nil {
		return err
	}
	m, err := session.Scan(path)
	if err != nil {
		return err
	}
	all, err := session.ReadTurns(path)
	if err != nil {
		return err
	}
	title := m.Title
	if title == "" {
		title = oneLine(m.FirstPrompt, 80)
	}
	fmt.Fprintf(w, "# %s\n\n", title)
	var meta []string
	if m.Repo != "" {
		meta = append(meta, "`@"+m.Repo+"`")
	}
	if m.Issue != "" {
		meta = append(meta, "`#"+m.Issue+"`")
	}
	if m.Branch != "" && m.Branch != "main" && m.Branch != "HEAD" {
		meta = append(meta, "`"+m.Branch+"`")
	}
	meta = append(meta, picker.Ago(m.Updated, time.Now())+" "+m.Updated.Local().Format("15:04"), fmt.Sprintf("%d prompts", m.Prompts))
	fmt.Fprintf(w, "%s  \n%s\n", strings.Join(meta, " · "), m.CWD)
	start := max(len(all)-*turns, 0)
	for i := len(all) - 1; i >= start; i-- {
		t := all[i]
		fmt.Fprintf(w, "\n---\n\n**turn %d** · %s %s\n\n> %s\n\n", t.Index, picker.Ago(t.Time, time.Now()), t.Time.Local().Format("15:04"),
			strings.ReplaceAll(truncate(t.Prompt, 400), "\n", "\n> "))
		if r := t.FinalText(); r != "" {
			fmt.Fprintln(w, truncate(r, 1500))
		}
	}
	return nil
}

// configDirs は primary を先頭に、CCREEL_CONFIG_DIRS の分を重ならないように足す。
func configDirs(primary string) []string {
	dirs := []string{filepath.Clean(primary)}
	for _, d := range filepath.SplitList(os.Getenv("CCREEL_CONFIG_DIRS")) {
		d = filepath.Clean(d)
		if d != "." && !slices.Contains(dirs, d) {
			dirs = append(dirs, d)
		}
	}
	return dirs
}

func discoverAll(dirs []string) ([]string, error) {
	var paths []string
	for _, d := range dirs {
		p, err := session.Discover(d)
		if err != nil {
			return nil, err
		}
		paths = append(paths, p...)
	}
	return paths, nil
}

// findSession は ID を全部の config dir から引き、見つかったパスと config dir を返す。
// resume はその config dir で claude を起動する。
func findSession(primary, id string) (string, string, error) {
	dirs := configDirs(primary)
	var path, dir string
	for _, d := range dirs {
		p, err := session.Find(d, id)
		if errors.Is(err, session.ErrNotFound) {
			continue
		}
		if err != nil {
			return "", "", err
		}
		if path != "" {
			return "", "", fmt.Errorf("%q matches sessions in both %s and %s", id, dir, d)
		}
		path, dir = p, d
	}
	if path == "" {
		return "", "", fmt.Errorf("%w: %q in %s", session.ErrNotFound, id, strings.Join(dirs, ", "))
	}
	return path, dir, nil
}

// currentRepo は呼ばれた場所のリポ名。worktree の中でも元のリポの名前を返す。git の外なら空。
func currentRepo() string {
	out, err := exec.Command("git", "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		return ""
	}
	return filepath.Base(filepath.Dir(strings.TrimSpace(string(out))))
}

func runFind(args []string, w, _ io.Writer) error {
	var c common
	fs := newFlags("find", &c)
	limit := fs.Int("limit", 8, "max candidates")
	var words []string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			return err
		}
		args = fs.Args()
		if len(args) > 0 {
			words, args = append(words, args[0]), args[1:]
		}
	}
	res, err := finder.Find(finder.ParseQuery(words), finder.Options{
		ConfigDirs: configDirs(c.configDir), CurrentRepo: currentRepo(),
		Exclude: os.Getenv("CLAUDE_CODE_SESSION_ID"), Limit: *limit,
	})
	if err != nil {
		return err
	}
	if c.asJSON {
		return json.NewEncoder(w).Encode(res)
	}
	switch {
	case len(res.Candidates) == 0:
		fmt.Fprintln(w, "no sessions found")
		return nil
	case res.Decided:
		fmt.Fprintln(w, "best match:")
	default:
		fmt.Fprintln(w, "several candidates; none stands out:")
	}
	for _, cd := range res.Candidates {
		title := cd.Title
		if title == "" {
			title = oneLine(cd.FirstPrompt, 60)
		}
		issue := ""
		if cd.Issue != "" {
			issue = "#" + cd.Issue
		}
		fmt.Fprintf(w, "%4d  %s  %s  @%-18s %-6s %s\n      %s\n", cd.Score, shortID(cd.ID), cd.Updated.Local().Format("01-02 15:04"),
			cd.Repo, issue, title, strings.Join(cd.Reasons, ", "))
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
