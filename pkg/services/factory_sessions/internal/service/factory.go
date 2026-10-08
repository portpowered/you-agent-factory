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
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/webhooks"
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
	durableOpening                 *DurableOpening
	modelInvocation                modelinvocation.RuntimeModelInvocationOperation
	workerService                  workers.Service
	modelService                   models.Service
	automationService              automations.Service
	recordingsService              recordings.Service
	recordingsRuntime              recordings.RuntimeScopeService
	recordingProjections           recordings.ProjectionService
	replayInputs                   recordings.ReplayInputLoader
	webhooksService                webhooks.Service
	factoryDefinitions             factorydefinitions.Service
	definitionRuntimeRouter        *factorysessions.DefinitionRuntimeRouter
	factoryScaffoldInitializer     factorysessions.FactoryScaffoldInitializer
	editableFactoryValidator       factorysessions.EditableFactoryValidator
	initialActivation              factoryruntime.InitialRuntimeActivationOperation
	workService                    work.Service
	providerSessions               providersessions.Service
	factoryDefinitionValidator     factorydefinitions.Validator
	namedPaths                     factorydefinitions.NamedPathResolver
	factoryWorkflows               factoryruntime.JavaScriptWorkflowDefinitions
	workflowPreview                factoryruntime.WorkflowPreviewOperation
	runtimeInputLoading            *RuntimeInputLoading
	snapshotSelection              *RuntimeSnapshotSelection
	resolveClock                   factoryruntime.ClockResolver
	baseLogger                     *zap.Logger
	processRuntimeFactory          roles.ProcessRuntimeFactory
	ensureOperatorBackendScope     operatorsettings.BackendScopeEnsurer
	generateSessionID              factorysessions.SessionIDGenerator
	generateRuntimeInstanceID      factorysessions.RuntimeInstanceIDGenerator
	resolveHome                    factorysessions.HomeDirectoryResolver
	providerIdentities             factorysessions.ProviderIdentityResolver
	factorySessionsRuntimeAssembly roles.RuntimeAssembly
	runtimeRoot                    FactoryRuntimeRoot
	clock                          factoryruntime.Clock
	providerOverride               providers.Service
	providerCommandRunner          platformprocess.CommandRunner
	scriptCommandRunner            platformprocess.CommandRunner
	submissionRecorder             recordings.SubmissionRecorder
	dispatchRecorder               recordings.DispatchRecorder
}

func NewRoot(
	providerSessions providersessions.Service,
	logger *zap.Logger,
	factoryWorkflows factoryruntime.JavaScriptWorkflowDefinitions,
	workflowPreview factoryruntime.WorkflowPreviewOperation,
	runtimeRoot FactoryRuntimeRoot,
	resolveClock factoryruntime.ClockResolver,
	clock factoryruntime.Clock,
	providerOverride ProviderOverrideService,
	submissionRecorder recordings.SubmissionRecorder,
	dispatchRecorder recordings.DispatchRecorder,
	validator factorydefinitions.Validator,
	namedPaths factorydefinitions.NamedPathResolver,
	definitions factorydefinitions.Service,
	runtimeRouter *factorysessions.DefinitionRuntimeRouter,
	runtimeInputLoading *RuntimeInputLoading,
	snapshotSelection *RuntimeSnapshotSelection,
	assembly roles.RuntimeAssembly,
	durableOpening *DurableOpening,
	factoryScaffoldInitializer factorysessions.FactoryScaffoldInitializer,
	editableFactoryValidator factorysessions.EditableFactoryValidator,
	processRuntimeFactory roles.ProcessRuntimeFactory,
	generateSessionID factorysessions.SessionIDGenerator,
	generateRuntimeInstanceID factorysessions.RuntimeInstanceIDGenerator,
	resolveHome factorysessions.HomeDirectoryResolver,
	providerIdentities factorysessions.ProviderIdentityResolver,
	workService work.Service,
	automationService automations.Service,
	webhooksService webhooks.Service,
	modelService models.Service,
	recordingsService recordings.Service,
	recordingsRuntime recordings.RuntimeScopeService,
	workerService workers.Service,
	providerCommandRunner ProviderCommandRunner,
	scriptCommandRunner ScriptCommandRunner,
	ensureBackendScope operatorsettings.BackendScopeEnsurer,
	initialActivation factoryruntime.InitialRuntimeActivationOperation,
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
		initialActivation:              initialActivation,
		modelInvocation:                modelInvocation,
		durableOpening:                 durableOpening,
		workerService:                  workerService,
		modelService:                   modelService,
		automationService:              automationService,
		factorySessionsRuntimeAssembly: assembly,
		recordingsService:              recordingsService,
		recordingsRuntime:              recordingsRuntime,
		recordingProjections:           recordingProjections,
		replayInputs:                   recordingsRuntime,
		webhooksService:                webhooksService,
		factoryDefinitions:             definitions,
		definitionRuntimeRouter:        runtimeRouter,
		factoryScaffoldInitializer:     factoryScaffoldInitializer,
		editableFactoryValidator:       editableFactoryValidator,
		workService:                    workService,
		providerSessions:               providerSessions,
		factoryDefinitionValidator:     validator,
		namedPaths:                     namedPaths,
		factoryWorkflows:               factoryWorkflows,
		workflowPreview:                workflowPreview,
		runtimeInputLoading:            runtimeInputLoading,
		snapshotSelection:              snapshotSelection,
		resolveClock:                   resolveClock,
		baseLogger:                     logger,
		processRuntimeFactory:          processRuntimeFactory,
		generateSessionID:              generateSessionID,
		ensureOperatorBackendScope:     ensureBackendScope,
		generateRuntimeInstanceID:      generateRuntimeInstanceID,
		resolveHome:                    resolveHome,
		providerIdentities:             providerIdentities,
		clock:                          clock,
		providerOverride:               providerOverride,
		providerCommandRunner:          providerCommandRunner,
		scriptCommandRunner:            scriptCommandRunner,
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
