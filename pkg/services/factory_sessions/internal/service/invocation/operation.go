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
	"github.com/portpowered/infinite-you/pkg/services/models"
	"go.uber.org/zap"
)

type sessionModelInvoker interface {
	InvokeModelForSession(context.Context, string, string, models.Request) (models.Result, error)
}

type sessionModelScopeReader interface {
	ModelsScopeForSession(context.Context, string) (models.RuntimeScopeRef, error)
}

type sessionInputReader interface {
	ResolveInvocationInputForSession(context.Context, string, factorysessions.InvocationRequest) (factorysessions.ResolvedInvocationInput, error)
}

type operation struct {
	sessions           factorysessions.Service
	sessionModels      sessionModelInvoker
	sessionModelScopes sessionModelScopeReader
	sessionInput       sessionInputReader
	modelsRoot         models.Service
	logger             *zap.Logger
	workingDirectory   platformfilesystem.WorkingDirectory
	resolveCurrentDir  factorydefinitions.CurrentFactoryDirectoryResolver
	artifactExporter   factorysessioncontracts.InvocationArtifactExporter
	modelTimeout       factorysessions.ModelInvocationTimeout
	artifactRoots      factoryruntime.RuntimeArtifactRootResolver
	generateSessionID  factorysessions.SessionIDGenerator
	presentations      factorysessions.OpeningPresentationOwner
}

