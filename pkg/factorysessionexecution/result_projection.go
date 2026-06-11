package factorysessionexecution

import (
	"encoding/json"
	"strings"
)

// ProjectResultRead shapes one canonical durable result read for the requested
// mode and includeArtifacts parameters.
func ProjectResultRead(canonical ResultReadResult, session SessionReadResult, artifacts []ArtifactSummary, req ResultRequest) (ResultReadResult, error) {
	normalized, err := NormalizeResultRequest(req)
	if err != nil {
		return ResultReadResult{}, err
	}

	status := canonicalResultStatus(canonical, session)
	projected := cloneResultRead(canonical)
	projected.Mode = normalized.Mode
	projected.IncludeArtifacts = normalized.IncludeArtifacts
	projected = applyResultModeShaping(projected, canonical, session, status, normalized.Mode)
	projected = applyResultArtifactShaping(projected, artifacts, normalized.IncludeArtifacts)
	return projected, nil
}

func canonicalResultStatus(canonical ResultReadResult, session SessionReadResult) ResultStatus {
	if session.ResultSummary != nil {
		if status := strings.TrimSpace(session.ResultSummary.ResultStatus); status != "" {
			return ResultStatus(status)
		}
	}
	return canonical.ResultStatus
}

func applyResultModeShaping(projected, canonical ResultReadResult, session SessionReadResult, status ResultStatus, mode ResultMode) ResultReadResult {
	switch mode {
	case ResultModePartial:
		return shapePartialModeResult(projected, canonical, session, status)
	case ResultModeFinal:
		return shapeFinalModeResult(projected, canonical, session, status)
	default:
		projected.ResultStatus = status
		return projected
	}
}

func shapePartialModeResult(projected, canonical ResultReadResult, session SessionReadResult, status ResultStatus) ResultReadResult {
	projected.ResultStatus = status
	switch status {
	case ResultStatusPartial, ResultStatusFinal, ResultStatusFailedWithPartial:
		projected.PrimaryResult = cloneRawJSON(canonical.PrimaryResult)
		projected.Failure = cloneFailureSummary(canonical.Failure)
		projected.Availability = nil
	case ResultStatusNotReady, ResultStatusUnavailable:
		projected.PrimaryResult = nil
		projected.Failure = nil
		projected.Availability = cloneResultAvailability(canonical.Availability)
		if projected.Availability == nil && status == ResultStatusNotReady {
			projected.Availability = defaultNotReadyAvailability(session)
		}
	}
	return projected
}

func shapeFinalModeResult(projected, canonical ResultReadResult, session SessionReadResult, status ResultStatus) ResultReadResult {
	switch status {
	case ResultStatusPartial:
		if !IsTerminalLifecycleStatus(session.Status) {
			projected.ResultStatus = ResultStatusNotReady
			projected.PrimaryResult = nil
			projected.Failure = nil
			projected.Availability = cloneResultAvailability(canonical.Availability)
			if projected.Availability == nil {
				projected.Availability = defaultNotReadyAvailability(session)
			}
			return projected
		}
		projected.ResultStatus = status
		projected.PrimaryResult = cloneRawJSON(canonical.PrimaryResult)
		projected.Failure = cloneFailureSummary(canonical.Failure)
		projected.Availability = nil
	case ResultStatusFinal, ResultStatusFailedWithPartial:
		projected.ResultStatus = status
		projected.PrimaryResult = cloneRawJSON(canonical.PrimaryResult)
		projected.Failure = cloneFailureSummary(canonical.Failure)
		projected.Availability = nil
	case ResultStatusNotReady, ResultStatusUnavailable:
		projected.ResultStatus = status
		projected.PrimaryResult = nil
		projected.Failure = nil
		projected.Availability = cloneResultAvailability(canonical.Availability)
		if projected.Availability == nil && status == ResultStatusNotReady {
			projected.Availability = defaultNotReadyAvailability(session)
		}
	default:
		projected.ResultStatus = status
	}
	return projected
}

func applyResultArtifactShaping(projected ResultReadResult, artifacts []ArtifactSummary, includeArtifacts bool) ResultReadResult {
	projected.ArtifactIDs = nil
	projected.ArtifactRefs = nil

	if includeArtifacts {
		projected.ArtifactRefs = artifactRefsFromSummaries(artifacts)
		return projected
	}

	projected.ArtifactIDs = artifactIDsFromSummaries(artifacts)
	return projected
}

func artifactRefsFromSummaries(artifacts []ArtifactSummary) []ArtifactRefSummary {
	if len(artifacts) == 0 {
		return nil
	}
	refs := make([]ArtifactRefSummary, 0, len(artifacts))
	for _, artifact := range artifacts {
		refs = append(refs, ArtifactRefSummary{
			ID:          artifact.ID,
			Kind:        artifact.Kind,
			Visibility:  artifact.Visibility,
			ContentHash: artifact.ContentHash,
			SizeBytes:   artifact.SizeBytes,
		})
	}
	return refs
}

func artifactIDsFromSummaries(artifacts []ArtifactSummary) []string {
	if len(artifacts) == 0 {
		return nil
	}
	ids := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		if id := strings.TrimSpace(artifact.ID); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return ids
}

func defaultNotReadyAvailability(session SessionReadResult) *ResultAvailabilityDetail {
	message := "Session is still running."
	if IsTerminalLifecycleStatus(session.Status) {
		message = "Final result is not available."
	}
	return &ResultAvailabilityDetail{
		Reason:    "RESULT_NOT_READY",
		Message:   message,
		Retryable: !IsTerminalLifecycleStatus(session.Status),
	}
}

func cloneFailureSummary(failure *FailureSummary) *FailureSummary {
	if failure == nil {
		return nil
	}
	cloned := *failure
	return &cloned
}

func cloneResultAvailability(availability *ResultAvailabilityDetail) *ResultAvailabilityDetail {
	if availability == nil {
		return nil
	}
	cloned := *availability
	return &cloned
}

func cloneRawJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}
