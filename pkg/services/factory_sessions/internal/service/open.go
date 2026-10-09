package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/modelinvocation"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeports"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/webhooks"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// openRuntimeWithOptions opens session-owned state using the collaborators already
// injected into this owner. Only invocation selections cross this boundary.
func (r *Root) openRuntimeWithOptions(
	ctx context.Context,
	definition factorydefinitions.RuntimeSelection,
	runtime factoryruntime.RuntimeSelection,
	session *factorysessions.SessionStartRequest,
	canonicalSessionIDGenerated bool,
	worker workers.RuntimeSelection,
	recording recordings.RuntimeSelection,
	modelCacheDirectory string,
	operatorDefaults operatorsettings.ResolvedDefaults,
	baseLogger *zap.Logger,
	definitionSnapshot *factorydefinitions.RuntimeSnapshot,
	replayInput *recordings.LoadReplayInputResult,
) (products runtimeProducts, err error) {
	opening, err := r.prepareRuntimeOpening(ctx, definition, runtime, session,
		canonicalSessionIDGenerated, worker, recording, modelCacheDirectory,
		operatorDefaults, baseLogger, definitionSnapshot, replayInput)
	if err != nil {
		return runtimeProducts{}, err
	}
	if opening.load.HistoricalReplay != nil {
		return r.openHistoricalSessionRuntime(ctx, opening)
	}
	cleanup := &runtimeOpeningCleanup{}
	defer func() {
		if err != nil {
			if cleanupErr := cleanup.Close(); cleanupErr != nil {
				err = errors.Join(err, cleanupErr)
				products.closeArtifacts = cleanup.Close
			}
			if opening.initial == nil {
				if logErr := r.logFailedSessionOpening(opening, err, cleanup); logErr != nil {
					err = errors.Join(err, logErr)
					products.closeArtifacts = cleanup.Close
				}
			}
		}
	}()
	resources, err := r.resourceAcquisition.Acquire(ctx, RuntimeResourceRequest{
		Configured: opening.configured, FactoryRootDir: opening.root.FactoryRootDir,
		ModelsRuntime: modelinvocation.ProjectModelsRuntimeConfig(opening.load.LoadedFactoryCfg),
	}, opening.clock, opening.logger, cleanup)
	if err != nil {
		return runtimeProducts{}, err
	}
	opening.operatorSettingsPath = resources.OperatorSettingsPath
	opening.durableExecution = resources.DurableExecution
	opening.observations = resources.Observations
	opening.modelsBind = modelsRuntimeBind{Scope: resources.ModelsScope}
	if err = r.openSessionEngine(ctx, opening, cleanup); err != nil {
		return runtimeProducts{}, err
	}
	return r.completeSessionOpening(ctx, opening, cleanup)
}

// sessionRuntimeOpening retains one opening's selections and partial results.
// Its lifetime ends with the opening; peer openings never share this state.
type sessionRuntimeOpening struct {
	configured                  preparedRuntime
	root                        RuntimeRoot
	load                        RuntimeLoad
	clock                       factoryruntime.Clock
	logger                      *zap.Logger
	sessionID                   string
	metricsSessionID            string
	providedCanonicalSessionID  string
	canonicalSessionIDGenerated bool
	sessionSelection            *factorysessions.SessionRuntimeSelection
	operatorSettingsPath        string
	durableExecution            DurableExecution
	observations                factoryruntime.SessionObservations
	modelsBind                  modelsRuntimeBind
	resumeInput                 *recordings.LoadResumeInputResult
	restoredWorldState          *factorydefinitions.FactoryWorldState
	restoredEventHistory        []factorydefinitions.FactoryEvent
	boardHistoryOpening         currentBoardHistoryOpening
	skippedBoardRecordings      []string
	hasCurrentBoardReference    bool
	emptyCurrentBoard           bool
	startupRecovery             *currentBoardStartupRecovery
	initial                     *factoryruntime.RuntimeInitialOpening
	startupRuntime              runtimeports.RuntimeInstance
	completion                  factoryruntime.RuntimeInitialCompletion
	activation                  *factoryruntime.RuntimeActivation
	recordingTargetValidator    recordings.RecordingTargetValidator
}