func NewOperation(
	sessions factorysessions.Service,
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
	if sessions == nil {
		return nil, errors.New("invocation session service is required")
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
	sessionModels, ok := sessions.(sessionModelInvoker)
	if !ok {
		return nil, errors.New("invocation session service does not support session model invocation")
	}
	sessionModelScopes, ok := sessions.(sessionModelScopeReader)
	if !ok {
		return nil, errors.New("invocation session service does not support session model scopes")
	}
	sessionInput, ok := sessions.(sessionInputReader)
	if !ok {
		return nil, errors.New("invocation session service does not support session input resolution")
	}
	return &operation{
		sessions:           sessions,
		sessionModels:      sessionModels,
		sessionModelScopes: sessionModelScopes,
		sessionInput:       sessionInput,
		modelsRoot:         modelsRoot,
		logger:             logger,
		workingDirectory:   workingDirectory,
		resolveCurrentDir:  resolveCurrentDir,
		artifactExporter:   artifactExporter,
		modelTimeout:       modelTimeout,
		artifactRoots:      artifactRoots,
		generateSessionID:  generateSessionID,
		presentations:      presentations,
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
	sessionID := invocationTargetSessionID(target)
	if _, err := o.sessions.Start(invokeCtx, ActivationOnlyStartRequest(target, o.artifactRoots(target.HomeDir))); err != nil {
		return models.Result{}, err
	}
	defer func() {
		cleanupCtx := context.WithoutCancel(ctx)
		_, controlErr := o.sessions.Control(cleanupCtx, factorysessions.SessionControlRequest{
			SessionID: sessionID,
			Mode:      factorysessions.SessionOperationModeLive,
			Operation: factorysessions.SessionControlClose,
		})
		resultErr = errors.Join(resultErr, controlErr)
	}()
	result, resultErr = o.sessionModels.InvokeModelForSession(invokeCtx, sessionID, modelName, request)
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
	request.Caller = request.Caller.Clone()
	if o == nil || o.sessions == nil {
		return outcome, errors.New("invocation session service is required")
	}
	sessionID := invocationTargetSessionID(target)
	if _, err := o.sessions.Start(ctx, ActivationOnlyStartRequest(target, o.artifactRoots(target.HomeDir))); err != nil {
		return outcome, err
	}
	defer func() {
		cleanupCtx := context.WithoutCancel(ctx)
		_, controlErr := o.sessions.Control(cleanupCtx, factorysessions.SessionControlRequest{
			SessionID: sessionID,
			Mode:      factorysessions.SessionOperationModeLive,
			Operation: factorysessions.SessionControlClose,
		})
		resultErr = errors.Join(resultErr, controlErr)
	}()
	bridge, err := o.startFactoryEventBridge(ctx, o.sessions, target)
	if err != nil {
		return outcome, err
	}
	projection, projectionErr := o.sessions.GetFactorySession(ctx, sessionID)
	if projectionErr != nil && ctx.Err() != nil {
		return outcome, ctx.Err()
	}
	if projectionErr == nil && factorydefinitions.IsJavaScriptOrchestratorFactory(projection.Context.FactoryCfg) {
		result, err := o.invokeJavaScriptFactoryViaSessions(ctx, sessionID, projection.Context, target, request)
		outcome.Result = result
		if bridge != nil {
			err = joinTeardownErrorUnlessResultDetermined(outcome, err, bridge.Finish(ctx, o.sessions, outcome), o.logger)
		}
		return outcome, err
	}
	invocationResult, err := o.sessions.Invoke(ctx, sessionInvokeRequest(sessionID, request))
	outcome.Result = factoryInvocationResultFromSessionInvocation(invocationResult)
	resultErr = err
	if bridge != nil {
		resultErr = joinTeardownErrorUnlessResultDetermined(
			outcome, resultErr, bridge.Finish(ctx, o.sessions, outcome), o.logger,
		)
	}
	return outcome, resultErr
}

func sessionInvokeRequest(sessionID string, request factorysessions.InvocationRequest) factorysessions.SessionInvokeRequest {
	invoke := factorysessions.SessionInvokeRequest{
		SessionID:       sessionID,
		Caller:          request.Caller.Clone(),
		Input:           request.PreparedInvocationInput,
		Content:         request.Content,
		ContentProvided: request.ContentProvided,
	}
	if request.RequestID != nil {
		invoke.Correlation.RequestID = strings.TrimSpace(*request.RequestID)
	}
	if request.Args != nil {
		invoke.Args = *request.Args
	}
	if request.TimeoutMillis != nil {
		invoke.Wait.TimeoutMillis = *request.TimeoutMillis
	}
	invoke.Wait.CancelOnTimeout = request.CancelOnTimeout
	return invoke
}

func (o *operation) invokeJavaScriptFactoryViaSessions(
	ctx context.Context,
	sessionID string,
	projection factorysessions.ProjectionContext,
	target roles.InvocationTarget,
	request factorysessions.InvocationRequest,
) (factorydefinitions.FactoryInvocationResult, error) {
	return InvokeJavaScriptFactoryViaSessions(ctx, o.sessions, o.sessionInput, o.generateSessionID, sessionID, projection, target, request, nil)
}

// InvokeJavaScriptFactoryViaSessions executes the same JavaScript Factory
// invocation for CLI and session-scoped callers such as ACP.
func InvokeJavaScriptFactoryViaSessions(
	ctx context.Context,
	sessions factorysessions.Service,
	inputs interface {
		ResolveInvocationInputForSession(context.Context, string, factorysessions.InvocationRequest) (factorysessions.ResolvedInvocationInput, error)
	},
	generateSessionID factorysessions.SessionIDGenerator,
	sessionID string,
	projection factorysessions.ProjectionContext,
	target roles.InvocationTarget,
	request factorysessions.InvocationRequest,
	configure func(*factorysessions.StartRequest),
) (factorydefinitions.FactoryInvocationResult, error) {
	request.Caller = request.Caller.Clone()
	resolved, err := inputs.ResolveInvocationInputForSession(ctx, sessionID, request)
	if err != nil {
		return factorydefinitions.FactoryInvocationResult{}, err
	}
	startRequest, err := javaScriptStartRequest(
		projection, target, request, resolved, generateSessionID,
	)
	if err != nil {
		return factorydefinitions.FactoryInvocationResult{}, err
	}
	if configure != nil {
		configure(&startRequest)
	}
	started, err := sessions.StartSync(ctx, startRequest)
	if err != nil {
		return factorydefinitions.FactoryInvocationResult{}, err
	}
	result, err := sessions.GetResult(ctx, started.SessionID, factorysessions.ResultRequest{
		Mode: factorysessions.ResultModeFinal,
	})
	if err != nil {
		return factorydefinitions.FactoryInvocationResult{}, err
	}
	var sessionFailure *factorysessions.FailureSummary
	if result.Failure == nil && !javaScriptInvocationSucceeded(result) {
		if session, sessionErr := sessions.GetSession(ctx, started.SessionID); sessionErr == nil {
			sessionFailure = session.Failure
		}
	}
	return javaScriptInvocationResult(startRequest.RequestID, result, sessionFailure), nil
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

func javaScriptInvocationSucceeded(result factorysessions.ResultReadResult) bool {
	return result.SessionStatus == factorysessions.LifecycleStatusSucceeded &&
		result.ResultStatus == factorysessions.ResultStatusFinal
}

func javaScriptStartRequest(
	projection factorysessions.ProjectionContext,
	target roles.InvocationTarget,
	request factorysessions.InvocationRequest,
	resolved factorysessions.ResolvedInvocationInput,
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
	args := javaScriptInvocationArgs(js.ArgsSchema, resolved)
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
		Caller:          request.Caller.Clone(),
		ProjectRoot:     javaScriptInvocationProjectRoot(projection, target),
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
		source.WorkflowFile = filepath.Join(javaScriptInvocationProjectRoot(projection, target), source.WorkflowFile)
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

func javaScriptInvocationProjectRoot(projection factorysessions.ProjectionContext, target roles.InvocationTarget) string {
	if projection.Session != nil && strings.TrimSpace(projection.Session.FactoryDir) != "" {
		return strings.TrimSpace(projection.Session.FactoryDir)
	}
	return strings.TrimSpace(target.FactoryDir)
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
	argsSchema json.RawMessage,
	resolved factorysessions.ResolvedInvocationInput,
) map[string]any {
	if resolved.NormalizedArguments == nil {
		return map[string]any{}
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
	return args
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

func invocationTargetSessionID(target roles.InvocationTarget) string {
	if sessionID := strings.TrimSpace(target.FactorySessionID); sessionID != "" {
		return sessionID
	}
	return factorysessions.DefaultSessionID
}
