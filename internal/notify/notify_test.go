package notify

import (
	"testing"

	"github.com/fclairamb/muster/internal/state"
	"github.com/fclairamb/muster/internal/state/watcher"
)

func TestEscape(t *testing.T) {
	cases := map[string]string{
		`hello`:           `hello`,
		`with "quotes"`:   `with \"quotes\"`,
		`back\slash`:      `back\\slash`,
		`evil"; rm -rf /`: `evil\"; rm -rf /`,
	}
	for in, want := range cases {
		if got := escape(in); got != want {
			t.Errorf("escape(%q) = %q, want %q", in, got, want)
		}
	}
}

type step struct {
	slug string
	kind state.Kind
}

func feed(d *Dispatcher, steps ...step) {
	for _, s := range steps {
		d.Handle(watcher.Event{Slug: s.slug, State: state.State{Kind: s.kind}})
	}
}

func bodies(notes []Notification) []string {
	out := make([]string, len(notes))
	for i, n := range notes {
		out[i] = n.Body
	}
	return out
}

func TestDispatcherTransitions(t *testing.T) {
	cases := []struct {
		name  string
		steps []step
		want  []string // expected bodies in order
	}{
		{"first event primes only", []step{{"a", state.KindReady}}, nil},
		{"working then ready notifies once", []step{{"a", state.KindWorking}, {"a", state.KindReady}}, []string{"Ready"}},
		{"ready then ready is silent", []step{{"a", state.KindIdle}, {"a", state.KindReady}, {"a", state.KindReady}}, []string{"Ready"}},
		{"working then waiting_input notifies", []step{{"a", state.KindWorking}, {"a", state.KindWaitingInput}}, []string{"Needs input"}},
		{"idle to working is silent", []step{{"a", state.KindIdle}, {"a", state.KindWorking}}, nil},
		{"two slugs independent",
			[]step{{"a", state.KindWorking}, {"a", state.KindReady}, {"b", state.KindWorking}, {"b", state.KindWaitingInput}},
			[]string{"Ready", "Needs input"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &RecordingNotifier{}
			d := NewDispatcher(rec, nil)
			feed(d, tc.steps...)
			got := bodies(rec.Snapshot())
			if len(got) != len(tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v want %v", got, tc.want)
				}
			}
		})
	}
}

func TestDispatcherEmitsSubtitleAndSound(t *testing.T) {
	rec := &RecordingNotifier{}
	d := NewDispatcher(rec, func(slug string) string { return "s/datalake [main]" })
	feed(d,
		step{"abc", state.KindWorking},
		step{"abc", state.KindReady},
	)
	calls := rec.Snapshot()
	if len(calls) != 1 {
		t.Fatalf("got %d calls", len(calls))
	}
	n := calls[0]
	if n.Title != "ssf" {
		t.Errorf("title = %q, want %q", n.Title, "ssf")
	}
	if n.Subtitle != "s/datalake [main]" {
		t.Errorf("subtitle = %q", n.Subtitle)
	}
	if n.Sound != "Glass" {
		t.Errorf("sound = %q, want Glass for ready", n.Sound)
	}
	if n.Group != "ssf-abc" {
		t.Errorf("group = %q", n.Group)
	}
}

func TestDispatcherWaitingInputUsesFunkSound(t *testing.T) {
	rec := &RecordingNotifier{}
	d := NewDispatcher(rec, nil)
	feed(d,
		step{"abc", state.KindWorking},
		step{"abc", state.KindWaitingInput},
	)
	calls := rec.Snapshot()
	if len(calls) != 1 || calls[0].Sound != "Funk" {
		t.Fatalf("got %v", calls)
	}
}

func TestDetectTerminalBundleID(t *testing.T) {
	cases := map[string]string{
		"iTerm.app":      "com.googlecode.iterm2",
		"Apple_Terminal": "com.apple.Terminal",
		"ghostty":        "com.mitchellh.ghostty",
		"WezTerm":        "com.github.wez.wezterm",
		"unknown":        "",
	}
	for env, want := range cases {
		t.Run(env, func(t *testing.T) {
			t.Setenv("TERM_PROGRAM", env)
			if got := detectTerminalBundleID(); got != want {
				t.Fatalf("got %q want %q", got, want)
			}
		})
	}
}
