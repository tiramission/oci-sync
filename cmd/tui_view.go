package cmd

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
	"github.com/tiramission/oci-sync/internal/oci"
)

// Rendering for the TUI. Every size is derived from the live window
// dimensions through geometry, so this file only formats state that the model
// logic has already resolved; it performs no I/O and holds no mutable state.

func (m model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	if m.width < minWidth || m.height < minHeight {
		return place(m.width, m.height,
			stWarn.Render(fmt.Sprintf("terminal too small — need at least %d×%d", minWidth, minHeight)))
	}

	if overlay := m.viewOverlay(); overlay != "" {
		return place(m.width, m.height, overlay)
	}
	return joinVertical(m.viewHeader(), m.viewPanes(), m.viewFooter())
}

// place centres a block in a window and pads it out to the full height.
//
// It replaces lipgloss.Place because that one measures with lipgloss's own
// width table, which follows the locale: on a CJK system it counts "↑", "↓"
// and "•" as two cells, so a block already sized to the window comes back
// wider than the window and the terminal scrolls. Every width in this file is
// measured with cellCond, and the compositor has to agree with it or the
// careful per-part arithmetic is undone at the last step.
func place(w, h int, content string) string {
	lines := strings.Split(content, "\n")
	// Centre within the content's own bounds first, then pad the block to the
	// window — otherwise a short dialog drifts toward the top-left of the room
	// its own width gives it.
	blockW, blockH := 0, len(lines)
	for _, l := range lines {
		blockW = max(blockW, cellWidth(l))
	}
	for i, l := range lines {
		lines[i] = padLeft(l, blockW+(w-blockW)/2)
	}
	if blockH > h {
		blockH = h
		lines = lines[:h]
	}
	top, bottom := (h-blockH)/2, h-blockH-(h-blockH)/2
	out := make([]string, 0, h)
	for range top {
		out = append(out, strings.Repeat(" ", w))
	}
	out = append(out, lines...)
	for range bottom {
		out = append(out, strings.Repeat(" ", w))
	}
	return strings.Join(out, "\n")
}

// joinVertical stacks blocks and pads every line to the widest one, so the
// frame is a rectangle. Like place, it measures with cellCond rather than
// lipgloss, for the same reason.
//
// It only pads; it never truncates. A line that is already too wide is a bug
// in the block that produced it, and hiding that here would lose the evidence
// the width assertions exist to catch.
func joinVertical(blocks ...string) string {
	lines := make([]string, 0, 16)
	width := 0
	for _, b := range blocks {
		for _, l := range strings.Split(b, "\n") {
			lines = append(lines, l)
			width = max(width, cellWidth(l))
		}
	}
	for i, l := range lines {
		lines[i] = padRight(l, width)
	}
	return strings.Join(lines, "\n")
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
	case m.state == stateFilter:
		// The query has to be on screen while it is being typed. The table
		// and the counter already move per keystroke; an input you cannot
		// see turns each keystroke into a guess, and the design asks for a
		// focus indicator that is always visible.
		right = stAccent.Render("/") +
			truncate(m.input.Value(), max(m.width/3, 8)) + stCursor.Render(" ")
		if count := stDim.Render(m.tagCount()); count != "" {
			right += "  " + count
		}
	case len(m.artifacts) > 0:
		right = stDim.Render(m.tagCount())
	}

	brand := stBrand.Render("  oci-sync  ")
	budget := m.width - cellWidth(brand) - cellWidth(right) - 3
	left := brand + "  " + stDim.Render(truncateMiddle(crumb, budget))
	pad := max(m.width-cellWidth(left)-cellWidth(right), 0)
	return left + strings.Repeat(" ", pad) + right
}

