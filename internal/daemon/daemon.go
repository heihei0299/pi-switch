package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type Service struct {
	PidFile    string
	LogFile    string
	Subcommand string
	Label      string
}

var Proxy = Service{PidFile: "proxy.pid", LogFile: "proxy.log", Subcommand: "proxy", Label: "Proxy"}
var WebUI = Service{PidFile: "webui.pid", LogFile: "webui.log", Subcommand: "webui", Label: "WebUI"}

// ErrPortInUse marks a start failure caused by another listener owning the port.
// Callers must match it with errors.Is instead of looking for the words in the
// message: the wording is for operators and may change.
var ErrPortInUse = errors.New("port already in use")

func ServiceByName(name string) *Service {
	switch name {
	case "proxy":
		return &Proxy
	case "webui":
		return &WebUI
	default:
		return nil
	}
}

type DaemonInfo struct {
	Pid        uint32 `json:"pid"`
	Host       string `json:"host"`
	Port       uint16 `json:"port"`
	StartedAt  uint64 `json:"startedAt"`
	Executable string `json:"executable,omitempty"`
	StartToken string `json:"startToken,omitempty"`
	State      string `json:"state,omitempty"`
}

type DaemonResult struct {
	Running   bool     `json:"running"`
	Pid       *uint32  `json:"pid,omitempty"`
	Host      *string  `json:"host,omitempty"`
	Port      *uint16  `json:"port,omitempty"`
	Targets   []string `json:"targets,omitempty"`
	Failover  []string `json:"failover,omitempty"`
	StartedAt *uint64  `json:"startedAt,omitempty"`
	Message   string   `json:"message"`
}

type ProcessIdentity struct {
	Executable string
	StartToken string
}

var errOperationLocked = errors.New("daemon operation already in progress")

func configDir() string {
	if p := os.Getenv("PI_SWITCH_CONFIG_DIR"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "/tmp/pi-switch"
	}
	return filepath.Join(home, ".pi-switch")
}

func pidPath(s Service) string {
	return filepath.Join(configDir(), s.PidFile)
}
func logPath(s Service) string {
	return filepath.Join(configDir(), s.LogFile)
}
func lockPath(s Service) string {
	return filepath.Join(configDir(), s.PidFile+".lock")
}

func isAlive(pid uint32) bool {
	if pid == 0 {
		return false
	}
	if runtime.GOOS == "windows" {
		cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/NH", "/FO", "CSV")
		out, err := cmd.Output()
		if err != nil {
			return false
		}
		s := string(out)
		for _, line := range strings.Split(s, "\n") {
			parts := strings.Split(line, ",")
			if len(parts) >= 2 {
				p := strings.Trim(parts[1], "\" \r\n")
				if p == strconv.FormatUint(uint64(pid), 10) {
					return true
				}
			}
		}
		return false
	}
	cmd := exec.Command("kill", "-0", strconv.Itoa(int(pid)))
	return cmd.Run() == nil
}