func (r *Root) prepareRuntimeOpening(
	ctx context.Context,
	definition factorydefinitions.RuntimeSelection,
	runtime factoryruntime.RuntimeSelection,
	session *factorysessions.SessionStartRequest,
	canonicalSessionIDGenerated bool,
	worker workers.RuntimeSelection,
	recording recordings.RuntimeSelection,
	modelCacheDirectory string,
	operatorDefaults operatorsettings.ResolvedDefaults,
	baseLogger *zap.Logger,
	definitionSnapshot *factorydefinitions.RuntimeSnapshot,
	replayInput *recordings.LoadReplayInputResult,
) (*sessionRuntimeOpening, error) {
	if session == nil {
		return nil, fmt.Errorf("factory session runtime selection is required")
	}
	if r.recordingsService == nil {
		return nil, fmt.Errorf("construct runtime scope: Recordings service is required")
	}
	if r.recordingsRuntime == nil {
		return nil, fmt.Errorf("construct runtime scope: Recordings runtime scope is required")
	}
	selection := sessionRuntimeSelection(session)
	providedCanonicalSessionID := strings.TrimSpace(selection.CanonicalSessionID)
	sessionID := strings.TrimSpace(session.SessionID)
	if sessionID == "" {
		sessionID = factorysessions.DefaultSessionID
	}
	session.SessionID = sessionID
	configured, root, load, clock, logger, err := r.preparation.Prepare(
		ctx,
		definition,
		runtime,
		*session,
		canonicalSessionIDGenerated,
		worker,
		recording,
		modelCacheDirectory,
		operatorDefaults,
		baseLogger,
		r.clock,
		definitionSnapshot,
		replayInput,
	)
	if err != nil {
		return nil, err
	}
	// Resolve and validate the Factory Definition before allocating the
	// canonical metrics identity. Invalid Current Factory or working-directory
	// input must not consume a Factory Session identity or create a product
	// lifecycle effect.
	if err := ensureDefaultCanonicalSessionID(session, recording.ReplayPath, r.generateRuntimeInstanceID); err != nil {
		return nil, err
	}
	canonicalSessionIDGenerated = canonicalSessionIDGenerated ||
		(providedCanonicalSessionID == "" && strings.TrimSpace(selection.CanonicalSessionID) != "")
	configured.CanonicalSessionIDGenerated = canonicalSessionIDGenerated
	sessionSelection := sessionRuntimeSelection(&configured.Session)
	sessionSelection.CanonicalSessionID = selection.CanonicalSessionID
	metricsSessionID := strings.TrimSpace(sessionSelection.CanonicalSessionID)
	if metricsSessionID == "" {
		metricsSessionID = sessionID
	}
	if err := applyRuntimeWorkerReasoningEffort(configured, load); err != nil {
		return nil, err
	}
	return &sessionRuntimeOpening{
		configured:                  configured,
		root:                        root,
		load:                        load,
		clock:                       clock,
		logger:                      logger,
		sessionID:                   sessionID,
		metricsSessionID:            metricsSessionID,
		providedCanonicalSessionID:  providedCanonicalSessionID,
		canonicalSessionIDGenerated: canonicalSessionIDGenerated,
		sessionSelection:            sessionSelection,
	}, nil
}

func applyRuntimeWorkerReasoningEffort(configured preparedRuntime, load RuntimeLoad) error {
	if effort := strings.TrimSpace(configured.Workers.WorkerReasoningEffort); effort != "" &&
		load.LoadedFactoryCfg != nil {
		if err := load.LoadedFactoryCfg.MutateWorkers(func(worker *factorydefinitions.FactoryWorkerConfig) error {
			if worker != nil {
				worker.ReasoningEffort = effort
			}
			return nil
		}); err != nil {
			return fmt.Errorf("apply worker reasoning effort override: %w", err)
		}
	}
	return nil
}

