package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildBinary compiles the ssf binary into a tempdir and returns its path.
func buildBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "ssf")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build: %v", err)
	}
	return bin
}

func runBin(t *testing.T, bin, xdg string, args ...string) string {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+xdg, "HOME="+xdg)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		t.Fatalf("run %v: %v %s", args, err, errBuf.String())
	}
	return out.String()
}

func TestCLIRegisterAndList(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain missing")
	}
	bin := buildBinary(t)
	xdg := t.TempDir()

	foo := t.TempDir()
	bar := t.TempDir()

	out := runBin(t, bin, xdg, foo)
	if !strings.Contains(out, filepath.Base(foo)) {
		t.Fatalf("first register output missing %q: %q", filepath.Base(foo), out)
	}

	out = runBin(t, bin, xdg, bar)
	if !strings.Contains(out, filepath.Base(foo)) || !strings.Contains(out, filepath.Base(bar)) {
		t.Fatalf("second register missing entries: %q", out)
	}
	// Newest first: bar should appear before foo.
	if i := strings.Index(out, filepath.Base(bar)); i == -1 || i > strings.Index(out, filepath.Base(foo)) {
		t.Fatalf("expected bar before foo: %q", out)
	}

	// Re-register foo: it should now be first.
	out = runBin(t, bin, xdg, foo)
	if i := strings.Index(out, filepath.Base(foo)); i == -1 || i > strings.Index(out, filepath.Base(bar)) {
		t.Fatalf("expected foo before bar after touch: %q", out)
	}
}

func TestHelpFlag(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go missing")
	}
	bin := buildBinary(t)
	out := runBin(t, bin, t.TempDir(), "--help")
	if !strings.Contains(out, "ssf") || !strings.Contains(out, "USAGE") {
		t.Fatalf("--help did not produce help text:\n%s", out)
	}
}

func TestVersionFlag(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go missing")
	}
	bin := buildBinary(t)
	out := runBin(t, bin, t.TempDir(), "--version")
	if !strings.Contains(out, "ssf") {
		t.Fatalf("--version output missing 'ssf':\n%s", out)
	}
}

func TestRejectsNonexistentPath(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go missing")
	}
	bin := buildBinary(t)
	xdg := t.TempDir()
	cmd := exec.Command(bin, "/definitely/does/not/exist")
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+xdg, "HOME="+xdg)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit, output: %s", out)
	}
	if !strings.Contains(string(out), "no such file") && !strings.Contains(string(out), "does/not/exist") {
		t.Fatalf("unexpected error output: %s", out)
	}
}

func TestRejectsExtraArg(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go missing")
	}
	bin := buildBinary(t)
	xdg := t.TempDir()
	cmd := exec.Command(bin, "/tmp", "/var")
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+xdg, "HOME="+xdg)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected error for extra arg, output: %s", out)
	}
	if !strings.Contains(string(out), "at most one") {
		t.Fatalf("unexpected error: %s", out)
	}
}

func TestHookWriteCreatesStateFile(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go missing")
	}
	bin := buildBinary(t)
	xdg := t.TempDir()
	repo := t.TempDir()

	cmd := exec.Command(bin, "hook", "write", "abc123", "ready")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+xdg, "HOME="+xdg)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("hook write: %v %s", err, out)
	}
	path := repo + "/.ssf/state/abc123.json"
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read state file: %v", err)
	}
	if !strings.Contains(string(b), "ready") {
		t.Fatalf("state file missing kind: %s", b)
	}
}
