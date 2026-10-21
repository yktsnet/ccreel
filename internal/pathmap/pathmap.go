// Package pathmap は、端末ごとに違うホームのパスと、ハブに置く端末に依らない形とを相互に変換する。
//
// ハブでは、プロジェクト名の先頭のホーム（-home-alice・-Users-alice）を "~" に、
// jsonl の "cwd" の先頭のホーム（/home/alice・/Users/alice）を "~" に置き換えて持つ。
// どの端末から push しても同じセッションは同じバイト列になるので、rsync が差分なしと判定できる。
package pathmap

import (
	"bytes"
	"strings"
)

const token = "~"

// EncodeDir は Claude Code がプロジェクト名を作るときと同じく、英数字以外を "-" に置き換える。
func EncodeDir(path string) string {
	b := []byte(path)
	for i, c := range b {
		if !('a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9') {
			b[i] = '-'
		}
	}
	return string(b)
}

// Mapper は1台の端末のホームを基準にした変換。
type Mapper struct {
	home    string // 例: /home/alice
	homeDir string // 例: -home-alice
}

func New(home string) Mapper {
	home = strings.TrimRight(home, "/")
	return Mapper{home: home, homeDir: EncodeDir(home)}
}

// ProjectToHub はホームの下のプロジェクト名を "~" で始まる形にする。ホームの外のものはそのまま返す。
// -home-alice2-x のように、ホームの名前で始まるだけの別のディレクトリは置き換えない。
func (m Mapper) ProjectToHub(name string) string {
	if name == m.homeDir {
		return token
	}
	if strings.HasPrefix(name, m.homeDir+"-") {
		return token + name[len(m.homeDir):]
	}
	return name
}

// ProjectFromHub は ProjectToHub の逆。
func (m Mapper) ProjectFromHub(name string) string {
	if name == token {
		return m.homeDir
	}
	if strings.HasPrefix(name, token+"-") {
		return m.homeDir + name[len(token):]
	}
	return name
}

var cwdKey = []byte(`"cwd":"`)

// ContentToHub は jsonl の中の "cwd" のうち、ホームで始まるものを "~" で始まる形にする。
// cwd 以外のパス（ツールの入出力など）は書き換えない。resume に要るのは cwd だけである。
func (m Mapper) ContentToHub(b []byte) []byte {
	return replaceCWDPrefix(b, m.home, token)
}

// ContentFromHub は ContentToHub の逆。
func (m Mapper) ContentFromHub(b []byte) []byte {
	return replaceCWDPrefix(b, token, m.home)
}

// replaceCWDPrefix は "cwd":"<from>" と "cwd":"<from>/..." の <from> を to に置き換える。
// <from> の直後が " か / のときだけ置き換え、/home/alice2 のような別のパスには触れない。
func replaceCWDPrefix(b []byte, from, to string) []byte {
	if !bytes.Contains(b, cwdKey) {
		return b
	}
	pat := append(append([]byte{}, cwdKey...), from...)
	var out bytes.Buffer
	out.Grow(len(b))
	for {
		i := bytes.Index(b, pat)
		if i < 0 {
			out.Write(b)
			return out.Bytes()
		}
		end := i + len(pat)
		if end < len(b) && (b[end] == '"' || b[end] == '/') {
			out.Write(b[:i])
			out.Write(cwdKey)
			out.WriteString(to)
		} else {
			out.Write(b[:end])
		}
		b = b[end:]
	}
}
