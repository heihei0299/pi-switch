package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/egoist/mygo"
)

func TestDesktopTasksRejectLateAndClosedResults(t *testing.T) {
	tasks := desktopTasks{}
	ctx, token, ok := tasks.begin("stats")
	if !ok || ctx.Err() != nil {
		t.Fatal("failed to start task")
	}
	if !tasks.accept("stats", token) {
		t.Fatal("active task was rejected")
	}
	tasks.finish("stats", token)
	if tasks.accept("stats", token) {
		t.Fatal("finished task accepted a late result")
	}

	_, token, ok = tasks.begin("stats")
	if !ok {
		t.Fatal("failed to restart task")
	}
	tasks.close()
	if tasks.accept("stats", token) {
		t.Fatal("closed task accepted a result")
	}
	if _, _, ok := tasks.begin("stats"); ok {
		t.Fatal("closed task started another operation")
	}
}

func TestStandardTaskPassesCancellationToWorker(t *testing.T) {
	shell := &desktopShell{window: &mygo.Window{}}
	defer shell.tasks.close()
	started := make(chan struct{})
	finished := make(chan error, 1)
	if !shell.runStandardTask("providers", func(ctx context.Context) (interface{}, error) {
		close(started)
		<-ctx.Done()
		finished <- ctx.Err()
		return nil, ctx.Err()
	}, func(interface{}, error) {}) {
		t.Fatal("task was not started")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	shell.tasks.close()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("worker error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("worker did not receive task cancellation")
	}
}