// tagCount is the header's right-hand counter: how many tags match, of how
// many there are, once a repository is loaded.
func (m model) tagCount() string {
	switch {
	case m.filterText != "" && len(m.artifacts) > 0:
		return fmt.Sprintf("%d/%d tags", len(m.visible), len(m.artifacts))
	case len(m.artifacts) > 0:
		return fmt.Sprintf("%d tags", len(m.artifacts))
	}
	return ""
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

// paneStyle draws one pane's frame.
//
// Focus is carried by the border's weight rather than its colour: the
// focused pane gets a double rule, the other a single one. A terminal gives
// us no half-pixel, and border weight is the cue that still reads at a
// glance in the corner of the eye, where a hue change between two similar
// violets does not. Colour still shifts with focus, but as reinforcement
// rather than as the signal.
func paneStyle(active bool, w, h int) lipgloss.Style {
	fg, border := colMuted, lipgloss.RoundedBorder()
	if active {
		fg, border = colAccent, lipgloss.ThickBorder()
	}
	return lipgloss.NewStyle().
		Border(border).
		BorderForeground(fg).
		Width(w).
		Height(h)
}

// paneHeight is the content height a pane gets.
//
// The panes are joined under a header row and above a footer row, and each
// pane adds a top and a bottom border of its own — four rows the window
// never has. Height() counts content only, so the borders come out of this
// number too. Getting it wrong pushes the footer off the bottom of the
// terminal, which shows up as a screen that scrolls instead of as an
// obviously broken one.
func (m model) paneHeight() int {
	return max(m.height-4, 1)
}

func (m model) viewSidebar(g geometry) string {
	active := !m.focusPanes && m.state == stateBrowse
	paneH := m.paneHeight()
	// The focused pane's title carries the accent, as in the design; the
	// border's weight is what reads at a glance, the colour reinforces it.
	titleStyle := stDim
	if active {
		titleStyle = stAccent
	}
	lines := []string{titleStyle.Render(truncatePane("Shortcuts", g.sbW))}
	for i := m.sbScroll; i < len(m.shortcuts) && i < m.sbScroll+g.listRows; i++ {
		s := m.shortcuts[i]
		gutter := "  "
		if i == m.shortcutIdx {
			gutter = "▸ "
		}
		name := truncatePane(s.Name, g.sbW-2)
		plain := gutter + name
		// The repo tail gets what the name left over — measured in wrap
		// cells, so a name that grew an ellipsis does not push the repo
		// tail past the frame's right edge.
		repoW := g.sbW - wrapWidth(plain) - 2
		repo := ""
		if i != m.shortcutIdx && repoW > 8 {
			repo = truncateMiddlePane(s.Repo, repoW)
		}
		switch {
		case i == m.shortcutIdx && active:
			lines = append(lines, stCursor.Render(padWrap(plain, g.sbW)))
		case i == m.shortcutIdx:
			lines = append(lines, stCursorBlur.Render(padWrap(plain, g.sbW)))
		case repo != "":
			lines = append(lines, padWrap(plain, g.sbW-wrapWidth(repo))+stDim.Render(repo))
		default:
			lines = append(lines, plain)
		}
	}
	if len(m.shortcuts) == 0 {
		lines = append(lines, stDim.Render("No shortcuts."))
	}
	// Pad, then append: the foot belongs on the pane's last row, and
	// lipgloss's Height pads after the content it was handed — putting the
	// foot in last is what pins it to the bottom border.
	for len(lines) < paneH-1 {
		lines = append(lines, "")
	}
	lines = append(lines, m.sidebarFoot(g))
	return paneStyle(active, g.sbW, paneH).Render(strings.Join(lines, "\n"))
}

// sidebarFoot is the sidebar's status line: the loaded repository with its
// tag count — the design's "ghcr.io/acme/configs · 14" — or, before
// anything is loaded, the repository the selection would load. The count is
// the point: it sits where the eye rests after a switch, so knowing how
// many artifacts a repository holds never costs a trip to the table.
func (m model) sidebarFoot(g geometry) string {
	var s string
	switch {
	case m.repo != "":
		s = fmt.Sprintf("%s · %d", m.repo, len(m.artifacts))
	case len(m.shortcuts) > 0:
		s = m.selectedShortcut().Repo
	default:
		return ""
	}
	return stDim.Render(truncateMiddlePane(s, g.sbW))
}

func (m model) viewArtifacts(g geometry) string {
	active := m.focusPanes && m.state == stateBrowse
	paneH := m.paneHeight()
	// The title names the table and, once one is loaded, the repository it
	// belongs to — a table with no repository above it reads as belonging
	// to whatever was selected last.
	title := "Artifacts"
	if m.repo != "" {
		title += " · " + truncateMiddlePane(m.repo, max(g.mainW-cellWidth(title)-3, 8))
	}
	titleStyle := stDim
	if active {
		titleStyle = stAccent
	}
	lines := []string{titleStyle.Render(truncatePane(title, g.mainW))}

	switch {
	case m.op == opFetch:
		// Six skeleton rows under the real column header, as the design
		// asks. One spinner line let the table collapse to a single row
		// and spring back when the data landed; the skeleton keeps the
		// frame's shape so nothing jumps. The spinner and the esc hint
		// live in the header while the fetch runs.
		lines = append(lines, m.tableHeader(g))
		for range min(6, max(paneH-2, 1)) {
			lines = append(lines, stDim.Render(m.skeletonRow(g)))
		}
	case m.repo == "":
		lines = append(lines, stDim.Render("No repository loaded."),
			stDim.Render("Select a shortcut and press enter."))
	case len(m.artifacts) == 0:
		// An empty repository and an over-narrow filter are different
		// problems: one needs a push, the other needs the filter cleared.
		lines = append(lines, stDim.Render("This repository has no artifacts yet."),
			stDim.Render("P pushes the first one."))
	case len(m.visible) == 0:
		// The query lands in the copy, so it gets the pane's width: a long
		// filter would otherwise be measured over the frame's wrap limit
		// and split onto a second row.
		lines = append(lines, stDim.Render(truncatePane(fmt.Sprintf("No tags match %q.", m.filterText), g.mainW)),
			stDim.Render("esc clears the filter."))
	default:
		lines = append(lines, m.tableHeader(g))
		shown := 0
		for i := m.scrollOff; i < len(m.visible) && i < m.scrollOff+g.tagRows; i++ {
			lines = append(lines, m.artifactRow(g, i))
			shown++
		}
		// The foot answers "is there more above or below" — rows the
		// window can hold, of rows there are. Pinned to the pane's last
		// row the same way the sidebar pins its status line.
		for len(lines) < paneH-1 {
			lines = append(lines, "")
		}
		foot := fmt.Sprintf("%d of %d shown", shown, len(m.artifacts))
		lines = append(lines, stDim.Render(truncatePane(foot, g.mainW)))
	}
	return paneStyle(active, g.mainW, paneH).Render(strings.Join(lines, "\n"))
}

// tableHeader renders the column header. The sorted column carries a
// direction marker, so the order is readable from the table itself instead
// of having to be inferred from the rows.
//
// The markers are the small triangles (▴ ▾), not the arrows (↑ ↓): the
// arrows are ambiguous-width and count double on a CJK terminal, which
// would push this line — built to the frame width exactly — over the pane
// renderer's wrap limit. The triangles measure one cell in both locales,
// like ▸ in the row gutter, so the header keeps its exact column widths
// everywhere.
func (m model) tableHeader(g geometry) string {
	tagHdr, sizeHdr := "TAG", "SIZE"
	switch m.sortBy {
	case sortSize:
		sizeHdr = "SIZE ▾"
	case sortTag:
		tagHdr = "TAG ▴"
	}
	return stDim.Render("  " + padRight(truncatePane(tagHdr, g.tagW), g.tagW) + " " +
		padLeft(sizeHdr, sizeW) + " " + padRight("ENC", encW) +
		map[bool]string{true: " " + padRight("VER", g.verW), false: ""}[g.showVer])
}

// skeletonRow mirrors one table row in placeholder blocks — same column
// widths, same alignment — so the loading state has the shape of the data
// that is about to replace it.
func (m model) skeletonRow(g geometry) string {
	row := "  " + padRight(strings.Repeat("░", g.tagW), g.tagW) + " " +
		padLeft(strings.Repeat("░", sizeW), sizeW) + " " +
		padRight(strings.Repeat("░", encW), encW)
	if g.showVer {
		row += " " + padRight(strings.Repeat("░", g.verW), g.verW)
	}
	return row
}

// artifactRow renders one table row.
//
// The selected row is deliberately built from plain text: it is drawn
// reversed, and a colour set inside a reversed run renders as a background —
// the warn-coloured "enc" would become a yellow slab behind the text
// instead of a warning. The row keeps the text channel ("enc" vs "·") and
// the reversal; on unselected rows the full palette applies.
//
// Column widths follow tableHeader's rule: everything but the last column
// holds its exact cell width, and the last column absorbs the row's
// ambiguous glyphs — the "·" of an unencrypted row, the "…" of a
// truncated tag — so the line measures within the frame's wrap limit while
// every column still starts where the header says it does.
func (m model) artifactRow(g geometry, i int) string {
	a := m.visible[i]
	gutter := "  "
	if i == m.artifactIdx {
		gutter = "▸ "
	}
	enc := "·"
	if a.Encrypted {
		enc = "enc"
	}
	selected := i == m.artifactIdx
	head := gutter + padRight(truncatePane(a.Tag, g.tagW), g.tagW) + " " +
		padLeft(formatBytes(a.Size), sizeW) + " "

	if g.showVer {
		// The encryption cell holds its exact width — VER's start must
		// not move between encrypted and plain rows — so the version
		// column absorbs the row's ambiguous glyphs: the "·" here, the
		// "…" of a truncated tag in head.
		encCell := padRight(enc, encW)
		verBudget := g.verW - amb(head) - amb(encCell)
		verCell := padWrap(truncatePane(a.Version, verBudget), verBudget)
		if selected {
			return m.rowStyle().Render(padWrap(head+encCell+" "+verCell, g.mainW))
		}
		return head + m.encStyle(a.Encrypted).Render(encCell) + " " + stDim.Render(verCell)
	}
	// No version column: the encryption cell is last, so the debt lands
	// here instead.
	encCell := padWrap(enc, encW-amb(head))
	if selected {
		return m.rowStyle().Render(padWrap(head+encCell, g.mainW))
	}
	return head + m.encStyle(a.Encrypted).Render(encCell)
}

// rowStyle is the selected row's rendering: reversed, and — when this pane
// owns the keyboard — bold. The row is built from plain text because a
// colour set inside a reversed run renders as a background, which would
// turn the warn-coloured "enc" into a yellow slab.
func (m model) rowStyle() lipgloss.Style {
	if m.focusPanes && m.state == stateBrowse {
		return stCursor
	}
	return stCursorBlur
}

// encStyle is the encryption cell's colour on an unselected row:
// encryption is the one column worth catching in a glance, so it gets the
// warning token and everything else stays quiet.
func (m model) encStyle(encrypted bool) lipgloss.Style {
	if encrypted {
		return stWarn
	}
	return stDim
}

func (m model) viewFooter() string {
	k := m.keys
	switch m.state {
	case stateFilter, stateInputPath, stateInputPassphrase, stateEditLabels:
		return stDim.Render("enter: submit   ·   esc: cancel")
	case statePush:
		return stDim.Render("tab: next field   ·   enter: push   ·   esc: cancel")
	case stateConfirmDelete:
		return stDim.Render("←/→: choose   ·   enter: confirm   ·   esc: cancel")
	case stateDetail, stateResult:
		return stDim.Render("enter/esc: close")
	case stateHelp:
		return stDim.Render("esc: close help")
	case stateBrowse:
		return m.clampHelp(untruncatingHelp(m.help).ShortHelpView(m.fitBindings(m.footerBindings())))
	}
	return m.clampHelp(untruncatingHelp(m.help).ShortHelpView([]key.Binding{k.Quit}))
}

// footerBindings is the key set the browse footer offers, in the order the
// entries are worth keeping: a narrow terminal drops from the end, so what
// survives is what a first-time user needs first.
//
// It is the design's footer line — navigate, open, pane, push, pull,
// delete, help, quit. Everything else (`e` labels, `/` filter, `r`
// reload, `esc` back) lives on the key table behind "?": at 80 columns
// there is room for one or the other, and a footer that has silently shed
// half its entries teaches nobody which key went missing.
func (m model) footerBindings() []key.Binding {
	k := m.keys
	if !m.focusPanes {
		// The sidebar has no row-scoped verbs to offer, so it spends the
		// room on loading the selected repository instead.
		return []key.Binding{k.Navigate, k.Enter, k.Tab, k.Refresh, k.Help, k.Quit}
	}
	return []key.Binding{k.Navigate, k.Enter, k.Tab, k.Push, k.Pull, k.Delete, k.Help, k.Quit}
}

// untruncatingHelp returns h with its own truncation switched off, so the
// footer is fitted here instead of inside bubbles/help. help.Width exists for
// callers that let help do the fitting; this app measures with cellWidth and
// must not have a second, locale-driven opinion appended to it.
func untruncatingHelp(h help.Model) help.Model {
	h.Width = 0
	return h
}

// fitBindings drops bindings from the end until the help line fits. When
// anything had to go, it appends a marker pointing at the full key table.
//
// The marker is English and short on purpose. It is the one part of the
// footer nobody presses — it points at the full key table behind "?" — so it
// should cost as little width as possible.
//
// A marker is only worth its width when something was actually dropped, so
// the whole set is tried on its own first. Appending the marker unconditionally
// meant a window sized to hold every binding still lost one to make room for a
// pointer to a key table that was already on screen.
func (m model) fitBindings(bs []key.Binding) []key.Binding {
	marker := key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "more"))
	limit := m.helpLimit()
	wide := untruncatingHelp(m.help)
	if len(bs) > 0 && cellWidth(wide.ShortHelpView(bs)) <= limit {
		return bs
	}
	for len(bs) > 0 {
		withMarker := append(append([]key.Binding{}, bs...), marker)
		if cellWidth(wide.ShortHelpView(withMarker)) <= limit {
			return withMarker
		}
		bs = bs[:len(bs)-1]
	}
	return []key.Binding{marker}
}

