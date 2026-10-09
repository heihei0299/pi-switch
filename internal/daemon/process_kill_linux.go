//go:build linux

package daemon

import (
	"errors"
	"fmt"
	"time"

	"golang.org/x/sys/unix"
)

func forceKillProcess(s Service, info DaemonInfo) (bool, error) {
	pidfd, err := unix.PidfdOpen(int(info.Pid), 0)
	if errors.Is(err, unix.ESRCH) {
		return false, nil
	}
	if errors.Is(err, unix.ENOSYS) {
		return false, fmt.Errorf("safe force termination is unavailable: pidfd_open is unsupported: %w", err)
	}
	if err != nil {
		return false, fmt.Errorf("cannot safely force-terminate %s PID %d (pidfd required): %w", s.Label, info.Pid, err)
	}
	defer unix.Close(pidfd)

	return forceKillWithPIDFD(s, info, pidfd)
}

func waitForPIDFDExit(pidfd int, timeout time.Duration) (bool, error) {
	deadline := time.Now().Add(timeout)
	fds := []unix.PollFd{{Fd: int32(pidfd), Events: unix.POLLIN}}
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return false, nil
		}
		timeoutMS := int(remaining.Milliseconds())
		if timeoutMS == 0 {
			timeoutMS = 1
		}
		n, err := unix.Poll(fds, timeoutMS)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return false, err
		}
		if n == 0 {
			return false, nil
		}
		return fds[0].Revents != 0, nil
	}
}

func forceKillWithPIDFD(s Service, info DaemonInfo, pidfd int) (bool, error) {
	if !managedProcess(info) {
		return false, fmt.Errorf("PID %d identity changed; refusing to force-kill %s daemon", info.Pid, s.Label)
	}
	if err := unix.PidfdSendSignal(pidfd, unix.SIGKILL, nil, 0); err != nil {
		if errors.Is(err, unix.ESRCH) {
			if exited, waitErr := waitForPIDFDExit(pidfd, time.Millisecond); waitErr == nil && exited {
				return false, nil
			}
		}
		if errors.Is(err, unix.ENOSYS) {
			return false, fmt.Errorf("safe force termination is unavailable: pidfd signaling is unsupported: %w", err)
		}
		return false, fmt.Errorf("failed to force-terminate %s PID %d: %w", s.Label, info.Pid, err)
	}
	exited, err := waitForPIDFDExit(pidfd, 5*time.Second)
	if err != nil {
		return false, fmt.Errorf("cannot confirm %s PID %d exit after force termination: %w", s.Label, info.Pid, err)
	}
	if !exited {
		return false, fmt.Errorf("cannot confirm %s PID %d exited after force termination", s.Label, info.Pid)
	}
	return true, nil
}
