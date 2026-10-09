//go:build darwin

package daemon

import (
	"fmt"
	"strings"

	"golang.org/x/sys/unix"
)

func processIdentityDarwin(pid uint32) ProcessIdentity {
	info, err := unix.SysctlKinfoProc("kern.proc.pid", int(pid))
	if err != nil || info.Proc.P_pid != int32(pid) {
		return ProcessIdentity{}
	}
	name := strings.TrimRight(string(info.Proc.P_comm[:]), "\x00")
	if name == "" {
		return ProcessIdentity{}
	}
	start := info.Proc.P_starttime
	return ProcessIdentity{
		Executable: name,
		StartToken: fmt.Sprintf("%d:%d", start.Sec, start.Usec),
	}
}
