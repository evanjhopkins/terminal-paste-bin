package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/evanjhopkins/terminal-paste-bin/internal/clipboard"
	"github.com/evanjhopkins/terminal-paste-bin/internal/store"
	"github.com/evanjhopkins/terminal-paste-bin/internal/tui"
)

const helpText = `Terminal Paste Bin

Usage:
  tpb [bin-name]
  tpb list
  tpb reset
  tpb doctor

Options:
  -h, --help      Show help
  -v, --version   Show version
`

// version is set at build time for release builds.
var version = "devel"

const (
	ansiRed   = "\x1b[31m"
	ansiGreen = "\x1b[32m"
	ansiReset = "\x1b[0m"
)

func main() {
	var launch *binLaunch
	var paths store.Paths
	var err error
	if isStandaloneInvocation(os.Args[1:]) {
		launch, err = run(os.Args[1:], paths, os.Stdout)
	} else {
		paths, err = store.DefaultPaths(os.Args[0])
		if err == nil {
			launch, err = run(os.Args[1:], paths, os.Stdout)
		}
	}
	if err == nil && launch != nil {
		systemClipboard := clipboard.New()
		result, runErr := tui.Run(launch.name, launch.directory, launch.slots, tui.Actions{
			DeleteSlot: func(slot int) error {
				return deleteBinSlot(paths, launch.id, slot)
			},
			WriteSlot: func(slot int) error {
				return writeClipboardToSlot(paths, launch.id, slot, systemClipboard)
			},
			CopySlot: func(slot int) (bool, error) {
				return copySlotToClipboard(paths, launch.id, slot, systemClipboard)
			},
		})
		err = runErr
		if err == nil && result.Execute {
			err = executeCommand(result.Command, launch.directory)
		}
	}
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			os.Exit(exitError.ExitCode())
		}
		if !errors.Is(err, errDoctorFailed) {
			fmt.Fprintln(os.Stderr, "tpb:", err)
		}
		os.Exit(1)
	}
}

func executeCommand(command, directory string) error {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	execution := exec.Command(shell, "-c", command)
	execution.Dir = directory
	execution.Stdin = os.Stdin
	execution.Stdout = os.Stdout
	execution.Stderr = os.Stderr
	return execution.Run()
}

type binLaunch struct {
	id        string
	name      string
	directory string
	slots     map[int]string
}

func run(args []string, paths store.Paths, output io.Writer) (*binLaunch, error) {
	return runInDirectory(args, paths, output, "")
}

func runInDirectory(args []string, paths store.Paths, output io.Writer, directory string) (*binLaunch, error) {
	if isStandaloneInvocation(args) {
		switch args[0] {
		case "-h", "--help":
			if _, err := fmt.Fprint(output, helpText); err != nil {
				return nil, fmt.Errorf("write help: %w", err)
			}
		case "-v", "--version":
			if _, err := fmt.Fprintf(output, "tpb %s\n", version); err != nil {
				return nil, fmt.Errorf("write version: %w", err)
			}
		case "doctor":
			return runDoctor(output, clipboard.Diagnose)
		}
		return nil, nil
	}

	if len(args) == 0 {
		return loadDirectoryBinAt(directory, paths)
	}
	if len(args) != 1 {
		return nil, fmt.Errorf("usage: tpb [bin-name | list | reset]")
	}

	switch args[0] {
	case "list":
		bins, err := store.Open(paths)
		if err != nil {
			return nil, err
		}
		for _, name := range bins.ListBins() {
			if name.IsDirectory() {
				if _, err := fmt.Fprintln(output, "(dir) "+name.Directory); err != nil {
					return nil, fmt.Errorf("write bin list: %w", err)
				}
				continue
			}
			if _, err := fmt.Fprintln(output, name.Name); err != nil {
				return nil, fmt.Errorf("write bin list: %w", err)
			}
		}
		return nil, nil
	case "reset":
		if err := store.Reset(paths); err != nil {
			return nil, err
		}
		if _, err := fmt.Fprintln(output, "Reset complete."); err != nil {
			return nil, fmt.Errorf("write reset result: %w", err)
		}
	default:
		return loadNamedBin(args[0], paths)
	}
	return nil, nil
}

// loadDirectoryBinAt resolves the supplied directory (falling back to the
// current working directory) and opens the bin scoped to it.
func loadDirectoryBinAt(directory string, paths store.Paths) (*binLaunch, error) {
	if directory == "" {
		var err error
		directory, err = currentDirectory()
		if err != nil {
			return nil, err
		}
	} else {
		var err error
		directory, err = canonicalDirectory(directory)
		if err != nil {
			return nil, err
		}
	}
	return loadDirectoryBin(directory, paths)
}

