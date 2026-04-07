package repoinfo

import (
	"os/exec"
	"testing"
)

func TestParseGitHubURL(t *testing.T) {
	cases := []struct {
		in        string
		org, repo string
		ok        bool
	}{
		{"git@github.com:stonal-tech/datalake.git", "stonal-tech", "datalake", true},
		{"git@github.com:stonal-tech/datalake", "stonal-tech", "datalake", true},
		{"https://github.com/fclairamb/solidping.git", "fclairamb", "solidping", true},
		{"https://github.com/fclairamb/solidping", "fclairamb", "solidping", true},
		{"http://github.com/foo/bar", "foo", "bar", true},
		{"git@gitlab.com:foo/bar.git", "", "", false},
		{"", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			org, repo, ok := parseGitHubURL(tc.in)
			if ok != tc.ok || org != tc.org || repo != tc.repo {
				t.Fatalf("got (%q, %q, %v), want (%q, %q, %v)", org, repo, ok, tc.org, tc.repo, tc.ok)
			}
		})
	}
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", "-q"},
		{"-c", "user.email=t@t.test", "-c", "user.name=t", "commit", "--allow-empty", "-m", "init", "-q"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
}

func gitRemote(t *testing.T, dir, name, url string) {
	t.Helper()
	cmd := exec.Command("git", "remote", "add", name, url)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git remote add: %v %s", err, out)
	}
}

func TestInspectGitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	gitInit(t, dir)
	gitRemote(t, dir, "origin", "git@github.com:stonal-tech/datalake.git")

	info, err := Inspect(dir)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if !info.IsGitHub {
		t.Fatal("expected IsGitHub")
	}
	if info.Org != "stonal-tech" || info.Repo != "datalake" {
		t.Fatalf("got %+v", info)
	}
}

func TestInspectPrefersUpstream(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	gitInit(t, dir)
	gitRemote(t, dir, "origin", "git@github.com:fclairamb/datalake.git")
	gitRemote(t, dir, "upstream", "git@github.com:stonal-tech/datalake.git")

	info, err := Inspect(dir)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if info.Org != "stonal-tech" {
		t.Fatalf("expected upstream org, got %q", info.Org)
	}
}

func TestInspectNonGit(t *testing.T) {
	dir := t.TempDir()
	info, err := Inspect(dir)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if info.IsGitHub {
		t.Fatal("non-git should not be IsGitHub")
	}
	if info.RepoRoot != dir {
		t.Fatalf("RepoRoot = %q, want %q", info.RepoRoot, dir)
	}
}
