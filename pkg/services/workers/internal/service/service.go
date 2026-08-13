// Package service implements the request-scoped Workers Execute path.
package service

import (
	"context"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
)

// factoryWorktreeReleaser is an optional owner-provided cleanup edge. The
// public Workers root exposes only the preparer contract; worktree lifecycle
// details stay behind the request-scoped Execute implementation.
type factoryWorktreeReleaser interface {
	Release(context.Context, workers.FactoryWorktreePreparation) error
}

// temporaryFileCleaner releases temporary files owned by one Execute call.
// Wire supplies this exact edge without widening the public Workers root.
type temporaryFileCleaner interface {
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
	temporaryFiles temporaryFileCleaner
}

// New constructs an inert Execute capability. Construction performs no runner
// execution, Worktree preparation, or observation delivery.
func New(
	runnerService runners.Service,
	providersService providers.Service,
	observe workers.ObservationSink,
	logger logging.Logger,
	clock func() time.Time,
	worktree workers.FactoryWorktreePreparer,
	temporaryFiles temporaryFileCleaner,
) (*Service, error) {
	if runnerService == nil {
		return nil, errMisconfigured("runners service is required")
	}
	if clock == nil {
		clock = time.Now
	}
	return &Service{
		runners:        runnerService,
		providers:      providersService,
		observe:        observe,
		logger:         logging.EnsureLogger(logger),
		clock:          clock,
		worktree:       worktree,
		temporaryFiles: temporaryFiles,
	}, nil
}
