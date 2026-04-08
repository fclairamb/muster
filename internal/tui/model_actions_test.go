package tui

import (
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fclairamb/muster/internal/session"
	"github.com/fclairamb/muster/internal/state"
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

// drainCmd executes a tea.Cmd to completion (single message, no batches)
// and feeds its result back into the model. Used by tests that need to
// observe the side effects of asynchronous git operations.
func drainCmd(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	msg := cmd()
	if msg == nil {
		return m
	}
	next, _ := m.Update(msg)
	return next.(Model)
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
	var cmd tea.Cmd
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = drainCmd(t, m, cmd)
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

// branchPickerSetup primes a Model with the picker open and the async
// branchListMsg already delivered, so individual tests start at the
// "user is now interacting with the picker" state.
func branchPickerSetup(t *testing.T, branches string, runErr error) (Model, *FakeGit) {
	t.Helper()
	_, g, _, deps := actionDeps()
	g.RunFunc = func(dir string, args []string) (string, error) {
		// Only respond to the branch-list call; checkout calls fall through.
		if len(args) >= 4 && args[2] == "branch" {
			return branches, nil
		}
		return "", runErr
	}
	m := NewModel([]Entry{entryAt("abc", "/repo")}).WithDeps(deps)
	next, cmd := m.Update(key("b"))
	m = next.(Model)
	if m.modal != modalBranchPicker {
		t.Fatalf("modal = %v, want picker", m.modal)
	}
	m = drainCmd(t, m, cmd)
	return m, g
}

func TestBranchPickerCheckoutExisting(t *testing.T) {
	m, g := branchPickerSetup(t, "main\nfeat/x\nfeat/y\n", nil)
	g.Calls = nil // forget the list call so assertions are easier
	// type "feat/y" → only one match → enter
	for _, r := range "y" {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(Model)
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = drainCmd(t, m, cmd)
	if m.modal != modalNone {
		t.Fatalf("modal should close on success, got %v", m.modal)
	}
	calls := g.Snapshot()
	if len(calls) != 1 {
		t.Fatalf("expected 1 git call, got %v", calls)
	}
	got := calls[0]
	want := []string{"", "-C", "/repo", "checkout", "feat/y"}
	if len(got) != len(want) {
		t.Fatalf("checkout args = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("checkout args = %v, want %v", got, want)
		}
	}
}

func TestBranchPickerCreatesWhenNoMatch(t *testing.T) {
	m, g := branchPickerSetup(t, "main\n", nil)
	g.Calls = nil
	for _, r := range "feat/new" {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(Model)
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = drainCmd(t, m, cmd)
	if m.modal != modalNone {
		t.Fatalf("modal should close on success, got %v", m.modal)
	}
	calls := g.Snapshot()
	if len(calls) != 1 {
		t.Fatalf("expected 1 git call, got %v", calls)
	}
	got := calls[0]
	want := []string{"", "-C", "/repo", "checkout", "-b", "feat/new"}
	if len(got) != len(want) {
		t.Fatalf("checkout args = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("checkout args = %v, want %v", got, want)
		}
	}
}

func TestBranchPickerCheckoutErrorKeepsModalOpen(t *testing.T) {
	_, g, _, deps := actionDeps()
	checkoutErr := errors.New("dirty tree")
	g.RunFunc = func(dir string, args []string) (string, error) {
		if len(args) >= 4 && args[2] == "branch" {
			return "main\nfeat/x\n", nil
		}
		return "", checkoutErr
	}
	m := NewModel([]Entry{entryAt("abc", "/repo")}).WithDeps(deps)
	next, cmd := m.Update(key("b"))
	m = next.(Model)
	m = drainCmd(t, m, cmd)
	// pick "main" (cursor at 0) and press enter
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = drainCmd(t, m, cmd)
	if m.modal != modalBranchPicker {
		t.Fatalf("modal should stay open on checkout error, got %v", m.modal)
	}
	if m.modalErr != "dirty tree" {
		t.Fatalf("modalErr = %q, want %q", m.modalErr, "dirty tree")
	}
}

func TestBranchPickerEscCancels(t *testing.T) {
	m, g := branchPickerSetup(t, "main\nfeat/x\n", nil)
	g.Calls = nil
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.modal != modalNone {
		t.Fatalf("modal should close on esc, got %v", m.modal)
	}
	if len(g.Snapshot()) != 0 {
		t.Fatalf("no git calls expected after esc, got %v", g.Snapshot())
	}
}

func TestBranchPickerFilterNarrows(t *testing.T) {
	m, _ := branchPickerSetup(t, "main\nfeat/x\nfeat/y\nbugfix\n", nil)
	for _, r := range "feat" {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(Model)
	}
	got := m.filteredBranches()
	if len(got) != 2 || got[0] != "feat/x" || got[1] != "feat/y" {
		t.Fatalf("filteredBranches = %v", got)
	}
}

func TestInflightTracksGitOps(t *testing.T) {
	_, _, _, deps := actionDeps()
	m := NewModel([]Entry{entryAt("abc", "/repo")}).WithDeps(deps)
	// Pressing 'p' kicks off a `git push` long op.
	next, cmd := m.Update(key("p"))
	m = next.(Model)
	if len(m.opOrder) != 1 {
		t.Fatalf("expected 1 in-flight op after key press, got %d", len(m.opOrder))
	}
	if got := m.inflight[m.opOrder[0]]; got != "git push" {
		t.Fatalf("inflight label = %q, want %q", got, "git push")
	}
	if cmd == nil {
		t.Fatal("expected a tea.Cmd to be returned")
	}
	// Drain: the wrapped opMsg arrives, op is cleared, inner gitDoneMsg dispatched.
	m = drainCmd(t, m, cmd)
	if len(m.opOrder) != 0 {
		t.Fatalf("expected in-flight to clear after completion, got %v", m.inflight)
	}
}

func TestInflightShowsBranchListAndCheckout(t *testing.T) {
	_, g, _, deps := actionDeps()
	g.RunFunc = func(dir string, args []string) (string, error) {
		if len(args) >= 4 && args[2] == "branch" {
			return "main\n", nil
		}
		return "", nil
	}
	m := NewModel([]Entry{entryAt("abc", "/repo")}).WithDeps(deps)
	next, cmd := m.Update(key("b"))
	m = next.(Model)
	if len(m.opOrder) != 1 || m.inflight[m.opOrder[0]] != "loading branches" {
		t.Fatalf("expected loading-branches op, got %v", m.inflight)
	}
	m = drainCmd(t, m, cmd)
	if len(m.opOrder) != 0 {
		t.Fatalf("expected op to clear after branchListMsg, got %v", m.inflight)
	}
	// Now checkout main.
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if len(m.opOrder) != 1 || m.inflight[m.opOrder[0]] != "git checkout main" {
		t.Fatalf("expected checkout op, got %v", m.inflight)
	}
	m = drainCmd(t, m, cmd)
	if len(m.opOrder) != 0 {
		t.Fatalf("expected op to clear after checkout, got %v", m.inflight)
	}
}

func TestRemoveUnregistersRegisteredDir(t *testing.T) {
	sm, g, _, deps := actionDeps()
	var unregistered []string
	deps.Unregister = func(path string) error {
		unregistered = append(unregistered, path)
		return nil
	}
	m := NewModel([]Entry{entryAt("abc", "/repo")}).WithDeps(deps)
	next, _ := m.Update(key("r"))
	m = next.(Model)
	next, _ = m.Update(key("y"))
	m = next.(Model)
	if len(unregistered) != 1 || unregistered[0] != "/repo" {
		t.Fatalf("expected Unregister(/repo), got %v", unregistered)
	}
	if len(m.Filtered()) != 0 {
		t.Fatalf("entry not removed from view: %v", m.Filtered())
	}
	// No git or session calls should have happened.
	if len(g.Snapshot()) != 0 {
		t.Fatalf("git unexpectedly called: %v", g.Snapshot())
	}
	if list, _ := sm.List(); len(list) != 0 {
		t.Fatalf("session unexpectedly touched: %v", list)
	}
}

func TestRemoveCleanWorktree(t *testing.T) {
	sm, g, _, deps := actionDeps()
	_ = sm.Start("abc", "/wt")
	wt := entryAt("abc", "/wt")
	wt.IsWorktree = true
	m := NewModel([]Entry{wt}).WithDeps(deps)
	next, _ := m.Update(key("r"))
	m = next.(Model)
	next, cmd := m.Update(key("y"))
	m = next.(Model)
	m = drainCmd(t, m, cmd)
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
	wt := entryAt("abc", "/wt")
	wt.IsWorktree = true
	m := NewModel([]Entry{wt}).WithDeps(deps)
	next, _ := m.Update(key("r"))
	m = next.(Model)
	next, cmd := m.Update(key("y"))
	m = next.(Model)
	m = drainCmd(t, m, cmd)
	// Modal should still be open with an error.
	if m.modal != modalConfirmRemove {
		t.Fatal("modal should remain open on dirty refusal")
	}
	// Only the IsDirty check should have run; no destructive Run calls.
	if len(g.Snapshot()) != 0 {
		t.Fatal("git Run should not have run on dirty refusal")
	}
	// Press f to force, then y.
	next, _ = m.Update(key("f"))
	m = next.(Model)
	next, cmd = m.Update(key("y"))
	m = next.(Model)
	m = drainCmd(t, m, cmd)
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
