package factorysession

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workflowsource "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

// DurableLifecycleAPI is the bounded lifecycle-routing seam used by the
// durable HTTP adapter. Live-session routing remains owned by the caller.
type DurableLifecycleAPI interface {
	PauseDurableFactorySession(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error)
	ResumeDurableFactorySession(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error)
	CancelDurableFactorySession(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error)
	TerminateDurableFactorySession(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error)
	ApproveDurableFactorySession(context.Context, string, factorysessionexecution.ApproveRequest) (factorysessionexecution.LifecycleControlResult, error)
	RetryDurableFactorySessionDispatch(context.Context, string, factorysessionexecution.RetryDispatchRequest) (factorysessionexecution.LifecycleControlResult, error)
	InterruptDurableFactorySessionDispatch(context.Context, string, factorysessionexecution.InterruptDispatchRequest) (factorysessionexecution.LifecycleControlResult, error)
	ReadDurableFactorySessionEventStream(context.Context, string, factorysessionexecution.EventReconnectRequest) (*interfaces.FactoryEventStream, error)
	ProbeDurableFactorySessionEvents(context.Context, string, factorysessionexecution.EventReconnectRequest) error
}

// DurableAPI owns public durable-session request/result translation over the
// execution service and the bounded lifecycle router. Compatibility facades
// and composed transports both delegate to this adapter.
type DurableAPI struct {
	execution DurableExecution
	lifecycle DurableLifecycleAPI
}

// NewDurableAPI composes the canonical durable HTTP collaborator.
func NewDurableAPI(
	execution DurableExecution,
	lifecycle DurableLifecycleAPI,
) *DurableAPI {
	return &DurableAPI{execution: execution, lifecycle: lifecycle}
}

func (api *DurableAPI) executionService() (DurableExecution, error) {
	if api == nil || api.execution == nil {
		return nil, factorysessionexecution.ErrExecutionServiceNotConfigured
	}
	return api.execution, nil
}

func (api *DurableAPI) StartDurableFactorySessionAsync(ctx context.Context, startReq factorysessionexecution.StartRequest) (factoryapi.FactorySessionExecutionResponse, error) {
	execution, err := api.executionService()
	if err != nil {
		return factoryapi.FactorySessionExecutionResponse{}, err
	}
	result, err := execution.StartAsync(ctx, startReq)
	if err != nil {
		return factoryapi.FactorySessionExecutionResponse{}, err
	}
	return AsyncStartResponseToAPI(result), nil
}

func (api *DurableAPI) StartDurableFactorySessionSync(ctx context.Context, startReq factorysessionexecution.StartRequest) (factoryapi.FactorySessionSyncExecutionResponse, error) {
	execution, err := api.executionService()
	if err != nil {
		return factoryapi.FactorySessionSyncExecutionResponse{}, err
	}
	result, err := execution.StartSync(ctx, startReq)
	if err != nil {
		return factoryapi.FactorySessionSyncExecutionResponse{}, err
	}
	return SyncStartResponseToAPI(result), nil
}

func (api *DurableAPI) ListDurableFactorySessions(ctx context.Context, req factorysessionexecution.ListSessionsRequest) (factoryapi.ListFactorySessionsResponse, error) {
	execution, err := api.executionService()
	if err != nil {
		return factoryapi.ListFactorySessionsResponse{}, err
	}
	result, err := execution.ListSessions(ctx, req)
	if err != nil {
		return factoryapi.ListFactorySessionsResponse{}, err
	}
	return ListSessionsResponseToAPI(result), nil
}

func (api *DurableAPI) GetDurableFactorySession(ctx context.Context, sessionID string) (factoryapi.FactorySessionDurableReadModel, error) {
	execution, err := api.executionService()
	if err != nil {
		return factoryapi.FactorySessionDurableReadModel{}, err
	}
	result, err := execution.GetSession(ctx, sessionID)
	if err != nil {
		return factoryapi.FactorySessionDurableReadModel{}, err
	}
	return SessionReadResponseToAPI(result), nil
}

