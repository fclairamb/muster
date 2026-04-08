// Package tui implements the Bubble Tea TUI for muster.
package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fclairamb/muster/internal/gitstats"
	"github.com/fclairamb/muster/internal/state"
)

// gitDoneMsg is the result of an asynchronous Git.Run invocation.
// All git work happens in a tea.Cmd goroutine so the TUI never blocks.
type gitDoneMsg struct{ err error }

// statsMsg carries an updated gitstats.Stats for one entry, computed in
// a goroutine off the TUI main loop.
type statsMsg struct {
	slug  string
	stats gitstats.Stats
}

// branchListMsg carries the result of an asynchronous `git branch` invocation
// fired when the user opens the branch picker.
type branchListMsg struct {
	slug     string
	branches []string
	err      error
}

// branchCheckoutDoneMsg is the result of an asynchronous `git checkout` fired
// from the branch picker. On success, the picker is dismissed and entries
// are refreshed; on error, the picker stays open and renders the message.
type branchCheckoutDoneMsg struct {
	slug string
	err  error
}

// worktreeRemoveDoneMsg is the result of an asynchronous worktree remove
// flow (dirty check + session kill + git worktree remove).
type worktreeRemoveDoneMsg struct {
	slug         string
	path         string
	dirtyRefused bool
	err          error
}

// StatsSuffix renders the per-entry git counts shown after the display
// string in the list. Each indicator is omitted when its count is zero;
// returns "" when there's nothing to show.
func StatsSuffix(s gitstats.Stats) string {
	var b strings.Builder
	if s.Unpushed > 0 {
		fmt.Fprintf(&b, " +%d", s.Unpushed)
	}
	if s.Behind > 0 {
		fmt.Fprintf(&b, " -%d", s.Behind)
	}
	if s.Modified > 0 {
		fmt.Fprintf(&b, " M%d", s.Modified)
	}
	if s.Untracked > 0 {
		fmt.Fprintf(&b, " ?%d", s.Untracked)
	}
	return b.String()
}

// modalKind is the kind of overlay currently shown over the list, if any.
type modalKind int

const (
	modalNone modalKind = iota
	modalBranchPrompt
	modalConfirmRemove
	modalBranchPicker
)

// Model holds the TUI state.
type Model struct {
	deps Deps

	entries  []Entry
	filtered []Entry
	cursor   int

	searching bool
	search    string

	modal      modalKind
	modalInput string
	modalForce bool
	modalErr   string

	// Branch picker state. Active when modal == modalBranchPicker.
	branchSlug    string   // entry the picker was opened on
	branchRepo    string   // repo path passed to git
	branches      []string // local branches loaded async
	branchFilter  string   // typed filter
	branchCursor  int      // index into the filtered view
	branchLoading bool     // true until branchListMsg arrives

	quitting bool

	// autoAttachSlug, if non-empty, makes Init emit an AttachMsg that
	// attaches to that slug as if the user had pressed Enter on it.
	autoAttachSlug string

	// Recording fields used by tests/main to learn what the model wants to do.
	pendingAttach string // slug to attach via tea.ExecProcess
	lastError     string

	// In-flight long-running operations, rendered at the bottom of the
	// view. Each op has a unique id and a human label. Ops are registered
	// synchronously when their tea.Cmd is dispatched and cleared when the
	// wrapped result message arrives. statsCmd refreshes are intentionally
	// not tracked — they're per-tick noise.
	nextOpID int
	inflight map[int]string
	opOrder  []int
}

// opMsg wraps the result of a tracked long-running command. Update unwraps
// it, clears the in-flight entry, then re-dispatches the inner message so
// the existing handlers stay unchanged.
type opMsg struct {
	id  int
	msg tea.Msg
}

// beginOp registers a new in-flight operation and returns its id.
func (m *Model) beginOp(label string) int {
	m.nextOpID++
	id := m.nextOpID
	if m.inflight == nil {
		m.inflight = map[int]string{}
	}
	m.inflight[id] = label
	m.opOrder = append(m.opOrder, id)
	return id
}

// endOp clears a previously-registered in-flight operation.
func (m *Model) endOp(id int) {
	delete(m.inflight, id)
	out := m.opOrder[:0]
	for _, x := range m.opOrder {
		if x != id {
			out = append(out, x)
		}
	}
	m.opOrder = out
}

