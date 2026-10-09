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
	return processIdentityWindowsHandle(process)
}

func processIdentityWindowsHandle(process windows.Handle) ProcessIdentity {
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

func terminateWindowsProcess(info DaemonInfo) error {
	access := uint32(windows.PROCESS_QUERY_LIMITED_INFORMATION | windows.PROCESS_TERMINATE | windows.SYNCHRONIZE)
	process, err := windows.OpenProcess(access, false, info.Pid)
	if err != nil {
		return fmt.Errorf("cannot open managed PID %d for termination: %w", info.Pid, err)
	}
	defer windows.CloseHandle(process)
	actual := processIdentityWindowsHandle(process)
	if !processIdentityMatches(ProcessIdentity{Executable: info.Executable, StartToken: info.StartToken}, actual) {
		return fmt.Errorf("PID %d identity changed; refusing to force-kill daemon", info.Pid)
	}
	if err := windows.TerminateProcess(process, 1); err != nil {
		result, waitErr := windows.WaitForSingleObject(process, 0)
		if waitErr == nil && result == windows.WAIT_OBJECT_0 {
			return nil
		}
		return fmt.Errorf("failed to terminate PID %d: %w", info.Pid, err)
	}
	result, err := windows.WaitForSingleObject(process, 5000)
	if err != nil {
		return fmt.Errorf("cannot confirm PID %d exited after termination: %w", info.Pid, err)
	}
	if result != windows.WAIT_OBJECT_0 {
		return fmt.Errorf("cannot confirm PID %d exited after termination (wait result %d)", info.Pid, result)
	}
	return nil
}

func processIdentityDarwin(uint32) ProcessIdentity { return ProcessIdentity{} }
