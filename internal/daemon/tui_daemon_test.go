package daemon

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// S4 daemon pid/log 与 isAlive/tasklist — configDir PI_SWITCH_CONFIG_DIR||~/.pi-switch → proxy.pid/webui.pid(JSON DaemonInfo{pid,host,port,startedAt}) + proxy.log/webui.log，isAlive: kill -0 / tasklist CSV第2列，checkHealth DialTimeout 500ms×2

func TestTuiDaemon_S4_ConfigDirAndPidLog(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PI_SWITCH_CONFIG_DIR", dir)
	if configDir() != dir {
		t.Fatalf("configDir want %q got %q", dir, configDir())
	}
	// pidPath and logPath should be inside configDir
	if !strings.HasPrefix(pidPath(Proxy), dir) || !strings.HasSuffix(pidPath(Proxy), "proxy.pid") {
		t.Fatalf("pidPath proxy = %q", pidPath(Proxy))
	}
	if !strings.HasPrefix(pidPath(WebUI), dir) || !strings.HasSuffix(pidPath(WebUI), "webui.pid") {
		t.Fatalf("pidPath webui = %q", pidPath(WebUI))
	}
	if !strings.HasSuffix(logPath(Proxy), "proxy.log") {
		t.Fatalf("logPath proxy = %q", logPath(Proxy))
	}
	if !strings.HasSuffix(logPath(WebUI), "webui.log") {
		t.Fatalf("logPath webui = %q", logPath(WebUI))
	}
	// writePidFile / readPidFile JSON DaemonInfo fields
	info := DaemonInfo{Pid: 12345, Host: "127.0.0.1", Port: 43112, StartedAt: 999}
	if err := writePidFile(Proxy, info); err != nil {
		t.Fatalf("writePidFile: %v", err)
	}
	got := readPidFile(Proxy)
	if got == nil || got.Pid != 12345 || got.Host != "127.0.0.1" || got.Port != 43112 || got.StartedAt != 999 {
		t.Fatalf("readPidFile got %+v want %+v", got, info)
	}
	// JSON content verify
	b, _ := os.ReadFile(filepath.Join(dir, "proxy.pid"))
	var raw map[string]interface{}
	_ = json.Unmarshal(b, &raw)
	if raw["pid"] == nil || raw["host"] == nil || raw["port"] == nil || raw["startedAt"] == nil {
		t.Fatalf("pid JSON missing keys: %s", string(b))
	}
	// log file append behavior (open append)
	lf := logPath(Proxy)
	_ = os.WriteFile(lf, []byte("first\n"), 0644)
	f, _ := os.OpenFile(lf, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	_, _ = f.WriteString("second\n")
	f.Close()
	data, _ := os.ReadFile(lf)
	if !strings.Contains(string(data), "first") || !strings.Contains(string(data), "second") {
		t.Fatalf("log append failed: %q", string(data))
	}
}

func TestTuiDaemon_S4_IsAliveAndTasklist(t *testing.T) {
	// current pid should be alive
	pid := uint32(os.Getpid())
	if !isAlive(pid) {
		t.Fatalf("isAlive current pid %d want true", pid)
	}
	// fake pid should be not alive
	if isAlive(999999) {
		t.Fatalf("isAlive 999999 want false")
	}
	// tasklist CSV second column parsing on windows is hard to test on linux, but we can verify kill -0 path via isAlive logic
	// Just ensure non-existent pid cleans up
	dir := t.TempDir()
	t.Setenv("PI_SWITCH_CONFIG_DIR", dir)
	info := DaemonInfo{Pid: 999999, Host: "127.0.0.1", Port: 43112, StartedAt: 1}
	b, _ := json.Marshal(info)
	_ = os.WriteFile(filepath.Join(dir, "proxy.pid"), b, 0644)
	res, _ := Status(Proxy)
	if res.Running {
		t.Fatalf("Status with fake pid should not running")
	}
	if _, err := os.Stat(filepath.Join(dir, "proxy.pid")); !os.IsNotExist(err) {
		t.Fatalf("stale pid file should be removed")
	}
}

func TestTuiDaemon_S4_CheckHealthDialTimeout500ms2(t *testing.T) {
	// false for unused port
	if checkHealth("127.0.0.1", 59999, 2) {
		t.Fatalf("checkHealth unused port should be false")
	}
	// true for listener
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	addr := ln.Addr().String()
	_, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)
	// at least 2 attempts, should succeed quickly (500ms each + 200ms retry)
	start := time.Now()
	if !checkHealth("127.0.0.1", uint16(port), 2) {
		t.Fatalf("checkHealth listening port should be true")
	}
	elapsed := time.Since(start)
	if elapsed > 2*time.Second {
		t.Fatalf("checkHealth took too long: %v", elapsed)
	}
	// also test attempts=2 means it retries 200ms between attempts; for failing port, should take ~ (500ms timeout +200ms sleep)*?
	// we just ensure it returns false not true
}

