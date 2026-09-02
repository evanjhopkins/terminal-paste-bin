package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/evanjhopkins/terminal-paste-bin/internal/clipboard"
	"github.com/evanjhopkins/terminal-paste-bin/internal/store"
)

func TestRunListsBinsInAlphabeticalOrder(t *testing.T) {
	paths, err := store.PathsFor(t.TempDir(), "tpb")
	if err != nil {
		t.Fatalf("PathsFor: %v", err)
	}
	bins, err := store.Open(paths)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	for _, name := range []string{"zeta", "alpha"} {
		if err := bins.EnsureBin(name); err != nil {
			t.Fatalf("EnsureBin(%q): %v", name, err)
		}
	}

	var output bytes.Buffer
	if _, err := run([]string{"list"}, paths, &output); err != nil {
		t.Fatalf("run list: %v", err)
	}
	if got, want := output.String(), "alpha\nzeta\n"; got != want {
		t.Errorf("list output = %q, want %q", got, want)
	}
}

func TestRunRejectsUnavailableCommands(t *testing.T) {
	paths, err := store.PathsFor(t.TempDir(), "tpb")
	if err != nil {
		t.Fatalf("PathsFor: %v", err)
	}

	for _, args := range [][]string{
		{"list", "extra"},
		{"one", "two"},
		{"doctor", "extra"},
		{"reset", "extra"},
		{"delete"},
		{"delete", "a", "b"},
		{"delete", "--force", "a"},
		{"rename", "a"},
		{"rename", "a", "b", "c"},
		{"prune", "extra"},
	} {
		_, err := run(args, paths, &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), "usage: tpb") {
			t.Errorf("run(%v) error = %v, want unavailable-command error", args, err)
		}
	}
	if _, err := os.Stat(paths.BinsFile); !os.IsNotExist(err) {
		t.Errorf("usage errors created storage: stat error = %v, want not exist", err)
	}
}

func TestRunInformationalFlagsDoNotCreateStorage(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{args: []string{"-h"}, want: helpText},
		{args: []string{"--help"}, want: helpText},
		{args: []string{"-v"}, want: "tpb " + version + "\n"},
		{args: []string{"--version"}, want: "tpb " + version + "\n"},
	}

	for _, test := range tests {
		t.Run(strings.Join(test.args, ""), func(t *testing.T) {
			paths, err := store.PathsFor(t.TempDir(), "tpb")
			if err != nil {
				t.Fatalf("PathsFor: %v", err)
			}

			var output bytes.Buffer
			launch, err := run(test.args, paths, &output)
			if err != nil {
				t.Fatalf("run(%v): %v", test.args, err)
			}
			if launch != nil {
				t.Errorf("run(%v) launch = %v, want nil", test.args, launch)
			}
			if got := output.String(); got != test.want {
				t.Errorf("run(%v) output = %q, want %q", test.args, got, test.want)
			}
			if _, err := os.Stat(paths.Directory); !os.IsNotExist(err) {
				t.Errorf("storage directory stat error = %v, want not exist", err)
			}
		})
	}
}

func TestHelpDocumentsLifecycleCommands(t *testing.T) {
	for _, command := range []string{"tpb delete [--yes] <bin>", "tpb rename <old> <new>", "tpb prune [--dry-run]"} {
		if !strings.Contains(helpText, command) {
			t.Errorf("helpText does not mention %q", command)
		}
	}
}

func TestRunDoctorReportsClipboardStatus(t *testing.T) {
	tests := []struct {
		name       string
		diagnostic clipboard.Diagnostic
		want       string
		wantErr    bool
	}{
		{
			name:       "available",
			diagnostic: clipboard.Diagnostic{Status: clipboard.StatusOK, Backend: "wl-clipboard"},
			want:       "Clipboard access: OK (wl-clipboard)\nStale directory bins: OK (none)\n",
		},
		{
			name:       "unavailable",
			diagnostic: clipboard.Diagnostic{Status: clipboard.StatusUnavailable},
			want:       "Clipboard access: FAIL\nStale directory bins: OK (none)\n\n1 check(s) failed\n",
			wantErr:    true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			paths, err := store.PathsFor(t.TempDir(), "tpb")
			if err != nil {
				t.Fatalf("PathsFor: %v", err)
			}
			var output bytes.Buffer
			_, err = runDoctor(paths, &output, func() clipboard.Diagnostic { return test.diagnostic })
			if test.wantErr && !errors.Is(err, errDoctorFailed) {
				t.Fatalf("runDoctor error = %v, want errDoctorFailed", err)
			}
			if !test.wantErr && err != nil {
				t.Fatalf("runDoctor error = %v, want nil", err)
			}
			if got := output.String(); got != test.want {
				t.Errorf("runDoctor output = %q, want %q", got, test.want)
			}
		})
	}
}

