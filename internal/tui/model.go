// Package tui implements the Bubble Tea TUI for ssf.
package tui

import (
	"strings"

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

	// Recording fields used by tests/main to learn what the model wants to do.
	pendingAttach string // slug to attach via tea.ExecProcess
	lastError     string
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
func (m Model) Init() tea.Cmd { return nil }

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
	}
	return m, nil
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
			return m, tea.ExecProcess(cmd, func(error) tea.Msg { return nil })
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

// StatusEmoji maps a state.Kind to its emoji marker. Returns a single space
// for KindNone so layout stays aligned.
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
		return " "
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
