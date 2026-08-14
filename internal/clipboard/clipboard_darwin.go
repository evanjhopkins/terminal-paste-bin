//go:build darwin

package clipboard

// New returns the macOS system clipboard reader.
func New() Reader {
	return newCommandClipboard("pbpaste")
}
