// Package hooks installs and uninstalls Claude Code settings hooks for muster.
package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// hookEvents lists the Claude Code hook event names muster reacts to, the
// optional tool-name matcher (empty = fire for all tools), and the state
// kind each one writes.
//
// PreToolUse + AskUserQuestion → waiting_input is the way muster detects
// interactive multiple-choice questions, which Notification does NOT fire
// for. PostToolUse with the same matcher clears back to working once the
// user has answered and claude resumes.
var hookEvents = []hookEvent{
	{"SessionStart", "", "idle"},
	{"UserPromptSubmit", "", "working"},
	{"Stop", "", "ready"},
	{"Notification", "", "waiting_input"},
	{"PreToolUse", "AskUserQuestion", "waiting_input"},
	{"PostToolUse", "AskUserQuestion", "working"},
}

type hookEvent struct {
	Event   string
	Matcher string // tool name (or regex) — "" means fire for all tools
	Kind    string
}

// SettingsLocalPath is the path to a repo's local (gitignored) Claude Code
// settings file. muster writes hook entries here so they never end up in
// commits or merge conflicts. Claude Code merges this file on top of the
// committed settings.json automatically.
func SettingsLocalPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".claude", "settings.local.json")
}

// SettingsSharedPath is the team-shared, committed Claude Code settings
// file. muster only reads/scrubs it (never writes muster hook entries to
// it) so historical or upgraded installations can be moved into
// settings.local.json.
func SettingsSharedPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".claude", "settings.json")
}

// CommandPrefix is the literal prefix of every hook command muster installs
// into .claude/settings.local.json. Anything in the file starting with this
// string is considered a muster hook entry and is eligible for scrubbing or
// rewriting on (re)install.
const CommandPrefix = "muster hook write "

// command builds the shell command muster wires into each hook entry.
//
// IMPORTANT: this string is hard-coded into every .claude/settings.local.json
// file muster installs. Renaming "hook write" breaks every existing
// installation. The corresponding subcommand definition lives in
// cmd/muster/main.go's hookCommand(); keep them in sync and treat both as
// part of the public contract.
//
// HISTORY:
//   - "ssf hook write <slug> <kind>" — initial form, scrubbed by
//     UninstallLegacy on the ssf→muster rename.
//   - "muster hook write <slug> <kind>" — slug-in-argv form, used until
//     parallel claude instances per repo became a requirement. Scrubbed
//     and rewritten on next install.
//   - "muster hook write <kind>" — current form. The slug is read from
//     $MUSTER_SLUG, which muster injects into each tmux session via
//     `new-session -e MUSTER_SLUG=<slug>`. This lets multiple claude
//     instances share one settings.local.json while still routing their
//     hook writes to distinct on-disk state files.
func command(kind string) string {
	return CommandPrefix + kind
}

// Install merges muster hook entries into
// <repoRoot>/.claude/settings.local.json, preserving any unrelated keys.
// Idempotent and repo-scoped: a single repo gets one set of entries
// regardless of how many slugs (parallel claude instances) muster manages
// for it. Each entry's command reads its slug from $MUSTER_SLUG at runtime,
// which muster injects into the tmux session env.
//
// As a courtesy, Install also scrubs any stale muster hook entries (any
// command starting with CommandPrefix) from BOTH the local and the
// committed .claude/settings.json before reinstalling, so legacy
// "muster hook write <slug> <kind>" entries from older versions are
// upgraded automatically. Ensures .gitignore mentions settings.local.json.
func Install(repoRoot string) error {
	if err := scrubMusterFromShared(repoRoot); err != nil {
		return err
	}
	if err := EnsureGitignored(repoRoot); err != nil {
		// Non-fatal: the install itself succeeds. A read-only or unusual
		// repo shouldn't block hook installation.
		_ = err
	}
	settings, err := loadSettings(SettingsLocalPath(repoRoot))
	if err != nil {
		return err
	}
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = make(map[string]any)
	}
	// Scrub any stale muster entries (legacy slug-in-argv shape included)
	// before appending the canonical ones, so reinstalling upgrades old
	// installations in place.
	for _, ev := range hookEvents {
		removeHookByPrefix(hooks, ev.Event, CommandPrefix)
	}
	for _, ev := range hookEvents {
		appendHook(hooks, ev.Event, ev.Matcher, command(ev.Kind))
	}
	settings["hooks"] = hooks
	return saveSettings(SettingsLocalPath(repoRoot), settings)
}

// UninstallLegacy strips any leftover hook entries whose command starts with
// "ssf hook write " — the literal used by the project's previous name. Used
// by `muster migrate` to scrub legacy installations before re-installing the
// new commands. Independent of slug. Scrubs both the local and the shared
// settings file so historical installs in either location are cleaned up.
func UninstallLegacy(repoRoot string) error {
	for _, p := range []string{SettingsLocalPath(repoRoot), SettingsSharedPath(repoRoot)} {
		if err := mutateSettings(p, func(hooks map[string]any) {
			for _, ev := range hookEvents {
				removeHookByPrefix(hooks, ev.Event, "ssf hook write ")
			}
		}); err != nil {
			return err
		}
	}
	return nil
}

