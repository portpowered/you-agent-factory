// Package run implements the agent-factory run command behavior.
package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"

	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/runconfig"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"

	"github.com/portpowered/infinite-you/pkg/initializer"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	state "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimecli "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/cli"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionscli "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingscli "github.com/portpowered/infinite-you/pkg/services/recordings/transports/cli"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"go.uber.org/zap"
)

type RunConfig = runconfig.Config

// ValidateRecordingInvocationFlags exposes the pure Recordings input-shape
// check to the process startup boundary before any local activation effects.
func ValidateRecordingInvocationFlags(cfg RunConfig) error {
	return recordingscli.ValidateInvocationFlags(recordingscli.InvocationRequest{
		RecordPath:              cfg.RecordPath,
		ReplayPath:              cfg.ReplayPath,
		ResumePath:              cfg.ResumePath,
		DisableDefaultRecording: cfg.DisableDefaultRecording,
	})
}

// ModelCacheDirEnvironment selects the managed local-model cache root at the
// customer process boundary.
const ModelCacheDirEnvironment = "INFINITE_YOU_OMNIVOICE_CACHE_DIR"

type factoryServiceRunner interface {
	Run(ctx context.Context) error
}

func runRemoteResponseStream(
	ctx context.Context,
	cfg RunConfig,
	server string,
	start factoryapi.FactorySessionExecutionResponse,
	requestID string,
	events RemoteInvocationEventOperation,
	results RemoteInvocationResultOperation,
	presentation factoryvisualization.ResponsePresentation,
) (apisurface.FactoryInvocationResult, visualizationcliFactoryEventRenderer, error) {
	renderer, err := invocationFactoryEventRenderer(cfg, presentation)
	if err != nil {
		return apisurface.FactoryInvocationResult{}, nil, err
	}
	if renderer == nil {
		return apisurface.FactoryInvocationResult{}, nil, fmt.Errorf("remote response stream renderer is required")
	}
	if results == nil {
		return apisurface.FactoryInvocationResult{}, renderer, fmt.Errorf("remote response stream result operation is required")
	}
	return followRemoteResponseStream(ctx, cfg, server, start, requestID, events, results, renderer)
}

func followRemoteResponseStream(
	ctx context.Context,
	cfg RunConfig,
	server string,
	start factoryapi.FactorySessionExecutionResponse,
	requestID string,
	events RemoteInvocationEventOperation,
	results RemoteInvocationResultOperation,
	renderer visualizationcliFactoryEventRenderer,
) (apisurface.FactoryInvocationResult, visualizationcliFactoryEventRenderer, error) {
	if events == nil {
		return apisurface.FactoryInvocationResult{}, renderer, errors.New("remote response stream event operation is required")
	}

	cursor := RemoteInvocationEventCursor{}
	reconnectAttempts := 0
	for {
		if err := readRemoteFactoryEvents(ctx, cfg, server, start.SessionId, events, renderer, &cursor, &reconnectAttempts); err != nil {
			return apisurface.FactoryInvocationResult{}, renderer, err
		}

		result, err := results.GetFactorySessionResult(ctx, RemoteInvocationResultRequest{
			Server:      server,
			SessionID:   start.SessionId,
			Diagnostics: cfg.Diagnostics,
			Verbose:     cfg.Verbose,
		})
		if err != nil {
			return apisurface.FactoryInvocationResult{}, renderer, err
		}
		invocationResult, ready, poll, err := remoteInvocationResultFromDurable(result, start.SessionId, requestID)
		if err != nil {
			return apisurface.FactoryInvocationResult{}, renderer, err
		}
		if ready {
			return invocationResult, renderer, nil
		}
		if !poll {
			return apisurface.FactoryInvocationResult{}, renderer, &InvocationError{
				Code:    RemoteDurableResponseInvalidCode,
				Message: "remote durable result ended without a terminal classification",
			}
		}
		reconnectAttempts = 0
		if err := factorysessionscli.Wait(ctx, remoteDurableResultPollInterval); err != nil {
			return apisurface.FactoryInvocationResult{}, renderer, err
		}
	}
}

