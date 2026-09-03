package invocation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessioncontracts "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/contracts"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeopening"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
)

type operation struct {
	openRuntime       runtimeopening.InvocationRuntimeOpening
	logger            *zap.Logger
	modelsRoot        models.Service
	workingDirectory  platformfilesystem.WorkingDirectory
	resolveCurrentDir factorydefinitions.CurrentFactoryDirectoryResolver
	artifactExporter  factorysessioncontracts.InvocationArtifactExporter
	modelTimeout      factorysessions.ModelInvocationTimeout
	artifactRoots     factoryruntime.RuntimeArtifactRootResolver
	generateSessionID factorysessions.SessionIDGenerator
	presentations     factorysessions.OpeningPresentationOwner
}

// NewOperation binds the stable process graph used by all one-shot
// invocations. Each call opens dynamic Factory Session state without rebuilding
// or selecting application dependencies.
func NewOperation(
	openRuntime runtimeopening.InvocationRuntimeOpening,
	modelsRoot models.Service,
	workingDirectory platformfilesystem.WorkingDirectory,
	resolveCurrentDir factorydefinitions.CurrentFactoryDirectoryResolver,
	artifactExporter factorysessioncontracts.InvocationArtifactExporter,
	modelTimeout factorysessions.ModelInvocationTimeout,
	artifactRoots factoryruntime.RuntimeArtifactRootResolver,
	generateSessionID factorysessions.SessionIDGenerator,
	logger *zap.Logger,
	presentations factorysessions.OpeningPresentationOwner,
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
	if logger == nil {
		return nil, errors.New("invocation logger is required")
	}
	if presentations == nil {
		return nil, errors.New("invocation presentation owner is required")
	}
	return &operation{
		openRuntime:       openRuntime,
		logger:            logger,
		modelsRoot:        modelsRoot,
		workingDirectory:  workingDirectory,
		resolveCurrentDir: resolveCurrentDir,
		artifactExporter:  artifactExporter,
		modelTimeout:      modelTimeout,
		artifactRoots:     artifactRoots,
		generateSessionID: generateSessionID,
		presentations:     presentations,
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
	modelInvoker := opened.ModelInvoker
	if modelInvoker == nil {
		// Keep lightweight opening fakes and older callers usable while the
		// opened runtime remains the authoritative model-invocation capability.
		modelInvoker = runtimeopening.NewRuntimeModelInvoker(runtimeopening.RuntimeModelInvokerConfig{
			Models: o.modelsRoot, Scope: opened.ModelsScope,
			Sessions: opened.Sessions, Workers: opened.Workers,
			RuntimeID: opened.RuntimeID, GenerationID: opened.GenerationID,
			FactoryDirectory: target.FactoryDir, WorkingDirectory: target.Worktree,
		})
	}
	result, resultErr = modelInvoker.InvokeModel(lifecycle.runContext, modelName, request)
	return result, resultErr
}

func modelInvocationContext(
	ctx context.Context,
	timeout factorysessions.ModelInvocationTimeout,
) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, time.Duration(timeout))
}

func (o *operation) ResolveModelInvocationFactoryDir(explicit string) (string, error) {
	return o.resolveModelInvocationFactoryDir(explicit, "")
}

func (o *operation) ResolveModelInvocationFactoryDirForWorkingDirectory(
	explicit string,
	requestedWorkingDirectory string,
) (string, error) {
	return o.resolveModelInvocationFactoryDir(explicit, requestedWorkingDirectory)
}

func (o *operation) resolveModelInvocationFactoryDir(explicit, requestedWorkingDirectory string) (string, error) {
	if root := strings.TrimSpace(explicit); root != "" {
		return root, nil
	}
	cwd := strings.TrimSpace(requestedWorkingDirectory)
	if cwd == "" {
		var err error
		cwd, err = o.workingDirectory.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve models invoke factory root: %w", err)
		}
	}
	factoryRoot := filepath.Join(cwd, factorydefinitions.FactoryDir)
	resolved, err := o.resolveCurrentDir(factoryRoot)
	if err == nil {
		return resolved, nil
	}
	if errors.Is(err, factorydefinitions.ErrFactoryLayoutNotFound) {
		if legacyResolved, legacyErr := o.resolveCurrentDir(cwd); legacyErr == nil {
			return legacyResolved, nil
		}
	}
	return "", fmt.Errorf("resolve models invoke factory root %s: %w", factoryRoot, err)
}

func (o *operation) ExportModelInvocationArtifact(sourcePath, destinationPath string) error {
	return o.artifactExporter.ExportInvocationArtifact(sourcePath, destinationPath)
}

