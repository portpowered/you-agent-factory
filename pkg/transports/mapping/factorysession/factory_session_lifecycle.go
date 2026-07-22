package factorysession

import (
	"errors"
	"net/http"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

// ControlRequestFromAPI maps one public lifecycle control request into the shared
// service contract.
func ControlRequestFromAPI(req factoryapi.FactorySessionLifecycleControlRequest) (factorysessionexecution.ControlRequest, error) {
	return factorysessionexecution.ControlRequest{
		RequestID: derefString(req.RequestId),
		Reason:    derefString(req.Reason),
	}, nil
}

// ApproveRequestFromAPI maps one public approval request into the shared service contract.
func ApproveRequestFromAPI(req factoryapi.FactorySessionApproveRequest) (factorysessionexecution.ApproveRequest, error) {
	control := factorysessionexecution.ControlRequest{
		RequestID: derefString(req.RequestId),
		Reason:    derefString(req.Reason),
	}
	approve := factorysessionexecution.ApproveRequest{
		ControlRequest:    control,
		ApprovalPreviewID: derefString(req.ApprovalPreviewId),
	}
	if req.ApprovedPolicy != nil {
		approve.ApprovedPolicy = policyMapFromAPI(*req.ApprovedPolicy)
	}
	return approve, nil
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
	return retry, nil
}

// InterruptDispatchRequestFromAPI maps one public interrupt-dispatch request into the shared service contract.
func InterruptDispatchRequestFromAPI(req factoryapi.FactorySessionInterruptDispatchRequest) (factorysessionexecution.InterruptDispatchRequest, error) {
	interrupt := factorysessionexecution.InterruptDispatchRequest{
		ControlRequest: factorysessionexecution.ControlRequest{
			RequestID: derefString(req.RequestId),
			Reason:    derefString(req.Reason),
		},
		DispatchID: req.DispatchId,
	}
	return interrupt, nil
}

// SessionReadResponseToAPI maps one durable session read projection to the public response shape.
func SessionReadResponseToAPI(result factorysessionexecution.SessionReadResult) factoryapi.FactorySessionDurableReadModel {
	response := factoryapi.FactorySessionDurableReadModel{
		SessionId:        result.SessionID,
		Status:           factoryapi.FactorySessionDurableLifecycleStatus(result.Status),
		OrchestratorKind: factoryapi.FactoryOrchestratorKind(interfaces.StrictPublicFactoryOrchestratorKind(result.OrchestratorKind)),
		ResolvedSource:   resolvedSourceToAPI(result.ResolvedSource),
	}
	applyOptionalSessionReadResponseFields(&response, result)
	return response
}

func applyOptionalSessionReadResponseFields(
	response *factoryapi.FactorySessionDurableReadModel,
	result factorysessionexecution.SessionReadResult,
) {
	applyOptionalSessionReadPolicyFields(response, result)
	applyOptionalSessionReadOutcomeFields(response, result)
}

func applyOptionalSessionReadPolicyFields(
	response *factoryapi.FactorySessionDurableReadModel,
	result factorysessionexecution.SessionReadResult,
) {
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
	if checkpoint := result.LatestCheckpoint; checkpoint != nil {
		response.LatestCheckpoint = &factoryapi.FactorySessionCheckpointRef{Id: checkpoint.ID}
		if label := strings.TrimSpace(checkpoint.Label); label != "" {
			response.LatestCheckpoint.Label = &label
		}
		if phase := strings.TrimSpace(checkpoint.Phase); phase != "" {
			response.LatestCheckpoint.Phase = &phase
		}
	}
	if progress := progressCountsToAPI(result.Progress); progress != nil {
		response.Progress = progress
	}
	if budgets := sessionBudgetsToAPI(result.Budgets); budgets != nil {
		response.Budgets = budgets
	}
	usage := sessionUsageToAPI(result.Usage)
	response.Usage = &usage
}

func applyOptionalSessionReadOutcomeFields(
	response *factoryapi.FactorySessionDurableReadModel,
	result factorysessionexecution.SessionReadResult,
) {
	if summary := resultSummaryToAPI(result.ResultSummary); summary != nil {
		response.ResultSummary = summary
	}
	if refs := artifactRefsToAPI(result.ArtifactRefs); refs != nil {
		response.ArtifactRefs = refs
	}
	if failure := failureSummaryToAPI(result.Failure); failure != nil {
		response.FailureDetail = failure
		if result.Failure.PartialResultAvailable {
			value := true
			response.PartialResultAvailable = &value
		}
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
	setProgressCount(&out.TotalDispatches, counts.TotalDispatches)
	setProgressCount(&out.CompletedDispatches, counts.CompletedDispatches)
	setProgressCount(&out.FailedDispatches, counts.FailedDispatches)
	setProgressCount(&out.InFlightDispatches, counts.InFlightDispatches)
	setProgressCount(&out.QueuedDispatches, counts.QueuedDispatches)
	setProgressCount(&out.RunningDispatches, counts.RunningDispatches)
	setProgressCount(&out.CanceledDispatches, counts.CanceledDispatches)
	setProgressCount(&out.TimedOutDispatches, counts.TimedOutDispatches)
	setProgressCount(&out.SkippedDispatches, counts.SkippedDispatches)
	setProgressCount(&out.InterruptedDispatches, counts.InterruptedDispatches)
	setProgressCount(&out.PhaseCount, counts.PhaseCount)
	if *out == (factoryapi.FactorySessionDurableProgressCounts{}) {
		return nil
	}
	return out
}

func setProgressCount(target **int, count int) {
	if count > 0 {
		value := count
		*target = &value
	}
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

func sessionActionAvailabilityToAPI(actions factorysessionexecution.SessionActionAvailability) *factoryapi.FactorySessionDurableActionAvailability {
	canPause := actions.CanPause
	canResume := actions.CanResume
	canCancel := actions.CanCancel
	canTerminate := actions.CanTerminate
	canApprove := actions.CanApprove
	canRetryDispatch := actions.CanRetryDispatch
	canInterruptDispatch := actions.CanInterruptDispatch
	return &factoryapi.FactorySessionDurableActionAvailability{
		CanPause:             &canPause,
		CanResume:            &canResume,
		CanCancel:            &canCancel,
		CanTerminate:         &canTerminate,
		CanApprove:           &canApprove,
		CanRetryDispatch:     &canRetryDispatch,
		CanInterruptDispatch: &canInterruptDispatch,
	}
}

func sessionActionAvailabilityFromAPI(actions factoryapi.FactorySessionDurableActionAvailability) factorysessionexecution.SessionActionAvailability {
	out := factorysessionexecution.SessionActionAvailability{}
	if actions.CanPause != nil {
		out.CanPause = *actions.CanPause
	}
	if actions.CanResume != nil {
		out.CanResume = *actions.CanResume
	}
	if actions.CanCancel != nil {
		out.CanCancel = *actions.CanCancel
	}
	if actions.CanTerminate != nil {
		out.CanTerminate = *actions.CanTerminate
	}
	if actions.CanApprove != nil {
		out.CanApprove = *actions.CanApprove
	}
	if actions.CanRetryDispatch != nil {
		out.CanRetryDispatch = *actions.CanRetryDispatch
	}
	if actions.CanInterruptDispatch != nil {
		out.CanInterruptDispatch = *actions.CanInterruptDispatch
	}
	return out
}

func failureSummaryToAPI(failure *factorysessionexecution.FailureSummary) *factoryapi.FailureDetail {
	if failure == nil {
		return nil
	}
	reason := strings.TrimSpace(failure.Reason)
	message := strings.TrimSpace(failure.Message)
	if reason == "" || message == "" {
		return nil
	}
	return &factoryapi.FailureDetail{Reason: failureReasonToAPI(reason), Message: message}
}

func failureReasonToAPI(reason string) factoryapi.WorkFailureType {
	candidate := factoryapi.WorkFailureType(strings.TrimSpace(reason))
	switch candidate {
	case factoryapi.WorkFailureTypeAuthFailure,
		factoryapi.WorkFailureTypePermanentBadRequest,
		factoryapi.WorkFailureTypeThrottled,
		factoryapi.WorkFailureTypeInternalServerError,
		factoryapi.WorkFailureTypeTimeout,
		factoryapi.WorkFailureTypeMisconfigured,
		factoryapi.WorkFailureTypeMissingExecutable,
		factoryapi.WorkFailureTypeCommandLineTooLong:
		return candidate
	default:
		return factoryapi.WorkFailureTypeUnknown
	}
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

// LifecycleControlSuccessStatus maps one accepted lifecycle-control result to the
// HTTP success status for the public durable control routes.
func LifecycleControlSuccessStatus(result factorysessionexecution.LifecycleControlResult) int {
	switch result.Outcome {
	case factorysessionexecution.LifecycleControlOutcomeNoOp:
		return http.StatusOK
	case factorysessionexecution.LifecycleControlOutcomeAccepted:
		if result.Status == factorysessionexecution.LifecycleStatusCanceling {
			return http.StatusAccepted
		}
		return http.StatusOK
	default:
		return http.StatusOK
	}
}

// LifecycleControlErrorResponse maps durable lifecycle-control contract errors to
// HTTP status and the public response shape. It returns false when err is not a
// known lifecycle-control contract failure.
func LifecycleControlErrorResponse(sessionID string, err error) (int, any, bool) {
	if err == nil {
		return 0, nil, false
	}

	var controlErr *factorysessionexecution.ControlError
	if errors.As(err, &controlErr) {
		return http.StatusConflict, ControlErrorToAPI(sessionID, controlErr), true
	}

	if errors.Is(err, factorysessionexecution.ErrDurableSessionNotFound) ||
		errors.Is(err, apisurface.ErrFactorySessionNotFound) {
		return http.StatusNotFound, factoryapi.ErrorResponse{
			Message: "factory session not found",
			Family:  factoryapi.ErrorFamilyNotFound,
			Code:    factoryapi.ErrorResponseCodeNOTFOUND,
		}, true
	}

	if errors.Is(err, factorysessionexecution.ErrDispatchNotFound) {
		return http.StatusNotFound, factoryapi.ErrorResponse{
			Message: "dispatch not found",
			Family:  factoryapi.ErrorFamilyNotFound,
			Code:    factoryapi.ErrorResponseCodeNOTFOUND,
		}, true
	}

	var validationErr *factorysessionexecution.ExecutionValidationError
	if errors.As(err, &validationErr) {
		return http.StatusBadRequest, factoryapi.ErrorResponse{
			Message: validationErr.Message,
			Family:  factoryapi.ErrorFamilyBadRequest,
			Code:    factoryapi.ErrorResponseCodeBADREQUEST,
		}, true
	}

	return 0, nil, false
}
