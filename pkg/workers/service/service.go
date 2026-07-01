package service

import (
	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/pkg/workers"
	"go.uber.org/zap"
)

// Service supervises script pollers and cron workstations using injected collaborators.
type Service struct {
	cfg Config
}

// New constructs a workers scheduling service with explicit dependencies.
func New(cfg Config) *Service {
	return &Service{cfg: cfg}
}

func (s *Service) logger() *zap.Logger {
	if s == nil || s.cfg.Logger == nil {
		return zap.NewNop()
	}
	return s.cfg.Logger
}

func (s *Service) commandRunner() workers.CommandRunner {
	if s != nil && s.cfg.CommandRunner != nil {
		return s.cfg.CommandRunner
	}
	return workers.ExecCommandRunner{}
}

func (s *Service) supervisorClock() clockwork.Clock {
	if s != nil && s.cfg.Clock != nil {
		return s.cfg.Clock
	}
	return clockwork.NewRealClock()
}

func (s *Service) pollerLogger(workstationName, workerName string) *zap.Logger {
	return s.logger().With(
		zap.String("workstation", workstationName),
		zap.String("worker", workerName),
	)
}
