package store

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
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
	if err := store.EnsureBin("default"); err != nil {
		t.Fatalf("EnsureBin default: %v", err)
	}
	if err := store.EnsureBin("myapp"); err != nil {
		t.Fatalf("EnsureBin myapp: %v", err)
	}
	if err := store.WriteSlot("myapp", 3, "hello\nworld"); err != nil {
		t.Fatalf("WriteSlot: %v", err)
	}

	reopened, err := Open(paths)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got, want := reopened.ListBins(), []string{"default", "myapp"}; !reflect.DeepEqual(got, want) {
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

	for _, name := range []string{"", "list", "bad\nname", strings.Repeat("a", maxBinNameLength+1)} {
		if err := store.EnsureBin(name); !errors.Is(err, ErrInvalidBinName) {
			t.Errorf("EnsureBin(%q) error = %v, want ErrInvalidBinName", name, err)
		}
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
