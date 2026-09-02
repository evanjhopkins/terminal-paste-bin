//go:build darwin

package clipboard

// New returns the macOS system clipboard.
func New() Clipboard {
	return newCommandClipboard("pbpaste", nil, "pbcopy", nil)
}

// Diagnose reports the availability of the macOS system clipboard.
func Diagnose() Diagnostic {
	return Diagnostic{Status: StatusOK, Backend: "pbcopy/pbpaste"}
}
