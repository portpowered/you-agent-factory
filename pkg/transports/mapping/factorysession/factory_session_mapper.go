package factorysession

import (
	"encoding/json"
	"strings"
	"time"

	workflowsource "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	contentcontract "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
)

// InvocationRequestFromAPI maps the public invocation carrier into the Factory
// Session-owned request before domain normalization and submission.
func InvocationRequestFromAPI(request factoryapi.InvocationRequest) factorysessionexecution.InvocationRequest {
	result := factorysessionexecution.InvocationRequest{
		Content:         contentcontract.PartsFromGenerated(request.Content),
		ContentProvided: request.Content != nil,
	}
	if request.Args != nil {
		args := cloneAnyMap(*request.Args)
		result.Args = &args
	}
	if request.RequestId != nil {
		requestID := *request.RequestId
		result.RequestID = &requestID
	}
	if request.SourceKind != nil {
		sourceKind := factorysessionexecution.InvocationInputSourceKind(*request.SourceKind)
		result.SourceKind = &sourceKind
	}
	if request.TimeoutMillis != nil {
		timeoutMillis := *request.TimeoutMillis
		result.TimeoutMillis = &timeoutMillis
	}
	return result
}

// OrchestratorOverrideFromAPI maps one public orchestrator override into the shared
// service contract.
func OrchestratorOverrideFromAPI(orchestrator factoryapi.FactoryOrchestrator) (*factorysessionexecution.OrchestratorOverride, error) {
	encoded, err := json.Marshal(orchestrator)
	if err != nil {
		return nil, &apisurface.RequestValidationError{Message: "orchestrator must be a JSON object"}
	}
	return &factorysessionexecution.OrchestratorOverride{
		Kind: string(orchestrator.Kind),
		Raw:  encoded,
	}, nil
}

// AsyncStartResultFromAPI maps one public async execution response into the shared
// service contract.
func AsyncStartResultFromAPI(response factoryapi.FactorySessionExecutionResponse) factorysessionexecution.AsyncStartResult {
	result := factorysessionexecution.AsyncStartResult{
		SessionID:        response.SessionId,
		Status:           string(response.Status),
		OrchestratorKind: string(response.OrchestratorKind),
		ResolvedSource:   resolvedSourceFromAPI(response.ResolvedSource),
	}
	if response.Dialect != nil {
		result.Dialect = strings.TrimSpace(*response.Dialect)
	}
	if response.SourceHash != nil {
		result.SourceHash = strings.TrimSpace(*response.SourceHash)
	}
	if response.RequestedPolicy != nil {
		result.Policy.Requested = policyMapFromAPI(*response.RequestedPolicy)
	}
	if response.EffectivePolicy != nil {
		result.Policy.Effective = effectivePolicyMapFromAPI(*response.EffectivePolicy)
	}
	if response.EffectivePolicyHash != nil {
		result.Policy.EffectiveHash = strings.TrimSpace(*response.EffectivePolicyHash)
	}
	if response.Links != nil {
		result.Links = executionLinksFromAPI(*response.Links)
	}
	return result
}

