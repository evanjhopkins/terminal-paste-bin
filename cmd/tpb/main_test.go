package main

import (
	"bytes"
	"os"
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
	if err := run([]string{"list"}, paths, &output); err != nil {
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

	for _, args := range [][]string{nil, {"myapp"}, {"list", "extra"}} {
		err := run(args, paths, &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), "only \"tpb list\" and \"tpb reset\"") {
			t.Errorf("run(%v) error = %v, want unavailable-command error", args, err)
		}
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
	if err := run([]string{"reset"}, developmentPaths, &output); err != nil {
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
