//go:build darwin || linux

package store

import (
	"os"
	"syscall"
)

func acquireFileLock(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, lockError("open", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, lockError("set permissions on", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		_ = file.Close()
		return nil, lockError("acquire", err)
	}
	return file, nil
}

func releaseFileLock(file *os.File) error {
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_UN); err != nil {
		_ = file.Close()
		return lockError("release", err)
	}
	if err := file.Close(); err != nil {
		return lockError("close", err)
	}
	return nil
}
