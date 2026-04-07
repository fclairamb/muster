// Package hooks installs and uninstalls Claude Code settings hooks for ssf.
package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// hookEvents lists the Claude Code hook event names ssf reacts to and the
// state kind each one writes.
var hookEvents = []struct {
	Event string
	Kind  string
}{
	{"SessionStart", "idle"},
	{"UserPromptSubmit", "working"},
	{"Stop", "ready"},
	{"Notification", "waiting_input"},
}

// SettingsPath is the path to a repo's local Claude Code settings file.
func SettingsPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".claude", "settings.json")
}

// command builds the shell command ssf wires into each hook entry.
//
// IMPORTANT: this string is hard-coded into every .claude/settings.json
// file ssf installs. Renaming "hook write" or reordering its arguments
// breaks every existing installation. The corresponding subcommand
// definition lives in cmd/ssf/main.go's hookCommand(); keep them in sync
// and treat both as part of the public contract.
func command(slug, kind string) string {
	return "ssf hook write " + slug + " " + kind
}

// Install merges ssf hook entries into <repoRoot>/.claude/settings.json,
// preserving any unrelated keys. Idempotent.
func Install(repoRoot, slug string) error {
	settings, err := loadSettings(repoRoot)
	if err != nil {
		return err
	}
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = make(map[string]any)
	}

	for _, ev := range hookEvents {
		cmd := command(slug, ev.Kind)
		appendHook(hooks, ev.Event, cmd)
	}
	settings["hooks"] = hooks
	return saveSettings(repoRoot, settings)
}

// Uninstall removes only the entries whose command matches our slug.
func Uninstall(repoRoot, slug string) error {
	settings, err := loadSettings(repoRoot)
	if err != nil {
		return err
	}
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		return nil
	}
	for _, ev := range hookEvents {
		removeHook(hooks, ev.Event, slug)
	}
	settings["hooks"] = hooks
	return saveSettings(repoRoot, settings)
}

func loadSettings(repoRoot string) (map[string]any, error) {
	b, err := os.ReadFile(SettingsPath(repoRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("read settings: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parse settings: %w", err)
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

func saveSettings(repoRoot string, m map[string]any) error {
	dir := filepath.Dir(SettingsPath(repoRoot))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir settings dir: %w", err)
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".settings.*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, SettingsPath(repoRoot))
}

// appendHook adds a hook entry of the form
//
//	{ "hooks": [{"type":"command","command":"<cmd>"}] }
//
// under hooksMap[event], deduping by exact command string.
func appendHook(hooksMap map[string]any, event, cmd string) {
	entries, _ := hooksMap[event].([]any)
	// Dedupe.
	for _, e := range entries {
		em, _ := e.(map[string]any)
		inner, _ := em["hooks"].([]any)
		for _, h := range inner {
			hm, _ := h.(map[string]any)
			if c, _ := hm["command"].(string); c == cmd {
				return
			}
		}
	}
	entries = append(entries, map[string]any{
		"hooks": []any{
			map[string]any{"type": "command", "command": cmd},
		},
	})
	hooksMap[event] = entries
}

// removeHook drops any inner hook whose command contains
// "ssf hook write <slug>".
func removeHook(hooksMap map[string]any, event, slug string) {
	needle := "ssf hook write " + slug + " "
	entries, _ := hooksMap[event].([]any)
	out := entries[:0]
	for _, e := range entries {
		em, _ := e.(map[string]any)
		inner, _ := em["hooks"].([]any)
		filtered := inner[:0]
		for _, h := range inner {
			hm, _ := h.(map[string]any)
			c, _ := hm["command"].(string)
			if strings.HasPrefix(c, needle) {
				continue
			}
			filtered = append(filtered, h)
		}
		if len(filtered) == 0 {
			continue
		}
		em["hooks"] = filtered
		out = append(out, em)
	}
	if len(out) == 0 {
		delete(hooksMap, event)
		return
	}
	hooksMap[event] = out
}