func TestRunDoctorWarnsAboutStaleDirectoryBins(t *testing.T) {
	paths, err := store.PathsFor(t.TempDir(), "tpb")
	if err != nil {
		t.Fatalf("PathsFor: %v", err)
	}
	stale := t.TempDir()
	live := t.TempDir()
	bins, err := store.Open(paths)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	for _, directory := range []string{stale, live} {
		if _, err := bins.EnsureDirectoryBin(directory); err != nil {
			t.Fatalf("EnsureDirectoryBin(%q): %v", directory, err)
		}
	}
	if err := os.RemoveAll(stale); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}

	var output bytes.Buffer
	available := func() clipboard.Diagnostic {
		return clipboard.Diagnostic{Status: clipboard.StatusOK, Backend: "pbcopy/pbpaste"}
	}
	if _, err := runDoctor(paths, &output, available); err != nil {
		t.Fatalf("runDoctor error = %v, want nil for warnings", err)
	}
	want := "Clipboard access: OK (pbcopy/pbpaste)\nStale directory bins: WARN (1 stale; run 'tpb prune --dry-run' to review)\n"
	if got := output.String(); got != want {
		t.Errorf("runDoctor output = %q, want %q", got, want)
	}

	reopened, err := store.Open(paths)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got := len(reopened.ListBins()); got != 2 {
		t.Errorf("doctor changed bin count to %d, want 2 (doctor must not prune)", got)
	}
}

func TestColorize(t *testing.T) {
	tests := []struct {
		name   string
		status checkStatus
		want   string
	}{
		{name: "ok", status: checkOK, want: "\x1b[32mClipboard access: OK\x1b[0m"},
		{name: "warn", status: checkWarn, want: "\x1b[33mClipboard access: WARN\x1b[0m"},
		{name: "fail", status: checkFail, want: "\x1b[31mClipboard access: FAIL\x1b[0m"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := colorize("Clipboard access: "+test.status.label(), test.status); got != test.want {
				t.Errorf("colorize = %q, want %q", got, test.want)
			}
		})
	}
}

func TestTerminalOutputIsFalseForPipes(t *testing.T) {
	if terminalOutput(&bytes.Buffer{}) {
		t.Error("terminalOutput reported a buffer as a terminal")
	}
	if isTerminal(strings.NewReader("")) {
		t.Error("isTerminal reported a string reader as a terminal")
	}
}

func TestRunDoctorDoesNotCreateStorage(t *testing.T) {
	paths, err := store.PathsFor(t.TempDir(), "tpb")
	if err != nil {
		t.Fatalf("PathsFor: %v", err)
	}

	run([]string{"doctor"}, paths, &bytes.Buffer{})
	if _, err := os.Stat(paths.Directory); !os.IsNotExist(err) {
		t.Errorf("storage directory stat error = %v, want not exist", err)
	}
}

