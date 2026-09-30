package config_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/heihei0299/pi-switch/internal/config"
)

func TestUpdateAtPathFailureDoesNotWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.SaveAtPath(config.DefaultConfig(), path); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rejected := errors.New("rejected")
	err = config.UpdateAtPath(path, func(cfg *config.PiSwitchConfig) error {
		cfg.Profiles = nil
		return rejected
	})
	if !errors.Is(err, rejected) {
		t.Fatalf("error = %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("rejected mutation changed config")
	}
	if err := config.UpdateAtPath(path, func(*config.PiSwitchConfig) error { return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateAtPathAcrossProcesses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := config.DefaultConfig()
	cfg.Profiles = map[string]config.ProviderProfile{}
	if err := config.SaveAtPath(cfg, path); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	const processes = 8
	cmds := make([]*exec.Cmd, processes)
	outputs := make([]bytes.Buffer, processes)
	gate := path + ".go"
	for i := range cmds {
		cmds[i] = exec.CommandContext(ctx, os.Args[0], "-test.run=^TestConfigProcessHelper$")
		cmds[i].Env = append(os.Environ(), "PI_SWITCH_LOCK_TEST_PATH="+path, "PI_SWITCH_LOCK_TEST_GATE="+gate, fmt.Sprintf("PI_SWITCH_LOCK_TEST_ID=%d", i))
		cmds[i].Stdout = &outputs[i]
		cmds[i].Stderr = &outputs[i]
		if err := cmds[i].Start(); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(gate, nil, 0600); err != nil {
		t.Fatal(err)
	}
	for i, cmd := range cmds {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("child %d: %v\n%s", i, err, outputs[i].String())
		}
	}
	cfg, _, err := config.LoadConfigAtPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Profiles) != processes*10 {
		t.Fatalf("profiles = %d, want %d", len(cfg.Profiles), processes*10)
	}
}

func TestConfigLockReleasedOnProcessExit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := config.SaveAtPath(config.DefaultConfig(), path); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ready := path + ".ready"
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestConfigProcessHelper$")
	cmd.Env = append(os.Environ(), "PI_SWITCH_LOCK_TEST_PATH="+path, "PI_SWITCH_LOCK_TEST_GATE="+ready, "PI_SWITCH_LOCK_TEST_ID=hold")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if ctx.Err() != nil {
			t.Fatal("child did not acquire lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	if err := config.UpdateAtPath(path, func(cfg *config.PiSwitchConfig) error { cfg.Current = nil; return nil }); err != nil {
		t.Fatalf("exited process left config locked: %v", err)
	}
}

func TestConfigProcessHelper(t *testing.T) {
	path := os.Getenv("PI_SWITCH_LOCK_TEST_PATH")
	if path == "" {
		return
	}
	gate, id := os.Getenv("PI_SWITCH_LOCK_TEST_GATE"), os.Getenv("PI_SWITCH_LOCK_TEST_ID")
	if id == "hold" {
		if err := config.UpdateAtPath(path, func(*config.PiSwitchConfig) error {
			if err := os.WriteFile(gate, nil, 0600); err != nil {
				return err
			}
			select {}
		}); err != nil {
			t.Fatal(err)
		}
		return
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(gate); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("gate never opened")
		}
		time.Sleep(time.Millisecond)
	}
	for i := 0; i < 10; i++ {
		if err := config.UpdateAtPath(path, func(cfg *config.PiSwitchConfig) error {
			cfg.Profiles[fmt.Sprintf("%s-%d", id, i)] = config.ProviderProfile{API: "openai-completions"}
			time.Sleep(time.Millisecond)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
}
