package board

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines the key bindings for the board model.
type KeyMap struct {
	Left   key.Binding
	Right  key.Binding
	Up     key.Binding
	Down   key.Binding
	Select key.Binding
	Quit   key.Binding
}

// DefaultKeyMap returns the default key bindings for the board.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Left:   key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "previous column")),
		Right:  key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "next column")),
		Up:     key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:   key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Select: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open detail")),
		Quit:   key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}