func TestRunResetClearsOnlyRequestedEnvironment(t *testing.T) {
	configDirectory := t.TempDir()
	productionPaths, err := store.PathsFor(configDirectory, "tpb")
	if err != nil {
		t.Fatalf("PathsFor production: %v", err)
	}
	developmentPaths, err := store.PathsFor(configDirectory, "tpbd")
	if err != nil {
		t.Fatalf("PathsFor development: %v", err)
	}
	for _, paths := range []store.Paths{productionPaths, developmentPaths} {
		bins, err := store.Open(paths)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		if err := bins.EnsureBin("myapp"); err != nil {
			t.Fatalf("EnsureBin: %v", err)
		}
		if err := bins.WriteSlot("myapp", 1, paths.Directory); err != nil {
			t.Fatalf("WriteSlot: %v", err)
		}
	}

	var output bytes.Buffer
	if _, err := run([]string{"reset"}, developmentPaths, &output); err != nil {
		t.Fatalf("run reset: %v", err)
	}
	if got, want := output.String(), "Reset complete.\n"; got != want {
		t.Errorf("reset output = %q, want %q", got, want)
	}
	for _, path := range []string{developmentPaths.BinsFile, developmentPaths.ConfigFile} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("reset file %s stat error = %v, want not exist", path, err)
		}
	}

	production, err := store.Open(productionPaths)
	if err != nil {
		t.Fatalf("reopen production store: %v", err)
	}
	value, exists, err := production.ReadSlot("myapp", 1)
	if err != nil || !exists || value != productionPaths.Directory {
		t.Errorf("production slot = (%q, %t, %v), want (%q, true, nil)", value, exists, err, productionPaths.Directory)
	}
}

func TestRunOpensDirectoryBinWithoutAnArgument(t *testing.T) {
	paths, err := store.PathsFor(t.TempDir(), "tpb")
	if err != nil {
		t.Fatalf("PathsFor: %v", err)
	}
	directory := filepath.Join(t.TempDir(), "project")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	canonicalPath, err := canonicalDirectory(directory)
	if err != nil {
		t.Fatalf("canonicalDirectory: %v", err)
	}

	launch, err := runInDirectory(nil, paths, &bytes.Buffer{}, directory)
	if err != nil {
		t.Fatalf("run directory bin: %v", err)
	}
	if launch.directory != canonicalPath || launch.id == "" {
		t.Errorf("directory launch without argument = %+v", launch)
	}

	bins, err := store.Open(paths)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	for _, info := range bins.ListBins() {
		if !info.IsDirectory() || info.Directory != canonicalPath {
			t.Errorf("expected only the %s directory bin, saw %+v", canonicalPath, info)
		}
	}
}