func readRemoteFactoryEvents(
	ctx context.Context,
	cfg RunConfig,
	server string,
	sessionID string,
	events RemoteInvocationEventOperation,
	renderer visualizationcliFactoryEventRenderer,
	cursor *RemoteInvocationEventCursor,
	reconnectAttempts *int,
) error {
	for {
		stream, err := events.OpenFactorySessionEvents(ctx, RemoteInvocationEventRequest{
			Server:        server,
			SessionID:     sessionID,
			AfterEventID:  cursor.EventID,
			AfterSequence: cursor.Sequence,
			Diagnostics:   cfg.Diagnostics,
			Verbose:       cfg.Verbose,
		})
		if err == nil {
			if stream == nil {
				err = errors.New("remote Factory Event stream is unavailable")
			} else {
				err = consumeRemoteFactoryEventStream(ctx, stream, renderer, cursor)
				_ = stream.Close()
			}
		}
		if err == nil || errors.Is(err, io.EOF) {
			return nil
		}
		if retryErr := retryRemoteFactoryEventStream(ctx, server, sessionID, reconnectAttempts, err); retryErr != nil {
			return retryErr
		}
	}
}

// visualizationcliFactoryEventRenderer is kept as a narrow local alias so
// remote response-stream orchestration depends only on the renderer contract.
type visualizationcliFactoryEventRenderer interface {
	PresentFactoryEvents([]interfaces.FactoryEvent)
	StopProgressRendering()
	WriteFinalInvocationResult(apisurface.FactoryInvocationResult) error
}

func consumeRemoteFactoryEventStream(
	ctx context.Context,
	stream RemoteInvocationEventStream,
	renderer visualizationcliFactoryEventRenderer,
	cursor *RemoteInvocationEventCursor,
) error {
	if stream == nil {
		return errors.New("remote Factory Event stream is unavailable")
	}
	if renderer == nil {
		return errors.New("remote response stream renderer is unavailable")
	}
	for {
		event, err := stream.Next(ctx)
		if err != nil {
			return err
		}
		domainEvent, err := interfaces.NewFactoryEvent(event)
		if err != nil {
			return fmt.Errorf("decode remote canonical Factory Event %q: %w", event.Id, err)
		}
		renderer.PresentFactoryEvents([]interfaces.FactoryEvent{domainEvent})
		if cursor != nil {
			*cursor = remoteFactoryEventCursor(event)
		}
	}
}

func retryRemoteFactoryEventStream(
	ctx context.Context,
	server string,
	sessionID string,
	attempts *int,
	cause error,
) error {
	if !remoteFactoryEventRetryable(cause) {
		return cause
	}
	if attempts == nil {
		return cause
	}
	if *attempts >= remoteFactoryEventMaxReconnectAttempts {
		return fmt.Errorf(
			"remote Factory Event stream for session %q reconnect attempts exhausted after %d attempt(s): %w",
			sessionID,
			*attempts,
			cause,
		)
	}
	(*attempts)++
	if err := waitForRemoteEventReconnect(ctx); err != nil {
		return &InvocationError{
			Code:    RemoteDurableResultCode,
			Message: fmt.Sprintf("remote Factory Event stream reconnect canceled at %s: %v", safeRemoteEndpoint(server), err),
			Cause:   err,
		}
	}
	return nil
}

func remoteResponseStreamFailureResult(requestID, sessionID string, err error) apisurface.FactoryInvocationResult {
	status := interfaces.InvocationTerminalStatusFailed
	code := RemoteDurableResultCode
	message := "remote Factory Event stream failed before the invocation result was available"
	if err != nil {
		message = strings.TrimSpace(err.Error())
	}
	return remoteDurableInvocationFailure(requestID, sessionID, status, code, message, nil)
}

