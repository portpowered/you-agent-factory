package factorysessionexecution

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workflowsource "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
)

// NormalizeStartRequest validates and normalizes one durable execution start request.
func NormalizeStartRequest(req StartRequest) (StartRequest, error) {
	requestID := strings.TrimSpace(req.RequestID)
	if requestID == "" {
		return StartRequest{}, NewValidationError("requestId", "requestId is required")
	}

	source, err := normalizeSource(req.Source)
	if err != nil {
		return StartRequest{}, err
	}

	normalized := StartRequest{
		RequestID:       requestID,
		Source:          source,
		Args:            cloneArgs(req.Args),
		RequestedPolicy: cloneArgs(req.RequestedPolicy),
		EventConsumer:   req.EventConsumer,
	}
	if req.Runtime != nil {
		runtime := *req.Runtime
		runtime.ChildExecutorMode = normalizeChildExecutorMode(runtime.ChildExecutorMode)
		normalized.Runtime = &runtime
	}
	if req.Orchestrator != nil {
		override := *req.Orchestrator
		override.Kind = strings.TrimSpace(override.Kind)
		if len(override.Raw) > 0 {
			canonical, err := canonicalizeRawJSON(override.Raw)
			if err != nil {
				return StartRequest{}, NewValidationError("orchestrator", "orchestrator must be a JSON object")
			}
			encoded, err := json.Marshal(canonical)
			if err != nil {
				return StartRequest{}, fmt.Errorf("marshal orchestrator: %w", err)
			}
			override.Raw = encoded
		}
		normalized.Orchestrator = &override
	}
	if req.Wait != nil {
		wait := *req.Wait
		normalized.Wait = &wait
	}
	return normalized, nil
}

func normalizeSource(source Source) (Source, error) {
	switch source.Kind {
	case workflowsource.WorkflowSourceKindFactoryID:
		factoryID := strings.TrimSpace(source.FactoryID)
		if factoryID == "" {
			return Source{}, NewValidationError("source.factoryId", "factoryId is required when source.kind is FACTORY_ID")
		}
		return Source{Kind: source.Kind, FactoryID: factoryID}, nil
	case workflowsource.WorkflowSourceKindFactoryInline:
		if len(source.FactoryInline) == 0 {
			return Source{}, NewValidationError("source.factoryInline", "factoryInline is required when source.kind is FACTORY_INLINE")
		}
		canonical, err := canonicalizeRawJSON(source.FactoryInline)
		if err != nil {
			return Source{}, NewValidationError("source.factoryInline", "factoryInline must be a JSON object")
		}
		encoded, err := json.Marshal(canonical)
		if err != nil {
			return Source{}, fmt.Errorf("marshal factoryInline: %w", err)
		}
		return Source{Kind: source.Kind, FactoryInline: encoded}, nil
	case workflowsource.WorkflowSourceKindWorkflowFile:
		workflowFile := strings.TrimSpace(source.WorkflowFile)
		if workflowFile == "" {
			return Source{}, NewValidationError("source.workflowFile", "workflowFile is required when source.kind is WORKFLOW_FILE")
		}
		return Source{
			Kind:           source.Kind,
			WorkflowFile:   workflowFile,
			InlineWorkflow: cloneInlineWorkflowSource(source.InlineWorkflow),
		}, nil
	case workflowsource.WorkflowSourceKindWorkflowName:
		workflowName := strings.TrimSpace(source.WorkflowName)
		if workflowName == "" {
			return Source{}, NewValidationError("source.workflowName", "workflowName is required when source.kind is WORKFLOW_NAME")
		}
		return Source{Kind: source.Kind, WorkflowName: workflowName}, nil
	case workflowsource.WorkflowSourceKindInlineWorkflow:
		if source.InlineWorkflow == nil {
			return Source{}, NewValidationError("source.inlineWorkflow", "inlineWorkflow is required when source.kind is INLINE_WORKFLOW")
		}
		inlineSource := strings.TrimSpace(source.InlineWorkflow.InlineSource)
		if inlineSource == "" {
			return Source{}, NewValidationError("source.inlineWorkflow.inlineSource", "inlineSource is required when source.kind is INLINE_WORKFLOW")
		}
		return Source{
			Kind: source.Kind,
			InlineWorkflow: &InlineWorkflowSource{
				Dialect:       strings.TrimSpace(source.InlineWorkflow.Dialect),
				InlineSource:  inlineSource,
				Entrypoint:    strings.TrimSpace(source.InlineWorkflow.Entrypoint),
				Metadata:      cloneStringMap(source.InlineWorkflow.Metadata),
				Agents:        cloneJavaScriptAgents(source.InlineWorkflow.Agents),
				ArgsSchema:    append(json.RawMessage(nil), source.InlineWorkflow.ArgsSchema...),
				DefaultPolicy: append(json.RawMessage(nil), source.InlineWorkflow.DefaultPolicy...),
			},
		}, nil
	default:
		return Source{}, NewValidationError("source.kind", "source.kind must be one of FACTORY_ID, FACTORY_INLINE, WORKFLOW_FILE, WORKFLOW_NAME, or INLINE_WORKFLOW")
	}
}

