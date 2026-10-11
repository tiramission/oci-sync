package cmd

import (
	"github.com/charmbracelet/bubbles/key"
)

// Keyboard bindings for the TUI. They are declared in one place so the
// footer, the help overlay and the update handlers can never drift apart.

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
