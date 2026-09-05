package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	canonicaldurable "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/canonical/durable"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/controlplane"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionvalidation"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func (s *Service) durableExecution() (durableexecution.Service, error) {
	if s == nil || s.durable == nil {
		return nil, factorysessions.ErrExecutionServiceNotConfigured
	}
	return s.durable, nil
}

func (s *Service) StartAsync(ctx context.Context, request factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.AsyncStartResult{}, err
	}
	return execution.StartAsync(ctx, request)
}

func (s *Service) StartSync(ctx context.Context, request factorysessions.StartRequest) (factorysessions.SyncStartResult, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.SyncStartResult{}, err
	}
	return execution.StartSync(ctx, request)
}

func (s *Service) ResumeInterruptedSession(ctx context.Context, sessionID string, request factorysessions.ResumeSessionRequest) (factorysessions.AsyncStartResult, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.AsyncStartResult{}, err
	}
	return execution.ResumeInterruptedSession(ctx, sessionID, request)
}

func (s *Service) GetSession(ctx context.Context, sessionID string) (factorysessions.SessionReadResult, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.SessionReadResult{}, err
	}
	return execution.GetSession(ctx, sessionID)
}

func (s *Service) Pause(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return execution.Pause(ctx, sessionID, request)
}

func (s *Service) Resume(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return execution.Resume(ctx, sessionID, request)
}

func (s *Service) Cancel(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return execution.Cancel(ctx, sessionID, request)
}

func (s *Service) Terminate(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return execution.Terminate(ctx, sessionID, request)
}

func (s *Service) Approve(ctx context.Context, sessionID string, request factorysessions.ApproveRequest) (factorysessions.LifecycleControlResult, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return execution.Approve(ctx, sessionID, request)
}

func (s *Service) RetryDispatch(ctx context.Context, sessionID string, request factorysessions.RetryDispatchRequest) (factorysessions.LifecycleControlResult, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return execution.RetryDispatch(ctx, sessionID, request)
}

func (s *Service) InterruptDispatch(ctx context.Context, sessionID string, request factorysessions.InterruptDispatchRequest) (factorysessions.LifecycleControlResult, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return execution.InterruptDispatch(ctx, sessionID, request)
}

func (s *Service) GetResult(ctx context.Context, sessionID string, request factorysessions.ResultRequest) (factorysessions.ResultReadResult, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.ResultReadResult{}, err
	}
	return execution.GetResult(ctx, sessionID, request)
}

func (s *Service) ListDispatches(ctx context.Context, sessionID string) (factorysessions.ListDispatchesResult, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.ListDispatchesResult{}, err
	}
	return execution.ListDispatches(ctx, sessionID)
}

func (s *Service) QueryDispatches(ctx context.Context, request factorysessions.DispatchQueryRequest) (factorysessions.ListDispatchesResult, error) {
	return s.queryCanonicalDispatches(ctx, request)
}

func (s *Service) GetDispatch(ctx context.Context, sessionID, dispatchID string) (factorysessions.DispatchDetail, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.DispatchDetail{}, err
	}
	return execution.GetDispatch(ctx, sessionID, dispatchID)
}

func (s *Service) ListArtifacts(ctx context.Context, sessionID string) (factorysessions.ListArtifactsResult, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.ListArtifactsResult{}, err
	}
	return execution.ListArtifacts(ctx, sessionID)
}

func (s *Service) GetArtifact(ctx context.Context, sessionID, artifactID string) (factorysessions.ArtifactDetail, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.ArtifactDetail{}, err
	}
	return execution.GetArtifact(ctx, sessionID, artifactID)
}

func (s *Service) ReadEvents(ctx context.Context, sessionID string, request factorysessions.EventReconnectRequest) (factorysessions.EventReadResult, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.EventReadResult{}, err
	}
	return execution.ReadEvents(ctx, sessionID, request)
}

