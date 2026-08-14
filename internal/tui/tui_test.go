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

func TestEscapeQuits(t *testing.T) {
	model := New("default", nil)
	_, command := model.handleKey("esc")
	if command == nil {
		t.Fatal("Escape did not return a quit command")
	}
}

func TestModelTogglesExpandedView(t *testing.T) {
	model := New("default", map[int]string{3: "hello\nworld"})
	selected, _ := model.handleKey("3")
	expanded, _ := selected.(Model).handleKey("v")
	view := expanded.(Model).View().Content

	if !strings.Contains(view, "Slot 3\n\nhello\nworld") {
		t.Errorf("expanded view missing full value: %q", view)
	}
	if !strings.Contains(view, "v collapse") {
		t.Errorf("expanded view missing collapse instruction: %q", view)
	}

	collapsed, _ := expanded.(Model).handleKey("v")
	if collapsed.(Model).expanded {
		t.Error("second v keypress did not collapse the view")
	}
}

func TestSelectingDifferentSlotCollapsesView(t *testing.T) {
	model := New("default", nil)
	selected, _ := model.handleKey("3")
	expanded, _ := selected.(Model).handleKey("v")
	updated, _ := expanded.(Model).handleKey("4")
	got := updated.(Model)

	if got.selectedSlot != 4 || got.expanded {
		t.Errorf("selection = (slot %d, expanded %t), want (slot 4, false)", got.selectedSlot, got.expanded)
	}
}

func TestModelUsesArrowKeysForViewState(t *testing.T) {
	model := New("default", nil)
	selected, _ := model.handleKey("3")
	expanded, _ := selected.(Model).handleKey("right")
	if !expanded.(Model).expanded {
		t.Error("right arrow did not expand the selected slot")
	}

	collapsed, _ := expanded.(Model).handleKey("left")
	if collapsed.(Model).expanded {
		t.Error("left arrow did not collapse the selected slot")
	}
}

func TestModelRendersArrowKeyHints(t *testing.T) {
	model := New("default", nil)
	if view := model.View().Content; !strings.Contains(view, "↑/↓ move") {
		t.Errorf("compact view missing arrow hint: %q", view)
	}

	selected, _ := model.handleKey("3")
	expanded, _ := selected.(Model).handleKey("right")
	if view := expanded.(Model).View().Content; !strings.Contains(view, "←/v collapse") {
		t.Errorf("expanded view missing arrow hint: %q", view)
	}
}

func TestModelMovesSelectionWithArrowKeys(t *testing.T) {
	model := New("default", nil)
	updated, _ := model.handleKey("down")
	if got := updated.(Model).selectedSlot; got != 1 {
		t.Errorf("first down selection = %d, want 1", got)
	}

	updated, _ = updated.(Model).handleKey("up")
	if got := updated.(Model).selectedSlot; got != 0 {
		t.Errorf("up selection = %d, want 0", got)
	}

	updated, _ = updated.(Model).handleKey("down")
	if got := updated.(Model).selectedSlot; got != 1 {
		t.Errorf("down selection = %d, want 1", got)
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
