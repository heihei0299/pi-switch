//go:build linux

package config

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
)

func tryConfigWriteLock(f *os.File) (bool, error) {
	err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EINTR) {
		return false, nil
	}
	return err == nil, err
}

func unlockConfigWriteLock(f *os.File) {
	_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
}
