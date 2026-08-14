package main

import (
	"bytes"
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
		if err == nil || !strings.Contains(err.Error(), "only \"tpb list\"") {
			t.Errorf("run(%v) error = %v, want unavailable-command error", args, err)
		}
	}
}
