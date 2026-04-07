// Package repoinfo inspects a directory to extract git repo metadata.
package repoinfo

import (
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// Info describes a directory's git status as far as ssf cares.
type Info struct {
	RepoRoot string
	Branch   string
	IsGitHub bool
	Org      string
	Repo     string
}

// Inspect runs git inside dir and returns its Info. Non-git dirs are not an
// error: they get RepoRoot=dir and IsGitHub=false.
func Inspect(dir string) (Info, error) {
	info := Info{RepoRoot: dir}

	root, err := runGit(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		// Not a git repo. Not an error.
		return info, nil
	}
	info.RepoRoot = strings.TrimSpace(root)

	branch, err := runGit(info.RepoRoot, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return info, fmt.Errorf("read branch: %w", err)
	}
	info.Branch = strings.TrimSpace(branch)
	if info.Branch == "HEAD" {
		info.Branch = "HEAD detached"
	}

	// Prefer upstream remote, fall back to origin.
	for _, remote := range []string{"upstream", "origin"} {
		url, err := runGit(info.RepoRoot, "remote", "get-url", remote)
		if err != nil {
			continue
		}
		org, repo, ok := parseGitHubURL(strings.TrimSpace(url))
		if ok {
			info.IsGitHub = true
			info.Org = org
			info.Repo = repo
			break
		}
	}

	return info, nil
}

func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w (%s)", strings.Join(args, " "), err, errBuf.String())
	}
	return out.String(), nil
}

var (
	sshRe   = regexp.MustCompile(`^git@github\.com:([^/]+)/(.+?)(?:\.git)?$`)
	httpsRe = regexp.MustCompile(`^https?://github\.com/([^/]+)/(.+?)(?:\.git)?$`)
)

func parseGitHubURL(url string) (org, repo string, ok bool) {
	if m := sshRe.FindStringSubmatch(url); m != nil {
		return m[1], m[2], true
	}
	if m := httpsRe.FindStringSubmatch(url); m != nil {
		return m[1], m[2], true
	}
	return "", "", false
}
