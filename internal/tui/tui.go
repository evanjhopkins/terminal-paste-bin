// Package tui renders and handles input for the interactive TPB interface.
package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

const defaultWidth = 80

var slotOrder = [10]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 0}

// Model is the interactive state for a single TPB bin.
type Model struct {
	binName      string
	slots        map[int]string
	selectedSlot int
	expanded     bool
	status       string
	actions      Actions
	width        int
}

// Actions contains operations the TUI can request from the application layer.
type Actions struct {
	DeleteSlot func(slot int) error
	WriteSlot  func(slot int) error
}

// New creates a compact slot-selection view for binName.
func New(binName string, slots map[int]string) Model {
	return NewWithActions(binName, slots, Actions{})
}

// NewWithActions creates a compact slot-selection view with application actions.
func NewWithActions(binName string, slots map[int]string, actions Actions) Model {
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
		actions:      actions,
		width:        defaultWidth,
	}
}

// Run starts the interactive TUI for a bin.
func Run(binName string, slots map[int]string, actions Actions) error {
	_, err := tea.NewProgram(NewWithActions(binName, slots, actions)).Run()
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
	if key == "q" || key == "esc" {
		return m, tea.Quit
	}
	if m.selectedSlot >= 0 {
		if key == "d" {
			return m.deleteSelectedSlot()
		}
		if key == "w" {
			return m.writeSelectedSlot()
		}
		if !m.expanded && (key == "v" || key == "right") {
			m.expanded = true
			return m, nil
		}
		if m.expanded && (key == "v" || key == "left") {
			m.expanded = false
			return m, nil
		}
	}
	if !m.expanded && (key == "up" || key == "down") {
		m.moveSelection(key)
		return m, nil
	}
	if len(key) == 1 && key[0] >= '0' && key[0] <= '9' {
		m.selectSlot(int(key[0] - '0'))
	}
	return m, nil
}

func (m Model) deleteSelectedSlot() (tea.Model, tea.Cmd) {
	if m.actions.DeleteSlot == nil {
		return m, nil
	}
	if err := m.actions.DeleteSlot(m.selectedSlot); err != nil {
		m.expanded = false
		m.status = fmt.Sprintf("Error deleting slot %d: %v", m.selectedSlot, err)
		return m, nil
	}

	delete(m.slots, m.selectedSlot)
	m.expanded = false
	m.status = fmt.Sprintf("Slot %d cleared.", m.selectedSlot)
	return m, nil
}

func (m Model) writeSelectedSlot() (tea.Model, tea.Cmd) {
	if m.actions.WriteSlot == nil {
		return m, nil
	}
	if err := m.actions.WriteSlot(m.selectedSlot); err != nil {
		m.expanded = false
		m.status = fmt.Sprintf("Error writing slot %d: %v", m.selectedSlot, err)
		return m, nil
	}
	return m, tea.Quit
}

func (m *Model) moveSelection(direction string) {
	if m.selectedSlot < 0 {
		m.selectSlot(slotOrder[0])
		return
	}

	for index, slot := range slotOrder {
		if slot != m.selectedSlot {
			continue
		}
		if direction == "up" {
			m.selectSlot(slotOrder[(index+len(slotOrder)-1)%len(slotOrder)])
			return
		}
		m.selectSlot(slotOrder[(index+1)%len(slotOrder)])
		return
	}
}

func (m *Model) selectSlot(slot int) {
	m.selectedSlot = slot
	m.expanded = false
	m.status = ""
}

func (m Model) render() string {
	if m.expanded {
		return m.renderExpanded()
	}

	var output strings.Builder
	fmt.Fprintf(&output, "Terminal Paste Bin - %s\n\n", m.binName)

	for _, slot := range slotOrder {
		marker := " "
		if m.selectedSlot == slot {
			marker = ">"
		}
		fmt.Fprintf(&output, "%s %d  %s\n", marker, slot, preview(m.slots[slot], m.width-5))
	}
	if m.status != "" {
		fmt.Fprintf(&output, "\n%s\n", m.status)
	}

	if m.selectedSlot >= 0 {
		output.WriteString("\n↑/↓ move   1-0 select   →/v view   w write   d delete   q quit\n")
	} else {
		output.WriteString("\n↑/↓ move   1-0 select   q quit\n")
	}
	return output.String()
}

func (m Model) renderExpanded() string {
	value := m.slots[m.selectedSlot]
	if value == "" {
		value = "<blank>"
	}

	return fmt.Sprintf("Slot %d\n\n%s\n\n1-0 select   ←/v collapse   w write   d delete   q quit\n", m.selectedSlot, value)
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
