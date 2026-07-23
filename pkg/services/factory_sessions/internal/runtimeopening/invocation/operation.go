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
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeopening"
	"github.com/portpowered/infinite-you/pkg/services/models"
)

type operation struct {
	openRuntime       *runtimeopening.Factory
	edges             serviceedges.Edges
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
	edges serviceedges.Edges,
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
	edges.APIServerStarter = nil
	return &operation{
		openRuntime:       openRuntime,
		edges:             edges,
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
) (outcome roles.FactoryInvocationOutcome, resultErr error) {
	opened, lifecycle, err := o.open(ctx, target)
	if err != nil {
		return outcome, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, lifecycle.close(ctx, opened))
	}()

	if result, handled, err := invokeJavaScriptFactory(lifecycle.runContext, opened, target, request, o.generateSessionID); handled {
		outcome.Result = result
		return outcome, err
	}

	outcome.Result, resultErr = opened.Invoker.InvokeFactorySession(
		lifecycle.runContext, factorysessions.DefaultSessionID, request,
	)
	if session := opened.Sessions.ResolveFactorySession(factorysessions.DefaultSessionID); session != nil && session.ResponseEvents != nil {
		outcome.ResponseEvents = session.ResponseEvents.Events()
	}
	return outcome, resultErr
}

func invokeJavaScriptFactory(
	ctx context.Context,
	opened roles.OpenedInvocationRuntime,
	target roles.InvocationTarget,
	request factorysessions.InvocationRequest,
	generateSessionID factorysessions.SessionIDGenerator,
) (factorydefinitions.FactoryInvocationResult, bool, error) {
	projection, err := opened.Sessions.GetFactorySession(ctx, factorysessions.DefaultSessionID)
	if err != nil || !factorydefinitions.IsJavaScriptOrchestratorFactory(projection.Context.FactoryCfg) {
		return factorydefinitions.FactoryInvocationResult{}, false, nil
	}
	resolver := opened.InputResolver
	if resolver == nil {
		return factorydefinitions.FactoryInvocationResult{}, true, errors.New("Factory Session invocation input resolver is required")
	}
	startRequest, err := javaScriptStartRequest(projection.Context, target, request, resolver, generateSessionID)
	if err != nil {
		return factorydefinitions.FactoryInvocationResult{}, true, err
	}
	started, err := opened.Execution.StartSync(ctx, startRequest)
	if err != nil {
		return factorydefinitions.FactoryInvocationResult{}, true, err
	}
	result, err := opened.Execution.GetResult(ctx, started.SessionID, factorysessions.ResultRequest{
		Mode: factorysessions.ResultModeFinal,
	})
	if err != nil {
		return factorydefinitions.FactoryInvocationResult{}, true, err
	}
	return javaScriptInvocationResult(startRequest.RequestID, result), true, nil
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
		RequestID: requestID,
		Source:    source,
		Args:      args,
		Runtime:   &factorysessions.RuntimeOptions{ChildExecutorMode: childMode},
		Wait:      &factorysessions.WaitOptions{TimeoutMillis: request.TimeoutMillis},
	}, nil
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
) factorydefinitions.FactoryInvocationResult {
	out := factorydefinitions.FactoryInvocationResult{
		RequestID: requestID,
		SessionID: result.SessionID,
		Status:    factorydefinitions.InvocationTerminalStatusFailed,
		ErrorCode: string(factorydefinitions.InvocationErrorCodeRuntimeFailure),
	}
	if result.SessionStatus == factorysessions.LifecycleStatusSucceeded && result.ResultStatus == factorysessions.ResultStatusFinal {
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
	if result.Failure != nil {
		out.Message = strings.TrimSpace(result.Failure.Message)
		if out.Message == "" {
			out.Message = strings.TrimSpace(result.Failure.Reason)
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
	edges := o.edges
	if target.MetricsRecorder != nil {
		edges.InvocationMetricsRecorder = target.MetricsRecorder
	}
	opened, err := o.openRuntime.OpenInvocationRuntime(ctx, &config, edges, target.Logger)
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