func (r *Root) openHistoricalSessionRuntime(ctx context.Context, opening *sessionRuntimeOpening) (runtimeProducts, error) {
	var err error
	var liveOwner durableexecution.Service
	var replayClose func() error
	if opening.load.HistoricalReplay.Checkpoint != nil {
		// Portable history owns the resume identity, independently of the CLI
		// inspection host selection (which can be ~default).
		configured := opening.configured
		configured.Session.SessionID = opening.load.HistoricalReplay.Session.SessionID
		liveOwner, replayClose, err = r.openPortableReplayDurableOwner(
			ctx,
			configured,
			opening.root,
		)
		if err != nil {
			if replayClose != nil {
				if cleanupErr := replayClose(); cleanupErr != nil {
					return runtimeProducts{closeArtifacts: replayClose}, errors.Join(err, cleanupErr)
				}
			}
			return runtimeProducts{}, err
		}
	}
	historicalProducts := r.historicalReplayRuntimeProducts(
		opening.logger,
		*opening.load.HistoricalReplay,
		liveOwner,
		replayClose,
	)
	historicalProducts.replayMetadataWarnings = append(
		[]recordings.MetadataMismatchWarning(nil),
		opening.load.ReplayMetadataWarnings...,
	)
	return historicalProducts, nil
}

func (r *Root) restoreSessionOpeningHistory(ctx context.Context, opening *sessionRuntimeOpening) error {
	var err error
	if opening.emptyCurrentBoard {
		return nil
	}
	if strings.TrimSpace(opening.configured.Recordings.ResumePath) != "" {
		input := opening.configured.Recordings.ResumeInput
		opening.resumeInput = &input
	}
	// A portable resume input owns its selected-history reconstruction. Only a
	// direct live opening should restore the current board recording; applying
	// that restart-only probe to an explicit resume artifact would reject valid
	// replay fixtures that intentionally have no current-board recording.
	canonicalSessionIDWasProvided := opening.providedCanonicalSessionID != "" && !opening.canonicalSessionIDGenerated
	// A JSONL header supplies the existing canonical identity, but that does
	// not replace reconstruction of its board and retained event prefix.
	jsonlRecording := strings.HasSuffix(strings.ToLower(opening.configured.Recordings.RecordPath), ".jsonl")
	if opening.load.ReplayArtifact == nil && opening.resumeInput == nil &&
		(!canonicalSessionIDWasProvided || opening.hasCurrentBoardReference || jsonlRecording) {
		if strings.TrimSpace(opening.configured.Recordings.RecordPath) != "" {
			opening.boardHistoryOpening, err = inspectCurrentBoardHistory(
				ctx,
				opening.durableExecution.Service,
				opening.sessionID,
			)
			if err != nil {
				return err
			}
		}
		var restoredBoard *currentBoardHistory
		restoredBoard, err = restoreCurrentBoardHistory(
			r.recordingsService,
			opening.configured.Recordings.RecordPath,
			opening.sessionID,
			opening.boardHistoryOpening.allowMissingHistory && !opening.hasCurrentBoardReference,
		)
		if err != nil {
			if opening.usesImplicitCurrentBoard() {
				return r.quarantineSelectedCurrentBoardRecording(ctx, opening, err)
			}
			logCurrentBoardHistoryFailure(
				opening.logger,
				opening.sessionID,
				factoryruntime.RecordingPath(opening.configured.Recordings.RecordPath).ForSession(opening.sessionID),
				err,
			)
			return err
		}
		if restoredBoard != nil {
			if opening.hasCurrentBoardReference {
				if err := validateCurrentBoardFactoryDirectory(restoredBoard.events, opening.load.LoadedFactoryCfg.FactoryDir()); err != nil {
					return currentBoardHistoryFailure(opening.configured.Recordings.RecordPath, opening.sessionID,
						"CORRUPT_HISTORY: selected recording does not match this repository; preserve the recording and reference", err)
				}
			}
			opening.restoredWorldState = restoredBoard.state
			opening.restoredEventHistory = restoredBoard.events
		}
	}
	return nil
}