func (api *DurableAPI) GetDurableFactorySessionResult(ctx context.Context, sessionID string, req factorysessionexecution.ResultRequest) (factoryapi.FactorySessionResult, error) {
	execution, err := api.executionService()
	if err != nil {
		return factoryapi.FactorySessionResult{}, err
	}
	result, err := execution.GetResult(ctx, sessionID, req)
	if err != nil {
		return factoryapi.FactorySessionResult{}, err
	}
	return ResultResponseToAPI(result), nil
}

func (api *DurableAPI) ReadDurableFactorySessionEvents(ctx context.Context, sessionID string, reconnect factorysessionexecution.EventReconnectRequest) (*interfaces.FactoryEventStream, error) {
	if api == nil || api.lifecycle == nil {
		return nil, factorysessionexecution.ErrExecutionServiceNotConfigured
	}
	stream, err := api.lifecycle.ReadDurableFactorySessionEventStream(ctx, sessionID, reconnect)
	if err != nil {
		if errors.Is(err, factorysessionexecution.ErrDurableSessionNotFound) {
			return nil, apisurface.ErrFactorySessionNotFound
		}
		if errors.Is(err, factorysessionexecution.ErrReconnectCursorNotFound) {
			return nil, fmt.Errorf("%w: %v", apisurface.ErrInvalidEventReconnectCursor, err)
		}
		return nil, err
	}
	return stream, nil
}

func (api *DurableAPI) SubscribeDurableFactoryResponseEvents(
	ctx context.Context,
	request factorysessionexecution.ResponseEventSubscriptionRequest,
) (apisurface.FactoryResponseEventSubscription, error) {
	if subscriber, ok := api.execution.(interface {
		SubscribeDurableFactoryResponseEvents(context.Context, factorysessionexecution.ResponseEventSubscriptionRequest) (*factorysessionexecution.ResponseEventCursor, error)
	}); ok {
		cursor, err := subscriber.SubscribeDurableFactoryResponseEvents(ctx, request)
		if err != nil {
			if errors.Is(err, factorysessionexecution.ErrSessionNotFound) {
				return nil, apisurface.ErrFactorySessionNotFound
			}
			if errors.Is(err, factorysessionexecution.ErrResponseEventStoreExpired) {
				return nil, fmt.Errorf("%w: %s", apisurface.ErrFactoryResponseEventStreamExpired, request.SessionID)
			}
			return nil, err
		}
		return NewResponseEventSubscription(cursor), nil
	}
	execution, err := api.executionService()
	if err != nil {
		return nil, err
	}
	direct, ok := execution.(interface {
		SubscribeResponseEvents(context.Context, string, factorysessionexecution.ResponseEventSubscriptionRequest) (*factorysessionexecution.ResponseEventCursor, error)
	})
	if !ok {
		return nil, factorysessionexecution.ErrRuntimeNotAvailable
	}
	cursor, err := direct.SubscribeResponseEvents(ctx, request.SessionID, request)
	if err != nil {
		if errors.Is(err, factorysessionexecution.ErrSessionNotFound) {
			return nil, apisurface.ErrFactorySessionNotFound
		}
		if errors.Is(err, factorysessionexecution.ErrResponseEventStoreExpired) {
			return nil, fmt.Errorf("%w: %s", apisurface.ErrFactoryResponseEventStreamExpired, request.SessionID)
		}
		return nil, err
	}
	return NewResponseEventSubscription(cursor), nil
}

func (api *DurableAPI) ProbeDurableFactorySessionEvents(ctx context.Context, sessionID string, reconnect factorysessionexecution.EventReconnectRequest) error {
	if api == nil || api.lifecycle == nil {
		return factorysessionexecution.ErrExecutionServiceNotConfigured
	}
	err := api.lifecycle.ProbeDurableFactorySessionEvents(ctx, sessionID, reconnect)
	if errors.Is(err, factorysessionexecution.ErrDurableSessionNotFound) {
		return apisurface.ErrFactorySessionNotFound
	}
	if errors.Is(err, factorysessionexecution.ErrReconnectCursorNotFound) {
		return fmt.Errorf("%w: %v", apisurface.ErrInvalidEventReconnectCursor, err)
	}
	return err
}

