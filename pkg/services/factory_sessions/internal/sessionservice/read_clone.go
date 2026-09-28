package service

import (
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
)

func cloneCanonicalControlRequest(
	request factorysessions.SessionControlRequest,
) factorysessions.SessionControlRequest {
	cloned := request
	if request.Recover != nil {
		recoverRequest := *request.Recover
		cloned.Recover = &recoverRequest
	}
	if request.Approve != nil {
		approve := *request.Approve
		approve.ApprovedPolicy = cloneCanonicalAnyMap(request.Approve.ApprovedPolicy)
		cloned.Approve = &approve
	}
	if request.Retry != nil {
		retry := *request.Retry
		cloned.Retry = &retry
	}
	if request.Interrupt != nil {
		interrupt := *request.Interrupt
		cloned.Interrupt = &interrupt
	}
	return cloned
}

func cloneCanonicalSessionListFilters(
	filters factorysessions.SessionListFilters,
) factorysessions.SessionListFilters {
	cloned := filters
	cloned.Statuses = append([]factorysessions.LifecycleStatus(nil), filters.Statuses...)
	cloned.OrchestratorKinds = append([]string(nil), filters.OrchestratorKinds...)
	cloned.CreatedAfter = cloneCanonicalTime(filters.CreatedAfter)
	cloned.CreatedBefore = cloneCanonicalTime(filters.CreatedBefore)
	cloned.UpdatedAfter = cloneCanonicalTime(filters.UpdatedAfter)
	cloned.UpdatedBefore = cloneCanonicalTime(filters.UpdatedBefore)
	return cloned
}

func cloneCanonicalTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneCanonicalDurableResult(
	result factorysessions.ResultReadResult,
) *factorysessions.SessionDurableResult {
	cloned := &factorysessions.SessionDurableResult{
		SessionID:        result.SessionID,
		Status:           result.ResultStatus,
		SessionStatus:    result.SessionStatus,
		Mode:             result.Mode,
		IncludeArtifacts: result.IncludeArtifacts,
		PrimaryResult:    append([]byte(nil), result.PrimaryResult...),
		ArtifactIDs:      append([]string(nil), result.ArtifactIDs...),
		ArtifactRefs:     append([]factorysessions.ArtifactRefSummary(nil), result.ArtifactRefs...),
	}
	if result.Failure != nil {
		failure := *result.Failure
		cloned.Failure = &failure
	}
	if result.Availability != nil {
		availability := *result.Availability
		cloned.Availability = &availability
	}
	return cloned
}

func cloneCanonicalCheckpointRefs(
	refs []factorydefinitions.FactorySessionJavaScriptCheckpointEventRef,
) []factorydefinitions.FactorySessionJavaScriptCheckpointEventRef {
	if len(refs) == 0 {
		return nil
	}
	cloned := make([]factorydefinitions.FactorySessionJavaScriptCheckpointEventRef, len(refs))
	for index, ref := range refs {
		cloned[index] = ref
		cloned[index].ArtifactRef = cloneCanonicalArtifactRef(ref.ArtifactRef)
		cloned[index].Label = cloneCanonicalString(ref.Label)
		cloned[index].Summary = cloneCanonicalString(ref.Summary)
		cloned[index].Timestamp = cloneCanonicalTime(ref.Timestamp)
	}
	return cloned
}

func cloneCanonicalArtifactRef(
	ref *factorydefinitions.FactoryArtifactRef,
) *factorydefinitions.FactoryArtifactRef {
	if ref == nil {
		return nil
	}
	cloned := *ref
	if ref.ContentHash != nil {
		contentHash := *ref.ContentHash
		cloned.ContentHash = &contentHash
	}
	if ref.SizeBytes != nil {
		sizeBytes := *ref.SizeBytes
		cloned.SizeBytes = &sizeBytes
	}
	return &cloned
}

func cloneCanonicalString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneCanonicalDispatchesResult(
	result factorysessions.ListDispatchesResult,
) factorysessions.ListDispatchesResult {
	cloned := factorysessions.ListDispatchesResult{SessionID: result.SessionID}
	if len(result.Dispatches) == 0 {
		return cloned
	}
	cloned.Dispatches = make([]factorysessions.DispatchSummary, len(result.Dispatches))
	for index, dispatch := range result.Dispatches {
		cloned.Dispatches[index] = cloneCanonicalDispatch(dispatch)
	}
	return cloned
}

func cloneCanonicalDispatch(
	dispatch factorysessions.DispatchSummary,
) factorysessions.DispatchSummary {
	cloned := dispatch
	cloned.ProviderSessionRefs = append([]factorysessions.ProviderSessionRef(nil), dispatch.ProviderSessionRefs...)
	cloned.OutputArtifactIDs = append([]string(nil), dispatch.OutputArtifactIDs...)
	if dispatch.Retryable != nil {
		retryable := *dispatch.Retryable
		cloned.Retryable = &retryable
	}
	if dispatch.Usage != nil {
		usage := *dispatch.Usage
		cloned.Usage = &usage
	}
	cloned.Warnings = append([]factorysessions.DispatchWarning(nil), dispatch.Warnings...)
	if dispatch.FailureDetail != nil {
		failure := *dispatch.FailureDetail
		cloned.FailureDetail = &failure
	}
	if dispatch.JavaScript != nil {
		javaScript := *dispatch.JavaScript
		cloned.JavaScript = &javaScript
	}
	return cloned
}