// Uninstall removes all muster hook entries (any command starting with
// CommandPrefix) from a repo. Repo-scoped — there's no per-slug uninstall
// in the env-var design because hooks are shared across all of a repo's
// claude instances. Cleans both the local and shared settings files.
func Uninstall(repoRoot string) error {
	for _, p := range []string{SettingsLocalPath(repoRoot), SettingsSharedPath(repoRoot)} {
		if err := mutateSettings(p, func(hooks map[string]any) {
			for _, ev := range hookEvents {
				removeHookByPrefix(hooks, ev.Event, CommandPrefix)
			}
		}); err != nil {
			return err
		}
	}
	return nil
}

// scrubMusterFromShared removes all muster hook entries from the committed
// .claude/settings.json. Used when (re)installing into the local file so a
// stale shared-file install left over from an older muster is cleaned up
// automatically.
func scrubMusterFromShared(repoRoot string) error {
	return mutateSettings(SettingsSharedPath(repoRoot), func(hooks map[string]any) {
		for _, ev := range hookEvents {
			removeHookByPrefix(hooks, ev.Event, CommandPrefix)
		}
	})
}

// mutateSettings loads a settings file, applies fn to its hooks map, then
// saves the result. If the file doesn't exist, fn is not called. After fn
// runs, an empty hooks map is dropped from the settings object, and an
// empty settings object causes the file to be removed entirely so muster
// never leaves an empty stub behind.
func mutateSettings(path string, fn func(hooks map[string]any)) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	settings, err := loadSettings(path)
	if err != nil {
		return err
	}
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = make(map[string]any)
	}
	fn(hooks)
	if len(hooks) == 0 {
		delete(settings, "hooks")
	} else {
		settings["hooks"] = hooks
	}
	if len(settings) == 0 {
		_ = os.Remove(path)
		return nil
	}
	return saveSettings(path, settings)
}

// EnsureGitignored appends `.claude/settings.local.json` to <repoRoot>/.gitignore
// when not already present, so the file muster writes never sneaks into a
// commit. No-op when the entry is already covered.
func EnsureGitignored(repoRoot string) error {
	gi := filepath.Join(repoRoot, ".gitignore")
	existing, err := os.ReadFile(gi)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	target := ".claude/settings.local.json"
	for _, line := range strings.Split(string(existing), "\n") {
		l := strings.TrimSpace(line)
		if l == target || l == "/"+target || l == ".claude/" || l == ".claude" {
			return nil
		}
	}
	var b strings.Builder
	if len(existing) > 0 && !strings.HasSuffix(string(existing), "\n") {
		b.WriteString("\n")
	}
	b.WriteString(target)
	b.WriteString("\n")
	return os.WriteFile(gi, append(existing, []byte(b.String())...), 0o644)
}

func loadSettings(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
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

func saveSettings(path string, m map[string]any) error {
	dir := filepath.Dir(path)
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
	return os.Rename(tmpName, path)
}

// appendHook adds a hook entry of the form
//
//	{ "matcher": "<matcher>", "hooks": [{"type":"command","command":"<cmd>"}] }
//
// under hooksMap[event], deduping by (matcher, command). The matcher key is
// omitted entirely when matcher is empty.
func appendHook(hooksMap map[string]any, event, matcher, cmd string) {
	entries, _ := hooksMap[event].([]any)
	for _, e := range entries {
		em, _ := e.(map[string]any)
		emMatcher, _ := em["matcher"].(string)
		if emMatcher != matcher {
			continue
		}
		inner, _ := em["hooks"].([]any)
		for _, h := range inner {
			hm, _ := h.(map[string]any)
			if c, _ := hm["command"].(string); c == cmd {
				return
			}
		}
	}
	entry := map[string]any{
		"hooks": []any{
			map[string]any{"type": "command", "command": cmd},
		},
	}
	if matcher != "" {
		entry["matcher"] = matcher
	}
	entries = append(entries, entry)
	hooksMap[event] = entries
}

// removeHookByPrefix drops any inner hook whose command starts with the
// given prefix string. Used both for slug-scoped removal (Uninstall) and
// legacy cleanup (UninstallLegacy).
func removeHookByPrefix(hooksMap map[string]any, event, prefix string) {
	entries, _ := hooksMap[event].([]any)
	out := entries[:0]
	for _, e := range entries {
		em, _ := e.(map[string]any)
		inner, _ := em["hooks"].([]any)
		filtered := inner[:0]
		for _, h := range inner {
			hm, _ := h.(map[string]any)
			c, _ := hm["command"].(string)
			if strings.HasPrefix(c, prefix) {
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