func (api *DurableAPI) ListDurableFactorySessionDispatches(ctx context.Context, sessionID string, params factoryapi.ListFactorySessionDispatchesParams) (factoryapi.ListFactorySessionDispatchesResponse, error) {
	execution, err := api.executionService()
	if err != nil {
		return factoryapi.ListFactorySessionDispatchesResponse{}, err
	}
	filters := factorysessionexecution.DispatchFilters{}
	if params.Phase != nil {
		filters.Phase = string(*params.Phase)
	}
	if params.Status != nil {
		filters.Status = factorysessionexecution.DispatchStatus(*params.Status)
	}
	result, err := execution.QueryDispatches(ctx, factorysessionexecution.DispatchQueryRequest{
		SessionID: sessionID, Filters: filters,
	})
	if err != nil {
		return factoryapi.ListFactorySessionDispatchesResponse{}, err
	}
	return ListDispatchesResponseToAPI(result), nil
}

func (api *DurableAPI) GetDurableFactorySessionDispatch(ctx context.Context, sessionID, dispatchID string) (factoryapi.FactoryDispatch, error) {
	execution, err := api.executionService()
	if err != nil {
		return factoryapi.FactoryDispatch{}, err
	}
	result, err := execution.GetDispatch(ctx, sessionID, dispatchID)
	if err != nil {
		return factoryapi.FactoryDispatch{}, err
	}
	return DispatchDetailResponseToAPI(result), nil
}

func (api *DurableAPI) ListDurableFactorySessionArtifacts(ctx context.Context, sessionID string) (factoryapi.ListFactorySessionArtifactsResponse, error) {
	execution, err := api.executionService()
	if err != nil {
		return factoryapi.ListFactorySessionArtifactsResponse{}, err
	}
	result, err := execution.ListArtifacts(ctx, sessionID)
	if err != nil {
		return factoryapi.ListFactorySessionArtifactsResponse{}, err
	}
	return ListArtifactsResponseToAPI(result), nil
}

func (api *DurableAPI) GetDurableFactorySessionArtifact(ctx context.Context, sessionID, artifactID string) (factoryapi.FactorySessionArtifactDetail, error) {
	execution, err := api.executionService()
	if err != nil {
		return factoryapi.FactorySessionArtifactDetail{}, err
	}
	result, err := execution.GetArtifact(ctx, sessionID, artifactID)
	if err != nil {
		return factoryapi.FactorySessionArtifactDetail{}, err
	}
	return ArtifactDetailResponseToAPI(result), nil
}

func (api *DurableAPI) PauseDurableFactorySession(ctx context.Context, sessionID string, control factorysessionexecution.ControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	result, err := api.lifecycle.PauseDurableFactorySession(ctx, sessionID, control)
	return lifecycleResultToAPI(result, err)
}

func (api *DurableAPI) ResumeDurableFactorySession(ctx context.Context, sessionID string, control factorysessionexecution.ControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	result, err := api.lifecycle.ResumeDurableFactorySession(ctx, sessionID, control)
	return lifecycleResultToAPI(result, err)
}

func (api *DurableAPI) CancelDurableFactorySession(ctx context.Context, sessionID string, control factorysessionexecution.ControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	result, err := api.lifecycle.CancelDurableFactorySession(ctx, sessionID, control)
	return lifecycleResultToAPI(result, err)
}

func (api *DurableAPI) TerminateDurableFactorySession(ctx context.Context, sessionID string, control factorysessionexecution.ControlRequest) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	result, err := api.lifecycle.TerminateDurableFactorySession(ctx, sessionID, control)
	return lifecycleResultToAPI(result, err)
}

func (api *DurableAPI) ApproveDurableFactorySession(ctx context.Context, sessionID string, approve factorysessionexecution.ApproveRequest) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	result, err := api.lifecycle.ApproveDurableFactorySession(ctx, sessionID, approve)
	return lifecycleResultToAPI(result, err)
}

func (api *DurableAPI) RetryDurableFactorySessionDispatch(ctx context.Context, sessionID string, retry factorysessionexecution.RetryDispatchRequest) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	result, err := api.lifecycle.RetryDurableFactorySessionDispatch(ctx, sessionID, retry)
	return lifecycleResultToAPI(result, err)
}

