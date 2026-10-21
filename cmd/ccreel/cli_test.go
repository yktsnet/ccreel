package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/yktsnet/ccreel/internal/finder"
)

const otherID = "bbbbbbbb-0000-4000-8000-000000000001"

// secondConfigDir は、fixture とは別の config dir に1本だけセッションを置く。cwd は実在するディレクトリにする。
func secondConfigDir(t *testing.T) (dir, cwd string) {
	t.Helper()
	tmp := t.TempDir()
	dir = filepath.Join(tmp, "work")
	cwd = filepath.Join(tmp, "src", "work-app")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "projects", "-src-work-app", otherID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	line := `{"type":"user","message":{"role":"user","content":"tune the parser for the work app"},"timestamp":"2026-03-04T09:00:00Z","cwd":` +
		jsonString(cwd) + `}` + "\n"
	if err := os.WriteFile(p, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, cwd
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// resume は claude にプロセスを置き換えるので、このテストのバイナリを子プロセスとして main から走らせる
func TestMain(m *testing.M) {
	if os.Getenv("CCREEL_TEST_MAIN") == "1" {
		os.Args = append([]string{"ccreel"}, strings.Fields(os.Getenv("CCREEL_TEST_ARGS"))...)
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestResumeUsesTheSessionsConfigDir(t *testing.T) {
	work, cwd := secondConfigDir(t)
	out := filepath.Join(t.TempDir(), "claude.out")
	fake := filepath.Join(t.TempDir(), "fake-claude")
	script := "#!/bin/sh\n{ pwd; echo \"$CLAUDE_CONFIG_DIR\"; echo \"$*\"; } > " + out + "\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	fixture, _ := filepath.Abs("../../testdata")

	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(),
		"CCREEL_TEST_MAIN=1",
		"CCREEL_TEST_ARGS=resume bbbbbbbb",
		"CCREEL_CLAUDE="+fake,
		"CLAUDE_CONFIG_DIR="+fixture,
		"CCREEL_CONFIG_DIRS="+fixture+":"+work,
	)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("resume: %v\n%s", err, b)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(strings.TrimSpace(string(b)), "\n")
	want := []string{cwd, work, "--resume " + otherID}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("claude saw %q, want %q", got, want)
	}
}

func TestShowFindsSessionInOtherConfigDir(t *testing.T) {
	work, _ := secondConfigDir(t)
	t.Setenv("CLAUDE_CONFIG_DIR", "../../testdata")
	t.Setenv("CCREEL_CONFIG_DIRS", "../../testdata:"+work)
	var out, errw bytes.Buffer
	if err := runShow([]string{"bbbbbbbb", "--json"}, &out, &errw); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "tune the parser for the work app") {
		t.Errorf("show = %q", out.String())
	}
	out.Reset()
	if err := runOutline([]string{"bbbbbbbb", "--json"}, &out, &errw); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "tune the parser") {
		t.Errorf("outline = %q", out.String())
	}
}

func TestFindRanksAcrossConfigDirs(t *testing.T) {
	work, _ := secondConfigDir(t)
	res, err := finder.Find(finder.ParseQuery([]string{"parser"}), finder.Options{
		ConfigDirs: configDirsFor(t, "../../testdata", work),
	})
	if err != nil {
		t.Fatal(err)
	}
	dirs := map[string]bool{}
	for _, c := range res.Candidates {
		dirs[filepath.Base(c.ConfigDir)] = true
	}
	if !dirs["testdata"] || !dirs["work"] {
		t.Errorf("candidates came from %v, want both config dirs", dirs)
	}
}

func configDirsFor(t *testing.T, primary, extra string) []string {
	t.Helper()
	t.Setenv("CCREEL_CONFIG_DIRS", extra)
	return configDirs(primary)
}

// 偽の fzf が受け取った行の順が、find の順位と同じかを見る。偽の fzf は何も選ばずに閉じる
func TestPickWithWordsUsesFindOrder(t *testing.T) {
	bin := t.TempDir()
	got := filepath.Join(bin, "lines")
	script := "#!/bin/sh\ncat > " + got + "\nexit 130\n"
	if err := os.WriteFile(filepath.Join(bin, "fzf"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	t.Setenv("CLAUDE_CONFIG_DIR", "../../testdata")
	t.Setenv("CCREEL_HUB", "")

	var errw bytes.Buffer
	if err := runPick([]string{"logs", "parser", "--no-pull"}, &errw, &errw); err != nil && !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("pick: %v %s", err, errw.String())
	}
	b, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	var picked []string
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		id, _, _ := strings.Cut(line, "\t")
		picked = append(picked, id)
	}
	res, err := finder.Find(finder.ParseQuery([]string{"logs", "parser"}), finder.Options{ConfigDirs: []string{"../../testdata"}})
	if err != nil {
		t.Fatal(err)
	}
	var ranked []string
	for _, c := range res.Candidates {
		ranked = append(ranked, c.ID)
	}
	if strings.Join(picked, ",") != strings.Join(ranked, ",") || len(ranked) < 2 {
		t.Errorf("pick order %v, find order %v", picked, ranked)
	}
}

func jsonKeys(t *testing.T, line string) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		t.Fatalf("not JSON: %q", line)
	}
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}

func TestOutlineAndSearchJSONFields(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "../../testdata")
	var out, errw bytes.Buffer
	if err := runOutline([]string{"1111", "--json"}, &out, &errw); err != nil {
		t.Fatal(err)
	}
	first, _, _ := strings.Cut(out.String(), "\n")
	if got := jsonKeys(t, first); got != "prompt,reply,time,tools,turn" {
		t.Errorf("outline fields = %s", got)
	}
	out.Reset()
	if err := runSearch([]string{"parser", "--json", "--limit", "1"}, &out, &errw); err != nil {
		t.Fatal(err)
	}
	first, _, _ = strings.Cut(out.String(), "\n")
	if got := jsonKeys(t, first); got != "id,kind,repo,snippet,title,turn,updated" {
		t.Errorf("search fields = %s", got)
	}
}

func TestPushIsQuietWhenNothingIsSent(t *testing.T) {
	if _, err := exec.LookPath("rsync"); err != nil {
		t.Skip("rsync not found")
	}
	tmp := t.TempDir()
	args := []string{"--config-dir", "../../testdata", "--hub", filepath.Join(tmp, "hub"), "--mirror", filepath.Join(tmp, "mirror")}
	var out, errw bytes.Buffer
	if err := runPush(args, &out, &errw); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), "pushed to ") {
		t.Fatalf("first push printed %q", out.String())
	}
	out.Reset()
	if err := runPush(args, &out, &errw); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 || errw.Len() != 0 {
		t.Errorf("push with nothing to send printed %q / %q", out.String(), errw.String())
	}
}

func TestPreviewIsNewestFirstMarkdown(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "../../testdata")
	var out, errw bytes.Buffer
	if err := runPreview([]string{"1111"}, &out, &errw); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"# Fix parser trailing token bug", "`@demo` · `#12` · `task/12-fix-parser`", "> Fix it and add a test"} {
		if !strings.Contains(got, want) {
			t.Errorf("preview lacks %q:\n%s", want, got)
		}
	}
	// 新しいターンを先に出す
	if strings.Index(got, "**turn 2**") > strings.Index(got, "**turn 1**") {
		t.Errorf("turns are not newest first:\n%s", got)
	}
}
