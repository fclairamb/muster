package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fclairamb/ssf/internal/state"
)

func sample() []Entry {
	now := time.Now()
	return []Entry{
		{Display: "f/foo [main]", Kind: state.KindNone, LastOpen: now.Add(-3 * time.Hour)},
		{Display: "s/datalake [main]", Kind: state.KindReady, LastOpen: now.Add(-2 * time.Hour)},
		{Display: "s/api [feat/x]", Kind: state.KindWaitingInput, LastOpen: now.Add(-time.Hour)},
		{Display: "s/web [main]", Kind: state.KindWorking, LastOpen: now.Add(-time.Minute)},
		{Display: "s/idle [main]", Kind: state.KindIdle, LastOpen: now.Add(-30 * time.Second)},
	}
}

func TestSortOrder(t *testing.T) {
	got := SortEntries(sample())
	wantOrder := []state.Kind{
		state.KindWaitingInput,
		state.KindReady,
		state.KindWorking,
		state.KindIdle,
		state.KindNone,
	}
	for i, e := range got {
		if e.Kind != wantOrder[i] {
			t.Errorf("position %d: got %q want %q", i, e.Kind, wantOrder[i])
		}
	}
}

func TestSortNoneByLastOpenedDesc(t *testing.T) {
	now := time.Now()
	in := []Entry{
		{Display: "older", Kind: state.KindNone, LastOpen: now.Add(-2 * time.Hour)},
		{Display: "newer", Kind: state.KindNone, LastOpen: now.Add(-1 * time.Hour)},
	}
	got := SortEntries(in)
	if got[0].Display != "newer" {
		t.Fatalf("expected newer first, got %q", got[0].Display)
	}
}

func TestFilterMatchesDisplay(t *testing.T) {
	got := FilterEntries(sample(), "DAT")
	if len(got) != 1 || !strings.Contains(got[0].Display, "datalake") {
		t.Fatalf("filter failed: %+v", got)
	}
}

func TestFilterIncludesChildrenOfMatchingParent(t *testing.T) {
	in := []Entry{
		{Display: "s/repo [main]", Indent: 0, Kind: state.KindNone},
		{Display: "[feat/x]", Indent: 1, Kind: state.KindNone},
		{Display: "f/other [main]", Indent: 0, Kind: state.KindNone},
	}
	got := FilterEntries(in, "repo")
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}
}

func TestModelCursorMovement(t *testing.T) {
	m := NewModel(sample())
	for i := 0; i < 3; i++ {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = next.(Model)
	}
	if m.Cursor() != 3 {
		t.Fatalf("cursor = %d", m.Cursor())
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = next.(Model)
	if m.Cursor() != 2 {
		t.Fatalf("cursor after up = %d", m.Cursor())
	}
}

func TestModelSearchFlow(t *testing.T) {
	m := NewModel(sample())
	// Enter search mode.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = next.(Model)
	for _, r := range "datalake" {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(Model)
	}
	if !m.searching {
		t.Fatal("expected searching=true")
	}
	if got := m.Filtered(); len(got) != 1 || !strings.Contains(got[0].Display, "datalake") {
		t.Fatalf("filtered = %v", got)
	}
	// Esc clears.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.searching || m.search != "" {
		t.Fatalf("esc did not reset")
	}
	if len(m.Filtered()) != 5 {
		t.Fatalf("filter not cleared, got %d", len(m.Filtered()))
	}
}

func TestModelQuit(t *testing.T) {
	m := NewModel(sample())
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = next.(Model)
	if !m.Quitting() {
		t.Fatal("expected quitting")
	}
	if cmd == nil {
		t.Fatal("expected tea.Quit cmd")
	}
}

func TestViewRendersEmoji(t *testing.T) {
	m := NewModel(sample())
	v := m.View()
	if !strings.Contains(v, "🔴") || !strings.Contains(v, "🟢") || !strings.Contains(v, "🟡") {
		t.Fatalf("view missing emoji:\n%s", v)
	}
	// First row should be the red one (sorted to top).
	red := strings.Index(v, "🔴")
	green := strings.Index(v, "🟢")
	if red < 0 || green < 0 || red > green {
		t.Fatalf("red should appear before green:\n%s", v)
	}
}

func TestViewIndentNestsChildren(t *testing.T) {
	in := []Entry{
		{Display: "s/repo [main]", Indent: 0, Kind: state.KindNone, LastOpen: time.Now()},
		{Display: "[feat/x]", Indent: 1, Kind: state.KindNone, LastOpen: time.Now()},
	}
	m := NewModel(in)
	v := m.View()
	if !strings.Contains(v, "  [feat/x]") {
		t.Fatalf("child not indented:\n%s", v)
	}
}
