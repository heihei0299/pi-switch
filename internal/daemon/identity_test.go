package daemon

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestIdentityHoldProcess(t *testing.T) {
	if os.Getenv("PI_SWITCH_TEST_HOLD_PROCESS") == "1" {
		select {}
	}
}

func TestIsAlivePIDBoundsAndChildExit(t *testing.T) {
	if isAlive(0) {
		t.Fatal("PID 0 must not be considered alive")
	}
	if isAlive(^uint32(0)) {
		t.Fatal("PID above signed pid_t range must not be considered alive")
	}
	if !isAlive(uint32(os.Getpid())) {
		t.Fatal("current process must be considered alive")
	}

	child := exec.Command(os.Args[0], "-test.run=^TestIdentityHoldProcess$")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	childPID := uint32(child.Process.Pid)
	if err := child.Wait(); err != nil {
		t.Fatal(err)
	}
	if isAlive(childPID) {
		t.Fatalf("reaped child PID %d must not be considered alive", childPID)
	}
}

func startIdentityHoldProcess(t *testing.T) (uint32, <-chan struct{}) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestIdentityHoldProcess$")
	cmd.Env = append(os.Environ(), "PI_SWITCH_TEST_HOLD_PROCESS=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
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
	return uint32(cmd.Process.Pid), exited
}

func assertProcessSurvived(t *testing.T, pid uint32, exited <-chan struct{}) {
	t.Helper()
	select {
	case <-exited:
		t.Fatalf("unverified PID %d was terminated", pid)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestLegacyPIDStateIsNotManagedOrStopped(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PI_SWITCH_CONFIG_DIR", dir)
	pid, exited := startIdentityHoldProcess(t)
	info := DaemonInfo{Pid: pid, Host: "127.0.0.1", Port: 43112}
	if managedProcess(info) {
		t.Fatal("legacy PID without identity was trusted")
	}
	if err := writePidFile(Proxy, info); err != nil {
		t.Fatal(err)
	}
	status, err := Status(Proxy)
	if err != nil || status.Running {
		t.Fatalf("Status(%+v) = %+v, %v", info, status, err)
	}
	assertProcessSurvived(t, pid, exited)
	if err := writePidFile(Proxy, info); err != nil {
		t.Fatal(err)
	}
	stopped, err := Stop(Proxy)
	if err != nil || stopped.Running {
		t.Fatalf("Stop(%+v) = %+v, %v", info, stopped, err)
	}
	assertProcessSurvived(t, pid, exited)
}

func TestStopTerminatesManagedProcessAndWaitsForExit(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PI_SWITCH_CONFIG_DIR", dir)
	pid, exited := startIdentityHoldProcess(t)
	identity := processIdentity(pid)
	info := DaemonInfo{Pid: pid, Host: "127.0.0.1", Port: 43112, Executable: identity.Executable, StartToken: identity.StartToken}
	if err := writePidFile(Proxy, info); err != nil {
		t.Fatal(err)
	}
	result, err := Stop(Proxy)
	if err != nil || result.Running {
		t.Fatalf("Stop(%+v) = %+v, %v", info, result, err)
	}
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatalf("Stop returned before managed PID %d exited", pid)
	}
}

func TestStartPromptsForLiveLegacyProxyWithoutStoppingIt(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PI_SWITCH_CONFIG_DIR", dir)
	pid, exited := startIdentityHoldProcess(t)
	if err := writePidFile(Proxy, DaemonInfo{Pid: pid, Host: "127.0.0.1", Port: 43112}); err != nil {
		t.Fatal(err)
	}
	_, err := Start(Proxy, "127.0.0.1", freePort(t))
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "identity") {
		t.Fatalf("Start error=%v, want an identity warning", err)
	}
	assertProcessSurvived(t, pid, exited)
}