func (s *Service) ListSessions(ctx context.Context, request factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error) {
	scope := request.Scope
	if scope == "" {
		scope = factorysessions.DefaultSessionListScope
		request.Scope = scope
	}
	includeRecordedHistory := s != nil && s.recordedHistory != nil &&
		!request.ExcludeRecordedHistory &&
		(scope == factorysessions.SessionListScopeHistory || scope == factorysessions.SessionListScopeAll)
	if scope == factorysessions.SessionListScopeHistory && includeRecordedHistory {
		return s.recordedHistory(ctx, factorysessions.ListSessionsRequest{
			Scope:   factorysessions.SessionListScopeHistory,
			Filters: request.Filters,
		})
	}
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.ListSessionsResult{}, err
	}
	result, err := execution.ListSessions(ctx, request)
	if err != nil || !includeRecordedHistory {
		return result, err
	}
	history, err := s.recordedHistory(ctx, factorysessions.ListSessionsRequest{
		Scope:   factorysessions.SessionListScopeHistory,
		Filters: request.Filters,
	})
	if err != nil {
		return factorysessions.ListSessionsResult{}, err
	}
	result.RecordedSessions = append(result.RecordedSessions, history.RecordedSessions...)
	sort.SliceStable(result.RecordedSessions, func(left, right int) bool {
		if result.RecordedSessions[left].SessionID != result.RecordedSessions[right].SessionID {
			return result.RecordedSessions[left].SessionID < result.RecordedSessions[right].SessionID
		}
		return result.RecordedSessions[left].ArtifactReference < result.RecordedSessions[right].ArtifactReference
	})
	return result, nil
}

// Start executes one mode-neutral Factory Sessions start operation. The
// canonical boundary owns the request conversion and returns only Sessions
// projections; legacy public start methods remain compatibility entrypoints.
func (s *Service) Start(
	ctx context.Context,
	request factorysessions.SessionStartRequest,
) (factorysessions.SessionStartResult, error) {
	if err := validateCanonicalStartRequest(request); err != nil {
		return factorysessions.SessionStartResult{}, err
	}
	if s == nil {
		return factorysessions.SessionStartResult{}, fmt.Errorf("Factory Sessions gateway is required")
	}

	switch request.Mode {
	case factorysessions.SessionOperationModeLive:
		return s.startCanonicalLive(ctx, request)
	case factorysessions.SessionOperationModeDurable:
		return s.startCanonicalDurable(ctx, request)
	default:
		return factorysessions.SessionStartResult{}, canonicalRequestError(
			"mode", "mode must be live or durable",
		)
	}
}

// Invoke executes one mode-neutral invocation through the already-bound
// session invocation owner. Prepared Work input is cloned before the private
// owner boundary and the result is projected into Sessions-owned values.
func (s *Service) Invoke(
	ctx context.Context,
	request factorysessions.SessionInvokeRequest,
) (factorysessions.InvocationResult, error) {
	if err := validateCanonicalInvokeRequest(request); err != nil {
		return factorysessions.InvocationResult{}, err
	}
	if s == nil || s.invoker == nil {
		return factorysessions.InvocationResult{}, fmt.Errorf("Factory Session invocation service is required")
	}
	canonicalInvoker, ok := s.invoker.(roles.CanonicalSessionInvoker)
	if !ok || canonicalInvoker == nil {
		return factorysessions.InvocationResult{}, fmt.Errorf("Factory Session canonical invocation service is required")
	}
	result, err := canonicalInvoker.Invoke(
		ctx,
		strings.TrimSpace(request.SessionID),
		canonicalInvocationRequest(request),
	)
	if err != nil {
		return factorysessions.InvocationResult{}, err
	}
	return factorysessions.InvocationResult{
		RequestID:     result.RequestID,
		TraceID:       result.TraceID,
		Status:        factorysessions.InvocationTerminalStatus(result.Status),
		PrimaryResult: work.CloneWorkContentParts(result.PrimaryResult),
		ErrorCode:     result.ErrorCode,
		Message:       result.Message,
		SessionID:     result.SessionID,
		WorkID:        result.WorkID,
		WorkName:      result.WorkName,
		WorkState:     result.WorkState,
	}, nil
}

