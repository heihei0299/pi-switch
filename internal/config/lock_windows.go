package config

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
)

func tryConfigWriteLock(f *os.File) (bool, error) {
	var overlap windows.Overlapped
	err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlap)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return false, nil
	}
	return err == nil, err
}

func unlockConfigWriteLock(f *os.File) {
	var overlap windows.Overlapped
	_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &overlap)
}
