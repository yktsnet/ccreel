package hubsync

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	idA   = "aaaaaaaa-0000-4000-8000-000000000001"
	idOld = "aaaaaaaa-0000-4000-8000-000000000002"
)

type machine struct {
	opt Options
}

func newMachine(t *testing.T, base, home string, hub Hub) machine {
	t.Helper()
	return machine{Options{
		ConfigDir: filepath.Join(base, "cfg", ".claude"),
		Home:      home,
		Mirror:    filepath.Join(base, "mirror"),
		Hub:       hub,
		PullDays:  30,
		KeepDays:  40,
	}}
}

func (m machine) write(t *testing.T, rel, body string, mtime time.Time) string {
	t.Helper()
	p := filepath.Join(m.opt.ConfigDir, "projects", rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, mtime, mtime); err != nil {
		t.Fatal(err)
	}
	return p
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func setup(t *testing.T) (linux, mac machine, hubRoot string) {
	t.Helper()
	if _, err := exec.LookPath("rsync"); err != nil {
		t.Skip("rsync not found")
	}
	tmp := t.TempDir()
	hubRoot = filepath.Join(tmp, "hub")
	hub := Hub{Root: hubRoot}
	linux = newMachine(t, filepath.Join(tmp, "linux"), "/home/alice", hub)
	mac = newMachine(t, filepath.Join(tmp, "mac"), "/Users/alice", hub)
	return
}

func TestPushThenPullAcrossMachines(t *testing.T) {
	linux, mac, hubRoot := setup(t)
	now := time.Now().Add(-time.Hour).Truncate(time.Second)

	mac.write(t, "-Users-alice-src-demo/"+idA+".jsonl", `{"cwd":"/Users/alice/src/demo"}`+"\n", now)
	mac.write(t, "-Users-alice-src-demo/"+idA+"/subagents/agent-1.jsonl", `{"cwd":"/Users/alice/src/demo"}`+"\n", now)
	// セッションの本体以外（付属物・memory/）は運ばない
	mac.write(t, "-Users-alice-src-demo/memory/note.md", "x", now)

	if _, err := Push(mac.opt); err != nil {
		t.Fatal(err)
	}
	onHub := filepath.Join(hubRoot, ".claude", "projects", "~-src-demo", idA+".jsonl")
	if got := read(t, onHub); got != `{"cwd":"~/src/demo"}`+"\n" {
		t.Errorf("hub content = %q", got)
	}
	for _, name := range []string{"memory", idA} {
		if exists(filepath.Join(hubRoot, ".claude", "projects", "~-src-demo", name)) {
			t.Errorf("%s/ was pushed", name)
		}
	}

	st, err := Pull(linux.opt)
	if err != nil {
		t.Fatal(err)
	}
	if st.Materialize != 1 {
		t.Errorf("materialized %d files, want 1", st.Materialize)
	}
	local := filepath.Join(linux.opt.ConfigDir, "projects", "-home-alice-src-demo", idA+".jsonl")
	if got := read(t, local); got != `{"cwd":"/home/alice/src/demo"}`+"\n" {
		t.Errorf("local content = %q", got)
	}
	if fi, _ := os.Stat(local); !fi.ModTime().Equal(now) {
		t.Errorf("mtime = %v, want %v (kept from the source)", fi.ModTime(), now)
	}

	// pull したものを push し直しても、ハブの中身も日時も変わらない
	before, _ := os.Stat(onHub)
	if _, err := Push(linux.opt); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(onHub)
	if got := read(t, onHub); got != `{"cwd":"~/src/demo"}`+"\n" || !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("re-push changed the hub: %q %v → %v", got, before.ModTime(), after.ModTime())
	}
}

func TestNewerSideWins(t *testing.T) {
	linux, mac, _ := setup(t)
	t0 := time.Now().Add(-2 * time.Hour).Truncate(time.Second)

	mac.write(t, "-Users-alice-src-demo/"+idA+".jsonl", `{"cwd":"/Users/alice/src/demo","n":1}`+"\n", t0)
	if _, err := Push(mac.opt); err != nil {
		t.Fatal(err)
	}
	if _, err := Pull(linux.opt); err != nil {
		t.Fatal(err)
	}
	// linux で resume して追記した
	local := linux.write(t, "-home-alice-src-demo/"+idA+".jsonl",
		`{"cwd":"/home/alice/src/demo","n":1}`+"\n"+`{"cwd":"/home/alice/src/demo","n":2}`+"\n", t0.Add(time.Hour))
	// 古い側の pull は手元の新しい方を上書きしない
	if _, err := Pull(linux.opt); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read(t, local), `"n":2`) {
		t.Fatal("pull overwrote a newer local session")
	}
	if _, err := Push(linux.opt); err != nil {
		t.Fatal(err)
	}
	if _, err := Pull(mac.opt); err != nil {
		t.Fatal(err)
	}
	got := read(t, filepath.Join(mac.opt.ConfigDir, "projects", "-Users-alice-src-demo", idA+".jsonl"))
	if got != `{"cwd":"/Users/alice/src/demo","n":1}`+"\n"+`{"cwd":"/Users/alice/src/demo","n":2}`+"\n" {
		t.Errorf("mac did not get the newer session: %q", got)
	}
}

