// Package finder は、言葉からセッションを探して順位を付ける。人の picker と、Claude が skill から呼ぶ find の
// 両方がここを通る。
//
// 分岐は LLM に判断させず、ここで決め打ちにする。
//
//	parse  言葉を @リポ・#番号・語に分ける
//	collect 全部の config dir から、番号・題・本文のどれかに当たるセッションを rg で集める
//	score  Issue 番号の一致 > 題の一致 > 本文の一致、に同じリポと新しさを足して点を付ける
//	decide 1位が2位より十分に上なら決まり、そうでなければ候補を並べて聞く
package finder

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yktsnet/ccreel/internal/session"
)

// Query は言葉を役割ごとに分けたもの。
type Query struct {
	Terms []string `json:"terms,omitempty"`
	Issue string   `json:"issue,omitempty"`
	Repo  string   `json:"repo,omitempty"`
}

var numberRe = regexp.MustCompile(`^#?([0-9]+)$`)

// ParseQuery は @リポ をリポの指定、#123 か 123 を Issue 番号として読む。番号は main の上で
// 「PR #951」と話した会話にも当たるよう、本文の語としても残す。
func ParseQuery(words []string) Query {
	var q Query
	for _, w := range words {
		for _, f := range strings.Fields(w) {
			switch {
			case strings.HasPrefix(f, "@") && len(f) > 1:
				q.Repo = f[1:]
			case numberRe.MatchString(f):
				n := numberRe.FindStringSubmatch(f)[1]
				q.Issue = n
				q.Terms = append(q.Terms, n)
			default:
				q.Terms = append(q.Terms, f)
			}
		}
	}
	return q
}

type Options struct {
	ConfigDirs  []string // 探す config dir。先頭が既定のもの
	CurrentRepo string   // 呼ばれたリポ。同じリポの会話に点を足す
	Exclude     string   // 外すセッション（いま話しているもの）
	Limit       int
	Now         time.Time
}

// Candidate は順位の付いたセッション1本。Reasons は点の内訳で、人と Claude が当たった理由を読むために出す。
type Candidate struct {
	session.Meta
	ConfigDir string   `json:"config_dir"`
	Score     int      `json:"score"`
	Reasons   []string `json:"reasons"`

	body map[string]int // 言葉ごとの本文の当たり行数
}

type Result struct {
	Query      Query       `json:"query"`
	Decided    bool        `json:"decided"`
	Candidates []Candidate `json:"candidates"`
}

const (
	issueScore    = 100
	titleScore    = 30
	bodyCap       = 20
	allTermsScore = 15
	sameRepoScore = 10
	recencyMax    = 5
	recencyDays   = 30
	// 決まりとみなすのは、1位が2位の1.5倍以上で、かつ20点以上離れているとき
	decideRatio = 1.5
	decideGap   = 20
	// 中身を読む候補の数。番号はツールの出力の数字にも当たり、候補が数百本になることがある。
	// rg の当たりだけで仮の順位を付け、上位だけを読む
	scanMax = 40
)

// bodyScore は当たり行数の対数で点を付ける。数が多いほど上がるが、1本が飛び抜けすぎないようにする
func bodyScore(n int) int {
	return min(bodyCap, int(math.Round(5*math.Log2(1+float64(n)))))
}

var ErrEmpty = errors.New("nothing to search for: give words, #issue or @repo")

// Find は言葉に当たるセッションを探し、点の高い順に返す。
func Find(q Query, o Options) (Result, error) {
	res := Result{Query: q}
	if len(q.Terms) == 0 && q.Issue == "" && q.Repo == "" {
		return res, ErrEmpty
	}
	if o.Now.IsZero() {
		o.Now = time.Now()
	}
	var all []Candidate
	for _, dir := range o.ConfigDirs {
		cs, err := collect(q, dir)
		if err != nil {
			return res, err
		}
		all = append(all, cs...)
	}
	var kept []Candidate
	for _, c := range all {
		if c.ID == o.Exclude || (q.Repo != "" && c.Repo != q.Repo) {
			continue
		}
		if score(&c, q, o) {
			kept = append(kept, c)
		}
	}
	sort.SliceStable(kept, func(i, j int) bool {
		if kept[i].Score != kept[j].Score {
			return kept[i].Score > kept[j].Score
		}
		return kept[i].Updated.After(kept[j].Updated)
	})
	res.Decided = decided(kept)
	if o.Limit > 0 && len(kept) > o.Limit {
		kept = kept[:o.Limit]
	}
	res.Candidates = kept
	return res, nil
}

func decided(cs []Candidate) bool {
	switch len(cs) {
	case 0:
		return false
	case 1:
		return true
	}
	a, b := float64(cs[0].Score), float64(cs[1].Score)
	return a >= b*decideRatio && a-b >= decideGap
}