// helpLimit is the width the footer line may occupy. It leaves a cell of slack
// on each side so the frame never butts against the window edge.
func (m model) helpLimit() int {
	return max(m.width-2, 10)
}

// clampHelp is the last line of defence on the footer width. fitBindings has
// already chosen a set of bindings that fits, so in a correct frame this is a
// no-op — it is here so that a future binding with an unusually long
// description cannot widen the whole screen, which lipgloss would answer by
// widening every row with it.
func (m model) clampHelp(s string) string {
	return truncate(s, m.helpLimit())
}

func (m model) viewOverlay() string {
	switch {
	case m.op == opPull || m.op == opPush || m.op == opDelete || m.op == opLabel:
		return m.viewBusy()

	case m.state == statePush:
		return m.viewPushForm()

	case m.state == stateEditLabels:
		return m.overlay(stHeading.Render("Labels"), m.editLabelsBody(), "enter: apply   ·   esc: cancel", colInfo, modalWidth)

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
		// The design requires the full repository:tag here, not the
		// shortcut's name: the line sits above an irreversible warning,
		// and an abbreviation that could belong to any of several
		// repositories is exactly the wrong thing to skim.
		body := stWarn.Render(a.FullName) +
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

// viewBusy covers every in-flight remote operation. The wording follows the
// verb so the overlay always names what is happening; pull is the only one
// with real byte progress, because it is the only one whose payload size is
// known before the transfer starts.
func (m model) viewBusy() string {
	var body string
	switch m.op {
	case opDelete:
		body = "Deleting " + m.opTarget + "…"
	case opPush:
		body = "Packing and pushing " + m.opTarget + "…"
	case opLabel:
		body = "Updating labels on " + m.opTarget + "…"
	default:
		body = "Pulling " + m.opTarget + " to " + m.destPath + "\n\n" + m.pullProgressBody(30)
	}
	body += "\n\n" + stDim.Render("elapsed "+time.Since(m.opStart).Truncate(time.Second).String())
	return m.overlay(stHeading.Render(m.spinner.View()+" working"), body, "esc: cancel", colWarn, modalWidth)
}

// viewPushForm renders the push dialog. Each row is a label column and a
// value column on one line: the label is padded to a fixed width and the
// value truncated to what is left, so a long path can never push the layout
// onto a second line or out of the box. The focused row carries the cursor.
func (m model) viewPushForm() string {
	const labelW = 14
	inner := modalWidth - 8
	valueW := inner - labelW

	var rows []string
	for i, f := range m.push.fields {
		value := f.value
		if !f.echo {
			// A masked input still draws one dot per character, which leaks
			// the length; every masked row gets the same fixed run instead.
			value = maskSecret(f.value)
		} else if i == m.push.focus {
			// The focused row draws the live input, which owns the cursor;
			// echoing the plain value too would render it twice.
			value = m.input.View()
		}
		if i == m.push.focus && value == "" {
			// A masked row has no input view to fall back on, so an empty
			// focused field still needs a cursor to show it is editable.
			value = stCursor.Render(" ")
		}
		label := padRight(f.label, labelW)
		if i == m.push.focus {
			// The focused row is the one with the cursor on it; marking it
			// by prefixing the label keeps the marker out of the value
			// column, where it would shift the text the user is editing.
			label = stAccent.Render(padRight("▸ "+f.label, labelW))
		} else {
			label = stDim.Render(padRight("  "+f.label, labelW))
		}
		rows = append(rows, label+truncate(value, valueW-2))
	}
	// The input's own placeholder reads as content on the focused row, so a
	// field still being filled in would otherwise look like it had a value.
	if m.push.fields[m.push.focus].value == "" {
		rows[m.push.focus] += stDim.Render(m.input.Placeholder)
	}
	body := strings.Join(rows, "\n") + "\n\n" +
		stDim.Render("passphrase empty = push plaintext")

	if m.push.err != "" {
		body += "\n\n" + stErr.Render("✗ "+m.push.err)
	}
	return m.overlay(stHeading.Render("Push to "+m.repo), body,
		"tab: next field   ·   enter: push   ·   esc: cancel", colInfo, modalWidth)
}

// maskSecret hides a passphrase that is not the focused field, so switching
// tabs never leaves the secret on screen.
func maskSecret(s string) string {
	if s == "" {
		return stDim.Render("(empty)")
	}
	return stDim.Render(strings.Repeat("•", 8))
}

func (m model) editLabelsBody() string {
	a, ok := m.currentArtifact()
	if !ok {
		return ""
	}
	var lines []string
	for _, k := range sortedLabelKeys(a.Labels) {
		lines = append(lines, "  "+truncate(k+"="+a.Labels[k], modalWidth-16))
	}
	body := ""
	if len(lines) == 0 {
		body = stDim.Render("No custom labels on this artifact.\n\n")
	} else {
		body = stHeading.Render("current") + "\n" + strings.Join(lines, "\n") + "\n\n"
	}
	return body + "change:\n" + m.input.View() +
		"\n\n" + stDim.Render("key=value sets a label, a bare key removes it")
}

func sortedLabelKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// The label column is padded to 11 and the value starts at 12, so the
// longest key ("repository", exactly 10 cells) still leaves one space
// before its value. Padding to the key's own width instead runs them
// together on exactly that key — and the one that runs together is the
// first field in the list.
func (m model) detailBody(a oci.ArtifactInfo, w int) string {
	const labelW = 11
	label := func(s string) string { return stDim.Render(padRight(s, labelW)) }
	field := func(k, v string) string {
		return label(k) + truncate(v, max(w-labelW-1, 8))
	}
	lines := []string{
		field("repository", a.Repo),
		field("name", a.FullName),
		field("digest", a.Digest),
		field("size", formatBytes(a.Size)),
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
		{k.Push, k.Pull, k.Delete, k.Edit, k.Sort, k.Help, k.Quit},
	})
}

// overlay draws a centred dialog.
//
// The height is capped to the window before it is drawn, because
// lipgloss.Place only centres what it is given: an overlay taller than the
// window is not centred on it, it simply runs off the bottom, taking the
// hint line — the one that says which key closes it — with it. Clamping
// the width alone would leave a short window able to break the same way.
func (m model) overlay(title, body, hints string, accent lipgloss.Color, w int) string {
	w = min(w, m.width-6)
	// Three different numbers, and using one for all three is how the hint
	// line gets cut. Measured behaviour of Border + Padding(1,2) +
	// MaxHeight: MaxHeight is applied to the padded text, so a body of b
	// rows plus padding is capped at MaxHeight, and the borders add two
	// more on top.
	//
	//   bodyRows  the most the body may be trimmed to
	//   maxHeight the cap passed to lipgloss, on padded text
	//   framed    bodyRows + padding + borders, which must fit the window
	// lipgloss's MaxHeight caps the whole framed dialog, borders included,
	// and Padding(1) is applied before the cap, so the room the text
	// actually gets is the window less two borders and two padding rows:
	//
	//   maxHeight  the cap to hand lipgloss — the whole frame
	//   bodyRows   what title + blanks + body + hint must fit inside,
	//              which is the same window less the four rows of frame
	//
	// Trimming against maxHeight instead leaves the text two rows over and
	// MaxHeight cuts the bottom, which is exactly where the hint is.
	const frameRows = 4
	maxHeight := max(m.height, 1)
	bodyRows := max(maxHeight-frameRows, 1)

	// The hint line is the one row that must never go: it is what says
	// which key closes the dialog, and a short window that cut it would
	// leave the user with no visible way out. So the body gives up rows
	// first, from the end — a missing field is a smaller problem than a
	// dialog that cannot be dismissed.
	hint := stDim.Render(hints)
	fit := func(b string) string { return title + "\n\n" + b + "\n\n" + hint }
	for {
		candidate := fit(body)
		if lipgloss.Height(candidate) <= bodyRows {
			break
		}
		lines := strings.Split(body, "\n")
		if len(lines) <= 1 {
			body = ""
			break
		}
		body = strings.Join(lines[:len(lines)-1], "\n")
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accent).
		Width(w).
		// MaxHeight, not Height: Height would also pad a short dialog out
		// to the full window, leaving a tall empty box around two fields.
		MaxHeight(maxHeight).
		Padding(1, 2).
		Render(fit(body))
}