func TestPullWindowAndPrune(t *testing.T) {
	linux, mac, hubRoot := setup(t)
	now := time.Now().Truncate(time.Second)
	hubDir := filepath.Join(hubRoot, ".claude", "projects", "~-src-demo")

	mac.write(t, "-Users-alice-src-demo/"+idA+".jsonl", `{}`+"\n", now.Add(-35*24*time.Hour))
	// 40日を過ぎた手元のものは送らない（送ってもハブで消え、毎日送り直すことになる）
	mac.write(t, "-Users-alice-src-demo/"+idOld+".jsonl", `{}`+"\n", now.Add(-45*24*time.Hour))
	// ハブにある40日を過ぎたものは消える
	stale := filepath.Join(hubRoot, ".claude", "projects", "~-src-other", idOld+".jsonl")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := now.Add(-45 * 24 * time.Hour)
	os.Chtimes(stale, old, old)

	st, err := Push(mac.opt)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Pruned {
		t.Error("first push did not prune")
	}
	if st.Staged != 1 {
		t.Errorf("staged %d files, want 1 (the 45-day-old one is skipped)", st.Staged)
	}
	if !exists(filepath.Join(hubDir, idA+".jsonl")) {
		t.Error("35-day-old session is missing from the hub")
	}
	if exists(filepath.Join(hubDir, idOld+".jsonl")) {
		t.Error("45-day-old local session was pushed")
	}
	if exists(stale) || exists(filepath.Dir(stale)) {
		t.Error("45-day-old session on the hub was not pruned")
	}

	// 30日より古いものは pull しない
	if _, err := Pull(linux.opt); err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(linux.opt.ConfigDir, "projects", "-home-alice-src-demo", idA+".jsonl")) {
		t.Error("35-day-old session was pulled")
	}
}

func TestPushOnlyWhenChanged(t *testing.T) {
	_, mac, _ := setup(t)
	now := time.Now().Add(-time.Hour).Truncate(time.Second)
	p := mac.write(t, "-Users-alice-src-demo/"+idA+".jsonl", `{"n":1}`+"\n", now)

	st, err := Push(mac.opt)
	if err != nil || !st.Sent || !st.Pruned {
		t.Fatalf("first push: %+v, %v", st, err)
	}

	// 変わったものが無ければハブにつながない。つなげば届かない宛先で失敗する
	good := mac.opt.Hub
	mac.opt.Hub = Hub{Host: "ccreel-test.invalid", Root: "hub"}
	if st, err := Push(mac.opt); err != nil || st.Sent {
		t.Fatalf("unchanged push: %+v, %v", st, err)
	}

	// 送れなかった変更は、次の回に変わったものが無くても送り直す
	os.WriteFile(p, []byte(`{"n":1}`+"\n"+`{"n":2}`+"\n"), 0o644)
	os.Chtimes(p, now.Add(time.Minute), now.Add(time.Minute))
	if _, err := Push(mac.opt); err == nil {
		t.Fatal("push to an unreachable hub succeeded")
	}
	mac.opt.Hub = good
	st, err = Push(mac.opt)
	if err != nil || st.Staged != 0 || !st.Sent {
		t.Fatalf("retry push: %+v, %v", st, err)
	}
	// 古いものを消すのは1日に1回
	if st.Pruned {
		t.Error("pruned twice within a day")
	}

	mac.opt.Force = true
	if st, err := Push(mac.opt); err != nil || !st.Sent {
		t.Errorf("forced push: %+v, %v", st, err)
	}
}

func TestLockedMirror(t *testing.T) {
	linux, _, _ := setup(t)
	unlock, err := lock(linux.opt.Mirror)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if _, err := Push(linux.opt); err != ErrBusy {
		t.Errorf("Push = %v, want ErrBusy", err)
	}
	if _, err := Pull(linux.opt); err != ErrBusy {
		t.Errorf("Pull = %v, want ErrBusy", err)
	}
}

func TestParseHub(t *testing.T) {
	cases := []struct{ spec, host, root string }{
		{"vps:ccreel", "vps", "ccreel"},
		{"vps:", "vps", "."},
		{"/srv/hub", "", "/srv/hub"},
		{"./a:b", "", "./a:b"},
	}
	for _, c := range cases {
		h, err := ParseHub(c.spec)
		if err != nil || h.Host != c.host || h.Root != c.root {
			t.Errorf("ParseHub(%q) = %+v, %v", c.spec, h, err)
		}
	}
	if _, err := ParseHub(""); err == nil {
		t.Error("empty hub accepted")
	}
}

func TestIsSessionFile(t *testing.T) {
	for rel, want := range map[string]bool{
		"~-src/" + idA + ".jsonl":             true,
		"~-src/memory.jsonl":                  false,
		idA + ".jsonl":                        false,
		"~-src/" + idA + "/subagents/x.jsonl": false,
	} {
		if got := isSessionFile(rel); got != want {
			t.Errorf("isSessionFile(%q) = %v", rel, got)
		}
	}
}