// SessionReadResultFromAPI maps one public durable session read model into the shared
// service contract.
//
// pkgmaintcheck:ignore-cyclomatic-complexity this inverse mapper keeps durable session read fields together for API round-trip coverage.
func SessionReadResultFromAPI(response factoryapi.FactorySessionDurableReadModel) factorysessionexecution.SessionReadResult {
	result := factorysessionexecution.SessionReadResult{
		SessionID:        response.SessionId,
		Status:           factorysessionexecution.LifecycleStatus(response.Status),
		OrchestratorKind: string(response.OrchestratorKind),
		ResolvedSource:   resolvedSourceFromAPI(response.ResolvedSource),
	}
	if response.Dialect != nil {
		result.Dialect = strings.TrimSpace(*response.Dialect)
	}
	if response.SourceHash != nil {
		result.SourceHash = strings.TrimSpace(*response.SourceHash)
	}
	if response.RequestedPolicy != nil {
		result.Policy.Requested = policyMapFromAPI(*response.RequestedPolicy)
	}
	if response.EffectivePolicy != nil {
		result.Policy.Effective = effectivePolicyMapFromAPI(*response.EffectivePolicy)
	}
	if response.EffectivePolicyHash != nil {
		result.Policy.EffectiveHash = strings.TrimSpace(*response.EffectivePolicyHash)
	}
	if response.Phase != nil {
		result.Phase = strings.TrimSpace(*response.Phase)
	}
	if response.PhaseSummaries != nil {
		result.PhaseSummaries = phaseSummariesFromAPI(*response.PhaseSummaries)
	}
	if response.LatestCheckpoint != nil {
		result.LatestCheckpoint = &factorysessionexecution.CheckpointRef{ID: response.LatestCheckpoint.Id}
		if response.LatestCheckpoint.Label != nil {
			result.LatestCheckpoint.Label = strings.TrimSpace(*response.LatestCheckpoint.Label)
		}
		if response.LatestCheckpoint.Phase != nil {
			result.LatestCheckpoint.Phase = strings.TrimSpace(*response.LatestCheckpoint.Phase)
		}
	}
	if response.Progress != nil {
		result.Progress = progressCountsFromAPI(*response.Progress)
	}
	result.Budgets = sessionBudgetsFromAPI(response.Budgets)
	result.Usage = sessionUsageFromAPI(response.Usage)
	if response.ResultSummary != nil {
		result.ResultSummary = &factorysessionexecution.ResultSummary{
			ResultStatus: string(response.ResultSummary.ResultStatus),
		}
		if response.ResultSummary.Summary != nil {
			result.ResultSummary.Summary = strings.TrimSpace(*response.ResultSummary.Summary)
		}
	}
	if response.ArtifactRefs != nil {
		result.ArtifactRefs = artifactRefsFromAPI(*response.ArtifactRefs)
	}
	if response.FailureDetail != nil {
		result.Failure = failureSummaryFromAPI(*response.FailureDetail)
		if response.PartialResultAvailable != nil {
			result.Failure.PartialResultAvailable = *response.PartialResultAvailable
		}
	}
	if response.Lifecycle != nil {
		result.Lifecycle = lifecycleTimestampsFromAPI(*response.Lifecycle)
	}
	if response.StaleLease != nil && *response.StaleLease {
		result.StaleLease = true
	}
	if response.Links != nil {
		result.Links = executionLinksFromAPI(*response.Links)
	}
	return result
}

// ResultReadResultFromAPI maps one public durable result read model into the shared
// service contract.
func ResultReadResultFromAPI(response factoryapi.FactorySessionResult) factorysessionexecution.ResultReadResult {
	result := factorysessionexecution.ResultReadResult{
		SessionID:    response.SessionId,
		ResultStatus: factorysessionexecution.ResultStatus(response.ResultStatus),
	}
	if response.SessionStatus != nil {
		result.SessionStatus = factorysessionexecution.LifecycleStatus(*response.SessionStatus)
	}
	if response.Mode != nil {
		result.Mode = factorysessionexecution.ResultMode(*response.Mode)
	}
	if response.IncludeArtifacts != nil {
		result.IncludeArtifacts = *response.IncludeArtifacts
	}
	if response.PrimaryResult != nil {
		if encoded, err := json.Marshal(response.PrimaryResult); err == nil {
			result.PrimaryResult = encoded
		}
	}
	if response.ArtifactIds != nil {
		result.ArtifactIDs = append([]string(nil), *response.ArtifactIds...)
	}
	if response.ArtifactRefs != nil {
		result.ArtifactRefs = artifactRefsFromAPI(*response.ArtifactRefs)
	}
	if response.FailureDetail != nil {
		result.Failure = failureSummaryFromAPI(*response.FailureDetail)
		if response.PartialResultAvailable != nil {
			result.Failure.PartialResultAvailable = *response.PartialResultAvailable
		}
	}
	if response.Availability != nil {
		result.Availability = resultAvailabilityFromAPI(*response.Availability)
	}
	return result
}