func cloneJavaScriptAgents(agents map[string]interfaces.FactoryOrchestratorJavaScriptAgent) map[string]interfaces.FactoryOrchestratorJavaScriptAgent {
	if len(agents) == 0 {
		return nil
	}
	cloned := make(map[string]interfaces.FactoryOrchestratorJavaScriptAgent, len(agents))
	for name, agent := range agents {
		cloned[name] = agent
	}
	return cloned
}

func cloneInlineWorkflowSource(source *InlineWorkflowSource) *InlineWorkflowSource {
	if source == nil {
		return nil
	}
	cloned := *source
	cloned.Metadata = cloneStringMap(source.Metadata)
	cloned.Agents = cloneJavaScriptAgents(source.Agents)
	cloned.ArgsSchema = append(json.RawMessage(nil), source.ArgsSchema...)
	cloned.DefaultPolicy = append(json.RawMessage(nil), source.DefaultPolicy...)
	return &cloned
}

func cloneArgs(args map[string]any) map[string]any {
	if len(args) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(args))
	for key, value := range args {
		cloned[key] = value
	}
	return cloned
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

func normalizeChildExecutorMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case "", ChildExecutorModeFake:
		return ChildExecutorModeFake
	case ChildExecutorModeLive:
		return ChildExecutorModeLive
	default:
		return strings.TrimSpace(mode)
	}
}

func resolveChildExecutorMode(configMode string, req StartRequest) string {
	if req.Runtime != nil && strings.TrimSpace(req.Runtime.ChildExecutorMode) != "" {
		return normalizeChildExecutorMode(req.Runtime.ChildExecutorMode)
	}
	return normalizeChildExecutorMode(configMode)
}

// NormalizeListSessionsRequest validates and normalizes one scoped session list request.
func NormalizeListSessionsRequest(req ListSessionsRequest) (ListSessionsRequest, error) {
	scope := req.Scope
	if scope == "" {
		scope = DefaultSessionListScope
	}
	switch scope {
	case SessionListScopeLive, SessionListScopePersisted, SessionListScopeHistory, SessionListScopeAll:
	default:
		return ListSessionsRequest{}, NewValidationError("scope", "scope must be live, persisted, or all; history is also supported")
	}

	filters, err := normalizeSessionListFilters(req.Filters)
	if err != nil {
		return ListSessionsRequest{}, err
	}
	return ListSessionsRequest{
		Scope:   scope,
		Filters: filters,
	}, nil
}

func normalizeSessionListFilters(filters SessionListFilters) (SessionListFilters, error) {
	normalized := SessionListFilters{
		SourceKind:      filters.SourceKind,
		SourceRef:       strings.TrimSpace(filters.SourceRef),
		ProjectBoundary: strings.TrimSpace(filters.ProjectBoundary),
		Recoverable:     filters.Recoverable,
		StaleLease:      filters.StaleLease,
		CreatedAfter:    filters.CreatedAfter,
		CreatedBefore:   filters.CreatedBefore,
		UpdatedAfter:    filters.UpdatedAfter,
		UpdatedBefore:   filters.UpdatedBefore,
	}
	if len(filters.Statuses) > 0 {
		normalized.Statuses = append([]LifecycleStatus(nil), filters.Statuses...)
	}
	if len(filters.OrchestratorKinds) > 0 {
		normalized.OrchestratorKinds = make([]string, 0, len(filters.OrchestratorKinds))
		for _, kind := range filters.OrchestratorKinds {
			trimmed := strings.TrimSpace(kind)
			if trimmed != "" {
				normalized.OrchestratorKinds = append(normalized.OrchestratorKinds, trimmed)
			}
		}
	}
	if filters.SourceKind != "" && !isKnownWorkflowSourceKind(filters.SourceKind) {
		return SessionListFilters{}, NewValidationError("filters.sourceKind", "unsupported source kind")
	}
	if err := validateTimeRange("filters.created", normalized.CreatedAfter, normalized.CreatedBefore); err != nil {
		return SessionListFilters{}, err
	}
	if err := validateTimeRange("filters.updated", normalized.UpdatedAfter, normalized.UpdatedBefore); err != nil {
		return SessionListFilters{}, err
	}
	return normalized, nil
}