// RuntimeRunner is the local in-process runtime seam used by CLI startup.
type RuntimeRunner = factoryServiceRunner

// RuntimeRunnerBuilder is the CLI edge adapter for one owner-bounded Factory
// Sessions request. Presentation state is registered with the process-scoped
// opening owner before this value-only request is built.
type RuntimeRunnerBuilder func(
	context.Context,
	*factorysessions.RuntimeOpeningRequest,
	initializer.InvocationCancellation,
	factorysessions.VisualizationSinkID,
) (initializer.LocalRuntimeRunner, error)

// RuntimeOpeningRequestFactory is the Wire-selected mapping from CLI values
// to one immutable Factory Sessions operation request.
type RuntimeOpeningRequestFactory func(
	RunConfig,
	*workers.MockWorkersConfig,
) *factorysessions.RuntimeOpeningRequest

type Opener func(
	context.Context,
	RunConfig,
	RuntimeRunnerBuilder,
	InvocationOperation,
	factoryvisualization.ResponsePresentation,
) (*Operation, error)

const (
	completedPlaceIDSuffix  = "completed"
	failedPlaceIDSuffix     = "failed"
	defaultFactorySessionID = "~default"
)

func bootstrapFactoryWithInitializer(
	dir string,
	initialize interfaces.ScaffoldInitializer,
	resolve interfaces.CurrentFactoryDirectoryResolver,
	directories platformfilesystem.DirectoryCreator,
) error {
	if resolve == nil {
		return fmt.Errorf("bootstrap Factory Definition at %q: current Factory resolver is required", dir)
	}
	resolvedDir, err := resolve(dir)
	if err != nil {
		if errors.Is(err, interfaces.ErrFactoryLayoutNotFound) {
			if initialize == nil {
				return fmt.Errorf("bootstrap Factory Definition at %q: scaffold initializer is required", dir)
			}
			return initialize(interfaces.ScaffoldConfig{Dir: dir})
		}
		return err
	}

	defaultInputDir := filepath.Join(resolvedDir, interfaces.InputsDir, interfaces.DefaultFactoryInputType, interfaces.DefaultChannelName)
	if directories == nil {
		return fmt.Errorf("bootstrap Factory Definition at %q: directory creator is required", dir)
	}
	return directories.MkdirAll(defaultInputDir, 0o755)
}

type resolvedRunRecordPath struct {
	servicePath   string
	reportedPath  string
	autoGenerated bool
}

// Operation is one invocation-local run selected by the customer command.
// Its runtime state is opened through injected service operations.
type Operation struct {
	cfg                    RunConfig
	logger                 *zap.Logger
	runner                 RuntimeRunner
	batchReportProvider    batchReportProvider
	invocationRequest      *factoryapi.InvocationRequest
	invocationTarget       factorysessions.InvocationTarget
	invocation             InvocationOperation
	presentation           factoryvisualization.ResponsePresentation
	invocationMode         bool
	startupPrepared        bool
	recordPath             resolvedRunRecordPath
	hostedInvocation       HostedInvocationOperation
	historicalReplay       *factorysessions.HistoricalReplayInspection
	replayMetadataWarnings []recordings.MetadataMismatchWarning
	openingPresentations   factorysessions.OpeningPresentationOwner
	visualizations         factoryvisualization.RuntimeSinkOwner
	visualizationSinkID    factoryvisualization.RuntimeSinkID
}

