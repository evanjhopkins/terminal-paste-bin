package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestStorePersistsBinsAndSlots(t *testing.T) {
	paths, err := PathsFor(t.TempDir(), "tpbd")
	if err != nil {
		t.Fatalf("PathsFor: %v", err)
	}

	store, err := Open(paths)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := store.EnsureBin("myapp"); err != nil {
		t.Fatalf("EnsureBin myapp: %v", err)
	}
	if err := store.EnsureBin("myproxy"); err != nil {
		t.Fatalf("EnsureBin myproxy: %v", err)
	}
	if err := store.WriteSlot("myapp", 3, "hello\nworld"); err != nil {
		t.Fatalf("WriteSlot: %v", err)
	}

	reopened, err := Open(paths)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got, want := reopened.ListBins(), []BinInfo{{ID: "myapp", Name: "myapp"}, {ID: "myproxy", Name: "myproxy"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("ListBins() = %v, want %v", got, want)
	}
	if got, exists, err := reopened.ReadSlot("myapp", 3); err != nil || !exists || got != "hello\nworld" {
		t.Errorf("ReadSlot() = (%q, %t, %v), want (%q, true, nil)", got, exists, err, "hello\nworld")
	}
	if err := reopened.DeleteSlot("myapp", 3); err != nil {
		t.Fatalf("DeleteSlot: %v", err)
	}
	if _, exists, err := reopened.ReadSlot("myapp", 3); err != nil || exists {
		t.Errorf("deleted slot = (exists %t, err %v), want (false, nil)", exists, err)
	}
}

func TestStorePersistsDirectoryBins(t *testing.T) {
	paths, err := PathsFor(t.TempDir(), "tpb")
	if err != nil {
		t.Fatalf("PathsFor: %v", err)
	}
	directory := filepath.Join(t.TempDir(), "project")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	first, err := Open(paths)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	info, err := first.EnsureDirectoryBin(directory)
	if err != nil {
		t.Fatalf("EnsureDirectoryBin: %v", err)
	}
	if !info.IsDirectory() || info.Directory != directory || info.ID == directory {
		t.Errorf("directory bin info = %+v", info)
	}
	if err := first.WriteSlot(info.ID, 2, "project value"); err != nil {
		t.Fatalf("WriteSlot: %v", err)
	}

	reopened, err := Open(paths)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got, want := reopened.ListBins(), []BinInfo{info}; !reflect.DeepEqual(got, want) {
		t.Errorf("ListBins() = %v, want %v", got, want)
	}
	if got, exists, err := reopened.ReadSlot(info.ID, 2); err != nil || !exists || got != "project value" {
		t.Errorf("ReadSlot() = (%q, %t, %v), want (project value, true, nil)", got, exists, err)
	}
}

func TestOpenCreatesEmptyConfigAndBinsFiles(t *testing.T) {
	paths, err := PathsFor(t.TempDir(), "tpb")
	if err != nil {
		t.Fatalf("PathsFor: %v", err)
	}

	if _, err := Open(paths); err != nil {
		t.Fatalf("Open: %v", err)
	}

	config, err := os.ReadFile(paths.ConfigFile)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if string(config) != "{}\n" {
		t.Errorf("config file = %q, want %q", config, "{}\n")
	}
	if _, err := os.Stat(paths.BinsFile); err != nil {
		t.Fatalf("stat bins file: %v", err)
	}
	for _, path := range []string{paths.ConfigFile, paths.BinsFile} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
			t.Errorf("permissions for %s = %o, want %o", path, got, want)
		}
	}
}

func TestOpenRejectsMalformedFiles(t *testing.T) {
	for _, test := range []struct {
		name string
		file func(Paths) string
	}{
		{name: "config", file: func(paths Paths) string { return paths.ConfigFile }},
		{name: "bins", file: func(paths Paths) string { return paths.BinsFile }},
	} {
		t.Run(test.name, func(t *testing.T) {
			paths, err := PathsFor(t.TempDir(), "tpb")
			if err != nil {
				t.Fatalf("PathsFor: %v", err)
			}
			if err := os.MkdirAll(paths.Directory, 0o700); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}
			if err := os.WriteFile(test.file(paths), []byte("{"), 0o600); err != nil {
				t.Fatalf("write malformed file: %v", err)
			}

			_, err = Open(paths)
			if err == nil {
				t.Fatal("Open succeeded for malformed JSON")
			}
			contents, readErr := os.ReadFile(test.file(paths))
			if readErr != nil {
				t.Fatalf("read malformed file: %v", readErr)
			}
			if string(contents) != "{" {
				t.Errorf("malformed file was changed to %q", contents)
			}
		})
	}
}

func TestWriteFileAtomicReplacesContentsAndCleansUp(t *testing.T) {
	directory := t.TempDir()
	path := directory + "/data.json"
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatalf("write original file: %v", err)
	}

	if err := writeFileAtomic(path, []byte("new")); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read replacement file: %v", err)
	}
	if string(contents) != "new" {
		t.Errorf("replacement contents = %q, want %q", contents, "new")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read storage directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "data.json" {
		t.Errorf("directory entries = %v, want only data.json", entries)
	}
}

