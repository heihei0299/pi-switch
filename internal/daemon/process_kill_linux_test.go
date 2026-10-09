//go:build linux

package daemon

import (
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestStopSignalProcess(t *testing.T) {
	if os.Getenv("PI_SWITCH_TEST_SIGNAL_PROCESS") != "1" {
		return
	}
	ready := os.Getenv("PI_SWITCH_TEST_SIGNAL_READY")
	log := os.Getenv("PI_SWITCH_TEST_SIGNAL_LOG")
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	defer signal.Stop(signals)
	if err := os.WriteFile(ready, []byte("ready"), 0600); err != nil {
		t.Fatal(err)
	}
	for sig := range signals {
		file, err := os.OpenFile(log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		_, writeErr := file.WriteString(sig.String() + "\n")
		_ = file.Close()
		if writeErr != nil {
			t.Fatal(writeErr)
		}
		if os.Getenv("PI_SWITCH_TEST_IGNORE_INTERRUPT") != "1" {
			return
		}
	}
}

func startStopSignalProcess(t *testing.T, ignoreInterrupt bool) (uint32, <-chan struct{}, string) {
	t.Helper()
	dir := t.TempDir()
	ready := filepath.Join(dir, "ready")
	log := filepath.Join(dir, "signals")
	cmd := exec.Command(os.Args[0], "-test.run=^TestStopSignalProcess$")
	ignore := "0"
	if ignoreInterrupt {
		ignore = "1"
	}
	cmd.Env = append(os.Environ(),
		"PI_SWITCH_TEST_SIGNAL_PROCESS=1",
		"PI_SWITCH_TEST_SIGNAL_READY="+ready,
		"PI_SWITCH_TEST_SIGNAL_LOG="+log,
		"PI_SWITCH_TEST_IGNORE_INTERRUPT="+ignore,
	)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := uint32(cmd.Process.Pid)
	exited := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(exited)
	}()
	t.Cleanup(func() {
		select {
		case <-exited:
		default:
			_ = cmd.Process.Kill()
			<-exited
		}
	})
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(ready); err == nil {
			return pid, exited, log
		}
		select {
		case <-exited:
			t.Fatal("signal test process exited before becoming ready")
		case <-time.After(10 * time.Millisecond):
		}
	}
	t.Fatal("signal test process did not become ready")
	return 0, nil, ""
}

func TestStopForceKillsWithOneGracefulSignal(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PI_SWITCH_CONFIG_DIR", dir)
	pid, exited, signalLog := startStopSignalProcess(t, true)
	identity := processIdentity(pid)
	info := DaemonInfo{Pid: pid, Host: "127.0.0.1", Port: 43112, Executable: identity.Executable, StartToken: identity.StartToken}
	if err := writePidFile(Proxy, info); err != nil {
		t.Fatal(err)
	}

	result, err := Stop(Proxy)
	if err != nil || result.Running || !strings.Contains(result.Message, "force killed") {
		t.Fatalf("Stop() = %+v, %v; want force-killed result", result, err)
	}
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatalf("forced PID %d did not exit", pid)
	}
	data, err := os.ReadFile(signalLog)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), os.Interrupt.String()+"\n"; got != want {
		t.Fatalf("graceful signals = %q, want exactly %q", got, want)
	}
}

func TestForceKillReportsPIDFDSignalFailure(t *testing.T) {
	pid, exited, _ := startStopSignalProcess(t, true)
	identity := processIdentity(pid)
	info := DaemonInfo{Pid: pid, Executable: identity.Executable, StartToken: identity.StartToken}
	pidfd, err := unix.PidfdOpen(int(pid), 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := unix.Close(pidfd); err != nil {
		t.Fatal(err)
	}

	if _, err := forceKillWithPIDFD(Proxy, info, pidfd); err == nil || !strings.Contains(strings.ToLower(err.Error()), "failed to force-terminate") {
		t.Fatalf("force kill error=%v, want signal request failure", err)
	}
	assertProcessSurvived(t, pid, exited)
}

func TestStopCleansPIDForManagedProcessThatAlreadyExited(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PI_SWITCH_CONFIG_DIR", dir)
	pid, exited := startIdentityHoldProcess(t)
	identity := processIdentity(pid)
	info := DaemonInfo{Pid: pid, Host: "127.0.0.1", Port: 43112, Executable: identity.Executable, StartToken: identity.StartToken}
	if err := writePidFile(Proxy, info); err != nil {
		t.Fatal(err)
	}
	proc, err := os.FindProcess(int(pid))
	if err != nil {
		t.Fatal(err)
	}
	if err := proc.Kill(); err != nil {
		t.Fatal(err)
	}
	<-exited

	result, err := Stop(Proxy)
	if err != nil || result.Running {
		t.Fatalf("Stop() for exited PID = %+v, %v", result, err)
	}
	if _, err := os.Stat(pidPath(Proxy)); !os.IsNotExist(err) {
		t.Fatalf("stale PID file remains after exited process cleanup: %v", err)
	}
}

func TestForceKillWithPIDFDRefusesChangedIdentity(t *testing.T) {
	pid, exited, _ := startStopSignalProcess(t, true)
	identity := processIdentity(pid)
	pidfd, err := unix.PidfdOpen(int(pid), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(pidfd)
	info := DaemonInfo{Pid: pid, Executable: identity.Executable, StartToken: identity.StartToken + "-changed"}
	if _, err := forceKillWithPIDFD(Proxy, info, pidfd); err == nil || !strings.Contains(strings.ToLower(err.Error()), "identity changed") {
		t.Fatalf("force kill error=%v, want identity-change refusal", err)
	}
	assertProcessSurvived(t, pid, exited)
}
