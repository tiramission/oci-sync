package cmd

import (
	"github.com/charmbracelet/bubbles/key"
)

// Keyboard bindings for the TUI. They are declared in one place so the
// footer, the help overlay and the update handlers can never drift apart.
//
// Every help key carries a trailing colon: the design's footer reads
// "enter: open · tab: pane", and the colon is part of the key's help text
// because bubbles/help always joins a binding as "key desc".

type keyMap struct {
	Quit     key.Binding
	Up       key.Binding
	Down     key.Binding
	Top      key.Binding
	Bottom   key.Binding
	Enter    key.Binding
	Back     key.Binding
	Next     key.Binding
	Tab      key.Binding
	BackTab  key.Binding
	Panel1   key.Binding
	Panel2   key.Binding
	Refresh  key.Binding
	Filter   key.Binding
	Push     key.Binding
	Pull     key.Binding
	Delete   key.Binding
	Edit     key.Binding
	Sort     key.Binding
	Help     key.Binding
	Yes      key.Binding
	No       key.Binding
	Navigate key.Binding // footer-only: up and down merged into one help entry
}

func newKeyMap() keyMap {
	return keyMap{
		Quit:    key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q:", "quit")),
		Up:      key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k:", "up")),
		Down:    key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j:", "down")),
		Top:     key.NewBinding(key.WithKeys("g"), key.WithHelp("g:", "top")),
		Bottom:  key.NewBinding(key.WithKeys("G"), key.WithHelp("G:", "bottom")),
		Enter:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter:", "open")),
		Back:    key.NewBinding(key.WithKeys("esc", "left", "h"), key.WithHelp("esc:", "back")),
		Next:    key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l:", "open")),
		Tab:     key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab:", "pane")),
		BackTab: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab:", "pane")),
		Panel1:  key.NewBinding(key.WithKeys("1"), key.WithHelp("1:", "shortcuts")),
		Panel2:  key.NewBinding(key.WithKeys("2"), key.WithHelp("2:", "artifacts")),
		Refresh: key.NewBinding(key.WithKeys("r"), key.WithHelp("r:", "refresh")),
		Filter:  key.NewBinding(key.WithKeys("/"), key.WithHelp("/:", "filter")),
		// Push is deliberately capital P while pull is lowercase p. The two
		// verbs sit side by side in the footer, and sharing one key would
		// make a muscle-memory slip on the most frequent action write to
		// the registry instead of reading from it.
		Push:   key.NewBinding(key.WithKeys("P"), key.WithHelp("P:", "push")),
		Pull:   key.NewBinding(key.WithKeys("p"), key.WithHelp("p:", "pull")),
		Delete: key.NewBinding(key.WithKeys("d"), key.WithHelp("d:", "delete")),
		Edit:   key.NewBinding(key.WithKeys("e"), key.WithHelp("e:", "labels")),
		Sort:   key.NewBinding(key.WithKeys("s"), key.WithHelp("s:", "sort")),
		Help:   key.NewBinding(key.WithKeys("?"), key.WithHelp("?:", "help")),
		Yes:    key.NewBinding(key.WithKeys("y"), key.WithHelp("y:", "yes")),
		No:     key.NewBinding(key.WithKeys("n"), key.WithHelp("n:", "no")),
		// Navigate is help text only — the footer offers one entry for
		// vertical movement, "↑/k ↓/j: navigate", instead of two. Never
		// match against it: its key list spans both directions, so a match
		// would be ambiguous about which way the cursor went. Up and Down
		// keep handling the actual keys.
		Navigate: key.NewBinding(key.WithKeys("up", "k", "down", "j"), key.WithHelp("↑/k ↓/j:", "navigate")),
	}
}
