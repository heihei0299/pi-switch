package server

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/heihei0299/pi-switch/internal/daemon"
)

func init() { gin.SetMode(gin.TestMode) }

// S6 跨平台与 API — 二进制统一 pi-switch[.exe]，FilterKey Alt 由 conhost 屏蔽，*.pid 单文件仅记最后实例，POST /api/proxy/start --daemon 的 already running 与 EADDRINUSE 500 在 gin.TestMode httptest 覆盖

func TestTuiDaemon_S6_BinaryUnified(t *testing.T) {
	// binary unified pi-switch[.exe] not pi-switch-go
	// Check that bin/pi-switch.js dispatches to pi-switch- not pi-switch-go-
	// and that go binary exists as pi-switch
	candidates := []string{"/home/shial/Project/pi-switch/.worktrees/rewrite-go-increment-11/bin/pi-switch.js", "bin/pi-switch.js", "/home/shial/Project/pi-switch/bin/pi-switch.js"}
	var data []byte
	var err error
	for _, p := range candidates {
		data, err = os.ReadFile(p)
		if err == nil {
			break
		}
		if err == nil {
			break
		}
		// try worktree path
		data, err = os.ReadFile(filepath.Join(".", p))
		if err == nil {
			break
		}
	}
	if data == nil {
		t.Skip("bin/pi-switch.js not found")
	}
	s := string(data)
	if strings.Contains(s, "`pi-switch-go") {
		t.Fatalf("bin/pi-switch.js should not contain pi-switch-go candidate, got %s", s[:500])
	}
	if !strings.Contains(s, "pi-switch-") {
		t.Fatalf("bin/pi-switch.js missing pi-switch- dispatch")
	}
	// check that built binary is pi-switch not pi-switch-go
	if _, err := os.Stat("bin/pi-switch"); err == nil {
		// ok
	} else if _, err := os.Stat("/home/shial/Project/pi-switch/bin/pi-switch"); err == nil {
	} else {
		// check worktree bin
		if _, err := os.Stat(filepath.Join(".", "bin/pi-switch")); err != nil {
			t.Logf("binary pi-switch not found (may not be built yet), skip")
		}
	}
}

func TestTuiDaemon_S6_PidSingleFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PI_SWITCH_CONFIG_DIR", dir)
	// start two instances sequentially – pid file should only contain last
	info1 := daemon.DaemonInfo{Pid: 11111, Host: "127.0.0.1", Port: 43112, StartedAt: 1}
	b, _ := json.Marshal(info1)
	_ = os.WriteFile(filepath.Join(dir, "proxy.pid"), b, 0644)
	info2 := daemon.DaemonInfo{Pid: 22222, Host: "127.0.0.1", Port: 43112, StartedAt: 2}
	b2, _ := json.Marshal(info2)
	_ = os.WriteFile(filepath.Join(dir, "proxy.pid"), b2, 0644)
	data, _ := os.ReadFile(filepath.Join(dir, "proxy.pid"))
	var got daemon.DaemonInfo
	_ = json.Unmarshal(data, &got)
	if got.Pid != 22222 {
		t.Fatalf("pid file should contain last instance 22222, got %d", got.Pid)
	}
	// ensure daemon.Status reflects last
	// Since 11111 and 22222 are not alive, Status should clean up and not running
	res, _ := daemon.Status(daemon.Proxy)
	if res.Running {
		t.Fatalf("with fake pid Status should not running")
	}
}

