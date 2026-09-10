package daemon

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// known-gaps/02(b): the health wait ran out all attempts even when the child was
// already gone. A daemon that dies on startup (port taken, bad argument, lock
// held) is detectable in milliseconds, while a full run-out costs
// 15 × (/healthz + /health, each with a 500ms client timeout, + a 200ms sleep) —
// about 18s when something answers on the port but is not ours, about 3s when
// connections are refused. In both of those the child is dead.
//
// The early exit is tested directly rather than through Start, and that choice is
// deliberate: Start spawns os.Executable(), which under `go test` is this test
// binary, and a re-run test binary stays alive for the whole suite — so "the child
// is gone" is not reachable through Start in this environment. An earlier version
// of this test asserted it through Start and measured 3.03s, i.e. it silently
// proved nothing. A reaped pid reproduces the exact condition instead.

func isolatedDaemonDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PI_SWITCH_CONFIG_DIR", dir)
	t.Setenv("PI_SWITCH_CONFIG", filepath.Join(dir, "config.json"))
	t.Setenv("PI_SWITCH_DB", filepath.Join(dir, "requests.db"))
	return dir
}

func freePort(t *testing.T) uint16 {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}
	return uint16(port)
}

// F1: a child that has exited ends the wait instead of consuming all attempts.
//
// The signal is a closed channel, exactly what Start's Wait goroutine produces.
// A pid-based fixture would NOT reproduce the condition: an unreaped child stays a
// zombie and still answers `kill -0` (verified with ps -o stat= reporting Z), so an
// earlier version of this test passed against an implementation that could never
// fire in production.
func TestWaitForHealth_StopsWhenTheChildIsGone(t *testing.T) {
	isolatedDaemonDir(t)
	info := DaemonInfo{Pid: uint32(os.Getpid()), Host: "127.0.0.1", Port: freePort(t), State: "starting"}
	exited := make(chan struct{})
	close(exited)

	started := time.Now()
	healthy := waitForHealth(info, exited)
	elapsed := time.Since(started)

	if healthy {
		t.Fatal("a daemon that is already gone was reported healthy")
	}
	// One attempt costs ~200ms (refused connections) or ~1.2s (probes that time
	// out); consuming all 15 costs seconds. The bound is loose on purpose so a slow
	// machine cannot flake it, while a full run-out still fails it.
	if elapsed > 2*time.Second {
		t.Fatalf("the wait ran to completion (%s) although the child was already gone", elapsed)
	}
}

// F2: a live pid still gets the full wait — the early exit must not turn into
// "give up immediately", which would break slow-starting daemons.
func TestWaitForHealth_DoesNotSkipALiveChild(t *testing.T) {
	isolatedDaemonDir(t)
	info := DaemonInfo{Pid: uint32(os.Getpid()), Host: "127.0.0.1", Port: freePort(t), State: "starting"}
	exited := make(chan struct{}) // never closed: the child is still starting

	started := time.Now()
	healthy := waitForHealth(info, exited)
	elapsed := time.Since(started)

	if healthy {
		t.Fatal("nothing served health, yet the wait reported success")
	}
	// A live child must still get every attempt: ~3s when connections are refused.
	if elapsed < 2500*time.Millisecond {
		t.Fatalf("a live child was only waited for %s; the early exit fired too eagerly", elapsed)
	}
}

// F3: the port-in-use condition is a typed error, so callers stop matching text.
// The condition is injected through the daemon log, which is what Start inspects —
// the same fixture shape the server-level test already used.
func TestStart_PortInUseIsATypedError(t *testing.T) {
	dir := isolatedDaemonDir(t)
	port := freePort(t)
	log := filepath.Join(dir, "proxy.log")
	if err := os.WriteFile(log, []byte("listen tcp 127.0.0.1:"+strconv.Itoa(int(port))+": bind: address already in use\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Start(Proxy, "127.0.0.1", port)
	if err == nil {
		t.Fatal("Start reported success although the port was reported as taken")
	}
	if !errors.Is(err, ErrPortInUse) {
		t.Fatalf("error %v does not carry ErrPortInUse, so callers must guess from its text", err)
	}
	// The message operators already rely on must survive the typed error.
	msg := err.Error()
	for _, want := range []string{"port already in use", "ss -tlnp"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q lost %q", msg, want)
		}
	}
}