func TestRunLoadsNamedBins(t *testing.T) {
	paths, err := store.PathsFor(t.TempDir(), "tpb")
	if err != nil {
		t.Fatalf("PathsFor: %v", err)
	}
	if _, err := run([]string{"myapp"}, paths, &bytes.Buffer{}); err != nil {
		t.Fatalf("run myapp: %v", err)
	}

	bins, err := store.Open(paths)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got, want := bins.ListBins(), []store.BinInfo{{ID: "myapp", Name: "myapp"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("ListBins() = %v, want %v", got, want)
	}
}

func TestRunOpensCanonicalDirectoryBin(t *testing.T) {
	paths, err := store.PathsFor(t.TempDir(), "tpb")
	if err != nil {
		t.Fatalf("PathsFor: %v", err)
	}
	directory := filepath.Join(t.TempDir(), "project")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	link := filepath.Join(t.TempDir(), "project-link")
	if err := os.Symlink(directory, link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	canonicalPath, err := canonicalDirectory(directory)
	if err != nil {
		t.Fatalf("canonicalDirectory: %v", err)
	}

	launch, err := runInDirectory(nil, paths, &bytes.Buffer{}, link)
	if err != nil {
		t.Fatalf("run directory bin: %v", err)
	}
	if launch.name != "current directory" || launch.directory != canonicalPath || launch.id == "" {
		t.Errorf("directory launch = %+v", launch)
	}

	var output bytes.Buffer
	if _, err := run([]string{"list"}, paths, &output); err != nil {
		t.Fatalf("run list: %v", err)
	}
	if got, want := output.String(), "(dir) "+canonicalPath+"\n"; got != want {
		t.Errorf("list output = %q, want %q", got, want)
	}

	again, err := runInDirectory(nil, paths, &bytes.Buffer{}, directory)
	if err != nil {
		t.Fatalf("reopen directory bin: %v", err)
	}
	if again.id != launch.id {
		t.Errorf("directory bin IDs = %q and %q, want one canonical bin", launch.id, again.id)
	}
}

func TestExecuteCommandUsesDirectoryBinWorkingDirectory(t *testing.T) {
	directory := t.TempDir()
	outputPath := filepath.Join(t.TempDir(), "working-directory")
	t.Setenv("SHELL", "/bin/sh")

	if err := executeCommand("pwd > "+shellQuote(outputPath), directory); err != nil {
		t.Fatalf("executeCommand: %v", err)
	}
	contents, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read command output: %v", err)
	}
	if got, want := strings.TrimSpace(string(contents)), directory; got != want {
		t.Errorf("command working directory = %q, want %q", got, want)
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\\"'\\\"'") + "'"
}

func TestDeleteBinSlotPersistsDeletion(t *testing.T) {
	paths, err := store.PathsFor(t.TempDir(), "tpb")
	if err != nil {
		t.Fatalf("PathsFor: %v", err)
	}
	bins, err := store.Open(paths)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := bins.EnsureBin("myapp"); err != nil {
		t.Fatalf("EnsureBin: %v", err)
	}
	if err := bins.WriteSlot("myapp", 3, "value"); err != nil {
		t.Fatalf("WriteSlot: %v", err)
	}

	if err := deleteBinSlot(paths, "myapp", 3); err != nil {
		t.Fatalf("deleteBinSlot: %v", err)
	}
	reopened, err := store.Open(paths)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if _, exists, err := reopened.ReadSlot("myapp", 3); err != nil || exists {
		t.Errorf("deleted slot = (exists %t, err %v), want (false, nil)", exists, err)
	}
}

func TestWriteClipboardToSlotPreservesContents(t *testing.T) {
	paths, err := store.PathsFor(t.TempDir(), "tpb")
	if err != nil {
		t.Fatalf("PathsFor: %v", err)
	}
	bins, err := store.Open(paths)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := bins.EnsureBin("myapp"); err != nil {
		t.Fatalf("EnsureBin: %v", err)
	}

	want := "hello\n\u4e16\u754c\n"
	if err := writeClipboardToSlot(paths, "myapp", 3, fakeClipboard{value: want}); err != nil {
		t.Fatalf("writeClipboardToSlot: %v", err)
	}
	reopened, err := store.Open(paths)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, exists, err := reopened.ReadSlot("myapp", 3)
	if err != nil || !exists || got != want {
		t.Errorf("stored slot = (%q, %t, %v), want (%q, true, nil)", got, exists, err, want)
	}
}

func TestCopySlotToClipboardPreservesContents(t *testing.T) {
	paths, err := store.PathsFor(t.TempDir(), "tpb")
	if err != nil {
		t.Fatalf("PathsFor: %v", err)
	}
	bins, err := store.Open(paths)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := bins.EnsureBin("myapp"); err != nil {
		t.Fatalf("EnsureBin: %v", err)
	}
	want := "hello\n\u4e16\u754c\n"
	if err := bins.WriteSlot("myapp", 3, want); err != nil {
		t.Fatalf("WriteSlot: %v", err)
	}

	writer := &recordingWriter{}
	copied, err := copySlotToClipboard(paths, "myapp", 3, writer)
	if err != nil || !copied {
		t.Fatalf("copySlotToClipboard = (%t, %v), want (true, nil)", copied, err)
	}
	if writer.value != want {
		t.Errorf("clipboard value = %q, want %q", writer.value, want)
	}
}

func TestCopySlotToClipboardLeavesBlankSlotUntouched(t *testing.T) {
	paths, err := store.PathsFor(t.TempDir(), "tpb")
	if err != nil {
		t.Fatalf("PathsFor: %v", err)
	}
	bins, err := store.Open(paths)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := bins.EnsureBin("myapp"); err != nil {
		t.Fatalf("EnsureBin: %v", err)
	}

	writer := &recordingWriter{}
	copied, err := copySlotToClipboard(paths, "myapp", 3, writer)
	if err != nil || copied {
		t.Fatalf("copySlotToClipboard = (%t, %v), want (false, nil)", copied, err)
	}
	if writer.calls != 0 {
		t.Errorf("clipboard writes = %d, want 0", writer.calls)
	}
}

type fakeClipboard struct {
	value string
	err   error
}

func (c fakeClipboard) Read() (string, error) {
	return c.value, c.err
}

type recordingWriter struct {
	value string
	calls int
	err   error
}

func (w *recordingWriter) Write(value string) error {
	w.calls++
	w.value = value
	return w.err
}

// newStoreWithBin opens temporary storage containing one named bin with the
// supplied slots and returns its paths.
func newStoreWithBin(t *testing.T, name string, slots map[int]string) store.Paths {
	t.Helper()
	paths, err := store.PathsFor(t.TempDir(), "tpb")
	if err != nil {
		t.Fatalf("PathsFor: %v", err)
	}
	bins, err := store.Open(paths)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := bins.EnsureBin(name); err != nil {
		t.Fatalf("EnsureBin: %v", err)
	}
	for slot, value := range slots {
		if err := bins.WriteSlot(name, slot, value); err != nil {
			t.Fatalf("WriteSlot(%d): %v", slot, err)
		}
	}
	return paths
}

func binNames(t *testing.T, paths store.Paths) []string {
	t.Helper()
	bins, err := store.Open(paths)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	names := make([]string, 0)
	for _, info := range bins.ListBins() {
		if info.IsDirectory() {
			names = append(names, "(dir) "+info.Directory)
			continue
		}
		names = append(names, info.Name)
	}
	return names
}

func TestRunDeleteRefusesWithoutConfirmationWhenNotInteractive(t *testing.T) {
	paths := newStoreWithBin(t, "myapp", map[int]string{1: "keep"})

	var output bytes.Buffer
	_, err := runSession([]string{"delete", "myapp"}, session{paths: paths, output: &output, input: strings.NewReader("y\n"), interactive: false})
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("non-interactive delete error = %v, want refusal mentioning --yes", err)
	}
	if output.Len() != 0 {
		t.Errorf("non-interactive delete wrote %q, want no prompt", output.String())
	}
	if got, want := binNames(t, paths), []string{"myapp"}; !reflect.DeepEqual(got, want) {
		t.Errorf("bins after refused delete = %v, want %v", got, want)
	}
}

