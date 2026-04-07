// Package watcher watches state file directories and emits debounced events.
package watcher

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/fclairamb/ssf/internal/state"
)

// Defaults for the debounce / green-confirm windows. Tests may shorten them.
var (
	DebounceWindow = 250 * time.Millisecond
	GreenConfirm   = 2 * time.Second
)

// Event is a coalesced state change for one (repoRoot, slug).
type Event struct {
	RepoRoot string
	Slug     string
	State    state.State
}

// Watch starts a goroutine that watches each repoRoot's .ssf/state directory
// and emits events on the returned channel until ctx is cancelled.
//
// Repos whose .ssf/state directory does not yet exist are still tracked: the
// watcher polls for the directory's creation every second.
func Watch(ctx context.Context, repoRoots []string) (<-chan Event, error) {
	out := make(chan Event, 16)

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	// Map state-dir → repo root for reverse lookup.
	dirToRepo := make(map[string]string)
	var mu sync.Mutex
	tryAdd := func(repoRoot string) {
		dir := state.DirPath(repoRoot)
		mu.Lock()
		defer mu.Unlock()
		if _, ok := dirToRepo[dir]; ok {
			return
		}
		if err := w.Add(dir); err != nil {
			return
		}
		dirToRepo[dir] = repoRoot
	}
	for _, r := range repoRoots {
		tryAdd(r)
	}

	go func() {
		defer close(out)
		defer w.Close()

		// Pending events keyed by (repoRoot, slug). Coalesced over DebounceWindow.
		type pendingKey struct{ repo, slug string }
		pending := make(map[pendingKey]*time.Timer)
		var pendingMu sync.Mutex

		// Track sessions in KindReady that may be flipped by an imminent
		// Working/WaitingInput within GreenConfirm.
		readyTimers := make(map[pendingKey]*time.Timer)
		var readyMu sync.Mutex

		emit := func(repoRoot, slug string) {
			st, err := state.Read(repoRoot, slug)
			if err != nil {
				slog.Warn("read state", "repo", repoRoot, "slug", slug, "err", err)
				return
			}
			key := pendingKey{repoRoot, slug}

			// Green-confirm: wait GreenConfirm before emitting Ready, unless
			// a Working/WaitingInput supersedes it.
			if st.Kind == state.KindReady {
				readyMu.Lock()
				if t, ok := readyTimers[key]; ok {
					t.Stop()
				}
				readyTimers[key] = time.AfterFunc(GreenConfirm, func() {
					readyMu.Lock()
					delete(readyTimers, key)
					readyMu.Unlock()
					select {
					case out <- Event{RepoRoot: repoRoot, Slug: slug, State: st}:
					case <-ctx.Done():
					}
				})
				readyMu.Unlock()
				return
			}

			// Any non-Ready event cancels a pending Ready emission.
			readyMu.Lock()
			if t, ok := readyTimers[key]; ok {
				t.Stop()
				delete(readyTimers, key)
			}
			readyMu.Unlock()

			select {
			case out <- Event{RepoRoot: repoRoot, Slug: slug, State: st}:
			case <-ctx.Done():
			}
		}

		schedule := func(repoRoot, slug string) {
			key := pendingKey{repoRoot, slug}
			pendingMu.Lock()
			defer pendingMu.Unlock()
			if t, ok := pending[key]; ok {
				t.Stop()
			}
			pending[key] = time.AfterFunc(DebounceWindow, func() {
				pendingMu.Lock()
				delete(pending, key)
				pendingMu.Unlock()
				emit(repoRoot, slug)
			})
		}

		// Background: re-add directories that didn't exist at startup.
		go func() {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					for _, r := range repoRoots {
						tryAdd(r)
					}
				}
			}
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				name := ev.Name
				base := filepath.Base(name)
				if !strings.HasSuffix(base, ".json") {
					continue
				}
				slug := strings.TrimSuffix(base, ".json")
				dir := filepath.Dir(name)
				mu.Lock()
				repoRoot, known := dirToRepo[dir]
				mu.Unlock()
				if !known {
					continue
				}
				if ev.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Rename) == 0 {
					continue
				}
				schedule(repoRoot, slug)
			case err, ok := <-w.Errors:
				if !ok {
					return
				}
				slog.Warn("watcher error", "err", err)
			}
		}
	}()

	return out, nil
}
