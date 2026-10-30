// gensessions はデモ GIF 用に、架空のセッションを config dir の形で書き出す。
// 一覧の更新日（*0・*1 など）が撮った日に合うよう、時刻は実行した時点から逆算する。
package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type turn struct{ prompt, reply string }

type demoSession struct {
	title  string
	repo   string
	branch string
	ageH   float64
	turns  []turn
}

var sessions = []demoSession{
	{"Lock invoices after month close", "app", "task/417-month-close", 0.5, []turn{
		{"Continue #417. Posted invoices must not change after the month is closed.", "Added a `closed_at` check to the invoice update path and a test for edits after close."},
		{"What happens to credit notes issued after close?", "They go into the next open month. I left the closed month untouched and noted it in the PR."},
	}},
	{"Rate limit the export endpoint", "api", "main", 3, []turn{
		{"The CSV export is hammering the DB. Add a rate limit.", "Added a token bucket per API key: 5 exports per minute."},
		{"Return Retry-After on 429.", "Done, with the seconds until the next token."},
	}},
	{"Fix flaky login test", "app", "task/402-flaky-login", 5, []turn{
		{"The login test fails about once in ten runs.", "It races the session cookie write. I wait for the redirect instead of sleeping."},
		{"Run it 50 times.", "50/50 passed."},
	}},
	{"Close and reopen a month", "app", "task/417-month-close", 26, []turn{
		{"Start #417: we need a way to close a month and reopen it by mistake.", "Proposed `close-month` and `reopen-month` commands with an audit log entry each."},
		{"Only admins can reopen.", "Reopen now checks the admin role and records who reopened it."},
	}},
	{"Rotate staging TLS certs", "infra", "main", 28, []turn{
		{"Staging certs expire next week.", "Renewed them and added a reminder 30 days before expiry."},
		{"Same for production?", "Production renews automatically; only staging was manual."},
	}},
	{"Invoice PDF layout tweaks", "app", "task/388-invoice-pdf", 50, []turn{
		{"Long customer names overflow the PDF header.", "Wrapped the name to two lines and shrank the font when it still overflows."},
		{"Check Japanese names too.", "Tested with full-width names; they wrap at the same width."},
	}},
	{"Paginate /v1/customers", "api", "task/395-paginate", 74, []turn{
		{"Customers list returns everything at once.", "Added cursor pagination with a default page size of 50."},
		{"Keep the old behavior behind a flag?", "Added `?all=true` for one release, marked deprecated."},
	}},
	{"Upgrade Postgres to 17", "infra", "main", 100, []turn{
		{"Plan the Postgres 17 upgrade.", "Wrote the steps: logical replica, cut over, keep the old primary for a day."},
		{"How long is the downtime?", "About a minute for the cutover."},
	}},
	{"CSV import: detect encoding", "app", "main", 120, []turn{
		{"Imports from Excel come in Shift_JIS sometimes.", "Detect the encoding from the BOM and fall back to Shift_JIS when UTF-8 decoding fails."},
		{"Add a test file.", "Added a small Shift_JIS fixture."},
	}},
	{"Tmux binding for the picker", "dotfiles", "main", 150, []turn{
		{"Bind prefix+h to open the session picker.", "Added a popup binding in tmux.conf."},
		{"Make it full screen.", "Switched to a new window instead of a popup."},
	}},
	{"Webhook retries with backoff", "api", "task/377-webhook-retry", 200, []turn{
		{"Failed webhooks are dropped.", "Queued them with exponential backoff up to 6 tries."},
		{"Log the final failure.", "The last failure is logged with the response body."},
	}},
	{"Backup restore drill", "infra", "main", 300, []turn{
		{"Let's restore last night's backup to a scratch DB.", "Restored in 7 minutes; row counts match production."},
		{"Write it down.", "Added the steps to the runbook."},
	}},
	{"Dark mode for the settings page", "app", "task/360-dark-mode", 400, []turn{
		{"Settings page ignores dark mode.", "It used hard-coded colors; switched them to the theme tokens."},
		{"Check the date picker.", "The date picker follows the theme now."},
	}},
	{"Shell prompt shows git status", "dotfiles", "main", 450, []turn{
		{"Show the branch and dirty state in the prompt.", "Added a git segment that shows the branch and a dot when dirty."},
		{"It's slow in big repos.", "Skipped untracked files in the dirty check."},
	}},
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: gensessions <config dir>")
		os.Exit(2)
	}
	root := filepath.Join(os.Args[1], "projects")
	now := time.Now().UTC()
	for i, s := range sessions {
		id := uuid(s.title)
		if i == 0 {
			// README の例と同じ ID にそろえる
			id = "1c0e662e" + id[8:]
		}
		cwd := "/home/alice/src/" + s.repo
		if s.branch != "main" {
			cwd += "/.claude/worktrees/" + strings.TrimPrefix(s.branch, "task/")
		}
		dir := filepath.Join(root, strings.ReplaceAll(cwd, "/", "-"))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fail(err)
		}
		end := now.Add(-time.Duration(s.ageH * float64(time.Hour)))
		t := end.Add(-time.Duration(len(s.turns)*10) * time.Minute)
		var b strings.Builder
		write := func(v map[string]any) {
			v["sessionId"], v["cwd"], v["gitBranch"] = id, cwd, s.branch
			line, _ := json.Marshal(v)
			b.Write(line)
			b.WriteByte('\n')
		}
		for _, tr := range s.turns {
			write(map[string]any{"type": "user", "timestamp": t.Format(time.RFC3339),
				"message": map[string]any{"role": "user", "content": tr.prompt}})
			t = t.Add(5 * time.Minute)
			write(map[string]any{"type": "assistant", "timestamp": t.Format(time.RFC3339),
				"message": map[string]any{"role": "assistant", "content": []map[string]string{{"type": "text", "text": tr.reply}}}})
			t = t.Add(5 * time.Minute)
		}
		write(map[string]any{"type": "ai-title", "aiTitle": s.title})
		p := filepath.Join(dir, id+".jsonl")
		if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
			fail(err)
		}
		if err := os.Chtimes(p, end, end); err != nil {
			fail(err)
		}
	}
}

// uuid はタイトルから UUID の形の ID を作る。撮り直しても同じ ID になる
func uuid(title string) string {
	h := sha256.Sum256([]byte(title))
	x := fmt.Sprintf("%x", h[:16])
	return x[:8] + "-" + x[8:12] + "-4" + x[13:16] + "-8" + x[17:20] + "-" + x[20:32]
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
