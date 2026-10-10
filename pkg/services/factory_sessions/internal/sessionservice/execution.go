package service

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/workersettings"
	"sort"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	canonicaldurable "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/canonical/durable"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func (s *Service) durableExecution() (durableexecution.Service, error) {
	if s == nil || s.durable == nil {
		return nil, factorysessions.ErrExecutionServiceNotConfigured
	}
	return s.durable, nil
}

// replayExecutionBinding scopes the existing replay owner to its inspection.
type replayExecutionBinding struct{ owner durableexecution.Service }

// BindHistoricalExecution attaches an existing owner to the addressed public
// route without publishing a live Factory runtime. Cleanup releases only this
// inspection's binding, including when another inspection replaced it.
func (s *Service) BindHistoricalExecution(sessionID string, owner durableexecution.Service) func() {
	binding := &replayExecutionBinding{owner: owner}
	s.replayMu.Lock()
	if s.replayExecutions == nil {
		s.replayExecutions = make(map[string]*replayExecutionBinding)
	}
	s.replayExecutions[sessionID] = binding
	s.replayMu.Unlock()
	return func() {
		s.replayMu.Lock()
		defer s.replayMu.Unlock()
		if s.replayExecutions[sessionID] == binding {
			delete(s.replayExecutions, sessionID)
		}
	}
}

func (s *Service) executionForSession(sessionID string) (durableexecution.Service, error) {
	if s != nil {
		s.replayMu.RLock()
		binding := s.replayExecutions[sessionID]
		s.replayMu.RUnlock()
		if binding != nil {
			return binding.owner, nil
		}
	}
	return s.durableExecution()
}

func (s *Service) ApplyDurableLiveChange(ctx context.Context, sessionID string, request factorysessions.LiveChangeRequest, runtime factoryruntime.Service, projectRoot string) (factorysessions.LiveChangeResult, error) {
	execution, err := s.executionForSession(sessionID)
	if err != nil {
		return factorysessions.LiveChangeResult{}, err
	}
	capability, ok := execution.(interface {
		ApplyLiveChangeWithRuntime(context.Context, string, factorysessions.LiveChangeRequest, factoryruntime.Service, string) (factorysessions.LiveChangeResult, error)
	})
	if !ok {
		return factorysessions.LiveChangeResult{}, factorysessions.ErrRuntimeNotAvailable
	}
	return capability.ApplyLiveChangeWithRuntime(ctx, sessionID, request, runtime, projectRoot)
}

func (s *Service) RecoverDurableLiveChange(ctx context.Context, sessionID, requestID string) (factorysessions.LiveChangeResult, error) {
	execution, err := s.executionForSession(sessionID)
	if err != nil {
		return factorysessions.LiveChangeResult{}, err
	}
	capability, ok := execution.(interface {
		RecoverLiveChange(context.Context, string, string) (factorysessions.LiveChangeResult, error)
	})
	if !ok {
		return factorysessions.LiveChangeResult{}, factorysessions.ErrRuntimeNotAvailable
	}
	return capability.RecoverLiveChange(ctx, sessionID, requestID)
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
	execution, err := s.executionForSession(sessionID)
	if err != nil {
		return factorysessions.AsyncStartResult{}, err
	}
	return execution.ResumeInterruptedSession(ctx, sessionID, request)
}

func (s *Service) GetSession(ctx context.Context, sessionID string) (factorysessions.SessionReadResult, error) {
	execution, err := s.executionForSession(sessionID)
	if err != nil {
		return factorysessions.SessionReadResult{}, err
	}
	return execution.GetSession(ctx, sessionID)
}

func (s *Service) Pause(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	execution, err := s.executionForSession(sessionID)
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return execution.Pause(ctx, sessionID, request)
}

func (s *Service) Resume(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	execution, err := s.executionForSession(sessionID)
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return execution.Resume(ctx, sessionID, request)
}

func (s *Service) Cancel(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	execution, err := s.executionForSession(sessionID)
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return execution.Cancel(ctx, sessionID, request)
}

func (s *Service) Terminate(ctx context.Context, sessionID string, request factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	execution, err := s.executionForSession(sessionID)
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return execution.Terminate(ctx, sessionID, request)
}

func (s *Service) Approve(ctx context.Context, sessionID string, request factorysessions.ApproveRequest) (factorysessions.LifecycleControlResult, error) {
	execution, err := s.executionForSession(sessionID)
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return execution.Approve(ctx, sessionID, request)
}

func (s *Service) RetryDispatch(ctx context.Context, sessionID string, request factorysessions.RetryDispatchRequest) (factorysessions.LifecycleControlResult, error) {
	execution, err := s.executionForSession(sessionID)
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return execution.RetryDispatch(ctx, sessionID, request)
}

