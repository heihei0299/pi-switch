package filegateway

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	domaingateway "github.com/heihei0299/pi-switch/internal/domain/gateway"
	"time"
)

func TestWriteLockSerializesPublishers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "models.json")
	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondEntered := make(chan struct{})
	errCh := make(chan error, 2)
	go func() {
		errCh <- withWriteLock(context.Background(), path, func() error {
			close(firstEntered)
			<-releaseFirst
			return nil
		})
	}()
	select {
	case <-firstEntered:
	case <-time.After(time.Second):
		t.Fatal("first publisher did not acquire the lock")
	}
	go func() {
		errCh <- withWriteLock(context.Background(), path, func() error {
			close(secondEntered)
			return nil
		})
	}()
	enteredEarly := false
	select {
	case <-secondEntered:
		enteredEarly = true
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseFirst)
	if !enteredEarly {
		select {
		case <-secondEntered:
		case <-time.After(time.Second):
			t.Fatal("second publisher did not enter after the first released the lock")
		}
	}
	for range 2 {
		if err := <-errCh; err != nil {
			t.Fatalf("publisher lock: %v", err)
		}
	}
	if enteredEarly {
		t.Fatal("second publisher entered while the first held the lock")
	}
}

func TestPublishCancelsWhileWaitingForLockWithoutTouchingModels(t *testing.T) {
	path := filepath.Join(t.TempDir(), "models.json")
	original := []byte(`{"providers":{}}`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Unix(1_600_000_000, 0)
	if err := os.Chtimes(path, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	held, release := make(chan struct{}), make(chan struct{})
	lockErr := make(chan error, 1)
	go func() {
		lockErr <- withWriteLock(context.Background(), path, func() error {
			close(held)
			<-release
			return nil
		})
	}()
	select {
	case <-held:
	case <-time.After(time.Second):
		t.Fatal("test lock was not acquired")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()
	plan := domaingateway.CanonicalGatewayPlan{
		Current:      map[string]interface{}{"providers": map[string]interface{}{}},
		Proposed:     map[string]interface{}{"providers": map[string]interface{}{"pi-switch-chat": map[string]interface{}{"models": []interface{}{}}}},
		PendingCount: 1,
	}
	if err := New(path).Publish(ctx, plan); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("publish error = %v, want context deadline exceeded", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) || !info.ModTime().Equal(oldTime) {
		t.Fatalf("canceled publish touched models.json: bytes=%q mtime=%s", after, info.ModTime())
	}
	close(release)
	if err := <-lockErr; err != nil {
		t.Fatal(err)
	}
}
