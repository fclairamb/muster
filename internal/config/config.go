// Package config loads and persists the on-disk ssf configuration.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

// Config is the on-disk shape of ~/.config/ssf/config.toml.
type Config struct {
	Dirs     []Dir    `toml:"dirs"`
	Settings Settings `toml:"settings"`
}

// Dir is a registered directory.
type Dir struct {
	Path       string    `toml:"path"`
	LastOpened time.Time `toml:"last_opened"`
}

// Settings is the user-tunable configuration.
type Settings struct {
	FileManager  string            `toml:"file_manager"`
	Editor       string            `toml:"editor"`
	OrgOverrides map[string]string `toml:"org_overrides"`
}

// DefaultPath returns the location of the config file, honoring XDG_CONFIG_HOME.
func DefaultPath() (string, error) {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "ssf", "config.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".config", "ssf", "config.toml"), nil
}

// Load reads the config from path. A missing file returns an empty Config and no error.
func Load(path string) (Config, error) {
	var c Config
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, nil
		}
		return c, fmt.Errorf("read config: %w", err)
	}
	if err := toml.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("parse config: %w", err)
	}
	return c, nil
}

// Save writes the config to path atomically (tempfile + rename).
func Save(path string, c Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir config dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".config.toml.*.tmp")
	if err != nil {
		return fmt.Errorf("create tempfile: %w", err)
	}
	tmpName := tmp.Name()
	encoder := toml.NewEncoder(tmp)
	if err := encoder.Encode(c); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("encode config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close tempfile: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("rename tempfile: %w", err)
	}
	return nil
}

// ResolveFileManager returns the configured file manager, falling back to $FILE_MANAGER, then "open".
func (s Settings) ResolveFileManager() string {
	if s.FileManager != "" {
		return s.FileManager
	}
	if v := os.Getenv("FILE_MANAGER"); v != "" {
		return v
	}
	return "open"
}

// ResolveEditor returns the configured editor, falling back to $VISUAL, $EDITOR, then "zed".
func (s Settings) ResolveEditor() string {
	if s.Editor != "" {
		return s.Editor
	}
	if v := os.Getenv("VISUAL"); v != "" {
		return v
	}
	if v := os.Getenv("EDITOR"); v != "" {
		return v
	}
	return "zed"
}
