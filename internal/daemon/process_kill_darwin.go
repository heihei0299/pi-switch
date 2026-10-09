//go:build darwin

package daemon

import (
	"fmt"
)

func forceKillProcess(s Service, info DaemonInfo) (bool, error) {
	if !managedProcess(info) {
		return false, fmt.Errorf("PID %d identity changed; refusing to force-kill %s daemon", info.Pid, s.Label)
	}
	return false, fmt.Errorf("safe force termination is unavailable on darwin; refusing to kill %s PID %d", s.Label, info.Pid)
}
