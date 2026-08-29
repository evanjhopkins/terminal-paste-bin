package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

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

	for _, args := range [][]string{{"list", "extra"}, {"one", "two"}} {
		_, err := run(args, paths, &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), "usage: tpb") {
			t.Errorf("run(%v) error = %v, want unavailable-command error", args, err)
		}
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
		if err := bins.EnsureBin("default"); err != nil {
			t.Fatalf("EnsureBin: %v", err)
		}
		if err := bins.WriteSlot("default", 1, paths.Directory); err != nil {
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
	value, exists, err := production.ReadSlot("default", 1)
	if err != nil || !exists || value != productionPaths.Directory {
		t.Errorf("production slot = (%q, %t, %v), want (%q, true, nil)", value, exists, err, productionPaths.Directory)
	}
}

func TestRunLoadsDefaultAndNamedBins(t *testing.T) {
	paths, err := store.PathsFor(t.TempDir(), "tpb")
	if err != nil {
		t.Fatalf("PathsFor: %v", err)
	}

	for _, args := range [][]string{nil, {"myapp"}} {
		launch, err := run(args, paths, &bytes.Buffer{})
		if err != nil {
			t.Fatalf("run(%v): %v", args, err)
		}
		if launch == nil {
			t.Fatalf("run(%v) did not prepare a bin", args)
		}
	}

	bins, err := store.Open(paths)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if got, want := bins.ListBins(), []store.BinInfo{{ID: "default", Name: "default"}, {ID: "myapp", Name: "myapp"}}; !reflect.DeepEqual(got, want) {
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

	launch, err := runInDirectory([]string{"."}, paths, &bytes.Buffer{}, link)
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

	again, err := runInDirectory([]string{"."}, paths, &bytes.Buffer{}, directory)
	if err != nil {
		t.Fatalf("reopen directory bin: %v", err)
	}
	if again.id != launch.id {
		t.Errorf("directory bin IDs = %q and %q, want one canonical bin", launch.id, again.id)
	}
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
	if err := bins.EnsureBin("default"); err != nil {
		t.Fatalf("EnsureBin: %v", err)
	}
	if err := bins.WriteSlot("default", 3, "value"); err != nil {
		t.Fatalf("WriteSlot: %v", err)
	}

	if err := deleteBinSlot(paths, "default", 3); err != nil {
		t.Fatalf("deleteBinSlot: %v", err)
	}
	reopened, err := store.Open(paths)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if _, exists, err := reopened.ReadSlot("default", 3); err != nil || exists {
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
	if err := bins.EnsureBin("default"); err != nil {
		t.Fatalf("EnsureBin: %v", err)
	}

	want := "hello\n\u4e16\u754c\n"
	if err := writeClipboardToSlot(paths, "default", 3, fakeClipboard{value: want}); err != nil {
		t.Fatalf("writeClipboardToSlot: %v", err)
	}
	reopened, err := store.Open(paths)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, exists, err := reopened.ReadSlot("default", 3)
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
	if err := bins.EnsureBin("default"); err != nil {
		t.Fatalf("EnsureBin: %v", err)
	}
	want := "hello\n\u4e16\u754c\n"
	if err := bins.WriteSlot("default", 3, want); err != nil {
		t.Fatalf("WriteSlot: %v", err)
	}

	writer := &recordingWriter{}
	copied, err := copySlotToClipboard(paths, "default", 3, writer)
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
	if err := bins.EnsureBin("default"); err != nil {
		t.Fatalf("EnsureBin: %v", err)
	}

	writer := &recordingWriter{}
	copied, err := copySlotToClipboard(paths, "default", 3, writer)
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