// DispatchSummaryFromAPI maps one public dispatch summary into the shared service contract.
func DispatchSummaryFromAPI(response factoryapi.FactorySessionDispatchSummary) factorysessionexecution.DispatchSummary {
	summary := factorysessionexecution.DispatchSummary{
		ID:           response.Id,
		Status:       factorysessionexecution.DispatchStatus(response.Status),
		DispatchKind: string(response.DispatchKind),
	}
	if response.Phase != nil {
		summary.Phase = strings.TrimSpace(*response.Phase)
	}
	if response.Label != nil {
		summary.Label = strings.TrimSpace(*response.Label)
	}
	if response.Attempt != nil {
		summary.Attempt = int(*response.Attempt)
	}
	if response.RunnerId != nil {
		summary.RunnerID = strings.TrimSpace(*response.RunnerId)
	}
	if response.PresetId != nil {
		summary.PresetID = strings.TrimSpace(*response.PresetId)
	}
	if response.ModelProvider != nil {
		summary.ModelProvider = strings.TrimSpace(*response.ModelProvider)
	}
	if response.Model != nil {
		summary.Model = strings.TrimSpace(*response.Model)
	}
	if response.ReasoningEffort != nil {
		summary.ReasoningEffort = strings.TrimSpace(*response.ReasoningEffort)
	}
	if response.Provider != nil {
		summary.Provider = strings.TrimSpace(*response.Provider)
	}
	summary.ProviderSessionRefs = providerSessionRefsFromAPI(response.ProviderSessionRefs)
	if response.OutputArtifactIds != nil {
		summary.OutputArtifactIDs = append([]string(nil), *response.OutputArtifactIds...)
	}
	if response.Usage != nil {
		summary.Usage = dispatchUsageFromAPI(*response.Usage)
	}
	if response.Warnings != nil {
		summary.Warnings = dispatchWarningsFromAPI(*response.Warnings)
	}
	if response.FailureDetail != nil {
		summary.FailureDetail = dispatchFailureFromAPI(*response.FailureDetail)
	}
	if response.Javascript != nil {
		summary.JavaScript = dispatchJavaScriptFromAPI(*response.Javascript)
	}
	return summary
}

// DispatchDetailFromAPI maps one public dispatch detail into the shared service contract.
//
// pkgmaintcheck:ignore-cyclomatic-complexity this inverse mapper keeps dispatch detail fields together for API round-trip coverage.
func DispatchDetailFromAPI(response factoryapi.FactoryDispatch) factorysessionexecution.DispatchDetail {
	summary := factorysessionexecution.DispatchSummary{
		ID:           response.Id,
		Status:       factorysessionexecution.DispatchStatus(response.Status),
		DispatchKind: string(response.DispatchKind),
	}
	if response.Phase != nil {
		summary.Phase = strings.TrimSpace(*response.Phase)
	}
	if response.Label != nil {
		summary.Label = strings.TrimSpace(*response.Label)
	}
	if response.Attempt != nil {
		summary.Attempt = int(*response.Attempt)
	}
	if response.RunnerId != nil {
		summary.RunnerID = strings.TrimSpace(*response.RunnerId)
	}
	if response.PresetId != nil {
		summary.PresetID = strings.TrimSpace(*response.PresetId)
	}
	if response.ModelProvider != nil {
		summary.ModelProvider = strings.TrimSpace(*response.ModelProvider)
	}
	if response.Model != nil {
		summary.Model = strings.TrimSpace(*response.Model)
	}
	if response.ReasoningEffort != nil {
		summary.ReasoningEffort = strings.TrimSpace(*response.ReasoningEffort)
	}
	if response.Provider != nil {
		summary.Provider = strings.TrimSpace(*response.Provider)
	}
	summary.ProviderSessionRefs = providerSessionRefsFromAPI(response.ProviderSessionRefs)
	if response.Usage != nil {
		summary.Usage = dispatchUsageFromAPI(*response.Usage)
	}
	if response.Warnings != nil {
		summary.Warnings = dispatchWarningsFromAPI(*response.Warnings)
	}
	if response.FailureDetail != nil {
		summary.FailureDetail = dispatchFailureFromAPI(*response.FailureDetail)
	}
	detail := factorysessionexecution.DispatchDetail{
		DispatchSummary:  summary,
		SessionID:        response.SessionId,
		OrchestratorKind: string(response.OrchestratorKind),
	}
	if response.ArtifactIds != nil {
		detail.ArtifactIDs = append([]string(nil), *response.ArtifactIds...)
	}
	if response.StatusTransitions != nil {
		detail.StatusTransitions = make([]factorysessionexecution.DispatchStatus, 0, len(*response.StatusTransitions))
		for _, transition := range *response.StatusTransitions {
			status := strings.TrimSpace(string(transition))
			if status == "" {
				continue
			}
			detail.StatusTransitions = append(detail.StatusTransitions, factorysessionexecution.DispatchStatus(status))
		}
	}
	if response.Petri != nil {
		detail.Petri = &factorysessionexecution.DispatchPetriProjection{
			TransitionID: response.Petri.TransitionId,
		}
		if response.Petri.WorkstationName != nil {
			detail.Petri.WorkstationName = strings.TrimSpace(*response.Petri.WorkstationName)
		}
		if response.Petri.WorkerType != nil {
			detail.Petri.WorkerType = strings.TrimSpace(*response.Petri.WorkerType)
		}
	}
	if response.Javascript != nil {
		detail.JavaScript = &factorysessionexecution.DispatchJavaScriptProjection{
			TaskKind: string(response.Javascript.TaskKind),
		}
		if response.Javascript.TaskLabel != nil {
			detail.JavaScript.TaskLabel = strings.TrimSpace(*response.Javascript.TaskLabel)
		}
		if response.Javascript.ExecutionMode != nil {
			detail.JavaScript.ExecutionMode = strings.TrimSpace(*response.Javascript.ExecutionMode)
		}
	}
	return detail
}