func isKnownWorkflowSourceKind(kind workflowsource.WorkflowSourceKind) bool {
	switch kind {
	case workflowsource.WorkflowSourceKindFactoryID,
		workflowsource.WorkflowSourceKindFactoryInline,
		workflowsource.WorkflowSourceKindWorkflowFile,
		workflowsource.WorkflowSourceKindWorkflowName,
		workflowsource.WorkflowSourceKindInlineWorkflow:
		return true
	default:
		return false
	}
}

func factoryNameFromStart(req StartRequest, resolved ResolvedSource) string {
	if name := strings.TrimSpace(resolved.Metadata["factoryName"]); name != "" {
		return name
	}
	switch req.Source.Kind {
	case workflowsource.WorkflowSourceKindFactoryID:
		return strings.TrimSpace(req.Source.FactoryID)
	case workflowsource.WorkflowSourceKindFactoryInline:
		var declaration struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(req.Source.FactoryInline, &declaration); err == nil {
			return strings.TrimSpace(declaration.Name)
		}
	}
	if req.Source.InlineWorkflow != nil {
		if name := strings.TrimSpace(req.Source.InlineWorkflow.Metadata["factoryName"]); name != "" {
			return name
		}
	}
	return ""
}

func validateTimeRange(field string, after, before *time.Time) error {
	if after != nil && before != nil && after.After(*before) {
		return NewValidationError(field, "after must be before or equal to before")
	}
	return nil
}

// StartCanonical is the direct fake durable-owner start operation. It keeps
// canonical dispatch off the compatibility methods while preserving the fake's
// deterministic start projections.
func (s *FakeService) StartCanonical(
	ctx context.Context,
	req StartRequest,
	synchronous bool,
) (durableexecution.CanonicalStartResult, error) {
	if synchronous {
		started, err := s.startSync(ctx, req)
		if err != nil {
			return durableexecution.CanonicalStartResult{}, err
		}
		return durableexecution.CanonicalStartResult{Sync: &started}, nil
	}
	started, err := s.startAsync(ctx, req)
	if err != nil {
		return durableexecution.CanonicalStartResult{}, err
	}
	return durableexecution.CanonicalStartResult{Async: &started}, nil
}

func (s *FakeService) GetCanonical(ctx context.Context, sessionID string) (factorysessions.SessionReadResult, error) {
	return s.getSession(ctx, sessionID)
}

func (s *FakeService) ListCanonical(ctx context.Context, req factorysessions.ListSessionsRequest) (factorysessions.ListSessionsResult, error) {
	return s.listSessions(ctx, req)
}

func (s *FakeService) ControlCanonical(
	ctx context.Context,
	request factorysessions.SessionControlRequest,
) (durableexecution.CanonicalControlResult, error) {
	control := canonicalControlRequest(request)
	switch request.Operation {
	case factorysessions.SessionControlPause:
		return s.canonicalLifecycle(ctx, request.SessionID, LifecycleControlPause, control, ApproveRequest{}, RetryDispatchRequest{}, InterruptDispatchRequest{})
	case factorysessions.SessionControlResume:
		return s.canonicalLifecycle(ctx, request.SessionID, LifecycleControlResume, control, ApproveRequest{}, RetryDispatchRequest{}, InterruptDispatchRequest{})
	case factorysessions.SessionControlCancel:
		return s.canonicalLifecycle(ctx, request.SessionID, LifecycleControlCancel, control, ApproveRequest{}, RetryDispatchRequest{}, InterruptDispatchRequest{})
	case factorysessions.SessionControlTerminate:
		return s.canonicalLifecycle(ctx, request.SessionID, LifecycleControlTerminate, control, ApproveRequest{}, RetryDispatchRequest{}, InterruptDispatchRequest{})
	case factorysessions.SessionControlRecover:
		return s.canonicalRecovery(ctx, request, control)
	case factorysessions.SessionControlApprove:
		return s.canonicalApprove(ctx, request, control)
	case factorysessions.SessionControlRetryDispatch:
		return s.canonicalRetry(ctx, request, control)
	case factorysessions.SessionControlInterruptDispatch:
		return s.canonicalInterrupt(ctx, request, control)
	default:
		return durableexecution.CanonicalControlResult{}, fmt.Errorf("unsupported canonical durable control operation %q", request.Operation)
	}
}

