// Package hubsync は、手元の config dir とハブとのあいだでセッションを運ぶ。
//
// 手元とハブのあいだに、ハブと同じ形（pathmap で ~ に置き換えた形）の写しを1つ持つ（mirror）。
//
//	push: config dir → mirror（手元の方が新しいものだけ変換して書く）→ rsync → ハブ
//	pull: ハブ → rsync（直近 PullDays 日の分だけ）→ mirror → config dir（mirror の方が新しいものだけ戻して書く）
//
// 書いたファイルには元の更新日時を付け直す。pull で運んだものは push のときに mirror と同じ日時・同じ中身になり、
// rsync が差分なしと判定するので、送り返しが起きない。
package hubsync

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/yktsnet/ccreel/internal/pathmap"
)

type Options struct {
	ConfigDir string // 手元の config dir（~/.claude など）
	Home      string // 手元のホーム。ハブの形との変換に使う
	Mirror    string // ハブと同じ形の写しの置き場。config dir ごとに分ける
	Hub       Hub
	PullDays  int // pull で運ぶ範囲。Claude Code の保持期間に合わせる
	KeepDays  int // ハブと mirror に残す期間。PullDays より長くする
}

// Stats は運んだ件数。表示に使う。
type Stats struct {
	Staged      int // push で mirror に書いた数
	Materialize int // pull で config dir に書いた数
}

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// isSessionFile は "<project>/<uuid>.jsonl" の形かを見る。memory/ などセッション以外のものは運ばない。
func isSessionFile(rel string) bool {
	dir, file := filepath.Split(rel)
	return dir != "" && !strings.Contains(strings.TrimSuffix(dir, "/"), "/") &&
		strings.HasSuffix(file, ".jsonl") && uuidRe.MatchString(strings.TrimSuffix(file, ".jsonl"))
}

func (o Options) configName() string { return filepath.Base(filepath.Clean(o.ConfigDir)) }

func (o Options) mirrorProjects() string { return filepath.Join(o.Mirror, "projects") }

func (o Options) localProjects() string { return filepath.Join(o.ConfigDir, "projects") }

// Push は手元のセッションを mirror へ写し、ハブへ送り、ハブと mirror の古いものを消す。
// 初回は手元のセッションが全部送られる（バックフィル）。
func Push(o Options) (Stats, error) {
	var st Stats
	m := pathmap.New(o.Home)
	n, err := stage(o, m)
	st.Staged = n
	if err != nil {
		return st, err
	}
	if err := o.Hub.mkdir(o.configName()); err != nil {
		return st, err
	}
	args := append(rsyncArgs(o.Hub), o.mirrorProjects()+"/", o.Hub.rsyncPath(projectsRel(o.configName())))
	if err := runRsync(args, nil); err != nil {
		return st, err
	}
	if err := o.Hub.prune(o.configName(), o.KeepDays); err != nil {
		return st, err
	}
	return st, pruneLocal(o.mirrorProjects(), o.KeepDays)
}

// Pull はハブから直近 PullDays 日の分を mirror へ取り、手元の config dir へ戻す。
func Pull(o Options) (Stats, error) {
	var st Stats
	list, err := o.Hub.listRecent(o.configName(), o.PullDays)
	if err != nil || len(list) == 0 {
		return st, err
	}
	if err := os.MkdirAll(o.mirrorProjects(), 0o755); err != nil {
		return st, err
	}
	// --files-from は -a の再帰を外すので、付属物のディレクトリを辿らせるために -r を足す
	args := append(rsyncArgs(o.Hub), "-r", "--files-from=-", o.Hub.rsyncPath(projectsRel(o.configName())), o.mirrorProjects()+"/")
	if err := runRsync(args, strings.NewReader(strings.Join(list, "\n")+"\n")); err != nil {
		return st, err
	}
	m := pathmap.New(o.Home)
	for _, rel := range list {
		n, err := materialize(o, m, rel)
		st.Materialize += n
		if err != nil {
			return st, err
		}
	}
	return st, nil
}

// stage は手元の方が新しいセッションと付属物を、ハブの形に変えて mirror へ書く。
func stage(o Options, m pathmap.Mapper) (int, error) {
	root := o.localProjects()
	n := 0
	projects, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	for _, p := range projects {
		if !p.IsDir() {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(root, p.Name()))
		if err != nil {
			return n, err
		}
		hubProject := m.ProjectToHub(p.Name())
		for _, e := range entries {
			name := strings.TrimSuffix(e.Name(), ".jsonl")
			if !uuidRe.MatchString(name) {
				continue
			}
			src := filepath.Join(root, p.Name(), e.Name())
			dst := filepath.Join(o.mirrorProjects(), hubProject, e.Name())
			c, err := copyTree(src, dst, m.ContentToHub)
			n += c
			if err != nil {
				return n, err
			}
		}
	}
	return n, nil
}

// materialize は mirror の1件（セッションか付属物のディレクトリ）を、手元の形に戻して config dir へ書く。
func materialize(o Options, m pathmap.Mapper, rel string) (int, error) {
	hubProject, rest, _ := strings.Cut(rel, "/")
	src := filepath.Join(o.mirrorProjects(), hubProject, rest)
	dst := filepath.Join(o.localProjects(), m.ProjectFromHub(hubProject), rest)
	return copyTree(src, dst, m.ContentFromHub)
}

// copyTree は src（ファイルかディレクトリ）を dst へ写す。dst の方が新しいか同じ日時のファイルは飛ばす。
// .jsonl には conv を通し、それ以外はそのまま写す。書いたファイルには src の更新日時を付ける。
func copyTree(src, dst string, conv func([]byte) []byte) (int, error) {
	n := 0
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		si, err := d.Info()
		if err != nil {
			return err
		}
		if ti, err := os.Stat(target); err == nil && !si.ModTime().After(ti.ModTime()) {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.HasSuffix(path, ".jsonl") {
			b = conv(b)
		}
		if err := writeAtomic(target, b, si.ModTime()); err != nil {
			return err
		}
		n++
		return nil
	})
	return n, err
}

// Claude Code が開いているセッションへ途中まで書いた状態を見せないよう、一時ファイルから rename する。
func writeAtomic(path string, b []byte, mtime time.Time) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".ccreel-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chtimes(tmp.Name(), mtime, mtime); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// pruneLocal は mirror に対して Hub.prune と同じ規則で消す。
func pruneLocal(projects string, days int) error {
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	dirs, err := os.ReadDir(projects)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, p := range dirs {
		if !p.IsDir() {
			continue
		}
		dir := filepath.Join(projects, p.Name())
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if !isSessionFile(p.Name() + "/" + e.Name()) {
				continue
			}
			fi, err := e.Info()
			if err != nil || fi.ModTime().After(cutoff) {
				continue
			}
			path := filepath.Join(dir, e.Name())
			if err := os.RemoveAll(path); err != nil {
				return err
			}
			if err := os.RemoveAll(strings.TrimSuffix(path, ".jsonl")); err != nil {
				return err
			}
		}
		// 空になったときだけ消える
		os.Remove(dir)
	}
	return nil
}
