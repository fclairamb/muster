package notify

import (
	"reflect"
	"testing"

	"github.com/fclairamb/ssf/internal/state"
	"github.com/fclairamb/ssf/internal/state/watcher"
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

func TestDispatcherTransitions(t *testing.T) {
	cases := []struct {
		name  string
		steps []step
		want  []Recording
	}{
		{
			name:  "first event primes only",
			steps: []step{{"a", state.KindReady}},
			want:  nil,
		},
		{
			name:  "working then ready notifies once",
			steps: []step{{"a", state.KindWorking}, {"a", state.KindReady}},
			want:  []Recording{{Title: "ssf · a", Body: "Ready"}},
		},
		{
			name:  "ready then ready is silent",
			steps: []step{{"a", state.KindIdle}, {"a", state.KindReady}, {"a", state.KindReady}},
			want:  []Recording{{Title: "ssf · a", Body: "Ready"}},
		},
		{
			name:  "working then waiting_input notifies",
			steps: []step{{"a", state.KindWorking}, {"a", state.KindWaitingInput}},
			want:  []Recording{{Title: "ssf · a", Body: "Needs input"}},
		},
		{
			name:  "idle to working is silent",
			steps: []step{{"a", state.KindIdle}, {"a", state.KindWorking}},
			want:  nil,
		},
		{
			name: "two slugs independent",
			steps: []step{
				{"a", state.KindWorking}, {"a", state.KindReady},
				{"b", state.KindWorking}, {"b", state.KindWaitingInput},
			},
			want: []Recording{
				{Title: "ssf · a", Body: "Ready"},
				{Title: "ssf · b", Body: "Needs input"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &RecordingNotifier{}
			d := NewDispatcher(rec, nil)
			feed(d, tc.steps...)
			got := rec.Snapshot()
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestDispatcherUsesNameFunc(t *testing.T) {
	rec := &RecordingNotifier{}
	d := NewDispatcher(rec, func(slug string) string { return "s/datalake [main]" })
	feed(d,
		step{"abc", state.KindWorking},
		step{"abc", state.KindReady},
	)
	calls := rec.Snapshot()
	if len(calls) != 1 || calls[0].Title != "ssf · s/datalake [main]" {
		t.Fatalf("got %v", calls)
	}
}
