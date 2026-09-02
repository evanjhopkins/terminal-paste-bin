package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/evanjhopkins/terminal-paste-bin/internal/clipboard"
	"github.com/evanjhopkins/terminal-paste-bin/internal/store"
	"github.com/evanjhopkins/terminal-paste-bin/internal/tui"
)

const helpText = `Terminal Paste Bin

Usage:
  tpb [bin-name]
  tpb list
  tpb delete [--yes] <bin>
  tpb rename <old> <new>
  tpb prune [--dry-run]
  tpb reset
  tpb doctor
  tpb search <query>

Commands:
  list      List named and directory bins
  delete    Permanently remove a named bin and its slots (asks first; --yes/-y skips)
  rename    Rename a named bin, keeping its slots
  prune     Remove directory bins whose directory no longer exists (--dry-run to preview)
  reset     Remove all stored data without confirmation
  doctor    Check clipboard access and report stale directory bins
  search    Search every bin's slots for text (case-insensitive substring)

Options:
  -h, --help      Show help
  -v, --version   Show version
`

const usageText = "usage: tpb [bin-name | list | delete | rename | prune | reset | doctor | search]"

// version is set at build time for release builds.
var version = "devel"

const (
	ansiRed    = "\x1b[31m"
	ansiGreen  = "\x1b[32m"
	ansiYellow = "\x1b[33m"
	ansiReset  = "\x1b[0m"
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
		if errors.Is(err, errSearchNoMatch) {
			os.Exit(searchNoMatchExitCode)
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

// session carries the I/O and environment a command invocation runs against.
// Tests construct sessions directly to simulate terminals and prompt answers.
type session struct {
	paths  store.Paths
	output io.Writer
	// input supplies answers to confirmation prompts.
	input io.Reader
	// interactive reports whether both input and output are attached to a
	// terminal, so confirmation prompts can be shown and answered.
	interactive bool
	// directory overrides the working directory used for directory bins; the
	// process working directory is used when empty.
	directory string
}

func run(args []string, paths store.Paths, output io.Writer) (*binLaunch, error) {
	return runInDirectory(args, paths, output, "")
}

func runInDirectory(args []string, paths store.Paths, output io.Writer, directory string) (*binLaunch, error) {
	return runSession(args, session{
		paths:       paths,
		output:      output,
		input:       os.Stdin,
		interactive: isTerminal(os.Stdin) && isTerminal(output),
		directory:   directory,
	})
}

func runSession(args []string, s session) (*binLaunch, error) {
	if isStandaloneInvocation(args) {
		switch args[0] {
		case "-h", "--help":
			if _, err := fmt.Fprint(s.output, helpText); err != nil {
				return nil, fmt.Errorf("write help: %w", err)
			}
		case "-v", "--version":
			if _, err := fmt.Fprintf(s.output, "tpb %s\n", version); err != nil {
				return nil, fmt.Errorf("write version: %w", err)
			}
		}
		return nil, nil
	}

	if len(args) == 0 {
		return loadDirectoryBinAt(s.directory, s.paths)
	}

	switch args[0] {
	case "list":
		if len(args) != 1 {
			return nil, errors.New(usageText)
		}
		return nil, runList(s)
	case "reset":
		if len(args) != 1 {
			return nil, errors.New(usageText)
		}
		if err := store.Reset(s.paths); err != nil {
			return nil, err
		}
		if _, err := fmt.Fprintln(s.output, "Reset complete."); err != nil {
			return nil, fmt.Errorf("write reset result: %w", err)
		}
		return nil, nil
	case "doctor":
		if len(args) != 1 {
			return nil, errors.New(usageText)
		}
		return runDoctor(s.paths, s.output, clipboard.Diagnose)
	case "delete":
		return nil, runDelete(args[1:], s)
	case "rename":
		return nil, runRename(args[1:], s)
	case "prune":
		return nil, runPrune(args[1:], s)
	case "search":
		return nil, runSearch(args[1:], s)
	default:
		if len(args) != 1 {
			return nil, errors.New(usageText)
		}
		return loadNamedBin(args[0], s.paths)
	}
}

func runList(s session) error {
	bins, err := store.Open(s.paths)
	if err != nil {
		return err
	}
	for _, name := range bins.ListBins() {
		if name.IsDirectory() {
			if _, err := fmt.Fprintln(s.output, "(dir) "+name.Directory); err != nil {
				return fmt.Errorf("write bin list: %w", err)
			}
			continue
		}
		if _, err := fmt.Fprintln(s.output, name.Name); err != nil {
			return fmt.Errorf("write bin list: %w", err)
		}
	}
	return nil
}

// runDelete implements `tpb delete [--yes|-y] <bin>`. Deletion is
// irreversible, so it prompts when attached to a terminal and refuses outright
// when it is not, unless the skip flag is supplied.
func runDelete(args []string, s session) error {
	skipPrompt := false
	var name string
	for _, arg := range args {
		switch {
		case arg == "--yes" || arg == "-y":
			skipPrompt = true
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown flag %q\nusage: tpb delete [--yes] <bin>", arg)
		case name == "":
			name = arg
		default:
			return errors.New("usage: tpb delete [--yes] <bin>")
		}
	}
	if name == "" {
		return errors.New("usage: tpb delete [--yes] <bin>")
	}

	bins, err := store.Open(s.paths)
	if err != nil {
		return err
	}
	info, exists := bins.Lookup(name)
	if !exists {
		return fmt.Errorf("%w: %s", store.ErrBinNotFound, name)
	}
	if info.IsDirectory() {
		return fmt.Errorf("%w: %s (use 'tpb prune' to remove stale directory bins)", store.ErrDirectoryBin, name)
	}

	if !skipPrompt {
		if !s.interactive {
			return fmt.Errorf("refusing to delete bin %q without confirmation: pass --yes when not attached to a terminal", name)
		}
		count, err := nonBlankSlotCount(bins, name)
		if err != nil {
			return err
		}
		prompt := fmt.Sprintf("Delete bin %q and its %d non-blank slot(s)? This cannot be undone. [y/N] ", name, count)
		confirmed, err := confirm(s, prompt)
		if err != nil {
			return err
		}
		if !confirmed {
			return errors.New("deletion cancelled")
		}
	}

	if err := bins.DeleteBin(name); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(s.output, "Deleted bin %q.\n", name); err != nil {
		return fmt.Errorf("write delete result: %w", err)
	}
	return nil
}

// runRename implements `tpb rename <old> <new>`.
func runRename(args []string, s session) error {
	if len(args) != 2 {
		return errors.New("usage: tpb rename <old> <new>")
	}
	oldName, newName := args[0], args[1]

	bins, err := store.Open(s.paths)
	if err != nil {
		return err
	}
	if err := bins.RenameBin(oldName, newName); err != nil {
		if errors.Is(err, store.ErrBinNotFound) && filepath.IsAbs(oldName) {
			return fmt.Errorf("%w (directory bins are keyed by path and cannot be renamed)", err)
		}
		return err
	}
	if _, err := fmt.Fprintf(s.output, "Renamed bin %q to %q.\n", oldName, newName); err != nil {
		return fmt.Errorf("write rename result: %w", err)
	}
	return nil
}

// runPrune implements `tpb prune [--dry-run]`.
func runPrune(args []string, s session) error {
	dryRun := false
	for _, arg := range args {
		if arg == "--dry-run" || arg == "-n" {
			dryRun = true
			continue
		}
		return fmt.Errorf("unexpected argument %q\nusage: tpb prune [--dry-run]", arg)
	}

	bins, err := store.Open(s.paths)
	if err != nil {
		return err
	}

	var pruned []store.BinInfo
	verb := "Pruned"
	if dryRun {
		pruned = bins.StaleDirectoryBins()
		verb = "Would prune"
	} else {
		pruned, err = bins.PruneDirectoryBins()
		if err != nil {
			return err
		}
	}

	if len(pruned) == 0 {
		if _, err := fmt.Fprintln(s.output, "Nothing to prune."); err != nil {
			return fmt.Errorf("write prune result: %w", err)
		}
		return nil
	}
	for _, info := range pruned {
		if _, err := fmt.Fprintf(s.output, "%s (dir) %s\n", verb, info.Directory); err != nil {
			return fmt.Errorf("write prune result: %w", err)
		}
	}
	return nil
}

// searchNoMatchExitCode is the process exit code for a search that completed
// successfully but found no matches. It is distinct from the generic error
// exit code so scripts can distinguish "nothing matched" from "failed".
const searchNoMatchExitCode = 2

// errSearchNoMatch signals a completed search with no matches. The command
// prints nothing (or, in interactive use, a short note) and exits with
// searchNoMatchExitCode.
var errSearchNoMatch = errors.New("search: no matches")

// searchPreviewWidth bounds the preview column of a search result line so
// matches stay on one line without depending on terminal width.
const searchPreviewWidth = 80

// runSearch implements `tpb search <query>`.
func runSearch(args []string, s session) error {
	if len(args) != 1 {
		return errors.New("usage: tpb search <query>")
	}
	query := strings.TrimSpace(args[0])
	if query == "" {
		return errors.New("usage: tpb search <query>")
	}
	needle := strings.ToLower(query)

	bins, err := store.Open(s.paths)
	if err != nil {
		return err
	}

	found := false
	for _, info := range bins.ListBins() {
		label := info.Name
		if info.IsDirectory() {
			label = "(dir) " + info.Directory
		}
		for slot := 0; slot <= 9; slot++ {
			value, exists, err := bins.ReadSlot(info.ID, slot)
			if err != nil {
				return err
			}
			if !exists || value == "" {
				continue
			}
			if !strings.Contains(strings.ToLower(value), needle) {
				continue
			}
			found = true
			if _, err := fmt.Fprintf(s.output, "%s\t%d\t%s\n", label, slot, tui.Preview(value, searchPreviewWidth)); err != nil {
				return fmt.Errorf("write search result: %w", err)
			}
		}
	}
	if !found {
		return errSearchNoMatch
	}
	return nil
}

// nonBlankSlotCount counts the slots in a bin that hold non-empty text.
func nonBlankSlotCount(bins *store.Store, binName string) (int, error) {
	count := 0
	for slot := 0; slot <= 9; slot++ {
		value, exists, err := bins.ReadSlot(binName, slot)
		if err != nil {
			return 0, err
		}
		if exists && value != "" {
			count++
		}
	}
	return count, nil
}

// confirm prints a prompt and reads one line of input, accepting "y" or "yes"
// (case-insensitive) as confirmation. End of input counts as a refusal.
func confirm(s session, prompt string) (bool, error) {
	if _, err := fmt.Fprint(s.output, prompt); err != nil {
		return false, fmt.Errorf("write confirmation prompt: %w", err)
	}
	answer, err := bufio.NewReader(s.input).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("read confirmation: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
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

// isStandaloneInvocation reports whether args is an informational flag that
// must never touch storage.
func isStandaloneInvocation(args []string) bool {
	if len(args) != 1 {
		return false
	}
	switch args[0] {
	case "-h", "--help", "-v", "--version":
		return true
	default:
		return false
	}
}

// errDoctorFailed signals that doctor checks failed. The report, including
// the failure summary, has already been printed.
var errDoctorFailed = errors.New("doctor checks failed")

// checkStatus is the outcome of one doctor check.
type checkStatus int

const (
	checkOK checkStatus = iota
	// checkWarn flags something worth attention that does not fail doctor.
	checkWarn
	checkFail
)

func (s checkStatus) label() string {
	switch s {
	case checkWarn:
		return "WARN"
	case checkFail:
		return "FAIL"
	default:
		return "OK"
	}
}

type checkResult struct {
	name   string
	status checkStatus
	detail string
}

func (c checkResult) String() string {
	line := c.name + ": " + c.status.label()
	if c.detail != "" {
		line += " (" + c.detail + ")"
	}
	return line
}

// runDoctor prints the result of TPB's diagnostic checks. Failures make the
// command exit non-zero; warnings are reported but do not.
func runDoctor(paths store.Paths, output io.Writer, diagnose func() clipboard.Diagnostic) (*binLaunch, error) {
	checks := []checkResult{
		clipboardCheck(diagnose()),
		staleDirectoryBinsCheck(paths),
	}

	failures := 0
	for _, check := range checks {
		if check.status == checkFail {
			failures++
		}
		line := check.String()
		if terminalOutput(output) {
			line = colorize(line, check.status)
		}
		if _, err := fmt.Fprintln(output, line); err != nil {
			return nil, fmt.Errorf("write doctor output: %w", err)
		}
	}
	if failures > 0 {
		if _, err := fmt.Fprintln(output); err != nil {
			return nil, fmt.Errorf("write doctor output: %w", err)
		}
		summary := fmt.Sprintf("%d check(s) failed", failures)
		if terminalOutput(output) {
			summary = colorize(summary, checkFail)
		}
		if _, err := fmt.Fprintln(output, summary); err != nil {
			return nil, fmt.Errorf("write doctor output: %w", err)
		}
		return nil, errDoctorFailed
	}
	return nil, nil
}

func clipboardCheck(diagnostic clipboard.Diagnostic) checkResult {
	check := checkResult{name: "Clipboard access", detail: diagnostic.Backend}
	if diagnostic.Status == clipboard.StatusUnavailable {
		check.status = checkFail
	}
	return check
}

// staleDirectoryBinsCheck counts directory bins whose directory has been
// removed. Storage that does not exist yet is left untouched and reported as
// having no stale bins.
func staleDirectoryBinsCheck(paths store.Paths) checkResult {
	check := checkResult{name: "Stale directory bins"}
	if _, err := os.Stat(paths.BinsFile); errors.Is(err, os.ErrNotExist) {
		check.detail = "none"
		return check
	}
	bins, err := store.Open(paths)
	if err != nil {
		check.status = checkFail
		check.detail = err.Error()
		return check
	}
	stale := len(bins.StaleDirectoryBins())
	if stale == 0 {
		check.detail = "none"
		return check
	}
	check.status = checkWarn
	check.detail = fmt.Sprintf("%d stale; run 'tpb prune --dry-run' to review", stale)
	return check
}

// colorize wraps text in the ANSI color matching a check status.
func colorize(text string, status checkStatus) string {
	color := ansiGreen
	switch status {
	case checkWarn:
		color = ansiYellow
	case checkFail:
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
	return isTerminal(w)
}

// isTerminal reports whether a reader or writer is backed by a character
// device such as a TTY.
func isTerminal(stream any) bool {
	file, ok := stream.(*os.File)
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
