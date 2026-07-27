package invocation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeopening"
	"github.com/portpowered/infinite-you/pkg/services/models"
)

type operation struct {
	openRuntime       *runtimeopening.Factory
	effects           runtimeopening.ExternalEffects
	workingDirectory  platformfilesystem.WorkingDirectory
	resolveCurrentDir factorydefinitions.CurrentFactoryDirectoryResolver
	artifactExporter  models.InvocationArtifactExporter
	modelTimeout      factorysessions.ModelInvocationTimeout
	artifactRoots     factoryruntime.RuntimeArtifactRootResolver
	generateSessionID factorysessions.SessionIDGenerator
}

// NewOperation binds the stable process graph used by all one-shot
// invocations. Each call opens dynamic Factory Session state without rebuilding
// or selecting application dependencies.
func NewOperation(
	openRuntime *runtimeopening.Factory,
	effects runtimeopening.ExternalEffects,
	workingDirectory platformfilesystem.WorkingDirectory,
	resolveCurrentDir factorydefinitions.CurrentFactoryDirectoryResolver,
	artifactExporter models.InvocationArtifactExporter,
	modelTimeout factorysessions.ModelInvocationTimeout,
	artifactRoots factoryruntime.RuntimeArtifactRootResolver,
	generateSessionID factorysessions.SessionIDGenerator,
) (roles.InvocationOperation, error) {
	if openRuntime == nil {
		return nil, errors.New("invocation runtime opening factory is required")
	}
	if workingDirectory == nil {
		return nil, errors.New("invocation working directory is required")
	}
	if resolveCurrentDir == nil {
		return nil, errors.New("current Factory directory resolver is required")
	}
	if artifactExporter == nil {
		return nil, errors.New("model invocation artifact exporter is required")
	}
	if modelTimeout <= 0 {
		return nil, errors.New("model invocation timeout is required")
	}
	if artifactRoots == nil {
		return nil, errors.New("runtime artifact root resolver is required")
	}
	if generateSessionID == nil {
		return nil, errors.New("Factory Session ID generator is required")
	}
	return &operation{
		openRuntime:       openRuntime,
		effects:           effects,
		workingDirectory:  workingDirectory,
		resolveCurrentDir: resolveCurrentDir,
		artifactExporter:  artifactExporter,
		modelTimeout:      modelTimeout,
		artifactRoots:     artifactRoots,
		generateSessionID: generateSessionID,
	}, nil
}

func (o *operation) InvokeModel(
	ctx context.Context,
	target roles.InvocationTarget,
	modelName string,
	request models.Request,
) (result models.Result, resultErr error) {
	invokeCtx, cancel := modelInvocationContext(ctx, o.modelTimeout)
	defer cancel()
	opened, lifecycle, err := o.open(invokeCtx, target)
	if err != nil {
		return models.Result{}, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, lifecycle.close(ctx, opened))
	}()
	result, resultErr = opened.Workers.InvokeModel(lifecycle.runContext, modelName, request)
	return result, resultErr
}

func modelInvocationContext(
	ctx context.Context,
	timeout factorysessions.ModelInvocationTimeout,
) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, time.Duration(timeout))
}

func (o *operation) ResolveModelInvocationFactoryDir(explicit string) (string, error) {
	if root := strings.TrimSpace(explicit); root != "" {
		return root, nil
	}
	cwd, err := o.workingDirectory.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve models invoke factory root: %w", err)
	}
	return o.resolveCurrentDir(filepath.Join(cwd, factorydefinitions.FactoryDir))
}

func (o *operation) ExportModelInvocationArtifact(sourcePath, destinationPath string) error {
	return o.artifactExporter.ExportInvocationArtifact(sourcePath, destinationPath)
}

