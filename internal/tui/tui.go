// Package tui renders and handles input for the interactive TPB interface.
package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

const defaultWidth = 80

// Model is the interactive state for a single TPB bin.
type Model struct {
	binName      string
	slots        map[int]string
	selectedSlot int
	expanded     bool
	width        int
}

// New creates a compact slot-selection view for binName.
func New(binName string, slots map[int]string) Model {
	values := make(map[int]string, 10)
	for slot, value := range slots {
		if slot >= 0 && slot <= 9 {
			values[slot] = value
		}
	}

	return Model{
		binName:      binName,
		slots:        values,
		selectedSlot: -1,
		width:        defaultWidth,
	}
}

// Run starts the interactive TUI for a bin.
func Run(binName string, slots map[int]string) error {
	_, err := tea.NewProgram(New(binName, slots)).Run()
	return err
}

// Init performs no startup work.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update applies keypresses and terminal resize events.
func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.KeyPressMsg:
		return m.handleKey(message.String())
	case tea.WindowSizeMsg:
		m.width = message.Width
	}
	return m, nil
}

// View renders the entire current interface.
func (m Model) View() tea.View {
	view := tea.NewView(m.render())
	view.AltScreen = true
	view.WindowTitle = "Terminal Paste Bin"
	return view
}

func (m Model) handleKey(key string) (tea.Model, tea.Cmd) {
	if key == "q" {
		return m, tea.Quit
	}
	if key == "v" && m.selectedSlot >= 0 {
		m.expanded = !m.expanded
		return m, nil
	}
	if len(key) == 1 && key[0] >= '0' && key[0] <= '9' {
		m.selectedSlot = int(key[0] - '0')
		m.expanded = false
	}
	return m, nil
}

func (m Model) render() string {
	if m.expanded {
		return m.renderExpanded()
	}

	var output strings.Builder
	fmt.Fprintf(&output, "Terminal Paste Bin - %s\n\n", m.binName)

	for _, slot := range []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 0} {
		marker := " "
		if m.selectedSlot == slot {
			marker = ">"
		}
		fmt.Fprintf(&output, "%s %d  %s\n", marker, slot, preview(m.slots[slot], m.width-5))
	}

	if m.selectedSlot >= 0 {
		output.WriteString("\n1-0 select   v view   q quit\n")
	} else {
		output.WriteString("\n1-0 select   q quit\n")
	}
	return output.String()
}

func (m Model) renderExpanded() string {
	value := m.slots[m.selectedSlot]
	if value == "" {
		value = "<blank>"
	}

	return fmt.Sprintf("Slot %d\n\n%s\n\n1-0 select   v collapse   q quit\n", m.selectedSlot, value)
}

func preview(value string, maxWidth int) string {
	if value == "" {
		value = "<blank>"
	}

	value = strings.NewReplacer("\r\n", " \u21b5 ", "\n", " \u21b5 ", "\r", " \u21b5 ").Replace(value)
	runes := []rune(value)
	if len(runes) <= maxWidth {
		return value
	}
	if maxWidth <= 3 {
		return string(runes[:max(0, maxWidth)])
	}
	return string(runes[:maxWidth-3]) + "..."
}