func isStandaloneInvocation(args []string) bool {
	if len(args) != 1 {
		return false
	}
	switch args[0] {
	case "-h", "--help", "-v", "--version", "doctor":
		return true
	default:
		return false
	}
}

// errDoctorFailed signals that doctor checks failed. The report, including
// the failure summary, has already been printed.
var errDoctorFailed = errors.New("doctor checks failed")

// runDoctor prints the result of TPB's diagnostic checks.
func runDoctor(output io.Writer, diagnose func() clipboard.Diagnostic) (*binLaunch, error) {
	diagnostic := diagnose()
	status := "OK"
	failures := 0
	if diagnostic.Status == clipboard.StatusUnavailable {
		status = "FAIL"
		failures++
	}
	line := "Clipboard access: " + status
	if diagnostic.Backend != "" {
		line += " (" + diagnostic.Backend + ")"
	}
	if terminalOutput(output) {
		line = colorize(line, diagnostic.Status)
	}
	if _, err := fmt.Fprintln(output, line); err != nil {
		return nil, fmt.Errorf("write doctor output: %w", err)
	}
	if failures > 0 {
		if _, err := fmt.Fprintln(output); err != nil {
			return nil, fmt.Errorf("write doctor output: %w", err)
		}
		summary := fmt.Sprintf("%d check(s) failed", failures)
		if terminalOutput(output) {
			summary = colorize(summary, diagnostic.Status)
		}
		if _, err := fmt.Fprintln(output, summary); err != nil {
			return nil, fmt.Errorf("write doctor output: %w", err)
		}
		return nil, errDoctorFailed
	}
	return nil, nil
}

// colorize wraps text in the ANSI color matching the diagnostic status.
func colorize(text string, status clipboard.Status) string {
	color := ansiGreen
	if status == clipboard.StatusUnavailable {
		color = ansiRed
	}
	return color + text + ansiReset
}

// terminalOutput reports whether the writer is an interactive terminal and
// colors should be emitted.
func terminalOutput(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func currentDirectory() (string, error) {
	directory, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get current directory: %w", err)
	}
	return canonicalDirectory(directory)
}

func canonicalDirectory(directory string) (string, error) {
	absPath, err := filepath.Abs(directory)
	if err != nil {
		return "", fmt.Errorf("resolve current directory: %w", err)
	}
	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", fmt.Errorf("resolve current directory symlinks: %w", err)
	}
	info, err := os.Stat(resolvedPath)
	if err != nil {
		return "", fmt.Errorf("stat current directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("current path is not a directory: %s", resolvedPath)
	}
	return resolvedPath, nil
}

func loadNamedBin(name string, paths store.Paths) (*binLaunch, error) {
	if err := store.ValidateBinName(name); err != nil {
		return nil, err
	}

	bins, err := store.Open(paths)
	if err != nil {
		return nil, err
	}
	if err := bins.EnsureBin(name); err != nil {
		return nil, err
	}
	return loadBin(store.BinInfo{ID: name, Name: name}, bins)
}

func loadDirectoryBin(directory string, paths store.Paths) (*binLaunch, error) {
	bins, err := store.Open(paths)
	if err != nil {
		return nil, err
	}
	info, err := bins.EnsureDirectoryBin(directory)
	if err != nil {
		return nil, err
	}
	info.Name = "current directory"
	return loadBin(info, bins)
}

func loadBin(info store.BinInfo, bins *store.Store) (*binLaunch, error) {
	slots := make(map[int]string, 10)
	for slot := 0; slot <= 9; slot++ {
		value, exists, err := bins.ReadSlot(info.ID, slot)
		if err != nil {
			return nil, err
		}
		if exists {
			slots[slot] = value
		}
	}
	return &binLaunch{id: info.ID, name: info.Name, directory: info.Directory, slots: slots}, nil
}

func deleteBinSlot(paths store.Paths, binName string, slot int) error {
	bins, err := store.Open(paths)
	if err != nil {
		return err
	}
	return bins.DeleteSlot(binName, slot)
}

func writeClipboardToSlot(paths store.Paths, binName string, slot int, reader clipboard.Reader) error {
	value, err := reader.Read()
	if err != nil {
		return err
	}

	bins, err := store.Open(paths)
	if err != nil {
		return err
	}
	return bins.WriteSlot(binName, slot, value)
}

func copySlotToClipboard(paths store.Paths, binName string, slot int, writer clipboard.Writer) (bool, error) {
	bins, err := store.Open(paths)
	if err != nil {
		return false, err
	}
	value, exists, err := bins.ReadSlot(binName, slot)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}
	if err := writer.Write(value); err != nil {
		return false, err
	}
	return true, nil
}