func (o *operation) InvokeFactory(
	ctx context.Context,
	target roles.InvocationTarget,
	request factorysessions.InvocationRequest,
	consume factorysessions.FactoryEventConsumer,
) (outcome roles.FactoryInvocationOutcome, resultErr error) {
	opened, lifecycle, err := o.open(ctx, target)
	if err != nil {
		return outcome, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, lifecycle.close(ctx, opened))
	}()

	projection, projectionErr := opened.Sessions.GetFactorySession(
		lifecycle.runContext, factorysessions.DefaultSessionID,
	)
	if projectionErr == nil && factorydefinitions.IsJavaScriptOrchestratorFactory(projection.Context.FactoryCfg) {
		delivery := newInvocationFactoryEventDelivery(consume)
		result, err := invokeJavaScriptFactory(
			lifecycle.runContext, opened, projection.Context, target, request, o.generateSessionID,
			delivery.present,
		)
		outcome.Result = result
		if err == nil && consume != nil {
			events, readErr := readInvocationFactoryEvents(lifecycle.runContext, opened.Sessions, result)
			if readErr != nil {
				return outcome, readErr
			}
			delivery.present(events)
		}
		return outcome, err
	}

	liveEvents, err := startLiveInvocationFactoryEvents(lifecycle.runContext, opened.Sessions, consume)
	if err != nil {
		return outcome, err
	}
	outcome.Result, resultErr = opened.Invoker.InvokeFactorySession(
		lifecycle.runContext, factorysessions.DefaultSessionID, request,
	)
	if liveEvents != nil {
		resultErr = errors.Join(resultErr, liveEvents.finish(lifecycle.runContext, opened.Sessions, outcome.Result))
	}
	return outcome, resultErr
}

type liveInvocationFactoryEvents struct {
	consume factorysessions.FactoryEventConsumer
	cancel  context.CancelFunc
	done    chan struct{}
	seen    map[string]struct{}
}

type invocationFactoryEventDelivery struct {
	mu      sync.Mutex
	consume factorysessions.FactoryEventConsumer
	seen    map[string]struct{}
}

func newInvocationFactoryEventDelivery(
	consume factorysessions.FactoryEventConsumer,
) *invocationFactoryEventDelivery {
	return &invocationFactoryEventDelivery{consume: consume, seen: make(map[string]struct{})}
}

func (delivery *invocationFactoryEventDelivery) present(events []factorydefinitions.FactoryEvent) {
	if delivery == nil || delivery.consume == nil {
		return
	}
	delivery.mu.Lock()
	defer delivery.mu.Unlock()
	unseen := make([]factorydefinitions.FactoryEvent, 0, len(events))
	for _, event := range events {
		if _, ok := delivery.seen[event.Id]; ok {
			continue
		}
		delivery.seen[event.Id] = struct{}{}
		unseen = append(unseen, event.Clone())
	}
	if len(unseen) > 0 {
		delivery.consume(unseen)
	}
}

func startLiveInvocationFactoryEvents(
	ctx context.Context,
	reader invocationFactoryEventReader,
	consume factorysessions.FactoryEventConsumer,
) (*liveInvocationFactoryEvents, error) {
	if consume == nil {
		return nil, nil
	}
	streamCtx, cancel := context.WithCancel(ctx)
	stream, err := reader.SubscribeFactoryEventsForSession(
		streamCtx, factorysessions.DefaultSessionID, nil,
	)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("subscribe invocation Factory Events: %w", err)
	}
	if stream == nil || stream.Events == nil {
		cancel()
		return nil, errors.New("subscribe invocation Factory Events: stream is unavailable")
	}
	live := &liveInvocationFactoryEvents{
		consume: consume, cancel: cancel, done: make(chan struct{}), seen: make(map[string]struct{}),
	}
	live.presentUnseen(stream.History)
	go func() {
		defer close(live.done)
		for event := range stream.Events {
			live.presentUnseen([]factorydefinitions.FactoryEvent{event})
		}
	}()
	return live, nil
}

func (live *liveInvocationFactoryEvents) finish(
	ctx context.Context,
	reader invocationFactoryEventReader,
	result factorydefinitions.FactoryInvocationResult,
) error {
	live.cancel()
	<-live.done
	events, err := readInvocationFactoryEvents(ctx, reader, result)
	if err != nil {
		return err
	}
	live.presentUnseen(events)
	return nil
}