// wrapOp envelopes a tea.Cmd's result so Update can clear the matching
// in-flight entry. A nil cmd is returned untouched.
func wrapOp(id int, cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg { return opMsg{id: id, msg: cmd()} }
}

// WithAutoAttach makes the next Init() emit an AttachMsg targeting slug.
// Used by `muster <dir>` to immediately drop the user into claude for the
// directory they just registered.
func (m Model) WithAutoAttach(slug string) Model {
	m.autoAttachSlug = slug
	return m
}

// NewModel constructs a Model from raw entries (will be sorted).
func NewModel(entries []Entry) Model {
	sorted := SortEntries(entries)
	return Model{entries: sorted, filtered: sorted}
}

// WithDeps returns a copy of m with collaborators wired up.
func (m Model) WithDeps(d Deps) Model {
	m.deps = d
	return m
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{tea.SetWindowTitle("muster: list")}
	if m.autoAttachSlug != "" {
		slug := m.autoAttachSlug
		cmds = append(cmds, func() tea.Msg { return AttachMsg{Slug: slug} })
	}
	return tea.Batch(cmds...)
}

// titleListView is the terminal title shown while the user is in the list.
const titleListView = "muster: list"

// titleAttached returns the terminal title shown while the user is attached
// to a session: "<display> <emoji>". The "(ssf)" tag is only shown in the
// list view title — the user explicitly does not want it duplicated here.
func titleAttached(e Entry) string {
	emoji := StatusEmoji(e.Kind)
	if e.Kind == state.KindNone {
		emoji = StatusEmoji(state.KindIdle) // we're attached → at least idle
	}
	return e.Display + " " + emoji
}

// RefreshMsg triggers a re-read of state from disk for every entry. The
// message is exported so cmd/muster can drive the refresh from a background
// goroutine via program.Send — more reliable than tea.Tick across
// ExecProcess suspensions.
type RefreshMsg struct{}

// AttachMsg is a request to attach to the claude session for a given slug,
// emitted programmatically (not from a key press). Used to auto-attach when
// muster is invoked with a directory argument.
type AttachMsg struct{ Slug string }

// attachExitedMsg is emitted by the tea.ExecProcess callback once the user
// detaches from a tmux session. It carries the slug so the post-detach
// handler can clear a sticky "ready" state back to "idle".
type attachExitedMsg struct{ slug string }

// StateMsg notifies the model that one entry's session state has changed.
// Sent from the watcher pump goroutine via program.Send.
type StateMsg struct {
	Slug string
	Kind state.Kind
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case opMsg:
		(&m).endOp(msg.id)
		if msg.msg == nil {
			return m, nil
		}
		return m.Update(msg.msg)
	case tea.KeyMsg:
		return m.handleKey(msg)
	case StateMsg:
		return m.applyStateMsg(msg), nil
	case RefreshMsg:
		m = m.applyRefresh()
		return m, m.statsCmd()
	case statsMsg:
		for i := range m.entries {
			if m.entries[i].Slug == msg.slug {
				m.entries[i].Stats = msg.stats
			}
		}
		m.applySearch()
		return m, nil
	case gitDoneMsg:
		if msg.err != nil {
			m.lastError = msg.err.Error()
		} else {
			m.lastError = ""
		}
		return m, nil
	case branchListMsg:
		// Late delivery: ignore if the picker has already been dismissed,
		// or if the user opened a different entry's picker since.
		if m.modal != modalBranchPicker || m.branchSlug != msg.slug {
			return m, nil
		}
		m.branchLoading = false
		if msg.err != nil {
			m.modalErr = msg.err.Error()
			return m, nil
		}
		m.branches = msg.branches
		m.branchCursor = 0
		return m, nil
	case branchCheckoutDoneMsg:
		if m.modal != modalBranchPicker || m.branchSlug != msg.slug {
			return m, nil
		}
		if msg.err != nil {
			m.modalErr = msg.err.Error()
			return m, nil
		}
		m.modal = modalNone
		m.branches = nil
		m.branchFilter = ""
		m.branchSlug = ""
		m.branchRepo = ""
		m = m.applyRefresh()
		return m, m.statsCmd()
	case worktreeRemoveDoneMsg:
		if msg.dirtyRefused {
			m.modalErr = "worktree is dirty; press f to force"
			return m, nil
		}
		if msg.err != nil {
			m.modalErr = msg.err.Error()
			return m, nil
		}
		m.entries = removeBySlugPath(m.entries, msg.slug, msg.path)
		m.applySearch()
		m.modal = modalNone
		return m, nil
	case AttachMsg:
		// Move the cursor to the requested entry, then fall through to
		// the same actionEnter path used by the Enter key.
		for i, e := range m.filtered {
			if e.Slug == msg.Slug {
				m.cursor = i
				return m.actionEnter()
			}
		}
		return m, nil
	case attachExitedMsg:
		// If the entry was sitting on "ready" (green) when the user
		// detached, clear it back to "idle" so the green dot doesn't
		// keep nagging after the user has already seen the result.
		if m.deps.ClearState != nil && m.deps.ReadState != nil {
			for _, e := range m.entries {
				if e.Slug != msg.slug {
					continue
				}
				root := e.RepoRoot
				if root == "" {
					root = e.Path
				}
				st := m.deps.ReadState(root, e.Slug)
				if st.Kind == state.KindReady {
					_ = m.deps.ClearState(root, e.Slug)
				}
				break
			}
		}
		return m.applyRefresh(), nil
	}
	return m, nil
}

