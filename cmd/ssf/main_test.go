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
