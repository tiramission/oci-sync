package cmd

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"charm.land/log/v2"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/spf13/cobra"
	"github.com/tiramission/oci-sync/internal/config"
	"github.com/tiramission/oci-sync/internal/oci"
)

// The TUI is a full-screen session on the alternate screen: a shortcuts
// sidebar, an artifacts table, and modal overlays for input, confirmation,
// detail and results. Focus is always on exactly one pane while browsing;
// overlays own the keyboard while they are up. All remote work runs in
// tea.Cmd goroutines and reports back through messages, never in Update or
// View. Geometry is derived from the latest window size on every frame, and
// the layout collapses to the focused pane alone on narrow terminals.

const (
	minWidth    = 48
	minHeight   = 12
	narrowWidth = 76 // below this width only the focused pane is rendered
	modalWidth  = 56
	detailWidth = 72
	sizeW       = 9
	encW        = 3
)

type viewState int

const (
	stateBrowse viewState = iota
	stateFilter
	stateInputPath
	stateInputPassphrase
	stateConfirmDelete
	stateDetail
	stateResult
	stateHelp
)

type opKind int

const (
	opNone opKind = iota
	opFetch
	opPull
	opDelete
)

// Messages returned by async commands.

type fetchArtifactsMsg struct {
	seq       int
	artifacts []oci.ArtifactInfo
	err       error
}

type pullResultMsg struct{ err error }

type deleteResultMsg struct{ err error }

// Semantic color tokens. Values are plain ANSI-16 so the user's terminal
// theme stays authoritative. Meaning is always paired with text, never
// color alone; lipgloss downsamples per profile and honors NO_COLOR.
var (
	colAccent = lipgloss.Color("5") // focus, brand
	colInfo   = lipgloss.Color("4") // secondary highlight
	colOK     = lipgloss.Color("2") // success
	colWarn   = lipgloss.Color("3") // pending, destructive target
	colErr    = lipgloss.Color("1") // failure
	colMuted  = lipgloss.Color("8") // chrome, metadata

	stApp        = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	stDim        = lipgloss.NewStyle().Foreground(colMuted)
	stHeading    = lipgloss.NewStyle().Bold(true)
	stCursor     = lipgloss.NewStyle().Reverse(true).Bold(true)
	stCursorBlur = lipgloss.NewStyle().Foreground(colMuted)
	stOK         = lipgloss.NewStyle().Bold(true).Foreground(colOK)
	stErr        = lipgloss.NewStyle().Bold(true).Foreground(colErr)
	stWarn       = lipgloss.NewStyle().Bold(true).Foreground(colWarn)
)

// truncate cuts s to w display cells, appending an ellipsis. Width is
// measured in terminal cells so CJK and wide glyphs stay aligned. It must be
// applied to plain strings, before any styling.
func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= w {
		return s
	}
	tail := "…"
	budget := w - runewidth.StringWidth(tail)
	if budget < 0 {
		return truncate(tail, max(w, 0))
	}
	var b strings.Builder
	used := 0
	for _, r := range s {
		rw := runewidth.RuneWidth(r)
		if used+rw > budget {
			break
		}
		b.WriteRune(r)
		used += rw
	}
	return b.String() + tail
}

func padRight(s string, w int) string {
	return s + strings.Repeat(" ", max(w-runewidth.StringWidth(s), 0))
}

func padLeft(s string, w int) string {
	return strings.Repeat(" ", max(w-runewidth.StringWidth(s), 0)) + s
}

type keyMap struct {
	Quit    key.Binding
	Up      key.Binding
	Down    key.Binding
	Top     key.Binding
	Bottom  key.Binding
	Enter   key.Binding
	Back    key.Binding
	Next    key.Binding
	Tab     key.Binding
	BackTab key.Binding
	Panel1  key.Binding
	Panel2  key.Binding
	Refresh key.Binding
	Filter  key.Binding
	Pull    key.Binding
	Delete  key.Binding
	Help    key.Binding
	Yes     key.Binding
	No      key.Binding
}

