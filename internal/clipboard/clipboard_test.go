package clipboard

import (
	"errors"
	"strings"
	"testing"
)

func TestCommandClipboardPreservesContents(t *testing.T) {
	reader := commandClipboard{
		name: "fake-clipboard",
		run: func(string, ...string) ([]byte, error) {
			return []byte("hello\n\u4e16\u754c\n"), nil
		},
	}

	contents, err := reader.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got, want := contents, "hello\n\u4e16\u754c\n"; got != want {
		t.Errorf("contents = %q, want %q", got, want)
	}
}

func TestCommandClipboardWrapsErrors(t *testing.T) {
	reader := commandClipboard{
		name: "fake-clipboard",
		run: func(string, ...string) ([]byte, error) {
			return nil, errors.New("not available")
		},
	}

	_, err := reader.Read()
	if err == nil || !strings.Contains(err.Error(), "fake-clipboard") {
		t.Errorf("Read error = %v, want command context", err)
	}
}
