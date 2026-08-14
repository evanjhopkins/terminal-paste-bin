//go:build !darwin && !linux

package clipboard

// New returns an actionable error on unsupported operating systems.
func New() Reader {
	return unavailableReader{reason: "TPB clipboard support is currently available on macOS and Linux"}
}
