package profile_test

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/heihei0299/pi-switch/internal/config"
	"github.com/heihei0299/pi-switch/internal/profile"
)

func TestConcurrentCreatePreservesProfiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("PI_SWITCH_CONFIG", path)
	cfg := config.DefaultConfig()
	cfg.Profiles = map[string]config.ProviderProfile{}
	if err := config.SaveAtPath(cfg, path); err != nil {
		t.Fatal(err)
	}
	name := "main"
	p := config.ProviderProfile{API: "openai-completions", Upstreams: []config.Upstream{{Name: &name, API: "openai-completions", BaseURL: "http://127.0.0.1:1"}}}
	const count = 32
	start := make(chan struct{})
	errors := make(chan error, count)
	var workers sync.WaitGroup
	for i := 0; i < count; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			<-start
			errors <- profile.CreateProfile(fmt.Sprintf("supplier-%02d", i), p)
		}(i)
	}
	close(start)
	workers.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	got, _, err := config.LoadConfigAtPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Profiles) != count {
		t.Fatalf("successful creates=%d, persisted profiles=%d", count, len(got.Profiles))
	}
}