// Open resolves run inputs and opens invocation-local runtime state without
// starting its transport, sidecars, or runtime loop.
func Open(
	ctx context.Context,
	cfg RunConfig,
	buildRunner RuntimeRunnerBuilder,
	invocation InvocationOperation,
	presentation factoryvisualization.ResponsePresentation,
	prepareWorkTarget work.SingleWorkTargetPreparation,
	loadMockWorkers workers.MockWorkersConfigLoader,
	buildRuntimeRequest RuntimeOpeningRequestFactory,
	presentations ...factorysessions.OpeningPresentationOwner,
) (*Operation, error) {
	var presentationOwner factorysessions.OpeningPresentationOwner
	if len(presentations) > 0 {
		presentationOwner = presentations[0]
	}
	return open(ctx, cfg, buildRunner, invocation, presentation, prepareWorkTarget,
		loadMockWorkers, nil, buildRuntimeRequest, presentationOwner, nil)
}

// OpenWithVisualizationOwnerAndDiagnostics is the canonical composition entry
// point when the mock-worker loader also reports ignored forward-compatible
// fields. The older Open functions remain available for callers that provide
// only the established loader contract.
func OpenWithVisualizationOwnerAndDiagnostics(
	ctx context.Context,
	cfg RunConfig,
	buildRunner RuntimeRunnerBuilder,
	invocation InvocationOperation,
	presentation factoryvisualization.ResponsePresentation,
	prepareWorkTarget work.SingleWorkTargetPreparation,
	loadMockWorkers workers.MockWorkersConfigLoader,
	loadMockWorkersWithDiagnostics workers.MockWorkersConfigDiagnosticsLoader,
	buildRuntimeRequest RuntimeOpeningRequestFactory,
	presentations factorysessions.OpeningPresentationOwner,
	visualizations factoryvisualization.RuntimeSinkOwner,
) (*Operation, error) {
	return open(ctx, cfg, buildRunner, invocation, presentation, prepareWorkTarget,
		loadMockWorkers, loadMockWorkersWithDiagnostics, buildRuntimeRequest, presentations, visualizations)
}

func open(
	ctx context.Context,
	cfg RunConfig,
	buildRunner RuntimeRunnerBuilder,
	invocation InvocationOperation,
	presentation factoryvisualization.ResponsePresentation,
	prepareWorkTarget work.SingleWorkTargetPreparation,
	loadMockWorkers workers.MockWorkersConfigLoader,
	loadMockWorkersWithDiagnostics workers.MockWorkersConfigDiagnosticsLoader,
	buildRuntimeRequest RuntimeOpeningRequestFactory,
	presentationOwner factorysessions.OpeningPresentationOwner,
	visualizations factoryvisualization.RuntimeSinkOwner,
) (*Operation, error) {
	canonicalReasoningEffort, err := NormalizeWorkerReasoningEffort(cfg.WorkerReasoningEffort)
	if err != nil {
		return nil, err
	}
	cfg.WorkerReasoningEffort = canonicalReasoningEffort
	cfg = normalizeRunInvocationMode(cfg)
	logger := cfg.Logger
	if logger == nil {
		logger = zap.NewNop()
	}
	cfg, invocationRequest, invocationMode, recordPath, err := prepareRunConfig(cfg, prepareWorkTarget)
	if err != nil {
		return nil, err
	}

	mockWorkersConfig, ignoredJSONPaths, err := loadSelectedMockWorkersConfigWithDiagnostics(
		cfg, loadMockWorkers, loadMockWorkersWithDiagnostics,
	)
	if err != nil {
		return nil, err
	}
	if len(ignoredJSONPaths) > 0 {
		logger.Warn(
			"workers.mock_workers_config.unknown_fields_ignored",
			zap.String("path", cfg.MockWorkersConfigPath),
			zap.Strings("json_paths", ignoredJSONPaths),
		)
	}

	requestedPort := cfg.Port
	emitNamedFactoryResolutionDiagnostics(cfg, logger)

	if invocationMode && !cfg.WithServer && !cfg.WithSite {
		emitVerboseStartupDiagnostics(cfg, recordPath, requestedPort)
		return openInvocation(
			ctx,
			cfg,
			logger,
			invocationRequest,
			recordPath,
			invocation,
			presentation,
			mockWorkersConfig,
			presentationOwner,
		)
	}

	operation, err := openHostedRuntime(
		ctx, cfg, logger, invocationRequest, recordPath, invocation, presentation,
		prepareWorkTarget, mockWorkersConfig, invocationMode, requestedPort,
		buildRunner, buildRuntimeRequest, presentationOwner, visualizations,
	)
	return operation, classifyRunInputFailure(cfg, err)
}

