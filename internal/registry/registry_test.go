package registry

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func newReg(t *testing.T) *Registry {
	t.Helper()
	dir := t.TempDir()
	r, err := New(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	return r
}

func TestAddListRoundTrip(t *testing.T) {
	r := newReg(t)
	if err := r.Add("/repo/one"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := r.Add("/repo/two"); err != nil {
		t.Fatalf("add: %v", err)
	}
	dirs, err := r.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(dirs) != 2 {
		t.Fatalf("expected 2 dirs, got %d", len(dirs))
	}
}

func TestAddIsIdempotent(t *testing.T) {
	r := newReg(t)
	now := time.Now().UTC()
	r.Now = func() time.Time { return now }
	if err := r.Add("/repo/one"); err != nil {
		t.Fatal(err)
	}
	r.Now = func() time.Time { return now.Add(time.Hour) }
	if err := r.Add("/repo/one"); err != nil {
		t.Fatal(err)
	}
	dirs, _ := r.List()
	if len(dirs) != 1 {
		t.Fatalf("expected 1 dir, got %d", len(dirs))
	}
	if !dirs[0].LastOpened.Equal(now.Add(time.Hour)) {
		t.Fatalf("LastOpened not bumped: %v", dirs[0].LastOpened)
	}
}

func TestAddResolvesRelative(t *testing.T) {
	r := newReg(t)
	if err := r.Add("."); err != nil {
		t.Fatalf("add .: %v", err)
	}
	dirs, _ := r.List()
	if !filepath.IsAbs(dirs[0].Path) {
		t.Fatalf("path not absolute: %q", dirs[0].Path)
	}
}

func TestRemove(t *testing.T) {
	r := newReg(t)
	_ = r.Add("/a")
	_ = r.Add("/b")
	if err := r.Remove("/a"); err != nil {
		t.Fatal(err)
	}
	dirs, _ := r.List()
	if len(dirs) != 1 || dirs[0].Path != "/b" {
		t.Fatalf("after remove: %+v", dirs)
	}
}

func TestRemoveAbsent(t *testing.T) {
	r := newReg(t)
	if err := r.Remove("/never"); err != nil {
		t.Fatalf("remove absent should be no-op, got %v", err)
	}
}

func TestTouchMissingReturnsErrNotFound(t *testing.T) {
	r := newReg(t)
	err := r.Touch("/never")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestTouchBumps(t *testing.T) {
	r := newReg(t)
	now := time.Now().UTC()
	r.Now = func() time.Time { return now }
	_ = r.Add("/a")
	r.Now = func() time.Time { return now.Add(time.Minute) }
	if err := r.Touch("/a"); err != nil {
		t.Fatal(err)
	}
	dirs, _ := r.List()
	if !dirs[0].LastOpened.Equal(now.Add(time.Minute)) {
		t.Fatalf("touch did not bump")
	}
}
