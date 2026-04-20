package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
)

// Run starts the TUI application
func Run(ctx context.Context) error {
	m := New(ctx)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}