// NormalizeWorkerReasoningEffort validates and canonicalizes the run-scoped
// worker reasoning-effort override before any Factory Runtime is constructed.
func NormalizeWorkerReasoningEffort(value string) (string, error) {
	canonical, ok := interfaces.CanonicalizeReasoningEffort(value)
	if !ok {
		return "", fmt.Errorf("invalid --worker-reasoning-effort %q: expected one of minimal, low, medium, high, xhigh, or max", value)
	}
	return canonical, nil
}

// Run activates an operation that was opened successfully.
func (operation *Operation) Run(ctx context.Context) error {
	if operation == nil {
		return fmt.Errorf("run local operation: operation is required")
	}
	if operation.visualizations != nil {
		defer operation.visualizations.CloseRuntimeSink(operation.visualizationSinkID)
	}
	if operation.historicalReplay != nil {
		if operation.runner == nil {
			return fmt.Errorf("run historical replay: runtime runner is required")
		}
		if err := operation.prepareStartup(ctx, true); err != nil {
			return err
		}
		if err := operation.runner.Run(ctx); err != nil {
			return err
		}
		return emitHistoricalReplayInspection(operation.cfg.Output, *operation.historicalReplay)
	}
	if operation.invocationMode {
		if err := operation.prepareStartup(ctx, false); err != nil {
			return err
		}
		if operation.runner != nil {
			if runner, ok := operation.runner.(initializer.CompletionRuntimeRunner); ok {
				return runner.RunWithCompletion(ctx, operation.runInvocation)
			}
			return operation.runner.Run(ctx)
		}
		return operation.runInvocation(ctx)
	}

	if operation.cfg.Port <= 0 {
		if err := operation.prepareStartup(ctx, true); err != nil {
			return err
		}
		emitStartupDetails(operation.cfg, runtimeLogDiagnosticsForRunner(operation.runner))
	}

	if err := runFactoryServiceAndEmitResult(
		ctx,
		operation.cfg,
		operation.runner,
		operation.recordPath,
		operation.batchReportProvider,
	); err != nil {
		return err
	}
	if operation.cfg.JSONOutput {
		return nil
	}
	return emitReplayMetadataWarnings(replayMetadataOutput(operation.cfg), operation.replayMetadataWarnings)
}

func (operation *Operation) prepareStartup(ctx context.Context, discloseHome bool) error {
	if operation == nil {
		return fmt.Errorf("prepare local startup: operation is required")
	}
	if operation.startupPrepared {
		return nil
	}
	if operation.cfg.StartupPreparation != nil {
		if err := operation.cfg.StartupPreparation(ctx, discloseHome, operation.cfg.StartupOutput); err != nil {
			return err
		}
		operation.startupPrepared = true
		return nil
	}
	if discloseHome {
		emitHomeDirectoryDisclosure(operation.cfg)
	}
	operation.startupPrepared = true
	return nil
}

