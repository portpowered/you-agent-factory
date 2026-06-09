package apisurface

import (
	"encoding/json"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workflowsource"
)

// StartRequestFromAPI maps one public durable execution request into the shared
// service contract.
func StartRequestFromAPI(req factoryapi.FactorySessionExecutionRequest) (factorysessionexecution.StartRequest, error) {
	source, err := executionSourceFromAPI(req.Source)
	if err != nil {
		return factorysessionexecution.StartRequest{}, err
	}

	startReq := factorysessionexecution.StartRequest{
		RequestID: strings.TrimSpace(req.RequestId),
		Source:    source,
	}
	if req.Args != nil {
		startReq.Args = cloneAnyMap(*req.Args)
	}
	if req.Orchestrator != nil {
		encoded, marshalErr := json.Marshal(req.Orchestrator)
		if marshalErr != nil {
			return factorysessionexecution.StartRequest{}, &RequestValidationError{Message: "orchestrator must be a JSON object"}
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
	return factorysessionexecution.NormalizeStartRequest(startReq)
}

// AsyncStartResponseToAPI maps one async service result to the public response shape.
func AsyncStartResponseToAPI(result factorysessionexecution.AsyncStartResult) factoryapi.FactorySessionExecutionResponse {
	response := factoryapi.FactorySessionExecutionResponse{
		SessionId:        result.SessionID,
		Status:           factoryapi.FactorySessionDurableLifecycleStatus(result.Status),
		OrchestratorKind: interfaces.GeneratedPublicFactoryOrchestratorKind(result.OrchestratorKind),
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
		SessionId:        async.SessionId,
		Status:           async.Status,
		OrchestratorKind: async.OrchestratorKind,
		ResolvedSource:   async.ResolvedSource,
		Dialect:          async.Dialect,
		SourceHash:       async.SourceHash,
		RequestedPolicy:  async.RequestedPolicy,
		EffectivePolicy:  async.EffectivePolicy,
		EffectivePolicyHash: async.EffectivePolicyHash,
		Links:            async.Links,
		SyncOutcome:      factoryapi.FactorySessionSyncExecutionOutcome(result.SyncOutcome),
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
	case workflowsource.KindFactoryID:
		return factorysessionexecution.Source{
			Kind:      kind,
			FactoryID: derefString(source.FactoryId),
		}, nil
	case workflowsource.KindFactoryInline:
		if source.FactoryInline == nil {
			return factorysessionexecution.Source{}, &RequestValidationError{Message: "source.factoryInline is required when source.kind is FACTORY_INLINE"}
		}
		encoded, marshalErr := json.Marshal(source.FactoryInline)
		if marshalErr != nil {
			return factorysessionexecution.Source{}, &RequestValidationError{Message: "source.factoryInline must be a JSON object"}
		}
		return factorysessionexecution.Source{
			Kind:          kind,
			FactoryInline: encoded,
		}, nil
	case workflowsource.KindWorkflowFile:
		return factorysessionexecution.Source{
			Kind:         kind,
			WorkflowFile: derefString(source.WorkflowFile),
		}, nil
	case workflowsource.KindWorkflowName:
		return factorysessionexecution.Source{
			Kind:         kind,
			WorkflowName: derefString(source.WorkflowName),
		}, nil
	case workflowsource.KindInlineWorkflow:
		if source.InlineWorkflow == nil {
			return factorysessionexecution.Source{}, &RequestValidationError{Message: "source.inlineWorkflow is required when source.kind is INLINE_WORKFLOW"}
		}
		inline := &factorysessionexecution.InlineWorkflowSource{
			InlineSource: strings.TrimSpace(source.InlineWorkflow.InlineSource.Inline),
		}
		if source.InlineWorkflow.Dialect != nil {
			inline.Dialect = strings.TrimSpace(*source.InlineWorkflow.Dialect)
		}
		if source.InlineWorkflow.Entrypoint != nil {
			inline.Entrypoint = strings.TrimSpace(*source.InlineWorkflow.Entrypoint)
		}
		if source.InlineWorkflow.Metadata != nil {
			inline.Metadata = cloneStringMap(*source.InlineWorkflow.Metadata)
		}
		return factorysessionexecution.Source{
			Kind:           kind,
			InlineWorkflow: inline,
		}, nil
	default:
		return factorysessionexecution.Source{}, &RequestValidationError{Message: "source.kind is invalid"}
	}
}

func executionSourceKindFromAPI(kind factoryapi.FactorySessionExecutionSourceKind) (workflowsource.Kind, error) {
	switch workflowsource.Kind(strings.TrimSpace(string(kind))) {
	case workflowsource.KindFactoryID,
		workflowsource.KindFactoryInline,
		workflowsource.KindWorkflowFile,
		workflowsource.KindWorkflowName,
		workflowsource.KindInlineWorkflow:
		return workflowsource.Kind(strings.TrimSpace(string(kind))), nil
	default:
		return "", &RequestValidationError{Message: "source.kind must be one of FACTORY_ID, FACTORY_INLINE, WORKFLOW_FILE, WORKFLOW_NAME, or INLINE_WORKFLOW"}
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