func validateCanonicalStartRequest(request factorysessions.SessionStartRequest) error {
	if request.Wait.TimeoutMillis < 0 {
		return canonicalRequestError("wait.timeoutMillis", "timeout must not be negative")
	}
	switch request.Mode {
	case factorysessions.SessionOperationModeLive:
	case factorysessions.SessionOperationModeDurable:
		if strings.TrimSpace(request.Correlation.RequestID) == "" {
			return canonicalRequestError("correlation.requestId", "request id is required")
		}
	default:
		return canonicalRequestError("mode", "mode must be live or durable")
	}
	if request.ValidateOnly && request.InitNewFactory {
		return sessionvalidation.New(
			factorysessions.ValidationReasonRequired,
			"initNewFactory",
			fmt.Errorf("initNewFactory cannot be combined with validateOnly"),
		)
	}
	return nil
}

func validateCanonicalInvokeRequest(request factorysessions.SessionInvokeRequest) error {
	if strings.TrimSpace(request.SessionID) == "" {
		return canonicalRequestError("sessionId", "session id is required")
	}
	if request.Wait.TimeoutMillis < 0 {
		return canonicalRequestError("wait.timeoutMillis", "timeout must not be negative")
	}
	return nil
}

func canonicalRequestError(field, message string) error {
	return &factorysessions.DetachedRequestError{Field: field, Message: message}
}

func (s *Service) startCanonicalLive(
	ctx context.Context,
	request factorysessions.SessionStartRequest,
) (factorysessions.SessionStartResult, error) {
	if s.host == nil {
		return factorysessions.SessionStartResult{}, fmt.Errorf("Factory Sessions gateway is required")
	}
	if s.liveRuntime == nil {
		return factorysessions.SessionStartResult{}, fmt.Errorf(
			"%w: live session service is required", factorysessions.ErrRuntimeNotAvailable,
		)
	}
	opened, err := controlplane.OpenFromFolder(
		ctx,
		s.host,
		s.liveRuntime,
		strings.TrimSpace(request.FolderPath),
		cloneCanonicalTarget(request.Target),
		request.ValidateOnly,
		request.InitNewFactory,
	)
	if err != nil {
		return factorysessions.SessionStartResult{}, err
	}
	if opened == nil {
		return factorysessions.SessionStartResult{}, fmt.Errorf("open Factory Session returned no result")
	}
	return s.canonicalLiveStartResult(opened), nil
}

func (s *Service) canonicalLiveStartResult(
	opened *factorysessions.OpenResult,
) factorysessions.SessionStartResult {
	result := factorysessions.SessionStartResult{
		SessionID: opened.SessionID,
		Mode:      factorysessions.SessionOperationModeLive,
		Status:    "OPENED",
		Live: &factorysessions.SessionOpenResult{
			SessionID:             opened.SessionID,
			Targets:               cloneCanonicalTargets(opened.Targets),
			InitializedNewFactory: opened.InitsNewFactory,
			FolderPath:            opened.FolderPath,
		},
	}
	if s.liveRuntime == nil || strings.TrimSpace(opened.SessionID) == "" {
		return result
	}
	session := s.liveRuntime.Resolve(opened.SessionID)
	if session == nil {
		return result
	}
	view := factorysessions.SessionView{
		SessionID:        livesession.CanonicalID(session),
		Mode:             factorysessions.SessionOperationModeLive,
		Status:           "OPENED",
		FactoryDir:       session.FactoryDir,
		FolderPath:       session.FolderPath,
		Project:          session.Project,
		IsDefault:        session.IsDefault,
		Target:           session.Target,
		RuntimeAvailable: session.Runtime != nil,
	}
	if view.SessionID == "" {
		view.SessionID = opened.SessionID
	}
	result.SessionID = view.SessionID
	result.Live.SessionID = view.SessionID
	result.Live.Session = &view
	return result
}

