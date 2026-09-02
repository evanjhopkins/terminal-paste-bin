//go:build linux

package clipboard

import (
	"os"
	"os/exec"
)

// New returns the first available Linux desktop clipboard.
func New() Clipboard {
	if os.Getenv("WAYLAND_DISPLAY") != "" && commandAvailable("wl-paste") && commandAvailable("wl-copy") {
		return newCommandClipboard("wl-paste", []string{"--no-newline"}, "wl-copy", nil)
	}
	if commandAvailable("xclip") {
		return newCommandClipboard("xclip", []string{"-selection", "clipboard", "-o"}, "xclip", []string{"-selection", "clipboard", "-in"})
	}
	if commandAvailable("xsel") {
		return newCommandClipboard("xsel", []string{"--clipboard", "--output"}, "xsel", []string{"--clipboard", "--input"})
	}
	return unavailableClipboard{reason: "install wl-clipboard, xclip, or xsel and run TPB in a graphical session"}
}

// Diagnose reports the availability of a Linux desktop clipboard backend.
func Diagnose() Diagnostic {
	if os.Getenv("WAYLAND_DISPLAY") != "" && commandAvailable("wl-paste") && commandAvailable("wl-copy") {
		return Diagnostic{Status: StatusOK, Backend: "wl-clipboard"}
	}
	if commandAvailable("xclip") {
		return Diagnostic{Status: StatusOK, Backend: "xclip"}
	}
	if commandAvailable("xsel") {
		return Diagnostic{Status: StatusOK, Backend: "xsel"}
	}
	return Diagnostic{Status: StatusUnavailable}
}

func commandAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