func emitHistoricalReplayInspection(
	output io.Writer,
	inspection factorysessions.HistoricalReplayInspection,
) error {
	if output == nil {
		return nil
	}
	if _, err := fmt.Fprintf(
		output,
		"Replayed Factory Session: %s\nSource: %s\nStatus: %s\nResult: %s\n",
		inspection.Session.SessionID,
		inspection.Session.ResolvedSource.SourceRef,
		inspection.Session.Status,
		inspection.Result.ResultStatus,
	); err != nil {
		return fmt.Errorf("write historical replay inspection: %w", err)
	}
	if _, err := fmt.Fprintf(
		output,
		"Worker history: %s (reason=%s)\n",
		inspection.WorkerHistory.Availability,
		inspection.WorkerHistory.Reason,
	); err != nil {
		return fmt.Errorf("write historical replay inspection: %w", err)
	}
	if inspection.Checkpoint != nil {
		if _, err := fmt.Fprintf(
			output,
			"Checkpoint: %s (%s)\n",
			inspection.Checkpoint.ID,
			inspection.Checkpoint.Summary,
		); err != nil {
			return fmt.Errorf("write historical replay inspection: %w", err)
		}
	}
	if _, err := fmt.Fprintf(
		output,
		"Artifacts: %d\nEvents: %d\nRedaction: runtimeStateOmitted=%t checkpointBodiesOmitted=%t providerTranscriptsOmitted=%t childDispatchesOmitted=%t secretsRedacted=%d\n",
		len(inspection.Artifacts.Artifacts),
		len(inspection.Events.Events),
		inspection.Redaction.RuntimeStateOmitted,
		inspection.Redaction.CheckpointBodiesOmitted,
		inspection.Redaction.ProviderTranscriptsOmitted,
		inspection.Redaction.ChildDispatchesOmitted,
		inspection.Redaction.SecretsRedacted,
	); err != nil {
		return fmt.Errorf("write historical replay inspection: %w", err)
	}
	for _, artifact := range inspection.Artifacts.Artifacts {
		if _, err := fmt.Fprintf(output, "Artifact: %s (%s)\n", artifact.ID, artifact.Kind); err != nil {
			return fmt.Errorf("write historical replay inspection: %w", err)
		}
	}
	for index, event := range inspection.Events.Events {
		var summary struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		}
		if err := json.Unmarshal(event, &summary); err != nil {
			return fmt.Errorf("write historical replay inspection: decode event %d: %w", index, err)
		}
		if _, err := fmt.Fprintf(output, "Event %d: %s (%s)\n", index, summary.Type, summary.ID); err != nil {
			return fmt.Errorf("write historical replay inspection: %w", err)
		}
	}
	return nil
}

func (operation *Operation) runInvocation(ctx context.Context) error {
	if operation == nil || operation.invocationRequest == nil {
		return fmt.Errorf("run factory invocation: operation is required")
	}
	target := operation.invocationTarget
	invocation := operation.invocation
	if operation.hostedInvocation != nil {
		invocation = &hostedInvocationOperation{
			delegate: invocation, hosted: operation.hostedInvocation,
			logger: operation.logger, presentations: operation.openingPresentations,
		}
	}
	var startedAt time.Time
	if operation.cfg.CleanInvocation {
		if operation.cfg.Clock == nil {
			return fmt.Errorf("run clock is required")
		}
		startedAt = operation.cfg.Clock.Now().UTC()
		recordCleanInvocationAttempt()
	}
	result, err := runFactoryInvocationWithResult(
		ctx, operation.cfg, target, *operation.invocationRequest,
		invocation, operation.presentation, operation.openingPresentations,
	)
	if operation.cfg.CleanInvocation {
		duration := operation.cfg.Clock.Now().Sub(startedAt)
		var resultPtr *apisurface.FactoryInvocationResult
		if result.Status != "" {
			resultPtr = &result
		}
		recordCleanInvocationCompletion(cleanInvocationLogger(operation.logger), operation.cfg, cleanInvocationCompletionLogInput{
			Duration: duration,
			Result:   resultPtr,
			Err:      err,
		})
	}
	return err
}

func normalizeRunInvocationMode(cfg RunConfig) RunConfig {
	if !cfg.CleanInvocation {
		return cfg
	}
	if cfg.JSON {
		cfg.JSONOutput = true
	}
	cfg.SuppressDashboardRendering = true
	cfg.StartupOutput = nil
	if !cfg.WithSite {
		cfg.OpenDashboard = false
	}
	return cfg
}

