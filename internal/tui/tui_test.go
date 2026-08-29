package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/mattn/go-runewidth"
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

func TestModelDeletesSelectedSlot(t *testing.T) {
	deletedSlot := -1
	model := NewWithActions("default", map[int]string{3: "value"}, Actions{
		DeleteSlot: func(slot int) error {
			deletedSlot = slot
			return nil
		},
	})
	selected, _ := model.handleKey("3")
	updated, _ := selected.(Model).handleKey("d")
	got := updated.(Model)

	if deletedSlot != 3 {
		t.Errorf("deleted slot = %d, want 3", deletedSlot)
	}
	if _, exists := got.slots[3]; exists {
		t.Error("deleted slot still has an in-memory value")
	}
	if got.expanded || got.status != "Slot 3 cleared." {
		t.Errorf("delete state = (expanded %t, status %q)", got.expanded, got.status)
	}
}

func TestModelRetainsSlotWhenDeleteFails(t *testing.T) {
	model := NewWithActions("default", map[int]string{3: "value"}, Actions{
		DeleteSlot: func(int) error { return errors.New("disk unavailable") },
	})
	selected, _ := model.handleKey("3")
	updated, _ := selected.(Model).handleKey("d")
	got := updated.(Model)

	if got.slots[3] != "value" {
		t.Error("failed delete removed the slot value")
	}
	if !strings.Contains(got.status, "disk unavailable") {
		t.Errorf("delete error status = %q", got.status)
	}
}

func TestModelWritesSelectedSlotAndQuits(t *testing.T) {
	writtenSlot := -1
	model := NewWithActions("default", nil, Actions{
		WriteSlot: func(slot int) error {
			writtenSlot = slot
			return nil
		},
	})
	selected, _ := model.handleKey("3")
	_, command := selected.(Model).handleKey("w")

	if writtenSlot != 3 {
		t.Errorf("written slot = %d, want 3", writtenSlot)
	}
	if command == nil {
		t.Fatal("successful write did not return a quit command")
	}
}

func TestModelStaysOpenWhenWriteFails(t *testing.T) {
	model := NewWithActions("default", nil, Actions{
		WriteSlot: func(int) error { return errors.New("clipboard unavailable") },
	})
	selected, _ := model.handleKey("3")
	updated, command := selected.(Model).handleKey("w")
	got := updated.(Model)

	if command != nil {
		t.Error("failed write returned a quit command")
	}
	if !strings.Contains(got.status, "clipboard unavailable") {
		t.Errorf("write error status = %q", got.status)
	}
	if got.expanded {
		t.Error("failed write remained in the expanded view")
	}
}

func TestActionFailuresReturnToVisibleCompactView(t *testing.T) {
	actions := Actions{
		DeleteSlot: func(int) error { return errors.New("delete failed") },
		WriteSlot:  func(int) error { return errors.New("write failed") },
		CopySlot:   func(int) (bool, error) { return false, errors.New("read failed") },
	}
	for _, key := range []string{"d", "w", "r"} {
		t.Run(key, func(t *testing.T) {
			model := NewWithActions("default", map[int]string{3: "value"}, actions)
			selected, _ := model.handleKey("3")
			expanded, _ := selected.(Model).handleKey("v")
			updated, command := expanded.(Model).handleKey(key)
			got := updated.(Model)

			if command != nil {
				t.Error("failed action returned a quit command")
			}
			if got.expanded {
				t.Error("failed action remained in the expanded view")
			}
			if !strings.Contains(got.View().Content, "Error") {
				t.Errorf("compact view did not render the error status: %q", got.View().Content)
			}
		})
	}
}

func TestModelCopiesSelectedSlotAndQuits(t *testing.T) {
	copiedSlot := -1
	model := NewWithActions("default", nil, Actions{
		CopySlot: func(slot int) (bool, error) {
			copiedSlot = slot
			return true, nil
		},
	})
	selected, _ := model.handleKey("3")
	_, command := selected.(Model).handleKey("r")

	if copiedSlot != 3 {
		t.Errorf("copied slot = %d, want 3", copiedSlot)
	}
	if command == nil {
		t.Fatal("successful copy did not return a quit command")
	}
}