func (o *operation) InvokeFactory(
	ctx context.Context,
	target roles.InvocationTarget,
	request factorysessions.InvocationRequest,
) (outcome roles.FactoryInvocationOutcome, resultErr error) {
	opened, lifecycle, err := o.open(ctx, target)
	if err != nil {
		return outcome, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, lifecycle.close(ctx, opened))
	}()
	return o.invokeFactoryOnOpenedRuntime(ctx, opened, lifecycle.runContext, target, request)
}

// joinTeardownErrorUnlessResultDetermined merges a post-result error from a
// best-effort trailing Factory Event read into resultErr only when the
// invocation never reached a terminal result. Trailing event delivery and the
// invocation's own event-derived terminal result race each other; a failure
// in the former must not erase an already-determined public outcome, since
// the record a caller observes stays tied to what the invocation itself
// decided. This is deliberately narrower than runtime teardown (lifecycle.close):
// teardown failures (session close, worker stop, lifecycle stop, artifact
// close) are genuine resource-cleanup errors and must always propagate and
// preserve their failing exit semantics, even after a terminal result exists.
func joinTeardownErrorUnlessResultDetermined(
	outcome roles.FactoryInvocationOutcome,
	resultErr error,
	postResultErr error,
	logger *zap.Logger,
) error {
	if postResultErr == nil {
		return resultErr
	}
	if outcome.Result.Status == "" {
		return errors.Join(resultErr, postResultErr)
	}
	if logger != nil {
		logger.Warn(
			"invocation post-result step failed after terminal result was determined",
			zap.Error(postResultErr),
		)
	}
	return resultErr
}

func (o *operation) invokeFactoryOnHostedLiveRuntime(
	ctx context.Context,
	hosted roles.HostedInvocationOperation,
	target roles.InvocationTarget,
	request factorysessions.InvocationRequest,
) (outcome roles.FactoryInvocationOutcome, resultErr error) {
	if hosted == nil {
		return outcome, errors.New("hosted live invocation runtime is incomplete")
	}
	sessionID := invocationTargetSessionID(target)
	if projectionReader, ok := hosted.(interface {
		GetFactorySession(context.Context, string) (factorysessions.SessionProjection, error)
	}); ok {
		projection, projectionErr := projectionReader.GetFactorySession(
			ctx, sessionID,
		)
		if projectionErr != nil && ctx.Err() != nil {
			return outcome, ctx.Err()
		}
		if projectionErr == nil && factorydefinitions.IsJavaScriptOrchestratorFactory(projection.Context.FactoryCfg) {
			return o.invokeFactoryOnEphemeralRuntime(ctx, target, request)
		}
	}
	bridge, err := o.startFactoryEventBridge(ctx, hosted, target)
	if err != nil {
		return outcome, err
	}
	invocationResult, err := hosted.InvokeFactorySession(
		ctx, sessionID, request,
	)
	outcome.Result = factoryInvocationResultFromSessionInvocation(invocationResult)
	resultErr = err
	if bridge != nil {
		resultErr = joinTeardownErrorUnlessResultDetermined(
			outcome, resultErr, bridge.Finish(ctx, hosted, outcome), o.logger,
		)
	}
	return outcome, resultErr
}

func factoryInvocationResultFromSessionInvocation(
	result factorysessions.InvocationResult,
) factorydefinitions.FactoryInvocationResult {
	return factorydefinitions.FactoryInvocationResult{
		RequestID: result.RequestID, TraceID: result.TraceID,
		Status:        factorydefinitions.InvocationTerminalStatus(result.Status),
		PrimaryResult: result.PrimaryResult, ErrorCode: result.ErrorCode,
		Message: result.Message, SessionID: result.SessionID, WorkID: result.WorkID,
		WorkName: result.WorkName, WorkState: result.WorkState,
	}
}

func (o *operation) invokeFactoryOnEphemeralRuntime(
	ctx context.Context,
	target roles.InvocationTarget,
	request factorysessions.InvocationRequest,
) (outcome roles.FactoryInvocationOutcome, resultErr error) {
	opened, lifecycle, err := o.open(ctx, target)
	if err != nil {
		return outcome, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, lifecycle.close(ctx, opened))
	}()
	return o.invokeFactoryOnOpenedRuntime(ctx, opened, lifecycle.runContext, target, request)
}