func TestRunDeleteSkipsPromptWithFlag(t *testing.T) {
	for _, flag := range []string{"--yes", "-y"} {
		t.Run(flag, func(t *testing.T) {
			paths := newStoreWithBin(t, "myapp", map[int]string{1: "gone"})

			var output bytes.Buffer
			if _, err := run([]string{"delete", flag, "myapp"}, paths, &output); err != nil {
				t.Fatalf("run delete %s: %v", flag, err)
			}
			if got, want := output.String(), "Deleted bin \"myapp\".\n"; got != want {
				t.Errorf("delete output = %q, want %q", got, want)
			}
			if got := binNames(t, paths); len(got) != 0 {
				t.Errorf("bins after delete = %v, want none", got)
			}
			contents, err := os.ReadFile(paths.BinsFile)
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			if strings.Contains(string(contents), "gone") {
				t.Errorf("deleted slot data still on disk: %s", contents)
			}
		})
	}
}

func TestRunDeleteFlagMayFollowBinName(t *testing.T) {
	paths := newStoreWithBin(t, "myapp", nil)
	if _, err := run([]string{"delete", "myapp", "--yes"}, paths, &bytes.Buffer{}); err != nil {
		t.Fatalf("run delete myapp --yes: %v", err)
	}
	if got := binNames(t, paths); len(got) != 0 {
		t.Errorf("bins after delete = %v, want none", got)
	}
}

