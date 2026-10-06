package factorysessionexecution

import (
	"encoding/json"
	"errors"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// RecordPetriTokenMutations appends applied Petri transition records through
// the canonical Factory Session persistence owner. Persistence succeeds before
// the updated history becomes visible to live readers.
func (s *JavaScriptRuntimeService) RecordPetriTokenMutations(
	sessionID string,
	mutations []interfaces.TokenMutationRecord,
) error {
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return err
	}
	if len(mutations) == 0 {
		return nil
	}
	if _, err := s.snapshotSessionState(id); err != nil && !errors.Is(err, ErrSessionNotFound) {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.sessions[id]
	if !ok {
		initial := projectPetriRunningSessionState(id, s.nowForSession(id))
		state = &initial
	}
	candidate := cloneRuntimeSessionState(state)
	s.applyPendingPetriHistory(&candidate)
	candidate.petriMutations = append(candidate.petriMutations, clonePetriMutations(mutations)...)
	compactRuntimePetriHistory(&candidate)
	if err := s.persistPetriCandidate(candidate); err != nil {
		return err
	}
	if ok {
		*state = candidate
	} else {
		s.sessions[id] = &candidate
	}
	return nil
}

func (s *JavaScriptRuntimeService) syncStartFromState(state runtimeSessionState) SyncStartResult {
	async := s.asyncStartFromState(state)
	result := SyncStartResult{AsyncStartResult: async}
	if state.result.Availability != nil && state.result.Availability.Reason == "SYNC_WAIT_TIMED_OUT" {
		result.SyncOutcome = SyncOutcomeTimedOut
		result.TimedOut = true
		return result
	}
	if IsTerminalLifecycleStatus(state.session.Status) {
		result.SyncOutcome = SyncOutcomeCompleted
		projected, err := ProjectResultRead(state.result, state.session, state.artifacts, ResultRequest{
			Mode: ResultModeFinal,
		})
		if err == nil {
			if encoded, err := json.Marshal(projected); err == nil {
				result.Result = encoded
			}
		}
	}
	return result
}

func (s *JavaScriptRuntimeService) asyncStartFromState(state runtimeSessionState) AsyncStartResult {
	return AsyncStartResult{
		SessionID:        state.session.SessionID,
		Status:           string(state.session.Status),
		OrchestratorKind: state.session.OrchestratorKind,
		Dialect:          state.session.Dialect,
		ResolvedSource:   state.session.ResolvedSource,
		SourceHash:       state.session.SourceHash,
		Policy:           state.session.Policy,
		Links:            state.session.Links,
	}
}

func defaultUnavailableAvailability() *ResultAvailabilityDetail {
	return &ResultAvailabilityDetail{
		Reason:    "UNAVAILABLE",
		Message:   "session result is unavailable",
		Retryable: false,
	}
}

func marshalStartArgs(args map[string]any) (json.RawMessage, error) {
	if len(args) == 0 {
		return nil, nil
	}
	encoded, err := json.Marshal(args)
	if err != nil {
		return nil, NewValidationError("args", "args must be JSON-compatible")
	}
	return encoded, nil
}

func workflowMetadataFromResolved(resolved ResolvedSource, req StartRequest) map[string]string {
	metadata := map[string]string{}
	if req.Source.InlineWorkflow != nil {
		for key, value := range req.Source.InlineWorkflow.Metadata {
			metadata[key] = value
		}
	}
	for key, value := range resolved.Metadata {
		metadata[key] = value
	}
	if name := strings.TrimSpace(req.Source.WorkflowName); name != "" {
		metadata["name"] = name
	} else if _, ok := metadata["name"]; !ok {
		base := strings.TrimSpace(resolved.SourceRef)
		if base != "" {
			metadata["name"] = base
		}
	}
	return metadata
}
