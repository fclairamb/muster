package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fclairamb/ssf/internal/session"
	"github.com/fclairamb/ssf/internal/state"
)

func actionDeps() (*session.FakeManager, *FakeGit, *FakeOpener, Deps) {
	sm := session.NewFake()
	g := &FakeGit{}
	op := &FakeOpener{}
	return sm, g, op, Deps{
		Session:     sm,
		Git:         g,
		Opener:      op,
		FileManager: "open",
		Editor:      "zed",
	}
}

func entryAt(slug, path string) Entry {
	return Entry{Display: slug, Path: path, Slug: slug, LastOpen: time.Now(), Kind: state.KindNone}
}

func key(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestEnterStartsAndAttaches(t *testing.T) {
	sm, _, _, deps := actionDeps()
	m := NewModel([]Entry{entryAt("abc", "/repo")}).WithDeps(deps)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if !sm.Has("abc") {
		t.Fatal("session not started")
	}
	if got := sm.Attached(); len(got) != 1 || got[0] != "abc" {
		t.Fatalf("attached = %v", got)
	}
	if m.PendingAttach() != "abc" {
		t.Fatalf("pendingAttach = %q", m.PendingAttach())
	}
}

func TestEnterReusesExistingSession(t *testing.T) {
	sm, _, _, deps := actionDeps()
	_ = sm.Start("abc", "/repo")
	m := NewModel([]Entry{entryAt("abc", "/repo")}).WithDeps(deps)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_ = next
	if list, _ := sm.List(); len(list) != 1 {
		t.Fatalf("expected 1 session, got %v", list)
	}
}

func TestOpenSpawnsFileManager(t *testing.T) {
	_, _, op, deps := actionDeps()
	m := NewModel([]Entry{entryAt("abc", "/repo")}).WithDeps(deps)
	next, _ := m.Update(key("o"))
	_ = next
	calls := op.Snapshot()
	if len(calls) != 1 || calls[0].Binary != "open" || calls[0].Path != "/repo" {
		t.Fatalf("got %v", calls)
	}
}

func TestEditSpawnsEditor(t *testing.T) {
	_, _, op, deps := actionDeps()
	m := NewModel([]Entry{entryAt("abc", "/repo")}).WithDeps(deps)
	next, _ := m.Update(key("e"))
	_ = next
	calls := op.Snapshot()
	if len(calls) != 1 || calls[0].Binary != "zed" {
		t.Fatalf("got %v", calls)
	}
}

func TestNewWorktreeFlow(t *testing.T) {
	_, g, _, deps := actionDeps()
	m := NewModel([]Entry{entryAt("abc", "/repo")}).WithDeps(deps)
	next, _ := m.Update(key("n"))
	m = next.(Model)
	for _, r := range "feat/x" {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(Model)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	calls := g.Snapshot()
	if len(calls) != 1 {
		t.Fatalf("expected 1 git call, got %v", calls)
	}
	// First arg is dir (""), then -C /repo worktree add ...
	if calls[0][1] != "-C" || calls[0][2] != "/repo" || calls[0][3] != "worktree" {
		t.Fatalf("git args = %v", calls[0])
	}
}

func TestNewWorktreeRejectsBadBranch(t *testing.T) {
	_, g, _, deps := actionDeps()
	m := NewModel([]Entry{entryAt("abc", "/repo")}).WithDeps(deps)
	next, _ := m.Update(key("n"))
	m = next.(Model)
	for _, r := range "bad name" {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(Model)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.modal != modalBranchPrompt {
		t.Fatal("modal should remain open after invalid input")
	}
	if len(g.Snapshot()) != 0 {
		t.Fatal("git should not have been called")
	}
}

func TestRemoveCleanWorktree(t *testing.T) {
	sm, g, _, deps := actionDeps()
	_ = sm.Start("abc", "/wt")
	m := NewModel([]Entry{entryAt("abc", "/wt")}).WithDeps(deps)
	next, _ := m.Update(key("r"))
	m = next.(Model)
	next, _ = m.Update(key("y"))
	m = next.(Model)
	if sm.Has("abc") {
		t.Fatal("session not killed")
	}
	calls := g.Snapshot()
	if len(calls) != 1 || calls[0][1] != "worktree" || calls[0][2] != "remove" {
		t.Fatalf("git not called: %v", calls)
	}
	if len(m.Filtered()) != 0 {
		t.Fatalf("entry not removed from view: %v", m.Filtered())
	}
}

func TestRemoveDirtyRequiresForce(t *testing.T) {
	sm, g, _, deps := actionDeps()
	g.Dirty = true
	_ = sm.Start("abc", "/wt")
	m := NewModel([]Entry{entryAt("abc", "/wt")}).WithDeps(deps)
	next, _ := m.Update(key("r"))
	m = next.(Model)
	next, _ = m.Update(key("y"))
	m = next.(Model)
	// Modal should still be open with an error.
	if m.modal != modalConfirmRemove {
		t.Fatal("modal should remain open on dirty refusal")
	}
	if len(g.Snapshot()) != 0 {
		t.Fatal("git should not have run on dirty refusal")
	}
	// Press f to force, then y.
	next, _ = m.Update(key("f"))
	m = next.(Model)
	next, _ = m.Update(key("y"))
	m = next.(Model)
	calls := g.Snapshot()
	if len(calls) != 1 {
		t.Fatalf("expected 1 git call, got %v", calls)
	}
	// args should contain --force
	found := false
	for _, a := range calls[0] {
		if a == "--force" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected --force in %v", calls[0])
	}
}
