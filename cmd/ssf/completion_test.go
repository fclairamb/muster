package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestCompletionShells(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go missing")
	}
	bin := buildBinary(t)
	for _, shell := range []string{"bash", "zsh", "fish"} {
		t.Run(shell, func(t *testing.T) {
			out := runBin(t, bin, t.TempDir(), "completion", shell)
			if len(strings.TrimSpace(out)) == 0 {
				t.Fatalf("%s completion empty", shell)
			}
		})
	}
}

func TestCompletionUnknownShell(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go missing")
	}
	bin := buildBinary(t)
	cmd := exec.Command(bin, "completion", "tcsh")
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+t.TempDir(), "HOME="+t.TempDir())
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected error: %s", out)
	}
	if !strings.Contains(string(out), "unknown shell") {
		t.Fatalf("unexpected: %s", out)
	}
}

func TestRmCompleter(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go missing")
	}
	bin := buildBinary(t)
	xdg := t.TempDir()
	a := t.TempDir()
	b := t.TempDir()
	runBin(t, bin, xdg, a)
	runBin(t, bin, xdg, b)

	// urfave/cli/v3 triggers completion when --generate-shell-completion
	// appears as a trailing argument.
	cmd := exec.Command(bin, "rm", "--generate-shell-completion")
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+xdg, "HOME="+xdg)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("completer: %v %s", err, out)
	}
	s := string(out)
	// Both registered paths should appear in completion output. Use
	// filepath.EvalSymlinks-equivalent comparison via substring matches
	// against the basename, since stored paths may be symlink-resolved.
	for _, want := range []string{a, b} {
		// The stored path may be the symlink-resolved variant.
		base := filepathBase(want)
		if !strings.Contains(s, base) {
			t.Fatalf("completer output missing %q: %s", base, s)
		}
	}
}

// filepathBase avoids importing path/filepath at the top of this small test file.
func filepathBase(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[i+1:]
		}
	}
	return p
}
