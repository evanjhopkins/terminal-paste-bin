//go:build !darwin && !linux

package clipboard

// New returns an actionable error on unsupported operating systems.
func New() Clipboard {
	return unavailableClipboard{reason: "TPB clipboard support is currently available on macOS and Linux"}
}
