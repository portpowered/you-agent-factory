package factorysessionexecution

import "encoding/json"

func cloneFakeSessionState(state *fakeSessionState) *fakeSessionState {
	if state == nil {
		return nil
	}
	cloned := &fakeSessionState{
		scenarioID:      state.scenarioID,
		session:         cloneSessionRead(state.session),
		dispatches:      cloneDispatchSummaries(state.dispatches),
		dispatchDetails: cloneDispatchDetails(state.dispatchDetails),
		artifacts:       cloneArtifactSummaries(state.artifacts),
		artifactDetails: cloneArtifactDetails(state.artifactDetails),
		result:          cloneResultRead(state.result),
		events:          make([]json.RawMessage, len(state.events)),
	}
	for index, event := range state.events {
		cloned.events[index] = append(json.RawMessage(nil), event...)
	}
	return cloned
}

func cloneSessionRead(session SessionReadResult) SessionReadResult {
	cloned := session
	cloned.ResolvedSource = cloneResolvedSource(session.ResolvedSource)
	cloned.Policy = clonePolicyProjection(session.Policy)
	if session.Progress != nil {
		progress := *session.Progress
		cloned.Progress = &progress
	}
	if session.ResultSummary != nil {
		summary := *session.ResultSummary
		cloned.ResultSummary = &summary
	}
	if session.Failure != nil {
		failure := *session.Failure
		cloned.Failure = &failure
	}
	if session.Lifecycle != nil {
		lifecycle := *session.Lifecycle
		cloned.Lifecycle = &lifecycle
	}
	if session.Budgets != nil {
		budgets := *session.Budgets
		cloned.Budgets = &budgets
	}
	cloned.Usage = cloneSessionUsage(session.Usage)
	cloned.PhaseSummaries = append([]PhaseSummary(nil), session.PhaseSummaries...)
	cloned.ArtifactRefs = append([]ArtifactRefSummary(nil), session.ArtifactRefs...)
	cloned.Links = session.Links
	return cloned
}

func cloneResolvedSource(source ResolvedSource) ResolvedSource {
	cloned := source
	cloned.ResolutionOrder = append([]string(nil), source.ResolutionOrder...)
	if len(source.Metadata) > 0 {
		cloned.Metadata = make(map[string]string, len(source.Metadata))
		for key, value := range source.Metadata {
			cloned.Metadata[key] = value
		}
	}
	return cloned
}

func clonePolicyProjection(policy PolicyProjection) PolicyProjection {
	cloned := PolicyProjection{
		EffectiveHash: policy.EffectiveHash,
	}
	if len(policy.Requested) > 0 {
		cloned.Requested = cloneArgs(policy.Requested)
	}
	if len(policy.Effective) > 0 {
		cloned.Effective = cloneArgs(policy.Effective)
	}
	return cloned
}

func cloneDispatchSummaries(dispatches []DispatchSummary) []DispatchSummary {
	if len(dispatches) == 0 {
		return nil
	}
	cloned := make([]DispatchSummary, len(dispatches))
	copy(cloned, dispatches)
	return cloned
}

func cloneDispatchDetails(details map[string]DispatchDetail) map[string]DispatchDetail {
	if len(details) == 0 {
		return nil
	}
	cloned := make(map[string]DispatchDetail, len(details))
	for key, value := range details {
		cloned[key] = value
	}
	return cloned
}

func cloneArtifactSummaries(artifacts []ArtifactSummary) []ArtifactSummary {
	if len(artifacts) == 0 {
		return nil
	}
	cloned := make([]ArtifactSummary, len(artifacts))
	copy(cloned, artifacts)
	return cloned
}

func cloneArtifactDetails(details map[string]ArtifactDetail) map[string]ArtifactDetail {
	if len(details) == 0 {
		return nil
	}
	cloned := make(map[string]ArtifactDetail, len(details))
	for key, value := range details {
		cloned[key] = value
	}
	return cloned
}

func cloneResultRead(result ResultReadResult) ResultReadResult {
	cloned := result
	if len(result.PrimaryResult) > 0 {
		cloned.PrimaryResult = append(json.RawMessage(nil), result.PrimaryResult...)
	}
	cloned.ArtifactIDs = append([]string(nil), result.ArtifactIDs...)
	cloned.ArtifactRefs = append([]ArtifactRefSummary(nil), result.ArtifactRefs...)
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
