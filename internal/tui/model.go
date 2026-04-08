// Package tui implements the Bubble Tea TUI for ssf.
package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fclairamb/ssf/internal/state"
)

// modalKind is the kind of overlay currently shown over the list, if any.
type modalKind int

const (
	modalNone modalKind = iota
	modalBranchPrompt
	modalConfirmRemove
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

	quitting bool

	// autoAttachSlug, if non-empty, makes Init emit an AttachMsg that
	// attaches to that slug as if the user had pressed Enter on it.
	autoAttachSlug string

	// Recording fields used by tests/main to learn what the model wants to do.
	pendingAttach string // slug to attach via tea.ExecProcess
	lastError     string
}

// WithAutoAttach makes the next Init() emit an AttachMsg targeting slug.
// Used by `ssf <dir>` to immediately drop the user into claude for the
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
	cmds := []tea.Cmd{tea.SetWindowTitle("ssf: list")}
	if m.autoAttachSlug != "" {
		slug := m.autoAttachSlug
		cmds = append(cmds, func() tea.Msg { return AttachMsg{Slug: slug} })
	}
	return tea.Batch(cmds...)
}

// titleListView is the terminal title shown while the user is in the list.
const titleListView = "ssf: list"

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
// message is exported so cmd/ssf can drive the refresh from a background
// goroutine via program.Send — more reliable than tea.Tick across
// ExecProcess suspensions.
type RefreshMsg struct{}

// AttachMsg is a request to attach to the claude session for a given slug,
// emitted programmatically (not from a key press). Used to auto-attach when
// ssf is invoked with a directory argument.
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
	case tea.KeyMsg:
		return m.handleKey(msg)
	case StateMsg:
		return m.applyStateMsg(msg), nil
	case RefreshMsg:
		return m.applyRefresh(), nil
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
	case "r":
		if len(m.filtered) > 0 {
			m.modal = modalConfirmRemove
			m.modalForce = false
			m.modalErr = ""
		}
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
		if m.deps.Git != nil {
			args := BuildWorktreeAddArgs(entry.Path, m.modalInput)
			if _, err := m.deps.Git.Run("", args...); err != nil {
				m.lastError = err.Error()
			}
		}
		m.modal = modalNone
		m.modalInput = ""
	case tea.KeyBackspace:
		if len(m.modalInput) > 0 {
			m.modalInput = m.modalInput[:len(m.modalInput)-1]
		}
	case tea.KeyRunes, tea.KeySpace:
		m.modalInput += string(msg.Runes)
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
			// Worktree path: dirty check, kill session, git worktree remove.
			if m.deps.Git != nil {
				dirty, _ := m.deps.Git.IsDirty(entry.Path)
				if dirty && !m.modalForce {
					m.modalErr = "worktree is dirty; press f to force"
					return m, nil
				}
			}
			if m.deps.Session != nil {
				_ = m.deps.Session.Kill(entry.Slug)
			}
			if m.deps.Git != nil {
				args := BuildWorktreeRemoveArgs(entry.Path, m.modalForce)
				_, _ = m.deps.Git.Run("", args...)
			}
		} else {
			// Registered dir: just unregister. Sessions and worktrees are
			// kept on disk per spec.
			if m.deps.Unregister != nil {
				if err := m.deps.Unregister(entry.Path); err != nil {
					m.modalErr = err.Error()
					return m, nil
				}
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
	b.WriteString("ssf — superset, fixed\n\n")
	for i, e := range m.filtered {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}
		b.WriteString(cursor)
		for j := 0; j < e.Indent; j++ {
			b.WriteString("  ")
		}
		b.WriteString(StatusEmoji(e.Kind))
		b.WriteString(" ")
		b.WriteString(e.Display)
		b.WriteString("\n")
	}
	switch {
	case m.modal == modalBranchPrompt:
		b.WriteString("\nNew worktree branch: ")
		b.WriteString(m.modalInput)
		if m.modalErr != "" {
			b.WriteString("\n  ⚠ ")
			b.WriteString(m.modalErr)
		}
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
		b.WriteString("\n↑↓ move  / search  enter open  o files  e edit  n new  r remove  q quit")
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
