// Package run implements the agent-factory run command behavior.
package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"

	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/portpowered/infinite-you/pkg/transports/cli/runconfig"
	"github.com/portpowered/infinite-you/pkg/transports/cli/terminalpolicy"
	"github.com/portpowered/infinite-you/pkg/transports/cli/timedisplay"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"

	"github.com/portpowered/infinite-you/pkg/initializer"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	state "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimecli "github.com/portpowered/infinite-you/pkg/services/factory_runtime/transports/cli"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionscli "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	recordingscli "github.com/portpowered/infinite-you/pkg/services/recordings/transports/cli"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"go.uber.org/zap"
)

type RunConfig = runconfig.Config

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
	cfg                  RunConfig
	logger               *zap.Logger
	runner               RuntimeRunner
	invocationRequest    *factoryapi.InvocationRequest
	invocationTarget     factorysessions.InvocationTarget
	invocation           InvocationOperation
	presentation         factoryvisualization.ResponsePresentation
	invocationMode       bool
	recordPath           resolvedRunRecordPath
	hostedInvocation     HostedInvocationOperation
	historicalReplay     *factorysessions.HistoricalReplayInspection
	openingPresentations factorysessions.OpeningPresentationOwner
	visualizations       factoryvisualization.RuntimeSinkOwner
	visualizationSinkID  factoryvisualization.RuntimeSinkID
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
		loadMockWorkers, buildRuntimeRequest, presentationOwner, nil)
}

// OpenWithVisualizationOwner is the canonical CLI composition entrypoint.
// Visualization sink state is retained by its own owner and represented in
// the application-opening request by an opaque value ID.
func OpenWithVisualizationOwner(
	ctx context.Context,
	cfg RunConfig,
	buildRunner RuntimeRunnerBuilder,
	invocation InvocationOperation,
	presentation factoryvisualization.ResponsePresentation,
	prepareWorkTarget work.SingleWorkTargetPreparation,
	loadMockWorkers workers.MockWorkersConfigLoader,
	buildRuntimeRequest RuntimeOpeningRequestFactory,
	presentations factorysessions.OpeningPresentationOwner,
	visualizations factoryvisualization.RuntimeSinkOwner,
) (*Operation, error) {
	return open(ctx, cfg, buildRunner, invocation, presentation, prepareWorkTarget,
		loadMockWorkers, buildRuntimeRequest, presentations, visualizations)
}

