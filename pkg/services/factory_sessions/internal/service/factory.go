package service

import (
	"context"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/modelinvocation"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	legacyservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionservice"
	factorysessioncontracts "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire/contracts"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// WorkerCommandRunnerAdapter is a composition-only identity adapter for the
// shared platform process effect. Workers performs its richer adaptation
// privately at its own service boundary.
type WorkerCommandRunnerAdapter func(platformprocess.CommandRunner) platformprocess.CommandRunner

// ProviderOverrideService is the optional request-scoped Providers root
// replacement selected by process composition. The distinct interface keeps
// Wire from confusing the override with the process-owned Providers root when
// both are available in the same provider set.
type ProviderOverrideService interface {
	providers.Service
}

// ProviderCommandRunner and ScriptCommandRunner are distinct Wire keys for
// the two platform process effects. They remain separate so composition
// cannot accidentally bind one selected runner to both effect owners.
type ProviderCommandRunner interface {
	platformprocess.CommandRunner
}

type ScriptCommandRunner interface {
	platformprocess.CommandRunner
}

// Root owns the process-scoped Factory Sessions state and fixed collaborators.
// Its Assembly and live-change coordinator are injected during construction.
type Root struct {
	start             func(context.Context, factorysessions.SessionStartRequest) (factorysessions.SessionStartResult, error)
	inspectHistorical func(context.Context, factorysessions.SessionStartRequest) (HistoricalApplicationInspection, bool, error)
	durable           durableexecution.Service
	*legacyservice.Assembly
	liveChangeCoordinator          factorysessioncontracts.LiveChangeCoordinator
	modelInvocation                modelinvocation.RuntimeModelInvocationOperation
	recordingsService              recordings.Service
	recordingProjections           recordings.ProjectionService
	factoryDefinitions             factorydefinitions.Service
	generateSessionID              factorysessions.SessionIDGenerator
	factorySessionsRuntimeAssembly roles.RuntimeAssembly
}

func NewRoot(
	assembly roles.RuntimeAssembly,
	durable durableexecution.Service,
	start func(context.Context, factorysessions.SessionStartRequest) (factorysessions.SessionStartResult, error),
	liveChangeCoordinator factorysessioncontracts.LiveChangeCoordinator,
	inspectHistorical func(context.Context, factorysessions.SessionStartRequest) (HistoricalApplicationInspection, bool, error),
	definitions factorydefinitions.Service,
	recordingsService recordings.Service,
	recordingProjections recordings.ProjectionService,
	modelInvocation modelinvocation.RuntimeModelInvocationOperation,
	generateSessionID factorysessions.SessionIDGenerator,
) (*Root, error) {
	concrete, err := requireRootAssembly(assembly, liveChangeCoordinator)
	if err != nil {
		return nil, err
	}
	root := &Root{
		start: start, inspectHistorical: inspectHistorical, durable: durable,
		Assembly:                       concrete,
		liveChangeCoordinator:          liveChangeCoordinator,
		modelInvocation:                modelInvocation,
		factorySessionsRuntimeAssembly: assembly,
		recordingsService:              recordingsService,
		recordingProjections:           recordingProjections,
		factoryDefinitions:             definitions,
		generateSessionID:              generateSessionID,
	}
	return root, nil
}

func replayRequestsHistoricalInspection(host factorysessions.RuntimeHostRequest) bool {
	// The CLI clears Port when the API transport is disabled but retains the
	// parsed AutoPort default. AutoPort is meaningful only with a concrete
	// listener request, so the effective hosting decision is the resolved port.
	return host.Port <= 0
}

func selectsHistoricalReplayInspection(input recordings.LoadReplayInputResult) bool {
	if input.Portable != nil || input.Legacy == nil {
		return true
	}
	if input.LegacyFormat != "" {
		return input.LegacyFormat == string(recordings.RecordedSessionFormatV2JSONL)
	}
	// Preserve the established compatibility contract for narrow callers that
	// return a legacy artifact without the newer framing metadata. Production
	// path loaders always identify V1 versus V2 above.
	return legacyReplayArtifactHasCanonicalEventShape(*input.Legacy)
}
