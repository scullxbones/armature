package dagsum

import "github.com/charmbracelet/bubbles/key"

type KeyMap struct {
	Confirm key.Binding
	Skip    key.Binding
	Quit    key.Binding
	Up      key.Binding
	Down    key.Binding
}

func DefaultKeyMap() KeyMap {
	return KeyMap{
		Confirm: key.NewBinding(key.WithKeys("enter", "y"), key.WithHelp("enter/y", "confirm item")),
		Skip:    key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "skip item")),
		Quit:    key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		Up:      key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:    key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	}
}