func (live *liveInvocationFactoryEvents) presentUnseen(events []factorydefinitions.FactoryEvent) {
	unseen := make([]factorydefinitions.FactoryEvent, 0, len(events))
	for _, event := range events {
		if _, ok := live.seen[event.Id]; ok {
			continue
		}
		live.seen[event.Id] = struct{}{}
		unseen = append(unseen, event.Clone())
	}
	if len(unseen) > 0 {
		live.consume(unseen)
	}
}

type invocationFactoryEventReader interface {
	SubscribeFactoryEventsForSession(context.Context, string, *factorydefinitions.FactoryEventReconnectCursor) (*factorydefinitions.FactoryEventStream, error)
	ReadDurableFactorySessionEventStream(context.Context, string, factorysessions.EventReconnectRequest) (*factorydefinitions.FactoryEventStream, error)
}

func readInvocationFactoryEvents(
	ctx context.Context,
	reader invocationFactoryEventReader,
	result factorydefinitions.FactoryInvocationResult,
) ([]factorydefinitions.FactoryEvent, error) {
	if reader == nil {
		return nil, errors.New("Factory Session event reader is required")
	}
	sessionID := strings.TrimSpace(result.SessionID)
	var (
		stream *factorydefinitions.FactoryEventStream
		err    error
	)
	if sessionID != "" && sessionID != factorysessions.DefaultSessionID {
		stream, err = reader.ReadDurableFactorySessionEventStream(
			ctx, sessionID, factorysessions.EventReconnectRequest{},
		)
	} else {
		stream, err = reader.SubscribeFactoryEventsForSession(
			ctx, factorysessions.DefaultSessionID, nil,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("read invocation Factory Events: %w", err)
	}
	if stream == nil {
		return nil, errors.New("read invocation Factory Events: stream is unavailable")
	}
	events := make([]factorydefinitions.FactoryEvent, len(stream.History))
	for i := range stream.History {
		events[i] = stream.History[i].Clone()
	}
	return events, nil
}

func invokeJavaScriptFactory(
	ctx context.Context,
	opened roles.OpenedInvocationRuntime,
	projection factorysessions.ProjectionContext,
	target roles.InvocationTarget,
	request factorysessions.InvocationRequest,
	generateSessionID factorysessions.SessionIDGenerator,
	consume factorysessions.FactoryEventConsumer,
) (factorydefinitions.FactoryInvocationResult, error) {
	resolver := opened.InputResolver
	if resolver == nil {
		return factorydefinitions.FactoryInvocationResult{}, errors.New("Factory Session invocation input resolver is required")
	}
	startRequest, err := javaScriptStartRequest(projection, target, request, resolver, generateSessionID)
	if err != nil {
		return factorydefinitions.FactoryInvocationResult{}, err
	}
	startRequest.EventConsumer = factorysessions.ExecutionFactoryEventConsumer(consume)
	started, err := opened.Execution.StartSync(ctx, startRequest)
	if err != nil {
		return factorydefinitions.FactoryInvocationResult{}, err
	}
	result, err := opened.Execution.GetResult(ctx, started.SessionID, factorysessions.ResultRequest{
		Mode: factorysessions.ResultModeFinal,
	})
	if err != nil {
		return factorydefinitions.FactoryInvocationResult{}, err
	}
	var sessionFailure *factorysessions.FailureSummary
	if result.Failure == nil && !javaScriptInvocationSucceeded(result) {
		session, sessionErr := opened.Execution.GetSession(ctx, started.SessionID)
		if sessionErr == nil {
			sessionFailure = session.Failure
		}
	}
	return javaScriptInvocationResult(startRequest.RequestID, result, sessionFailure), nil
}

func javaScriptInvocationSucceeded(result factorysessions.ResultReadResult) bool {
	return result.SessionStatus == factorysessions.LifecycleStatusSucceeded &&
		result.ResultStatus == factorysessions.ResultStatusFinal
}

func javaScriptStartRequest(
	projection factorysessions.ProjectionContext,
	target roles.InvocationTarget,
	request factorysessions.InvocationRequest,
	resolver roles.InvocationInputResolver,
	generateSessionID factorysessions.SessionIDGenerator,
) (factorysessions.StartRequest, error) {
	if projection.FactoryCfg == nil || projection.FactoryCfg.Orchestrator == nil || projection.FactoryCfg.Orchestrator.JavaScript == nil {
		return factorysessions.StartRequest{}, errors.New("JavaScript Factory orchestrator configuration is required")
	}
	js := projection.FactoryCfg.Orchestrator.JavaScript
	source := factorysessions.Source{}
	if js.InlineSource != nil {
		source.Kind = factoryruntime.WorkflowSourceKindInlineWorkflow
		source.InlineWorkflow = &factorysessions.InlineWorkflowSource{
			Dialect: js.Dialect, InlineSource: js.InlineSource.Inline, Entrypoint: js.Entrypoint,
			Metadata: cloneStringMap(js.Metadata), Agents: cloneJavaScriptAgents(js.Agents),
			ArgsSchema:    append(json.RawMessage(nil), js.ArgsSchema...),
			DefaultPolicy: append(json.RawMessage(nil), js.DefaultPolicy...),
		}
	} else {
		source.Kind = factoryruntime.WorkflowSourceKindWorkflowFile
		source.WorkflowFile = strings.TrimSpace(js.SourceRef)
		if source.WorkflowFile == "" {
			return factorysessions.StartRequest{}, errors.New("JavaScript Factory workflow sourceRef is required")
		}
		if !filepath.IsAbs(source.WorkflowFile) {
			factoryDir := target.FactoryDir
			if projection.Session != nil && strings.TrimSpace(projection.Session.FactoryDir) != "" {
				factoryDir = projection.Session.FactoryDir
			}
			source.WorkflowFile = filepath.Join(factoryDir, source.WorkflowFile)
		}
		if len(js.DefaultPolicy) > 0 || len(js.ArgsSchema) > 0 || len(js.Agents) > 0 {
			source.InlineWorkflow = &factorysessions.InlineWorkflowSource{
				Agents:        cloneJavaScriptAgents(js.Agents),
				ArgsSchema:    append(json.RawMessage(nil), js.ArgsSchema...),
				DefaultPolicy: append(json.RawMessage(nil), js.DefaultPolicy...),
			}
		}
	}
	args, err := javaScriptInvocationArgs(projection.FactoryCfg, request, js.ArgsSchema, resolver)
	if err != nil {
		return factorysessions.StartRequest{}, err
	}
	requestID := ""
	if request.RequestID != nil {
		requestID = strings.TrimSpace(*request.RequestID)
	}
	if requestID == "" {
		requestID = "run-" + strings.TrimSpace(generateSessionID())
		if requestID == "run-" {
			return factorysessions.StartRequest{}, errors.New("Factory Session ID generator returned an empty identity")
		}
	}
	childMode := factorysessions.ChildExecutorModeLive
	if target.MockWorkersConfig != nil {
		childMode = factorysessions.ChildExecutorModeFake
	}
	return factorysessions.StartRequest{
		RequestID:       requestID,
		Source:          source,
		Args:            args,
		RequestedPolicy: factoryDefaultPolicyMap(js.DefaultPolicy),
		Runtime:         &factorysessions.RuntimeOptions{ChildExecutorMode: childMode},
		Wait:            &factorysessions.WaitOptions{TimeoutMillis: request.TimeoutMillis},
	}, nil
}

func factoryDefaultPolicyMap(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var policy map[string]any
	if err := json.Unmarshal(raw, &policy); err != nil || len(policy) == 0 {
		return nil
	}
	return policy
}

func javaScriptInvocationArgs(
	cfg *factorydefinitions.FactoryConfig,
	request factorysessions.InvocationRequest,
	argsSchema json.RawMessage,
	resolver roles.InvocationInputResolver,
) (map[string]any, error) {
	if resolver == nil {
		return nil, errors.New("Factory Session invocation input resolver is required")
	}
	resolved, err := resolver.ResolveInvocationInput(cfg, request)
	if err != nil {
		return nil, err
	}
	if resolved.NormalizedArguments == nil {
		return map[string]any{}, nil
	}
	types := javaScriptArgumentTypes(argsSchema)
	args := make(map[string]any, len(resolved.NormalizedArguments.Arguments))
	for name, argument := range resolved.NormalizedArguments.Arguments {
		values := make([]any, len(argument.Values))
		for i, value := range argument.Values {
			values[i] = coerceJavaScriptArgument(value, types[name])
		}
		if len(values) == 1 {
			args[name] = values[0]
		} else {
			args[name] = values
		}
	}
	return args, nil
}

func javaScriptArgumentTypes(raw json.RawMessage) map[string]string {
	var schema struct {
		Properties map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &schema) != nil {
		return nil
	}
	types := make(map[string]string, len(schema.Properties))
	for name, property := range schema.Properties {
		types[name] = strings.ToLower(strings.TrimSpace(property.Type))
	}
	return types
}

func coerceJavaScriptArgument(value, valueType string) any {
	switch valueType {
	case "integer":
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
			return parsed
		}
	case "number":
		if parsed, err := strconv.ParseFloat(value, 64); err == nil {
			return parsed
		}
	case "boolean":
		if parsed, err := strconv.ParseBool(value); err == nil {
			return parsed
		}
	}
	return value
}

