package store

import (
	"path/filepath"
	"testing"
)

func TestPathsForSeparatesProductionAndDevelopment(t *testing.T) {
	configDirectory := t.TempDir()

	production, err := PathsFor(configDirectory, "tpb")
	if err != nil {
		t.Fatalf("PathsFor production: %v", err)
	}
	development, err := PathsFor(configDirectory, "./tpbd")
	if err != nil {
		t.Fatalf("PathsFor development: %v", err)
	}

	if production.Directory != filepath.Join(configDirectory, "tpb") {
		t.Errorf("production directory = %q", production.Directory)
	}
	if development.Directory != filepath.Join(configDirectory, "tpbd") {
		t.Errorf("development directory = %q", development.Directory)
	}
	if production.BinsFile == development.BinsFile || production.ConfigFile == development.ConfigFile {
		t.Error("production and development paths must be distinct")
	}
}

func TestPathsForRejectsUnknownExecutable(t *testing.T) {
	_, err := PathsFor(t.TempDir(), "other")
	if err == nil {
		t.Fatal("PathsFor succeeded for an unknown executable")
	}
}
