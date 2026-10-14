package proxycontrol

import "errors"

var ErrPortInUse = errors.New("port already in use")

type Target struct {
	Name       string
	Executable string
}

var (
	Proxy = Target{Name: "proxy"}
	WebUI = Target{Name: "webui"}
)

type Result struct {
	Running   bool     `json:"running"`
	Pid       *uint32  `json:"pid,omitempty"`
	Host      *string  `json:"host,omitempty"`
	Port      *uint16  `json:"port,omitempty"`
	Targets   []string `json:"targets,omitempty"`
	Failover  []string `json:"failover,omitempty"`
	StartedAt *uint64  `json:"startedAt,omitempty"`
	Message   string   `json:"message"`
}

type Daemon interface {
	Start(Target, string, uint16) (Result, error)
	Stop(Target) (Result, error)
	Status(Target) (Result, error)
}

type Service struct{ daemon Daemon }

func New(daemon Daemon) Service { return Service{daemon: daemon} }

func (s Service) Start(target Target, host string, port uint16) (Result, error) {
	return s.daemon.Start(target, host, port)
}

func (s Service) Stop(target Target) (Result, error) { return s.daemon.Stop(target) }

func (s Service) Status(target Target) (Result, error) { return s.daemon.Status(target) }

func (s Service) Restart(target Target, host string, port uint16) (Result, error) {
	if _, err := s.daemon.Stop(target); err != nil {
		return Result{}, err
	}
	return s.daemon.Start(target, host, port)
}
