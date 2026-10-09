//go:build windows

package daemon

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func processIdentityWindows(pid uint32) ProcessIdentity {
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ProcessIdentity{}
	}
	defer windows.CloseHandle(process)

	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(process, &created, &exited, &kernel, &user); err != nil || created.HighDateTime == 0 && created.LowDateTime == 0 {
		return ProcessIdentity{}
	}
	image := make([]uint16, 32768)
	size := uint32(len(image))
	if err := windows.QueryFullProcessImageName(process, 0, &image[0], &size); err != nil {
		return ProcessIdentity{}
	}
	executable := windows.UTF16ToString(image[:size])
	if executable == "" {
		return ProcessIdentity{}
	}
	return ProcessIdentity{
		Executable: executable,
		StartToken: fmt.Sprintf("%d:%d", created.HighDateTime, created.LowDateTime),
	}
}

func processIdentityDarwin(uint32) ProcessIdentity { return ProcessIdentity{} }
