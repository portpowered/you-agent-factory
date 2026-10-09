package service

import (
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/modelinvocation"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	legacyservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionservice"
	factorysessioncontracts "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire/contracts"
	"github.com/portpowered/infinite-you/pkg/services/models"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
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
	opening *RuntimeOpening
	*legacyservice.Assembly
	liveChangeCoordinator          factorysessioncontracts.LiveChangeCoordinator
	modelInvocation                modelinvocation.RuntimeModelInvocationOperation
	workerService                  workers.Service
	modelService                   models.Service
	recordingsService              recordings.Service
	recordingProjections           recordings.ProjectionService
	factoryDefinitions             factorydefinitions.Service
	workService                    work.Service
	providerSessions               providersessions.Service
	workflowPreview                factoryruntime.WorkflowPreviewOperation
	generateSessionID              factorysessions.SessionIDGenerator
	factorySessionsRuntimeAssembly roles.RuntimeAssembly
}

func NewRoot(
	opening *RuntimeOpening,
	providerSessions providersessions.Service,
	workflowPreview factoryruntime.WorkflowPreviewOperation,
	definitions factorydefinitions.Service,
	assembly roles.RuntimeAssembly,
	generateSessionID factorysessions.SessionIDGenerator,
	workService work.Service,
	modelService models.Service,
	recordingsService recordings.Service,
	workerService workers.Service,
	modelInvocation modelinvocation.RuntimeModelInvocationOperation,
	liveChangeCoordinator factorysessioncontracts.LiveChangeCoordinator,
	recordingProjections recordings.ProjectionService,
) (*Root, error) {
	concrete, err := requireRootAssembly(assembly, liveChangeCoordinator)
	if err != nil {
		return nil, err
	}
	root := &Root{
		opening:                        opening,
		Assembly:                       concrete,
		liveChangeCoordinator:          liveChangeCoordinator,
		modelInvocation:                modelInvocation,
		workerService:                  workerService,
		modelService:                   modelService,
		factorySessionsRuntimeAssembly: assembly,
		recordingsService:              recordingsService,
		recordingProjections:           recordingProjections,
		factoryDefinitions:             definitions,
		workService:                    workService,
		providerSessions:               providerSessions,
		workflowPreview:                workflowPreview,
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
