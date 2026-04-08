package files

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitInit(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", "-q"},
		{"-c", "user.email=t@t.test", "-c", "user.name=t", "commit", "--allow-empty", "-m", "init", "-q"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
}

func TestRenderEmpty(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git missing")
	}
	dir := t.TempDir()
	gitInit(t, dir)

	var buf bytes.Buffer
	Render(&buf, dir, 80)
	out := buf.String()
	if !strings.Contains(out, "clean") {
		t.Fatalf("expected 'clean', got:\n%s", out)
	}
}

func TestRenderModified(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git missing")
	}
	dir := t.TempDir()
	gitInit(t, dir)

	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	add := exec.Command("git", "add", "tracked.txt")
	add.Dir = dir
	add.Run()
	commit := exec.Command("git", "-c", "user.email=t@t.test", "-c", "user.name=t", "commit", "-m", "x", "-q")
	commit.Dir = dir
	commit.Run()

	if err := os.WriteFile(filepath.Join(dir, "tracked.txt"), []byte("a\nb\nc\nd\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	Render(&buf, dir, 80)
	out := buf.String()
	if !strings.Contains(out, "modified") || !strings.Contains(out, "tracked.txt") {
		t.Fatalf("expected modified tracked.txt, got:\n%s", out)
	}
}

func TestRenderUntracked(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git missing")
	}
	dir := t.TempDir()
	gitInit(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "scratch.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	Render(&buf, dir, 80)
	out := buf.String()
	if !strings.Contains(out, "untracked") || !strings.Contains(out, "scratch.txt") {
		t.Fatalf("expected untracked scratch.txt, got:\n%s", out)
	}
}

func TestTruncatePath(t *testing.T) {
	cases := []struct {
		in     string
		maxLen int
		want   string
	}{
		{"short", 10, "short"},
		{"way/too/long/path/here.go", 10, "…/here.go"},
		{"abc", 0, "abc"},
	}
	for _, tc := range cases {
		got := truncatePath(tc.in, tc.maxLen)
		if got != tc.want {
			t.Errorf("truncatePath(%q, %d) = %q want %q", tc.in, tc.maxLen, got, tc.want)
		}
	}
}

func TestShouldSkipDir(t *testing.T) {
	cases := map[string]bool{
		".git":         true,
		"node_modules": true,
		"src":          false,
		".hidden":      true,
		".":            false,
	}
	for in, want := range cases {
		if got := shouldSkipDir(in); got != want {
			t.Errorf("shouldSkipDir(%q) = %v want %v", in, got, want)
		}
	}
}