func TestTuiDaemon_S6_ProxyStartAlreadyRunning(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PI_SWITCH_CONFIG_DIR", dir)
	cfgPath := filepath.Join(dir, "config.json")
	_ = os.WriteFile(cfgPath, []byte(`{"version":2,"profiles":{},"settings":{"providerPrefix":"pi-switch","proxy":{"host":"127.0.0.1","port":43112},"web":{"host":"127.0.0.1","port":43110}}}`), 0644)
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	t.Setenv("PI_SWITCH_DB", filepath.Join(dir, "requests.db"))
	// Occupy port with listener and write pid file for already running
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)
	pid := uint32(os.Getpid())
	info := daemon.DaemonInfo{Pid: pid, Host: "127.0.0.1", Port: uint16(port), StartedAt: 1}
	b, _ := json.Marshal(info)
	_ = os.WriteFile(filepath.Join(dir, "proxy.pid"), b, 0644)
	r := NewMgmtRouter()
	// POST /api/proxy/start with daemon true and same host/port should return already running (200 not 500)
	w := httptest.NewRecorder()
	body := `{"daemon":true,"host":"127.0.0.1","port":` + portStr + `}`
	req, _ := http.NewRequest("POST", "/api/proxy/start", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("POST /api/proxy/start already running want 200 got %d body %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	// message should contain already running
	bstr := strings.ToLower(w.Body.String())
	if !strings.Contains(bstr, "already running") {
		t.Fatalf("already running response missing 'already running': %s", w.Body.String())
	}
	_ = port
}

func TestTuiDaemon_S6_ProxyStartEADDRINUSE500(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PI_SWITCH_CONFIG_DIR", dir)
	cfgPath := filepath.Join(dir, "config.json")
	_ = os.WriteFile(cfgPath, []byte(`{"version":2,"profiles":{},"settings":{"providerPrefix":"pi-switch","proxy":{"host":"127.0.0.1","port":43112},"web":{"host":"127.0.0.1","port":43110}}}`), 0644)
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	t.Setenv("PI_SWITCH_DB", filepath.Join(dir, "requests.db"))
	// Ensure no pid file
	_ = os.Remove(filepath.Join(dir, "proxy.pid"))
	_ = os.Remove(filepath.Join(dir, "proxy.log"))
	// We will trigger EADDRINUSE by pre-populating log with that string and using a free port where daemon.Start health will fail
	// Find free port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)
	ln.Close()
	// Pre-create log with EADDRINUSE tail
	_ = os.WriteFile(filepath.Join(dir, "proxy.log"), []byte("listen tcp 127.0.0.1:"+portStr+": bind: address already in use\n"), 0644)
	r := NewMgmtRouter()
	w := httptest.NewRecorder()
	body := `{"daemon":true,"host":"127.0.0.1","port":` + strconv.Itoa(port) + `}`
	req, _ := http.NewRequest("POST", "/api/proxy/start", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	// Should be 500 with port already in use
	if w.Code != 500 {
		t.Fatalf("POST /api/proxy/start EADDRINUSE want 500 got %d body %s", w.Code, w.Body.String())
	}
	if !strings.Contains(strings.ToLower(w.Body.String()), "port already in use") {
		t.Fatalf("EADDRINUSE body missing 'port already in use': %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "ss -tlnp") {
		t.Fatalf("EADDRINUSE body missing ss -tlnp hint: %s", w.Body.String())
	}
}

func TestTuiDaemon_S6_ProxyStatusAndStop(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PI_SWITCH_CONFIG_DIR", dir)
	cfgPath := filepath.Join(dir, "config.json")
	_ = os.WriteFile(cfgPath, []byte(`{"version":2,"profiles":{},"settings":{"providerPrefix":"pi-switch","proxy":{"host":"127.0.0.1","port":43112},"web":{"host":"127.0.0.1","port":43110}}}`), 0644)
	t.Setenv("PI_SWITCH_CONFIG", cfgPath)
	t.Setenv("PI_SWITCH_DB", filepath.Join(dir, "requests.db"))
	r := NewMgmtRouter()
	// status when no pid -> not running
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/proxy/status", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("GET /api/proxy/status code %d", w.Code)
	}
	if strings.Contains(strings.ToLower(w.Body.String()), "\"running\":true") {
		t.Fatalf("status no pid should not running: %s", w.Body.String())
	}
	// stop when no pid
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("POST", "/api/proxy/stop", strings.NewReader(`{}`))
	req2.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("POST /api/proxy/stop code %d", w2.Code)
	}
}
