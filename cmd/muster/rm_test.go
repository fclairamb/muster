package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// pathSlug mirrors slug.Slug for the test without importing internal pkgs
// directly (we already do, but this keeps the test self-contained).
func pathSlug(p string) string {
	sum := sha256.Sum256([]byte(p))
	return hex.EncodeToString(sum[:])[:12]
}

func TestRmByPath(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go missing")
	}
	bin := buildBinary(t)
	xdg := t.TempDir()
	a := t.TempDir()
	runBin(t, bin, xdg, a)

	cmd := exec.Command(bin, "rm", a)
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+xdg, "HOME="+xdg)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rm: %v %s", err, out)
	}
	if !strings.Contains(string(out), "unregistered") {
		t.Fatalf("expected confirmation: %s", out)
	}
	// Confirm via list.
	listOut := runBin(t, bin, xdg, "list")
	if strings.TrimSpace(listOut) != "" {
		t.Fatalf("entry still present after rm: %q", listOut)
	}
}

func TestRmBySlug(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go missing")
	}
	bin := buildBinary(t)
	xdg := t.TempDir()
	a := t.TempDir()
	runBin(t, bin, xdg, a)

	// The stored path is symlink-resolved by repoinfo, so compute slug
	// from the resolved path.
	resolved, err := filepath.EvalSymlinks(a)
	if err != nil {
		resolved = a
	}
	s := pathSlug(resolved)

	cmd := exec.Command(bin, "rm", s)
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+xdg, "HOME="+xdg)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Slug computed from non-resolved path may differ; try the raw path slug.
		s2 := pathSlug(a)
		cmd = exec.Command(bin, "rm", s2)
		cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+xdg, "HOME="+xdg)
		out, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("rm by slug: %v %s", err, out)
		}
	}
	if !strings.Contains(string(out), "unregistered") {
		t.Fatalf("expected confirmation: %s", out)
	}
}

func TestRmNotFound(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go missing")
	}
	bin := buildBinary(t)
	xdg := t.TempDir()
	cmd := exec.Command(bin, "rm", "/never/registered")
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+xdg, "HOME="+xdg)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected error: %s", out)
	}
	if !strings.Contains(string(out), "not registered") {
		t.Fatalf("unexpected error: %s", out)
	}
}

func TestVersionSubcommand(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go missing")
	}
	bin := buildBinary(t)
	out := runBin(t, bin, t.TempDir(), "version")
	if !strings.Contains(out, "muster version") {
		t.Fatalf("unexpected: %q", out)
	}
}
