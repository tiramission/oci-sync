package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Modern color scheme
const (
	colorBg     = "#1e1e2e"
	colorFg     = "#cdd6f4"
	colorBorder = "#45475a"
	colorCursor = "#f38ba8"
	colorAccent = "#89b4fa"
	colorSubtle = "#6c7086"
)

var (
	styleBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(colorBorder))

	styleHeader = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorAccent)).
			Bold(true)

	styleCursor = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorBg)).
			Background(lipgloss.Color(colorCursor))

	styleMuted = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorSubtle))

	styleText = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFg))
)

// Render full-screen modern TUI
func (m *Model) renderFullScreen() string {
	// Check minimum size
	if m.width < 120 || m.height < 28 {
		return m.renderSizeWarning()
	}

	var output strings.Builder

	// Calculate dimensions using ratios (4:6)
	shortcutsRatio := 0.4
	leftWidth := int(float64(m.width) * shortcutsRatio)
	if leftWidth < 30 {
		leftWidth = 30
	}
	rightWidth := m.width - leftWidth - 3
	topHeight := (m.height - 5) / 2
	bottomHeight := m.height - 5 - topHeight - 1

	// Top section: shortcuts and artifacts side by side
	leftContent := m.renderShortcutsPanel(leftWidth-3, topHeight-3)
	rightContent := m.renderArtifactsPanel(rightWidth-3, topHeight-3)

	leftPanel := m.styledPanel(leftContent, leftWidth, topHeight, "SHORTCUTS")
	rightPanel := m.styledPanel(rightContent, rightWidth, topHeight, "ARTIFACTS")

	// Combine top panels horizontally
	topRow := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)
	output.WriteString(topRow)
	output.WriteString("\n")

	// Bottom section: details full width
	detailContent := m.renderDetailsPanel(m.width-3, bottomHeight-3)
	detailPanel := m.styledPanel(detailContent, m.width, bottomHeight, "DETAILS")
	output.WriteString(detailPanel)
	output.WriteString("\n")

	// Footer
	footer := m.renderFooter(m.width)
	output.WriteString(footer)

	return output.String()
}

// Render size warning
func (m *Model) renderSizeWarning() string {
	title := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorAccent)).
		Bold(true).
		Render("OCI-SYNC")

	message := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorSubtle)).
		Render("Terminal window too small!\n\nRequired size: 120x28 or larger\n\nCurrent size: %dx%d")

	return fmt.Sprintf("%s\n\n%s\n", title, fmt.Sprintf(message, m.width, m.height))
}

// Create styled panel with title
func (m *Model) styledPanel(content string, width int, height int, title string) string {
	// Title bar
	titleBar := fmt.Sprintf("  %s", strings.ToUpper(title))
	titleLine := styleHeader.
		Width(width - 2).
		Render(titleBar)

	// Separator
	sep := strings.Repeat("─", width-2)

	// Combine content
	full := titleLine + "\n" + sep + "\n" + content

	return styleBorder.
		Width(width).
		Height(height).
		Padding(0, 1).
		Render(full)
}

// Shortcuts panel
func (m *Model) renderShortcutsPanel(width int, height int) string {
	var sb strings.Builder

	if m.loading {
		sb.WriteString(styleMuted.Render("Loading shortcuts..."))
		return sb.String()
	}

	if len(m.shortcuts) == 0 {
		sb.WriteString(styleMuted.Render("No shortcuts"))
		return sb.String()
	}

	// Show all shortcuts (limit by available height)
	linesPerItem := 2
	maxItems := (height - 3) / linesPerItem
	if maxItems < 0 {
		maxItems = 0
	}

	itemCount := 0
	for i, sc := range m.shortcuts {
		if itemCount >= maxItems {
			break
		}

		name := sc.Name
		if len(name) > width-5 {
			name = name[:width-8] + "..."
		}

		if i == m.selectedShortcut {
			sb.WriteString(styleCursor.Width(width).Render(fmt.Sprintf("❯ %s", name)))
		} else {
			sb.WriteString(styleText.Width(width).Render(fmt.Sprintf("  %s", name)))
		}
		sb.WriteString("\n")

		// Repo line (indented, muted)
		repo := sc.Repo
		if len(repo) > width-6 {
			repo = repo[:width-9] + "..."
		}
		sb.WriteString(styleMuted.Width(width).Render(fmt.Sprintf("    %s", repo)))
		sb.WriteString("\n")

		itemCount++
	}

	return sb.String()
}