func TestModelLeavesClipboardUntouchedForBlankSlot(t *testing.T) {
	copyCalls := 0
	model := NewWithActions("default", nil, Actions{
		CopySlot: func(int) (bool, error) {
			copyCalls++
			return false, nil
		},
	})
	selected, _ := model.handleKey("3")
	updated, command := selected.(Model).handleKey("r")
	got := updated.(Model)

	if copyCalls != 1 {
		t.Errorf("copy calls = %d, want 1", copyCalls)
	}
	if command != nil {
		t.Error("blank slot returned a quit command")
	}
	if got.status != "Slot 3 is blank." {
		t.Errorf("blank slot status = %q", got.status)
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

func TestModelRendersTruncatedDirectoryContext(t *testing.T) {
	directory := "/Users/evanhopkins/code/projects/terminal-paste-bin"
	model := NewWithDetails("current directory", directory, nil, Actions{})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 28})
	view := updated.(Model).View().Content

	if !strings.Contains(view, "Terminal Paste Bin - current directory") {
		t.Errorf("directory view missing label: %q", view)
	}
	pathPreview := truncatePath(directory, 28)
	if !strings.Contains(view, pathPreview) {
		t.Errorf("directory view missing truncated path: %q", view)
	}

	selected, _ := updated.(Model).handleKey("3")
	expanded, _ := selected.(Model).handleKey("v")
	if view := expanded.(Model).View().Content; !strings.Contains(view, pathPreview) {
		t.Errorf("expanded directory view missing path: %q", view)
	}
}

func TestTruncatePathPreservesDirectoryTail(t *testing.T) {
	path := "/Users/evanhopkins/code/terminal-paste-bin"
	got := truncatePath(path, 24)
	if !strings.HasPrefix(got, "...") || !strings.HasSuffix(got, "/terminal-paste-bin") {
		t.Errorf("truncatePath = %q, want an ellipsized path ending in the directory name", got)
	}
	if width := runewidth.StringWidth(got); width > 24 {
		t.Errorf("truncatePath width = %d, exceeds 24", width)
	}
}

func TestModelHighlightsOnlyTheSelectedRow(t *testing.T) {
	model := New("default", map[int]string{3: "value"})
	selected, _ := model.handleKey("3")
	view := selected.(Model).View().Content

	if !strings.Contains(view, selectedRowStart+"> 3  value"+selectedRowEnd) {
		t.Errorf("selected row was not color highlighted: %q", view)
	}
	if strings.Contains(view, selectedRowStart+"  2") {
		t.Errorf("unselected row was highlighted: %q", view)
	}
}

func TestPreviewTruncatesToAvailableWidth(t *testing.T) {
	if got, want := preview("abcdefgh", 6), "abc..."; got != want {
		t.Errorf("preview = %q, want %q", got, want)
	}
	if got, want := preview("", 4), "<..."; got != want {
		t.Errorf("blank preview = %q, want %q", got, want)
	}
	if got, want := preview("\u4e16\u754cabc", 5), "\u4e16..."; got != want {
		t.Errorf("wide preview = %q, want %q", got, want)
	}
	if got, want := preview("e\u0301clair", 4), "e\u0301..."; got != want {
		t.Errorf("combining preview = %q, want %q", got, want)
	}
	if got := preview("value", 0); got != "" {
		t.Errorf("zero-width preview = %q, want empty", got)
	}

	for _, test := range []struct {
		value string
		width int
	}{
		{value: preview("\u4e16\u754cabc", 5), width: 5},
		{value: preview("e\u0301clair", 4), width: 4},
	} {
		if got := runewidth.StringWidth(test.value); got > test.width {
			t.Errorf("preview width = %d, exceeds %d for %q", got, test.width, test.value)
		}
	}
}
