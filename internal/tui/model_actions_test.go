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
	next, _ := m.Update(key("w"))
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
	// After success, a new indented worktree entry must be inserted
	// directly under the parent so the user sees it without restarting.
	if len(m.entries) != 2 {
		t.Fatalf("expected 2 entries (parent + new worktree), got %d: %+v", len(m.entries), m.entries)
	}
	parent, child := m.entries[0], m.entries[1]
	if parent.Slug != "abc" || parent.Indent != 0 {
		t.Fatalf("parent shifted: %+v", parent)
	}
	if child.Indent != 1 || !child.IsWorktree {
		t.Fatalf("child not nested or not marked as worktree: %+v", child)
	}
	if child.Display != "[feat/x]" {
		t.Fatalf("child display = %q, want %q", child.Display, "[feat/x]")
	}
	wantPath := "/repo/.muster/worktrees/repo-feat-x"
	if child.Path != wantPath {
		t.Fatalf("child path = %q, want %q", child.Path, wantPath)
	}
}

func TestNewWorktreeRejectsBadBranch(t *testing.T) {
	_, g, _, deps := actionDeps()
	m := NewModel([]Entry{entryAt("abc", "/repo")}).WithDeps(deps)
	next, _ := m.Update(key("w"))
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

func TestSetDisplayBranch(t *testing.T) {
	cases := []struct{ in, branch, want string }{
		{"f/muster [main]", "feat/x", "f/muster [feat/x]"},
		{"f/muster apps/api [main]", "feat/x", "f/muster apps/api [feat/x]"},
		{"local-dir", "feat/x", "local-dir"}, // no bracket → unchanged
		{"weird]", "feat/x", "weird]"},        // no " [" prefix → unchanged
	}
	for _, c := range cases {
		got := setDisplayBranch(c.in, c.branch)
		if got != c.want {
			t.Errorf("setDisplayBranch(%q,%q) = %q, want %q", c.in, c.branch, got, c.want)
		}
	}
}

func TestBranchPickerCheckoutUpdatesDisplay(t *testing.T) {
	_, g, _, deps := actionDeps()
	g.RunFunc = func(dir string, args []string) (string, error) {
		if len(args) >= 4 && args[2] == "branch" {
			return "main\nfeat/y\n", nil
		}
		return "", nil
	}
	e := entryAt("abc", "/repo")
	e.Display = "f/muster [main]"
	m := NewModel([]Entry{e}).WithDeps(deps)
	// Open picker, drain list load.
	next, cmd := m.Update(key("b"))
	m = next.(Model)
	m = drainCmd(t, m, cmd)
	// Move cursor to feat/y and enter.
	for _, r := range "feat/y" {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(Model)
	}
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = drainCmd(t, m, cmd)
	if m.entries[0].Display != "f/muster [feat/y]" {
		t.Fatalf("entry display not updated: %q", m.entries[0].Display)
	}
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

func TestShellStartsAndAttachesWithSuffixedSlug(t *testing.T) {
	sm, _, _, deps := actionDeps()
	m := NewModel([]Entry{entryAt("abc", "/repo")}).WithDeps(deps)
	next, _ := m.Update(key("s"))
	m = next.(Model)
	if !sm.Has("abc-sh") {
		t.Fatal("shell session not started under suffixed slug")
	}
	if sm.Has("abc") {
		t.Fatal("claude session unexpectedly started by shell action")
	}
	got := sm.Attached()
	if len(got) != 1 || got[0] != "abc-sh" {
		t.Fatalf("attached = %v, want [abc-sh]", got)
	}
	if m.PendingAttach() != "abc-sh" {
		t.Fatalf("pendingAttach = %q, want abc-sh", m.PendingAttach())
	}
	// Pressing s a second time must reuse the existing session.
	next, _ = m.Update(key("s"))
	_ = next
	list, _ := sm.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 session after second press, got %v", list)
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

func TestNewInstancePromptDefaultsToTwo(t *testing.T) {
	sm, _, _, deps := actionDeps()
	var added [][2]string
	deps.AddInstance = func(parent, instance string) (string, error) {
		added = append(added, [2]string{parent, instance})
		return "abc-" + instance, nil
	}
	deps.Session = sm
	m := NewModel([]Entry{entryAt("abc", "/repo")}).WithDeps(deps)
	// Open the instance modal — pre-filled with "2".
	next, _ := m.Update(key("n"))
	m = next.(Model)
	if m.modal != modalInstancePrompt {
		t.Fatalf("modal = %v, want modalInstancePrompt", m.modal)
	}
	if m.modalInput != "2" {
		t.Fatalf("default name = %q, want %q", m.modalInput, "2")
	}
	// Accept the default.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if len(added) != 1 || added[0] != [2]string{"/repo", "2"} {
		t.Fatalf("AddInstance call = %v, want [/repo 2]", added)
	}
	if len(m.entries) != 2 {
		t.Fatalf("expected 2 entries (parent + #2), got %d: %+v", len(m.entries), m.entries)
	}
	parent, child := m.entries[0], m.entries[1]
	if parent.Slug != "abc" || parent.Indent != 0 {
		t.Fatalf("parent shifted: %+v", parent)
	}
	if !child.IsInstance || child.Indent != 1 || child.Slug != "abc-2" || child.Instance != "2" {
		t.Fatalf("child malformed: %+v", child)
	}
	if child.Display != "#2" {
		t.Fatalf("child display = %q", child.Display)
	}

	// Pressing n again pre-fills "3" because "2" is now taken.
	next, _ = m.Update(key("n"))
	m = next.(Model)
	if m.modalInput != "3" {
		t.Fatalf("second default name = %q, want %q", m.modalInput, "3")
	}
}

func TestNewInstanceRejectsBadName(t *testing.T) {
	_, _, _, deps := actionDeps()
	deps.AddInstance = func(parent, instance string) (string, error) {
		t.Fatalf("AddInstance must not be called: parent=%s instance=%s", parent, instance)
		return "", nil
	}
	m := NewModel([]Entry{entryAt("abc", "/repo")}).WithDeps(deps)
	next, _ := m.Update(key("n"))
	m = next.(Model)
	// Replace the default with garbage.
	for range "23" {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
		m = next.(Model)
	}
	for _, r := range "BAD NAME" {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = next.(Model)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.modal != modalInstancePrompt {
		t.Fatal("modal should remain open after invalid input")
	}
	if m.modalErr == "" {
		t.Fatal("modalErr should be populated")
	}
}

func TestRemoveInstanceKillsSessionAndUnregisters(t *testing.T) {
	sm, _, _, deps := actionDeps()
	_ = sm.Start("abc-2", "/repo")
	var unregistered [][2]string
	deps.Unregister = func(path, instance string) error {
		unregistered = append(unregistered, [2]string{path, instance})
		return nil
	}
	deps.Session = sm
	parent := entryAt("abc", "/repo")
	child := Entry{
		Display:    "#2",
		Indent:     1,
		Path:       "/repo",
		Slug:       "abc-2",
		Instance:   "2",
		IsInstance: true,
		LastOpen:   time.Now(),
		Kind:       state.KindIdle,
	}
	m := NewModel([]Entry{parent, child}).WithDeps(deps)
	// Move cursor to the instance row.
	m.cursor = 1
	next, _ := m.Update(key("r"))
	m = next.(Model)
	next, _ = m.Update(key("y"))
	m = next.(Model)
	if len(unregistered) != 1 || unregistered[0] != [2]string{"/repo", "2"} {
		t.Fatalf("Unregister calls = %v", unregistered)
	}
	if sm.Has("abc-2") {
		t.Fatal("instance session should be killed on remove")
	}
	// Parent must remain.
	if len(m.entries) != 1 || m.entries[0].Slug != "abc" {
		t.Fatalf("entries after remove = %+v", m.entries)
	}
}

func TestRemoveUnregistersRegisteredDir(t *testing.T) {
	sm, g, _, deps := actionDeps()
	var unregistered []string
	deps.Unregister = func(path, instance string) error {
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