func (s *Service) InterruptDispatch(ctx context.Context, sessionID string, request factorysessions.InterruptDispatchRequest) (factorysessions.LifecycleControlResult, error) {
	execution, err := s.executionForSession(sessionID)
	if err != nil {
		return factorysessions.LifecycleControlResult{}, err
	}
	return execution.InterruptDispatch(ctx, sessionID, request)
}

func (s *Service) GetResult(ctx context.Context, sessionID string, request factorysessions.ResultRequest) (factorysessions.ResultReadResult, error) {
	execution, err := s.executionForSession(sessionID)
	if err != nil {
		return factorysessions.ResultReadResult{}, err
	}
	return execution.GetResult(ctx, sessionID, request)
}

func (s *Service) ListDispatches(ctx context.Context, sessionID string) (factorysessions.ListDispatchesResult, error) {
	execution, err := s.executionForSession(sessionID)
	if err != nil {
		return factorysessions.ListDispatchesResult{}, err
	}
	return execution.ListDispatches(ctx, sessionID)
}

func (s *Service) QueryDispatches(ctx context.Context, request factorysessions.DispatchQueryRequest) (factorysessions.ListDispatchesResult, error) {
	return s.queryCanonicalDispatches(ctx, request)
}

func (s *Service) GetDispatch(ctx context.Context, sessionID, dispatchID string) (factorysessions.DispatchDetail, error) {
	execution, err := s.executionForSession(sessionID)
	if err != nil {
		return factorysessions.DispatchDetail{}, err
	}
	return execution.GetDispatch(ctx, sessionID, dispatchID)
}

func (s *Service) ListArtifacts(ctx context.Context, sessionID string) (factorysessions.ListArtifactsResult, error) {
	execution, err := s.executionForSession(sessionID)
	if err != nil {
		return factorysessions.ListArtifactsResult{}, err
	}
	return execution.ListArtifacts(ctx, sessionID)
}

func (s *Service) GetArtifact(ctx context.Context, sessionID, artifactID string) (factorysessions.ArtifactDetail, error) {
	execution, err := s.executionForSession(sessionID)
	if err != nil {
		return factorysessions.ArtifactDetail{}, err
	}
	return execution.GetArtifact(ctx, sessionID, artifactID)
}