// ArtifactSummaryFromAPI maps one public artifact summary into the shared service contract.
func ArtifactSummaryFromAPI(response factoryapi.FactorySessionArtifactSummary) (factorysessionexecution.ArtifactSummary, error) {
	summary := factorysessionexecution.ArtifactSummary{
		ID:         response.Id,
		Kind:       string(response.Kind),
		Visibility: string(response.Visibility),
	}
	if response.Label != nil {
		summary.Label = strings.TrimSpace(*response.Label)
	}
	if response.ContentHash != nil {
		summary.ContentHash = strings.TrimSpace(*response.ContentHash)
	}
	if response.SizeBytes != nil {
		summary.SizeBytes = *response.SizeBytes
	}
	if response.CreatedAt != nil {
		summary.CreatedAt = response.CreatedAt
	}
	if response.DispatchId != nil {
		summary.DispatchID = strings.TrimSpace(*response.DispatchId)
	}
	if response.AuditMode != nil {
		summary.AuditMode = string(*response.AuditMode)
	}
	if response.RedactionCounts != nil {
		summary.RedactionCounts = artifactRedactionCountsFromAPI(*response.RedactionCounts)
	}
	if response.RetrievalRef != nil {
		ref, err := artifactRetrievalRefFromAPI(*response.RetrievalRef)
		if err != nil {
			return factorysessionexecution.ArtifactSummary{}, err
		}
		summary.RetrievalRef = ref
	}
	return summary, nil
}

// ArtifactDetailFromAPI maps one public artifact detail into the shared service contract.
func ArtifactDetailFromAPI(response factoryapi.FactorySessionArtifactDetail) (factorysessionexecution.ArtifactDetail, error) {
	summary := factorysessionexecution.ArtifactSummary{
		ID:         response.Id,
		Kind:       string(response.Kind),
		Visibility: string(response.Visibility),
	}
	if response.Label != nil {
		summary.Label = strings.TrimSpace(*response.Label)
	}
	if response.ContentHash != nil {
		summary.ContentHash = strings.TrimSpace(*response.ContentHash)
	}
	if response.SizeBytes != nil {
		summary.SizeBytes = *response.SizeBytes
	}
	if response.CreatedAt != nil {
		summary.CreatedAt = response.CreatedAt
	}
	if response.DispatchId != nil {
		summary.DispatchID = strings.TrimSpace(*response.DispatchId)
	}
	if response.AuditMode != nil {
		summary.AuditMode = string(*response.AuditMode)
	}
	if response.RedactionCounts != nil {
		summary.RedactionCounts = artifactRedactionCountsFromAPI(*response.RedactionCounts)
	}
	detail := factorysessionexecution.ArtifactDetail{
		ArtifactSummary: summary,
		SessionID:       response.SessionId,
	}
	if response.Summary != nil {
		detail.Summary = strings.TrimSpace(*response.Summary)
	}
	if response.Content != nil {
		if encoded, err := json.Marshal(response.Content); err == nil {
			detail.Content = encoded
		}
	}
	if response.ContentRef != nil {
		ref, err := artifactRetrievalRefFromAPI(*response.ContentRef)
		if err != nil {
			return factorysessionexecution.ArtifactDetail{}, err
		}
		detail.ContentRef = ref
	}
	if metadata := artifactCaptureMetadataFromAPI(response.CaptureMetadata); len(metadata) > 0 {
		detail.CaptureMetadata = metadata
	}
	return detail, nil
}

