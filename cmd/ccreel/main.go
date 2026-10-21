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
  list    List sessions, newest first

Run 'ccreel <command> -h' for flags.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "list":
		err = runList(os.Args[2:], os.Stdout)
	case "-h", "--help", "help":
		fmt.Print(usage)
		return
	default:
		fmt.Fprintf(os.Stderr, "ccreel: unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
	if err != nil {
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

func runList(args []string, w io.Writer) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	configDir := fs.String("config-dir", defaultConfigDir(), "Claude Code config dir")
	asJSON := fs.Bool("json", false, "print JSON Lines")
	limit := fs.Int("limit", 50, "max sessions to print (0 = no limit)")
	since := fs.String("since", "", "only sessions updated within this period (e.g. 12h, 7d)")
	repo := fs.String("repo", "", "only sessions whose repo name contains this")
	if err := fs.Parse(args); err != nil {
		return err
	}

	var cutoff time.Time
	if *since != "" {
		d, err := parseSince(*since)
		if err != nil {
			return err
		}
		cutoff = time.Now().Add(-d)
	}

	paths, err := session.Discover(*configDir)
	if err != nil {
		return err
	}
	metas := session.ScanAll(paths)

	enc := json.NewEncoder(w)
	n := 0
	for _, m := range metas {
		if !cutoff.IsZero() && m.Updated.Before(cutoff) {
			break
		}
		if *repo != "" && !strings.Contains(m.Repo, *repo) {
			continue
		}
		if *limit > 0 && n >= *limit {
			break
		}
		n++
		if *asJSON {
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
