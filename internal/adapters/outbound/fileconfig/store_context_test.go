package fileconfig_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/heihei0299/pi-switch/internal/adapters/outbound/fileconfig"
	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/usecase/providers"
)

func TestStoreUpdateCancelsWhileWaitingForCrossProcessLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.SaveAtPath(config.DefaultConfig(), path); err != nil {
		t.Fatal(err)
	}

	ready := path + ".ready"
	childCtx, childCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer childCancel()
	child := exec.CommandContext(childCtx, os.Args[0], "-test.run=^TestFileConfigLockHelper$")
	child.Env = append(os.Environ(), "PI_SWITCH_FILECONFIG_LOCK_PATH="+path, "PI_SWITCH_FILECONFIG_LOCK_READY="+ready)
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if child.Process != nil {
			_ = child.Process.Kill()
		}
		_ = child.Wait()
	}()

	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("child did not acquire config lock")
		}
		time.Sleep(5 * time.Millisecond)
	}

	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()
	called := false
	start := time.Now()
	err = fileconfig.New(path, config.DefaultConfig).Update(ctx, false, func(*providers.State) error {
		called = true
		return nil
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Update error = %v, want context deadline exceeded", err)
	}
	if time.Since(start) > time.Second {
		t.Fatalf("Update took %s after context deadline", time.Since(start))
	}
	if called {
		t.Fatal("mutation ran without the config lock")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("canceled update changed config bytes")
	}
}

// TestFileConfigLockHelper runs only in the child process to hold the shared
// lock until the parent kills it.
func TestFileConfigLockHelper(t *testing.T) {
	path := os.Getenv("PI_SWITCH_FILECONFIG_LOCK_PATH")
	if path == "" {
		return
	}
	if err := config.UpdateAtPath(path, func(*config.PiSwitchConfig) error {
		if err := os.WriteFile(os.Getenv("PI_SWITCH_FILECONFIG_LOCK_READY"), nil, 0600); err != nil {
			return err
		}
		select {}
	}); err != nil {
		t.Fatal(err)
	}
}
