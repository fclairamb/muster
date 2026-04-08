package config

import (
	"path/filepath"
	"testing"
	"time"
)

func TestLoadMissingReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	c, err := Load(filepath.Join(dir, "nope.toml"))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(c.Dirs) != 0 {
		t.Fatalf("expected empty dirs, got %d", len(c.Dirs))
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	now := time.Now().UTC().Truncate(time.Second)
	in := Config{
		Dirs: []Dir{
			{Path: "/repo/one", LastOpened: now},
			{Path: "/repo/two", LastOpened: now.Add(-time.Hour)},
		},
		Settings: Settings{
			FileManager:  "yazi",
			Editor:       "nvim",
			OrgOverrides: map[string]string{"microsoft": "ms"},
		},
	}
	if err := Save(path, in); err != nil {
		t.Fatalf("save: %v", err)
	}
	out, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(out.Dirs) != 2 {
		t.Fatalf("expected 2 dirs, got %d", len(out.Dirs))
	}
	if out.Dirs[0].Path != "/repo/one" {
		t.Fatalf("dir[0].Path = %q", out.Dirs[0].Path)
	}
	if !out.Dirs[0].LastOpened.Equal(now) {
		t.Fatalf("dir[0].LastOpened = %v, want %v", out.Dirs[0].LastOpened, now)
	}
	if out.Settings.FileManager != "yazi" {
		t.Fatalf("settings.FileManager = %q", out.Settings.FileManager)
	}
	if out.Settings.OrgOverrides["microsoft"] != "ms" {
		t.Fatalf("orgOverrides not preserved")
	}
}

func TestSaveCreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deeply", "nested", "config.toml")
	if err := Save(path, Config{}); err != nil {
		t.Fatalf("save: %v", err)
	}
}

func TestDefaultPathHonorsXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg")
	p, err := DefaultPath()
	if err != nil {
		t.Fatalf("default path: %v", err)
	}
	if p != "/tmp/xdg/muster/config.toml" {
		t.Fatalf("got %q", p)
	}
}

func TestSettingsDefaults(t *testing.T) {
	t.Setenv("FILE_MANAGER", "")
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	s := Settings{}
	if got := s.ResolveFileManager(); got != "open" {
		t.Fatalf("default file manager = %q", got)
	}
	if got := s.ResolveEditor(); got != "zed" {
		t.Fatalf("default editor = %q", got)
	}
}

func TestSettingsConfigOverridesEnv(t *testing.T) {
	t.Setenv("VISUAL", "vim")
	s := Settings{Editor: "nvim"}
	if got := s.ResolveEditor(); got != "nvim" {
		t.Fatalf("config should win, got %q", got)
	}
}

func TestResolveClaudeArgsDefault(t *testing.T) {
	s := Settings{}
	got := s.ResolveClaudeArgs()
	if len(got) != 1 || got[0] != "--dangerously-skip-permissions" {
		t.Fatalf("default = %v", got)
	}
}

func TestResolveClaudeArgsExplicitEmpty(t *testing.T) {
	empty := []string{}
	s := Settings{ClaudeArgs: &empty}
	if got := s.ResolveClaudeArgs(); len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}

func TestResolveClaudeArgsCustom(t *testing.T) {
	custom := []string{"--foo", "--bar"}
	s := Settings{ClaudeArgs: &custom}
	got := s.ResolveClaudeArgs()
	if len(got) != 2 || got[0] != "--foo" || got[1] != "--bar" {
		t.Fatalf("got %v", got)
	}
}

func TestResolveSidePanelDefault(t *testing.T) {
	s := Settings{}
	if !s.ResolveSidePanel() {
		t.Fatal("default should be true")
	}
}

func TestResolveSidePanelExplicitFalse(t *testing.T) {
	f := false
	s := Settings{SidePanel: &f}
	if s.ResolveSidePanel() {
		t.Fatal("explicit false should be honored")
	}
}

func TestSettingsEnvFallback(t *testing.T) {
	t.Setenv("FILE_MANAGER", "ranger")
	t.Setenv("EDITOR", "vi")
	t.Setenv("VISUAL", "")
	s := Settings{}
	if got := s.ResolveFileManager(); got != "ranger" {
		t.Fatalf("file manager env fallback failed: %q", got)
	}
	if got := s.ResolveEditor(); got != "vi" {
		t.Fatalf("editor env fallback failed: %q", got)
	}
}
