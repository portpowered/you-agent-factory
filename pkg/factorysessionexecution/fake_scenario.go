package factorysessionexecution

import "encoding/json"

// FakeScenario is one deterministic durable-session projection bundle used by
// FakeService. Scenarios are keyed by execution requestId for start routing.
type FakeScenario struct {
	ID        string
	RequestID string
	Session   SessionReadResult
	Dispatches []DispatchSummary
	DispatchDetails map[string]DispatchDetail
	Artifacts []ArtifactSummary
	ArtifactDetails map[string]ArtifactDetail
	Result    ResultReadResult
	Events    []json.RawMessage
	AsyncStart *AsyncStartResult
	SyncStart  *SyncStartResult
	ListSummary *DurableSessionListSummary
}

type fakeSessionState struct {
	scenarioID      string
	session         SessionReadResult
	dispatches      []DispatchSummary
	dispatchDetails map[string]DispatchDetail
	artifacts       []ArtifactSummary
	artifactDetails map[string]ArtifactDetail
	result          ResultReadResult
	events          []json.RawMessage
}

func fakeSessionStateFromScenario(scenario FakeScenario) *fakeSessionState {
	state := &fakeSessionState{
		scenarioID:      scenario.ID,
		session:         cloneSessionRead(scenario.Session),
		dispatches:      cloneDispatchSummaries(scenario.Dispatches),
		dispatchDetails: cloneDispatchDetails(scenario.DispatchDetails),
		artifacts:       cloneArtifactSummaries(scenario.Artifacts),
		artifactDetails: cloneArtifactDetails(scenario.ArtifactDetails),
		result:          cloneResultRead(scenario.Result),
		events:          append([]json.RawMessage(nil), scenario.Events...),
	}
	if len(state.events) == 0 {
		state.events = deriveProjectionEvents(state.session, state.result)
	}
	return state
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

func deriveProjectionEvents(session SessionReadResult, result ResultReadResult) []json.RawMessage {
	events := []json.RawMessage{
		json.RawMessage(`{"type":"SESSION_STARTED","payload":{"sessionId":"` + session.SessionID + `"}}`),
	}
	if result.ResultStatus != "" {
		payload, err := json.Marshal(map[string]any{
			"type": "SESSION_RESULT_UPDATED",
			"payload": map[string]any{
				"sessionId":    session.SessionID,
				"resultStatus": string(result.ResultStatus),
			},
		})
		if err == nil {
			events = append(events, payload)
		}
	}
	if IsTerminalLifecycleStatus(session.Status) {
		payload, err := json.Marshal(map[string]any{
			"type": "SESSION_COMPLETED",
			"payload": map[string]any{
				"sessionId": session.SessionID,
				"status":    string(session.Status),
			},
		})
		if err == nil {
			events = append(events, payload)
		}
	}
	return events
}