func (s *Service) ReadEvents(ctx context.Context, sessionID string, request factorysessions.EventReconnectRequest) (factorysessions.EventReadResult, error) {
	execution, err := s.executionForSession(sessionID)
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
		return s.recordedHistory.ListSessions(ctx, factorysessions.ListSessionsRequest{
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
	history, err := s.recordedHistory.ListSessions(ctx, factorysessions.ListSessionsRequest{
		Scope:   factorysessions.SessionListScopeHistory,
		Filters: request.Filters,
	})
	if err != nil {
		return factorysessions.ListSessionsResult{}, err
	}
	result.RecordedSessions = append(result.RecordedSessions, history.RecordedSessions...)
	result.Warnings = append(result.Warnings, history.Warnings...)
	sort.SliceStable(result.RecordedSessions, func(left, right int) bool {
		if result.RecordedSessions[left].SessionID != result.RecordedSessions[right].SessionID {
			return result.RecordedSessions[left].SessionID < result.RecordedSessions[right].SessionID
		}
		return result.RecordedSessions[left].ArtifactReference < result.RecordedSessions[right].ArtifactReference
	})
	return result, nil
}

// StartDurable admits persisted execution through the process-owned durable service.
func (s *Service) StartDurable(ctx context.Context, request factorysessions.SessionStartRequest) (factorysessions.SessionStartResult, error) {
	if err := ValidateCanonicalDurableStartRequest(request); err != nil {
		return factorysessions.SessionStartResult{}, err
	}
	return s.startCanonicalDurable(ctx, request)
}

// ValidateCanonicalDurableStartRequest preserves mode-neutral admission validation.
func ValidateCanonicalDurableStartRequest(request factorysessions.SessionStartRequest) error {
	if err := validateCanonicalStartRequest(request); err != nil {
		return err
	}
	if request.Mode != factorysessions.SessionOperationModeDurable {
		return canonicalRequestError("mode", "mode must be durable")
	}
	return nil
}

// Invoke executes one mode-neutral invocation through the already-bound
// session invocation owner. Prepared Work input is cloned before the private
// owner boundary and the result is projected into Sessions-owned values.
func (s *Service) Invoke(
	ctx context.Context,
	request factorysessions.SessionInvokeRequest,
) (factorysessions.InvocationResult, error) {
	if s == nil {
		return factorysessions.InvocationResult{}, fmt.Errorf("Factory Session invocation service is required")
	}
	canonicalInvoker, ok := s.invoker.(roles.CanonicalSessionInvoker)
	if !ok {
		return factorysessions.InvocationResult{}, fmt.Errorf("Factory Session canonical invocation service is required")
	}
	return invokeCanonicalSession(ctx, canonicalInvoker, request)
}

func invokeCanonicalSession(
	ctx context.Context,
	invoker roles.CanonicalSessionInvoker,
	request factorysessions.SessionInvokeRequest,
) (factorysessions.InvocationResult, error) {
	if err := validateCanonicalInvokeRequest(request); err != nil {
		return factorysessions.InvocationResult{}, err
	}
	if invoker == nil {
		return factorysessions.InvocationResult{}, fmt.Errorf("Factory Session canonical invocation service is required")
	}
	result, err := invoker.Invoke(
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
		FailureReason: result.FailureReason,
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
		if strings.TrimSpace(request.FolderPath) == "" {
			return canonicalRequestError("folderPath", "Factory project root is required")
		}
	default:
		return canonicalRequestError("mode", "mode must be live or durable")
	}
	if request.ValidateOnly && request.InitNewFactory {
		return canonicalRequestError("initNewFactory", "initNewFactory cannot be combined with validateOnly")
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
	legacyRequest := CanonicalDurableStartRequest(request)
	started, err := canonicalExecution.StartCanonical(ctx, legacyRequest, request.Synchronous)
	if err != nil {
		return factorysessions.SessionStartResult{}, err
	}
	return CanonicalDurableStartResult(started)
}

// CanonicalDurableStartResult projects and clones the selected durable result.
func CanonicalDurableStartResult(started durableexecution.CanonicalStartResult) (factorysessions.SessionStartResult, error) {
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

// CanonicalDurableStartRequest snapshots the mode-neutral selections for durable execution.
func CanonicalDurableStartRequest(
	request factorysessions.SessionStartRequest,
) factorysessions.StartRequest {
	source := cloneCanonicalSource(request.Source)
	if source.Kind == "" && strings.TrimSpace(request.Definition.FactoryID) != "" {
		source.Kind = factoryruntime.WorkflowSourceKindFactoryID
		source.FactoryID = strings.TrimSpace(request.Definition.FactoryID)
	}
	legacy := factorysessions.StartRequest{
		RequestID:               strings.TrimSpace(request.Correlation.RequestID),
		Caller:                  request.Caller.Clone(),
		Source:                  source,
		Args:                    cloneCanonicalAnyMap(request.Args),
		RequestedPolicy:         cloneCanonicalAnyMap(request.Policy),
		Orchestrator:            cloneCanonicalOrchestrator(request.Orchestrator),
		Runtime:                 cloneCanonicalRuntimeOptions(request.RuntimeOptions),
		ProjectRoot:             strings.TrimSpace(request.FolderPath),
		PersistencePolicy:       request.Persistence,
		MockWorkers:             runtimeSelectionMockWorkers(request.RuntimeSelection),
		WorkerSettings:          workersettings.Clone(request.WorkerSettings),
		WorkerAttemptStarter:    request.WorkerAttemptStarter,
		WorkerProgressPublisher: request.WorkerProgressPublisher,
		WorkerResourceAdmission: request.WorkerResourceAdmission,
	}
	if legacy.PersistencePolicy == "" {
		legacy.PersistencePolicy = factorysessions.PersistencePolicyEnabled
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

func runtimeSelectionMockWorkers(selection *factorysessions.SessionRuntimeSelection) *workers.MockWorkersConfig {
	if selection == nil {
		return nil
	}
	return selection.Workers.MockWorkers.Clone()
}

func canonicalInvocationRequest(
	request factorysessions.SessionInvokeRequest,
) factorysessions.InvocationRequest {
	legacy := factorysessions.InvocationRequest{Caller: request.Caller.Clone()}
	if request.Args != nil {
		args := cloneCanonicalAnyMap(request.Args)
		legacy.Args = &args
	}
	if request.Input != nil {
		legacy.PreparedInvocationInput = request.Input.Clone()
		sourceKind := factorysessions.InvocationInputSourceKind(request.Input.Source)
		legacy.SourceKind = &sourceKind
	}
	if request.ContentProvided {
		legacy.Content = work.CloneWorkContentParts(request.Content)
		legacy.ContentProvided = true
		sourceKind := factorysessions.InvocationInputSourceKindText
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

// CanonicalInvocationRequest maps an existing-session invoke command to the
// invocation input shared by the live and JavaScript execution paths.
func CanonicalInvocationRequest(request factorysessions.SessionInvokeRequest) factorysessions.InvocationRequest {
	return canonicalInvocationRequest(request)
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
