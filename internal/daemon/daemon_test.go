package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestServiceByName(t *testing.T) {
	if ServiceByName("proxy") == nil || ServiceByName("proxy").PidFile != "proxy.pid" {
		t.Fatalf("proxy service mismatch")
	}
	if ServiceByName("webui") == nil || ServiceByName("webui").PidFile != "webui.pid" {
		t.Fatalf("webui service mismatch")
	}
	if ServiceByName("unknown") != nil {
		t.Fatalf("unknown should be nil")
	}
}

func TestPidFileLifecycle_StaleCleanup(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PI_SWITCH_CONFIG_DIR", dir)
	// write stale pid (non-existent pid 999999)
	info := DaemonInfo{Pid: 999999, Host: "127.0.0.1", Port: 43112, StartedAt: 1}
	b, _ := json.Marshal(info)
	_ = os.WriteFile(filepath.Join(dir, "proxy.pid"), b, 0644)

	// Status should detect not alive and clean up
	res, _ := Status(Proxy)
	if res.Running {
		t.Fatalf("stale pid should not be running")
	}
	if _, err := os.Stat(filepath.Join(dir, "proxy.pid")); !os.IsNotExist(err) {
		t.Fatalf("stale pid file should be removed")
	}
	// Stop on missing file
	res2, _ := Stop(Proxy)
	if res2.Running {
		t.Fatalf("stop missing should not running")
	}
}

func TestStop_RemovesStale(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PI_SWITCH_CONFIG_DIR", dir)
	info := DaemonInfo{Pid: 999998, Host: "127.0.0.1", Port: 43110, StartedAt: 1}
	b, _ := json.Marshal(info)
	_ = os.WriteFile(filepath.Join(dir, "webui.pid"), b, 0644)
	res, _ := Stop(WebUI)
	if res.Running {
		t.Fatalf("should not running")
	}
	if _, err := os.Stat(filepath.Join(dir, "webui.pid")); !os.IsNotExist(err) {
		t.Fatalf("should removed")
	}
}

func TestStatus_NoPidFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PI_SWITCH_CONFIG_DIR", dir)
	res, _ := Status(Proxy)
	if res.Running {
		t.Fatalf("no pid should not running")
	}
	if res.Message == "" {
		t.Fatalf("message empty")
	}
}

func TestCheckHealth_False(t *testing.T) {
	if checkHealth("127.0.0.1", 59999, 1) {
		t.Fatalf("should be false for unused port")
	}
}
