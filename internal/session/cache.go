package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// 一覧の組み立てを変えたら上げる。古い版が書いた行を、組み立て直さずに出さないため
const cacheVersion = 1

// cached は jsonl 1本ぶんの Scan の結果。Headless は Meta の JSON に出さないので別に持つ
type cached struct {
	Size     int64 `json:"size"`
	ModNano  int64 `json:"mod"`
	Meta     Meta  `json:"meta"`
	Headless bool  `json:"headless"`
}

type cacheFile struct {
	Version int               `json:"version"`
	Exe     string            `json:"exe"`
	Entries map[string]cached `json:"entries"`
}

// exeKey は実行中のバイナリを識別する。nix run で新しい版を引くとストアのパスが変わり、書き直さずに古い行を捨てられる
func exeKey() string {
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	fi, err := os.Stat(p)
	if err != nil {
		return p
	}
	return fmt.Sprintf("%s|%d|%d", p, fi.Size(), fi.ModTime().UnixNano())
}

func loadCache(path, exe string) map[string]cached {
	b, err := os.ReadFile(path)
	if err != nil {
		return map[string]cached{}
	}
	var c cacheFile
	if json.Unmarshal(b, &c) != nil || c.Version != cacheVersion || c.Exe != exe || c.Entries == nil {
		return map[string]cached{}
	}
	return c.Entries
}

// saveCache は別の名前に書いてから置き換える。並んで走った pick と list が書きかけを読まないため
func saveCache(path, exe string, entries map[string]cached) {
	b, err := json.Marshal(cacheFile{Version: cacheVersion, Exe: exe, Entries: entries})
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".meta-*.json")
	if err != nil {
		return
	}
	_, werr := tmp.Write(b)
	cerr := tmp.Close()
	if werr != nil || cerr != nil || os.Rename(tmp.Name(), path) != nil {
		os.Remove(tmp.Name())
	}
}

// ScanAllCached は ScanAll と同じ一覧を返す。大きさと更新日時が前回と同じ jsonl は読み直さず、cachePath に残した結果を使う。
// 履歴は数 GB になり、全部を読むと pick の一覧が出るまで数秒かかる。cachePath が空なら残さない。
func ScanAllCached(paths []string, cachePath string) []Meta {
	if cachePath == "" {
		return ScanAll(paths)
	}
	exe := exeKey()
	old := loadCache(cachePath, exe)
	next := make(map[string]cached, len(paths))
	var stale []string
	stats := map[string]os.FileInfo{}
	for _, p := range paths {
		fi, err := os.Stat(p)
		if err != nil {
			continue
		}
		stats[p] = fi
		if c, ok := old[p]; ok && c.Size == fi.Size() && c.ModNano == fi.ModTime().UnixNano() {
			next[p] = c
			continue
		}
		stale = append(stale, p)
	}
	for i, m := range scanEach(stale, nil) {
		fi := stats[stale[i]]
		if m == nil {
			continue
		}
		next[stale[i]] = cached{Size: fi.Size(), ModNano: fi.ModTime().UnixNano(), Meta: *m, Headless: m.Headless}
	}
	// 今回の paths に無くても、別の呼び方（ハブも読む list など）の行は、ファイルが残っていれば捨てない
	for p, c := range old {
		if _, ok := next[p]; ok {
			continue
		}
		if _, seen := stats[p]; seen {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			next[p] = c
		}
	}
	if len(stale) > 0 || len(next) != len(old) {
		saveCache(cachePath, exe, next)
	}

	metas := make([]Meta, 0, len(paths))
	for _, p := range paths {
		c, ok := next[p]
		if !ok {
			continue
		}
		m := c.Meta
		m.Headless = c.Headless
		metas = append(metas, m)
	}
	return finish(metas)
}