// StaleThreshold is the age beyond which a "working" or "waiting_input"
// state is treated as idle. Without this, a state file that was written
// during a working turn but never updated by a Stop hook keeps the bubble
// stuck on yellow forever.
var StaleThreshold = 30 * time.Second

// Refresh re-reads on-disk state for every entry and reconciles it with
// the live tmux session set. Exposed publicly so main.go can call it once
// before program.Run() to ensure the very first frame is correct.
//
// Reconciliation rules:
//
//   - tmux session present + state file says X    → X
//   - tmux session present + no state file (None) → KindIdle (claude is
//     running but hasn't fired any hook yet)
//   - tmux session absent → KindNone, regardless of stale state file
func (m Model) Refresh() Model { return m.applyRefresh() }

func (m Model) applyRefresh() Model {
	if m.deps.ReadState == nil {
		return m
	}
	var running map[string]struct{}
	if m.deps.Session != nil {
		list, _ := m.deps.Session.List()
		running = make(map[string]struct{}, len(list))
		for _, s := range list {
			running[s] = struct{}{}
		}
	}
	now := time.Now()
	for i := range m.entries {
		slug := m.entries[i].Slug
		root := m.entries[i].RepoRoot
		if root == "" {
			root = m.entries[i].Path
		}
		st := m.deps.ReadState(root, slug)
		k := decayStale(st, now)
		if running != nil {
			if _, alive := running[slug]; alive {
				if k == state.KindNone {
					k = state.KindIdle
				}
			} else {
				k = state.KindNone
			}
		}
		m.entries[i].Kind = k
	}
	m.entries = SortEntries(m.entries)
	m.applySearch()
	return m
}

// decayStale collapses stale "working" / "waiting_input" states into idle.
// "ready" and "idle" don't decay because they're stable resting states.
func decayStale(st state.State, now time.Time) state.Kind {
	if st.Kind != state.KindWorking && st.Kind != state.KindWaitingInput {
		return st.Kind
	}
	if st.Ts.IsZero() {
		return state.KindIdle
	}
	if now.Sub(st.Ts) > StaleThreshold {
		return state.KindIdle
	}
	return st.Kind
}

func (m Model) applyStateMsg(msg StateMsg) Model {
	for i := range m.entries {
		if m.entries[i].Slug == msg.Slug {
			m.entries[i].Kind = msg.Kind
		}
	}
	m.entries = SortEntries(m.entries)
	m.applySearch()
	return m
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.modal {
	case modalBranchPrompt:
		return m.handleBranchPromptKey(msg)
	case modalConfirmRemove:
		return m.handleConfirmRemoveKey(msg)
	case modalBranchPicker:
		return m.handleBranchPickerKey(msg)
	}
	if m.searching {
		return m.handleSearchKey(msg)
	}
	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
		}
	case "/":
		m.searching = true
		m.search = ""
		m.applySearch()
	case "enter":
		return m.actionEnter()
	case "o":
		return m.actionOpen()
	case "e":
		return m.actionEdit()
	case "n":
		m.modal = modalBranchPrompt
		m.modalInput = ""
		m.modalErr = ""
	case "b":
		return m.actionBranchPicker()
	case "r":
		if len(m.filtered) > 0 {
			m.modal = modalConfirmRemove
			m.modalForce = false
			m.modalErr = ""
		}
	case "x":
		return m.actionKill()
	case "u":
		return m.actionGit("pull")
	case "m":
		return m.actionGit("pull", "origin", "main")
	case "p":
		return m.actionGit("push")
	}
	return m, nil
}

