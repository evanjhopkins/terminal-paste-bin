package store

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	productionCommand  = "tpb"
	developmentCommand = "tpbd"
)

// Paths identifies the files used to persist one TPB environment.
type Paths struct {
	Directory  string
	BinsFile   string
	ConfigFile string
}

// DefaultPaths resolves the storage directory from the user's OS configuration
// directory and the executable name.
func DefaultPaths(executable string) (Paths, error) {
	configDirectory, err := os.UserConfigDir()
	if err != nil {
		return Paths{}, fmt.Errorf("get user config directory: %w", err)
	}

	return PathsFor(configDirectory, executable)
}

// PathsFor resolves paths from a supplied configuration root. It is separate
// from DefaultPaths so callers can use temporary directories in tests.
func PathsFor(configDirectory, executable string) (Paths, error) {
	if configDirectory == "" {
		return Paths{}, fmt.Errorf("configuration directory is required")
	}

	directoryName, err := directoryNameFor(executable)
	if err != nil {
		return Paths{}, err
	}

	directory := filepath.Join(configDirectory, directoryName)
	return Paths{
		Directory:  directory,
		BinsFile:   filepath.Join(directory, "bins.json"),
		ConfigFile: filepath.Join(directory, "config.json"),
	}, nil
}

func directoryNameFor(executable string) (string, error) {
	switch filepath.Base(executable) {
	case productionCommand:
		return productionCommand, nil
	case developmentCommand:
		return developmentCommand, nil
	default:
		return "", fmt.Errorf("unsupported executable name %q", executable)
	}
}
