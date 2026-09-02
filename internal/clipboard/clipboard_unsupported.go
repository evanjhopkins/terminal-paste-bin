//go:build !darwin && !linux

package clipboard

// New returns an actionable error on unsupported operating systems.
func New() Clipboard {
	return unavailableClipboard{reason: "TPB clipboard support is currently available on macOS and Linux"}
}

// Diagnose reports that clipboard access is unavailable on this platform.
func Diagnose() Diagnostic {
	return Diagnostic{Status: StatusUnavailable}
}
