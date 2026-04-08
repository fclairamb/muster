package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildBinary compiles the muster binary into a tempdir and returns its path.
func buildBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "muster")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build: %v", err)
	}
	return bin
}

func runBin(t *testing.T, bin, xdg string, args ...string) string {
	t.Helper()
	// Strip TMUX so the test suite passes when run from inside a tmux
	// session — rootAction now refuses to launch the TUI in that case.
	env := stripTMUX(os.Environ())
	env = append(env, "XDG_CONFIG_HOME="+xdg, "HOME="+xdg)
	cmd := exec.Command(bin, args...)
	cmd.Env = env
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		t.Fatalf("run %v: %v %s", args, err, errBuf.String())
	}
	return out.String()
}

func stripTMUX(env []string) []string {
	out := env[:0]
	for _, e := range env {
		if strings.HasPrefix(e, "TMUX=") || strings.HasPrefix(e, "TMUX_PANE=") {
			continue
		}
		out = append(out, e)
	}
	dup := make([]string, len(out))
	copy(dup, out)
	return dup
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
	if !strings.Contains(out, "muster") || !strings.Contains(out, "USAGE") {
		t.Fatalf("--help did not produce help text:\n%s", out)
	}
}

func TestVersionFlag(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go missing")
	}
	bin := buildBinary(t)
	out := runBin(t, bin, t.TempDir(), "--version")
	if !strings.Contains(out, "muster") {
		t.Fatalf("--version output missing 'muster':\n%s", out)
	}
}

func TestRejectsNonexistentPath(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go missing")
	}
	bin := buildBinary(t)
	xdg := t.TempDir()
	cmd := exec.Command(bin, "/definitely/does/not/exist")
	cmd.Env = append(stripTMUX(os.Environ()), "XDG_CONFIG_HOME="+xdg, "HOME="+xdg)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected non-zero exit, output: %s", out)
	}
	if !strings.Contains(string(out), "no such file") && !strings.Contains(string(out), "does/not/exist") {
		t.Fatalf("unexpected error output: %s", out)
	}
}

func TestBareInvocationDoesNotRegisterCwd(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go missing")
	}
	bin := buildBinary(t)
	xdg := t.TempDir()
	cwd := t.TempDir()

	cmd := exec.Command(bin)
	cmd.Dir = cwd
	cmd.Env = append(stripTMUX(os.Environ()), "XDG_CONFIG_HOME="+xdg, "HOME="+xdg)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bare muster: %v %s", err, out)
	}
	// Registry must remain empty.
	cfg := filepath.Join(xdg, "muster", "config.toml")
	if b, err := os.ReadFile(cfg); err == nil {
		if strings.Contains(string(b), cwd) {
			t.Fatalf("bare muster added cwd to registry:\n%s", b)
		}
	}
}

func TestRejectsExtraArg(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go missing")
	}
	bin := buildBinary(t)
	xdg := t.TempDir()
	cmd := exec.Command(bin, "/tmp", "/var")
	cmd.Env = append(stripTMUX(os.Environ()), "XDG_CONFIG_HOME="+xdg, "HOME="+xdg)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected error for extra arg, output: %s", out)
	}
	if !strings.Contains(string(out), "at most one") {
		t.Fatalf("unexpected error: %s", out)
	}
}

func TestRefusesToLaunchInsideTmux(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go missing")
	}
	bin := buildBinary(t)
	xdg := t.TempDir()
	dir := t.TempDir()

	// Inject a fake $TMUX so the binary thinks it's running inside a
	// tmux session. The exact value doesn't matter — tmux sets it to
	// "/private/tmp/tmux-501/default,12345,0" or similar; muster only
	// checks for non-empty.
	env := append(stripTMUX(os.Environ()),
		"XDG_CONFIG_HOME="+xdg,
		"HOME="+xdg,
		"TMUX=/tmp/fake,1,0",
	)
	cmd := exec.Command(bin, dir)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected refusal, got success:\n%s", out)
	}
	if !strings.Contains(string(out), "tmux session") {
		t.Fatalf("unexpected error message:\n%s", out)
	}
	// And the entry must NOT have been registered: the refusal happens
	// before reg.Add.
	cfg := filepath.Join(xdg, "muster", "config.toml")
	if b, err := os.ReadFile(cfg); err == nil {
		if strings.Contains(string(b), dir) {
			t.Fatalf("dir was registered despite refusal:\n%s", b)
		}
	}
}