func newKeyMap() keyMap {
	return keyMap{
		Quit:    key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		Up:      key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:    key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Top:     key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "top")),
		Bottom:  key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "bottom")),
		Enter:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		Back:    key.NewBinding(key.WithKeys("esc", "left", "h"), key.WithHelp("esc", "back")),
		Next:    key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "open")),
		Tab:     key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "pane")),
		BackTab: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "pane")),
		Panel1:  key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "shortcuts")),
		Panel2:  key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "artifacts")),
		Refresh: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		Filter:  key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		Pull:    key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "pull")),
		Delete:  key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
		Help:    key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Yes:     key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "yes")),
		No:      key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "no")),
	}
}

type model struct {
	baseCtx context.Context

	state      viewState
	focusPanes bool // false: shortcuts sidebar, true: artifacts table
	confirmYes bool
	destPath   string
	input      textinput.Model

	shortcuts   []config.ShortcutInfo
	shortcutIdx int
	sbScroll    int

	repo        string // shortcut repo currently loaded or loading
	artifacts   []oci.ArtifactInfo
	visible     []oci.ArtifactInfo
	artifactIdx int
	scrollOff   int

	fetchSeq    int
	fetchCancel context.CancelFunc
	filterText  string

	op        opKind
	opStart   time.Time
	opTarget  string
	opCancel  context.CancelFunc
	pullStage *progressSink

	lastOp      opKind
	resultOK    bool
	resultTitle string
	resultBody  string

	spinner spinner.Model
	help    help.Model
	keys    keyMap

	width, height int
}

func newTuiCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Interactive TUI to manage shortcut artifacts",
		Long:  `Launch a full-screen interactive terminal UI to view shortcuts, list remote tags, and pull or delete artifacts.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTui(cmd.Context())
		},
	}
	return cmd
}

func runTui(ctx context.Context) error {
	// Keep log output off the screen the TUI owns.
	origLevel := log.GetLevel()
	log.SetLevel(log.ErrorLevel)
	defer log.SetLevel(origLevel)

	shortcuts := config.GetAllShortcuts()
	if len(shortcuts) == 0 {
		log.SetLevel(origLevel)
		fmt.Println("No shortcuts configured. Use 'oci-sync alias add <name> --repo <repo>' to add one first.")
		return nil
	}

	p := tea.NewProgram(newTuiModel(ctx, shortcuts), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func newTuiModel(ctx context.Context, shortcuts []config.ShortcutInfo) model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Width = modalWidth - 10
	return model{
		baseCtx:   ctx,
		shortcuts: shortcuts,
		input:     ti,
		spinner:   spinner.New(spinner.WithSpinner(spinner.Dot)),
		help:      help.New(),
		keys:      newKeyMap(),
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) selectedShortcut() config.ShortcutInfo {
	if m.shortcutIdx >= len(m.shortcuts) {
		return config.ShortcutInfo{}
	}
	return m.shortcuts[m.shortcutIdx]
}

func (m model) currentArtifact() (oci.ArtifactInfo, bool) {
	if m.artifactIdx >= len(m.visible) {
		return oci.ArtifactInfo{}, false
	}
	return m.visible[m.artifactIdx], true
}

func (m model) currentRef() string {
	if a, ok := m.currentArtifact(); ok {
		return m.selectedShortcut().Name + ":" + a.Tag
	}
	return ""
}

// geometry derives every size from the current terminal dimensions, so a
// resize re-layouts from live state.
type geometry struct {
	narrow   bool
	sbW      int // sidebar content width
	mainW    int // artifacts content width
	listRows int // visible sidebar rows
	tagRows  int // visible table rows
	tagW     int
	showVer  bool
	verW     int
}

func (m model) geo() geometry {
	w, h := m.width, m.height
	g := geometry{narrow: w < narrowWidth}
	if g.narrow {
		g.sbW = w - 2
		g.mainW = w - 2
	} else {
		g.sbW = min(max(w/4, 16), 28)
		g.mainW = w - g.sbW - 4
	}
	// Each pane spends 2 rows on borders; the table also uses one column
	// header row and both panes one title row.
	g.listRows = max(h-5, 1)
	g.tagRows = max(h-6, 1)
	g.showVer = g.mainW >= 62
	g.verW = 8
	if g.mainW >= 72 {
		g.verW = 12
	}
	g.tagW = g.mainW - 2 - 1 - sizeW - 1 - encW
	if g.showVer {
		g.tagW -= 1 + g.verW
	}
	g.tagW = max(g.tagW, 8)
	return g
}

// Async command starters. startFetch invalidates any in-flight fetch so
// stale results are dropped by their sequence number.

// progressSink carries the latest pull stage from the command goroutine to
// the UI goroutine. It is written by runPull and read while rendering.
type progressSink struct {
	mu    sync.Mutex
	stage PullStage
}

func (p *progressSink) set(s PullStage) {
	p.mu.Lock()
	p.stage = s
	p.mu.Unlock()
}

func (p *progressSink) get() PullStage {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stage
}

// renderBar draws a determinate progress bar of w cells using eighth-block
// sub-cell precision.
func renderBar(done, total int64, w int) string {
	if w < 1 {
		return ""
	}
	ratio := 0.0
	if total > 0 {
		ratio = min(max(float64(done)/float64(total), 0), 1)
	}
	filled := ratio * float64(w)
	full := int(filled)
	bar := strings.Repeat("█", full)
	used := full
	if full < w {
		if e := int((filled - float64(full)) * 8); e > 0 {
			bar += string([]rune("▏▎▍▌▋▊▉")[e-1])
		}
		used++
	}
	return bar + strings.Repeat("░", max(w-used, 0))
}

// pullProgressBody renders the current pull stage, with a byte-counted
// progress bar during the download phase.
func (m model) pullProgressBody(barW int) string {
	if m.pullStage == nil {
		return stDim.Render("preparing…")
	}
	s := m.pullStage.get()
	switch s.Phase {
	case "":
		return stDim.Render("preparing…")
	case "check":
		return "Resolving manifest…"
	case "download":
		if s.Total > 0 {
			pct := min(s.Done*100/max(s.Total, 1), 100)
			return renderBar(s.Done, s.Total, barW) + "  " +
				fmt.Sprintf("%3d%%  %s / %s", pct, formatBytes(int(s.Done)), formatBytes(int(s.Total)))
		}
		return fmt.Sprintf("Downloading %s…", formatBytes(int(s.Done)))
	case "decrypt":
		return renderBar(0, 1, barW) + "  Decrypting…"
	case "unpack":
		return renderBar(1, 1, barW) + "  Unpacking files…"
	default:
		return "Working…"
	}
}

func (m *model) startFetch() tea.Cmd {
	if m.fetchCancel != nil {
		m.fetchCancel()
		m.fetchCancel = nil
	}
	repo := m.selectedShortcut().Repo
	m.repo = repo
	m.artifacts = nil
	m.visible = nil
	m.artifactIdx = 0
	m.scrollOff = 0
	m.filterText = ""
	m.fetchSeq++
	m.op = opFetch
	m.opStart = time.Now()
	ctx, cancel := context.WithCancel(m.baseCtx)
	m.fetchCancel = cancel
	seq := m.fetchSeq
	return func() tea.Msg {
		artifacts, err := oci.List(ctx, repo)
		return fetchArtifactsMsg{seq: seq, artifacts: artifacts, err: err}
	}
}

func (m *model) startPull(tag, path, passphrase string) tea.Cmd {
	shortcut := m.selectedShortcut().Name
	ctx, cancel := context.WithCancel(m.baseCtx)
	m.opCancel = cancel
	m.op = opPull
	m.opStart = time.Now()
	m.opTarget = shortcut + ":" + tag
	sink := &progressSink{}
	m.pullStage = sink
	return func() tea.Msg {
		remote, err := buildShortcutRemoteRef(shortcut, tag)
		if err == nil {
			err = runPull(ctx, remote, path, passphrase, sink.set)
		}
		return pullResultMsg{err: err}
	}
}

func (m *model) startDelete(tag string) tea.Cmd {
	shortcut := m.selectedShortcut().Name
	ctx, cancel := context.WithCancel(m.baseCtx)
	m.opCancel = cancel
	m.op = opDelete
	m.opStart = time.Now()
	m.opTarget = shortcut + ":" + tag
	return func() tea.Msg {
		remote, err := buildShortcutRemoteRef(shortcut, tag)
		if err == nil {
			err = runDelete(ctx, remote)
		}
		return deleteResultMsg{err: err}
	}
}

func (m *model) cancelFetch() {
	if m.fetchCancel != nil {
		m.fetchCancel()
		m.fetchCancel = nil
	}
	m.op = opNone
}

func (m *model) setFilter(q string) {
	m.filterText = q
	m.visible = make([]oci.ArtifactInfo, 0, len(m.artifacts))
	for _, a := range m.artifacts {
		if matchFilter(a.Tag, q) {
			m.visible = append(m.visible, a)
		}
	}
	m.artifactIdx = min(m.artifactIdx, max(len(m.visible)-1, 0))
	m.scrollOff = 0
}

// matchFilter implements smart-case: a lowercase query matches case-insensitively.
func matchFilter(tag, q string) bool {
	if q == "" {
		return true
	}
	if strings.ToLower(q) == q {
		return strings.Contains(strings.ToLower(tag), q)
	}
	return strings.Contains(tag, q)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.Width = max(msg.Width-4, 20)
		return m, nil

	case spinner.TickMsg:
		if m.op == opNone {
			return m, nil // stop the tick loop once work is done
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case fetchArtifactsMsg:
		return m.onFetchResult(msg)

	case pullResultMsg:
		m.op, m.opCancel = opNone, nil
		return m.showResult(opPull, msg.err), nil

	case deleteResultMsg:
		m.op, m.opCancel = opNone, nil
		return m.showResult(opDelete, msg.err), nil

	case tea.KeyMsg:
		return m.updateKey(msg)
	}
	return m, nil
}

func (m model) onFetchResult(msg fetchArtifactsMsg) (tea.Model, tea.Cmd) {
	m.fetchCancel = nil
	if m.op != opFetch || msg.seq != m.fetchSeq {
		return m, nil // canceled or superseded result
	}
	m.op = opNone
	if msg.err != nil {
		m.repo = ""
		m.state = stateResult
		m.lastOp = opFetch
		m.resultOK = false
		m.resultTitle = "Cannot list tags"
		m.resultBody = msg.err.Error()
		return m, nil
	}
	m.artifacts = msg.artifacts
	m.applyFilter()
	m.artifactIdx = 0
	m.scrollOff = 0
	m.focusPanes = true
	return m, nil
}

func (m *model) applyFilter() {
	q := m.filterText
	m.visible = make([]oci.ArtifactInfo, 0, len(m.artifacts))
	for _, a := range m.artifacts {
		if matchFilter(a.Tag, q) {
			m.visible = append(m.visible, a)
		}
	}
}

func (m model) showResult(op opKind, err error) model {
	m.state = stateResult
	m.focusPanes = true
	m.lastOp = op
	m.pullStage = nil
	verb := "Pull"
	if op == opDelete {
		verb = "Delete"
	}
	switch {
	case err == nil:
		m.resultOK = true
		m.resultTitle = verb + " complete"
		if op == opDelete {
			m.resultBody = m.opTarget + " was removed from the registry."
		} else {
			m.resultBody = m.opTarget + " was unpacked to " + m.destPath
		}
	case errors.Is(err, context.Canceled):
		m.resultOK = true
		m.resultTitle = verb + " canceled"
		m.resultBody = "The operation was canceled before it finished."
	default:
		m.resultOK = false
		m.resultTitle = verb + " failed"
		m.resultBody = err.Error()
	}
	return m
}

func (m model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := m.keys

	// A pull/delete in flight owns the screen: only cancel is accepted.
	if m.op == opPull || m.op == opDelete {
		switch msg.String() {
		case "esc":
			if m.opCancel != nil {
				m.opCancel()
			}
		case "ctrl+c":
			if m.opCancel != nil {
				m.opCancel()
			}
			return m, tea.Quit
		}
		return m, nil
	}

	// Text-entry states consume printable keys before any shortcut does.
	switch m.state {
	case stateFilter, stateInputPath, stateInputPassphrase:
		return m.updateText(msg)
	}

	switch {
	case key.Matches(msg, k.Quit):
		return m, tea.Quit
	case key.Matches(msg, k.Help):
		switch m.state {
		case stateBrowse:
			m.state = stateHelp
		case stateHelp:
			m.state = stateBrowse
		}
		return m, nil
	}

	switch m.state {
	case stateBrowse:
		return m.updateBrowse(msg)
	case stateDetail, stateResult, stateConfirmDelete:
		return m.updateModal(msg)
	}
	return m, nil
}

func (m model) updateText(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case msg.String() == "esc":
		m.input.Blur()
		switch m.state {
		case stateFilter:
			m.setFilter("")
			m.state = stateBrowse
		case stateInputPassphrase:
			m.state = stateInputPath
			m.input.SetValue(m.destPath)
			m.input.EchoMode = textinput.EchoNormal
		case stateInputPath:
			m.state = stateBrowse
		}
		return m, nil
	case msg.String() == "enter":
		return m.submitText()
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.state == stateFilter {
		m.setFilter(m.input.Value())
	}
	return m, cmd
}

func (m model) submitText() (tea.Model, tea.Cmd) {
	switch m.state {
	case stateFilter:
		m.input.Blur()
		m.state = stateBrowse
	case stateInputPath:
		path := strings.TrimSpace(m.input.Value())
		if path == "" {
			return m, nil
		}
		m.destPath = path
		m.input.Blur()
		m.state = stateBrowse
		a, ok := m.currentArtifact()
		if !ok {
			break
		}
		if a.Encrypted {
			m.state = stateInputPassphrase
			m.input.SetValue("")
			m.input.Placeholder = "passphrase"
			m.input.EchoMode = textinput.EchoPassword
			return m, m.input.Focus()
		}
		return m, tea.Batch(m.startPull(a.Tag, path, ""), m.spinner.Tick)
	case stateInputPassphrase:
		pass := m.input.Value()
		m.input.Blur()
		m.input.EchoMode = textinput.EchoNormal
		m.state = stateBrowse
		if a, ok := m.currentArtifact(); ok {
			return m, tea.Batch(m.startPull(a.Tag, m.destPath, pass), m.spinner.Tick)
		}
	}
	return m, nil
}

func (m model) updateBrowse(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := m.keys

	switch {
	case key.Matches(msg, k.Tab), key.Matches(msg, k.Panel2):
		m.focusPanes = true
		return m, nil
	case key.Matches(msg, k.BackTab), key.Matches(msg, k.Panel1):
		m.focusPanes = false
		return m, nil
	}

	if !m.focusPanes {
		switch {
		case key.Matches(msg, k.Up):
			m.moveShortcut(-1)
		case key.Matches(msg, k.Down):
			m.moveShortcut(1)
		case key.Matches(msg, k.Top):
			m.shortcutIdx = 0
		case key.Matches(msg, k.Bottom):
			m.shortcutIdx = max(len(m.shortcuts)-1, 0)
		case key.Matches(msg, k.Enter), key.Matches(msg, k.Next):
			m.focusPanes = true
			return m, tea.Batch(m.startFetch(), m.spinner.Tick)
		case key.Matches(msg, k.Refresh):
			return m, tea.Batch(m.startFetch(), m.spinner.Tick)
		}
		m.clampScroll()
		return m, nil
	}

	switch {
	case key.Matches(msg, k.Back):
		if m.op == opFetch {
			m.cancelFetch()
			return m, nil
		}
		m.focusPanes = false
	case key.Matches(msg, k.Up):
		m.moveArtifact(-1)
	case key.Matches(msg, k.Down):
		m.moveArtifact(1)
	case key.Matches(msg, k.Top):
		m.artifactIdx = 0
	case key.Matches(msg, k.Bottom):
		m.artifactIdx = max(len(m.visible)-1, 0)
	case key.Matches(msg, k.Enter):
		if _, ok := m.currentArtifact(); ok {
			m.state = stateDetail
		}
	case key.Matches(msg, k.Filter):
		m.input.SetValue(m.filterText)
		m.input.Placeholder = "filter tags"
		m.state = stateFilter
		return m, m.input.Focus()
	case key.Matches(msg, k.Refresh):
		return m, tea.Batch(m.startFetch(), m.spinner.Tick)
	case key.Matches(msg, k.Pull):
		if a, ok := m.currentArtifact(); ok {
			m.input.SetValue("./" + a.Tag)
			m.input.Placeholder = "destination directory"
			m.state = stateInputPath
			return m, m.input.Focus()
		}
	case key.Matches(msg, k.Delete):
		if _, ok := m.currentArtifact(); ok {
			m.state = stateConfirmDelete
			m.confirmYes = false
		}
	}
	m.clampScroll()
	return m, nil
}

func (m model) updateModal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.state {
	case stateDetail:
		if msg.String() == "esc" || msg.String() == "enter" {
			m.state = stateBrowse
		}
	case stateResult:
		if msg.String() == "esc" || msg.String() == "enter" {
			m.state = stateBrowse
			if m.lastOp == opDelete && m.resultOK {
				return m, tea.Batch(m.startFetch(), m.spinner.Tick)
			}
		}
	case stateConfirmDelete:
		switch {
		case key.Matches(msg, m.keys.No), msg.String() == "left", msg.String() == "h":
			if key.Matches(msg, m.keys.No) {
				m.state = stateBrowse
			} else {
				m.confirmYes = false
			}
		case key.Matches(msg, m.keys.Yes):
			m.confirmYes = true
		case msg.String() == "right", msg.String() == "l":
			m.confirmYes = true
		case msg.String() == "esc":
			m.state = stateBrowse
		case msg.String() == "enter":
			m.state = stateBrowse
			if m.confirmYes {
				if a, ok := m.currentArtifact(); ok {
					return m, tea.Batch(m.startDelete(a.Tag), m.spinner.Tick)
				}
			}
		}
	}
	return m, nil
}

func (m *model) moveShortcut(delta int) {
	if len(m.shortcuts) == 0 {
		return
	}
	m.shortcutIdx = min(max(m.shortcutIdx+delta, 0), len(m.shortcuts)-1)
}

func (m *model) moveArtifact(delta int) {
	if len(m.visible) == 0 {
		return
	}
	m.artifactIdx = min(max(m.artifactIdx+delta, 0), len(m.visible)-1)
}

func (m *model) clampScroll() {
	g := m.geo()
	m.sbScroll = min(m.sbScroll, m.shortcutIdx)
	if m.sbScroll+g.listRows <= m.shortcutIdx {
		m.sbScroll = max(m.shortcutIdx-g.listRows+1, 0)
	}
	m.scrollOff = min(m.scrollOff, m.artifactIdx)
	if m.scrollOff+g.tagRows <= m.artifactIdx {
		m.scrollOff = max(m.artifactIdx-g.tagRows+1, 0)
	}
}

func (m model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	if m.width < minWidth || m.height < minHeight {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
			stWarn.Render(fmt.Sprintf("terminal too small — need at least %d×%d", minWidth, minHeight)))
	}

	if overlay := m.viewOverlay(); overlay != "" {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlay)
	}
	return lipgloss.JoinVertical(lipgloss.Top, m.viewHeader(), m.viewPanes(), m.viewFooter())
}

func (m model) viewHeader() string {
	crumb := "no repository loaded"
	switch {
	case m.repo != "":
		crumb = m.repo
	case len(m.shortcuts) > 0:
		crumb = m.selectedShortcut().Name + " → " + m.selectedShortcut().Repo
	}

	var right string
	switch {
	case m.op == opFetch:
		right = m.spinner.View() + " " + stDim.Render("loading…  esc cancels")
	case m.op == opPull:
		right = m.spinner.View() + " " + stDim.Render("pulling…  esc cancels")
	case m.op == opDelete:
		right = m.spinner.View() + " " + stDim.Render("deleting…  esc cancels")
	case m.filterText != "" && len(m.artifacts) > 0:
		right = stDim.Render(fmt.Sprintf("%d/%d tags", len(m.visible), len(m.artifacts)))
	case len(m.artifacts) > 0:
		right = stDim.Render(fmt.Sprintf("%d tags", len(m.artifacts)))
	}

	brand := stApp.Render("oci-sync")
	budget := m.width - lipgloss.Width(brand) - lipgloss.Width(right) - 3
	left := brand + "  " + stDim.Render(truncate(crumb, budget))
	pad := max(m.width-lipgloss.Width(left)-lipgloss.Width(right), 0)
	return left + strings.Repeat(" ", pad) + right
}

func (m model) viewPanes() string {
	g := m.geo()
	sidebar, main := m.viewSidebar(g), m.viewArtifacts(g)
	if g.narrow {
		if m.focusPanes {
			return main
		}
		return sidebar
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, sidebar, main)
}

func paneStyle(active bool, w, h int) lipgloss.Style {
	fg := colMuted
	if active {
		fg = colAccent
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(fg).
		Width(w).
		Height(h)
}

func (m model) viewSidebar(g geometry) string {
	active := !m.focusPanes && m.state == stateBrowse
	title := "Shortcuts"
	var lines []string
	if active {
		lines = append(lines, stHeading.Render(truncate(title, g.sbW)))
	} else {
		lines = append(lines, stDim.Render(truncate(title, g.sbW)))
	}
	for i := m.sbScroll; i < len(m.shortcuts) && i < m.sbScroll+g.listRows; i++ {
		s := m.shortcuts[i]
		gutter := "  "
		if i == m.shortcutIdx {
			gutter = "▸ "
		}
		name := truncate(s.Name, g.sbW-2)
		plain := gutter + name
		repoW := g.sbW - runewidth.StringWidth(plain) - 2
		switch {
		case i == m.shortcutIdx && active:
			lines = append(lines, stCursor.Render(padRight(plain, g.sbW)))
		case i == m.shortcutIdx:
			lines = append(lines, stCursorBlur.Render(padRight(plain, g.sbW)))
		case repoW > 8:
			lines = append(lines, padRight(plain, g.sbW-repoW)+stDim.Render(truncate(s.Repo, repoW)))
		default:
			lines = append(lines, plain)
		}
	}
	if len(m.shortcuts) == 0 {
		lines = append(lines, stDim.Render("No shortcuts."))
	}
	return paneStyle(active, g.sbW, m.height-2).Render(strings.Join(lines, "\n"))
}

func (m model) viewArtifacts(g geometry) string {
	active := m.focusPanes && m.state == stateBrowse
	var lines []string
	if active {
		lines = append(lines, stHeading.Render(truncate("Artifacts", g.mainW)))
	} else {
		lines = append(lines, stDim.Render(truncate("Artifacts", g.mainW)))
	}

	switch {
	case m.op == opFetch:
		lines = append(lines, m.spinner.View()+" "+stDim.Render("Loading tags…   esc: cancel"))
	case m.repo == "":
		lines = append(lines, stDim.Render("No repository loaded."),
			stDim.Render("Select a shortcut and press enter."))
	case len(m.visible) == 0 && m.filterText != "":
		lines = append(lines, stDim.Render(fmt.Sprintf("No tags match %q.", m.filterText)),
			stDim.Render("esc clears the filter."))
	case len(m.visible) == 0:
		lines = append(lines, stDim.Render("No tags in this repository."),
			stDim.Render("r refreshes."))
	default:
		header := stDim.Render("  " + padRight("TAG", g.tagW) + " " + padLeft("SIZE", sizeW) + " " + padRight("ENC", encW) +
			map[bool]string{true: " " + padRight("VER", g.verW), false: ""}[g.showVer])
		lines = append(lines, header)
		for i := m.scrollOff; i < len(m.visible) && i < m.scrollOff+g.tagRows; i++ {
			a := m.visible[i]
			body := padRight(truncate(a.Tag, g.tagW), g.tagW) + " " +
				padLeft(formatBytes(int(a.Size)), sizeW) + " " +
				padRight(map[bool]string{true: "enc", false: "·"}[a.Encrypted], encW)
			if g.showVer {
				body += " " + padRight(truncate(a.Version, g.verW), g.verW)
			}
			gutter := "  "
			if i == m.artifactIdx {
				gutter = "▸ "
			}
			line := gutter + body
			if i == m.artifactIdx {
				style := stCursorBlur
				if active {
					style = stCursor
				}
				line = style.Render(padRight(line, g.mainW))
			}
			lines = append(lines, line)
		}
	}
	return paneStyle(active, g.mainW, m.height-2).Render(strings.Join(lines, "\n"))
}

func (m model) viewFooter() string {
	k := m.keys
	switch m.state {
	case stateFilter, stateInputPath, stateInputPassphrase:
		return stDim.Render("enter: submit   ·   esc: cancel")
	case stateConfirmDelete:
		return stDim.Render("←/→: choose   ·   enter: confirm   ·   esc: cancel")
	case stateDetail, stateResult:
		return stDim.Render("enter/esc: close")
	case stateHelp:
		return stDim.Render("esc: close help")
	case stateBrowse:
		if m.focusPanes {
			return m.help.ShortHelpView([]key.Binding{
				k.Up, k.Down, k.Enter, k.Pull, k.Delete, k.Filter, k.Refresh, k.Back, k.Help, k.Quit,
			})
		}
		return m.help.ShortHelpView([]key.Binding{
			k.Up, k.Down, k.Enter, k.Tab, k.Refresh, k.Help, k.Quit,
		})
	}
	return m.help.ShortHelpView([]key.Binding{k.Quit})
}

func (m model) viewOverlay() string {
	switch {
	case m.op == opPull || m.op == opDelete:
		var body string
		if m.op == opDelete {
			body = "Deleting " + m.opTarget + "…"
		} else {
			body = "Pulling " + m.opTarget + " to " + m.destPath + "\n\n" + m.pullProgressBody(30)
		}
		body += "\n\n" + stDim.Render("elapsed "+time.Since(m.opStart).Truncate(time.Second).String())
		return m.overlay(stHeading.Render(m.spinner.View()+" working"), body, "esc: cancel", colWarn, modalWidth)

	case m.state == stateDetail:
		a, ok := m.currentArtifact()
		if !ok {
			return ""
		}
		return m.overlay(stHeading.Render("Details")+"  "+stDim.Render(m.repo+":"+a.Tag), m.detailBody(a, detailWidth-8), "enter/esc: close", colInfo, detailWidth)

	case m.state == stateInputPath:
		body := "Save " + m.currentRef() + " to:\n\n" + m.input.View()
		return m.overlay(stHeading.Render("Pull destination"), body, "enter: submit   ·   esc: cancel", colInfo, modalWidth)

	case m.state == stateInputPassphrase:
		body := "This artifact is encrypted.\n\n" + m.input.View()
		return m.overlay(stHeading.Render("Decryption passphrase"), body, "enter: submit   ·   esc: back", colInfo, modalWidth)

	case m.state == stateConfirmDelete:
		a, ok := m.currentArtifact()
		if !ok {
			return ""
		}
		no, yes := "❯ No", " Yes"
		if m.confirmYes {
			no, yes = " No", "❯ Yes"
		}
		body := stWarn.Render(m.selectedShortcut().Name+":"+a.Tag) +
			"\n\nThis removes the tag from the remote registry and cannot be undone.\n\n" +
			no + "     " + yes
		return m.overlay(stHeading.Render("Delete remote artifact?"), body, "←/→: choose   ·   enter: confirm   ·   esc: cancel", colWarn, modalWidth)

	case m.state == stateResult:
		head := stErr.Render("✗ " + m.resultTitle)
		if m.resultOK {
			head = stOK.Render("✓ " + m.resultTitle)
		}
		body := m.resultBody
		if m.lastOp == opFetch && !m.resultOK {
			body += "\n\nPress r to retry."
		}
		return m.overlay(head, body, "enter/esc: dismiss", colInfo, modalWidth)

	case m.state == stateHelp:
		return m.overlay(stHeading.Render("Keys"), m.helpBody(), "esc: close", colInfo, detailWidth)
	}
	return ""
}

func (m model) detailBody(a oci.ArtifactInfo, w int) string {
	label := func(s string) string { return stDim.Render(padRight(s, 10)) }
	field := func(k, v string) string {
		return label(k) + truncate(v, max(w-12, 8))
	}
	lines := []string{
		field("repository", a.Repo),
		field("name", a.FullName),
		field("digest", a.Digest),
		field("size", formatBytes(int(a.Size))),
		field("encrypted", map[bool]string{true: "yes", false: "no"}[a.Encrypted]),
		field("version", a.Version),
	}
	if len(a.Labels) > 0 {
		keys := make([]string, 0, len(a.Labels))
		for k := range a.Labels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		lines = append(lines, "", label("labels"))
		for _, k := range keys {
			lines = append(lines, "  "+truncate(k+"="+a.Labels[k], max(w-4, 8)))
		}
	}
	return strings.Join(lines, "\n")
}

func (m model) helpBody() string {
	k := m.keys
	return m.help.FullHelpView([][]key.Binding{
		{k.Up, k.Down, k.Top, k.Bottom},
		{k.Tab, k.BackTab, k.Panel1, k.Panel2},
		{k.Enter, k.Back, k.Filter, k.Refresh},
		{k.Pull, k.Delete, k.Help, k.Quit},
	})
}

func (m model) overlay(title, body, hints string, accent lipgloss.Color, w int) string {
	w = min(w, m.width-6)
	content := title + "\n\n" + body + "\n\n" + stDim.Render(hints)
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accent).
		Width(w).
		Padding(1, 2).
		Render(content)
}
