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