func javaScriptInvocationResult(
	requestID string,
	result factorysessions.ResultReadResult,
	sessionFailure *factorysessions.FailureSummary,
) factorydefinitions.FactoryInvocationResult {
	out := factorydefinitions.FactoryInvocationResult{
		RequestID: requestID,
		SessionID: result.SessionID,
		Status:    factorydefinitions.InvocationTerminalStatusFailed,
		ErrorCode: string(factorydefinitions.InvocationErrorCodeRuntimeFailure),
	}
	if javaScriptInvocationSucceeded(result) {
		out.Status = factorydefinitions.InvocationTerminalStatusCompleted
		out.ErrorCode = ""
		if len(result.PrimaryResult) > 0 {
			if err := json.Unmarshal(result.PrimaryResult, &out.PrimaryResult); err != nil {
				out.Status = factorydefinitions.InvocationTerminalStatusFailed
				out.ErrorCode = string(factorydefinitions.InvocationErrorCodeRuntimeFailure)
				out.Message = fmt.Sprintf("decode JavaScript Factory result: %v", err)
			}
		}
		return out
	}
	failure := result.Failure
	if failure == nil {
		failure = sessionFailure
	}
	if failure != nil {
		out.Message = strings.TrimSpace(failure.Message)
		if out.Message == "" {
			out.Message = strings.TrimSpace(failure.Reason)
		}
	}
	if out.Message == "" && result.Availability != nil {
		out.Message = strings.TrimSpace(result.Availability.Message)
	}
	if out.Message == "" {
		out.Message = "JavaScript Factory invocation did not produce a final result"
	}
	return out
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func cloneJavaScriptAgents(values map[string]factorydefinitions.FactoryOrchestratorJavaScriptAgent) map[string]factorydefinitions.FactoryOrchestratorJavaScriptAgent {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]factorydefinitions.FactoryOrchestratorJavaScriptAgent, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

type lifecycle struct {
	runContext context.Context
	cancel     context.CancelFunc
	stopWorker factorysessions.RuntimeStop
}

func (o *operation) open(
	ctx context.Context,
	target roles.InvocationTarget,
) (roles.OpenedInvocationRuntime, *lifecycle, error) {
	if o == nil || o.openRuntime == nil {
		return roles.OpenedInvocationRuntime{}, nil, errors.New("invocation operation is required")
	}
	config := o.runtimeConfig(target)
	effects := o.effects
	if target.MetricsRecorder != nil {
		effects.InvocationMetricsRecorder = target.MetricsRecorder
	}
	opened, err := o.openRuntime.OpenInvocationRuntime(ctx, &config, effects, target.Logger)
	if err != nil {
		return roles.OpenedInvocationRuntime{}, nil, fmt.Errorf("open invocation runtime: %w", err)
	}
	runContext, cancel := context.WithCancel(ctx)
	active := &lifecycle{runContext: runContext, cancel: cancel}
	err = opened.Lifecycle.StartLifecycle(ctx, runContext)
	if err == nil {
		active.stopWorker, err = opened.Lifecycle.StartWorkerLifecycle(ctx)
	}
	if err == nil {
		err = opened.Lifecycle.CompleteStartup(ctx)
	}
	if err != nil {
		return roles.OpenedInvocationRuntime{}, nil, errors.Join(err, active.close(ctx, opened))
	}
	return opened, active, nil
}

func (o *operation) runtimeConfig(target roles.InvocationTarget) factorysessions.RuntimeOpeningRequest {
	config := factorysessions.RuntimeOpeningRequest{}
	config.FactoryDefinition.Directory = target.FactoryDir
	config.FactoryDefinition.SourcePath = target.FactorySourcePath
	config.FactoryDefinition.ExecutionBaseDir = target.ExecutionBaseDir
	config.OperatorDefaults = target.OperatorDefaults
	config.FactoryRuntime.Mode = factorydefinitions.RuntimeModeService
	config.FactoryRuntime.Verbose = target.Verbose
	config.FactoryRuntime.LogDirectory = target.RuntimeLogDir
	roots := o.artifactRoots(target.HomeDir)
	if config.FactoryRuntime.LogDirectory == "" {
		config.FactoryRuntime.LogDirectory = roots.Logs
	}
	config.FactoryRuntime.LogConfig = target.RuntimeLogConfig
	config.FactoryRuntime.MetricsDirectory = target.RuntimeMetricsDir
	if config.FactoryRuntime.MetricsDirectory == "" {
		config.FactoryRuntime.MetricsDirectory = roots.Metrics
	}
	config.FactoryRuntime.MetricsConfig = target.RuntimeMetricsConfig
	config.FactorySession.SystemConfigHome = target.HomeDir
	config.FactorySession.WorkFile = ""
	config.FactorySession.Host.Port = 0
	config.FactorySession.Host.RuntimeMode = factorydefinitions.RuntimeModeService
	config.Workers.RunnerID = target.RunnerID
	config.Workers.MockWorkers = target.MockWorkersConfig
	config.Workers.InvocationSkipPermissionsOverride = target.SkipPermissionsOverride
	config.Workers.SkipBuiltInPrerequisiteValidation = target.SkipRunnerPrerequisiteValidation
	config.Recordings.RecordPath = target.RecordPath
	config.Recordings.ReplayPath = target.ReplayPath
	config.Recordings.WorkflowID = target.WorkflowID
	config.Models.CacheDirectory = target.ModelCacheDir
	return config
}

func (l *lifecycle) close(ctx context.Context, opened roles.OpenedInvocationRuntime) error {
	if l == nil {
		return closeArtifacts(opened)
	}
	cleanupContext := context.WithoutCancel(ctx)
	var result error
	if err := opened.Sessions.CloseFactorySession(cleanupContext, factorysessions.DefaultSessionID); err != nil {
		if !errors.Is(err, factorysessions.ErrSessionNotFound) {
			result = errors.Join(result, err)
		}
	}
	if l.stopWorker != nil {
		result = errors.Join(result, l.stopWorker(cleanupContext))
	}
	result = errors.Join(result, opened.Lifecycle.StopLifecycle(cleanupContext))
	l.cancel()
	if err := opened.Lifecycle.WaitForRuntime(cleanupContext); err != nil && !errors.Is(err, context.Canceled) {
		result = errors.Join(result, err)
	}
	result = errors.Join(result, closeArtifacts(opened))
	return result
}

func closeArtifacts(opened roles.OpenedInvocationRuntime) error {
	if opened.CloseArtifacts == nil {
		return nil
	}
	return opened.CloseArtifacts()
}

var _ roles.InvocationOperation = (*operation)(nil)