func TestSubcommandsAllowedInsideTmux(t *testing.T) {
	// Subcommands like `hook write`, `list`, `files`, `migrate` must
	// keep working from inside tmux — only the TUI-launching root
	// action refuses. The side panel literally runs `muster files`
	// from inside the muster tmux session.
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go missing")
	}
	bin := buildBinary(t)
	xdg := t.TempDir()
	repo := t.TempDir()

	env := append(stripTMUX(os.Environ()),
		"XDG_CONFIG_HOME="+xdg,
		"HOME="+xdg,
		"TMUX=/tmp/fake,1,0",
	)

	// `muster hook write` should still create the state file.
	hookCmd := exec.Command(bin, "hook", "write", "abc123", "ready")
	hookCmd.Dir = repo
	hookCmd.Env = env
	if out, err := hookCmd.CombinedOutput(); err != nil {
		t.Fatalf("hook write inside tmux failed: %v %s", err, out)
	}
	if _, err := os.Stat(filepath.Join(repo, ".muster", "state", "abc123.json")); err != nil {
		t.Fatalf("state file not created: %v", err)
	}

	// `muster list` should still work too (no entries, exits cleanly).
	listCmd := exec.Command(bin, "list")
	listCmd.Env = env
	if out, err := listCmd.CombinedOutput(); err != nil {
		t.Fatalf("list inside tmux failed: %v %s", err, out)
	}
}

func TestHookWriteFromEnv(t *testing.T) {
	// Canonical form: slug from $MUSTER_SLUG, kind from argv. This is
	// what the post-refactor settings.local.json installs.
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go missing")
	}
	bin := buildBinary(t)
	xdg := t.TempDir()
	repo := t.TempDir()

	cmd := exec.Command(bin, "hook", "write", "ready")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(),
		"XDG_CONFIG_HOME="+xdg,
		"HOME="+xdg,
		"MUSTER_SLUG=envslug42",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("hook write (env form): %v %s", err, out)
	}
	if _, err := os.Stat(filepath.Join(repo, ".muster", "state", "envslug42.json")); err != nil {
		t.Fatalf("state file for env-derived slug missing: %v", err)
	}
}

func TestHookWriteEnvFormRequiresSlug(t *testing.T) {
	// Without MUSTER_SLUG set, the canonical (1-arg) form must error
	// rather than silently writing to an empty slug.
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go missing")
	}
	bin := buildBinary(t)
	xdg := t.TempDir()
	repo := t.TempDir()

	cmd := exec.Command(bin, "hook", "write", "ready")
	cmd.Dir = repo
	cmd.Env = append(stripMusterSlug(os.Environ()),
		"XDG_CONFIG_HOME="+xdg, "HOME="+xdg,
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected error when MUSTER_SLUG unset, got success: %s", out)
	}
	if !strings.Contains(string(out), "MUSTER_SLUG") {
		t.Fatalf("error should mention MUSTER_SLUG, got: %s", out)
	}
}

func stripMusterSlug(env []string) []string {
	out := env[:0]
	for _, kv := range env {
		if strings.HasPrefix(kv, "MUSTER_SLUG=") {
			continue
		}
		out = append(out, kv)
	}
	return out
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
	path := repo + "/.muster/state/abc123.json"
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read state file: %v", err)
	}
	if !strings.Contains(string(b), "ready") {
		t.Fatalf("state file missing kind: %s", b)
	}
}