func (m Model) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.searching = false
		m.search = ""
		m.applySearch()
	case tea.KeyEnter:
		m.searching = false
	case tea.KeyBackspace:
		if len(m.search) > 0 {
			m.search = m.search[:len(m.search)-1]
			m.applySearch()
		}
	case tea.KeyRunes, tea.KeySpace:
		m.search += string(msg.Runes)
		m.applySearch()
	}
	return m, nil
}

func (m Model) handleBranchPromptKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.modal = modalNone
		m.modalInput = ""
		m.modalErr = ""
	case tea.KeyEnter:
		if err := ValidateBranchName(m.modalInput); err != nil {
			m.modalErr = "invalid branch name"
			return m, nil
		}
		entry := m.selectedEntry()
		if entry == nil {
			m.modal = modalNone
			return m, nil
		}
		var cmd tea.Cmd
		if m.deps.Git != nil {
			args := BuildWorktreeAddArgs(entry.Path, m.modalInput)
			id := (&m).beginOp("git worktree add " + m.modalInput)
			cmd = wrapOp(id, m.gitRunCmd("", args...))
		}
		m.modal = modalNone
		m.modalInput = ""
		return m, cmd
	case tea.KeyBackspace:
		if len(m.modalInput) > 0 {
			m.modalInput = m.modalInput[:len(m.modalInput)-1]
		}
	case tea.KeyRunes, tea.KeySpace:
		m.modalInput += string(msg.Runes)
	}
	return m, nil
}

// actionBranchPicker opens the branch picker modal for the selected entry and
// kicks off an asynchronous `git branch` to populate the list.
func (m Model) actionBranchPicker() (tea.Model, tea.Cmd) {
	entry := m.selectedEntry()
	if entry == nil || m.deps.Git == nil {
		return m, nil
	}
	m.modal = modalBranchPicker
	m.modalErr = ""
	m.branchSlug = entry.Slug
	m.branchRepo = entry.Path
	m.branches = nil
	m.branchFilter = ""
	m.branchCursor = 0
	m.branchLoading = true
	git := m.deps.Git
	slug, repo := entry.Slug, entry.Path
	cmd := func() tea.Msg {
		out, err := git.Run("", BuildBranchListArgs(repo)...)
		if err != nil {
			return branchListMsg{slug: slug, err: err}
		}
		return branchListMsg{slug: slug, branches: ParseBranchList(out)}
	}
	id := (&m).beginOp("loading branches")
	return m, wrapOp(id, cmd)
}

// filteredBranches returns branches whose name contains branchFilter
// (case-insensitive substring match). When the filter is empty, all branches
// are returned.
func (m Model) filteredBranches() []string {
	if m.branchFilter == "" {
		return m.branches
	}
	needle := strings.ToLower(m.branchFilter)
	var out []string
	for _, b := range m.branches {
		if strings.Contains(strings.ToLower(b), needle) {
			out = append(out, b)
		}
	}
	return out
}

