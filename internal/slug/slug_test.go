package slug

import (
	"regexp"
	"testing"
)

func TestSlugStable(t *testing.T) {
	a := Slug("/repo/foo")
	b := Slug("/repo/foo")
	if a != b {
		t.Fatalf("slug not stable: %q vs %q", a, b)
	}
}

func TestSlugDistinct(t *testing.T) {
	if Slug("/a") == Slug("/b") {
		t.Fatal("expected distinct slugs")
	}
}

func TestSlugFormat(t *testing.T) {
	s := Slug("/repo")
	if len(s) != Length {
		t.Fatalf("length = %d, want %d", len(s), Length)
	}
	if !regexp.MustCompile(`^[0-9a-f]+$`).MatchString(s) {
		t.Fatalf("slug not lowercase hex: %q", s)
	}
}