func (api *DurableAPI) InterruptDurableFactorySessionDispatch(ctx context.Context, sessionID string, interrupt factorysessionexecution.InterruptDispatchRequest) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	result, err := api.lifecycle.InterruptDurableFactorySessionDispatch(ctx, sessionID, interrupt)
	return lifecycleResultToAPI(result, err)
}

func lifecycleResultToAPI(result factorysessionexecution.LifecycleControlResult, err error) (factoryapi.FactorySessionLifecycleControlResponse, error) {
	if err != nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}, err
	}
	return LifecycleControlResponseToAPI(result), nil
}

// StartRequestFromAPI maps one public durable execution request into the shared
// service contract.
func StartRequestFromAPI(
	req factoryapi.FactorySessionExecutionRequest,
) (factorysessionexecution.StartRequest, error) {
	source, err := executionSourceFromAPI(req.Source)
	if err != nil {
		return factorysessionexecution.StartRequest{}, err
	}

	startReq := factorysessionexecution.StartRequest{
		RequestID: req.RequestId,
		Source:    source,
	}
	if req.Args != nil {
		startReq.Args = cloneAnyMap(*req.Args)
	}
	if req.Orchestrator != nil {
		encoded, marshalErr := json.Marshal(req.Orchestrator)
		if marshalErr != nil {
			return factorysessionexecution.StartRequest{}, &apisurface.RequestValidationError{Message: "orchestrator must be a JSON object"}
		}
		startReq.Orchestrator = &factorysessionexecution.OrchestratorOverride{
			Kind: string(req.Orchestrator.Kind),
			Raw:  encoded,
		}
	}
	if req.RequestedPolicy != nil {
		startReq.RequestedPolicy = policyMapFromAPI(*req.RequestedPolicy)
	}
	if req.Wait != nil {
		cancelOnTimeout := req.Wait.CancelOnTimeout != nil && *req.Wait.CancelOnTimeout
		startReq.Wait = &factorysessionexecution.WaitOptions{
			TimeoutMillis:   req.Wait.TimeoutMillis,
			CancelOnTimeout: cancelOnTimeout,
		}
	}
	return startReq, nil
}

// AsyncStartResponseToAPI maps one async service result to the public response shape.
func AsyncStartResponseToAPI(result factorysessionexecution.AsyncStartResult) factoryapi.FactorySessionExecutionResponse {
	response := factoryapi.FactorySessionExecutionResponse{
		SessionId:        result.SessionID,
		Status:           factoryapi.FactorySessionDurableLifecycleStatus(result.Status),
		OrchestratorKind: factoryapi.FactoryOrchestratorKind(interfaces.StrictPublicFactoryOrchestratorKind(result.OrchestratorKind)),
		ResolvedSource:   resolvedSourceToAPI(result.ResolvedSource),
	}
	if dialect := strings.TrimSpace(result.Dialect); dialect != "" {
		response.Dialect = &dialect
	}
	if sourceHash := strings.TrimSpace(result.SourceHash); sourceHash != "" {
		response.SourceHash = &sourceHash
	}
	if requested := policyToAPI(result.Policy.Requested); requested != nil {
		response.RequestedPolicy = requested
	}
	if effective := effectivePolicyToAPI(result.Policy.Effective); effective != nil {
		response.EffectivePolicy = effective
	}
	if effectiveHash := strings.TrimSpace(result.Policy.EffectiveHash); effectiveHash != "" {
		response.EffectivePolicyHash = &effectiveHash
	}
	if links := executionLinksToAPI(result.Links); links != nil {
		response.Links = links
	}
	return response
}

