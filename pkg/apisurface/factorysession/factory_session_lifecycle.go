package factorysession

import (
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// ControlRequestFromAPI maps one public lifecycle control request into the shared
// service contract.
func ControlRequestFromAPI(req factoryapi.FactorySessionLifecycleControlRequest) (factorysessionexecution.ControlRequest, error) {
	return factorysessionexecution.NormalizeControlRequest(factorysessionexecution.ControlRequest{
		RequestID: derefString(req.RequestId),
		Reason:    derefString(req.Reason),
	})
}

// ApproveRequestFromAPI maps one public approval request into the shared service contract.
func ApproveRequestFromAPI(req factoryapi.FactorySessionApproveRequest) (factorysessionexecution.ApproveRequest, error) {
	control, err := factorysessionexecution.NormalizeControlRequest(factorysessionexecution.ControlRequest{
		RequestID: derefString(req.RequestId),
		Reason:    derefString(req.Reason),
	})
	if err != nil {
		return factorysessionexecution.ApproveRequest{}, err
	}
	approve := factorysessionexecution.ApproveRequest{
		ControlRequest:    control,
		ApprovalPreviewID: derefString(req.ApprovalPreviewId),
	}
	if req.ApprovedPolicy != nil {
		approve.ApprovedPolicy = policyMapFromAPI(*req.ApprovedPolicy)
	}
	return factorysessionexecution.NormalizeApproveRequest(approve)
}

// RetryDispatchRequestFromAPI maps one public retry-dispatch request into the shared service contract.
func RetryDispatchRequestFromAPI(req factoryapi.FactorySessionRetryDispatchRequest) (factorysessionexecution.RetryDispatchRequest, error) {
	retry := factorysessionexecution.RetryDispatchRequest{
		ControlRequest: factorysessionexecution.ControlRequest{
			RequestID: derefString(req.RequestId),
			Reason:    derefString(req.Reason),
		},
		DispatchID: req.DispatchId,
	}
	if req.ForceNewAttempt != nil {
		retry.ForceNewAttempt = *req.ForceNewAttempt
	}
	if req.ResetAttemptCount != nil {
		retry.ResetAttemptCount = *req.ResetAttemptCount
	}
	return factorysessionexecution.NormalizeRetryDispatchRequest(retry)
}

// SessionReadResponseToAPI maps one durable session read projection to the public response shape.
func SessionReadResponseToAPI(result factorysessionexecution.SessionReadResult) factoryapi.FactorySessionDurableReadModel {
	response := factoryapi.FactorySessionDurableReadModel{
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
	if phase := strings.TrimSpace(result.Phase); phase != "" {
		response.Phase = &phase
	}
	if summaries := phaseSummariesToAPI(result.PhaseSummaries); summaries != nil {
		response.PhaseSummaries = summaries
	}
	if progress := progressCountsToAPI(result.Progress); progress != nil {
		response.Progress = progress
	}
	if summary := resultSummaryToAPI(result.ResultSummary); summary != nil {
		response.ResultSummary = summary
	}
	if refs := artifactRefsToAPI(result.ArtifactRefs); refs != nil {
		response.ArtifactRefs = refs
	}
	if failure := failureSummaryToAPI(result.Failure); failure != nil {
		response.Failure = failure
	}
	if lifecycle := lifecycleTimestampsToAPI(result.Lifecycle); lifecycle != nil {
		response.Lifecycle = lifecycle
	}
	if result.StaleLease {
		stale := true
		response.StaleLease = &stale
	}
	if links := executionLinksToAPI(result.Links); links != nil {
		response.Links = links
	}
	return response
}

// LifecycleControlResponseToAPI maps one lifecycle control result to the public response shape.
func LifecycleControlResponseToAPI(result factorysessionexecution.LifecycleControlResult) factoryapi.FactorySessionLifecycleControlResponse {
	response := factoryapi.FactorySessionLifecycleControlResponse{
		SessionId: result.SessionID,
		Operation: factoryapi.FactorySessionLifecycleControlKind(result.Operation),
		Outcome:   factoryapi.FactorySessionLifecycleControlOutcome(result.Outcome),
		Status:    factoryapi.FactorySessionDurableLifecycleStatus(result.Status),
	}
	if result.Session != nil {
		session := SessionReadResponseToAPI(*result.Session)
		response.Session = &session
	}
	if effectiveHash := strings.TrimSpace(result.EffectivePolicyHash); effectiveHash != "" {
		response.EffectivePolicyHash = &effectiveHash
	}
	if approvalPreviewID := strings.TrimSpace(result.ApprovalPreviewID); approvalPreviewID != "" {
		response.ApprovalPreviewId = &approvalPreviewID
	}
	if dispatchID := strings.TrimSpace(result.DispatchID); dispatchID != "" {
		response.DispatchId = &dispatchID
	}
	if retryDispatchID := strings.TrimSpace(result.RetryDispatchID); retryDispatchID != "" {
		response.RetryDispatchId = &retryDispatchID
	}
	if detail := strings.TrimSpace(result.Detail); detail != "" {
		response.Detail = &detail
	}
	if links := lifecycleControlLinksToAPI(result.Links); links != nil {
		response.Links = links
	}
	return response
}

func phaseSummariesToAPI(summaries []factorysessionexecution.PhaseSummary) *[]factoryapi.FactorySessionDurablePhaseSummary {
	if len(summaries) == 0 {
		return nil
	}
	out := make([]factoryapi.FactorySessionDurablePhaseSummary, 0, len(summaries))
	for _, summary := range summaries {
		row := factoryapi.FactorySessionDurablePhaseSummary{
			Phase: summary.Phase,
		}
		if label := strings.TrimSpace(summary.Label); label != "" {
			row.Label = &label
		}
		if summary.DispatchCount > 0 {
			count := summary.DispatchCount
			row.DispatchCount = &count
		}
		if summary.CompletedDispatchCount > 0 {
			count := summary.CompletedDispatchCount
			row.CompletedDispatchCount = &count
		}
		if summary.FailedDispatchCount > 0 {
			count := summary.FailedDispatchCount
			row.FailedDispatchCount = &count
		}
		out = append(out, row)
	}
	return &out
}

func progressCountsToAPI(counts *factorysessionexecution.ProgressCounts) *factoryapi.FactorySessionDurableProgressCounts {
	if counts == nil {
		return nil
	}
	out := &factoryapi.FactorySessionDurableProgressCounts{}
	if counts.TotalDispatches > 0 {
		value := counts.TotalDispatches
		out.TotalDispatches = &value
	}
	if counts.CompletedDispatches > 0 {
		value := counts.CompletedDispatches
		out.CompletedDispatches = &value
	}
	if counts.FailedDispatches > 0 {
		value := counts.FailedDispatches
		out.FailedDispatches = &value
	}
	if counts.InFlightDispatches > 0 {
		value := counts.InFlightDispatches
		out.InFlightDispatches = &value
	}
	if counts.PhaseCount > 0 {
		value := counts.PhaseCount
		out.PhaseCount = &value
	}
	if out.TotalDispatches == nil &&
		out.CompletedDispatches == nil &&
		out.FailedDispatches == nil &&
		out.InFlightDispatches == nil &&
		out.PhaseCount == nil {
		return nil
	}
	return out
}

func resultSummaryToAPI(summary *factorysessionexecution.ResultSummary) *factoryapi.FactorySessionDurableResultSummary {
	if summary == nil {
		return nil
	}
	out := &factoryapi.FactorySessionDurableResultSummary{
		ResultStatus: factoryapi.FactorySessionResultStatus(summary.ResultStatus),
	}
	if text := strings.TrimSpace(summary.Summary); text != "" {
		out.Summary = &text
	}
	return out
}

func failureSummaryToAPI(failure *factorysessionexecution.FailureSummary) *factoryapi.FactorySessionDurableFailureDetail {
	if failure == nil {
		return nil
	}
	out := &factoryapi.FactorySessionDurableFailureDetail{}
	if reason := strings.TrimSpace(failure.Reason); reason != "" {
		out.Reason = &reason
	}
	if message := strings.TrimSpace(failure.Message); message != "" {
		out.Message = &message
	}
	if errorClass := strings.TrimSpace(failure.ErrorClass); errorClass != "" {
		out.ErrorClass = &errorClass
	}
	if failure.PartialResultAvailable {
		value := true
		out.PartialResultAvailable = &value
	}
	if out.Reason == nil && out.Message == nil && out.ErrorClass == nil && out.PartialResultAvailable == nil {
		return nil
	}
	return out
}

// pkgmaintcheck:ignore-cyclomatic-complexity this mapper keeps optional durable lifecycle timestamp fields together on one API projection seam.
func lifecycleTimestampsToAPI(lifecycle *factorysessionexecution.LifecycleTimestamps) *factoryapi.FactorySessionDurableLifecycleTimestamps {
	if lifecycle == nil {
		return nil
	}
	out := &factoryapi.FactorySessionDurableLifecycleTimestamps{}
	if lifecycle.QueuedAt != nil {
		out.QueuedAt = lifecycle.QueuedAt
	}
	if lifecycle.AwaitingApprovalAt != nil {
		out.AwaitingApprovalAt = lifecycle.AwaitingApprovalAt
	}
	if lifecycle.StartedAt != nil {
		out.StartedAt = lifecycle.StartedAt
	}
	if lifecycle.PausedAt != nil {
		out.PausedAt = lifecycle.PausedAt
	}
	if lifecycle.ResumedAt != nil {
		out.ResumedAt = lifecycle.ResumedAt
	}
	if lifecycle.FinishedAt != nil {
		out.FinishedAt = lifecycle.FinishedAt
	}
	if lifecycle.InterruptedAt != nil {
		out.InterruptedAt = lifecycle.InterruptedAt
	}
	if lifecycle.TerminatedAt != nil {
		out.TerminatedAt = lifecycle.TerminatedAt
	}
	if lifecycle.UpdatedAt != nil {
		out.UpdatedAt = lifecycle.UpdatedAt
	}
	if out.QueuedAt == nil &&
		out.AwaitingApprovalAt == nil &&
		out.StartedAt == nil &&
		out.PausedAt == nil &&
		out.ResumedAt == nil &&
		out.FinishedAt == nil &&
		out.InterruptedAt == nil &&
		out.TerminatedAt == nil &&
		out.UpdatedAt == nil {
		return nil
	}
	return out
}

func artifactRefsToAPI(refs []factorysessionexecution.ArtifactRefSummary) *[]factoryapi.FactoryArtifactRef {
	if len(refs) == 0 {
		return nil
	}
	out := make([]factoryapi.FactoryArtifactRef, 0, len(refs))
	for _, ref := range refs {
		row := factoryapi.FactoryArtifactRef{
			Id: ref.ID,
		}
		if kind := strings.TrimSpace(ref.Kind); kind != "" {
			row.Kind = factoryapi.FactoryArtifactKind(kind)
		}
		if visibility := strings.TrimSpace(ref.Visibility); visibility != "" {
			row.Visibility = factoryapi.FactoryArtifactVisibility(visibility)
		}
		if contentHash := strings.TrimSpace(ref.ContentHash); contentHash != "" {
			row.ContentHash = &contentHash
		}
		if ref.SizeBytes > 0 {
			size := ref.SizeBytes
			row.SizeBytes = &size
		}
		out = append(out, row)
	}
	return &out
}

func lifecycleControlLinksToAPI(links factorysessionexecution.LifecycleControlLinks) *factoryapi.FactorySessionLifecycleControlLinks {
	if links == (factorysessionexecution.LifecycleControlLinks{}) {
		return nil
	}
	response := &factoryapi.FactorySessionLifecycleControlLinks{}
	if value := strings.TrimSpace(links.Session); value != "" {
		response.Session = &value
	}
	if value := strings.TrimSpace(links.Results); value != "" {
		response.Results = &value
	}
	if value := strings.TrimSpace(links.Dispatches); value != "" {
		response.Dispatches = &value
	}
	if value := strings.TrimSpace(links.Artifacts); value != "" {
		response.Artifacts = &value
	}
	if value := strings.TrimSpace(links.Events); value != "" {
		response.Events = &value
	}
	if value := strings.TrimSpace(links.Status); value != "" {
		response.Status = &value
	}
	return response
}