func (m Model) handleBranchPickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.modal = modalNone
		m.branches = nil
		m.branchFilter = ""
		m.branchSlug = ""
		m.branchRepo = ""
		m.modalErr = ""
		return m, nil
	case tea.KeyUp:
		if m.branchCursor > 0 {
			m.branchCursor--
		}
		return m, nil
	case tea.KeyDown:
		if m.branchCursor < len(m.filteredBranches())-1 {
			m.branchCursor++
		}
		return m, nil
	case tea.KeyEnter:
		filtered := m.filteredBranches()
		var target string
		create := false
		switch {
		case len(filtered) > 0:
			if m.branchCursor < 0 || m.branchCursor >= len(filtered) {
				return m, nil
			}
			target = filtered[m.branchCursor]
		case m.branchFilter != "":
			if err := ValidateBranchName(m.branchFilter); err != nil {
				m.modalErr = "invalid branch name"
				return m, nil
			}
			target = m.branchFilter
			create = true
		default:
			return m, nil
		}
		git := m.deps.Git
		if git == nil {
			return m, nil
		}
		slug, repo := m.branchSlug, m.branchRepo
		args := BuildCheckoutArgs(repo, target, create)
		m.modalErr = ""
		cmd := func() tea.Msg {
			_, err := git.Run("", args...)
			return branchCheckoutDoneMsg{slug: slug, err: err}
		}
		label := "git checkout " + target
		if create {
			label = "git checkout -b " + target
		}
		id := (&m).beginOp(label)
		return m, wrapOp(id, cmd)
	case tea.KeyBackspace:
		if len(m.branchFilter) > 0 {
			m.branchFilter = m.branchFilter[:len(m.branchFilter)-1]
			m.branchCursor = 0
			m.modalErr = ""
		}
		return m, nil
	case tea.KeyRunes, tea.KeySpace:
		m.branchFilter += string(msg.Runes)
		m.branchCursor = 0
		m.modalErr = ""
		return m, nil
	}
	return m, nil
}

func (m Model) handleConfirmRemoveKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "n":
		m.modal = modalNone
	case "f":
		m.modalForce = !m.modalForce
	case "y", "enter":
		entry := m.selectedEntry()
		if entry == nil {
			m.modal = modalNone
			return m, nil
		}
		if entry.IsWorktree {
			// Worktree path: dirty check + kill session + git worktree
			// remove all happen in a goroutine. The modal stays open and
			// shows "removing..." until worktreeRemoveDoneMsg arrives.
			id := (&m).beginOp("git worktree remove " + entry.Path)
			return m, wrapOp(id, m.removeWorktreeCmd(entry.Slug, entry.Path, m.modalForce))
		}
		// Registered dir: unregister is not git work, run inline.
		if m.deps.Unregister != nil {
			if err := m.deps.Unregister(entry.Path); err != nil {
				m.modalErr = err.Error()
				return m, nil
			}
		}
		m.entries = removeEntry(m.entries, *entry)
		m.applySearch()
		m.modal = modalNone
	}
	return m, nil
}

func (m Model) actionEnter() (tea.Model, tea.Cmd) {
	entry := m.selectedEntry()
	if entry == nil || m.deps.Session == nil {
		return m, nil
	}
	if !m.deps.Session.Has(entry.Slug) {
		if err := m.deps.Session.Start(entry.Slug, entry.Path); err != nil {
			m.lastError = err.Error()
			return m, nil
		}
	}
	m.pendingAttach = entry.Slug
	// Prefer tea.ExecProcess so the TUI suspends, runs tmux attach in the
	// foreground, and resumes when the user detaches.
	if m.deps.AttachCmdFunc != nil {
		cmd := m.deps.AttachCmdFunc(entry.Slug)
		if cmd != nil {
			slug := entry.Slug
			return m, tea.Sequence(
				tea.SetWindowTitle(titleAttached(*entry)),
				tea.ExecProcess(cmd, func(error) tea.Msg {
					return attachExitedMsg{slug: slug}
				}),
				tea.SetWindowTitle(titleListView),
			)
		}
	}
	if err := m.deps.Session.Attach(entry.Slug); err != nil {
		m.lastError = err.Error()
	}
	return m, nil
}

// actionKill stops the claude tmux session for the selected entry without
// removing it from the registry. The entry stays in the list and reverts to
// KindNone (no session) on the next refresh tick.
func (m Model) actionKill() (tea.Model, tea.Cmd) {
	entry := m.selectedEntry()
	if entry == nil || m.deps.Session == nil {
		return m, nil
	}
	if !m.deps.Session.Has(entry.Slug) {
		return m, nil
	}
	if err := m.deps.Session.Kill(entry.Slug); err != nil {
		m.lastError = err.Error()
		return m, nil
	}
	if m.deps.ClearState != nil {
		root := entry.RepoRoot
		if root == "" {
			root = entry.Path
		}
		_ = m.deps.ClearState(root, entry.Slug)
	}
	return m.applyRefresh(), nil
}