// DurableSessionListSummaryFromAPI maps one public durable list row into the shared
// service contract.
//
// pkgmaintcheck:ignore-cyclomatic-complexity this inverse mapper keeps durable list summary fields together for API round-trip coverage.
func DurableSessionListSummaryFromAPI(response factoryapi.FactorySessionDurableSummary) factorysessionexecution.DurableSessionListSummary {
	summary := factorysessionexecution.DurableSessionListSummary{
		SessionID:        response.SessionId,
		Status:           factorysessionexecution.LifecycleStatus(response.Status),
		OrchestratorKind: string(response.OrchestratorKind),
		ResolvedSource:   resolvedSourceFromAPI(response.ResolvedSource),
	}
	if response.Dialect != nil {
		summary.Dialect = strings.TrimSpace(*response.Dialect)
	}
	if response.SourceHash != nil {
		summary.SourceHash = strings.TrimSpace(*response.SourceHash)
	}
	if response.RequestedPolicy != nil {
		summary.Policy.Requested = policyMapFromAPI(*response.RequestedPolicy)
	}
	if response.EffectivePolicy != nil {
		summary.Policy.Effective = effectivePolicyMapFromAPI(*response.EffectivePolicy)
	}
	if response.EffectivePolicyHash != nil {
		summary.Policy.EffectiveHash = strings.TrimSpace(*response.EffectivePolicyHash)
	}
	if response.Phase != nil {
		summary.Phase = strings.TrimSpace(*response.Phase)
	}
	if response.Progress != nil {
		summary.Progress = progressCountsFromAPI(*response.Progress)
	}
	if response.ResultSummary != nil {
		summary.ResultSummary = &factorysessionexecution.ResultSummary{
			ResultStatus: string(response.ResultSummary.ResultStatus),
		}
		if response.ResultSummary.Summary != nil {
			summary.ResultSummary.Summary = strings.TrimSpace(*response.ResultSummary.Summary)
		}
	}
	if response.ArtifactCount != nil {
		summary.ArtifactCount = int(*response.ArtifactCount)
	}
	if response.Recoverable != nil {
		summary.Recoverable = *response.Recoverable
	}
	if response.Actions != nil {
		summary.Actions = sessionActionAvailabilityFromAPI(*response.Actions)
	}
	if response.StaleLease != nil && *response.StaleLease {
		summary.StaleLease = true
	}
	if response.Lifecycle != nil {
		summary.Lifecycle = lifecycleTimestampsFromAPI(*response.Lifecycle)
	}
	if response.Links != nil {
		summary.Links = executionLinksFromAPI(*response.Links)
	}
	return summary
}