func (r *Root) openSessionEngine(ctx context.Context, opening *sessionRuntimeOpening, cleanup *runtimeOpeningCleanup) error {
	if err := r.selectCurrentBoardReference(ctx, opening); err != nil {
		return err
	}
	selectedPath := opening.configured.Recordings.RecordPath
	if err := r.claimSessionRecordingTarget(ctx, opening, cleanup); err != nil {
		return err
	}
	if err := r.restoreSessionOpeningHistory(ctx, opening); err != nil {
		return err
	}
	if opening.configured.Recordings.RecordPath == selectedPath && opening.recordingTargetValidator != nil {
		if err := opening.recordingTargetValidator.Validate(); err != nil {
			return err
		}
	}
	if err := r.reserveFreshCurrentBoard(ctx, opening); err != nil {
		return err
	}
	if opening.configured.Recordings.RecordPath != selectedPath {
		if err := r.claimSessionRecordingTarget(ctx, opening, cleanup); err != nil {
			return err
		}
	}

	initial, err := r.initialEngine.OpenLive(ctx, opening.initialEngineRequest(), opening.observations)
	if initial != nil {
		opening.initial = initial
		opening.startupRuntime = initial.Record
		opening.completion = initial.Completion
		opening.activation = initial.Activation
		if initial.Activation != nil && initial.Activation.Close != nil {
			cleanup.Add(func() error { return initial.Activation.Close(context.WithoutCancel(ctx)) })
		}
	}
	if err != nil {
		return err
	}
	if strings.TrimSpace(opening.completion.SessionID) != opening.sessionID {
		return fmt.Errorf(
			"construct runtime scope: built Factory Session ID %q does not match requested ID %q",
			opening.completion.SessionID,
			opening.sessionID,
		)
	}
	opening.completion.CanonicalSessionIDGenerated = opening.canonicalSessionIDGenerated &&
		opening.sessionID == factorysessions.DefaultSessionID &&
		opening.metricsSessionID != factorysessions.DefaultSessionID
	opening.initial.Completion = opening.completion
	// Quarantine starts an empty board. Preserve its existing pre-host
	// reference-publication failure boundary; no retained history is activated.
	if (!opening.usesImplicitCurrentBoard() || opening.startupRecovery != nil) && strings.TrimSpace(opening.configured.Recordings.ResumePath) == "" {
		if err := opening.publishCurrentBoardReference(ctx); err != nil {
			return err
		}
	}
	if recovery := opening.startupRecovery; recovery != nil && opening.logger != nil {
		opening.logger.Warn("unreadable durable state quarantined; started an empty board",
			zap.String("file", recovery.file), zap.String("quarantined_file", recovery.quarantinedFile),
			zap.String("cause", recovery.cause))
	}
	opening.warnMissingBoardHistory()
	return nil
}

func (r *Root) claimSessionRecordingTarget(ctx context.Context, opening *sessionRuntimeOpening, cleanup *runtimeOpeningCleanup) error {
	path := strings.TrimSpace(opening.configured.Recordings.RecordPath)
	if path == "" {
		return nil
	}
	ownership, ok := r.recordingsService.(recordings.RecordingTargetOwnership)
	if !ok {
		return fmt.Errorf("%w: recording target %q ownership is unavailable", recordings.ErrRecordingBindingConflict, path)
	}
	lease, err := ownership.ClaimRecordingTarget(ctx, factoryruntime.RecordingPath(path).ForSession(opening.sessionID))
	if err != nil {
		return err
	}
	cleanup.OwnRecordingTarget(lease)
	opening.recordingTargetValidator, _ = lease.(recordings.RecordingTargetValidator)
	return nil
}

func (opening *sessionRuntimeOpening) initialEngineRequest() initialEngineLiveRequest {
	config := opening.load.LoadedFactoryCfg.FactoryConfig()
	request := initialEngineLiveRequest{
		Configured: opening.configured, EffectiveFactory: *config,
		ResumeInput: opening.resumeInput,
		Recovery: factoryruntime.RuntimeActivationRecoveryInput{
			WorldState: opening.restoredWorldState, EventHistory: opening.restoredEventHistory,
			ReplayArtifact: opening.load.ReplayArtifact,
		},
	}
	workers := config.Workers
	workstations := config.Workstations
	if snapshot := opening.configured.DefinitionSnapshot; snapshot != nil {
		workers = snapshot.Workers
		workstations = snapshot.Workstations
	}
	for _, selected := range workers {
		if worker, ok := opening.load.LoadedFactoryCfg.Worker(selected.Name); ok {
			request.Workers = append(request.Workers, *worker)
		}
	}
	for _, selected := range workstations {
		if workstation, ok := opening.load.LoadedFactoryCfg.Workstation(selected.Name); ok {
			request.Workstations = append(request.Workstations, *workstation)
		}
	}
	return request
}

