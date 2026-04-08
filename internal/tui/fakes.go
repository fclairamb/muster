package tui

import "sync"

// FakeGit captures Run/IsDirty calls. Configure Dirty to control IsDirty.
// RunFunc, when non-nil, supplies the (output, error) for each Run call so
// tests can stub commands like `git branch --format=...`.
type FakeGit struct {
	mu      sync.Mutex
	Calls   [][]string // each entry is [dir, args...]
	Dirty   bool
	RunFunc func(dir string, args []string) (string, error)
}

// Run records the call and returns no output (or RunFunc's result if set).
func (f *FakeGit) Run(dir string, args ...string) (string, error) {
	f.mu.Lock()
	f.Calls = append(f.Calls, append([]string{dir}, args...))
	fn := f.RunFunc
	f.mu.Unlock()
	if fn != nil {
		return fn(dir, args)
	}
	return "", nil
}

// IsDirty returns the configured dirty flag.
func (f *FakeGit) IsDirty(dir string) (bool, error) {
	return f.Dirty, nil
}

// Snapshot returns a copy of recorded calls.
func (f *FakeGit) Snapshot() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]string, len(f.Calls))
	for i, c := range f.Calls {
		out[i] = append([]string(nil), c...)
	}
	return out
}

// FakeOpener captures Open calls.
type FakeOpener struct {
	mu    sync.Mutex
	Calls []OpenCall
}

// OpenCall is a recorded Open invocation.
type OpenCall struct {
	Binary string
	Path   string
}

// Open records the call.
func (f *FakeOpener) Open(binary, path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, OpenCall{Binary: binary, Path: path})
	return nil
}

// Snapshot returns a copy of recorded calls.
func (f *FakeOpener) Snapshot() []OpenCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]OpenCall, len(f.Calls))
	copy(out, f.Calls)
	return out
}