func prepareRunConfig(
	cfg RunConfig,
	prepareWorkTarget work.SingleWorkTargetPreparation,
) (RunConfig, *factoryapi.InvocationRequest, bool, resolvedRunRecordPath, error) {
	if cfg.Bootstrap {
		if err := bootstrapFactoryWithInitializer(cfg.Dir, cfg.FactoryScaffoldInitializer, cfg.ResolveCurrentFactoryDir, cfg.DirectoryCreator); err != nil {
			return RunConfig{}, nil, false, resolvedRunRecordPath{}, err
		}
	}
	cfg, err := prepareCanonicalSessionIDForRun(cfg)
	if err != nil {
		return RunConfig{}, nil, false, resolvedRunRecordPath{}, err
	}
	recordPath, err := resolveRecordPathForRun(cfg)
	if err != nil {
		return RunConfig{}, nil, false, resolvedRunRecordPath{}, err
	}
	cfg.RecordPath = recordPath.servicePath

	invocationRequest, invocationMode, err := resolveFactoryInvocationRequestForRun(cfg, prepareWorkTarget)
	if err != nil {
		return RunConfig{}, nil, false, resolvedRunRecordPath{}, err
	}
	if err := validateInvocationOutputMode(cfg, invocationMode); err != nil {
		return RunConfig{}, nil, false, resolvedRunRecordPath{}, err
	}
	return cfg, invocationRequest, invocationMode, recordPath, nil
}

// classifyRunInputFailure keeps a submitted replay/resume path actionable
// when runtime opening fails before the command can produce a richer coded
// diagnostic. The underlying error remains available to callers, but the
// default CLI renderer receives only the safe flag/path context.
func classifyRunInputFailure(cfg RunConfig, err error) error {
	if err == nil || clidiag.HasCodedDiagnostic(err) || errors.Is(err, context.Canceled) {
		return err
	}
	if path := strings.TrimSpace(cfg.ReplayPath); path != "" {
		return clidiag.NewLocalInputFailure("--replay", path, err)
	}
	if path := strings.TrimSpace(cfg.ResumePath); path != "" {
		return clidiag.NewLocalInputFailure("--resume", path, err)
	}
	return err
}

func loadSelectedMockWorkersConfigWithDiagnostics(
	cfg RunConfig,
	load workers.MockWorkersConfigLoader,
	loadWithDiagnostics workers.MockWorkersConfigDiagnosticsLoader,
) (*workers.MockWorkersConfig, []string, error) {
	if !cfg.MockWorkersEnabled {
		return nil, nil, nil
	}
	if loadWithDiagnostics != nil {
		config, diagnostics, err := loadWithDiagnostics(cfg.MockWorkersConfigPath)
		return config, diagnostics.Paths(), err
	}
	if load == nil {
		return nil, nil, fmt.Errorf("load mock workers config: Workers config loader is required")
	}
	config, err := load(cfg.MockWorkersConfigPath)
	return config, nil, err
}

func resolveRecordPathForRun(cfg RunConfig) (resolvedRunRecordPath, error) {
	if cfg.RecordingsCLI == nil {
		return resolvedRunRecordPath{}, fmt.Errorf("Recordings CLI adapter is required")
	}
	resolved, err := cfg.RecordingsCLI.ResolveRecordPath(recordingscli.InvocationRequest{
		RecordPath:              cfg.RecordPath,
		ReplayPath:              cfg.ReplayPath,
		ResumePath:              cfg.ResumePath,
		DisableDefaultRecording: cfg.DisableDefaultRecording,
		HomeDir:                 cfg.HomeDir,
		RecordingTargetPlanner:  cfg.RecordingTargetPlanner,
		CanonicalSessionID:      cfg.CanonicalSessionID,
		ReportedSessionID:       defaultFactorySessionID,
	})
	if err != nil {
		return resolvedRunRecordPath{}, err
	}
	return resolvedRunRecordPath{
		servicePath:   resolved.ServicePath,
		reportedPath:  resolved.ReportedPath,
		autoGenerated: resolved.AutoGenerated,
	}, nil
}