// LifecycleControlResultFromAPI maps one public lifecycle control response into the
// shared service contract.
func LifecycleControlResultFromAPI(response factoryapi.FactorySessionLifecycleControlResponse) factorysessionexecution.LifecycleControlResult {
	result := factorysessionexecution.LifecycleControlResult{
		SessionID: response.SessionId,
		Operation: factorysessionexecution.LifecycleControlKind(response.Operation),
		Outcome:   factorysessionexecution.LifecycleControlOutcome(response.Outcome),
		Status:    factorysessionexecution.LifecycleStatus(response.Status),
	}
	if response.Session != nil {
		session := SessionReadResultFromAPI(*response.Session)
		result.Session = &session
	}
	if response.EffectivePolicyHash != nil {
		result.EffectivePolicyHash = strings.TrimSpace(*response.EffectivePolicyHash)
	}
	if response.ApprovalPreviewId != nil {
		result.ApprovalPreviewID = strings.TrimSpace(*response.ApprovalPreviewId)
	}
	if response.DispatchId != nil {
		result.DispatchID = strings.TrimSpace(*response.DispatchId)
	}
	if response.RetryDispatchId != nil {
		result.RetryDispatchID = strings.TrimSpace(*response.RetryDispatchId)
	}
	if response.Detail != nil {
		result.Detail = strings.TrimSpace(*response.Detail)
	}
	if response.Links != nil {
		result.Links = lifecycleControlLinksFromAPI(*response.Links)
	}
	return result
}

// ControlErrorToAPI maps one typed lifecycle control failure into the public response shape.
func ControlErrorToAPI(sessionID string, err *factorysessionexecution.ControlError) factoryapi.FactorySessionLifecycleControlResponse {
	if err == nil {
		return factoryapi.FactorySessionLifecycleControlResponse{}
	}
	return LifecycleControlResponseToAPI(factorysessionexecution.LifecycleControlResult{
		SessionID: sessionID, Operation: err.Operation, Outcome: err.Outcome,
		Status: err.Status, Detail: strings.TrimSpace(err.Message), Links: err.Links,
	})
}

func resolvedSourceFromAPI(source factoryapi.FactorySessionResolvedSourceIdentity) factorysessionexecution.ResolvedSource {
	result := factorysessionexecution.ResolvedSource{
		Kind: workflowsource.WorkflowSourceKind(source.Kind),
	}
	if source.Dialect != nil {
		result.Dialect = strings.TrimSpace(*source.Dialect)
	}
	if source.SourceRef != nil {
		result.SourceRef = strings.TrimSpace(*source.SourceRef)
	}
	if source.SourceHash != nil {
		result.SourceHash = strings.TrimSpace(*source.SourceHash)
	}
	if source.ResolutionOrder != nil {
		for _, stage := range *source.ResolutionOrder {
			result.ResolutionOrder = append(result.ResolutionOrder, string(stage))
		}
	}
	if source.Metadata != nil {
		result.Metadata = cloneStringMap(map[string]string(*source.Metadata))
	}
	return result
}

func executionLinksFromAPI(links factoryapi.FactorySessionExecutionLinks) factorysessionexecution.InspectionLinks {
	return factorysessionexecution.InspectionLinks{
		Session: strings.TrimSpace(derefString(links.Session)),
		Status:  strings.TrimSpace(derefString(links.Status)),
		Events:  strings.TrimSpace(derefString(links.Events)),
		Results: strings.TrimSpace(derefString(links.Results)),
	}
}

func lifecycleControlLinksFromAPI(links factoryapi.FactorySessionLifecycleControlLinks) factorysessionexecution.LifecycleControlLinks {
	return factorysessionexecution.LifecycleControlLinks{
		Session:    strings.TrimSpace(derefString(links.Session)),
		Status:     strings.TrimSpace(derefString(links.Status)),
		Events:     strings.TrimSpace(derefString(links.Events)),
		Results:    strings.TrimSpace(derefString(links.Results)),
		Dispatches: strings.TrimSpace(derefString(links.Dispatches)),
		Artifacts:  strings.TrimSpace(derefString(links.Artifacts)),
	}
}