func TestRunDeletePromptsInteractively(t *testing.T) {
	tests := []struct {
		name       string
		answer     string
		wantDelete bool
	}{
		{name: "yes", answer: "y\n", wantDelete: true},
		{name: "yes word", answer: "YES\n", wantDelete: true},
		{name: "no", answer: "n\n"},
		{name: "enter", answer: "\n"},
		{name: "eof", answer: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			paths := newStoreWithBin(t, "myapp", map[int]string{1: "one", 2: "", 7: "multi\nline"})

			var output bytes.Buffer
			_, err := runSession([]string{"delete", "myapp"}, session{paths: paths, output: &output, input: strings.NewReader(test.answer), interactive: true})
			prompt := "Delete bin \"myapp\" and its 2 non-blank slot(s)? This cannot be undone. [y/N] "
			if !strings.HasPrefix(output.String(), prompt) {
				t.Errorf("prompt = %q, want prefix %q", output.String(), prompt)
			}
			if test.wantDelete {
				if err != nil {
					t.Fatalf("confirmed delete error = %v", err)
				}
				if !strings.HasSuffix(output.String(), "Deleted bin \"myapp\".\n") {
					t.Errorf("confirmed delete output = %q, want deletion notice", output.String())
				}
				if got := binNames(t, paths); len(got) != 0 {
					t.Errorf("bins after confirmed delete = %v, want none", got)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "cancelled") {
				t.Fatalf("declined delete error = %v, want cancellation", err)
			}
			if got, want := binNames(t, paths), []string{"myapp"}; !reflect.DeepEqual(got, want) {
				t.Errorf("bins after declined delete = %v, want %v", got, want)
			}
		})
	}
}

func TestRunDeleteRejectsMissingAndDirectoryBins(t *testing.T) {
	paths := newStoreWithBin(t, "myapp", nil)
	directory := t.TempDir()
	bins, err := store.Open(paths)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	info, err := bins.EnsureDirectoryBin(directory)
	if err != nil {
		t.Fatalf("EnsureDirectoryBin: %v", err)
	}

	_, err = run([]string{"delete", "--yes", "missing"}, paths, &bytes.Buffer{})
	if !errors.Is(err, store.ErrBinNotFound) {
		t.Errorf("delete missing bin error = %v, want ErrBinNotFound", err)
	}
	_, err = run([]string{"delete", "--yes", info.ID}, paths, &bytes.Buffer{})
	if !errors.Is(err, store.ErrDirectoryBin) || !strings.Contains(err.Error(), "tpb prune") {
		t.Errorf("delete directory bin error = %v, want ErrDirectoryBin mentioning tpb prune", err)
	}
	if got, want := binNames(t, paths), []string{"myapp", "(dir) " + directory}; !reflect.DeepEqual(got, want) {
		t.Errorf("bins after rejected deletes = %v, want %v", got, want)
	}
}

func TestRunRenamePreservesSlots(t *testing.T) {
	slots := map[int]string{0: "zero", 3: "multi\nline\n\u4e16\u754c", 9: ""}
	paths := newStoreWithBin(t, "old", slots)

	var output bytes.Buffer
	if _, err := run([]string{"rename", "old", "new"}, paths, &output); err != nil {
		t.Fatalf("run rename: %v", err)
	}
	if got, want := output.String(), "Renamed bin \"old\" to \"new\".\n"; got != want {
		t.Errorf("rename output = %q, want %q", got, want)
	}
	if got, want := binNames(t, paths), []string{"new"}; !reflect.DeepEqual(got, want) {
		t.Errorf("bins after rename = %v, want %v", got, want)
	}

	bins, err := store.Open(paths)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	for slot := 0; slot <= 9; slot++ {
		value, exists, err := bins.ReadSlot("new", slot)
		if err != nil {
			t.Fatalf("ReadSlot(%d): %v", slot, err)
		}
		want, wantExists := slots[slot]
		if exists != wantExists || value != want {
			t.Errorf("slot %d = (%q, %t), want (%q, %t)", slot, value, exists, want, wantExists)
		}
	}
}

