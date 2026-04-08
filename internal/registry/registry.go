// Package registry exposes CRUD operations over the registered directories.
package registry

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/fclairamb/muster/internal/config"
)

// ErrNotFound is returned when an operation targets a path not present in the registry.
var ErrNotFound = errors.New("registry: dir not found")

// Registry wraps a config file path and provides CRUD over its Dirs slice.
type Registry struct {
	Path string
	Now  func() time.Time // injectable clock for tests; nil → time.Now
}

// New constructs a Registry rooted at path. If path is empty, DefaultPath is used.
func New(path string) (*Registry, error) {
	if path == "" {
		p, err := config.DefaultPath()
		if err != nil {
			return nil, err
		}
		path = p
	}
	return &Registry{Path: path}, nil
}

func (r *Registry) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now().UTC()
}

// Load returns the underlying Config.
func (r *Registry) Load() (config.Config, error) {
	return config.Load(r.Path)
}

// List returns the registered dirs in storage order.
func (r *Registry) List() ([]config.Dir, error) {
	c, err := r.Load()
	if err != nil {
		return nil, err
	}
	return c.Dirs, nil
}

// Add registers path (resolved to absolute) and updates its LastOpened.
// Idempotent: re-adding bumps LastOpened.
func (r *Registry) Add(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("abs path: %w", err)
	}
	c, err := r.Load()
	if err != nil {
		return err
	}
	now := r.now()
	for i, d := range c.Dirs {
		if d.Path == abs {
			c.Dirs[i].LastOpened = now
			return config.Save(r.Path, c)
		}
	}
	c.Dirs = append(c.Dirs, config.Dir{Path: abs, LastOpened: now})
	return config.Save(r.Path, c)
}

// Remove deletes path from the registry. No-op if absent.
func (r *Registry) Remove(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("abs path: %w", err)
	}
	c, err := r.Load()
	if err != nil {
		return err
	}
	out := c.Dirs[:0]
	for _, d := range c.Dirs {
		if d.Path != abs {
			out = append(out, d)
		}
	}
	c.Dirs = out
	return config.Save(r.Path, c)
}

// Touch bumps LastOpened for path. Returns ErrNotFound if absent.
func (r *Registry) Touch(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("abs path: %w", err)
	}
	c, err := r.Load()
	if err != nil {
		return err
	}
	for i, d := range c.Dirs {
		if d.Path == abs {
			c.Dirs[i].LastOpened = r.now()
			return config.Save(r.Path, c)
		}
	}
	return ErrNotFound
}
