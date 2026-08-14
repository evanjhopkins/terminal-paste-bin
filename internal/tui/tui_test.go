package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestModelSelectsSlotImmediately(t *testing.T) {
	model := New("default", nil)
	updated, _ := model.Update(tea.KeyPressMsg(tea.Key{Code: '3', Text: "3"}))
	selected := updated.(Model)
	if selected.selectedSlot != 3 {
		t.Errorf("selected slot = %d, want 3", selected.selectedSlot)
	}
}

func TestModelRendersOneLinePreviews(t *testing.T) {
	model := New("myapp", map[int]string{3: "hello\nworld"})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 20})
	view := updated.(Model).View().Content

	if !strings.Contains(view, "Terminal Paste Bin - myapp") {
		t.Errorf("view missing bin name: %q", view)
	}
	if !strings.Contains(view, "3  hello \u21b5 world") {
		t.Errorf("view missing multiline preview: %q", view)
	}
	if !strings.Contains(view, "2  <blank>") {
		t.Errorf("view missing blank slot: %q", view)
	}
}

func TestPreviewTruncatesToAvailableWidth(t *testing.T) {
	if got, want := preview("abcdefgh", 6), "abc..."; got != want {
		t.Errorf("preview = %q, want %q", got, want)
	}
	if got, want := preview("", 4), "<..."; got != want {
		t.Errorf("blank preview = %q, want %q", got, want)
	}
}