// collect は1つの config dir から、番号・言葉のどれかに当たるセッションを集め、言葉ごとの本文の当たり数を添える。
// @リポ だけの指定では本文を探さず、全部のセッションを候補にする。
func collect(q Query, configDir string) ([]Candidate, error) {
	root := filepath.Join(configDir, "projects")
	if _, err := os.Stat(root); err != nil {
		return nil, nil
	}
	found := map[string]map[string]int{}
	prelim := map[string]int{}
	get := func(p string) map[string]int {
		if found[p] == nil {
			found[p] = map[string]int{}
		}
		return found[p]
	}
	if len(q.Terms) == 0 && q.Issue == "" {
		paths, err := session.Discover(configDir)
		if err != nil {
			return nil, err
		}
		for _, p := range paths {
			get(p)
		}
	}
	if q.Issue != "" {
		// 番号はブランチ名か worktree の名前に入る。Meta.Issue で確かめるので、ここでは候補を広めに拾う
		pat := fmt.Sprintf(`/%s-[A-Za-z]|"gitBranch":"%s-[A-Za-z]`, q.Issue, q.Issue)
		counts, err := count(root, pat, false)
		if err != nil {
			return nil, err
		}
		for p := range counts {
			get(p)
			prelim[p] += issueScore
		}
	}
	for _, t := range q.Terms {
		counts, err := count(root, t, true)
		if err != nil {
			return nil, err
		}
		for p, n := range counts {
			get(p)[t] = n
			prelim[p] += bodyScore(n)
		}
		// 題は jsonl の ai-title の行にある。付け直された古い題にも当たるが、仮の順位には足りる
		titles, err := count(root, `"aiTitle":"[^"]*`+regexp.QuoteMeta(t), false)
		if err != nil {
			return nil, err
		}
		for p := range titles {
			prelim[p] += titleScore
		}
	}

	paths := make([]string, 0, len(found))
	for p := range found {
		paths = append(paths, p)
	}
	if len(paths) > scanMax {
		sort.Slice(paths, func(i, j int) bool { return prelim[paths[i]] > prelim[paths[j]] })
		paths = paths[:scanMax]
	}
	var out []Candidate
	for _, m := range session.ScanAll(paths) {
		out = append(out, Candidate{Meta: m, ConfigDir: configDir, body: found[m.Path]})
	}
	return out, nil
}

// score は点を付け、残すべき候補なら true を返す。
func score(c *Candidate, q Query, o Options) bool {
	matchedAny := false
	if q.Issue != "" && c.Issue == q.Issue {
		c.Score += issueScore
		c.Reasons = append(c.Reasons, "issue #"+q.Issue)
		matchedAny = true
	}
	title := strings.ToLower(c.Title)
	all := len(q.Terms) > 0
	for _, t := range q.Terms {
		inTitle := title != "" && strings.Contains(title, strings.ToLower(t))
		n := c.body[t]
		if inTitle {
			c.Score += titleScore
			c.Reasons = append(c.Reasons, "title:"+t)
		}
		if n > 0 {
			c.Score += bodyScore(n)
			c.Reasons = append(c.Reasons, fmt.Sprintf("body:%s×%d", t, n))
		}
		if !inTitle && n == 0 {
			all = false
		} else {
			matchedAny = true
		}
	}
	if all && len(q.Terms) > 1 {
		c.Score += allTermsScore
		c.Reasons = append(c.Reasons, "all terms")
	}
	if !matchedAny && (len(q.Terms) > 0 || q.Issue != "") {
		return false
	}
	if q.Repo != "" {
		c.Reasons = append(c.Reasons, "repo @"+q.Repo)
	}
	if o.CurrentRepo != "" && c.Repo == o.CurrentRepo {
		c.Score += sameRepoScore
		c.Reasons = append(c.Reasons, "same repo")
	}
	if age := o.Now.Sub(c.Updated).Hours() / 24; age < recencyDays {
		c.Score += int(math.Round(recencyMax * (recencyDays - age) / recencyDays))
	}
	return true
}

// count は projects 直下のセッションごとに、pattern に当たった行の数を返す。大文字小文字は区別しない。
// rg が無ければ同じことを Go で行う。
func count(root, pattern string, fixed bool) (map[string]int, error) {
	out := map[string]int{}
	if _, err := exec.LookPath("rg"); err == nil {
		args := []string{"-c", "-i", "--max-depth", "2", "--glob", "*.jsonl"}
		if fixed {
			args = append(args, "-F")
		}
		args = append(args, "-e", pattern, root)
		b, err := exec.Command("rg", args...).Output()
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 1 {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			i := strings.LastIndex(line, ":")
			if i < 0 {
				continue
			}
			if n, err := strconv.Atoi(line[i+1:]); err == nil {
				out[line[:i]] = n
			}
		}
		return out, nil
	}
	re, err := regexp.Compile("(?i)" + map[bool]string{true: regexp.QuoteMeta(pattern), false: pattern}[fixed])
	if err != nil {
		return nil, err
	}
	paths, err := filepath.Glob(filepath.Join(root, "*", "*.jsonl"))
	if err != nil {
		return nil, err
	}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		n := 0
		for _, line := range bytes.Split(b, []byte("\n")) {
			if re.Match(line) {
				n++
			}
		}
		if n > 0 {
			out[p] = n
		}
	}
	return out, nil
}
