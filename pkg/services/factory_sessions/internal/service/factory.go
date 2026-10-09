package service

import (
	"context"
	"fmt"
	"strings"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/recordingreplay"
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
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
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
	replayBehavior *recordingreplay.Behavior
	*legacyservice.Assembly
	startFlights                   singleflight.Group
	liveChangeCoordinator          factorysessioncontracts.LiveChangeCoordinator
	openingCompletion              *RuntimeOpeningCompletion
	openingBinding                 *RuntimeOpeningBinding
	resourceAcquisition            *RuntimeResourceAcquisition
	durableOpening                 *DurableOpening
	modelInvocation                modelinvocation.RuntimeModelInvocationOperation
	workerService                  workers.Service
	modelService                   models.Service
	recordingsService              recordings.Service
	recordingsRuntime              recordings.RuntimeScopeService
	recordingProjections           recordings.ProjectionService
	replayInputs                   recordings.ReplayInputLoader
	factoryDefinitions             factorydefinitions.Service
	initialEngine                  *RuntimeInitialEngine
	workService                    work.Service
	providerSessions               providersessions.Service
	factoryWorkflows               factoryruntime.JavaScriptWorkflowDefinitions
	workflowPreview                factoryruntime.WorkflowPreviewOperation
	snapshotSelection              *RuntimeSnapshotSelection
	preparation                    *RuntimePreparation
	resolveClock                   factoryruntime.ClockResolver
	baseLogger                     *zap.Logger
	runtimeLogs                    factoryruntime.RuntimeLogOwner
	generateSessionID              factorysessions.SessionIDGenerator
	generateRuntimeInstanceID      factorysessions.RuntimeInstanceIDGenerator
	resolveHome                    factorysessions.HomeDirectoryResolver
	factorySessionsRuntimeAssembly roles.RuntimeAssembly
	runtimeRoot                    FactoryRuntimeRoot
	clock                          factoryruntime.Clock
	providerOverride               providers.Service
	executionBinding               *ExecutionBinding
	submissionRecorder             recordings.SubmissionRecorder
	dispatchRecorder               recordings.DispatchRecorder
}

func NewRoot(
	providerSessions providersessions.Service,
	logger *zap.Logger,
	runtimeLogs factoryruntime.RuntimeLogOwner,
	factoryWorkflows factoryruntime.JavaScriptWorkflowDefinitions,
	workflowPreview factoryruntime.WorkflowPreviewOperation,
	runtimeRoot FactoryRuntimeRoot,
	resolveClock factoryruntime.ClockResolver,
	clock factoryruntime.Clock,
	providerOverride ProviderOverrideService,
	submissionRecorder recordings.SubmissionRecorder,
	dispatchRecorder recordings.DispatchRecorder,
	definitions factorydefinitions.Service,
	snapshotSelection *RuntimeSnapshotSelection,
	preparation *RuntimePreparation,
	assembly roles.RuntimeAssembly,
	durableOpening *DurableOpening,
	resourceAcquisition *RuntimeResourceAcquisition,
	openingCompletion *RuntimeOpeningCompletion,
	openingBinding *RuntimeOpeningBinding,
	generateSessionID factorysessions.SessionIDGenerator,
	generateRuntimeInstanceID factorysessions.RuntimeInstanceIDGenerator,
	resolveHome factorysessions.HomeDirectoryResolver,
	workService work.Service,
	automationService automations.Service,
	modelService models.Service,
	recordingsService recordings.Service,
	recordingsRuntime recordings.RuntimeScopeService,
	workerService workers.Service,
	executionBinding *ExecutionBinding,
	initialEngine *RuntimeInitialEngine,
	modelInvocation modelinvocation.RuntimeModelInvocationOperation,
	replayBehavior *recordingreplay.Behavior,
	liveChangeCoordinator factorysessioncontracts.LiveChangeCoordinator,
	recordingProjections recordings.ProjectionService,
) (*Root, error) {
	concrete, err := requireRootAssembly(assembly, liveChangeCoordinator)
	if err != nil {
		return nil, err
	}
	root := &Root{
		replayBehavior:                 replayBehavior,
		Assembly:                       concrete,
		liveChangeCoordinator:          liveChangeCoordinator,
		initialEngine:                  initialEngine,
		modelInvocation:                modelInvocation,
		durableOpening:                 durableOpening,
		resourceAcquisition:            resourceAcquisition,
		workerService:                  workerService,
		modelService:                   modelService,
		factorySessionsRuntimeAssembly: assembly,
		recordingsService:              recordingsService,
		recordingsRuntime:              recordingsRuntime,
		recordingProjections:           recordingProjections,
		replayInputs:                   recordingsRuntime,
		factoryDefinitions:             definitions,
		workService:                    workService,
		providerSessions:               providerSessions,
		factoryWorkflows:               factoryWorkflows,
		workflowPreview:                workflowPreview,
		snapshotSelection:              snapshotSelection,
		preparation:                    preparation,
		resolveClock:                   resolveClock,
		baseLogger:                     logger,
		runtimeLogs:                    runtimeLogs,
		openingCompletion:              openingCompletion,
		openingBinding:                 openingBinding,
		generateSessionID:              generateSessionID,
		generateRuntimeInstanceID:      generateRuntimeInstanceID,
		resolveHome:                    resolveHome,
		clock:                          clock,
		providerOverride:               providerOverride,
		executionBinding:               executionBinding,
		submissionRecorder:             submissionRecorder,
		dispatchRecorder:               dispatchRecorder,
	}
	root.runtimeRoot = runtimeRoot
	return root, nil
}