func (opening *sessionRuntimeOpening) warnMissingBoardHistory() {
	if opening.boardHistoryOpening.hasDurableState && opening.restoredWorldState == nil {
		if runtimeLogger := opening.startupRuntime.RuntimeLogger(); runtimeLogger != nil {
			runtimeLogger.Warn(
				"current Factory Session board recording is absent after durable state was preserved; board contents were lost, an empty board was initialized, and preserved durable state was not deleted",
				zap.String("session_id", opening.sessionID),
				zap.String(
					"recording_path",
					factoryruntime.RecordingPath(opening.configured.Recordings.RecordPath).ForSession(opening.sessionID),
				),
				zap.String("recovery", "missing_board_recording_after_durable_state"),
			)
		}
	}
}

func (r *Root) completeSessionOpening(ctx context.Context, opening *sessionRuntimeOpening, cleanup *runtimeOpeningCleanup) (runtimeProducts, error) {
	webhookSubscription, err := startFactoryWebhookSubscription(
		ctx,
		r.webhooksService,
		opening.startupRuntime.RecordingLedger(),
		opening.load.LoadedFactoryCfg,
		opening.load.ReplayArtifact == nil,
		opening.sessionID,
	)
	if err != nil {
		return runtimeProducts{}, err
	}
	if webhookSubscription != nil {
		cleanup.Add(func() error {
			return webhookSubscription(context.WithoutCancel(ctx))
		})
	}
	if r.factoryDefinitions == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Factory Definitions service is required")
	}
	if r.definitionRuntimeRouter == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Factory Definitions runtime router is required")
	}
	sessionRuntime, definitionHost, definitionActivationGateway, release, err := r.factorySessionsRuntimeAssembly.RegisterOpening(
		ctx, roles.SessionOpeningFacts{
			FactorySessionID: opening.sessionID, RuntimeID: opening.configured.Runtime.RuntimeInstanceID,
			GenerationID: opening.startupRuntime.StreamGeneration(), FactoryRootDir: opening.root.FactoryRootDir,
			Directory: opening.configured.Definition.Directory, ExecutionBaseDir: opening.configured.Definition.ExecutionBaseDir,
			RuntimeMode: opening.configured.Runtime.Mode, BackendScopeID: opening.sessionSelection.BackendScopeID,
			WorkFile: opening.sessionSelection.WorkFile, WorkflowID: opening.configured.Recordings.WorkflowID,
			ModelsScope: opening.modelsBind.Scope,
		}, opening.initial, opening.clock, opening.startupRuntime.RuntimeLogger(),
	)
	if release != nil {
		cleanup.Add(func() error { return release(context.WithoutCancel(ctx)) })
	}
	if err != nil {
		return runtimeProducts{}, err
	}

	if bound := runtimebinding.SessionStateFrom(r.factorySessionsRuntimeAssembly.Resolve(opening.sessionID)); bound != nil {
		bound.SetMockWorkers(opening.configured.Workers.MockWorkers)
	}
	if binder, ok := opening.startupRuntime.(interface {
		BindModelsRuntimeScope(models.RuntimeScopeRef) error
	}); ok {
		if err := binder.BindModelsRuntimeScope(opening.modelsBind.Scope); err != nil {
			return runtimeProducts{}, fmt.Errorf("bind Models runtime scope to Factory Runtime: %w", err)
		}
	}
	if err := r.definitionRuntimeRouter.Bind(
		opening.sessionID,
		definitionHost,
		definitionActivationGateway,
	); err != nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: bind Factory Definitions runtime: %w", err)
	}
	cleanup.Add(func() error {
		current := r.factorySessionsRuntimeAssembly.Resolve(opening.sessionID)
		bound := runtimebinding.SessionStateFrom(current)
		if current == nil || (bound != nil && any(bound.Owner) == any(sessionRuntime)) {
			r.definitionRuntimeRouter.Unbind(opening.sessionID)
		}
		return nil
	})
	if r.processRuntimeFactory == nil {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: Factory Sessions process runtime factory is required")
	}
	var startup roles.LifecycleRuntime = sessionRuntime
	if (opening.usesImplicitCurrentBoard() && opening.startupRecovery == nil) || (opening.publishesCurrentBoardWriter() && strings.TrimSpace(opening.configured.Recordings.ResumePath) != "") {
		startup = &successorBoardStartup{LifecycleRuntime: sessionRuntime, publish: opening.publishCurrentBoardReference}
	}
	processRuntime, err := r.processRuntimeFactory.Bind(
		startup,
		factorysessions.RuntimeHostRequest{
			Directory: opening.configured.Definition.Directory, RuntimeMode: opening.configured.Runtime.Mode,
			WorkFile: opening.sessionSelection.WorkFile, MockWorkers: opening.configured.Workers.MockWorkers != nil,
			Host: opening.sessionSelection.Host.Host, Port: opening.sessionSelection.Host.Port,
			AutoPort: opening.sessionSelection.Host.AutoPort, Pprof: opening.sessionSelection.Host.Pprof,
		},
		opening.startupRuntime.RuntimeLogger(),
	)
	if err != nil {
		return runtimeProducts{}, err
	}
	products, err := r.bindSessionOpeningProducts(ctx, opening, cleanup, sessionRuntime, processRuntime)
	if err == nil {
		products.lifecycle = startup
	}
	return products, err
}

