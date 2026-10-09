package main

import "testing"

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
