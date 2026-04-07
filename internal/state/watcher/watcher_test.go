package watcher

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/fclairamb/ssf/internal/state"
)

func init() {
	DebounceWindow = 30 * time.Millisecond
	GreenConfirm = 80 * time.Millisecond
}

func waitFor(t *testing.T, ch <-chan Event, timeout time.Duration) (Event, bool) {
	t.Helper()
	select {
	case ev, ok := <-ch:
		return ev, ok
	case <-time.After(timeout):
		return Event{}, false
	}
}

func TestWatcherEmitsOnWrite(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(state.DirPath(dir), 0o755)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := Watch(ctx, []string{dir})
	if err != nil {
		t.Fatalf("watch: %v", err)
	}

	if err := state.Write(dir, "abc", state.State{Kind: state.KindWorking}); err != nil {
		t.Fatalf("write: %v", err)
	}
	ev, ok := waitFor(t, ch, time.Second)
	if !ok {
		t.Fatal("no event")
	}
	if ev.Slug != "abc" || ev.State.Kind != state.KindWorking {
		t.Fatalf("got %+v", ev)
	}
}

func TestWatcherDebouncesBurst(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(state.DirPath(dir), 0o755)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := Watch(ctx, []string{dir})
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 5; i++ {
		_ = state.Write(dir, "abc", state.State{Kind: state.KindWorking})
	}
	if _, ok := waitFor(t, ch, time.Second); !ok {
		t.Fatal("expected at least one event")
	}
	// Drain: any extras within 200ms is a debounce failure.
	if _, ok := waitFor(t, ch, 200*time.Millisecond); ok {
		t.Fatal("expected debounce to coalesce burst into one event")
	}
}

func TestGreenConfirmation(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(state.DirPath(dir), 0o755)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := Watch(ctx, []string{dir})
	if err != nil {
		t.Fatal(err)
	}

	_ = state.Write(dir, "abc", state.State{Kind: state.KindReady})
	ev, ok := waitFor(t, ch, GreenConfirm+500*time.Millisecond)
	if !ok {
		t.Fatal("no event")
	}
	if ev.State.Kind != state.KindReady {
		t.Fatalf("expected ready, got %q", ev.State.Kind)
	}
}

func TestGreenSupersededByWorking(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(state.DirPath(dir), 0o755)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := Watch(ctx, []string{dir})
	if err != nil {
		t.Fatal(err)
	}

	_ = state.Write(dir, "abc", state.State{Kind: state.KindReady})
	time.Sleep(DebounceWindow + 10*time.Millisecond)
	_ = state.Write(dir, "abc", state.State{Kind: state.KindWorking})

	ev, ok := waitFor(t, ch, time.Second)
	if !ok {
		t.Fatal("no event")
	}
	if ev.State.Kind != state.KindWorking {
		t.Fatalf("expected working, got %q", ev.State.Kind)
	}
	// No further ready event should arrive: the green-confirm timer was cancelled.
	if _, ok := waitFor(t, ch, GreenConfirm+200*time.Millisecond); ok {
		t.Fatal("expected superseded green to be cancelled")
	}
}