func (r *Root) bindSessionOpeningProducts(
	ctx context.Context,
	opening *sessionRuntimeOpening,
	cleanup *runtimeOpeningCleanup,
	sessionRuntime roles.ApplicationRuntime,
	processRuntime roles.ProcessRuntime,
) (runtimeProducts, error) {
	if recording, ok := opening.startupRuntime.(recordings.RuntimeRecordingStartup); ok {
		recording.DeferRecordingPublication()
	}
	rootRuntime, ok := sessionRuntime.(factoryruntime.Service)
	if !ok {
		return runtimeProducts{}, fmt.Errorf("construct runtime scope: session runtime does not implement Factory Runtime root Service")
	}
	// A JavaScript workflow's children are detached Workers. The Workers root is
	// already composed before opening; Runtime contributes only the identity and
	// resource-admission capability that the child request needs. The existing
	// live-change Runtime bind remains separate and is not an execution route.
	var resourceLeaseAdmission factoryruntime.ResourceCapacityLeaseAdmission
	if admission, ok := rootRuntime.(factoryruntime.ResourceCapacityLeaseAdmission); ok {
		resourceLeaseAdmission = admission
	}
	releaseScope, err := bindDurableExecutionCapabilities(
		opening.sessionID,
		opening.durableExecution.Service,
		rootRuntime,
		resourceLeaseAdmission,
		opening.configured.Runtime.RuntimeInstanceID,
		opening.startupRuntime.StreamGeneration(),
		opening.startupRuntime.RecordingLedger(),
		r.providerOverride,
		opening.configured.Workers.MockWorkers,
		r.providerCommandRunner,
		runtimeProgressPublisher(opening.startupRuntime),
		runtimeWorkerAttemptStarter(opening.startupRuntime),
	)
	if err != nil {
		return runtimeProducts{}, err
	}
	cleanup.Add(func() error { releaseScope(); return nil })
	opened := assembleRuntimeProducts(
		ctx,
		r.SessionGateway,
		rootRuntime,
		opening.modelsBind.Scope,
		opening.startupRuntime,
		sessionRuntime,
		processRuntime,
		opening.configured.Definition.Directory,
		opening.configured.Runtime.RuntimeInstanceID,
		opening.sessionSelection.BackendScopeID,
		cleanup.Close,
		opening.sessionID,
	)
	opened.engine = opening.activation.Service
	opened.activation = opening.activation
	opened.clock = opening.clock
	opened.orderlyStop = opening.orderlyCurrentBoardStop(newOrderlyRecordingFlush(
		r.recordingsService,
		opened.runtimeInstanceID,
		opening.configured.Recordings.RecordPath,
	))
	opened.skippedBoardRecordings = append([]string(nil), opening.skippedBoardRecordings...)
	opened.currentBoardRecordPath = opening.configured.Recordings.RecordPath
	if recovery := opening.startupRecovery; recovery != nil {
		opened.startupRecovery = &factorysessions.StartupRecovery{
			Code: "DURABLE_STATE_QUARANTINED", File: recovery.file,
			Cause: recovery.cause, QuarantinedFile: recovery.quarantinedFile,
		}
	}
	opened.operatorSettingsPath = opening.operatorSettingsPath
	opened.workerSettings = opening.durableExecution.WorkerSettings
	opened.replayMetadataWarnings = append(
		[]recordings.MetadataMismatchWarning(nil),
		opening.load.ReplayMetadataWarnings...,
	)
	return opened, nil
}

