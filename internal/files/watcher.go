package files

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Tick fires whenever the file watcher detects activity in dir, debounced.
// A 5-second safety tick also fires unconditionally so missed fsnotify
// events don't leave the panel stale.
func Tick(ctx context.Context, dir string) <-chan struct{} {
	ch := make(chan struct{}, 1)

	go func() {
		defer close(ch)

		w, err := fsnotify.NewWatcher()
		if err == nil {
			defer w.Close()
			addRecursive(w, dir)
		}

		// Initial tick so the first frame paints immediately.
		ch <- struct{}{}

		var (
			mu      sync.Mutex
			pending bool
			timer   *time.Timer
		)
		fire := func() {
			mu.Lock()
			pending = false
			mu.Unlock()
			select {
			case ch <- struct{}{}:
			default:
			}
		}
		schedule := func() {
			mu.Lock()
			defer mu.Unlock()
			if pending {
				return
			}
			pending = true
			timer = time.AfterFunc(250*time.Millisecond, fire)
		}
		_ = timer

		safety := time.NewTicker(5 * time.Second)
		defer safety.Stop()

		var events <-chan fsnotify.Event
		if w != nil {
			events = w.Events
		}
		for {
			select {
			case <-ctx.Done():
				return
			case <-safety.C:
				schedule()
			case ev := <-events:
				if shouldSkip(ev.Name) {
					continue
				}
				schedule()
			}
		}
	}()

	return ch
}

// addRecursive walks dir and adds every directory to the watcher, skipping
// well-known build/dependency dirs and anything that starts with a dot.
func addRecursive(w *fsnotify.Watcher, dir string) {
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if shouldSkipDir(base) {
			return filepath.SkipDir
		}
		_ = w.Add(path)
		return nil
	})
}

var skipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	"target":       true,
	"dist":         true,
	"build":        true,
	".idea":        true,
	".vscode":      true,
}

func shouldSkipDir(name string) bool {
	if strings.HasPrefix(name, ".") && name != "." {
		return true
	}
	return skipDirs[name]
}

func shouldSkip(path string) bool {
	parts := strings.Split(path, string(filepath.Separator))
	for _, p := range parts {
		if shouldSkipDir(p) {
			return true
		}
	}
	return false
}