func (s *Service) startCanonicalDurable(
	ctx context.Context,
	request factorysessions.SessionStartRequest,
) (factorysessions.SessionStartResult, error) {
	execution, err := s.durableExecution()
	if err != nil {
		return factorysessions.SessionStartResult{}, err
	}
	canonicalExecution, ok := execution.(canonicaldurable.Service)
	if !ok || canonicalExecution == nil {
		return factorysessions.SessionStartResult{}, fmt.Errorf(
			"%w: canonical durable start service is required", factorysessions.ErrExecutionServiceNotConfigured,
		)
	}
	legacyRequest := canonicalDurableStartRequest(request)
	started, err := canonicalExecution.StartCanonical(ctx, legacyRequest, request.Synchronous)
	if err != nil {
		return factorysessions.SessionStartResult{}, err
	}
	if started.Sync != nil {
		syncResult := started.Sync
		return factorysessions.SessionStartResult{
			SessionID: syncResult.SessionID,
			Mode:      factorysessions.SessionOperationModeDurable,
			Status:    canonicalStartedStatus(syncResult.AsyncStartResult.Status, string(syncResult.SyncOutcome)),
			Sync:      cloneCanonicalSyncStartResult(syncResult),
		}, nil
	}
	if started.Async == nil {
		return factorysessions.SessionStartResult{}, fmt.Errorf("canonical durable start returned no result")
	}
	asyncResult := started.Async
	return factorysessions.SessionStartResult{
		SessionID: asyncResult.SessionID,
		Mode:      factorysessions.SessionOperationModeDurable,
		Status:    asyncResult.Status,
		Async:     cloneCanonicalAsyncStartResult(asyncResult),
	}, nil
}

func canonicalStartedStatus(status, fallback string) string {
	if strings.TrimSpace(status) != "" {
		return status
	}
	return fallback
}

func canonicalDurableStartRequest(
	request factorysessions.SessionStartRequest,
) factorysessions.StartRequest {
	source := cloneCanonicalSource(request.Source)
	if source.Kind == "" && strings.TrimSpace(request.Definition.FactoryID) != "" {
		source.Kind = factoryruntime.WorkflowSourceKindFactoryID
		source.FactoryID = strings.TrimSpace(request.Definition.FactoryID)
	}
	legacy := factorysessions.StartRequest{
		RequestID:       strings.TrimSpace(request.Correlation.RequestID),
		Source:          source,
		Args:            cloneCanonicalAnyMap(request.Args),
		RequestedPolicy: cloneCanonicalAnyMap(request.Policy),
		Orchestrator:    cloneCanonicalOrchestrator(request.Orchestrator),
		Runtime:         cloneCanonicalRuntimeOptions(request.RuntimeOptions),
	}
	if len(legacy.Args) == 0 && request.Input != nil && request.Input.NormalizedArguments != nil {
		legacy.Args = canonicalNormalizedArgumentsToValues(request.Input.NormalizedArguments)
	}
	if request.Wait.TimeoutMillis > 0 || request.Wait.CancelOnTimeout {
		timeout := request.Wait.TimeoutMillis
		legacy.Wait = &factorysessions.WaitOptions{
			TimeoutMillis:   &timeout,
			CancelOnTimeout: request.Wait.CancelOnTimeout,
		}
	}
	return legacy
}

func canonicalInvocationRequest(
	request factorysessions.SessionInvokeRequest,
) factorysessions.InvocationRequest {
	legacy := factorysessions.InvocationRequest{}
	if request.Input != nil {
		legacy.PreparedInvocationInput = request.Input.Clone()
		legacy.ContentProvided = true
		sourceKind := factorysessions.InvocationInputSourceKind(request.Input.Source)
		legacy.SourceKind = &sourceKind
	}
	if requestID := strings.TrimSpace(request.Correlation.RequestID); requestID != "" {
		legacy.RequestID = &requestID
	}
	if request.Wait.TimeoutMillis > 0 {
		timeoutMillis := request.Wait.TimeoutMillis
		legacy.TimeoutMillis = &timeoutMillis
	}
	legacy.CancelOnTimeout = request.Wait.CancelOnTimeout
	return legacy
}

func cloneCanonicalTarget(target *factorysessions.TargetRef) *factorysessions.TargetRef {
	if target == nil {
		return nil
	}
	cloned := *target
	return &cloned
}

func cloneCanonicalTargets(targets []factorysessions.Target) []factorysessions.Target {
	if len(targets) == 0 {
		return nil
	}
	return append([]factorysessions.Target(nil), targets...)
}