func startFactoryWebhookSubscription(
	ctx context.Context,
	webhooksService webhooks.Service,
	ledger recordings.Ledger,
	loaded factorydefinitions.MutableLoadedFactorySource,
	active bool,
	sessionID string,
) (webhooks.Subscription, error) {
	if !active || loaded == nil || !hasEnabledWebhooks(loaded.FactoryConfig()) {
		return nil, nil
	}
	if strings.TrimSpace(sessionID) == "" {
		sessionID = factorysessions.DefaultSessionID
	}
	scope := recordings.CanonicalEventScope{FactorySessionID: sessionID}
	return webhooksService.Start(ctx, webhooks.StartRequest{
		Definitions:      loaded.FactoryConfig().Webhooks,
		Scope:            scope,
		ActivationCursor: lastCanonicalCursor(ledger, scope),
		RuntimeSource:    loaded,
		DeadLetterPath:   factoryWebhookDeadLetterPath(loaded),
	})
}

func factoryWebhookDeadLetterPath(loaded factorydefinitions.LoadedFactorySource) string {
	if loaded == nil {
		return ""
	}
	baseDir := strings.TrimSpace(loaded.RuntimeBaseDir())
	if baseDir == "" {
		baseDir = strings.TrimSpace(loaded.FactoryDir())
	}
	if baseDir == "" {
		return ""
	}
	return filepath.Join(baseDir, filepath.FromSlash(webhooks.DeadLetterRelativePath))
}

func hasEnabledWebhooks(config *factorydefinitions.FactoryConfig) bool {
	if config == nil {
		return false
	}
	for _, webhook := range config.Webhooks {
		if webhook.Enabled {
			return true
		}
	}
	return false
}

func lastCanonicalCursor(
	ledger recordings.Ledger,
	scope recordings.CanonicalEventScope,
) *recordings.CanonicalEventCursor {
	if ledger == nil {
		return nil
	}
	events := ledger.CanonicalEvents()
	for index := len(events) - 1; index >= 0; index-- {
		event := events[index]
		if scope.FactorySessionID != "" &&
			(event.Context.SessionID == nil || *event.Context.SessionID != scope.FactorySessionID) {
			continue
		}
		return &recordings.CanonicalEventCursor{
			StreamGenerationID: ledger.StreamGenerationID(),
			Sequence:           recordings.CanonicalEventSequence(event.Context.Sequence),
		}
	}
	return nil
}

// workerScopeBinder accepts runtime-owned facts without replacing Workers.
type workerScopeBinder interface {
	BindWorkerScope(
		string,
		factoryruntime.ResourceCapacityLeaseAdmission,
		string,
		string,
		providers.Service,
		*workers.MockWorkersConfig,
		platformprocess.CommandRunner,
		workers.ProgressPublisher,
		func(context.Context, *workers.ExecuteRequest) (func(context.Context, workers.ExecuteResult, error) (workers.ExecuteResult, error), error),
	) (func(), error)
}