func checkHealth(host string, port uint16, attempts int) bool {
	for i := 0; i < attempts; i++ {
		addr := net.JoinHostPort(host, strconv.Itoa(int(port)))
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

func checkHealthEndpoint(host string, port uint16, attempts int) bool {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	for i := 0; i < attempts; i++ {
		for _, path := range []string{"/healthz", "/health"} {
			resp, err := client.Get("http://" + net.JoinHostPort(host, strconv.Itoa(int(port))) + path)
			if err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode >= 200 && resp.StatusCode < 300 {
					return true
				}
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

func readPidFile(s Service) *DaemonInfo {
	b, err := os.ReadFile(pidPath(s))
	if err != nil {
		return nil
	}
	var info DaemonInfo
	if err := json.Unmarshal(b, &info); err != nil || info.Pid == 0 {
		return nil
	}
	return &info
}

func writePidFile(s Service, info DaemonInfo) error {
	path := pidPath(s)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	b, err := json.Marshal(info)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0644); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func removePidFile(s Service) {
	_ = os.Remove(pidPath(s))
}

func acquireLock(s Service) (*os.File, error) {
	if err := os.MkdirAll(configDir(), 0755); err != nil {
		return nil, err
	}
	path := lockPath(s)
	for attempt := 0; attempt < 2; attempt++ {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err == nil {
			_, _ = fmt.Fprintf(file, "%d\n", os.Getpid())
			return file, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
		contents, readErr := os.ReadFile(path)
		lockPID, parseErr := strconv.ParseUint(strings.TrimSpace(string(contents)), 10, 32)
		if readErr == nil && parseErr == nil && isAlive(uint32(lockPID)) {
			return nil, errOperationLocked
		}
		_ = os.Remove(path)
	}
	return nil, errOperationLocked
}

func releaseLock(s Service, file *os.File) {
	if file != nil {
		_ = file.Close()
	}
	_ = os.Remove(lockPath(s))
}

func processIdentity(pid uint32) ProcessIdentity {
	identity := ProcessIdentity{}
	if runtime.GOOS == "windows" {
		if pid == uint32(os.Getpid()) {
			identity.Executable, _ = os.Executable()
		}
		if out, err := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH").Output(); err == nil {
			line := strings.TrimSpace(strings.Split(string(out), "\n")[0])
			parts := strings.Split(line, ",")
			if len(parts) > 0 {
				identity.Executable = strings.Trim(parts[0], "\"")
			}
		}
		// PowerShell exposes the creation time even on systems where WMIC is
		// absent. Keep a PID fallback only for minimal Windows images.
		command := fmt.Sprintf("(Get-Process -Id %d).StartTime.ToFileTimeUtc()", pid)
		if out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", command).Output(); err == nil {
			identity.StartToken = strings.TrimSpace(string(out))
		}
		if identity.StartToken == "" {
			identity.StartToken = fmt.Sprintf("pid:%d", pid)
		}
		return identity
	}
	identity.Executable, _ = os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err == nil {
		// The command name may contain spaces and ')' characters; the last ')'
		// terminates field 2. Linux field 22 (starttime) is index 19 after it.
		if end := strings.LastIndexByte(string(stat), ')'); end >= 0 {
			fields := strings.Fields(string(stat)[end+1:])
			if len(fields) > 19 {
				identity.StartToken = fields[19]
			}
		}
	}
	return identity
}

func sameExecutable(left, right string) bool {
	if left == "" || right == "" {
		return left == right
	}
	leftResolved, leftErr := filepath.EvalSymlinks(left)
	rightResolved, rightErr := filepath.EvalSymlinks(right)
	if leftErr == nil {
		left = leftResolved
	}
	if rightErr == nil {
		right = rightResolved
	}
	return filepath.Clean(left) == filepath.Clean(right)
}

func managedProcess(info DaemonInfo) bool {
	// Files written before identity metadata existed remain compatible, but
	// every newly written record must match both executable and start token.
	if info.Executable == "" && info.StartToken == "" {
		return true
	}
	actual := processIdentity(info.Pid)
	if info.Executable == "" || actual.Executable == "" || !sameExecutable(info.Executable, actual.Executable) {
		return false
	}
	if info.StartToken == "" || actual.StartToken == "" || info.StartToken != actual.StartToken {
		return false
	}
	return true
}

func managedHealth(info DaemonInfo, attempts int) bool {
	if info.Executable == "" && info.StartToken == "" {
		return checkHealth(info.Host, info.Port, attempts)
	}
	return checkHealthEndpoint(info.Host, info.Port, attempts)
}

// startHealthAttempts bounds how long Start waits for a freshly spawned daemon.
// One attempt probes both health paths with a 500ms client timeout and then
// sleeps 200ms, so a full run-out is about 18s.
const startHealthAttempts = 15

// waitForHealth polls a newly spawned daemon's health, stopping as soon as the
// child has exited: a daemon that died on startup (port taken, bad argument, lock
// held) cannot start answering later, so the remaining attempts only make the
// operator wait. The failure is already determined in milliseconds.
//
// `exited` is closed by the Wait goroutine in Start (never nil at the only call
// site), and it is the only reliable signal: liveness must NOT be derived from the pid. Verified on this machine —
// after a child exits without being waited for, `ps -o stat=` reports `Z` while
// `kill -0 <pid>` still succeeds, so a pid-based check calls a dead daemon alive
// and the early exit never fires.
func waitForHealth(info DaemonInfo, exited <-chan struct{}) bool {
	for attempt := 0; attempt < startHealthAttempts; attempt++ {
		select {
		case <-exited:
			return false
		default:
		}
		if managedHealth(info, 1) {
			return true
		}
	}
	return false
}

func listeningPID(port uint16) (uint32, bool) {
	if runtime.GOOS != "linux" {
		return 0, false
	}
	if output, err := exec.Command("ss", "-ltnp", "H").Output(); err == nil {
		needle := ":" + strconv.Itoa(int(port))
		for _, line := range strings.Split(string(output), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 4 || !strings.HasSuffix(fields[3], needle) {
				continue
			}
			marker := strings.Index(line, "pid=")
			if marker < 0 {
				continue
			}
			start := marker + len("pid=")
			end := start
			for end < len(line) && line[end] >= '0' && line[end] <= '9' {
				end++
			}
			pid, err := strconv.ParseUint(line[start:end], 10, 32)
			if err == nil {
				return uint32(pid), true
			}
		}
	}
	// Unprivileged ss omits process names. Resolve the TCP socket inode through
	// /proc so a same-user managed child can still be distinguished from an
	// unrelated listener.
	return procNetListenerPID(port)
}

func procNetListenerPID(port uint16) (uint32, bool) {
	inodes := map[string]struct{}{}
	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n")[1:] {
			fields := strings.Fields(line)
			if len(fields) < 10 || fields[3] != "0A" {
				continue
			}
			local := fields[1]
			colon := strings.LastIndexByte(local, ':')
			if colon < 0 {
				continue
			}
			value, err := strconv.ParseUint(local[colon+1:], 16, 16)
			if err == nil && uint16(value) == port {
				inodes[fields[9]] = struct{}{}
			}
		}
	}
	if len(inodes) == 0 {
		return 0, false
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0, false
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.ParseUint(entry.Name(), 10, 32)
		if err != nil {
			continue
		}
		fds, err := os.ReadDir(filepath.Join("/proc", entry.Name(), "fd"))
		if err != nil {
			continue
		}
		for _, fd := range fds {
			target, err := os.Readlink(filepath.Join("/proc", entry.Name(), "fd", fd.Name()))
			if err != nil {
				continue
			}
			if strings.HasPrefix(target, "socket:[") && strings.HasSuffix(target, "]") {
				inode := strings.TrimSuffix(strings.TrimPrefix(target, "socket:["), "]")
				if _, ok := inodes[inode]; ok {
					return uint32(pid), true
				}
			}
		}
	}
	return 0, false
}

func Start(s Service, host string, port uint16) (DaemonResult, error) {
	lock, err := acquireLock(s)
	if err != nil {
		return DaemonResult{}, err
	}
	defer releaseLock(s, lock)

	if info := readPidFile(s); info != nil {
		if isAlive(info.Pid) && managedProcess(*info) && managedHealth(*info, 2) {
			msg := fmt.Sprintf("%s daemon already running (PID %d) on http://%s:%d", s.Label, info.Pid, info.Host, info.Port)
			return DaemonResult{Running: true, Pid: &info.Pid, Host: &info.Host, Port: &info.Port, StartedAt: &info.StartedAt, Message: msg}, nil
		}
		removePidFile(s)
	}
	if host == "" {
		host = "127.0.0.1"
	}
	if port == 0 {
		if s.Subcommand == "webui" {
			port = 43110
		} else {
			port = 43112
		}
	}
	_ = os.MkdirAll(configDir(), 0755)
	lp := logPath(s)
	lf, err := os.OpenFile(lp, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return DaemonResult{}, fmt.Errorf("Failed to open log file: %w", err)
	}
	defer lf.Close()

	exe, err := os.Executable()
	if err != nil {
		return DaemonResult{}, fmt.Errorf("cannot get executable: %w", err)
	}
	cmd := exec.Command(exe, s.Subcommand, "start", "--host", host, "--port", strconv.Itoa(int(port)))
	cmd.Stdin = nil
	cmd.Stdout = lf
	cmd.Stderr = lf
	if err := cmd.Start(); err != nil {
		return DaemonResult{}, fmt.Errorf("Failed to spawn daemon: %w", err)
	}
	// Reap the child, and learn when it exits. cmd.Start leaves a zombie when
	// nobody waits, and a zombie still answers `kill -0`, so this goroutine is what
	// makes "the child is gone" observable at all. It also stops dead daemons from
	// accumulating as zombies for as long as this process lives.
	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()

	pid := uint32(cmd.Process.Pid)
	now := uint64(time.Now().UnixMilli())
	identity := processIdentity(pid)
	info := DaemonInfo{Pid: pid, Host: host, Port: port, StartedAt: now, Executable: identity.Executable, StartToken: identity.StartToken, State: "starting"}
	if err := writePidFile(s, info); err != nil {
		_ = cmd.Process.Kill()
		return DaemonResult{}, fmt.Errorf("cannot write daemon state: %w", err)
	}

	if runtime.GOOS == "linux" {
		if owner, known := listeningPID(info.Port); known && owner != info.Pid {
			removePidFile(s)
			_ = cmd.Process.Kill()
			return DaemonResult{}, fmt.Errorf("%w: unmanaged listener owns %s:%d (PID %d); %s daemon was not registered", ErrPortInUse, host, port, owner, s.Label)
		}
	}
	healthy := waitForHealth(info, exited)
	if healthy && runtime.GOOS == "linux" {
		if owner, known := listeningPID(info.Port); known && owner != info.Pid {
			removePidFile(s)
			_ = cmd.Process.Kill()
			return DaemonResult{}, fmt.Errorf("%w: unmanaged listener owns %s:%d (PID %d); %s daemon was not registered", ErrPortInUse, host, port, owner, s.Label)
		}
	}
	if !healthy {
		removePidFile(s)
		_ = cmd.Process.Kill()
		if data, err := os.ReadFile(lp); err == nil {
			if strings.Contains(strings.ToLower(string(data)), "address already in use") {
				return DaemonResult{}, fmt.Errorf("%w — use ss -tlnp to locate (address already in use on %s:%d)", ErrPortInUse, host, port)
			}
		}
		return DaemonResult{}, fmt.Errorf("%s daemon started but failed health check on http://%s:%d. Check %s for errors.", s.Label, host, port, lp)
	}
	info.State = "running"
	if err := writePidFile(s, info); err != nil {
		_ = cmd.Process.Kill()
		removePidFile(s)
		return DaemonResult{}, fmt.Errorf("cannot finalize daemon state: %w", err)
	}
	// Process.Release is deliberately NOT called: it is documented as the alternative
	// to Wait, and the Wait goroutine above now owns the child. Measured both orders —
	// when Wait has already started it survives Release, but Release *before* Wait makes
	// Wait return within microseconds with `invalid argument`, which would close the
	// `exited` channel while the daemon is still running.
	msg := fmt.Sprintf("%s daemon started (PID %d) on http://%s:%d", s.Label, pid, host, port)
	return DaemonResult{Running: true, Pid: &pid, Host: &host, Port: &port, StartedAt: &now, Message: msg}, nil
}

func Stop(s Service) (DaemonResult, error) {
	lock, err := acquireLock(s)
	if err != nil {
		return DaemonResult{}, err
	}
	defer releaseLock(s, lock)

	info := readPidFile(s)
	if info == nil {
		return DaemonResult{Running: false, Message: fmt.Sprintf("No %s daemon PID file found", s.Label)}, nil
	}
	if !isAlive(info.Pid) {
		removePidFile(s)
		return DaemonResult{Running: false, Pid: &info.Pid, Message: fmt.Sprintf("PID %d is not alive (cleaned up stale PID)", info.Pid)}, nil
	}
	if !managedProcess(*info) {
		removePidFile(s)
		return DaemonResult{Running: false, Pid: &info.Pid, Message: fmt.Sprintf("PID %d identity does not match managed %s daemon (unmanaged listener; cleaned up state)", info.Pid, s.Label)}, nil
	}
	proc, _ := os.FindProcess(int(info.Pid))
	if proc != nil {
		_ = proc.Signal(os.Interrupt)
		if runtime.GOOS != "windows" {
			_ = exec.Command("kill", strconv.Itoa(int(info.Pid))).Run()
		}
	}
	for i := 0; i < 20; i++ {
		time.Sleep(100 * time.Millisecond)
		if !isAlive(info.Pid) {
			removePidFile(s)
			return DaemonResult{Running: false, Pid: &info.Pid, Message: fmt.Sprintf("%s daemon (PID %d) stopped", s.Label, info.Pid)}, nil
		}
	}
	if proc != nil {
		_ = proc.Kill()
	}
	if runtime.GOOS != "windows" {
		_ = exec.Command("kill", "-9", strconv.Itoa(int(info.Pid))).Run()
	} else {
		_ = exec.Command("taskkill", "/F", "/PID", strconv.Itoa(int(info.Pid))).Run()
	}
	removePidFile(s)
	return DaemonResult{Running: false, Pid: &info.Pid, Message: fmt.Sprintf("%s daemon (PID %d) force killed", s.Label, info.Pid)}, nil
}

func Status(s Service) (DaemonResult, error) {
	info := readPidFile(s)
	if info == nil {
		return DaemonResult{Running: false, Message: fmt.Sprintf("%s daemon is not running (no PID file)", s.Label)}, nil
	}
	if isAlive(info.Pid) {
		if !managedProcess(*info) {
			removePidFile(s)
			return DaemonResult{Running: false, Pid: &info.Pid, Message: fmt.Sprintf("%s daemon PID %d identity mismatch; unmanaged listener state cleaned up", s.Label, info.Pid)}, nil
		}
		if !managedHealth(*info, 2) {
			removePidFile(s)
			return DaemonResult{Running: false, Pid: &info.Pid, Message: fmt.Sprintf("%s daemon process exists (PID %d) but port %s:%d is not responding. Cleaned up stale PID.", s.Label, info.Pid, info.Host, info.Port)}, nil
		}
		if runtime.GOOS == "linux" && info.Executable != "" {
			if owner, known := listeningPID(info.Port); known && owner != info.Pid {
				removePidFile(s)
				return DaemonResult{Running: false, Pid: &info.Pid, Message: fmt.Sprintf("%s daemon health endpoint is served by unmanaged listener PID %d; cleaned up state", s.Label, owner)}, nil
			}
		}
		msg := fmt.Sprintf("%s daemon is running (PID %d) on http://%s:%d", s.Label, info.Pid, info.Host, info.Port)
		if runtime.GOOS == "linux" {
			if out, err := exec.Command("ss", "-tlnp").Output(); err == nil {
				count := strings.Count(string(out), fmt.Sprintf(":%d", info.Port))
				if count > 1 {
					msg += fmt.Sprintf(" (note: %d listeners on :%d — use ss -tlnp to locate)", count, info.Port)
				}
			}
		}
		return DaemonResult{Running: true, Pid: &info.Pid, Host: &info.Host, Port: &info.Port, StartedAt: &info.StartedAt, Message: msg}, nil
	}
	removePidFile(s)
	return DaemonResult{Running: false, Pid: &info.Pid, Message: fmt.Sprintf("PID %d is not alive (cleaned up stale PID)", info.Pid)}, nil
}
