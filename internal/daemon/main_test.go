package daemon

import (
	"fmt"
	"os"
	"testing"
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
		fmt.Fprintln(os.Stderr, "simulated daemon child: exiting immediately so the parent observes a startup failure")
		os.Exit(1)
	}
	os.Exit(m.Run())
}
