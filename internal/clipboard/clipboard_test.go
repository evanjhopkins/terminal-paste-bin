package clipboard

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func TestCommandClipboardPreservesContents(t *testing.T) {
	reader := commandClipboard{
		readName: "fake-clipboard",
		runOutput: func(string, ...string) ([]byte, error) {
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
		readName: "fake-clipboard",
		runOutput: func(string, ...string) ([]byte, error) {
			return nil, errors.New("not available")
		},
	}

	_, err := reader.Read()
	if err == nil || !strings.Contains(err.Error(), "fake-clipboard") {
		t.Errorf("Read error = %v, want command context", err)
	}
}

func TestCommandClipboardWritesContentsUnchanged(t *testing.T) {
	var written string
	writer := commandClipboard{
		writeName: "fake-clipboard",
		runInput: func(_ string, _ []string, input io.Reader) error {
			contents, err := io.ReadAll(input)
			if err != nil {
				return err
			}
			written = string(contents)
			return nil
		},
	}

	want := "hello\n\u4e16\u754c\n"
	if err := writer.Write(want); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if written != want {
		t.Errorf("written value = %q, want %q", written, want)
	}
}

func TestCommandClipboardWriteWrapsErrors(t *testing.T) {
	writer := commandClipboard{
		writeName: "fake-clipboard",
		runInput: func(string, []string, io.Reader) error {
			return errors.New("not available")
		},
	}

	err := writer.Write("value")
	if err == nil || !strings.Contains(err.Error(), "fake-clipboard") {
		t.Errorf("Write error = %v, want command context", err)
	}
}

func TestUnavailableClipboardReturnsActionableErrors(t *testing.T) {
	clipboard := unavailableClipboard{reason: "install a clipboard command"}
	for _, operation := range []func() error{
		func() error {
			_, err := clipboard.Read()
			return err
		},
		func() error { return clipboard.Write("value") },
	} {
		err := operation()
		if err == nil || !strings.Contains(err.Error(), "install a clipboard command") {
			t.Errorf("clipboard error = %v, want actionable guidance", err)
		}
	}
}