func cloneCanonicalSource(source factorysessions.Source) factorysessions.Source {
	cloned := source
	cloned.FactoryInline = append(json.RawMessage(nil), source.FactoryInline...)
	if source.InlineWorkflow == nil {
		return cloned
	}
	inline := *source.InlineWorkflow
	inline.ArgsSchema = append(json.RawMessage(nil), source.InlineWorkflow.ArgsSchema...)
	inline.DefaultPolicy = append(json.RawMessage(nil), source.InlineWorkflow.DefaultPolicy...)
	inline.Metadata = cloneCanonicalStringMap(source.InlineWorkflow.Metadata)
	if len(source.InlineWorkflow.Agents) > 0 {
		inline.Agents = make(map[string]factorydefinitions.FactoryOrchestratorJavaScriptAgent, len(source.InlineWorkflow.Agents))
		for name, agent := range source.InlineWorkflow.Agents {
			inline.Agents[name] = agent
		}
	}
	cloned.InlineWorkflow = &inline
	return cloned
}

func cloneCanonicalOrchestrator(
	override *factorysessions.OrchestratorOverride,
) *factorysessions.OrchestratorOverride {
	if override == nil {
		return nil
	}
	cloned := *override
	cloned.Raw = append(json.RawMessage(nil), override.Raw...)
	return &cloned
}

func cloneCanonicalRuntimeOptions(
	options *factorysessions.RuntimeOptions,
) *factorysessions.RuntimeOptions {
	if options == nil {
		return nil
	}
	cloned := *options
	return &cloned
}

func cloneCanonicalAnyMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(values))
	for key, value := range values {
		cloned[key] = cloneCanonicalAnyValue(value)
	}
	return cloned
}

func cloneCanonicalAnyValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneCanonicalAnyMap(typed)
	case []any:
		cloned := make([]any, len(typed))
		for index, item := range typed {
			cloned[index] = cloneCanonicalAnyValue(item)
		}
		return cloned
	case []string:
		return append([]string(nil), typed...)
	default:
		return value
	}
}

func cloneCanonicalStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func canonicalNormalizedArgumentsToValues(
	arguments *work.NormalizedArguments,
) map[string]any {
	if arguments == nil || len(arguments.Arguments) == 0 {
		return nil
	}
	values := make(map[string]any, len(arguments.Arguments))
	for name, argument := range arguments.Arguments {
		if len(argument.Values) == 1 {
			values[name] = argument.Values[0]
			continue
		}
		values[name] = append([]string(nil), argument.Values...)
	}
	return values
}

func cloneCanonicalAsyncStartResult(
	result *factorysessions.AsyncStartResult,
) *factorysessions.AsyncStartResult {
	if result == nil {
		return nil
	}
	cloned := *result
	cloned.Policy.Requested = cloneCanonicalAnyMap(result.Policy.Requested)
	cloned.Policy.Effective = cloneCanonicalAnyMap(result.Policy.Effective)
	cloned.ResolvedSource.ResolutionOrder = append([]string(nil), result.ResolvedSource.ResolutionOrder...)
	cloned.ResolvedSource.Metadata = cloneCanonicalStringMap(result.ResolvedSource.Metadata)
	cloned.ResolvedSource.Agents = make(map[string]factorydefinitions.FactoryOrchestratorJavaScriptAgent, len(result.ResolvedSource.Agents))
	for name, agent := range result.ResolvedSource.Agents {
		cloned.ResolvedSource.Agents[name] = agent
	}
	cloned.ResolvedSource.ArgsSchema = append(json.RawMessage(nil), result.ResolvedSource.ArgsSchema...)
	cloned.ResolvedSource.DefaultPolicy = append(json.RawMessage(nil), result.ResolvedSource.DefaultPolicy...)
	return &cloned
}

func cloneCanonicalSyncStartResult(
	result *factorysessions.SyncStartResult,
) *factorysessions.SyncStartResult {
	if result == nil {
		return nil
	}
	cloned := *result
	if async := cloneCanonicalAsyncStartResult(&result.AsyncStartResult); async != nil {
		cloned.AsyncStartResult = *async
	}
	cloned.Result = append(json.RawMessage(nil), result.Result...)
	return &cloned
}
