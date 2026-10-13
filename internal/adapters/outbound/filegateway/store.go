package filegateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	domaingateway "github.com/heihei0299/pi-switch/internal/domain/gateway"
	"golang.org/x/sys/unix"
)

var ErrStale = errors.New("models.json changed since the Gateway preview; reload and review it again")

const writeLockWait = 5 * time.Second

type Store struct{ path string }

func New(path string) *Store { return &Store{path: path} }

func ModelsPath() string {
	if path := os.Getenv("PI_SWITCH_MODELS"); path != "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/pi-switch-models.json"
	}
	return filepath.Join(home, ".pi", "agent", "models.json")
}

func emptyGatewayWrapper() map[string]interface{} {
	return map[string]interface{}{"providers": map[string]interface{}{}}
}

func (s *Store) Read(ctx context.Context) (map[string]interface{}, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return readCurrent(s.path)
}

func readCurrent(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return emptyGatewayWrapper(), nil
	}
	if err != nil {
		return nil, err
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if _, ok := raw["providers"].(map[string]interface{}); !ok {
		return emptyGatewayWrapper(), nil
	}
	return map[string]interface{}{"providers": raw["providers"]}, nil
}

func (s *Store) Publish(ctx context.Context, plan domaingateway.CanonicalGatewayPlan) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(plan.Conflicts) > 0 {
		return fmt.Errorf("gateway plan rejected: %s", strings.Join(plan.Conflicts, "; "))
	}
	providers, ok := plan.Proposed["providers"].(map[string]interface{})
	if !ok || providers == nil {
		return fmt.Errorf("gateway.providers is required")
	}
	if plan.PendingCount == 0 {
		return nil
	}
	data, err := json.MarshalIndent(map[string]interface{}{"providers": providers}, "", "  ")
	if err != nil {
		return err
	}
	expected := plan.Current
	if expected == nil {
		expected = emptyGatewayWrapper()
	}
	expectedJSON, err := json.Marshal(expected)
	if err != nil {
		return err
	}
	return withWriteLock(ctx, s.path, func() error {
		current, err := readCurrent(s.path)
		if err != nil {
			return err
		}
		currentJSON, err := json.Marshal(current)
		if err != nil {
			return err
		}
		if !bytes.Equal(expectedJSON, currentJSON) {
			return ErrStale
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return writeFileAtomic(s.path, append(data, '\n'))
	})
}

func writeFileAtomic(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return err
	}
	tmp := file.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmp)
		}
	}()
	if err := file.Chmod(0600); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func withWriteLock(ctx context.Context, path string, publish func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	file, err := os.OpenFile(path+".lock", os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	deadline := time.Now().Add(writeLockWait)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			defer unix.Flock(int(file.Fd()), unix.LOCK_UN)
			if err := ctx.Err(); err != nil {
				return err
			}
			return publish()
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EINTR) {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("gateway write lock busy: %s", path)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
