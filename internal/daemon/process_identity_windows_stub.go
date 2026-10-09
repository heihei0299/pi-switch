//go:build !windows

package daemon

import "errors"

func processIdentityWindows(uint32) ProcessIdentity { return ProcessIdentity{} }
func terminateWindowsProcess(DaemonInfo) error {
	return errors.New("Windows process termination is unavailable on this platform")
}
