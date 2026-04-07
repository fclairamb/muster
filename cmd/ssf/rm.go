package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v3"

	"github.com/fclairamb/ssf/internal/config"
	"github.com/fclairamb/ssf/internal/hooks"
	"github.com/fclairamb/ssf/internal/registry"
	"github.com/fclairamb/ssf/internal/repoinfo"
	"github.com/fclairamb/ssf/internal/slug"
)

func rmCommand() *cli.Command {
	return &cli.Command{
		Name:      "rm",
		Usage:     "unregister a directory by path or slug",
		ArgsUsage: "<path-or-slug>",
		Action:    runRm,
	}
}

// errAmbiguous is returned when a slug arg matches more than one entry.
var errAmbiguous = errors.New("ambiguous: multiple entries matched")

func runRm(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() != 1 {
		return fmt.Errorf("usage: ssf rm <path-or-slug>")
	}
	arg := cmd.Args().First()

	cfgPath, err := config.DefaultPath()
	if err != nil {
		return err
	}
	reg, err := registry.New(cfgPath)
	if err != nil {
		return err
	}
	dirs, err := reg.List()
	if err != nil {
		return err
	}

	target, err := resolveTarget(dirs, arg)
	if err != nil {
		return err
	}
	if err := unregister(reg, target); err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, "unregistered", target)
	return nil
}

// resolveTarget returns the canonical path matching arg, which may be either
// an absolute path (must match an entry exactly) or a 12-char slug (must match
// the slug derived from exactly one entry's path).
func resolveTarget(dirs []config.Dir, arg string) (string, error) {
	// Path mode: resolve to abs and look for an exact match.
	if filepath.IsAbs(arg) || arg == "." {
		abs, err := filepath.Abs(arg)
		if err != nil {
			return "", err
		}
		for _, d := range dirs {
			if d.Path == abs {
				return d.Path, nil
			}
		}
		// Also try with symlink resolution via repoinfo, in case the
		// stored path was symlink-resolved on insert.
		if info, err := repoinfo.Inspect(abs); err == nil {
			for _, d := range dirs {
				if d.Path == info.RepoRoot {
					return d.Path, nil
				}
			}
		}
		return "", fmt.Errorf("not registered: %s", arg)
	}

	// Slug mode: exact match against slug.Slug(d.Path).
	var matches []string
	for _, d := range dirs {
		if slug.Slug(d.Path) == arg {
			matches = append(matches, d.Path)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("not registered: %s", arg)
	case 1:
		return matches[0], nil
	default:
		return "", errAmbiguous
	}
}

// unregister is the shared registered-dir removal path used by both the TUI's
// `r` action and the `ssf rm` subcommand.
func unregister(reg *registry.Registry, path string) error {
	if err := reg.Remove(path); err != nil {
		return err
	}
	if info, err := repoinfo.Inspect(path); err == nil {
		_ = hooks.Uninstall(info.RepoRoot, slug.Slug(info.RepoRoot))
	}
	return nil
}
