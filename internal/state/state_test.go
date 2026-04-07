package state

import (
	"os"
	"testing"
	"time"
)

func TestReadMissingReturnsNone(t *testing.T) {
	dir := t.TempDir()
	s, err := Read(dir, "abc")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if s.Kind != KindNone {
		t.Fatalf("expected KindNone, got %q", s.Kind)
	}
}

func TestWriteReadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	in := State{Kind: KindReady, Ts: time.Now().UTC().Truncate(time.Second), Session: "ssf-abc"}
	if err := Write(dir, "abc", in); err != nil {
		t.Fatalf("write: %v", err)
	}
	out, err := Read(dir, "abc")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if out.Kind != in.Kind || out.Session != in.Session {
		t.Fatalf("got %+v, want %+v", out, in)
	}
}

func TestReadCorruptReturnsNone(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(DirPath(dir), 0o755)
	if err := os.WriteFile(FilePath(dir, "abc"), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Read(dir, "abc")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if s.Kind != KindNone {
		t.Fatalf("expected KindNone, got %q", s.Kind)
	}
}
