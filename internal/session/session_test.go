package session

import (
	"reflect"
	"testing"
)

func TestBuildStartArgs(t *testing.T) {
	got := buildStartArgs("abc", "/repo", "claude", nil)
	want := []string{"-L", "ssf", "new-session", "-d", "-s", "ssf-abc", "-c", "/repo", "claude"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestBuildStartArgsWithClaudeArgs(t *testing.T) {
	got := buildStartArgs("abc", "/repo", "claude", []string{"--dangerously-skip-permissions", "--foo"})
	want := []string{"-L", "ssf", "new-session", "-d", "-s", "ssf-abc", "-c", "/repo", "claude", "--dangerously-skip-permissions", "--foo"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestFakeManagerLifecycle(t *testing.T) {
	f := NewFake()
	if f.Has("a") {
		t.Fatal("empty fake should not Has")
	}
	if err := f.Start("a", "/x"); err != nil {
		t.Fatal(err)
	}
	if !f.Has("a") {
		t.Fatal("Has should be true after Start")
	}
	// Idempotent.
	if err := f.Start("a", "/x"); err != nil {
		t.Fatal(err)
	}
	list, _ := f.List()
	if len(list) != 1 || list[0] != "a" {
		t.Fatalf("list = %v", list)
	}
	_ = f.Attach("a")
	if got := f.Attached(); len(got) != 1 || got[0] != "a" {
		t.Fatalf("attached = %v", got)
	}
	_ = f.Kill("a")
	if f.Has("a") {
		t.Fatal("Has should be false after Kill")
	}
}

func TestShouldSplitGating(t *testing.T) {
	cases := []struct {
		name string
		t    Tmux
		want bool
	}{
		{"disabled", Tmux{SidePanel: false, TerminalWidth: 200}, false},
		{"narrow terminal", Tmux{SidePanel: true, TerminalWidth: 80}, false},
		{"wide terminal", Tmux{SidePanel: true, TerminalWidth: 120}, true},
		{"unknown width assumes wide", Tmux{SidePanel: true, TerminalWidth: 0}, true},
		{"custom min", Tmux{SidePanel: true, TerminalWidth: 90, MinPanelWidth: 80}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.t.shouldSplit(); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestClaudeBinaryEnv(t *testing.T) {
	t.Setenv("SSF_CLAUDE_BINARY", "/path/to/fake")
	if got := claudeBinary(); got != "/path/to/fake" {
		t.Fatalf("got %q", got)
	}
	t.Setenv("SSF_CLAUDE_BINARY", "")
	if got := claudeBinary(); got != "claude" {
		t.Fatalf("default = %q", got)
	}
}
