package hubsync

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// Hub は rsync の宛先。"host:path" なら ssh の先、それ以外は手元のディレクトリとして扱う。
// 手元のディレクトリはテストと、ネットワーク越しのファイルシステムをハブにする場合に使う。
type Hub struct {
	Host string
	Root string
}

func ParseHub(spec string) (Hub, error) {
	if spec == "" {
		return Hub{}, fmt.Errorf("hub is not set (use --hub or CCREEL_HUB)")
	}
	// rsync と同じく、最初の / より前に : があれば host:path とみなす
	if i := strings.Index(spec, ":"); i > 0 && !strings.Contains(spec[:i], "/") {
		root := spec[i+1:]
		if root == "" {
			root = "."
		}
		return Hub{Host: spec[:i], Root: root}, nil
	}
	return Hub{Root: spec}, nil
}

func (h Hub) String() string {
	if h.Host == "" {
		return h.Root
	}
	return h.Host + ":" + h.Root
}

// rsyncPath は rsync の引数に渡す形。
func (h Hub) rsyncPath(rel string) string {
	p := strings.TrimRight(h.Root, "/") + "/" + rel
	if h.Host == "" {
		return p
	}
	return h.Host + ":" + p
}

// projects は config dir ごとの projects ディレクトリ（Root からの相対）。
func projectsRel(configName string) string {
	return configName + "/projects/"
}

// sh はハブの側で POSIX sh のスクリプトを走らせる。手元のハブでも ssh の先でも同じスクリプトを使い、
// テストで通したものがそのまま het などで動くようにする。
func (h Hub) sh(script string, stdin io.Reader) ([]byte, error) {
	var cmd *exec.Cmd
	if h.Host == "" {
		cmd = exec.Command("sh", "-c", script)
	} else {
		cmd = exec.Command("ssh", "-o", "ConnectTimeout=5", "-o", "BatchMode=yes", h.Host, script)
	}
	var stderr bytes.Buffer
	cmd.Stdin = stdin
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("%s: %v: %s", h, err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// quote は sh の単一引用符で囲む。
func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func (h Hub) dir(configName string) string {
	return quote(strings.TrimRight(h.Root, "/") + "/" + projectsRel(configName))
}

func (h Hub) mkdir(configName string) error {
	_, err := h.sh("mkdir -p "+h.dir(configName), nil)
	return err
}

// listRecent は、更新から days 日以内のセッションと、そのセッションの付属物のディレクトリを、
// projects からの相対パスで返す。rsync の --files-from にそのまま渡せる。
func (h Hub) listRecent(configName string, days int) ([]string, error) {
	script := fmt.Sprintf(`cd %s 2>/dev/null || exit 0
find . -mindepth 2 -maxdepth 2 -type f -name '*.jsonl' -mtime -%d
echo --
find . -mindepth 2 -maxdepth 2 -type d`, h.dir(configName), days)
	out, err := h.sh(script, nil)
	if err != nil {
		return nil, err
	}
	files, dirs, _ := strings.Cut(string(out), "--\n")
	keep := map[string]bool{}
	var list []string
	for _, f := range strings.Fields(files) {
		f = strings.TrimPrefix(f, "./")
		if isSessionFile(f) {
			list = append(list, f)
			keep[strings.TrimSuffix(f, ".jsonl")] = true
		}
	}
	for _, d := range strings.Fields(dirs) {
		d = strings.TrimPrefix(d, "./")
		if keep[d] {
			list = append(list, d)
		}
	}
	return list, nil
}

// prune は、更新から days 日を過ぎたセッションを付属物ごと消す。付属物は更新日を見ない。
// 動いているセッションの付属物を、それ自体が古いという理由で消さないため。
func (h Hub) prune(configName string, days int) error {
	script := fmt.Sprintf(`cd %s 2>/dev/null || exit 0
find . -mindepth 2 -maxdepth 2 -type f -name '*.jsonl' -mtime +%d | while read -r f; do
  rm -rf "$f" "${f%%.jsonl}"
done
find . -mindepth 1 -maxdepth 1 -type d -empty -exec rmdir {} +`, h.dir(configName), days)
	_, err := h.sh(script, nil)
	return err
}

func rsyncArgs(h Hub) []string {
	args := []string{"-a", "--update"}
	if h.Host != "" {
		args = append(args, "-e", "ssh -o ConnectTimeout=5 -o BatchMode=yes")
	}
	return args
}

func runRsync(args []string, stdin io.Reader) error {
	cmd := exec.Command("rsync", args...)
	var stderr bytes.Buffer
	cmd.Stdin = stdin
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("rsync: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