// actionGit runs a git command in the selected entry's directory and surfaces
// any error in the footer. Used by the u/m/p key bindings (pull, pull from
// main, push).
func (m Model) actionGit(args ...string) (tea.Model, tea.Cmd) {
	entry := m.selectedEntry()
	if entry == nil || m.deps.Git == nil {
		return m, nil
	}
	id := (&m).beginOp("git " + strings.Join(args, " "))
	return m, wrapOp(id, m.gitRunCmd(entry.Path, args...))
}

// gitRunCmd builds a tea.Cmd that runs Git.Run in a goroutine and reports
// the result via gitDoneMsg. This is the single funnel through which the
// model launches non-blocking git invocations.
func (m Model) gitRunCmd(dir string, args ...string) tea.Cmd {
	git := m.deps.Git
	if git == nil {
		return nil
	}
	argv := append([]string(nil), args...)
	return func() tea.Msg {
		_, err := git.Run(dir, argv...)
		return gitDoneMsg{err: err}
	}
}

// removeWorktreeCmd performs the dirty check, session kill, and worktree
// remove sequence in a goroutine. The TUI gets a single
// worktreeRemoveDoneMsg back when it finishes.
func (m Model) removeWorktreeCmd(slug, path string, force bool) tea.Cmd {
	git := m.deps.Git
	sess := m.deps.Session
	return func() tea.Msg {
		if git != nil {
			dirty, _ := git.IsDirty(path)
			if dirty && !force {
				return worktreeRemoveDoneMsg{slug: slug, path: path, dirtyRefused: true}
			}
		}
		if sess != nil {
			_ = sess.Kill(slug)
		}
		if git != nil {
			args := BuildWorktreeRemoveArgs(path, force)
			if _, err := git.Run("", args...); err != nil {
				return worktreeRemoveDoneMsg{slug: slug, path: path, err: err}
			}
		}
		return worktreeRemoveDoneMsg{slug: slug, path: path}
	}
}

// statsCmd builds a tea.Cmd batch that recomputes git stats for every
// entry off the TUI main loop. Each entry's stats arrive as a separate
// statsMsg so slow repos don't hold back fast ones.
func (m Model) statsCmd() tea.Cmd {
	if m.deps.GitStats == nil {
		return nil
	}
	fn := m.deps.GitStats
	cmds := make([]tea.Cmd, 0, len(m.entries))
	for _, e := range m.entries {
		slug, path := e.Slug, e.Path
		cmds = append(cmds, func() tea.Msg {
			return statsMsg{slug: slug, stats: fn(path)}
		})
	}
	return tea.Batch(cmds...)
}

// removeBySlugPath returns entries with any element matching slug+path removed.
func removeBySlugPath(entries []Entry, slug, path string) []Entry {
	out := entries[:0]
	for _, e := range entries {
		if e.Slug == slug && e.Path == path {
			continue
		}
		out = append(out, e)
	}
	dup := make([]Entry, len(out))
	copy(dup, out)
	return dup
}

func (m Model) actionOpen() (tea.Model, tea.Cmd) {
	entry := m.selectedEntry()
	if entry == nil || m.deps.Opener == nil {
		return m, nil
	}
	bin := m.deps.FileManager
	if bin == "" {
		bin = "open"
	}
	_ = m.deps.Opener.Open(bin, entry.Path)
	return m, nil
}

func (m Model) actionEdit() (tea.Model, tea.Cmd) {
	entry := m.selectedEntry()
	if entry == nil || m.deps.Opener == nil {
		return m, nil
	}
	bin := m.deps.Editor
	if bin == "" {
		bin = "zed"
	}
	_ = m.deps.Opener.Open(bin, entry.Path)
	return m, nil
}

func (m *Model) applySearch() {
	m.filtered = FilterEntries(m.entries, m.search)
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m Model) selectedEntry() *Entry {
	if m.cursor < 0 || m.cursor >= len(m.filtered) {
		return nil
	}
	e := m.filtered[m.cursor]
	return &e
}

func removeEntry(entries []Entry, target Entry) []Entry {
	out := entries[:0]
	for _, e := range entries {
		if e.Slug == target.Slug && e.Path == target.Path {
			continue
		}
		out = append(out, e)
	}
	dup := make([]Entry, len(out))
	copy(dup, out)
	return dup
}