func effectivePolicyMapFromAPI(policy factoryapi.FactorySessionEffectivePolicy) map[string]any {
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

func phaseSummariesFromAPI(summaries []factoryapi.FactorySessionDurablePhaseSummary) []factorysessionexecution.PhaseSummary {
	out := make([]factorysessionexecution.PhaseSummary, 0, len(summaries))
	for _, summary := range summaries {
		row := factorysessionexecution.PhaseSummary{Phase: summary.Phase}
		if summary.Label != nil {
			row.Label = strings.TrimSpace(*summary.Label)
		}
		if summary.DispatchCount != nil {
			row.DispatchCount = int(*summary.DispatchCount)
		}
		if summary.CompletedDispatchCount != nil {
			row.CompletedDispatchCount = int(*summary.CompletedDispatchCount)
		}
		if summary.FailedDispatchCount != nil {
			row.FailedDispatchCount = int(*summary.FailedDispatchCount)
		}
		out = append(out, row)
	}
	return out
}

func progressCountsFromAPI(counts factoryapi.FactorySessionDurableProgressCounts) *factorysessionexecution.ProgressCounts {
	out := &factorysessionexecution.ProgressCounts{}
	if counts.TotalDispatches != nil {
		out.TotalDispatches = int(*counts.TotalDispatches)
	}
	if counts.CompletedDispatches != nil {
		out.CompletedDispatches = int(*counts.CompletedDispatches)
	}
	if counts.FailedDispatches != nil {
		out.FailedDispatches = int(*counts.FailedDispatches)
	}
	if counts.InFlightDispatches != nil {
		out.InFlightDispatches = int(*counts.InFlightDispatches)
	}
	out.QueuedDispatches = intValueFromPointer(counts.QueuedDispatches)
	out.RunningDispatches = intValueFromPointer(counts.RunningDispatches)
	out.CanceledDispatches = intValueFromPointer(counts.CanceledDispatches)
	out.TimedOutDispatches = intValueFromPointer(counts.TimedOutDispatches)
	out.SkippedDispatches = intValueFromPointer(counts.SkippedDispatches)
	out.InterruptedDispatches = intValueFromPointer(counts.InterruptedDispatches)
	if counts.PhaseCount != nil {
		out.PhaseCount = int(*counts.PhaseCount)
	}
	return out
}

func intValueFromPointer(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func failureSummaryFromAPI(failure factoryapi.FailureDetail) *factorysessionexecution.FailureSummary {
	return &factorysessionexecution.FailureSummary{Reason: strings.TrimSpace(string(failure.Reason)), Message: strings.TrimSpace(failure.Message)}
}

func lifecycleTimestampsFromAPI(lifecycle factoryapi.FactorySessionDurableLifecycleTimestamps) *factorysessionexecution.LifecycleTimestamps {
	return &factorysessionexecution.LifecycleTimestamps{
		QueuedAt:           lifecycle.QueuedAt,
		AwaitingApprovalAt: lifecycle.AwaitingApprovalAt,
		StartedAt:          lifecycle.StartedAt,
		PausedAt:           lifecycle.PausedAt,
		ResumedAt:          lifecycle.ResumedAt,
		FinishedAt:         lifecycle.FinishedAt,
		InterruptedAt:      lifecycle.InterruptedAt,
		TerminatedAt:       lifecycle.TerminatedAt,
		UpdatedAt:          lifecycle.UpdatedAt,
	}
}

func artifactRefsFromAPI(refs []factoryapi.FactoryArtifactRef) []factorysessionexecution.ArtifactRefSummary {
	out := make([]factorysessionexecution.ArtifactRefSummary, 0, len(refs))
	for _, ref := range refs {
		row := factorysessionexecution.ArtifactRefSummary{ID: ref.Id}
		if ref.Kind != "" {
			row.Kind = string(ref.Kind)
		}
		if ref.Visibility != "" {
			row.Visibility = string(ref.Visibility)
		}
		if ref.ContentHash != nil {
			row.ContentHash = strings.TrimSpace(*ref.ContentHash)
		}
		if ref.SizeBytes != nil {
			row.SizeBytes = *ref.SizeBytes
		}
		out = append(out, row)
	}
	return out
}

func resultAvailabilityFromAPI(availability factoryapi.FactorySessionResultAvailabilityDetail) *factorysessionexecution.ResultAvailabilityDetail {
	out := &factorysessionexecution.ResultAvailabilityDetail{}
	if availability.Reason != nil {
		out.Reason = strings.TrimSpace(*availability.Reason)
	}
	if availability.Message != nil {
		out.Message = strings.TrimSpace(*availability.Message)
	}
	if availability.Retryable != nil {
		out.Retryable = *availability.Retryable
	}
	return out
}

func dispatchUsageFromAPI(usage factoryapi.FactoryDispatchUsage) *factorysessionexecution.DispatchUsage {
	out := &factorysessionexecution.DispatchUsage{}
	if usage.InputTokens != nil {
		out.InputTokens = *usage.InputTokens
	}
	if usage.OutputTokens != nil {
		out.OutputTokens = *usage.OutputTokens
	}
	if usage.TotalTokens != nil {
		out.TotalTokens = *usage.TotalTokens
	}
	if usage.DurationMillis != nil {
		out.DurationMillis = *usage.DurationMillis
	}
	if usage.CostUsd != nil {
		out.CostUSD = *usage.CostUsd
	}
	if usage.RetryCount != nil {
		out.RetryCount = *usage.RetryCount
	}
	return out
}

func dispatchWarningsFromAPI(warnings []factoryapi.FactoryDispatchWarning) []factorysessionexecution.DispatchWarning {
	out := make([]factorysessionexecution.DispatchWarning, 0, len(warnings))
	for _, warning := range warnings {
		out = append(out, factorysessionexecution.DispatchWarning{
			Code:    warning.Code,
			Message: strings.TrimSpace(warning.Message),
		})
	}
	return out
}

func dispatchFailureFromAPI(failure factoryapi.FailureDetail) *factorysessionexecution.DispatchFailureDetail {
	return &factorysessionexecution.DispatchFailureDetail{Reason: strings.TrimSpace(string(failure.Reason)), Message: strings.TrimSpace(failure.Message)}
}

func dispatchJavaScriptFromAPI(javascript factoryapi.FactoryDispatchJavaScriptProjection) *factorysessionexecution.DispatchJavaScriptProjection {
	out := &factorysessionexecution.DispatchJavaScriptProjection{
		TaskKind: strings.TrimSpace(string(javascript.TaskKind)),
	}
	if javascript.TaskLabel != nil {
		out.TaskLabel = strings.TrimSpace(*javascript.TaskLabel)
	}
	if javascript.ExecutionMode != nil {
		out.ExecutionMode = strings.TrimSpace(*javascript.ExecutionMode)
	}
	return out
}

func artifactRedactionCountsFromAPI(counts factoryapi.FactoryArtifactRedactionCounts) *factorysessionexecution.ArtifactRedactionCounts {
	out := &factorysessionexecution.ArtifactRedactionCounts{}
	if counts.Paths != nil {
		out.Paths = *counts.Paths
	}
	if counts.Secrets != nil {
		out.Secrets = *counts.Secrets
	}
	if counts.Tokens != nil {
		out.Tokens = *counts.Tokens
	}
	return out
}

func artifactCaptureMetadataFromAPI(metadata *factoryapi.FactoryArtifactCaptureMetadata) map[string]any {
	if metadata == nil {
		return nil
	}
	capture := make(map[string]any)
	if metadata.CapturedAt != nil {
		capture["capturedAt"] = metadata.CapturedAt.UTC().Format(time.RFC3339)
	}
	if metadata.SourceDispatchId != nil {
		if dispatchID := strings.TrimSpace(*metadata.SourceDispatchId); dispatchID != "" {
			capture["sourceDispatchId"] = dispatchID
		}
	}
	if metadata.MimeType != nil {
		if mimeType := strings.TrimSpace(*metadata.MimeType); mimeType != "" {
			capture["mimeType"] = mimeType
		}
	}
	if len(capture) == 0 {
		return nil
	}
	return capture
}

func artifactRetrievalRefFromAPI(ref factoryapi.FactorySessionArtifactRetrievalRef) (*factorysessionexecution.ArtifactRetrievalRef, error) {
	href := strings.TrimSpace(ref.Href)
	if href == "" {
		return nil, &apisurface.RequestValidationError{Message: "artifact retrieval ref href is required"}
	}
	if strings.Contains(href, "://") || strings.HasPrefix(href, "/var/") || strings.HasPrefix(href, "file:") {
		return nil, &apisurface.RequestValidationError{Message: "artifact retrieval ref href must be an API-relative path"}
	}
	result := &factorysessionexecution.ArtifactRetrievalRef{Href: href}
	if ref.Method != nil {
		result.Method = string(*ref.Method)
	}
	return result, nil
}
