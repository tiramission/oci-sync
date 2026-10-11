package cmd

import (
	"context"
	"errors"
	"fmt"
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
				fmt.Sprintf("%3d%%  %s / %s", pct, formatBytes(s.Done), formatBytes(s.Total))
		}
		return fmt.Sprintf("Downloading %s…", formatBytes(s.Done))
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
