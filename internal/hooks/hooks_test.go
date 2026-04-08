package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readSettings(t *testing.T, repo string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(SettingsLocalPath(repo))
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("parse: %v", err)
	}
	return m
}

func TestInstallFresh(t *testing.T) {
	repo := t.TempDir()
	if err := Install(repo, "abc"); err != nil {
		t.Fatalf("install: %v", err)
	}
	m := readSettings(t, repo)
	hooks, _ := m["hooks"].(map[string]any)
	for _, ev := range []string{"SessionStart", "UserPromptSubmit", "Stop", "Notification"} {
		entries, _ := hooks[ev].([]any)
		if len(entries) != 1 {
			t.Fatalf("event %s: expected 1 entry, got %d", ev, len(entries))
		}
	}
}

func TestInstallPreservesUnrelatedKeys(t *testing.T) {
	repo := t.TempDir()
	_ = os.MkdirAll(filepath.Dir(SettingsLocalPath(repo)), 0o755)
	pre := `{"permissions": {"allow": ["Bash"]}}`
	if err := os.WriteFile(SettingsLocalPath(repo), []byte(pre), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Install(repo, "abc"); err != nil {
		t.Fatal(err)
	}
	m := readSettings(t, repo)
	if _, ok := m["permissions"]; !ok {
		t.Fatal("permissions key dropped")
	}
}

func TestInstallIdempotent(t *testing.T) {
	repo := t.TempDir()
	if err := Install(repo, "abc"); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(SettingsLocalPath(repo))
	if err := Install(repo, "abc"); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(SettingsLocalPath(repo))
	if string(first) != string(second) {
		t.Fatalf("install not idempotent:\nfirst:  %s\nsecond: %s", first, second)
	}
}

func TestInstallSecondSlugCoexists(t *testing.T) {
	repo := t.TempDir()
	_ = Install(repo, "abc")
	_ = Install(repo, "xyz")
	m := readSettings(t, repo)
	hooks := m["hooks"].(map[string]any)
	entries := hooks["Stop"].([]any)
	if len(entries) != 2 {
		t.Fatalf("expected 2 Stop entries, got %d", len(entries))
	}
}

func TestInstallWritesAskUserQuestionMatcher(t *testing.T) {
	repo := t.TempDir()
	if err := Install(repo, "abc"); err != nil {
		t.Fatal(err)
	}
	m := readSettings(t, repo)
	hooks, _ := m["hooks"].(map[string]any)
	for _, ev := range []string{"PreToolUse", "PostToolUse"} {
		entries, _ := hooks[ev].([]any)
		if len(entries) == 0 {
			t.Fatalf("event %s missing", ev)
		}
		em, _ := entries[0].(map[string]any)
		if em["matcher"] != "AskUserQuestion" {
			t.Fatalf("event %s missing AskUserQuestion matcher: %v", ev, em)
		}
	}
}

func TestUninstallLegacy(t *testing.T) {
	repo := t.TempDir()
	if err := Install(repo, "abc"); err != nil {
		t.Fatal(err)
	}
	// Inject a legacy `ssf hook write` entry by hand.
	settings, _ := loadSettings(SettingsLocalPath(repo))
	hooksMap, _ := settings["hooks"].(map[string]any)
	stop, _ := hooksMap["Stop"].([]any)
	stop = append(stop, map[string]any{
		"hooks": []any{
			map[string]any{"type": "command", "command": "ssf hook write abc ready"},
		},
	})
	hooksMap["Stop"] = stop
	settings["hooks"] = hooksMap
	if err := saveSettings(SettingsLocalPath(repo), settings); err != nil {
		t.Fatal(err)
	}

	if err := UninstallLegacy(repo); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(SettingsLocalPath(repo))
	if strings.Contains(string(b), "ssf hook write") {
		t.Fatalf("legacy entry survived: %s", b)
	}
	if !strings.Contains(string(b), "muster hook write") {
		t.Fatalf("current entry removed by mistake: %s", b)
	}
}

func TestInstallScrubsSharedSettingsForSlug(t *testing.T) {
	// Stage a pre-existing committed settings.json containing both an old
	// muster entry for our slug AND an unrelated permissions key. After
	// Install runs, the slug entries must be gone from settings.json (so
	// nothing muster-related lingers in the team-shared file), the
	// unrelated key must survive, and the new entries land in
	// settings.local.json.
	repo := t.TempDir()
	_ = os.MkdirAll(filepath.Join(repo, ".claude"), 0o755)
	pre := `{
		"permissions": {"allow": ["Bash"]},
		"hooks": {
			"Stop": [{
				"hooks": [{"type":"command","command":"muster hook write abc ready"}]
			}]
		}
	}`
	if err := os.WriteFile(SettingsSharedPath(repo), []byte(pre), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Install(repo, "abc"); err != nil {
		t.Fatal(err)
	}
	// settings.json must still exist (permissions key) and must no longer
	// contain any muster hook references.
	sharedBytes, err := os.ReadFile(SettingsSharedPath(repo))
	if err != nil {
		t.Fatalf("shared settings should still exist: %v", err)
	}
	if strings.Contains(string(sharedBytes), "muster hook write") {
		t.Fatalf("muster entry survived in shared settings:\n%s", sharedBytes)
	}
	if !strings.Contains(string(sharedBytes), "permissions") {
		t.Fatalf("permissions key dropped from shared settings:\n%s", sharedBytes)
	}
	// settings.local.json holds the canonical install.
	localBytes, _ := os.ReadFile(SettingsLocalPath(repo))
	if !strings.Contains(string(localBytes), "muster hook write abc") {
		t.Fatalf("muster entry not installed in local settings:\n%s", localBytes)
	}
}

func TestInstallRemovesEmptyStubSharedSettings(t *testing.T) {
	// When the shared settings.json contained ONLY a muster entry, after
	// scrubbing the file should be removed entirely so the project tree
	// doesn't carry an empty stub forever.
	repo := t.TempDir()
	_ = os.MkdirAll(filepath.Join(repo, ".claude"), 0o755)
	pre := `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"muster hook write abc ready"}]}]}}`
	if err := os.WriteFile(SettingsSharedPath(repo), []byte(pre), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Install(repo, "abc"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(SettingsSharedPath(repo)); !os.IsNotExist(err) {
		t.Fatalf("expected empty shared settings to be removed, got err=%v", err)
	}
}

func TestEnsureGitignored(t *testing.T) {
	repo := t.TempDir()
	if err := EnsureGitignored(repo); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(repo, ".gitignore"))
	if err != nil {
		t.Fatalf("gitignore not created: %v", err)
	}
	if !strings.Contains(string(b), ".claude/settings.local.json") {
		t.Fatalf("gitignore missing entry:\n%s", b)
	}
	// Idempotent: a second call must not append a duplicate.
	if err := EnsureGitignored(repo); err != nil {
		t.Fatal(err)
	}
	b2, _ := os.ReadFile(filepath.Join(repo, ".gitignore"))
	if strings.Count(string(b2), ".claude/settings.local.json") != 1 {
		t.Fatalf("expected exactly one entry, got:\n%s", b2)
	}
}

func TestEnsureGitignoredHonorsBroaderPatterns(t *testing.T) {
	// If the user already ignores `.claude/` wholesale, EnsureGitignored
	// must not append a redundant settings.local.json line.
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(".claude/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureGitignored(repo); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(repo, ".gitignore"))
	if strings.Contains(string(b), "settings.local.json") {
		t.Fatalf("appended redundant entry:\n%s", b)
	}
}

func TestUninstall(t *testing.T) {
	repo := t.TempDir()
	_ = Install(repo, "abc")
	_ = Install(repo, "xyz")
	if err := Uninstall(repo, "abc"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(SettingsLocalPath(repo))
	if strings.Contains(string(b), "abc") {
		t.Fatal("abc still present after uninstall")
	}
	if !strings.Contains(string(b), "xyz") {
		t.Fatal("xyz removed when only abc should be")
	}
}