// View implements tea.Model.
func (m Model) View() string {
	if m.quitting {
		return ""
	}
	var b strings.Builder
	b.WriteString("muster\n\n")
	for i, e := range m.filtered {
		selected := i == m.cursor
		cursor := "  "
		if selected {
			cursor = "> "
		}
		if selected {
			b.WriteString("\x1b[7m")
		}
		b.WriteString(cursor)
		for j := 0; j < e.Indent; j++ {
			b.WriteString("  ")
		}
		b.WriteString(StatusEmoji(e.Kind))
		b.WriteString(" ")
		b.WriteString(e.Display)
		b.WriteString(StatsSuffix(e.Stats))
		if selected {
			b.WriteString("\x1b[0m")
		}
		b.WriteString("\n")
	}
	if len(m.opOrder) > 0 {
		b.WriteString("\n⏳ ")
		for i, id := range m.opOrder {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(m.inflight[id])
		}
	}
	switch {
	case m.modal == modalBranchPrompt:
		b.WriteString("\nNew worktree branch: ")
		b.WriteString(m.modalInput)
		if m.modalErr != "" {
			b.WriteString("\n  ⚠ ")
			b.WriteString(m.modalErr)
		}
	case m.modal == modalBranchPicker:
		b.WriteString("\nCheckout branch: ")
		b.WriteString(m.branchFilter)
		b.WriteString("\n")
		switch {
		case m.branchLoading:
			b.WriteString("  loading branches...\n")
		case len(m.branches) == 0:
			b.WriteString("  (no branches)\n")
		default:
			filtered := m.filteredBranches()
			if len(filtered) == 0 {
				if ValidateBranchName(m.branchFilter) == nil {
					b.WriteString("  ↵ create new branch \"")
					b.WriteString(m.branchFilter)
					b.WriteString("\"\n")
				} else {
					b.WriteString("  (no match)\n")
				}
			} else {
				for i, name := range filtered {
					if i == m.branchCursor {
						b.WriteString("> ")
					} else {
						b.WriteString("  ")
					}
					b.WriteString(name)
					b.WriteString("\n")
				}
			}
		}
		if m.modalErr != "" {
			b.WriteString("  ⚠ ")
			b.WriteString(m.modalErr)
			b.WriteString("\n")
		}
		b.WriteString("  ↑↓ move  ↵ checkout/create  esc cancel")
	case m.modal == modalConfirmRemove:
		entry := m.selectedEntry()
		if entry != nil && entry.IsWorktree {
			b.WriteString("\nRemove worktree AND kill its session? (y/n)")
			if m.modalForce {
				b.WriteString("  [force=on]")
			} else {
				b.WriteString("  press f to force")
			}
		} else {
			b.WriteString("\nUnregister this dir? Sessions and worktrees are kept. (y/n)")
		}
		if m.modalErr != "" {
			b.WriteString("\n  ⚠ ")
			b.WriteString(m.modalErr)
		}
	case m.searching:
		b.WriteString("\n/")
		b.WriteString(m.search)
	default:
		b.WriteString("\n↑↓ move  / search  enter open  o files  e edit  n new  b branch  u pull  m merge-main  p push  x stop  r remove  q quit")
	}
	return b.String()
}

// StatusEmoji maps a state.Kind to its emoji marker. Returns two spaces for
// KindNone — the colored emojis above are East-Asian-wide (2 columns), so
// padding the empty case with two spaces keeps every row's display column
// aligned regardless of status.
func StatusEmoji(k state.Kind) string {
	switch k {
	case state.KindWaitingInput:
		return "🔴"
	case state.KindReady:
		return "🟢"
	case state.KindWorking:
		return "🟡"
	case state.KindIdle:
		return "⚪"
	default:
		return "  "
	}
}

// Cursor returns the index of the currently selected entry in the filtered view.
func (m Model) Cursor() int { return m.cursor }

// Filtered returns a copy of the filtered entries currently displayed.
func (m Model) Filtered() []Entry {
	out := make([]Entry, len(m.filtered))
	copy(out, m.filtered)
	return out
}

// Quitting reports whether the model has requested termination.
func (m Model) Quitting() bool { return m.quitting }

// PendingAttach returns the slug of the most recent Enter target, for tests.
func (m Model) PendingAttach() string { return m.pendingAttach }