func (s *FakeService) canonicalLifecycle(
	ctx context.Context,
	sessionID string,
	operation LifecycleControlKind,
	control ControlRequest,
	approve ApproveRequest,
	retry RetryDispatchRequest,
	interrupt InterruptDispatchRequest,
) (durableexecution.CanonicalControlResult, error) {
	result, err := s.applyLifecycleControl(ctx, sessionID, operation, control, approve, retry, interrupt)
	if err != nil {
		return durableexecution.CanonicalControlResult{}, err
	}
	return durableexecution.CanonicalControlResult{Lifecycle: &result}, nil
}

func (s *FakeService) canonicalRecovery(
	ctx context.Context,
	request factorysessions.SessionControlRequest,
	control ControlRequest,
) (durableexecution.CanonicalControlResult, error) {
	recovery := factorysessions.ResumeSessionRequest{RequestID: control.RequestID}
	if request.Recover != nil {
		recovery = *request.Recover
		if recovery.RequestID == "" {
			recovery.RequestID = control.RequestID
		}
	}
	started, err := s.resumeInterruptedSession(ctx, request.SessionID, recovery)
	if err != nil {
		return durableexecution.CanonicalControlResult{}, err
	}
	return durableexecution.CanonicalControlResult{Recovery: &started}, nil
}

func (s *FakeService) canonicalApprove(
	ctx context.Context,
	request factorysessions.SessionControlRequest,
	control ControlRequest,
) (durableexecution.CanonicalControlResult, error) {
	approve := factorysessions.ApproveRequest{ControlRequest: control}
	if request.Approve != nil {
		approve = *request.Approve
		if approve.RequestID == "" {
			approve.RequestID = control.RequestID
		}
	}
	normalized, err := NormalizeApproveRequest(approve)
	if err != nil {
		return durableexecution.CanonicalControlResult{}, err
	}
	return s.canonicalLifecycle(ctx, request.SessionID, LifecycleControlApprove, normalized.ControlRequest, normalized, RetryDispatchRequest{}, InterruptDispatchRequest{})
}

func (s *FakeService) canonicalRetry(
	ctx context.Context,
	request factorysessions.SessionControlRequest,
	control ControlRequest,
) (durableexecution.CanonicalControlResult, error) {
	retry := factorysessions.RetryDispatchRequest{ControlRequest: control}
	if request.Retry != nil {
		retry = *request.Retry
		if retry.RequestID == "" {
			retry.RequestID = control.RequestID
		}
	}
	normalized, err := NormalizeRetryDispatchRequest(retry)
	if err != nil {
		return durableexecution.CanonicalControlResult{}, err
	}
	return s.canonicalLifecycle(ctx, request.SessionID, LifecycleControlRetryDispatch, normalized.ControlRequest, ApproveRequest{}, normalized, InterruptDispatchRequest{})
}

func (s *FakeService) canonicalInterrupt(
	ctx context.Context,
	request factorysessions.SessionControlRequest,
	control ControlRequest,
) (durableexecution.CanonicalControlResult, error) {
	interrupt := factorysessions.InterruptDispatchRequest{ControlRequest: control}
	if request.Interrupt != nil {
		interrupt = *request.Interrupt
		if interrupt.RequestID == "" {
			interrupt.RequestID = control.RequestID
		}
	}
	normalized, err := NormalizeInterruptDispatchRequest(interrupt)
	if err != nil {
		return durableexecution.CanonicalControlResult{}, err
	}
	return s.canonicalLifecycle(ctx, request.SessionID, LifecycleControlInterruptDispatch, normalized.ControlRequest, ApproveRequest{}, RetryDispatchRequest{}, normalized)
}

func (s *FakeService) ReadResultCanonical(ctx context.Context, sessionID string, req factorysessions.ResultRequest) (factorysessions.ResultReadResult, error) {
	return s.getResult(ctx, sessionID, req)
}

func (s *FakeService) QueryDispatchesCanonical(ctx context.Context, request factorysessions.DispatchQueryRequest) (factorysessions.ListDispatchesResult, error) {
	result, err := s.listDispatches(ctx, request.SessionID)
	if err != nil {
		return factorysessions.ListDispatchesResult{}, err
	}
	return FilterDispatches(result, request.Filters)
}

func (s *FakeService) SubscribeResponsesCanonical(ctx context.Context, request factorysessions.ResponseEventSubscriptionRequest) (*factorysessions.ResponseEventCursor, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, factorysessions.ErrRuntimeNotAvailable
}
