package cmd

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
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
	return paneStyle(active, g.sbW, m.paneHeight()).Render(strings.Join(lines, "\n"))
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
	case len(m.artifacts) == 0:
		// An empty repository and an over-narrow filter are different
		// problems: one needs a push, the other needs the filter cleared.
		lines = append(lines, stDim.Render("This repository has no artifacts yet."),
			stDim.Render("P pushes the first one."))
	case len(m.visible) == 0:
		lines = append(lines, stDim.Render(fmt.Sprintf("No tags match %q.", m.filterText)),
			stDim.Render("esc clears the filter."))
	default:
		header := stDim.Render("  " + padRight("TAG", g.tagW) + " " + padLeft("SIZE", sizeW) + " " + padRight("ENC", encW) +
			map[bool]string{true: " " + padRight("VER", g.verW), false: ""}[g.showVer])
		lines = append(lines, header)
		for i := m.scrollOff; i < len(m.visible) && i < m.scrollOff+g.tagRows; i++ {
			a := m.visible[i]
			body := padRight(truncate(a.Tag, g.tagW), g.tagW) + " " +
				padLeft(formatBytes(a.Size), sizeW) + " " +
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
	return paneStyle(active, g.mainW, m.paneHeight()).Render(strings.Join(lines, "\n"))
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
		// The footer carries the keys a first-time user needs and no
		// more: where the cursor moves, what opens it, and how to push,
		// pull or delete. Filtering and reloading are on the key table
		// (press ?) rather than here — at 80 columns there is room for
		// one or the other, and a footer that has silently shed half its
		// entries teaches nobody which key went missing.
		//
		// The bindings are added in the order they are worth keeping, so
		// a narrow terminal loses the last ones and not the first.
		all := []key.Binding{
			k.Up, k.Down, k.Enter,
			k.Push, k.Pull, k.Delete, k.Edit,
			k.Filter, k.Refresh, k.Back, k.Help, k.Quit,
		}
		if !m.focusPanes {
			// The sidebar has no row-scoped verbs to offer, so it spends
			// the room on moving between panes instead.
			all = []key.Binding{k.Up, k.Down, k.Enter, k.Tab, k.Refresh, k.Help, k.Quit}
		}
		return m.clampHelp(m.help.ShortHelpView(m.fitBindings(all)))
	}
	return m.clampHelp(m.help.ShortHelpView([]key.Binding{k.Quit}))
}

// fitBindings drops bindings from the end until the help line fits, then
// says so in a trailing marker.
//
// The marker is English and short on purpose. It is the one part of the
// footer nobody presses — it points at the full key table behind "?" — so
// it should cost as little width as possible, and measuring a CJK label to
// decide whether it fits is a whole class of bug this does not need.
// fitBindings drops bindings from the end until the help line fits whole,
// then appends a marker pointing at the full key table.
//
// bubbles/help truncates by itself once the line passes help.Width and ends
// it with an ellipsis, so the only honest test of "does this fit" is whether
// the rendered help still contains every binding's description. Measuring
// the returned string instead cannot tell a line that fits from one help
// has already cut, and the marker would be appended to the tail of a
// truncated line — visible in the output as a help entry with no key.
//
// The marker is English and short on purpose. It is the one part of the
// footer nobody presses, so it should cost as little width as possible,
// and measuring a CJK label to decide whether it fits is a class of bug
// this does not need.
func (m model) fitBindings(bs []key.Binding) []key.Binding {
	marker := key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "more"))
	for len(bs) > 0 {
		withMarker := append(append([]key.Binding{}, bs...), marker)
		if m.helpShowsAll(withMarker) {
			return withMarker
		}
		bs = bs[:len(bs)-1]
	}
	return []key.Binding{marker}
}

// helpShowsAll reports whether help renders every binding's description,
// which is false exactly when help had to cut the line short.
func (m model) helpShowsAll(bs []key.Binding) bool {
	rendered := m.help.ShortHelpView(bs)
	for _, b := range bs {
		if d := b.Help().Desc; d != "" && !strings.Contains(rendered, d) {
			return false
		}
	}
	return true
}

// clampHelp keeps the footer inside the window. bubbles/help renders whatever
// bindings it is given without truncating, so on a narrow terminal the extra
// row would otherwise be wider than the panes above it and lipgloss would
// widen the whole screen to match.
func (m model) clampHelp(s string) string {
	limit := m.width - 2
	if limit < 10 {
		limit = 10
	}
	return truncate(s, limit)
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
		{k.Push, k.Pull, k.Delete, k.Edit, k.Help, k.Quit},
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
