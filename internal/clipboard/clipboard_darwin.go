//go:build darwin

package clipboard

// New returns the macOS system clipboard.
func New() Clipboard {
	return newCommandClipboard("pbpaste", nil, "pbcopy", nil)
}