func TestRunRenameRejectsInvalidTargets(t *testing.T) {
	paths := newStoreWithBin(t, "old", map[int]string{1: "keep"})
	bins, err := store.Open(paths)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := bins.EnsureBin("taken"); err != nil {
		t.Fatalf("EnsureBin: %v", err)
	}
	directory := t.TempDir()
	info, err := bins.EnsureDirectoryBin(directory)
	if err != nil {
		t.Fatalf("EnsureDirectoryBin: %v", err)
	}

	tests := []struct {
		name    string
		args    []string
		wantErr error
		wantMsg string
	}{
		{name: "missing", args: []string{"rename", "missing", "new"}, wantErr: store.ErrBinNotFound},
		{name: "taken", args: []string{"rename", "old", "taken"}, wantErr: store.ErrBinExists},
		{name: "same", args: []string{"rename", "old", "old"}, wantErr: store.ErrBinExists},
		{name: "reserved", args: []string{"rename", "old", "prune"}, wantErr: store.ErrInvalidBinName},
		{name: "control character", args: []string{"rename", "old", "bad\nname"}, wantErr: store.ErrInvalidBinName},
		{name: "directory id", args: []string{"rename", info.ID, "new"}, wantErr: store.ErrDirectoryBin},
		{name: "directory path", args: []string{"rename", directory, "new"}, wantErr: store.ErrBinNotFound, wantMsg: "cannot be renamed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := run(test.args, paths, &bytes.Buffer{})
			if !errors.Is(err, test.wantErr) {
				t.Errorf("run(%v) error = %v, want %v", test.args, err, test.wantErr)
			}
			if test.wantMsg != "" && (err == nil || !strings.Contains(err.Error(), test.wantMsg)) {
				t.Errorf("run(%v) error = %v, want message containing %q", test.args, err, test.wantMsg)
			}
		})
	}

	if got, want := binNames(t, paths), []string{"old", "taken", "(dir) " + directory}; !reflect.DeepEqual(got, want) {
		t.Errorf("bins after rejected renames = %v, want %v", got, want)
	}
	reopened, err := store.Open(paths)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if value, exists, err := reopened.ReadSlot("old", 1); err != nil || !exists || value != "keep" {
		t.Errorf("old slot after rejected renames = (%q, %t, %v), want (keep, true, nil)", value, exists, err)
	}
}

func TestRunPruneRemovesOnlyStaleDirectoryBins(t *testing.T) {
	paths := newStoreWithBin(t, "named", map[int]string{1: "keep"})
	live := t.TempDir()
	staleA := filepath.Join(t.TempDir(), "a")
	staleB := filepath.Join(t.TempDir(), "b")
	for _, directory := range []string{staleA, staleB} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatalf("Mkdir: %v", err)
		}
	}
	for _, directory := range []string{live, staleA, staleB} {
		if _, err := runInDirectory(nil, paths, &bytes.Buffer{}, directory); err != nil {
			t.Fatalf("create directory bin %q: %v", directory, err)
		}
	}
	for _, directory := range []string{staleA, staleB} {
		if err := os.RemoveAll(directory); err != nil {
			t.Fatalf("RemoveAll: %v", err)
		}
	}
	before := binNames(t, paths)

	var dryRun bytes.Buffer
	if _, err := run([]string{"prune", "--dry-run"}, paths, &dryRun); err != nil {
		t.Fatalf("run prune --dry-run: %v", err)
	}
	if got, want := dryRun.String(), "Would prune (dir) "+staleA+"\nWould prune (dir) "+staleB+"\n"; got != want {
		t.Errorf("dry-run output = %q, want %q", got, want)
	}
	if got := binNames(t, paths); !reflect.DeepEqual(got, before) {
		t.Errorf("dry-run changed bins from %v to %v", before, got)
	}

	var output bytes.Buffer
	if _, err := run([]string{"prune"}, paths, &output); err != nil {
		t.Fatalf("run prune: %v", err)
	}
	if got, want := output.String(), "Pruned (dir) "+staleA+"\nPruned (dir) "+staleB+"\n"; got != want {
		t.Errorf("prune output = %q, want %q", got, want)
	}
	if got, want := binNames(t, paths), []string{"named", "(dir) " + live}; !reflect.DeepEqual(got, want) {
		t.Errorf("bins after prune = %v, want %v", got, want)
	}

	var again bytes.Buffer
	if _, err := run([]string{"prune"}, paths, &again); err != nil {
		t.Fatalf("run prune again: %v", err)
	}
	if got, want := again.String(), "Nothing to prune.\n"; got != want {
		t.Errorf("second prune output = %q, want %q", got, want)
	}
}

func TestRunPruneReportsNothingWhenAllDirectoryBinsAreLive(t *testing.T) {
	paths := newStoreWithBin(t, "named", nil)
	if _, err := runInDirectory(nil, paths, &bytes.Buffer{}, t.TempDir()); err != nil {
		t.Fatalf("create directory bin: %v", err)
	}
	before := binNames(t, paths)

	for _, args := range [][]string{{"prune", "--dry-run"}, {"prune"}} {
		var output bytes.Buffer
		if _, err := run(args, paths, &output); err != nil {
			t.Fatalf("run %v: %v", args, err)
		}
		if got, want := output.String(), "Nothing to prune.\n"; got != want {
			t.Errorf("run(%v) output = %q, want %q", args, got, want)
		}
	}
	if got := binNames(t, paths); !reflect.DeepEqual(got, before) {
		t.Errorf("prune changed bins from %v to %v", before, got)
	}
}

