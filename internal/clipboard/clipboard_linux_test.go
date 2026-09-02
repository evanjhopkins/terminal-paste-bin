//go:build linux

package clipboard

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiagnoseSelectsWaylandBackend(t *testing.T) {
	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "wl-paste"))
	writeExecutable(t, filepath.Join(bin, "wl-copy"))
	t.Setenv("PATH", bin)
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	t.Setenv("DISPLAY", "")

	diag := Diagnose()
	if diag.Status != StatusOK || diag.Backend != "wl-clipboard" {
		t.Errorf("Diagnose() = %+v, want wl-clipboard OK", diag)
	}
}

func TestDiagnoseSelectsXClipBackend(t *testing.T) {
	bin := t.TempDir()
	writeExecutable(t, filepath.Join(bin, "xclip"))
	t.Setenv("PATH", bin)
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", ":0")

	diag := Diagnose()
	if diag.Status != StatusOK || diag.Backend != "xclip" {
		t.Errorf("Diagnose() = %+v, want xclip OK", diag)
	}
}

func TestDiagnoseReportsUnavailableWaylandClipboard(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	t.Setenv("DISPLAY", "")

	diag := Diagnose()
	if diag.Status != StatusUnavailable {
		t.Fatalf("Diagnose() status = %v, want StatusUnavailable", diag.Status)
	}
}

func TestDiagnoseReportsHeadlessSession(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", "")

	diag := Diagnose()
	if diag.Status != StatusUnavailable {
		t.Fatalf("Diagnose() status = %v, want StatusUnavailable", diag.Status)
	}
}

func writeExecutable(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}
