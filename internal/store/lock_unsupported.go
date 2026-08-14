//go:build !darwin

package store

import (
	"errors"
	"os"
)

var errFileLockingUnsupported = errors.New("file locking is currently supported only on macOS")

func acquireFileLock(string) (*os.File, error) {
	return nil, errFileLockingUnsupported
}

func releaseFileLock(*os.File) error {
	return nil
}