func TestForceKillRefusesChangedProcessIdentity(t *testing.T) {
	pid, exited := startIdentityHoldProcess(t)
	identity := processIdentity(pid)
	proc, err := os.FindProcess(int(pid))
	if err != nil {
		t.Fatal(err)
	}
	info := DaemonInfo{Pid: pid, Executable: identity.Executable, StartToken: identity.StartToken + "-reused"}
	if err := forceKillManagedProcess(Proxy, info, proc); err == nil || !strings.Contains(strings.ToLower(err.Error()), "identity changed") {
		t.Fatalf("force kill error=%v, want identity-change refusal", err)
	}
	assertProcessSurvived(t, pid, exited)
}

func TestStartClearsDeadLegacyPIDAndContinues(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PI_SWITCH_CONFIG_DIR", dir)
	if err := writePidFile(Proxy, DaemonInfo{Pid: ^uint32(0), Host: "127.0.0.1", Port: 43112}); err != nil {
		t.Fatal(err)
	}
	_, err := Start(Proxy, "127.0.0.1", freePort(t))
	if err == nil || strings.Contains(strings.ToLower(err.Error()), "existing proxy pid") {
		t.Fatalf("Start error=%v, want startup attempt after stale-state cleanup", err)
	}
	if _, err := os.Stat(pidPath(Proxy)); !os.IsNotExist(err) {
		t.Fatalf("stale legacy PID file remains, stat err=%v", err)
	}
}

