package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
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

	for _, name := range []string{"", "list", "reset", "doctor", "default", "delete", "rename", "prune", "bad\nname", strings.Repeat("a", maxBinNameLength+1)} {
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

func openTestStore(t *testing.T) (*Store, Paths) {
	t.Helper()
	paths, err := PathsFor(t.TempDir(), "tpb")
	if err != nil {
		t.Fatalf("PathsFor: %v", err)
	}
	store, err := Open(paths)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return store, paths
}

func persistedBins(t *testing.T, paths Paths) map[string]bin {
	t.Helper()
	contents, err := os.ReadFile(paths.BinsFile)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var bins map[string]bin
	if err := json.Unmarshal(contents, &bins); err != nil {
		t.Fatalf("Unmarshal persisted bins: %v", err)
	}
	return bins
}

func TestDeleteBinRemovesBinAndSlotsFromDisk(t *testing.T) {
	store, paths := openTestStore(t)
	for _, name := range []string{"doomed", "survivor"} {
		if err := store.EnsureBin(name); err != nil {
			t.Fatalf("EnsureBin(%q): %v", name, err)
		}
		if err := store.WriteSlot(name, 4, name+" value"); err != nil {
			t.Fatalf("WriteSlot(%q): %v", name, err)
		}
	}

	if err := store.DeleteBin("doomed"); err != nil {
		t.Fatalf("DeleteBin: %v", err)
	}
	if _, exists := store.Lookup("doomed"); exists {
		t.Error("deleted bin still visible through Lookup")
	}
	if _, _, err := store.ReadSlot("doomed", 4); !errors.Is(err, ErrBinNotFound) {
		t.Errorf("ReadSlot on deleted bin error = %v, want ErrBinNotFound", err)
	}
	if got, want := store.ListBins(), []BinInfo{{ID: "survivor", Name: "survivor"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("ListBins() = %v, want %v", got, want)
	}

	persisted := persistedBins(t, paths)
	if _, exists := persisted["doomed"]; exists {
		t.Error("deleted bin still present on disk")
	}
	if persisted["survivor"].Slots[4] != "survivor value" {
		t.Errorf("surviving bin on disk = %+v, want slot 4 intact", persisted["survivor"])
	}
}

func TestDeleteBinRejectsMissingAndDirectoryBins(t *testing.T) {
	store, _ := openTestStore(t)
	info, err := store.EnsureDirectoryBin(t.TempDir())
	if err != nil {
		t.Fatalf("EnsureDirectoryBin: %v", err)
	}

	if err := store.DeleteBin("missing"); !errors.Is(err, ErrBinNotFound) {
		t.Errorf("DeleteBin(missing) error = %v, want ErrBinNotFound", err)
	}
	if err := store.DeleteBin(info.ID); !errors.Is(err, ErrDirectoryBin) {
		t.Errorf("DeleteBin(directory) error = %v, want ErrDirectoryBin", err)
	}
	if _, exists := store.Lookup(info.ID); !exists {
		t.Error("directory bin was removed by rejected DeleteBin")
	}
}

func TestRenameBinPreservesSlotsExactly(t *testing.T) {
	store, paths := openTestStore(t)
	if err := store.EnsureBin("old"); err != nil {
		t.Fatalf("EnsureBin: %v", err)
	}
	slots := map[int]string{0: "zero", 3: "line one\nline two\n\ttabbed \u4e16\u754c\n", 9: ""}
	for slot, value := range slots {
		if err := store.WriteSlot("old", slot, value); err != nil {
			t.Fatalf("WriteSlot(%d): %v", slot, err)
		}
	}

	if err := store.RenameBin("old", "new"); err != nil {
		t.Fatalf("RenameBin: %v", err)
	}
	if _, exists := store.Lookup("old"); exists {
		t.Error("old name still exists after rename")
	}
	if got, want := store.ListBins(), []BinInfo{{ID: "new", Name: "new"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("ListBins() = %v, want %v", got, want)
	}

	persisted := persistedBins(t, paths)
	if _, exists := persisted["old"]; exists {
		t.Error("old bin still present on disk after rename")
	}
	if got := persisted["new"].Slots; !reflect.DeepEqual(got, slots) {
		t.Errorf("renamed bin slots on disk = %#v, want %#v", got, slots)
	}
}

func TestRenameBinRejectsInvalidRequests(t *testing.T) {
	store, _ := openTestStore(t)
	for _, name := range []string{"old", "taken"} {
		if err := store.EnsureBin(name); err != nil {
			t.Fatalf("EnsureBin(%q): %v", name, err)
		}
	}
	info, err := store.EnsureDirectoryBin(t.TempDir())
	if err != nil {
		t.Fatalf("EnsureDirectoryBin: %v", err)
	}

	tests := []struct {
		name    string
		old     string
		new     string
		wantErr error
	}{
		{name: "missing", old: "missing", new: "new", wantErr: ErrBinNotFound},
		{name: "taken", old: "old", new: "taken", wantErr: ErrBinExists},
		{name: "same name", old: "old", new: "old", wantErr: ErrBinExists},
		{name: "reserved", old: "old", new: "delete", wantErr: ErrInvalidBinName},
		{name: "empty", old: "old", new: "", wantErr: ErrInvalidBinName},
		{name: "control character", old: "old", new: "bad\nname", wantErr: ErrInvalidBinName},
		{name: "directory bin", old: info.ID, new: "new", wantErr: ErrDirectoryBin},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := store.RenameBin(test.old, test.new); !errors.Is(err, test.wantErr) {
				t.Errorf("RenameBin(%q, %q) error = %v, want %v", test.old, test.new, err, test.wantErr)
			}
		})
	}

	want := []BinInfo{{ID: "old", Name: "old"}, {ID: "taken", Name: "taken"}, {ID: info.ID, Directory: info.Directory}}
	if got := store.ListBins(); !reflect.DeepEqual(got, want) {
		t.Errorf("ListBins() after rejected renames = %v, want %v", got, want)
	}
}

func TestOpenLoadsBinsWhoseNamesLaterBecameReserved(t *testing.T) {
	// A bin created before its name became a command word must still load so
	// the user can rename it, rather than making the whole store unreadable.
	store, paths := openTestStore(t)
	if err := store.EnsureBin("keep"); err != nil {
		t.Fatalf("EnsureBin: %v", err)
	}
	legacy := map[string]bin{
		"keep":  {Slots: make(map[int]string)},
		"prune": {Slots: map[int]string{1: "old data"}},
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
		t.Fatalf("Open with reserved-named bin: %v", err)
	}
	if _, exists := reopened.Lookup("prune"); !exists {
		t.Fatal("reserved-named bin was dropped on load")
	}
	if err := reopened.EnsureBin("prune"); !errors.Is(err, ErrInvalidBinName) {
		t.Errorf("EnsureBin(prune) error = %v, want ErrInvalidBinName", err)
	}
	if err := reopened.RenameBin("prune", "rescued"); err != nil {
		t.Fatalf("RenameBin rescue: %v", err)
	}
	if value, exists, err := reopened.ReadSlot("rescued", 1); err != nil || !exists || value != "old data" {
		t.Errorf("rescued slot = (%q, %t, %v), want (old data, true, nil)", value, exists, err)
	}
}

func TestPruneDirectoryBinsRemovesOnlyStaleBins(t *testing.T) {
	store, paths := openTestStore(t)
	if err := store.EnsureBin("named"); err != nil {
		t.Fatalf("EnsureBin: %v", err)
	}
	live := t.TempDir()
	staleDirectory := filepath.Join(t.TempDir(), "stale")
	if err := os.Mkdir(staleDirectory, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	replacedByFile := filepath.Join(t.TempDir(), "replaced")
	if err := os.Mkdir(replacedByFile, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	for _, directory := range []string{live, staleDirectory, replacedByFile} {
		if _, err := store.EnsureDirectoryBin(directory); err != nil {
			t.Fatalf("EnsureDirectoryBin(%q): %v", directory, err)
		}
	}
	if err := os.RemoveAll(staleDirectory); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}
	if err := os.RemoveAll(replacedByFile); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}
	if err := os.WriteFile(replacedByFile, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	stale := store.StaleDirectoryBins()
	wantStale := []string{replacedByFile, staleDirectory}
	sort.Strings(wantStale)
	if got := directoriesOf(stale); !reflect.DeepEqual(got, wantStale) {
		t.Fatalf("StaleDirectoryBins() = %v, want %v", got, wantStale)
	}
	if got := len(store.ListBins()); got != 4 {
		t.Errorf("StaleDirectoryBins changed bin count to %d, want 4", got)
	}

	pruned, err := store.PruneDirectoryBins()
	if err != nil {
		t.Fatalf("PruneDirectoryBins: %v", err)
	}
	if got := directoriesOf(pruned); !reflect.DeepEqual(got, wantStale) {
		t.Errorf("PruneDirectoryBins() = %v, want %v", got, wantStale)
	}
	want := []BinInfo{{ID: "named", Name: "named"}, {ID: directoryBinID(live), Directory: live}}
	if got := store.ListBins(); !reflect.DeepEqual(got, want) {
		t.Errorf("ListBins() after prune = %v, want %v", got, want)
	}
	persisted := persistedBins(t, paths)
	if len(persisted) != 2 {
		t.Errorf("persisted bins after prune = %v, want named and live directory bins only", persisted)
	}

	again, err := store.PruneDirectoryBins()
	if err != nil {
		t.Fatalf("second PruneDirectoryBins: %v", err)
	}
	if len(again) != 0 {
		t.Errorf("second prune removed %v, want nothing", again)
	}
}

func TestPruneDirectoryBinsReloadsBeforeRemoving(t *testing.T) {
	// A stale in-memory view must not prune a bin whose directory was
	// recreated by another process after this store was opened.
	store, _ := openTestStore(t)
	directory := filepath.Join(t.TempDir(), "flapping")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if _, err := store.EnsureDirectoryBin(directory); err != nil {
		t.Fatalf("EnsureDirectoryBin: %v", err)
	}
	if err := os.Remove(directory); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if got := len(store.StaleDirectoryBins()); got != 1 {
		t.Fatalf("StaleDirectoryBins() count = %d, want 1", got)
	}
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatalf("recreate Mkdir: %v", err)
	}

	pruned, err := store.PruneDirectoryBins()
	if err != nil {
		t.Fatalf("PruneDirectoryBins: %v", err)
	}
	if len(pruned) != 0 {
		t.Errorf("pruned %v after directory was recreated, want nothing", pruned)
	}
}

func TestDeleteAndRenameAreVisibleToOtherOpenStores(t *testing.T) {
	first, paths := openTestStore(t)
	if err := first.EnsureBin("shared"); err != nil {
		t.Fatalf("EnsureBin: %v", err)
	}
	if err := first.WriteSlot("shared", 1, "value"); err != nil {
		t.Fatalf("WriteSlot: %v", err)
	}
	second, err := Open(paths)
	if err != nil {
		t.Fatalf("Open second: %v", err)
	}

	if err := first.RenameBin("shared", "moved"); err != nil {
		t.Fatalf("RenameBin: %v", err)
	}
	if err := second.WriteSlot("shared", 2, "late"); !errors.Is(err, ErrBinNotFound) {
		t.Errorf("second WriteSlot after rename error = %v, want ErrBinNotFound", err)
	}
	reopened, err := Open(paths)
	if err != nil {
		t.Fatalf("reopen after rename: %v", err)
	}
	if value, exists, err := reopened.ReadSlot("moved", 1); err != nil || !exists || value != "value" {
		t.Errorf("renamed bin on disk = (%q, %t, %v), want (value, true, nil)", value, exists, err)
	}
	if _, exists := reopened.Lookup("shared"); exists {
		t.Error("stale write from second store resurrected the old bin name")
	}

	if err := first.DeleteBin("moved"); err != nil {
		t.Fatalf("DeleteBin: %v", err)
	}
	if err := second.DeleteSlot("moved", 1); !errors.Is(err, ErrBinNotFound) {
		t.Errorf("second DeleteSlot after delete error = %v, want ErrBinNotFound", err)
	}
	if got := len(persistedBins(t, paths)); got != 0 {
		t.Errorf("persisted bins after delete = %d, want 0", got)
	}
}

func directoriesOf(bins []BinInfo) []string {
	directories := make([]string, 0, len(bins))
	for _, info := range bins {
		directories = append(directories, info.Directory)
	}
	return directories
}
