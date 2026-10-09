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
	"syscall"
	"time"
)

type Service struct {
	PidFile    string
	LogFile    string
	Subcommand string
	Label      string
	Executable string // Optional process executable; defaults to the current executable.
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
	// Reject values that cannot fit the supported platforms' signed pid_t before converting to int.
	if pid == 0 || pid > 2_147_483_647 {
		return false
	}
	err := syscall.Kill(int(pid), 0)
	return err == nil || errors.Is(err, syscall.EPERM)
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

func removePidFileIfPID(s Service, pid uint32) {
	if info := readPidFile(s); info != nil && info.Pid == pid {
		removePidFile(s)
	}
}

func acquireLock(s Service) (*os.File, error) {
	if err := os.MkdirAll(configDir(), 0755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(lockPath(s), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := lockOperationFile(file); err != nil {
		_ = file.Close()
		if errors.Is(err, errOperationLocked) {
			return nil, errOperationLocked
		}
		return nil, fmt.Errorf("cannot lock daemon operation: %w", err)
	}
	return file, nil
}

func releaseLock(_ Service, file *os.File) {
	if file != nil {
		_ = unlockOperationFile(file)
		_ = file.Close()
	}
}

func processIdentity(pid uint32) ProcessIdentity {
	identity := ProcessIdentity{}
	if runtime.GOOS == "darwin" {
		return processIdentityDarwin(pid)
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

func processIdentityMatches(expected, actual ProcessIdentity) bool {
	return expected.Executable != "" && actual.Executable != "" &&
		sameExecutable(expected.Executable, actual.Executable) &&
		expected.StartToken != "" && actual.StartToken != "" &&
		expected.StartToken == actual.StartToken
}

func managedProcess(info DaemonInfo) bool {
	if info.Executable == "" || info.StartToken == "" {
		return false
	}
	return processIdentityMatches(
		ProcessIdentity{Executable: info.Executable, StartToken: info.StartToken},
		processIdentity(info.Pid),
	)
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
			select {
			case <-exited:
				return false
			default:
				return true
			}
		}
	}
	return false
}

func terminateStartedChild(cmd *exec.Cmd, exited <-chan struct{}) error {
	select {
	case <-exited:
		return nil
	default:
	}
	killErr := cmd.Process.Kill()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case <-exited:
		return nil
	case <-timer.C:
		if killErr != nil {
			return fmt.Errorf("could not terminate startup child PID %d: %w; exit was not confirmed", cmd.Process.Pid, killErr)
		}
		return fmt.Errorf("termination was requested for startup child PID %d, but exit was not confirmed", cmd.Process.Pid)
	}
}

func failedStart(s Service, pid uint32, cmd *exec.Cmd, exited <-chan struct{}, cause error) error {
	if cleanupErr := terminateStartedChild(cmd, exited); cleanupErr != nil {
		return fmt.Errorf("%w; startup child cleanup failed: %v", cause, cleanupErr)
	}
	removePidFileIfPID(s, pid)
	return cause
}

func portInUseStartError(logPath string, logStart int64, host string, port uint16) error {
	data, err := os.ReadFile(logPath)
	if err != nil || int64(len(data)) < logStart {
		return nil
	}
	currentLog := strings.ToLower(string(data[logStart:]))
	if !strings.Contains(currentLog, net.JoinHostPort(host, strconv.Itoa(int(port)))) || !strings.Contains(currentLog, "address already in use") {
		return nil
	}
	return fmt.Errorf("%w — use ss -tlnp to locate (address already in use on %s:%d)", ErrPortInUse, host, port)
}

func exitedStartError(label, logPath string, exited <-chan struct{}, childExit <-chan error) error {
	select {
	case <-exited:
		if err := <-childExit; err != nil {
			return fmt.Errorf("%s daemon exited before becoming healthy: %w. Check %s for errors.", label, err, logPath)
		}
		return fmt.Errorf("%s daemon exited before becoming healthy. Check %s for errors.", label, logPath)
	default:
		return nil
	}
}

func listeningPID(port uint16) (uint32, bool) {
	if runtime.GOOS != "linux" {
		return 0, false
	}
	if output, err := exec.Command("ss", "-ltnp", "-H").Output(); err == nil {
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

func checkPortAvailable(s Service, host string, port uint16) error {
	address := net.JoinHostPort(host, strconv.Itoa(int(port)))
	listener, err := net.Listen("tcp", address)
	if err == nil {
		return listener.Close()
	}
	if !errors.Is(err, syscall.EADDRINUSE) {
		return fmt.Errorf("cannot verify that %s is available: %w", address, err)
	}
	if owner, known := listeningPID(port); known {
		return fmt.Errorf("%w: unmanaged listener owns %s (PID %d); refusing to start %s; use ss -tlnp to locate", ErrPortInUse, address, owner, s.Label)
	}
	return fmt.Errorf("%w: %s is occupied by an unverified listener; refusing to start %s; use ss -tlnp to locate", ErrPortInUse, address, s.Label)
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

func serviceExecutable(s Service) (string, error) {
	if s.Executable != "" {
		return s.Executable, nil
	}
	return os.Executable()
}

func Start(s Service, host string, port uint16) (DaemonResult, error) {
	lock, err := acquireLock(s)
	if err != nil {
		return DaemonResult{}, err
	}
	defer releaseLock(s, lock)

	if info := readPidFile(s); info != nil {
		alive := isAlive(info.Pid)
		if alive && managedProcess(*info) && managedHealth(*info, 2) {
			if runtime.GOOS != "linux" {
				msg := fmt.Sprintf("%s daemon already running (PID %d) on http://%s:%d", s.Label, info.Pid, info.Host, info.Port)
				return DaemonResult{Running: true, Pid: &info.Pid, Host: &info.Host, Port: &info.Port, StartedAt: &info.StartedAt, Message: msg}, nil
			}
			if owner, known := listeningPID(info.Port); known && owner == info.Pid {
				msg := fmt.Sprintf("%s daemon already running (PID %d) on http://%s:%d", s.Label, info.Pid, info.Host, info.Port)
				return DaemonResult{Running: true, Pid: &info.Pid, Host: &info.Host, Port: &info.Port, StartedAt: &info.StartedAt, Message: msg}, nil
			}
		}
		removePidFile(s)
		if alive && s.Subcommand == "proxy" && (info.Executable == "" || info.StartToken == "") {
			return DaemonResult{}, fmt.Errorf("existing Proxy PID %d has no verifiable process identity; it was not stopped. Inspect it and stop it manually before starting another Proxy", info.Pid)
		}
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
	if err := checkPortAvailable(s, host, port); err != nil {
		return DaemonResult{}, err
	}
	_ = os.MkdirAll(configDir(), 0755)
	lp := logPath(s)
	lf, err := os.OpenFile(lp, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return DaemonResult{}, fmt.Errorf("Failed to open log file: %w", err)
	}
	defer lf.Close()
	logInfo, err := lf.Stat()
	if err != nil {
		return DaemonResult{}, fmt.Errorf("cannot inspect log file: %w", err)
	}
	logStart := logInfo.Size()

	exe, err := serviceExecutable(s)
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
	childExit := make(chan error, 1)
	go func() {
		childExit <- cmd.Wait()
		close(exited)
	}()

	pid := uint32(cmd.Process.Pid)
	now := uint64(time.Now().UnixMilli())
	identity := processIdentity(pid)
	if identity.Executable == "" || identity.StartToken == "" {
		cause := portInUseStartError(lp, logStart, host, port)
		if cause == nil {
			cause = exitedStartError(s.Label, lp, exited, childExit)
		}
		if cause == nil {
			cause = fmt.Errorf("cannot verify %s daemon process identity; refusing to register it", s.Label)
		}
		return DaemonResult{}, failedStart(s, pid, cmd, exited, cause)
	}
	info := DaemonInfo{Pid: pid, Host: host, Port: port, StartedAt: now, Executable: identity.Executable, StartToken: identity.StartToken, State: "starting"}
	if err := writePidFile(s, info); err != nil {
		cause := fmt.Errorf("cannot write daemon state: %w", err)
		return DaemonResult{}, failedStart(s, pid, cmd, exited, cause)
	}

	if runtime.GOOS == "linux" {
		if owner, known := listeningPID(info.Port); known && owner != info.Pid {
			cause := fmt.Errorf("%w: unmanaged listener owns %s:%d (PID %d); %s daemon was not registered", ErrPortInUse, host, port, owner, s.Label)
			return DaemonResult{}, failedStart(s, pid, cmd, exited, cause)
		}
	}
	healthy := waitForHealth(info, exited)
	if healthy && runtime.GOOS == "linux" {
		if owner, known := listeningPID(info.Port); !known || owner != info.Pid {
			var cause error
			if known {
				cause = fmt.Errorf("%w: unmanaged listener owns %s:%d (PID %d); %s daemon was not registered", ErrPortInUse, host, port, owner, s.Label)
			} else {
				cause = fmt.Errorf("%w: listener on %s:%d has an unverified owner; %s daemon was not registered", ErrPortInUse, host, port, s.Label)
			}
			return DaemonResult{}, failedStart(s, pid, cmd, exited, cause)
		}
	}
	if !healthy {
		cause := portInUseStartError(lp, logStart, host, port)
		if cause == nil {
			cause = exitedStartError(s.Label, lp, exited, childExit)
		}
		if cause == nil {
			cause = fmt.Errorf("%s daemon health check timed out on http://%s:%d. Check %s for errors.", s.Label, host, port, lp)
		}
		return DaemonResult{}, failedStart(s, pid, cmd, exited, cause)
	}
	info.State = "running"
	if err := writePidFile(s, info); err != nil {
		cause := fmt.Errorf("cannot finalize daemon state: %w", err)
		return DaemonResult{}, failedStart(s, pid, cmd, exited, cause)
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
		return DaemonResult{Running: false, Pid: &info.Pid, Message: fmt.Sprintf("PID %d identity does not match managed %s daemon; refusing to stop it and cleaned up state", info.Pid, s.Label)}, nil
	}
	proc, err := os.FindProcess(int(info.Pid))
	if err != nil {
		return DaemonResult{}, fmt.Errorf("cannot access managed %s PID %d: %w", s.Label, info.Pid, err)
	}
	if err := proc.Signal(os.Interrupt); err != nil {
		if !isAlive(info.Pid) {
			removePidFile(s)
			return DaemonResult{Running: false, Pid: &info.Pid, Message: fmt.Sprintf("%s daemon (PID %d) stopped", s.Label, info.Pid)}, nil
		}
		return DaemonResult{}, fmt.Errorf("failed to request stop of %s PID %d: %w", s.Label, info.Pid, err)
	}
	for i := 0; i < 20; i++ {
		time.Sleep(100 * time.Millisecond)
		if !isAlive(info.Pid) {
			removePidFile(s)
			return DaemonResult{Running: false, Pid: &info.Pid, Message: fmt.Sprintf("%s daemon (PID %d) stopped", s.Label, info.Pid)}, nil
		}
	}
	forced, err := forceKillManagedProcess(s, *info)
	if err != nil {
		return DaemonResult{}, err
	}
	removePidFile(s)
	message := fmt.Sprintf("%s daemon (PID %d) stopped", s.Label, info.Pid)
	if forced {
		message = fmt.Sprintf("%s daemon (PID %d) force killed", s.Label, info.Pid)
	}
	return DaemonResult{Running: false, Pid: &info.Pid, Message: message}, nil
}

func forceKillManagedProcess(s Service, info DaemonInfo) (bool, error) {
	return forceKillProcess(s, info)
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