// SyncStartResponseToAPI maps one sync service result to the public response shape.
func SyncStartResponseToAPI(result factorysessionexecution.SyncStartResult) factoryapi.FactorySessionSyncExecutionResponse {
	async := AsyncStartResponseToAPI(result.AsyncStartResult)
	response := factoryapi.FactorySessionSyncExecutionResponse{
		SessionId:           async.SessionId,
		Status:              async.Status,
		OrchestratorKind:    async.OrchestratorKind,
		ResolvedSource:      async.ResolvedSource,
		Dialect:             async.Dialect,
		SourceHash:          async.SourceHash,
		RequestedPolicy:     async.RequestedPolicy,
		EffectivePolicy:     async.EffectivePolicy,
		EffectivePolicyHash: async.EffectivePolicyHash,
		Links:               async.Links,
		SyncOutcome:         factoryapi.FactorySessionSyncExecutionOutcome(result.SyncOutcome),
	}
	if result.TimedOut {
		timedOut := true
		response.TimedOut = &timedOut
	}
	if result.SessionCanceledByTimeout {
		canceled := true
		response.SessionCanceledByTimeout = &canceled
	}
	if len(result.Result) > 0 {
		var sessionResult factoryapi.FactorySessionResult
		if err := json.Unmarshal(result.Result, &sessionResult); err == nil {
			response.Result = &sessionResult
		}
	}
	return response
}

func executionSourceFromAPI(source factoryapi.FactorySessionExecutionSource) (factorysessionexecution.Source, error) {
	kind, err := executionSourceKindFromAPI(source.Kind)
	if err != nil {
		return factorysessionexecution.Source{}, err
	}
	switch kind {
	case workflowsource.WorkflowSourceKindFactoryID:
		return factorysessionexecution.Source{
			Kind:      kind,
			FactoryID: derefString(source.FactoryId),
		}, nil
	case workflowsource.WorkflowSourceKindFactoryInline:
		if source.FactoryInline == nil {
			return factorysessionexecution.Source{}, &apisurface.RequestValidationError{Message: "source.factoryInline is required when source.kind is FACTORY_INLINE"}
		}
		encoded, marshalErr := json.Marshal(source.FactoryInline)
		if marshalErr != nil {
			return factorysessionexecution.Source{}, &apisurface.RequestValidationError{Message: "source.factoryInline must be a JSON object"}
		}
		return factorysessionexecution.Source{
			Kind:          kind,
			FactoryInline: encoded,
		}, nil
	case workflowsource.WorkflowSourceKindWorkflowFile:
		return factorysessionexecution.Source{
			Kind:         kind,
			WorkflowFile: derefString(source.WorkflowFile),
		}, nil
	case workflowsource.WorkflowSourceKindWorkflowName:
		return factorysessionexecution.Source{
			Kind:         kind,
			WorkflowName: derefString(source.WorkflowName),
		}, nil
	case workflowsource.WorkflowSourceKindInlineWorkflow:
		if source.InlineWorkflow == nil {
			return factorysessionexecution.Source{}, &apisurface.RequestValidationError{Message: "source.inlineWorkflow is required when source.kind is INLINE_WORKFLOW"}
		}
		inline := &factorysessionexecution.InlineWorkflowSource{InlineSource: source.InlineWorkflow.InlineSource.Inline}
		if source.InlineWorkflow.Dialect != nil {
			inline.Dialect = *source.InlineWorkflow.Dialect
		}
		if source.InlineWorkflow.Entrypoint != nil {
			inline.Entrypoint = *source.InlineWorkflow.Entrypoint
		}
		if source.InlineWorkflow.Metadata != nil {
			inline.Metadata = cloneStringMap(*source.InlineWorkflow.Metadata)
		}
		return factorysessionexecution.Source{
			Kind:           kind,
			InlineWorkflow: inline,
		}, nil
	default:
		return factorysessionexecution.Source{}, &apisurface.RequestValidationError{Message: "source.kind is invalid"}
	}
}

func executionSourceKindFromAPI(kind factoryapi.FactorySessionExecutionSourceKind) (workflowsource.WorkflowSourceKind, error) {
	switch workflowsource.WorkflowSourceKind(strings.TrimSpace(string(kind))) {
	case workflowsource.WorkflowSourceKindFactoryID,
		workflowsource.WorkflowSourceKindFactoryInline,
		workflowsource.WorkflowSourceKindWorkflowFile,
		workflowsource.WorkflowSourceKindWorkflowName,
		workflowsource.WorkflowSourceKindInlineWorkflow:
		return workflowsource.WorkflowSourceKind(strings.TrimSpace(string(kind))), nil
	default:
		return "", &apisurface.RequestValidationError{Message: "source.kind must be one of FACTORY_ID, FACTORY_INLINE, WORKFLOW_FILE, WORKFLOW_NAME, or INLINE_WORKFLOW"}
	}
}

