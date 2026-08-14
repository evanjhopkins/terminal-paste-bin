//go:build linux

package clipboard

import (
	"os"
	"os/exec"
)

// New returns the first available Linux desktop clipboard reader.
func New() Reader {
	if os.Getenv("WAYLAND_DISPLAY") != "" && commandAvailable("wl-paste") {
		return newCommandClipboard("wl-paste", "--no-newline")
	}
	if commandAvailable("xclip") {
		return newCommandClipboard("xclip", "-selection", "clipboard", "-o")
	}
	if commandAvailable("xsel") {
		return newCommandClipboard("xsel", "--clipboard", "--output")
	}
	return unavailableReader{reason: "install wl-clipboard, xclip, or xsel and run TPB in a graphical session"}
}

func commandAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
