package render

import (
	"testing"

	"github.com/fclairamb/ssf/internal/config"
	"github.com/fclairamb/ssf/internal/repoinfo"
)

func TestLineGitHub(t *testing.T) {
	got := Line(
		config.Dir{Path: "/x/datalake"},
		repoinfo.Info{IsGitHub: true, Org: "stonal-tech", Repo: "datalake", Branch: "main"},
		"s",
	)
	if got != "s/datalake [main]" {
		t.Fatalf("got %q", got)
	}
}

func TestLineDetached(t *testing.T) {
	got := Line(
		config.Dir{Path: "/x/datalake"},
		repoinfo.Info{IsGitHub: true, Org: "stonal-tech", Repo: "datalake", Branch: "HEAD detached"},
		"s",
	)
	if got != "s/datalake [HEAD detached]" {
		t.Fatalf("got %q", got)
	}
}

func TestLineNonGitHub(t *testing.T) {
	got := Line(config.Dir{Path: "/tmp/notes"}, repoinfo.Info{}, "")
	if got != "notes" {
		t.Fatalf("got %q", got)
	}
}