func resolvedSourceToAPI(source factorysessionexecution.ResolvedSource) factoryapi.FactorySessionResolvedSourceIdentity {
	response := factoryapi.FactorySessionResolvedSourceIdentity{
		Kind: factoryapi.FactorySessionExecutionSourceKind(source.Kind),
	}
	if dialect := strings.TrimSpace(source.Dialect); dialect != "" {
		response.Dialect = &dialect
	}
	if sourceRef := strings.TrimSpace(source.SourceRef); sourceRef != "" {
		response.SourceRef = &sourceRef
	}
	if sourceHash := strings.TrimSpace(source.SourceHash); sourceHash != "" {
		response.SourceHash = &sourceHash
	}
	if len(source.ResolutionOrder) > 0 {
		order := make([]factoryapi.FactorySessionWorkflowSourceResolutionOrder, 0, len(source.ResolutionOrder))
		for _, stage := range source.ResolutionOrder {
			order = append(order, factoryapi.FactorySessionWorkflowSourceResolutionOrder(stage))
		}
		response.ResolutionOrder = &order
	}
	if len(source.Metadata) > 0 {
		metadata := factoryapi.StringMap(cloneStringMap(source.Metadata))
		response.Metadata = &metadata
	}
	return response
}

func executionLinksToAPI(links factorysessionexecution.InspectionLinks) *factoryapi.FactorySessionExecutionLinks {
	if links == (factorysessionexecution.InspectionLinks{}) {
		return nil
	}
	response := &factoryapi.FactorySessionExecutionLinks{}
	if value := strings.TrimSpace(links.Session); value != "" {
		response.Session = &value
	}
	if value := strings.TrimSpace(links.Status); value != "" {
		response.Status = &value
	}
	if value := strings.TrimSpace(links.Events); value != "" {
		response.Events = &value
	}
	if value := strings.TrimSpace(links.Results); value != "" {
		response.Results = &value
	}
	return response
}

func policyToAPI(policy map[string]any) *factoryapi.FactorySessionRequestedPolicy {
	if len(policy) == 0 {
		return nil
	}
	out := &factoryapi.FactorySessionRequestedPolicy{
		AdditionalProperties: cloneAnyMap(policy),
	}
	if hash, ok := policy["policyHash"].(string); ok {
		if trimmed := strings.TrimSpace(hash); trimmed != "" {
			out.PolicyHash = &trimmed
			delete(out.AdditionalProperties, "policyHash")
		}
	}
	if len(out.AdditionalProperties) == 0 {
		out.AdditionalProperties = nil
	}
	return out
}

func effectivePolicyToAPI(policy map[string]any) *factoryapi.FactorySessionEffectivePolicy {
	if len(policy) == 0 {
		return nil
	}
	out := &factoryapi.FactorySessionEffectivePolicy{
		AdditionalProperties: cloneAnyMap(policy),
	}
	if hash, ok := policy["policyHash"].(string); ok {
		if trimmed := strings.TrimSpace(hash); trimmed != "" {
			out.PolicyHash = &trimmed
			delete(out.AdditionalProperties, "policyHash")
		}
	}
	if len(out.AdditionalProperties) == 0 {
		out.AdditionalProperties = nil
	}
	return out
}

func policyMapFromAPI(policy factoryapi.FactorySessionRequestedPolicy) map[string]any {
	out := cloneAnyMap(policy.AdditionalProperties)
	if policy.PolicyHash != nil {
		if trimmed := strings.TrimSpace(*policy.PolicyHash); trimmed != "" {
			if out == nil {
				out = make(map[string]any)
			}
			out["policyHash"] = trimmed
		}
	}
	return out
}

func cloneAnyMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(values))
	for key, value := range values {
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