func (o *operation) invokeFactoryOnOpenedRuntime(
	ctx context.Context,
	opened roles.OpenedInvocationRuntime,
	runContext context.Context,
	target roles.InvocationTarget,
	request factorysessions.InvocationRequest,
) (outcome roles.FactoryInvocationOutcome, resultErr error) {
	sessionID := invocationTargetSessionID(target)
	bridge, err := o.startFactoryEventBridge(runContext, opened.Sessions, target)
	if err != nil {
		return outcome, err
	}
	projection, projectionErr := opened.Sessions.GetFactorySession(
		runContext, sessionID,
	)
	if projectionErr != nil && runContext.Err() != nil {
		return outcome, runContext.Err()
	}
	if projectionErr == nil && factorydefinitions.IsJavaScriptOrchestratorFactory(projection.Context.FactoryCfg) {
		result, err := invokeJavaScriptFactory(
			runContext, opened, projection.Context, target, request, o.generateSessionID,
		)
		outcome.Result = result
		if bridge != nil {
			err = joinTeardownErrorUnlessResultDetermined(
				outcome, err, bridge.Finish(runContext, opened.Sessions, outcome), o.logger,
			)
		}
		return outcome, err
	}

	outcome.Result, resultErr = opened.Invoker.InvokeFactorySession(
		runContext, sessionID, request,
	)
	if bridge != nil {
		resultErr = joinTeardownErrorUnlessResultDetermined(
			outcome, resultErr, bridge.Finish(runContext, opened.Sessions, outcome), o.logger,
		)
	}
	return outcome, resultErr
}

func (o *operation) startFactoryEventBridge(
	ctx context.Context,
	reader roles.FactoryEventReader,
	target roles.InvocationTarget,
) (interface {
	Finish(context.Context, roles.FactoryEventReader, factorysessions.FactoryInvocationOutcome) error
}, error) {
	if target.EventScopeID == "" {
		return nil, nil
	}
	if o == nil || o.presentations == nil {
		return nil, errors.New("invocation presentation owner is required")
	}
	return o.presentations.StartFactoryEventBridge(ctx, reader, target.EventScopeID)
}

// InvokeOpenedJavaScriptFactory invokes one already-opened
// JavaScript-orchestrator Factory runtime through durable workflow execution.
//
// A JavaScript Factory's whole workflow is its program, so it declares no work
// types and cannot be invoked through the Work-submission path, which begins
// by resolving the single work type carrying handlingBehavior DEFAULT. Every
// consumer that invokes an opened runtime therefore has to branch on
// factorydefinitions.IsJavaScriptOrchestratorFactory first, and this is the
// one implementation of the branch's JavaScript side -- exported so the
// on-demand target activation ACP dispatch uses reaches exactly the path this
// package's own one-shot operation uses, rather than growing a second copy
// that can drift from it.
func InvokeOpenedJavaScriptFactory(
	ctx context.Context,
	opened roles.OpenedInvocationRuntime,
	projection factorysessions.ProjectionContext,
	target roles.InvocationTarget,
	request factorysessions.InvocationRequest,
	generateSessionID factorysessions.SessionIDGenerator,
) (factorydefinitions.FactoryInvocationResult, error) {
	return invokeJavaScriptFactory(ctx, opened, projection, target, request, generateSessionID)
}

