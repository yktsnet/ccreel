// Package picker は fzf で一覧から1本を選ばせ、選んだセッションを resume する。
package picker

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/yktsnet/ccreel/internal/session"
)

const (
	dim   = "\x1b[2m"
	reset = "\x1b[0m"
)

// Line は fzf に渡す1行で、列は「UUID・題・@リポ #番号・いつ」。UUID は画面に出さず、選ばれた行から ID を取り出すのに使う。
// 狭いペインでもまず題が見えるよう題を先頭に置き、リポと日時は後ろに薄く出す。
// fzf の中で @リポ・#番号 と打てば、その語にだけ当たる。題の中の同じ言葉と混ざらない
func Line(m session.Meta, now time.Time) string {
	title := m.Title
	if title == "" {
		title = m.FirstPrompt
	}
	title = strings.Join(strings.Fields(title), " ")
	var tags []string
	if m.Repo != "" {
		tags = append(tags, "@"+m.Repo)
	}
	if m.Issue != "" {
		tags = append(tags, "#"+m.Issue)
	}
	return fmt.Sprintf("%s\t%s\t%s%s%s\t%s%s%s", m.ID, title, dim, strings.Join(tags, " "), reset, dim, Ago(m.Updated, now), reset)
}

// Ago は一覧に出す日の目安。探すときに要るのは日付そのものより「今日か昨日か、何日前か」なので、
// 時刻ではなく手元の暦の日で数える。30日を超えたら月日にする
func Ago(t, now time.Time) string {
	y1, m1, d1 := t.Local().Date()
	y2, m2, d2 := now.Local().Date()
	days := int(time.Date(y2, m2, d2, 0, 0, 0, 0, time.UTC).Sub(time.Date(y1, m1, d1, 0, 0, 0, 0, time.UTC)).Hours() / 24)
	switch {
	case days <= 0:
		return "TDY"
	case days == 1:
		return "YDY"
	case days <= 30:
		return fmt.Sprintf("%dd", days)
	}
	return t.Local().Format("01-02")
}

type Options struct {
	Self      string   // プレビューで呼ぶ ccreel 自身のパス
	ConfigDir string   // プレビューの ccreel に渡す
	Query     string   // fzf の初期の絞り込み
	Extra     []string // CCREEL_FZF_OPTS から足す fzf の引数
}

// ErrCancelled は fzf を Esc などで閉じたとき。
var ErrCancelled = errors.New("cancelled")

// Pick は fzf を開き、選ばれたセッションの UUID を返す。
func Pick(metas []session.Meta, o Options) (string, error) {
	if len(metas) == 0 {
		return "", fmt.Errorf("no sessions in %s", o.ConfigDir)
	}
	var in bytes.Buffer
	now := time.Now()
	for _, m := range metas {
		in.WriteString(Line(m, now))
		in.WriteByte('\n')
	}
	preview := fmt.Sprintf("%s preview {1} --config-dir %s", shellQuote(o.Self), shellQuote(o.ConfigDir))
	// プレビューは Markdown なので、bat があれば色を付けて読ませる
	if bat, err := exec.LookPath("bat"); err == nil {
		preview += " | " + shellQuote(bat) + " --language md --style plain --color always --paging never --theme ansi"
	}
	args := []string{
		// --no-sort で、絞り込んでも渡した順（新しい順か find の順位）を保つ。
		// --nth で、題と @リポ #番号 だけを絞り込みの対象にする。日時の列に「12」が当たらないように
		"--delimiter", "\t", "--with-nth", "2..", "--nth", "1,2", "--no-sort", "--ansi", "--tabstop", "2",
		// 全画面で開く。--height の表示は閉じたあとも描いたものが残り、resume した claude の画面と重なる
		"--layout", "reverse", "--no-height",
		// 幅が足りない端末では、題を切らないようにプレビューを下へ回す。fzf の <N は端末ではなくプレビュー欄の幅で比べる
		"--preview", preview, "--preview-window", "right,55%,wrap,<60(down,45%,wrap)",
		"--bind", "ctrl-/:toggle-preview",
		"--header", "enter: resume · ctrl-/: preview · @repo #issue narrows",
		"--query", o.Query,
	}
	args = append(args, o.Extra...)
	cmd := exec.Command("fzf", args...)
	cmd.Stdin = &in
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	var ee *exec.ExitError
	// fzf は何も選ばずに閉じると 130、一致が無いと 1 を返す
	if errors.As(err, &ee) && (ee.ExitCode() == 130 || ee.ExitCode() == 1) {
		return "", ErrCancelled
	}
	if err != nil {
		return "", fmt.Errorf("fzf: %w", err)
	}
	id, _, _ := strings.Cut(strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0]), "\t")
	if id == "" {
		return "", ErrCancelled
	}
	return id, nil
}

// ResumeDir は resume するときに入るディレクトリを決める。記録された cwd が手元に無ければ
// （相手の端末にしか無い worktree など）、手元にある一番近い祖先を使う。claude --resume は
// 別のディレクトリからでも元の jsonl を開いて追記するので、jsonl は動かさない。
// 祖先がホームそのものか、ホームより上（/home や /）になるなら、ホームで開く。
func ResumeDir(cwd, home string) string {
	home = filepath.Clean(home)
	for d := filepath.Clean(cwd); cwd != ""; d = filepath.Dir(d) {
		if fi, err := os.Stat(d); err == nil && fi.IsDir() {
			if d != filepath.Clean(cwd) && strings.HasPrefix(home+"/", d+"/") {
				return home
			}
			return d
		}
		if d == filepath.Dir(d) {
			break
		}
	}
	return home
}

// Resume は dir に移って claude --resume を exec する。ccreel のプロセスは claude に置き換わる。
func Resume(claude, id, dir string, extra, env []string) error {
	path, err := exec.LookPath(claude)
	if err != nil {
		return err
	}
	if err := os.Chdir(dir); err != nil {
		return err
	}
	// fzf が描いたものが端末に残っていると、claude の信頼の確認のように画面を消さずに描く表示が重なって崩れる。
	// claude に渡す前に画面を消す
	if tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0); err == nil {
		tty.WriteString("\x1b[H\x1b[2J\x1b[3J")
		tty.Close()
	}
	argv := append([]string{filepath.Base(claude), "--resume", id}, extra...)
	return syscall.Exec(path, argv, env)
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
