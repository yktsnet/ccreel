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

	"github.com/yktsnet/ccreel/internal/session"
)

// Line は fzf に渡す1行。先頭の列は UUID 全体で、画面には出さず、選ばれた行から ID を取り出すのに使う。
func Line(m session.Meta) string {
	title := m.Title
	if title == "" {
		title = m.FirstPrompt
	}
	title = strings.Join(strings.Fields(title), " ")
	repo := m.Repo
	if repo == "" {
		repo = "-"
	}
	return fmt.Sprintf("%s\t%s\t%-18s\t%s", m.ID, m.Updated.Local().Format("01-02 15:04"), repo, title)
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
	for _, m := range metas {
		in.WriteString(Line(m))
		in.WriteByte('\n')
	}
	preview := fmt.Sprintf("%s show {1} --config-dir %s --last 4 --final --max-chars 800",
		shellQuote(o.Self), shellQuote(o.ConfigDir))
	args := []string{
		"--delimiter", "\t", "--with-nth", "2..", "--no-sort",
		"--layout", "reverse", "--height", "100%",
		"--preview", preview, "--preview-window", "right,55%,wrap",
		"--bind", "ctrl-/:toggle-preview",
		"--header", "Enter: resume  /  Ctrl-/: toggle preview",
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
	argv := append([]string{filepath.Base(claude), "--resume", id}, extra...)
	return syscall.Exec(path, argv, env)
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
