// Package service implements the request-scoped Workers Execute path.
package service

import (
	"context"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor/agentrun"
)

// Service executes one isolated Workers attempt through the private runner
// registry. It retains no Factory Session, Runtime, dispatch, or attempt state
// after Execute returns.
type Service struct {
	runners          runners.Service
	providers        providers.Service
	providerOverride providers.Service
	observe          workers.ObservationSink
	logger           logging.Logger
	clock            func() time.Time
	worktree         workers.FactoryWorktreePreparer
	worktreeRelease  func(context.Context, workers.FactoryWorktreePreparation) error
	temporaryFiles   workers.TemporaryFileSystem
	factoryDocs      workers.FactoryDocsLoader
	// agentRunHarness runs the agent/tool loop for an attempt whose target
	// declares Tools.AgentLoop. It is immutable process configuration; the loop
	// itself keeps no state between Execute calls.
	agentRunHarness agentrun.HarnessAdapter
	// decisionEnvelopes is the Factory Definitions owner used when a detached
	// provider override replaces the registered Agent runner.
	decisionEnvelopes factorydefinitions.DecisionEnvelopeService
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
	factoryDocs ...workers.FactoryDocsLoader,
) (*Service, error) {
	return newService(
		runnerService,
		providersService,
		observe,
		logger,
		clock,
		worktree,
		worktreeRelease,
		temporaryFiles,
		nil,
		nil,
		nil,
		factoryDocs...,
	)
}

// NewWithProviderOverride constructs an inert Execute capability with the
// optional process edge provider used by root composition. The override is
// immutable process configuration; request identity and cancellation remain
// owned by each Execute call.
func NewWithProviderOverride(
	runnerService runners.Service,
	providersService providers.Service,
	observe workers.ObservationSink,
	logger logging.Logger,
	clock func() time.Time,
	worktree workers.FactoryWorktreePreparer,
	worktreeRelease func(context.Context, workers.FactoryWorktreePreparation) error,
	temporaryFiles workers.TemporaryFileSystem,
	providerOverride providers.Service,
	agentRunHarness agentrun.HarnessAdapter,
	decisionEnvelopes factorydefinitions.DecisionEnvelopeService,
	factoryDocs ...workers.FactoryDocsLoader,
) (*Service, error) {
	return newService(
		runnerService,
		providersService,
		observe,
		logger,
		clock,
		worktree,
		worktreeRelease,
		temporaryFiles,
		providerOverride,
		agentRunHarness,
		decisionEnvelopes,
		factoryDocs...,
	)
}

func newService(
	runnerService runners.Service,
	providersService providers.Service,
	observe workers.ObservationSink,
	logger logging.Logger,
	clock func() time.Time,
	worktree workers.FactoryWorktreePreparer,
	worktreeRelease func(context.Context, workers.FactoryWorktreePreparation) error,
	temporaryFiles workers.TemporaryFileSystem,
	providerOverride providers.Service,
	agentRunHarness agentrun.HarnessAdapter,
	decisionEnvelopes factorydefinitions.DecisionEnvelopeService,
	factoryDocs ...workers.FactoryDocsLoader,
) (*Service, error) {
	if runnerService == nil {
		return nil, errMisconfigured("runners service is required")
	}
	if clock == nil {
		return nil, errMisconfigured("clock is required")
	}
	var docsLoader workers.FactoryDocsLoader
	if len(factoryDocs) > 0 {
		docsLoader = factoryDocs[0]
	}
	return &Service{
		runners:           runnerService,
		providers:         providersService,
		providerOverride:  providerOverride,
		observe:           observe,
		logger:            logging.EnsureLogger(logger),
		clock:             clock,
		worktree:          worktree,
		worktreeRelease:   worktreeRelease,
		temporaryFiles:    temporaryFiles,
		factoryDocs:       docsLoader,
		agentRunHarness:   agentRunHarness,
		decisionEnvelopes: decisionEnvelopes,
	}, nil
}
