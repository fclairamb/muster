// Command ssf orchestrates a set of Claude Code instances across worktrees.
package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/fclairamb/ssf/internal/config"
	"github.com/fclairamb/ssf/internal/orgprefix"
	"github.com/fclairamb/ssf/internal/registry"
	"github.com/fclairamb/ssf/internal/render"
	"github.com/fclairamb/ssf/internal/repoinfo"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "ssf:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr *os.File) error {
	// Subcommand routing.
	if len(args) >= 1 && args[0] == "hook" {
		return runHook(args[1:], stderr)
	}

	cfgPath, err := config.DefaultPath()
	if err != nil {
		return err
	}
	reg, err := registry.New(cfgPath)
	if err != nil {
		return err
	}

	// Determine target directory: argv[0] if present, else cwd.
	var target string
	if len(args) >= 1 {
		target = args[0]
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		target = cwd
	}
	if err := reg.Add(target); err != nil {
		return fmt.Errorf("register dir: %w", err)
	}

	dirs, err := reg.List()
	if err != nil {
		return err
	}
	infos := make([]repoinfo.Info, len(dirs))
	orgs := make([]string, 0, len(dirs))
	for i, d := range dirs {
		info, _ := repoinfo.Inspect(d.Path)
		infos[i] = info
		if info.IsGitHub {
			orgs = append(orgs, info.Org)
		}
	}
	prefixes := orgprefix.Derive(orgs, nil)

	// Sort: most recently opened first (status sorting comes in slice 08).
	idx := make([]int, len(dirs))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(i, j int) bool {
		return dirs[idx[i]].LastOpened.After(dirs[idx[j]].LastOpened)
	})

	for _, i := range idx {
		fmt.Fprintln(stdout, render.Line(dirs[i], infos[i], prefixes[infos[i].Org]))
	}
	return nil
}

func runHook(args []string, stderr *os.File) error {
	if len(args) >= 2 && args[0] == "write" {
		return fmt.Errorf("not implemented")
	}
	return fmt.Errorf("usage: ssf hook write <slug> <state>")
}
