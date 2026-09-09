package daemon

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

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
	if runtime.GOOS != "linux" {
		t.Skip("listener ownership assertion uses the Linux /proc adapter")
	}
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
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "unmanaged listener") {
		t.Fatalf("healthy unmanaged listener error=%v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "proxy.pid")); !os.IsNotExist(statErr) {
		t.Fatalf("unmanaged start must not leave pid state, stat err=%v", statErr)
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
}

func TestCurrentProcessIdentityHasStableFields(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("process start token adapters are covered by platform CI")
	}
	identity := processIdentity(uint32(os.Getpid()))
	if identity.Executable == "" || identity.StartToken == "" {
		t.Fatalf("current process identity incomplete: %+v", identity)
	}
}
