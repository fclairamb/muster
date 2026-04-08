package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestMigrateEndToEnd stages an old `ssf` install (old config dir, legacy
// state files, legacy hooks) and verifies `muster migrate` rewrites
// everything correctly and is idempotent on a second run.
func TestMigrateEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go missing")
	}
	bin := buildBinary(t)
	xdg := t.TempDir()

	// Stage a real git repo with a legacy state file and a legacy
	// .claude/settings.json that references `ssf hook write`.
	repo := t.TempDir()
	gitInit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	gitInit("init", "-q")
	gitInit("-c", "user.email=t@t.test", "-c", "user.name=t", "commit", "--allow-empty", "-m", "init", "-q")

	// Legacy state file under .ssf/state/.
	legacyState := filepath.Join(repo, ".ssf", "state")
	if err := os.MkdirAll(legacyState, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyState, "abc123.json"), []byte(`{"kind":"idle"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Legacy .claude/settings.json with an ssf hook write entry.
	if err := os.MkdirAll(filepath.Join(repo, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	legacySettings := `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"ssf hook write abc123 ready"}]}]}}`
	if err := os.WriteFile(filepath.Join(repo, ".claude", "settings.json"), []byte(legacySettings), 0o644); err != nil {
		t.Fatal(err)
	}

	// Legacy ssf config pointing at the repo.
	oldCfgDir := filepath.Join(xdg, "ssf")
	if err := os.MkdirAll(oldCfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "[[dirs]]\n  path = \"" + repo + "\"\n  last_opened = 2026-04-08T00:00:00Z\n"
	if err := os.WriteFile(filepath.Join(oldCfgDir, "config.toml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	// Run migrate.
	cmd := exec.Command(bin, "migrate")
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+xdg, "HOME="+xdg)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("migrate: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "migrated configuration") {
		t.Fatalf("migrate output missing confirmation:\n%s", out)
	}

	// Assertions:
	// 1. New config exists.
	newCfg := filepath.Join(xdg, "muster", "config.toml")
	if _, err := os.Stat(newCfg); err != nil {
		t.Fatalf("new config not created: %v", err)
	}
	// 2. Old config still exists (recovery breadcrumb).
	if _, err := os.Stat(filepath.Join(oldCfgDir, "config.toml")); err != nil {
		t.Fatalf("old config should be left in place: %v", err)
	}
	// 3. State dir was renamed.
	if _, err := os.Stat(filepath.Join(repo, ".muster", "state", "abc123.json")); err != nil {
		t.Fatalf(".muster/state not populated: %v", err)
	}
	if _, err := os.Stat(legacyState); !os.IsNotExist(err) {
		t.Fatalf(".ssf/state should be gone after rename: %v", err)
	}
	// 4. .claude/settings.json contained ONLY a legacy ssf entry; after
	//    migration the legacy entry is scrubbed AND the empty stub file
	//    is removed entirely. The muster entries land in settings.local.json
	//    where they don't pollute commits.
	if _, err := os.Stat(filepath.Join(repo, ".claude", "settings.json")); !os.IsNotExist(err) {
		b, _ := os.ReadFile(filepath.Join(repo, ".claude", "settings.json"))
		t.Fatalf("legacy settings.json should be removed, still exists:\n%s", b)
	}
	localBytes, err := os.ReadFile(filepath.Join(repo, ".claude", "settings.local.json"))
	if err != nil {
		t.Fatalf("settings.local.json missing: %v", err)
	}
	if strings.Contains(string(localBytes), "ssf hook write") {
		t.Fatalf("legacy hook entry survived migration:\n%s", localBytes)
	}
	if !strings.Contains(string(localBytes), "muster hook write") {
		t.Fatalf("muster hook entry not installed:\n%s", localBytes)
	}
	// 5. .gitignore was updated to ignore the new file.
	gi, err := os.ReadFile(filepath.Join(repo, ".gitignore"))
	if err != nil {
		t.Fatalf(".gitignore not created: %v", err)
	}
	if !strings.Contains(string(gi), ".claude/settings.local.json") {
		t.Fatalf(".gitignore missing entry:\n%s", gi)
	}

	// Idempotency: re-running migrate should be a no-op (no error).
	cmd2 := exec.Command(bin, "migrate")
	cmd2.Env = append(os.Environ(), "XDG_CONFIG_HOME="+xdg, "HOME="+xdg)
	if out, err := cmd2.CombinedOutput(); err != nil {
		t.Fatalf("idempotent migrate: %v\n%s", err, out)
	}
}