func TestStatusRejectsReusedPIDIdentity(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PI_SWITCH_CONFIG_DIR", dir)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_, portText, _ := net.SplitHostPort(listener.Addr().String())
	var port uint16
	for _, r := range portText {
		port = port*10 + uint16(r-'0')
	}
	info := DaemonInfo{
		Pid:        uint32(os.Getpid()),
		Host:       "127.0.0.1",
		Port:       port,
		StartedAt:  1,
		Executable: "/definitely/not/the/current/process",
		StartToken: "reused-pid-token",
		State:      "running",
	}
	payload, _ := json.Marshal(info)
	if err := os.WriteFile(filepath.Join(dir, "proxy.pid"), payload, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := Status(Proxy)
	if err != nil {
		t.Fatal(err)
	}
	if result.Running || !strings.Contains(strings.ToLower(result.Message), "identity") {
		t.Fatalf("reused pid must not be reported as managed daemon: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(dir, "proxy.pid")); !os.IsNotExist(err) {
		t.Fatalf("mismatched pid file should be removed, stat err=%v", err)
	}
}

func TestStartRejectsHealthyUnmanagedListener(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PI_SWITCH_CONFIG_DIR", dir)
	listener := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer listener.Close()
	_, portText, _ := net.SplitHostPort(strings.TrimPrefix(listener.URL, "http://"))
	var port uint16
	for _, r := range portText {
		port = port*10 + uint16(r-'0')
	}
	_, err := Start(Proxy, "127.0.0.1", port)
	if !errors.Is(err, ErrPortInUse) || !strings.Contains(strings.ToLower(err.Error()), "unmanaged listener") {
		t.Fatalf("healthy unmanaged listener error=%v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "proxy.pid")); !os.IsNotExist(statErr) {
		t.Fatalf("unmanaged start must not leave pid state, stat err=%v", statErr)
	}
}

func TestStartDoesNotTrustHealthServedByDifferentProcess(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("listener ownership is checked on Linux")
	}
	dir := t.TempDir()
	t.Setenv("PI_SWITCH_CONFIG_DIR", dir)
	pid, exited := startIdentityHoldProcess(t)
	identity := processIdentity(pid)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer server.Close()
	_, portText, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	var port uint16
	for _, digit := range portText {
		port = port*10 + uint16(digit-'0')
	}
	info := DaemonInfo{Pid: pid, Host: "127.0.0.1", Port: port, Executable: identity.Executable, StartToken: identity.StartToken}
	if err := writePidFile(Proxy, info); err != nil {
		t.Fatal(err)
	}
	if _, err := Start(Proxy, info.Host, info.Port); !errors.Is(err, ErrPortInUse) {
		t.Fatalf("Start accepted another process's health response: %v", err)
	}
	assertProcessSurvived(t, pid, exited)
}

func TestWaitForHealthRejectsChildExitDuringSuccessfulProbe(t *testing.T) {
	exited := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(exited)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	_, portText, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	var port uint16
	for _, digit := range portText {
		port = port*10 + uint16(digit-'0')
	}
	identity := processIdentity(uint32(os.Getpid()))
	info := DaemonInfo{Host: "127.0.0.1", Port: port, Executable: identity.Executable, StartToken: identity.StartToken}
	if waitForHealth(info, exited) {
		t.Fatal("health response after child exit was reported as successful startup")
	}
}

func TestDaemonOperationLockRejectsConcurrentMutation(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PI_SWITCH_CONFIG_DIR", dir)
	first, err := acquireLock(Proxy)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseLock(Proxy, first)
	if _, err := acquireLock(Proxy); !errors.Is(err, errOperationLocked) {
		t.Fatalf("second operation error=%v, want lock conflict", err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestOperationLockRejectsInChild$")
	cmd.Env = append(os.Environ(), "PI_SWITCH_TEST_LOCK_MUST_BLOCK=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("second process did not observe active lock: %v\n%s", err, output)
	}
}

func TestOperationLockRejectsInChild(t *testing.T) {
	if os.Getenv("PI_SWITCH_TEST_LOCK_MUST_BLOCK") != "1" {
		return
	}
	if _, err := acquireLock(Proxy); !errors.Is(err, errOperationLocked) {
		t.Fatalf("lock from another process error=%v, want lock conflict", err)
	}
}

func TestDaemonOperationLockRecoversAfterHolderExits(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PI_SWITCH_CONFIG_DIR", dir)
	first, err := acquireLock(Proxy)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := acquireLock(Proxy)
	if err != nil {
		t.Fatalf("acquire after holder exit: %v", err)
	}
	releaseLock(Proxy, second)
	if _, err := os.Stat(lockPath(Proxy)); err != nil {
		t.Fatalf("lock file should remain reusable: %v", err)
	}
}

func TestOldLockReleaseDoesNotRemoveNewHolder(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PI_SWITCH_CONFIG_DIR", dir)
	old, err := acquireLock(Proxy)
	if err != nil {
		t.Fatal(err)
	}
	releaseLock(Proxy, old)
	current, err := acquireLock(Proxy)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseLock(Proxy, current)
	releaseLock(Proxy, old)
	if _, err := acquireLock(Proxy); !errors.Is(err, errOperationLocked) {
		t.Fatalf("replacement holder lost its lock: %v", err)
	}
}

func TestCurrentProcessIdentityHasStableFields(t *testing.T) {
	pid := uint32(os.Getpid())
	identity := processIdentity(pid)
	if identity.Executable == "" || identity.StartToken == "" {
		t.Fatalf("current process identity incomplete: %+v", identity)
	}
	if strings.HasPrefix(identity.StartToken, "pid:") {
		t.Fatalf("PID alone is not a stable process identity: %+v", identity)
	}
	if !processIdentityMatches(identity, processIdentity(pid)) {
		t.Fatalf("current process identity changed between reads: %+v", identity)
	}
}

func TestProcessIdentityUnavailableForInvalidPID(t *testing.T) {
	if identity := processIdentity(0); identity.Executable != "" || identity.StartToken != "" {
		t.Fatalf("invalid PID should not yield a process identity: %+v", identity)
	}
}

func TestProcessIdentityMatchesRequiresExecutableAndStartToken(t *testing.T) {
	expected := ProcessIdentity{Executable: "pi-switch", StartToken: "123:456"}
	for _, test := range []struct {
		name string
		got  ProcessIdentity
		want bool
	}{
		{name: "match", got: expected, want: true},
		{name: "different executable", got: ProcessIdentity{Executable: "other", StartToken: expected.StartToken}},
		{name: "different start", got: ProcessIdentity{Executable: expected.Executable, StartToken: "123:457"}},
		{name: "unknown", got: ProcessIdentity{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := processIdentityMatches(expected, test.got); got != test.want {
				t.Fatalf("processIdentityMatches(%+v) = %t, want %t", test.got, got, test.want)
			}
		})
	}
}
