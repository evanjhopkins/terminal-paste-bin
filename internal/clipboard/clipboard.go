// Package clipboard provides platform-specific system clipboard access.
package clipboard

import (
	"fmt"
	"os/exec"
)

// Reader reads text from the system clipboard.
type Reader interface {
	Read() (string, error)
}

type commandClipboard struct {
	name string
	args []string
	run  func(string, ...string) ([]byte, error)
}

func newCommandClipboard(name string, args ...string) Reader {
	return commandClipboard{name: name, args: args, run: runCommand}
}

func (c commandClipboard) Read() (string, error) {
	contents, err := c.run(c.name, c.args...)
	if err != nil {
		return "", fmt.Errorf("read clipboard with %s: %w", c.name, err)
	}
	return string(contents), nil
}

type unavailableReader struct {
	reason string
}

func (r unavailableReader) Read() (string, error) {
	return "", fmt.Errorf("clipboard unavailable: %s", r.reason)
}

func runCommand(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}