func runtimeModeForRun(cfg RunConfig) interfaces.RuntimeMode {
	if cfg.Continuously {
		return interfaces.RuntimeModeService
	}
	return interfaces.RuntimeModeBatch
}

func runFactoryServiceAndEmitResult(
	ctx context.Context,
	cfg RunConfig,
	factorySvc factoryServiceRunner,
	recordPath resolvedRunRecordPath,
	batchProvider batchReportProvider,
) error {
	var snapshot state.CleanInvocationSnapshot
	var snapshotReady bool
	var err error
	if shouldReportBatchResult(cfg) {
		err = runFactoryService(ctx, factorySvc, batchProvider, &snapshot, &snapshotReady)
	} else {
		err = factorySvc.Run(ctx)
	}
	logRunServiceOutcome(ctx, cfg, err)
	if err == nil {
		reportRecordingPathOnShutdown(cfg.StartupOutput, recordPath, cfg.RecordingsCLI)
	}
	if err != nil {
		return err
	}
	if !shouldReportBatchResult(cfg) {
		return nil
	}
	if batchProvider == nil {
		return fmt.Errorf("report batch result: clean invocation snapshot provider is required")
	}
	if !snapshotReady {
		var snapshotErr error
		snapshot, snapshotErr = batchProvider.CleanInvocationSnapshot(ctx)
		if snapshotErr != nil {
			return fmt.Errorf("read batch result: %w", snapshotErr)
		}
	}
	return reportBatchResult(cfg, snapshot)
}

func shouldReportBatchResult(cfg RunConfig) bool {
	return strings.TrimSpace(cfg.WorkFile) != "" && !cfg.CleanInvocation && !cfg.Continuously
}

func runFactoryService(
	ctx context.Context,
	factorySvc factoryServiceRunner,
	batchProvider batchReportProvider,
	snapshot *state.CleanInvocationSnapshot,
	snapshotReady *bool,
) error {
	if batchProvider == nil {
		return factorySvc.Run(ctx)
	}
	managed, supportsCompletion := factorySvc.(initializer.CompletionRuntimeRunner)
	if !supportsCompletion {
		return factorySvc.Run(ctx)
	}
	return managed.RunWithCompletion(ctx, func(completionCtx context.Context) error {
		if waiter, ok := batchProvider.(batchCompletionWaiter); ok {
			waitResult := waiter.ControlWaitToComplete(state.WaitToCompleteRequest{})
			if waitResult.Done != nil {
				select {
				case <-waitResult.Done:
				case <-completionCtx.Done():
					return completionCtx.Err()
				}
			}
		}
		captured, err := batchProvider.CleanInvocationSnapshot(completionCtx)
		if err != nil {
			return fmt.Errorf("read batch result: %w", err)
		}
		if snapshot != nil {
			*snapshot = captured
		}
		if snapshotReady != nil {
			*snapshotReady = true
		}
		return nil
	})
}

// CountTokenStates counts tokens by their state category based on place ID conventions.
// Place IDs follow the pattern '{work_type_id}:{state_value}'.
// Terminal states contain "completed", failed states contain "failed".
func CountTokenStates(snap *state.PetriMarkingSnapshot) (wip, completed, failed int) {
	return factoryruntimecli.CountTokenStates(snap)
}

func isTerminalState(state string) bool {
	return state == completedPlaceIDSuffix
}

func isFailedState(state string) bool {
	return state == failedPlaceIDSuffix
}

// FormatDuration formats a duration as "Xm" or "Xh Ym".
func FormatDuration(d time.Duration) string {
	return factoryruntimecli.FormatDuration(d)
}