func TestRunPruneRemovesAllDirectoryBinsWhenAllAreStale(t *testing.T) {
	paths := newStoreWithBin(t, "named", nil)
	directory := filepath.Join(t.TempDir(), "gone")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if _, err := runInDirectory(nil, paths, &bytes.Buffer{}, directory); err != nil {
		t.Fatalf("create directory bin: %v", err)
	}
	if err := os.RemoveAll(directory); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}

	if _, err := run([]string{"prune"}, paths, &bytes.Buffer{}); err != nil {
		t.Fatalf("run prune: %v", err)
	}
	if got, want := binNames(t, paths), []string{"named"}; !reflect.DeepEqual(got, want) {
		t.Errorf("bins after prune = %v, want %v", got, want)
	}
}

func TestRunPruneComparesAgainstCanonicalPaths(t *testing.T) {
	paths := newStoreWithBin(t, "named", nil)
	target := filepath.Join(t.TempDir(), "project")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	link := filepath.Join(t.TempDir(), "project-link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	canonicalPath, err := canonicalDirectory(target)
	if err != nil {
		t.Fatalf("canonicalDirectory: %v", err)
	}

	// Create the bin through a symlink with a trailing slash; only the
	// canonical path is stored.
	if _, err := runInDirectory(nil, paths, &bytes.Buffer{}, link+string(filepath.Separator)); err != nil {
		t.Fatalf("create directory bin via symlink: %v", err)
	}
	if got, want := binNames(t, paths), []string{"named", "(dir) " + canonicalPath}; !reflect.DeepEqual(got, want) {
		t.Fatalf("bins = %v, want %v", got, want)
	}

	// Removing the symlink leaves the real directory intact: not stale.
	if err := os.Remove(link); err != nil {
		t.Fatalf("Remove link: %v", err)
	}
	var output bytes.Buffer
	if _, err := run([]string{"prune"}, paths, &output); err != nil {
		t.Fatalf("run prune: %v", err)
	}
	if got, want := output.String(), "Nothing to prune.\n"; got != want {
		t.Errorf("prune after removing symlink = %q, want %q", got, want)
	}

	// Removing the real directory makes the bin stale.
	if err := os.RemoveAll(target); err != nil {
		t.Fatalf("RemoveAll target: %v", err)
	}
	output.Reset()
	if _, err := run([]string{"prune"}, paths, &output); err != nil {
		t.Fatalf("run prune: %v", err)
	}
	if got, want := output.String(), "Pruned (dir) "+canonicalPath+"\n"; got != want {
		t.Errorf("prune after removing target = %q, want %q", got, want)
	}
	if got, want := binNames(t, paths), []string{"named"}; !reflect.DeepEqual(got, want) {
		t.Errorf("bins after prune = %v, want %v", got, want)
	}
}

func TestRunDeleteDoesNotSeeStaleBinsFromAnotherProcess(t *testing.T) {
	// Simulates another tpb process holding a bin open: its store instance was
	// loaded before the deletion and must not resurrect the bin on write.
	paths := newStoreWithBin(t, "myapp", map[int]string{1: "one"})
	other, err := store.Open(paths)
	if err != nil {
		t.Fatalf("Open other: %v", err)
	}

	if _, err := run([]string{"delete", "--yes", "myapp"}, paths, &bytes.Buffer{}); err != nil {
		t.Fatalf("run delete: %v", err)
	}
	if err := other.WriteSlot("myapp", 2, "late"); !errors.Is(err, store.ErrBinNotFound) {
		t.Errorf("write to deleted bin from stale store error = %v, want ErrBinNotFound", err)
	}
	if got := binNames(t, paths); len(got) != 0 {
		t.Errorf("bins after stale write = %v, want none", got)
	}
}
