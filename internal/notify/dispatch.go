package notify

import (
	"sync"

	"github.com/fclairamb/ssf/internal/state"
	"github.com/fclairamb/ssf/internal/state/watcher"
)

// NameFunc resolves a slug to its display name (e.g. "s/datalake [main]").
type NameFunc func(slug string) string

// Dispatcher fires notifications on transitions to ready or waiting_input.
//
// Transitions are detected per slug. The first event for any slug primes the
// "last seen" map but does not notify; subsequent events that change the kind
// to ready/waiting_input fire a notification.
type Dispatcher struct {
	notifier Notifier
	name     NameFunc
	mu       sync.Mutex
	last     map[string]state.Kind
}

// NewDispatcher constructs a Dispatcher. name may be nil, in which case the
// raw slug is used as the display name.
func NewDispatcher(n Notifier, name NameFunc) *Dispatcher {
	if name == nil {
		name = func(s string) string { return s }
	}
	return &Dispatcher{
		notifier: n,
		name:     name,
		last:     map[string]state.Kind{},
	}
}

// Handle reacts to one watcher event.
func (d *Dispatcher) Handle(ev watcher.Event) {
	d.mu.Lock()
	prev, seen := d.last[ev.Slug]
	d.last[ev.Slug] = ev.State.Kind
	d.mu.Unlock()

	// First-ever event for a slug: prime, do not notify.
	if !seen {
		return
	}
	// Same kind twice: silent.
	if prev == ev.State.Kind {
		return
	}

	switch ev.State.Kind {
	case state.KindReady:
		_ = d.notifier.Notify(Notification{
			Title:    "ssf",
			Subtitle: d.name(ev.Slug),
			Body:     "Ready",
			Sound:    "Glass",
			Group:    "ssf-" + ev.Slug,
		})
	case state.KindWaitingInput:
		_ = d.notifier.Notify(Notification{
			Title:    "ssf",
			Subtitle: d.name(ev.Slug),
			Body:     "Needs input",
			Sound:    "Funk",
			Group:    "ssf-" + ev.Slug,
		})
	}
}