func TestStoreValidatesBinNamesAndSlots(t *testing.T) {
	paths, err := PathsFor(t.TempDir(), "tpb")
	if err != nil {
		t.Fatalf("PathsFor: %v", err)
	}
	store, err := Open(paths)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	for _, name := range []string{"", "list", "reset", "doctor", "default", "bad\nname", strings.Repeat("a", maxBinNameLength+1)} {
		if err := store.EnsureBin(name); !errors.Is(err, ErrInvalidBinName) {
			t.Errorf("EnsureBin(%q) error = %v, want ErrInvalidBinName", name, err)
		}
	}
	if _, err := store.EnsureDirectoryBin("relative/path"); err == nil {
		t.Error("EnsureDirectoryBin accepted a relative path")
	}
	if err := store.EnsureBin("valid"); err != nil {
		t.Fatalf("EnsureBin valid: %v", err)
	}
	if err := store.WriteSlot("valid", 10, "value"); !errors.Is(err, ErrInvalidSlot) {
		t.Errorf("WriteSlot invalid slot error = %v, want ErrInvalidSlot", err)
	}
	if _, _, err := store.ReadSlot("missing", 1); !errors.Is(err, ErrBinNotFound) {
		t.Errorf("ReadSlot missing bin error = %v, want ErrBinNotFound", err)
	}
}

func TestOpenPurgesLegacyReservedBins(t *testing.T) {
	paths, err := PathsFor(t.TempDir(), "tpb")
	if err != nil {
		t.Fatalf("PathsFor: %v", err)
	}
	if _, err := Open(paths); err != nil {
		t.Fatalf("Open: %v", err)
	}

	legacy := map[string]bin{
		"default": {Slots: map[int]string{1: "legacy"}},
		"doctor":  {Slots: map[int]string{2: "doctor"}},
		"myapp":   {Slots: make(map[int]string)},
	}

	contents, err := json.MarshalIndent(legacy, "", "  ")
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.WriteFile(paths.BinsFile, contents, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	reopened, err := Open(paths)
	if err != nil {
		t.Fatalf("Open after legacy data: %v", err)
	}
	names := make([]string, 0, len(reopened.bins))
	for name := range reopened.bins {
		names = append(names, name)
	}
	for _, name := range names {
		if name == "default" || name == "doctor" {
			t.Errorf("legacy reserved bin %q survived Open", name)
		}
	}

	var cleaned map[string]bin
	persisted, err := os.ReadFile(paths.BinsFile)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if err := json.Unmarshal(persisted, &cleaned); err != nil {
		t.Fatalf("Unmarshal persisted: %v", err)
	}
	if _, exists := cleaned["default"]; exists {
		t.Error("legacy default bin still present on disk after Open")
	}
	if _, exists := cleaned["doctor"]; exists {
		t.Error("legacy doctor bin still present on disk after Open")
	}
	if _, exists := cleaned["myapp"]; !exists {
		t.Error("unrelated named bin was dropped during purge")
	}
}

func TestMutationsWaitForExistingLock(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("advisory file locking is currently supported only on macOS and Linux")
	}

	paths, err := PathsFor(t.TempDir(), "tpb")
	if err != nil {
		t.Fatalf("PathsFor: %v", err)
	}
	first, err := Open(paths)
	if err != nil {
		t.Fatalf("Open first store: %v", err)
	}
	if err := first.EnsureBin("myapp"); err != nil {
		t.Fatalf("EnsureBin: %v", err)
	}
	second, err := Open(paths)
	if err != nil {
		t.Fatalf("Open second store: %v", err)
	}

	lock, err := acquireFileLock(paths.LockFile)
	if err != nil {
		t.Fatalf("acquireFileLock: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- second.WriteSlot("myapp", 1, "saved")
	}()

	select {
	case err := <-done:
		t.Fatalf("WriteSlot completed while lock was held: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if err := releaseFileLock(lock); err != nil {
		t.Fatalf("releaseFileLock: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("WriteSlot: %v", err)
	}
	reopened, err := Open(paths)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	if value, exists, err := reopened.ReadSlot("myapp", 1); err != nil || !exists || value != "saved" {
		t.Errorf("persisted slot = (%q, %t, %v), want (saved, true, nil)", value, exists, err)
	}
}

func TestMutationsReloadCurrentBinsBeforeSaving(t *testing.T) {
	paths, err := PathsFor(t.TempDir(), "tpb")
	if err != nil {
		t.Fatalf("PathsFor: %v", err)
	}
	first, err := Open(paths)
	if err != nil {
		t.Fatalf("Open first store: %v", err)
	}
	if err := first.EnsureBin("myapp"); err != nil {
		t.Fatalf("EnsureBin: %v", err)
	}
	second, err := Open(paths)
	if err != nil {
		t.Fatalf("Open second store: %v", err)
	}

	if err := first.WriteSlot("myapp", 1, "first"); err != nil {
		t.Fatalf("first WriteSlot: %v", err)
	}
	if err := second.WriteSlot("myapp", 2, "second"); err != nil {
		t.Fatalf("second WriteSlot: %v", err)
	}

	reopened, err := Open(paths)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	for _, expected := range []struct {
		slot  int
		value string
	}{
		{slot: 1, value: "first"},
		{slot: 2, value: "second"},
	} {
		value, exists, err := reopened.ReadSlot("myapp", expected.slot)
		if err != nil || !exists || value != expected.value {
			t.Errorf("slot %d = (%q, %t, %v), want (%q, true, nil)", expected.slot, value, exists, err, expected.value)
		}
	}
}