// S5 daemon Status/Start/Stop — Status: nil→not running / !isAlive→rm stale / isAlive&&!health→rm stale / isAlive&&health→running+ss -tlnp>1 提示；Start: isAlive&&health→already running 否则 127.0.0.1:43112/43110 → exec.Command→*.log→Release→health 15×200ms, EADDRINUSE→500 port already in use；Stop: Kill→20×100ms poll→rm→超时 kill -9/taskkill /F

func TestTuiDaemon_S5_StatusCases(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PI_SWITCH_CONFIG_DIR", dir)
	// nil -> not running
	res, _ := Status(Proxy)
	if res.Running || !strings.Contains(res.Message, "not running") {
		t.Fatalf("nil pid Status want not running, got %+v", res)
	}
	// !isAlive -> rm stale
	info := DaemonInfo{Pid: 999998, Host: "127.0.0.1", Port: 43112, StartedAt: 1}
	b, _ := json.Marshal(info)
	_ = os.WriteFile(filepath.Join(dir, "proxy.pid"), b, 0644)
	res, _ = Status(Proxy)
	if res.Running {
		t.Fatalf("stale !isAlive should not running")
	}
	if _, err := os.Stat(filepath.Join(dir, "proxy.pid")); !os.IsNotExist(err) {
		t.Fatalf("stale pid not removed")
	}
	// isAlive && !health -> rm stale (current pid but no listener on that port)
	pid := uint32(os.Getpid())
	info2 := DaemonInfo{Pid: pid, Host: "127.0.0.1", Port: 59998, StartedAt: uint64(time.Now().UnixMilli())}
	b, _ = json.Marshal(info2)
	_ = os.WriteFile(filepath.Join(dir, "proxy.pid"), b, 0644)
	res, _ = Status(Proxy)
	if res.Running {
		t.Fatalf("isAlive but !health should not running")
	}
	if !strings.Contains(res.Message, "not responding") || !strings.Contains(res.Message, "Cleaned up stale") {
		t.Fatalf("isAlive !health message want cleaned up stale, got %q", res.Message)
	}
	if _, err := os.Stat(filepath.Join(dir, "proxy.pid")); !os.IsNotExist(err) {
		t.Fatalf("isAlive !health pid not removed")
	}
	// isAlive && health -> running + ss -tlnp>1 prompt (if linux and multiple listeners)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	addr := ln.Addr().String()
	_, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)
	info3 := DaemonInfo{Pid: pid, Host: "127.0.0.1", Port: uint16(port), StartedAt: 1}
	b, _ = json.Marshal(info3)
	_ = os.WriteFile(filepath.Join(dir, "proxy.pid"), b, 0644)
	res, _ = Status(Proxy)
	if !res.Running {
		t.Fatalf("isAlive && health should running, got %+v", res)
	}
	if !strings.Contains(res.Message, "is running") {
		t.Fatalf("running message missing: %q", res.Message)
	}
	// ss -tlnp hint only when count>1, not required to appear, but ensure not panic
}

func TestTuiDaemon_S5_StartAlreadyRunning(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PI_SWITCH_CONFIG_DIR", dir)
	// Start should detect already running via isAlive+health
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	addr := ln.Addr().String()
	_, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)
	pid := uint32(os.Getpid())
	info := DaemonInfo{Pid: pid, Host: "127.0.0.1", Port: uint16(port), StartedAt: 1}
	b, _ := json.Marshal(info)
	_ = os.WriteFile(filepath.Join(dir, "proxy.pid"), b, 0644)
	res, err := Start(Proxy, "127.0.0.1", uint16(port))
	if err != nil {
		t.Fatalf("Start already running should not error: %v", err)
	}
	if !res.Running || !strings.Contains(res.Message, "already running") {
		t.Fatalf("Start already running want already running, got %+v", res)
	}
	// ensure pid file not overwritten with new pid
	got := readPidFile(Proxy)
	if got.Pid != pid {
		t.Fatalf("already running should not overwrite pid file, got %d want %d", got.Pid, pid)
	}
}

func TestTuiDaemon_S5_StartEADDRINUSE(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PI_SWITCH_CONFIG_DIR", dir)
	// Simulate EADDRINUSE via pre-written log tail and health failure.
	// Start spawns os.Executable (which is the test binary, not pi-switch), so it will not listen on the port -> health fails after 15×200ms.
	// We pre-populate proxy.log with "address already in use" so that Start's EADDRINUSE detection should trigger.
	// Find a free port (no listener)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	_, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)
	ln.Close() // now port is free, health will fail
	// Pre-create log with EADDRINUSE tail
	logPath := filepath.Join(dir, "proxy.log")
	_ = os.WriteFile(logPath, []byte("some log\nlisten tcp 127.0.0.1:"+portStr+": bind: address already in use\n"), 0644)
	done := make(chan error, 1)
	go func() {
		_, err := Start(Proxy, "127.0.0.1", uint16(port))
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("Start with EADDRINUSE log should error")
		}
		if !strings.Contains(strings.ToLower(err.Error()), "already in use") && !strings.Contains(err.Error(), "port already in use") {
			t.Fatalf("Start EADDRINUSE want 'port already in use', got %q", err.Error())
		}
		if !strings.Contains(err.Error(), "ss -tlnp") {
			t.Fatalf("EADDRINUSE error should hint ss -tlnp, got %q", err.Error())
		}
	case <-time.After(6 * time.Second):
		t.Fatalf("Start EADDRINUSE timeout >6s")
	}
}

