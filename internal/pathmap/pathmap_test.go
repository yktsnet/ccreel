package pathmap

import "testing"

func TestEncodeDir(t *testing.T) {
	if got := EncodeDir("/home/alice/src/my_app/.claude/worktrees/fix"); got != "-home-alice-src-my-app--claude-worktrees-fix" {
		t.Errorf("EncodeDir = %q", got)
	}
}

func TestProjectRoundTrip(t *testing.T) {
	linux := New("/home/alice/")
	mac := New("/Users/alice")
	cases := []struct{ local, hub string }{
		{"-home-alice-src-demo", "~-src-demo"},
		{"-home-alice", "~"},
		{"-home-alice--claude-worktrees-x", "~--claude-worktrees-x"},
		// ホームの外と、ホームの名前で始まるだけの別のディレクトリはそのまま
		{"-home-alice2-src", "-home-alice2-src"},
		{"-tmp-scratch", "-tmp-scratch"},
	}
	for _, c := range cases {
		if got := linux.ProjectToHub(c.local); got != c.hub {
			t.Errorf("ProjectToHub(%q) = %q, want %q", c.local, got, c.hub)
		}
		if got := linux.ProjectFromHub(c.hub); got != c.local {
			t.Errorf("ProjectFromHub(%q) = %q, want %q", c.hub, got, c.local)
		}
	}
	// macOS で push したものを Linux で pull する
	if got := linux.ProjectFromHub(mac.ProjectToHub("-Users-alice-src-demo")); got != "-home-alice-src-demo" {
		t.Errorf("mac → linux = %q", got)
	}
}

func TestContentRoundTrip(t *testing.T) {
	linux := New("/home/alice")
	mac := New("/Users/alice")
	src := `{"type":"user","cwd":"/Users/alice/src/demo","message":{"content":"see /Users/alice/src/demo/a.go"}}
{"type":"user","cwd":"/Users/alice"}
{"type":"user","cwd":"/Users/alice2/x"}
{"type":"user","cwd":"/opt/x"}
`
	hub := `{"type":"user","cwd":"~/src/demo","message":{"content":"see /Users/alice/src/demo/a.go"}}
{"type":"user","cwd":"~"}
{"type":"user","cwd":"/Users/alice2/x"}
{"type":"user","cwd":"/opt/x"}
`
	onLinux := `{"type":"user","cwd":"/home/alice/src/demo","message":{"content":"see /Users/alice/src/demo/a.go"}}
{"type":"user","cwd":"/home/alice"}
{"type":"user","cwd":"/Users/alice2/x"}
{"type":"user","cwd":"/opt/x"}
`
	if got := string(mac.ContentToHub([]byte(src))); got != hub {
		t.Errorf("ContentToHub:\n%s\nwant\n%s", got, hub)
	}
	if got := string(linux.ContentFromHub([]byte(hub))); got != onLinux {
		t.Errorf("ContentFromHub:\n%s\nwant\n%s", got, onLinux)
	}
	// pull したものを push し直すと、ハブの中身と同じバイト列に戻る
	if got := string(linux.ContentToHub([]byte(onLinux))); got != hub {
		t.Errorf("re-push differs from hub:\n%s", got)
	}
}
