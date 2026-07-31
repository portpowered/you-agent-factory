// Package service implements the request-scoped Workers Execute path.
package service

import (
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
)

// Dependencies are the exact process-scoped edges required by Execute.
type Dependencies struct {
	Runners        runners.Service
	Providers      providers.Service
	Observe        workers.ObservationSink
	Logger         logging.Logger
	Clock          func() time.Time
	Worktree       workers.FactoryWorktreePreparer
	TemporaryFiles TemporaryFileCleaner
}

// TemporaryFileCleaner releases request-scoped temporary files created during
// one Execute call. Wire may leave it nil when no temporary-file effect is
// constructed.
type TemporaryFileCleaner interface {
	Cleanup(paths ...string) error
}

// Service executes one isolated Workers attempt through the private runner
// registry. It retains no Factory Session, Runtime, dispatch, or attempt state
// after Execute returns.
type Service struct {
	runners        runners.Service
	providers      providers.Service
	observe        workers.ObservationSink
	logger         logging.Logger
	clock          func() time.Time
	worktree       workers.FactoryWorktreePreparer
	temporaryFiles TemporaryFileCleaner
}

// New constructs an inert Execute capability. Construction performs no runner
// execution, Worktree preparation, or observation delivery.
func New(dependencies Dependencies) (*Service, error) {
	if dependencies.Runners == nil {
		return nil, errMisconfigured("runners service is required")
	}
	clock := dependencies.Clock
	if clock == nil {
		clock = time.Now
	}
	return &Service{
		runners:        dependencies.Runners,
		providers:      dependencies.Providers,
		observe:        dependencies.Observe,
		logger:         logging.EnsureLogger(dependencies.Logger),
		clock:          clock,
		worktree:       dependencies.Worktree,
		temporaryFiles: dependencies.TemporaryFiles,
	}, nil
}