func TestTuiDaemon_S5_StopKillPoll(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PI_SWITCH_CONFIG_DIR", dir)
	// nil -> not running
	res, _ := Stop(Proxy)
	if res.Running || !strings.Contains(res.Message, "No") && !strings.Contains(res.Message, "not running") {
		// Message for nil is "No Proxy daemon PID file found"
		if !strings.Contains(res.Message, "No") {
			t.Fatalf("Stop nil want No PID file, got %q", res.Message)
		}
	}
	// !isAlive -> rm
	info := DaemonInfo{Pid: 999997, Host: "127.0.0.1", Port: 43112, StartedAt: 1}
	b, _ := json.Marshal(info)
	_ = os.WriteFile(filepath.Join(dir, "proxy.pid"), b, 0644)
	res, _ = Stop(Proxy)
	if res.Running {
		t.Fatalf("Stop stale should not running")
	}
	if _, err := os.Stat(filepath.Join(dir, "proxy.pid")); !os.IsNotExist(err) {
		t.Fatalf("Stop stale should remove pid")
	}
	// isAlive -> Kill→20×100ms poll→rm
	// Use current pid but with a child that we can kill? Hard to test without real process.
	// We can test Stop with current pid: it will try to kill current process's pid (itself) – but isAlive true, then Kill will send Interrupt/KILL to self? Dangerous.
	// Instead we test that Stop with a pid that is alive but health false will still go through poll logic without killing self impossible.
	// We'll just verify Stop does not panic for nil and stale cases (already done) and that Stop with alive pid at least attempts.
}

func TestTuiDaemon_S5_DefaultHostPort(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PI_SWITCH_CONFIG_DIR", dir)
	// Start with empty host/port should default to 127.0.0.1:43112 for proxy and 43110 for webui
	// We can't actually start without spawning, but we can test that Status with default port works
	// Instead test that Start normalizes: call Start with ""/0 but with already running detection? We test via spawning with already running case where we pass 0 and expect defaults?
	// Simpler: verify Service definitions
	if Proxy.PidFile != "proxy.pid" || Proxy.LogFile != "proxy.log" || Proxy.Subcommand != "proxy" {
		t.Fatalf("Proxy service mismatch")
	}
	if WebUI.PidFile != "webui.pid" || WebUI.LogFile != "webui.log" {
		t.Fatalf("WebUI service mismatch")
	}
	if ServiceByName("proxy") != &Proxy || ServiceByName("webui") != &WebUI || ServiceByName("unknown") != nil {
		t.Fatalf("ServiceByName mismatch")
	}
	// Test that daemon.Start with empty host/port would use defaults – we can verify by checking that after Start failure due to EADDRINUSE, the error message contains default host/port?
	// Skip detailed, just ensure checkHealth uses 500ms×2
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	if ln != nil {
		addr := ln.Addr().String()
		_, ps, _ := net.SplitHostPort(addr)
		p, _ := strconv.Atoi(ps)
		start := time.Now()
		_ = checkHealth("127.0.0.1", uint16(p), 2)
		elapsed := time.Since(start)
		if elapsed > 1500*time.Millisecond {
			t.Fatalf("checkHealth 2 attempts took too long")
		}
		ln.Close()
	}
	// Verify a failed Start reports the failure and leaves no state behind.
	//
	// This block used to assert the duration of the health wait (a 2.5-4.5s
	// window, i.e. 15 × 200ms). That pinned behavior the P0 batch had already
	// recorded as a defect (F14: the operator waits out the whole health check for
	// a failure that is decided in milliseconds), and the duration is no longer a
	// fixed number: Start now stops as soon as the child is gone, so the elapsed
	// time legitimately depends on the child's lifetime. The early exit has its own
	// test with a controlled fixture (start_failfast_test.go); asserting timing
	// through Start here would measure the test binary's own lifetime instead.
	_ = os.Remove(filepath.Join(dir, "proxy.pid"))
	_ = os.Remove(filepath.Join(dir, "proxy.log"))
	ln2, err := net.Listen("tcp", "127.0.0.1:0")
	if err == nil {
		_, ps, _ := net.SplitHostPort(ln2.Addr().String())
		p, _ := strconv.Atoi(ps)
		ln2.Close() // free port, health will fail
		if _, err := Start(Proxy, "127.0.0.1", uint16(p)); err == nil {
			t.Fatalf("Start reported success although nothing ever served health on port %d", p)
		}
		if _, statErr := os.Stat(filepath.Join(dir, "proxy.pid")); statErr == nil {
			t.Fatal("a failed Start left its pid file behind, so the next start would see a phantom daemon")
		}
	}
}
