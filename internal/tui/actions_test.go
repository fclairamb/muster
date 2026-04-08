package tui

import (
	"reflect"
	"testing"
)

func TestBuildWorktreeAddArgs(t *testing.T) {
	got := BuildWorktreeAddArgs("/code/datalake", "feat/x")
	want := []string{"-C", "/code/datalake", "worktree", "add", "/code/datalake/.muster/worktrees/datalake-feat-x", "-b", "feat/x"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestBuildWorktreeRemoveArgs(t *testing.T) {
	got := BuildWorktreeRemoveArgs("/wt", false)
	want := []string{"worktree", "remove", "/wt"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
	got = BuildWorktreeRemoveArgs("/wt", true)
	want = []string{"worktree", "remove", "--force", "/wt"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestValidateBranchName(t *testing.T) {
	good := []string{"feat/x", "fix-bug", "main", "release/v1.2.3"}
	bad := []string{"", " ", "with space", "/abs", "-flag", "foo..bar", "../escape"}
	for _, s := range good {
		if err := ValidateBranchName(s); err != nil {
			t.Errorf("expected %q valid, got %v", s, err)
		}
	}
	for _, s := range bad {
		if err := ValidateBranchName(s); err == nil {
			t.Errorf("expected %q invalid", s)
		}
	}
}
