package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestMain makes the "daemon child" case deterministic for the tests that call Start.
//
// Start spawns os.Executable(), which under `go test` is this test binary. Without
// this guard a spawned child re-runs the whole suite: it is slow, it re-enters these
// same tests (spawning grandchildren and competing for the daemon lock), and — the
// part that matters — it stays alive for the suite's duration, so "the child is gone",
// the very condition the health-wait early exit is about, was unreachable through
// Start. Any test written through Start was therefore measuring the test binary's own
// lifetime rather than the behavior under test.
//
// Exiting immediately reproduces the real case it stands for: a daemon that dies on
// startup (port taken, bad argument, lock held).
func TestMain(m *testing.M) {
	if len(os.Args) > 2 && (os.Args[1] == "proxy" || os.Args[1] == "webui") && os.Args[2] == "start" {
		if os.Getenv("PI_SWITCH_TEST_DAEMON_CHILD_PORT_IN_USE") == "1" {
			fmt.Fprintf(os.Stderr, "listen tcp 127.0.0.1:%s: bind: address already in use\n", os.Getenv("PI_SWITCH_TEST_DAEMON_CHILD_PORT"))
			pidPath := filepath.Join(os.Getenv("PI_SWITCH_CONFIG_DIR"), os.Args[1]+".pid")
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				if _, err := os.Stat(pidPath); err == nil {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			os.Exit(1)
		}
		if os.Getenv("PI_SWITCH_TEST_HOLD_DAEMON_CHILD") == "1" {
			if path := os.Getenv("PI_SWITCH_TEST_DAEMON_CHILD_PID"); path != "" {
				if err := os.WriteFile(path, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0600); err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(2)
				}
			}
			select {}
		}
		fmt.Fprintln(os.Stderr, "simulated daemon child: exiting immediately so the parent observes a startup failure")
		os.Exit(1)
	}
	os.Exit(m.Run())
}
