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
	b, err := os.ReadFile(SettingsPath(repo))
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
	_ = os.MkdirAll(filepath.Dir(SettingsPath(repo)), 0o755)
	pre := `{"permissions": {"allow": ["Bash"]}}`
	if err := os.WriteFile(SettingsPath(repo), []byte(pre), 0o644); err != nil {
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
	first, _ := os.ReadFile(SettingsPath(repo))
	if err := Install(repo, "abc"); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(SettingsPath(repo))
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
	settings, _ := loadSettings(repo)
	hooksMap, _ := settings["hooks"].(map[string]any)
	stop, _ := hooksMap["Stop"].([]any)
	stop = append(stop, map[string]any{
		"hooks": []any{
			map[string]any{"type": "command", "command": "ssf hook write abc ready"},
		},
	})
	hooksMap["Stop"] = stop
	settings["hooks"] = hooksMap
	if err := saveSettings(repo, settings); err != nil {
		t.Fatal(err)
	}

	if err := UninstallLegacy(repo); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(SettingsPath(repo))
	if strings.Contains(string(b), "ssf hook write") {
		t.Fatalf("legacy entry survived: %s", b)
	}
	if !strings.Contains(string(b), "muster hook write") {
		t.Fatalf("current entry removed by mistake: %s", b)
	}
}

func TestUninstall(t *testing.T) {
	repo := t.TempDir()
	_ = Install(repo, "abc")
	_ = Install(repo, "xyz")
	if err := Uninstall(repo, "abc"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(SettingsPath(repo))
	if strings.Contains(string(b), "abc") {
		t.Fatal("abc still present after uninstall")
	}
	if !strings.Contains(string(b), "xyz") {
		t.Fatal("xyz removed when only abc should be")
	}
}