func open(
	ctx context.Context,
	cfg RunConfig,
	buildRunner RuntimeRunnerBuilder,
	invocation InvocationOperation,
	presentation factoryvisualization.ResponsePresentation,
	prepareWorkTarget work.SingleWorkTargetPreparation,
	loadMockWorkers workers.MockWorkersConfigLoader,
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

	mockWorkersConfig, err := loadSelectedMockWorkersConfig(cfg, loadMockWorkers)
	if err != nil {
		return nil, err
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

	return openHostedRuntime(
		ctx, cfg, logger, invocationRequest, recordPath, invocation, presentation,
		prepareWorkTarget, mockWorkersConfig, invocationMode, requestedPort,
		buildRunner, buildRuntimeRequest, presentationOwner, visualizations,
	)
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
		if err := operation.runner.Run(ctx); err != nil {
			return err
		}
		return emitHistoricalReplayInspection(operation.cfg.Output, *operation.historicalReplay)
	}
	if operation.invocationMode {
		if operation.runner != nil {
			if runner, ok := operation.runner.(initializer.CompletionRuntimeRunner); ok {
				return runner.RunWithCompletion(ctx, operation.runInvocation)
			}
			return operation.runner.Run(ctx)
		}
		return operation.runInvocation(ctx)
	}

	if operation.cfg.Port <= 0 {
		emitStartupMessages(operation.cfg, runtimeLogDiagnosticsForRunner(operation.runner))
	}

	return runFactoryServiceAndEmitResult(
		ctx,
		operation.cfg,
		operation.runner,
		operation.recordPath,
	)
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

func loadSelectedMockWorkersConfig(
	cfg RunConfig,
	load workers.MockWorkersConfigLoader,
) (*workers.MockWorkersConfig, error) {
	if !cfg.MockWorkersEnabled {
		return nil, nil
	}
	if load == nil {
		return nil, fmt.Errorf("load mock workers config: Workers config loader is required")
	}
	return load(cfg.MockWorkersConfigPath)
}

func resolveRecordPathForRun(cfg RunConfig) (resolvedRunRecordPath, error) {
	if cfg.RecordingsCLI == nil {
		return resolvedRunRecordPath{}, fmt.Errorf("Recordings CLI adapter is required")
	}
	resolved, err := cfg.RecordingsCLI.ResolveRecordPath(recordingscli.InvocationRequest{
		RecordPath:              cfg.RecordPath,
		ReplayPath:              cfg.ReplayPath,
		DisableDefaultRecording: cfg.DisableDefaultRecording,
		HomeDir:                 cfg.HomeDir,
		RecordingTargetPlanner:  cfg.RecordingTargetPlanner,
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
) error {
	err := factorySvc.Run(ctx)
	logRunServiceOutcome(ctx, cfg, err)
	if err == nil {
		reportRecordingPathOnShutdown(cfg.StartupOutput, recordPath, cfg.RecordingsCLI)
	}
	if err != nil {
		return err
	}
	return nil
}

func emitVerboseStartupDiagnostics(cfg RunConfig, recordPath resolvedRunRecordPath, requestedPort int) {
	resolvedFactoryDir := resolveFactoryDirForDiagnostics(cfg.Dir, cfg.ResolveCurrentFactoryDir)
	diagnosticsEnabled := terminalpolicy.DiagnosticsEnabled(cfg.TerminalPolicy, cfg.Verbose)
	clidiag.Printf(
		cfg.Diagnostics,
		diagnosticsEnabled,
		"run startup factoryDir=%q configuredDir=%q runtimeMode=%s workflow=%q mockWorkers=%t mockWorkersConfigPath=%q recording=%s runtimeLogDir=%q runtimeLogRoll=%s runtimeMetricsDir=%q runtimeMetricsRoll=%s dashboardPort=%d requestedDashboardPort=%d autoPort=%s",
		resolvedFactoryDir,
		cfg.Dir,
		runtimeModeForRun(cfg),
		workflowLabel(cfg.Workflow),
		cfg.MockWorkersEnabled,
		cfg.MockWorkersConfigPath,
		recordingDiagnostics(cfg.RecordingsCLI, recordPath, cfg.ReplayPath),
		runtimeLogDirLabel(cfg.RuntimeLogDir),
		rollingPolicyDiagnostics(cfg.RuntimeLogConfig.MaxSize, cfg.RuntimeLogConfig.MaxBackups, cfg.RuntimeLogConfig.MaxAge, cfg.RuntimeLogConfig.Compress),
		runtimeMetricsDirLabel(cfg.RuntimeMetricsDir),
		rollingPolicyDiagnostics(cfg.RuntimeMetricsConfig.MaxSize, cfg.RuntimeMetricsConfig.MaxBackups, cfg.RuntimeMetricsConfig.MaxAge, cfg.RuntimeMetricsConfig.Compress),
		cfg.Port,
		requestedPort,
		autoPortDiagnostics(cfg.AutoPort, requestedPort, cfg.Port),
	)
	clidiag.Printf(cfg.Diagnostics, diagnosticsEnabled, "%s", cfg.OperatorDefaults.DiagnosticsLine())
}

func emitNamedFactoryResolutionDiagnostics(cfg RunConfig, logger *zap.Logger) {
	resolution := cfg.NamedFactoryResolution
	if resolution == nil {
		return
	}

	clidiag.Printf(
		cfg.Diagnostics,
		terminalpolicy.DiagnosticsEnabled(cfg.TerminalPolicy, cfg.Verbose),
		"run named-factory resolution name=%q source=%s resolvedFactoryDir=%q projectRoot=%q globalRoot=%q precedence=%s",
		resolution.Name,
		resolution.Source,
		resolution.FactoryDir,
		resolution.ProjectRoot,
		resolution.GlobalRoot,
		resolution.PrecedenceDecision,
	)
	logger.Info(
		"named factory resolved",
		zap.String("named_factory_name", resolution.Name),
		zap.String("named_factory_resolution_source", string(resolution.Source)),
		zap.String("named_factory_dir", resolution.FactoryDir),
		zap.String("named_factory_project_root", resolution.ProjectRoot),
		zap.String("named_factory_global_root", resolution.GlobalRoot),
		zap.String("named_factory_precedence_decision", string(resolution.PrecedenceDecision)),
	)
	if resolution.PrecedenceDecision == interfaces.NamedFactoryPrecedenceDecisionProjectOverGlobal {
		logger.Info(
			"named factory precedence selected",
			zap.String("named_factory_name", resolution.Name),
			zap.String("named_factory_precedence_decision", string(resolution.PrecedenceDecision)),
			zap.String("named_factory_resolution_source", string(resolution.Source)),
		)
	}
}

func resolveFactoryDirForDiagnostics(
	dir string,
	resolve interfaces.CurrentFactoryDirectoryResolver,
) string {
	if resolve == nil {
		return "unresolved"
	}
	resolved, err := resolve(dir)
	if err != nil {
		return "unresolved"
	}
	return resolved
}

func workflowLabel(workflow string) string {
	if strings.TrimSpace(workflow) == "" {
		return "all"
	}
	return workflow
}

func runtimeLogDirLabel(dir string) string {
	if strings.TrimSpace(dir) == "" {
		return "default"
	}
	return dir
}

func runtimeMetricsDirLabel(dir string) string {
	if strings.TrimSpace(dir) == "" {
		return "default"
	}
	return dir
}

func rollingPolicyDiagnostics(maxSize, maxBackups, maxAge int, compress bool) string {
	return fmt.Sprintf("size_mb=%d backups=%d age_days=%d compress=%t", maxSize, maxBackups, maxAge, compress)
}

func recordingDiagnostics(
	adapter recordingscli.Adapter,
	recordPath resolvedRunRecordPath,
	replayPath string,
) string {
	if adapter == nil {
		return "disabled"
	}
	return adapter.RecordingDiagnosticsLabel(recordingscli.ResolvedRecordPath{
		ServicePath:   recordPath.servicePath,
		ReportedPath:  recordPath.reportedPath,
		AutoGenerated: recordPath.autoGenerated,
	}, replayPath)
}

func autoPortDiagnostics(autoPort bool, requestedPort, resolvedPort int) string {
	switch {
	case requestedPort <= 0:
		return "dashboard-disabled"
	case !autoPort:
		return "disabled"
	case requestedPort == resolvedPort:
		return "preferred-available"
	default:
		return "fallback"
	}
}

func bindDashboardHost(cfg RunConfig) string {
	if strings.TrimSpace(cfg.BindHost) != "" {
		return cfg.BindHost
	}
	return "localhost"
}

// DashboardURL returns the embedded browser dashboard URL for the configured
// local factory server host and port.
func DashboardURL(host string, port int) string {
	if port <= 0 {
		return ""
	}
	if strings.TrimSpace(host) == "" {
		host = "localhost"
	}
	authority := net.JoinHostPort(host, strconv.Itoa(port))
	return "http://" + authority + "/dashboard/ui"
}

func emitStartupMessages(
	cfg RunConfig,
	runtimeLog runtimeartifact.Diagnostics,
) bool {
	if cfg.StartupOutput == nil {
		return false
	}

	fmt.Fprintf(cfg.StartupOutput, "Factory initiated: %s\n", cfg.Dir)
	if cfg.Bootstrap {
		fmt.Fprintf(cfg.StartupOutput, "Factory directory ready: %s\n", cfg.Dir)
	}
	if cfg.Continuously {
		fmt.Fprintln(cfg.StartupOutput, "Runtime mode: continuous")
	}
	if strings.TrimSpace(runtimeLog.Path) != "" {
		fmt.Fprintf(cfg.StartupOutput, "Runtime log: %s\n", runtimeLog.Path)
		fmt.Fprintf(cfg.StartupOutput, "Runtime log start (UTC): %s\n", timedisplay.Timestamp(runtimeLog.StartTimeUTC))
	}
	if strings.TrimSpace(runtimeLog.MetricsPath) != "" {
		fmt.Fprintf(cfg.StartupOutput, "Runtime metrics: %s\n", runtimeLog.MetricsPath)
		fmt.Fprintf(cfg.StartupOutput, "Runtime metrics start (UTC): %s\n", timedisplay.Timestamp(runtimeLog.MetricsStartTimeUTC))
	}
	if cfg.Port <= 0 {
		fmt.Fprintln(cfg.StartupOutput, "Dashboard server disabled")
		return false
	}

	url := DashboardURL(bindDashboardHost(cfg), cfg.Port)
	fmt.Fprintf(cfg.StartupOutput, "Dashboard URL: %s\n", url)
	if !cfg.OpenDashboard {
		fmt.Fprintf(cfg.StartupOutput, "Dashboard auto-open disabled; open %s\n", url)
		return false
	}
	return true
}

func shouldOpenDashboard(cfg RunConfig) bool {
	return cfg.OpenDashboard
}

func reportRecordingPathOnShutdown(
	output io.Writer,
	recordPath resolvedRunRecordPath,
	adapter recordingscli.Adapter,
) {
	if adapter == nil {
		return
	}
	adapter.ReportRecordingPathOnShutdown(output, recordingscli.ResolvedRecordPath{
		ServicePath:   recordPath.servicePath,
		ReportedPath:  recordPath.reportedPath,
		AutoGenerated: recordPath.autoGenerated,
	})
}

func openDashboardAtBoundEndpoint(
	ctx context.Context,
	cfg RunConfig,
	openDashboard func(context.Context, string) error,
) {
	url := DashboardURL(bindDashboardHost(cfg), cfg.Port)
	if openDashboard == nil {
		if cfg.StartupOutput != nil {
			fmt.Fprintf(cfg.StartupOutput, "Dashboard auto-open unavailable: browser opener is required\nOpen the dashboard at %s\n", url)
		}
		return
	}
	if err := openDashboard(ctx, url); err != nil {
		if cfg.StartupOutput != nil {
			fmt.Fprintf(cfg.StartupOutput, "Dashboard auto-open unavailable: %v\nOpen the dashboard at %s\n", err, url)
		}
		return
	}
	if cfg.StartupOutput != nil {
		fmt.Fprintf(cfg.StartupOutput, "Opening dashboard: %s\n", url)
	}
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