func (r *Root) openForRequest(
	ctx context.Context,
	request factorysessions.SessionStartRequest,
) (runtimeProducts, error) {
	if strings.TrimSpace(request.FolderPath) == "" {
		return runtimeProducts{}, &factorysessions.DetachedRequestError{Field: "folderPath", Message: "folder path is required"}
	}
	selection := runtimeSelectionForStart(request)
	recording := recordingRequestForStart(request)
	open := func(replayInput *recordings.LoadReplayInputResult) (runtimeProducts, error) {
		session := request
		return r.openRuntimeWithOptions(ctx, definitionRequestForStart(request), runtimeOwnerRequestForStart(request), &session, false, workerRequestForStart(request), recording, selection.ModelCacheDirectory, selection.OperatorDefaults, r.baseLogger, nil, replayInput)
	}
	// Historical replay, whether portable or legacy, is an inspection-only
	// product and must select its detached projection before live Factory
	// Runtime assembly. Resume remains an explicit live successor path below.
	if recording.ReplayPath != "" {
		if r.runtimeRoot == nil {
			return runtimeProducts{}, fmt.Errorf("open Factory Runtime: Factory Runtime root is required for replay")
		}
		if r.replayInputs == nil {
			return runtimeProducts{}, fmt.Errorf("open Factory Runtime: replay input capability is required for replay")
		}
		input, err := r.replayInputs.LoadReplayInput(
			recordings.LoadReplayInputRequest{Path: recording.ReplayPath},
		)
		if err != nil {
			// The loader has already classified and safely detached the
			// replay input. Propagating that result preserves the one-read
			// runtime-opening contract; routing the error through openRuntime
			// would ask the same loader to read the artifact again.
			return runtimeProducts{}, err
		}
		// Offline replay is a detached historical inspection. A caller that
		// explicitly requested a hosted process still owns the established
		// ordinary replay contract, which exposes the replay through its live
		// API and metrics surfaces. Keeping that distinction here prevents the
		// inspection-only product from being wrapped in host-readiness or live
		// transport lifecycle requirements.
		if replayRequestsHistoricalInspection(selection.Host) && selectsHistoricalReplayInspection(input) {
			return open(&input)
		}
		// Hosted replay and legacy V1 JSON retain the ordinary activated runtime
		// path. Keep intentionally incomplete synthetic inputs used by narrow
		// compatibility callers on that same path; the real Recordings loader
		// reports the format before this branch.
		return r.openActivatedRuntimeWithReplayInput(ctx, request, &input)
	}
	if strings.TrimSpace(recording.ResumePath) != "" {
		if r.recordingsRuntime == nil {
			return runtimeProducts{}, fmt.Errorf("open Factory Runtime: Recordings resume input capability is required")
		}
		input, err := r.recordingsRuntime.LoadResumeInput(recordings.LoadResumeInputRequest{
			Path: recording.ResumePath,
		})
		if err != nil {
			return runtimeProducts{}, fmt.Errorf("open Factory Runtime: load resume input: %w", err)
		}
		return r.openActivatedRuntimeWithResumeInput(ctx, request, &input)
	}
	return r.openActivatedRuntime(ctx, request)
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
