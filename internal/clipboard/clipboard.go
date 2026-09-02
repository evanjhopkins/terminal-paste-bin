// Package clipboard provides platform-specific system clipboard access.
package clipboard

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// Reader reads text from the system clipboard.
type Reader interface {
	Read() (string, error)
}

// Writer writes text to the system clipboard.
type Writer interface {
	Write(value string) error
}

// Clipboard provides bidirectional system clipboard access.
type Clipboard interface {
	Reader
	Writer
}

// Status reports whether a capability is available.
type Status int

const (
	// StatusOK indicates the capability is available.
	StatusOK Status = iota
	// StatusUnavailable indicates the capability is not available.
	StatusUnavailable
)

// Diagnostic describes the availability of a TPB capability.
type Diagnostic struct {
	Status  Status
	Backend string // name of the working backend, empty when unavailable
}

type commandClipboard struct {
	readName  string
	readArgs  []string
	writeName string
	writeArgs []string
	runOutput func(string, ...string) ([]byte, error)
	runInput  func(string, []string, io.Reader) error
}

func newCommandClipboard(readName string, readArgs []string, writeName string, writeArgs []string) Clipboard {
	return commandClipboard{
		readName:  readName,
		readArgs:  readArgs,
		writeName: writeName,
		writeArgs: writeArgs,
		runOutput: runCommandOutput,
		runInput:  runCommandInput,
	}
}

func (c commandClipboard) Read() (string, error) {
	contents, err := c.runOutput(c.readName, c.readArgs...)
	if err != nil {
		return "", fmt.Errorf("read clipboard with %s: %w", c.readName, err)
	}
	return string(contents), nil
}

func (c commandClipboard) Write(value string) error {
	if err := c.runInput(c.writeName, c.writeArgs, strings.NewReader(value)); err != nil {
		return fmt.Errorf("write clipboard with %s: %w", c.writeName, err)
	}
	return nil
}

type unavailableClipboard struct {
	reason string
}

func (r unavailableClipboard) Read() (string, error) {
	return "", fmt.Errorf("clipboard unavailable: %s", r.reason)
}

func (r unavailableClipboard) Write(string) error {
	return fmt.Errorf("clipboard unavailable: %s", r.reason)
}

func runCommandOutput(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

func runCommandInput(name string, args []string, input io.Reader) error {
	command := exec.Command(name, args...)
	command.Stdin = input
	return command.Run()
}
