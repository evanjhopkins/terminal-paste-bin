package store

import "fmt"

func withFileLock(path string, action func() error) error {
	lock, err := acquireFileLock(path)
	if err != nil {
		return err
	}

	actionErr := action()
	releaseErr := releaseFileLock(lock)
	if actionErr != nil {
		return actionErr
	}
	if releaseErr != nil {
		return releaseErr
	}
	return nil
}

func lockError(operation string, err error) error {
	return fmt.Errorf("%s bins lock: %w", operation, err)
}
