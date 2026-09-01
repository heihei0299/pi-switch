package daemon

import (
	"encoding/json"
	"fmt"
	"net"
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
	Pid       uint32 `json:"pid"`
	Host      string `json:"host"`
	Port      uint16 `json:"port"`
	StartedAt uint64 `json:"startedAt"`
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

func isAlive(pid uint32) bool {
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
			conn.Close()
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

func readPidFile(s Service) *DaemonInfo {
	p := pidPath(s)
	b, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var info DaemonInfo
	if err := json.Unmarshal(b, &info); err != nil {
		return nil
	}
	return &info
}

func writePidFile(s Service, info DaemonInfo) error {
	p := pidPath(s)
	_ = os.MkdirAll(filepath.Dir(p), 0755)
	b, _ := json.Marshal(info)
	return os.WriteFile(p, b, 0644)
}

func removePidFile(s Service) {
	_ = os.Remove(pidPath(s))
}

func Start(s Service, host string, port uint16) (DaemonResult, error) {
	if info := readPidFile(s); info != nil {
		if isAlive(info.Pid) && checkHealth(info.Host, info.Port, 2) {
			msg := fmt.Sprintf("%s daemon already running (PID %d) on http://%s:%d", s.Label, info.Pid, info.Host, info.Port)
			return DaemonResult{Running: true, Pid: &info.Pid, Host: &info.Host, Port: &info.Port, Message: msg}, nil
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
	_ = cmd.Process.Release()
	pid := uint32(cmd.Process.Pid)
	now := uint64(time.Now().UnixMilli())
	info := DaemonInfo{Pid: pid, Host: host, Port: port, StartedAt: now}
	_ = writePidFile(s, info)

	if !checkHealth(host, port, 15) {
		removePidFile(s)
		_ = cmd.Process.Kill()
		// Check log tail for EADDRINUSE
		if data, err := os.ReadFile(lp); err == nil {
			if strings.Contains(strings.ToLower(string(data)), "address already in use") {
				return DaemonResult{}, fmt.Errorf("port already in use — use ss -tlnp to locate (address already in use on %s:%d)", host, port)
			}
		}
		return DaemonResult{}, fmt.Errorf("%s daemon started but failed health check on http://%s:%d. Check %s for errors.", s.Label, host, port, lp)
	}
	msg := fmt.Sprintf("%s daemon started (PID %d) on http://%s:%d", s.Label, pid, host, port)
	return DaemonResult{Running: true, Pid: &pid, Host: &host, Port: &port, StartedAt: &now, Message: msg}, nil
}

func Stop(s Service) (DaemonResult, error) {
	info := readPidFile(s)
	if info == nil {
		return DaemonResult{Running: false, Message: fmt.Sprintf("No %s daemon PID file found", s.Label)}, nil
	}
	if !isAlive(info.Pid) {
		removePidFile(s)
		return DaemonResult{Running: false, Pid: &info.Pid, Message: fmt.Sprintf("PID %d is not alive (cleaned up stale PID)", info.Pid)}, nil
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
		if !checkHealth(info.Host, info.Port, 2) {
			removePidFile(s)
			return DaemonResult{Running: false, Pid: &info.Pid, Message: fmt.Sprintf("%s daemon process exists (PID %d) but port %s:%d is not responding. Cleaned up stale PID.", s.Label, info.Pid, info.Host, info.Port)}, nil
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