func bindWorkerScope(
	sessionID string,
	execution any,
	admission factoryruntime.ResourceCapacityLeaseAdmission,
	runtimeID string,
	generationID string,
	providerOverride providers.Service,
	mockWorkers *workers.MockWorkersConfig,
	commandRunnerOverride platformprocess.CommandRunner,
	progressPublisher workers.ProgressPublisher,
	attemptStarter func(context.Context, *workers.ExecuteRequest) (func(context.Context, workers.ExecuteResult, error) (workers.ExecuteResult, error), error),
) (func(), error) {
	binder, ok := execution.(workerScopeBinder)
	if !ok {
		return nil, fmt.Errorf("bind worker scope for Factory Session %q: live child scope binder is required", strings.TrimSpace(sessionID))
	}
	release, err := binder.BindWorkerScope(sessionID, admission, runtimeID, generationID, providerOverride, mockWorkers, commandRunnerOverride, progressPublisher, attemptStarter)
	if err != nil {
		return nil, fmt.Errorf("bind worker scope for Factory Session %q: %w", strings.TrimSpace(sessionID), err)
	}
	return release, nil
}

type runtimeProgressPublisherProvider interface {
	RuntimeProgressPublisher() workers.ProgressPublisher
}

func runtimeProgressPublisher(runtime runtimeports.RuntimeInstance) workers.ProgressPublisher {
	if runtime == nil {
		return nil
	}
	if provider, ok := runtime.(runtimeProgressPublisherProvider); ok {
		return provider.RuntimeProgressPublisher()
	}
	if service := runtime.RuntimeService(); service != nil {
		if provider, ok := service.(runtimeProgressPublisherProvider); ok {
			return provider.RuntimeProgressPublisher()
		}
	}
	return nil
}

type runtimeWorkerAttemptStarterProvider interface {
	BeginWorkerAttempt(
		context.Context,
		*workers.ExecuteRequest,
	) (func(context.Context, workers.ExecuteResult, error) (workers.ExecuteResult, error), error)
}

func runtimeWorkerAttemptStarter(
	runtime runtimeports.RuntimeInstance,
) func(context.Context, *workers.ExecuteRequest) (func(context.Context, workers.ExecuteResult, error) (workers.ExecuteResult, error), error) {
	if runtime == nil {
		return nil
	}
	if provider, ok := runtime.(runtimeWorkerAttemptStarterProvider); ok {
		return provider.BeginWorkerAttempt
	}
	if service := runtime.RuntimeService(); service != nil {
		if provider, ok := service.(runtimeWorkerAttemptStarterProvider); ok {
			return provider.BeginWorkerAttempt
		}
	}
	return nil
}

type historicalRecordingReader interface {
	QueryHistoricalRecording(recordings.HistoricalRecordingQueryRequest) (recordings.HistoricalRecordingQueryResult, error)
}

// A history read can fail before Factory Runtime has opened its sinks. Retain
// that failed opening's diagnostic through the same injected log owner, without
// constructing a runtime or binding a recording merely to report its failure.
func (r *Root) logFailedSessionOpening(opening *sessionRuntimeOpening, cause error, cleanup *runtimeOpeningCleanup) error {
	if opening.configured.Runtime.FileLoggingPolicy == factoryruntime.RuntimeFileLoggingPolicyDisabled {
		return nil
	}
	if r.runtimeLogs == nil {
		return fmt.Errorf("log failed session opening: runtime log owner is required")
	}
	sink, openErr := r.runtimeLogs.Open(opening.logger, factoryruntime.RuntimeLogScopeRequest{
		SessionID: opening.sessionID, RuntimeInstanceID: opening.configured.Runtime.RuntimeInstanceID,
		FactoryDirectory: opening.load.LoadedFactoryCfg.FactoryDir(),
		RootDirectory:    opening.configured.Runtime.LogDirectory,
		Policy:           opening.configured.Runtime.FileLoggingPolicy, Config: opening.configured.Runtime.LogConfig,
	})
	if sink == nil {
		if openErr != nil {
			return fmt.Errorf("log failed session opening: %w", openErr)
		}
		return fmt.Errorf("log failed session opening: runtime log owner returned nil scope")
	}
	if logger := sink.Logger(); logger != nil {
		logger.Error("Factory Session runtime startup failed",
			zap.String("session_id", opening.sessionID),
			zap.String("failure_class", "runtime_startup_failed"),
			zap.String("cause", logging.SafeErrorCause(cause)),
		)
	}
	closeErr := sink.Close()
	if closeErr != nil {
		cleanup.Add(sink.Close)
	}
	return errors.Join(openErr, closeErr)
}