// Artifacts panel
func (m *Model) renderArtifactsPanel(width int, height int) string {
	var sb strings.Builder

	if len(m.shortcuts) == 0 {
		sb.WriteString(styleMuted.Render("No shortcuts"))
		return sb.String()
	}

	if m.loading {
		sb.WriteString(styleMuted.Render("Loading..."))
		return sb.String()
	}

	if len(m.artifacts) == 0 {
		sb.WriteString(styleMuted.Render("No artifacts"))
		return sb.String()
	}

	// Show all artifacts (limit by available height)
	linesPerItem := 2
	maxItems := (height - 3) / linesPerItem
	if maxItems < 0 {
		maxItems = 0
	}

	itemCount := 0
	for i, art := range m.artifacts {
		if itemCount >= maxItems {
			break
		}

		tag := art.Tag
		maxTagLen := width - 15
		if len(tag) > maxTagLen {
			tag = tag[:maxTagLen-3] + "..."
		}

		// Status indicator: ○ unencrypted, ● encrypted
		status := "○"
		if art.Encrypted {
			status = "●"
		}

		if i == m.selectedArtifact {
			sb.WriteString(styleCursor.Width(width).Render(fmt.Sprintf("❯ %s %s", status, tag)))
		} else {
			sb.WriteString(styleText.Width(width).Render(fmt.Sprintf("  %s %s", status, tag)))
		}
		sb.WriteString("\n")

		// Info line: version + size (muted)
		info := art.Version
		if art.Size > 0 {
			info = fmt.Sprintf("%s • %s", art.Version, formatSize(art.Size))
		}
		if len(info) > width-6 {
			info = info[:width-9] + "..."
		}
		sb.WriteString(styleMuted.Width(width).Render(fmt.Sprintf("    %s", info)))
		sb.WriteString("\n")

		itemCount++
	}

	return sb.String()
}

// Details panel
func (m *Model) renderDetailsPanel(width int, height int) string {
	var sb strings.Builder

	if len(m.artifacts) == 0 || m.selectedArtifact >= len(m.artifacts) {
		sb.WriteString(styleMuted.Render("Select artifact to view details"))
		return sb.String()
	}

	art := m.artifacts[m.selectedArtifact]

	// Detail fields
	details := []struct {
		label string
		value string
	}{
		{"Tag", truncateString(art.Tag, width-40)},
		{"Repository", truncateString(art.Repo, width-40)},
		{"Digest", art.Digest}, // Show full digest
		{"Encrypted", func() string {
			if art.Encrypted {
				return "Yes"
			}
			return "No"
		}()},
		{"Version", art.Version},
	}

	if art.Size > 0 {
		details = append(details, struct {
			label string
			value string
		}{"Size", formatSize(art.Size)})
	}

	if len(art.Labels) > 0 {
		details = append(details, struct {
			label string
			value string
		}{"Labels", fmt.Sprintf("%d", len(art.Labels))})
	}

	// Render as two columns
	for i := 0; i < len(details); i += 2 {
		left := details[i]
		leftStr := fmt.Sprintf("%-15s %s", left.label+":", truncateString(left.value, width/2-20))

		var line string
		if i+1 < len(details) {
			right := details[i+1]
			rightStr := fmt.Sprintf("%-15s %s", right.label+":", truncateString(right.value, width/2-20))
			line = fmt.Sprintf("%-*s │ %s", width/2-3, leftStr, rightStr)
		} else {
			line = leftStr
		}

		sb.WriteString(styleText.Render(line))
		sb.WriteString("\n")
	}

	return sb.String()
}

// Footer
func (m *Model) renderFooter(width int) string {
	left := " OCI-Sync"
	var right string

	if m.modal != nil {
		right = "ESC Cancel • Enter Confirm "
	} else {
		right = "↑↓ Select • Enter View • u Upload • d Download • x Delete • q Quit "
	}

	padding := width - len(left) - len(right) - 2
	if padding < 0 {
		padding = 0
		right = "q Quit"
	}

	footer := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorBg)).
		Background(lipgloss.Color(colorAccent)).
		Width(width).
		Render(left + strings.Repeat(" ", padding) + right)

	return footer
}

// Helper functions
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen < 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func formatSize(bytes int64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	size := float64(bytes)
	unitIdx := 0

	for size >= 1024 && unitIdx < len(units)-1 {
		size /= 1024
		unitIdx++
	}

	if unitIdx == 0 {
		return fmt.Sprintf("%dB", int64(size))
	}
	return fmt.Sprintf("%.1f%s", size, units[unitIdx])
}