func invokeJavaScriptFactory(
	ctx context.Context,
	opened roles.OpenedInvocationRuntime,
	projection factorysessions.ProjectionContext,
	target roles.InvocationTarget,
	request factorysessions.InvocationRequest,
	generateSessionID factorysessions.SessionIDGenerator,
) (factorydefinitions.FactoryInvocationResult, error) {
	resolver := opened.InputResolver
	if resolver == nil {
		return factorydefinitions.FactoryInvocationResult{}, errors.New("Factory Session invocation input resolver is required")
	}
	startRequest, err := javaScriptStartRequest(projection, target, request, resolver, generateSessionID)
	if err != nil {
		return factorydefinitions.FactoryInvocationResult{}, err
	}
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
	source, err := javaScriptWorkflowSource(js, projection, target)
	if err != nil {
		return factorysessions.StartRequest{}, err
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

func javaScriptWorkflowSource(
	js *factorydefinitions.FactoryOrchestratorJavaScriptConfig,
	projection factorysessions.ProjectionContext,
	target roles.InvocationTarget,
) (factorysessions.Source, error) {
	source := factorysessions.Source{}
	if js.InlineSource != nil {
		source.Kind = factoryruntime.WorkflowSourceKindInlineWorkflow
		source.InlineWorkflow = &factorysessions.InlineWorkflowSource{
			Dialect: js.Dialect, InlineSource: js.InlineSource.Inline, Entrypoint: js.Entrypoint,
			Metadata: javaScriptFactoryMetadata(js, projection), Agents: cloneJavaScriptAgents(js.Agents),
			ArgsSchema:    append(json.RawMessage(nil), js.ArgsSchema...),
			DefaultPolicy: append(json.RawMessage(nil), js.DefaultPolicy...),
		}
		return source, nil
	}
	source.Kind = factoryruntime.WorkflowSourceKindWorkflowFile
	source.WorkflowFile = strings.TrimSpace(js.SourceRef)
	if source.WorkflowFile == "" {
		return factorysessions.Source{}, errors.New("JavaScript Factory workflow sourceRef is required")
	}
	if !filepath.IsAbs(source.WorkflowFile) {
		factoryDir := target.FactoryDir
		if projection.Session != nil && strings.TrimSpace(projection.Session.FactoryDir) != "" {
			factoryDir = projection.Session.FactoryDir
		}
		source.WorkflowFile = filepath.Join(factoryDir, source.WorkflowFile)
	}
	metadata := javaScriptFactoryMetadata(js, projection)
	if len(js.DefaultPolicy) > 0 || len(js.ArgsSchema) > 0 || len(js.Agents) > 0 || len(metadata) > 0 {
		source.InlineWorkflow = &factorysessions.InlineWorkflowSource{
			Metadata:      metadata,
			Agents:        cloneJavaScriptAgents(js.Agents),
			ArgsSchema:    append(json.RawMessage(nil), js.ArgsSchema...),
			DefaultPolicy: append(json.RawMessage(nil), js.DefaultPolicy...),
		}
	}
	return source, nil
}

func javaScriptFactoryMetadata(
	js *factorydefinitions.FactoryOrchestratorJavaScriptConfig,
	projection factorysessions.ProjectionContext,
) map[string]string {
	metadata := cloneStringMap(js.Metadata)
	if projection.FactoryCfg == nil {
		return metadata
	}
	if factoryName := strings.TrimSpace(projection.FactoryCfg.Name); factoryName != "" {
		if metadata == nil {
			metadata = make(map[string]string)
		}
		metadata["factoryName"] = factoryName
	}
	return metadata
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
	sessionID  string
}

func (o *operation) open(
	ctx context.Context,
	target roles.InvocationTarget,
) (roles.OpenedInvocationRuntime, *lifecycle, error) {
	if o == nil || o.openRuntime == nil {
		return roles.OpenedInvocationRuntime{}, nil, errors.New("invocation operation is required")
	}
	config := o.runtimeConfig(target)
	opened, err := o.openRuntime.OpenInvocationRuntime(ctx, &config)
	if err != nil {
		return roles.OpenedInvocationRuntime{}, nil, fmt.Errorf("open invocation runtime: %w", err)
	}
	runContext, cancel := context.WithCancel(ctx)
	active := &lifecycle{
		runContext: runContext,
		cancel:     cancel,
		sessionID:  invocationTargetSessionID(target),
	}
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
	config.FactoryDefinition.InvocationArguments = work.CloneInvocationArguments(target.InvocationArguments)
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
	config.FactorySession.FactorySessionID = invocationTargetSessionID(target)
	config.FactorySession.CanonicalSessionID = target.CanonicalSessionID
	config.FactorySession.WorkFile = ""
	config.FactorySession.Host.Port = 0
	config.FactorySession.Host.RuntimeMode = factorydefinitions.RuntimeModeService
	config.Workers.RunnerID = target.RunnerID
	config.Workers.Worktree = target.Worktree
	config.Workers.WorkerReasoningEffort = target.WorkerReasoningEffort
	config.Workers.MockWorkers = target.MockWorkersConfig
	config.Workers.InvocationSkipPermissionsOverride = target.SkipPermissionsOverride
	config.Workers.SkipBuiltInPrerequisiteValidation = target.SkipRunnerPrerequisiteValidation
	config.Recordings.RecordPath = target.RecordPath
	config.Recordings.ReplayPath = target.ReplayPath
	config.Recordings.ResumePath = target.ResumePath
	config.Recordings.WorkflowID = target.WorkflowID
	config.ModelCacheDirectory = target.ModelCacheDir
	return config
}

func (l *lifecycle) close(ctx context.Context, opened roles.OpenedInvocationRuntime) error {
	if l == nil {
		return closeArtifacts(opened)
	}
	cleanupContext := context.WithoutCancel(ctx)
	var result error
	var closeSession func(context.Context, string) error
	if opened.LiveControl != nil {
		closeSession = opened.LiveControl.CloseFactorySession
	} else if opened.Sessions != nil {
		if closer, ok := opened.Sessions.(interface {
			CloseFactorySession(context.Context, string) error
		}); ok {
			closeSession = closer.CloseFactorySession
		}
	}
	if closeSession != nil {
		if err := closeSession(cleanupContext, l.sessionID); err != nil {
			if !errors.Is(err, factorysessions.ErrSessionNotFound) {
				result = errors.Join(result, err)
			}
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

func invocationTargetSessionID(target roles.InvocationTarget) string {
	if sessionID := strings.TrimSpace(target.FactorySessionID); sessionID != "" {
		return sessionID
	}
	return factorysessions.DefaultSessionID
}

func closeArtifacts(opened roles.OpenedInvocationRuntime) error {
	if opened.CloseArtifacts == nil {
		return nil
	}
	return opened.CloseArtifacts()
}

var _ roles.InvocationOperation = (*operation)(nil)
