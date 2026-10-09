package main

import (
	"context"
	"sync"
)

type desktopTask struct {
	ctx    context.Context
	cancel context.CancelFunc
	token  uint64
}

type desktopTasks struct {
	mu     sync.Mutex
	next   uint64
	closed bool
	active map[string]desktopTask
}

func (t *desktopTasks) begin(name string) (context.Context, uint64, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil, 0, false
	}
	if t.active == nil {
		t.active = make(map[string]desktopTask)
	}
	if _, ok := t.active[name]; ok {
		return nil, 0, false
	}
	t.next++
	ctx, cancel := context.WithCancel(context.Background())
	t.active[name] = desktopTask{ctx: ctx, cancel: cancel, token: t.next}
	return ctx, t.next, true
}

func (t *desktopTasks) accept(name string, token uint64) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	task, ok := t.active[name]
	return ok && !t.closed && task.token == token && task.ctx.Err() == nil
}

func (t *desktopTasks) finish(name string, token uint64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if task, ok := t.active[name]; ok && task.token == token {
		delete(t.active, name)
		task.cancel()
	}
}

func (t *desktopTasks) close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	for name, task := range t.active {
		task.cancel()
		delete(t.active, name)
	}
}
