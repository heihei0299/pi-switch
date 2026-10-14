package proxycontrol

import (
	"errors"
	"reflect"
	"testing"
)

type fakeDaemon struct {
	events  []string
	stopErr error
}

func (d *fakeDaemon) Start(target Target, _ string, _ uint16) (Result, error) {
	d.events = append(d.events, "start:"+target.Name)
	return Result{Running: true}, nil
}

func (d *fakeDaemon) Stop(target Target) (Result, error) {
	d.events = append(d.events, "stop:"+target.Name)
	return Result{}, d.stopErr
}

func (d *fakeDaemon) Status(target Target) (Result, error) {
	d.events = append(d.events, "status:"+target.Name)
	return Result{}, nil
}

func TestRestartStopsBeforeStarting(t *testing.T) {
	daemon := &fakeDaemon{}
	service := New(daemon)
	result, err := service.Restart(Proxy, "127.0.0.1", 43112)
	if err != nil || !result.Running || !reflect.DeepEqual(daemon.events, []string{"stop:proxy", "start:proxy"}) {
		t.Fatalf("restart = %+v, %v, events=%v", result, err, daemon.events)
	}
}

func TestRestartDoesNotStartAfterRefusedStop(t *testing.T) {
	stopErr := errors.New("unmanaged process owns port")
	daemon := &fakeDaemon{stopErr: stopErr}
	service := New(daemon)
	if _, err := service.Restart(Proxy, "127.0.0.1", 43112); !errors.Is(err, stopErr) {
		t.Fatalf("restart error = %v, want stop error", err)
	}
	if !reflect.DeepEqual(daemon.events, []string{"stop:proxy"}) {
		t.Fatalf("refused stop still started process: %v", daemon.events)
	}
}
