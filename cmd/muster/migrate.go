package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v3"

	"github.com/fclairamb/muster/internal/config"
	"github.com/fclairamb/muster/internal/hooks"
	"github.com/fclairamb/muster/internal/registry"
	"github.com/fclairamb/muster/internal/repoinfo"
)

// migrateCommand returns the user-visible `muster migrate` subcommand.
//
// The same logic also runs automatically the first time muster is started
// and detects an old ssf install — see autoMigrateIfNeeded.
func migrateCommand() *cli.Command {
	return &cli.Command{
		Name:  "migrate",
		Usage: "migrate an existing ssf installation to muster",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runMigrate(os.Stdout)
		},
	}
}

// runMigrate is the actual migration logic. Idempotent: running twice is a
// no-op the second time.
func runMigrate(out io.Writer) error {
	oldCfg, newCfg, err := configPaths()
	if err != nil {
		return err
	}

	migratedConfig := false
	if _, err := os.Stat(newCfg); os.IsNotExist(err) {
		if _, err := os.Stat(oldCfg); err == nil {
			if err := copyFile(oldCfg, newCfg); err != nil {
				return fmt.Errorf("copy config: %w", err)
			}
			migratedConfig = true
		}
	}

	// Walk the (possibly just-migrated) muster registry and reinstall hooks
	// for each entry, scrubbing any leftover ssf hook lines first.
	reg, err := registry.New(newCfg)
	if err != nil {
		return err
	}
	dirs, err := reg.List()
	if err != nil {
		return err
	}

	migratedDirs := 0
	for _, d := range dirs {
		// Move <repo>/.ssf/state → <repo>/.muster/state if needed.
		oldStateDir := filepath.Join(d.Path, ".ssf", "state")
		newStateDir := filepath.Join(d.Path, ".muster", "state")
		if _, err := os.Stat(newStateDir); os.IsNotExist(err) {
			if _, err := os.Stat(oldStateDir); err == nil {
				_ = os.MkdirAll(filepath.Dir(newStateDir), 0o755)
				if err := os.Rename(oldStateDir, newStateDir); err != nil {
					fmt.Fprintf(out, "warn: rename %s → %s: %v\n", oldStateDir, newStateDir, err)
				}
			}
		}

		// Resolve repo root for hook installation.
		info, _ := repoinfo.Inspect(d.Path)
		repoRoot := info.RepoRoot
		if repoRoot == "" {
			repoRoot = d.Path
		}
		// Strip legacy `ssf hook write` entries before installing the new ones.
		if err := hooks.UninstallLegacy(repoRoot); err != nil {
			fmt.Fprintf(out, "warn: uninstall legacy hooks at %s: %v\n", repoRoot, err)
		}
		if err := hooks.Install(repoRoot); err != nil {
			fmt.Fprintf(out, "warn: reinstall hooks at %s: %v\n", repoRoot, err)
			continue
		}
		migratedDirs++
	}

	if migratedConfig {
		fmt.Fprintf(out, "muster: migrated configuration from %s\n", oldCfg)
	}
	fmt.Fprintf(out, "muster: %d repos reconciled, %d hook installations rewritten\n",
		len(dirs), migratedDirs)
	if migratedConfig {
		fmt.Fprintf(out, "muster: existing tmux sessions on the 'ssf' socket are still reachable via 'tmux -L ssf attach'\n")
	}
	return nil
}

// autoMigrateIfNeeded runs the migration silently when the trigger condition
// holds (old config exists, new config does not). Called from main() before
// the CLI dispatches.
func autoMigrateIfNeeded() {
	oldCfg, newCfg, err := configPaths()
	if err != nil {
		return
	}
	if _, err := os.Stat(newCfg); err == nil {
		return // already migrated
	}
	if _, err := os.Stat(oldCfg); err != nil {
		return // nothing to migrate
	}
	_ = runMigrate(os.Stderr)
}

// configPaths returns the (old, new) config file paths.
func configPaths() (string, string, error) {
	newCfg, err := config.DefaultPath()
	if err != nil {
		return "", "", err
	}
	// Old path is the same shape with "ssf" instead of "muster".
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "ssf", "config.toml"), newCfg, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	return filepath.Join(home, ".config", "ssf", "config.toml"), newCfg, nil
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, in, 0o644)
}
