package session

import "sync"

// FakeManager is an in-memory Manager implementation for tests.
type FakeManager struct {
	mu       sync.Mutex
	sessions map[string]string // slug → cwd
	attached []string
}

// NewFake constructs an empty FakeManager.
func NewFake() *FakeManager { return &FakeManager{sessions: map[string]string{}} }

// Start records a session for slug.
func (f *FakeManager) Start(slug, cwd string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.sessions[slug]; ok {
		return nil
	}
	f.sessions[slug] = cwd
	return nil
}

// Has reports whether a session for slug exists.
func (f *FakeManager) Has(slug string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.sessions[slug]
	return ok
}

// Attach records that an attach was requested but does not block.
func (f *FakeManager) Attach(slug string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.attached = append(f.attached, slug)
	return nil
}

// Kill removes a session.
func (f *FakeManager) Kill(slug string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.sessions, slug)
	return nil
}

// List returns all known session slugs.
func (f *FakeManager) List() ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.sessions))
	for s := range f.sessions {
		out = append(out, s)
	}
	return out, nil
}

// Attached returns the slugs Attach was called with, in order.
func (f *FakeManager) Attached() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.attached))
	copy(out, f.attached)
	return out
}
