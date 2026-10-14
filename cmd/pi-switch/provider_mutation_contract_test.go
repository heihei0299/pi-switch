package main

import (
	"os"
	"strings"
	"testing"

	"github.com/heihei0299/pi-switch/internal/config"
)

const providerMutationCLIConfig = `{"version":2,"current":"p1","profiles":{"p1":{"api":"openai-completions","responsesMode":"auto","preset":"openai","baseUrl":"https://one.example/v1","apiKey":"fake-one","models":[{"id":"one","contextWindow":100,"maxTokens":10}]},"p2":{"api":"openai-completions","responsesMode":"auto","preset":"openai","baseUrl":"https://two.example/v1","apiKey":"fake-two","models":[{"id":"two","contextWindow":100,"maxTokens":10}]}},"settings":{}}`

func TestProviderCLIUseAndDeleteMutationContract(t *testing.T) {
	t.Run("use switches only current", func(t *testing.T) {
		isolateCLI(t)
		path := os.Getenv("PI_SWITCH_CONFIG")
		if err := os.WriteFile(path, []byte(providerMutationCLIConfig), 0600); err != nil {
			t.Fatal(err)
		}
		if code, out, errOut := runCLIStreams(t, func() int { return handleProvider([]string{"use", "p2"}) }); code != 0 {
			t.Fatalf("provider use: exit=%d stdout=%q stderr=%q", code, out, errOut)
		}
		cfg, _, err := config.LoadConfigAtPath(path)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Current == nil || *cfg.Current != "p2" || len(cfg.Profiles) != 2 {
			t.Fatalf("provider use changed more than current: %+v", cfg)
		}
		assertNoProviderBackups(t, path)
	})

	t.Run("delete current clears it and keeps others", func(t *testing.T) {
		isolateCLI(t)
		path := os.Getenv("PI_SWITCH_CONFIG")
		if err := os.WriteFile(path, []byte(providerMutationCLIConfig), 0600); err != nil {
			t.Fatal(err)
		}
		if code, out, errOut := runCLIStreams(t, func() int { return handleProvider([]string{"delete", "p1"}) }); code != 0 {
			t.Fatalf("provider delete: exit=%d stdout=%q stderr=%q", code, out, errOut)
		}
		cfg, _, err := config.LoadConfigAtPath(path)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Current != nil || len(cfg.Profiles) != 1 {
			t.Fatalf("provider delete did not clear current/preserve remaining profile: %+v", cfg)
		}
		if _, ok := cfg.Profiles["p2"]; !ok {
			t.Fatal("provider delete removed an unrelated profile")
		}
		assertNoProviderBackups(t, path)
	})

	t.Run("missing use does not write", func(t *testing.T) {
		isolateCLI(t)
		path := os.Getenv("PI_SWITCH_CONFIG")
		if err := os.WriteFile(path, []byte(providerMutationCLIConfig), 0600); err != nil {
			t.Fatal(err)
		}
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if code, _, _ := runCLIStreams(t, func() int { return handleProvider([]string{"use", "missing"}) }); code == 0 {
			t.Fatal("provider use accepted a missing profile")
		}
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(after) != string(before) {
			t.Fatal("missing provider use changed config bytes")
		}
		assertNoProviderBackups(t, path)
	})

	t.Run("missing delete remains idempotent", func(t *testing.T) {
		isolateCLI(t)
		path := os.Getenv("PI_SWITCH_CONFIG")
		if err := os.WriteFile(path, []byte(providerMutationCLIConfig), 0600); err != nil {
			t.Fatal(err)
		}
		code, out, errOut := runCLIStreams(t, func() int { return handleProvider([]string{"delete", "missing"}) })
		if code != 0 || !strings.Contains(out, "Deleted missing") {
			t.Fatalf("missing delete = exit %d, stdout=%q, stderr=%q", code, out, errOut)
		}
		cfg, _, err := config.LoadConfigAtPath(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(cfg.Profiles) != 2 || cfg.Current == nil || *cfg.Current != "p1" {
			t.Fatalf("missing delete changed profiles/current: %+v", cfg)
		}
		assertNoProviderBackups(t, path)
	})
}

func assertNoProviderBackups(t *testing.T, configPath string) {
	t.Helper()
	if _, err := os.Stat(configPath + ".backups"); !os.IsNotExist(err) {
		t.Fatalf("CLI provider operation created backups, stat error = %v", err)
	}
}
