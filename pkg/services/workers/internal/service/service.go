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

// Service executes one isolated Workers attempt through the private runner
// registry. It retains no Factory Session, Runtime, dispatch, or attempt state
// after Execute returns.
type Service struct {
	runners         runners.Service
	providers       providers.Service
	observe         workers.ObservationSink
	logger          logging.Logger
	clock           func() time.Time
	worktree        workers.FactoryWorktreePreparer
	worktreeRelease func(context.Context, workers.FactoryWorktreePreparation) error
	temporaryFiles  workers.TemporaryFileSystem
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
	worktreeRelease func(context.Context, workers.FactoryWorktreePreparation) error,
	temporaryFiles workers.TemporaryFileSystem,
) (*Service, error) {
	if runnerService == nil {
		return nil, errMisconfigured("runners service is required")
	}
	if clock == nil {
		return nil, errMisconfigured("clock is required")
	}
	return &Service{
		runners:         runnerService,
		providers:       providersService,
		observe:         observe,
		logger:          logging.EnsureLogger(logger),
		clock:           clock,
		worktree:        worktree,
		worktreeRelease: worktreeRelease,
		temporaryFiles:  temporaryFiles,
	}, nil
}
