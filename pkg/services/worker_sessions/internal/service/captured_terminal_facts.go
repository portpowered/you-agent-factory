package service

import (
	"context"
	"encoding/json"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Durable terminal attempts use the same host capture clock as archives.
// A live terminal may precede durable admission; retain its owned lifecycle
// timing until the committed capture can supply the complete timing tuple.
func (r *registry) withCapturedTerminalObservation(ctx context.Context, observation workersessions.Observation) workersessions.Observation {
	if _, err := terminalPhase(observation.State); err != nil {
		return observation
	}
	publication := r.publicationFor(r.workerAddress(observation.WorkerSessionID, observation.FactorySessionID))
	if publication == nil {
		return observation
	}
	publication.mu.Lock()
	recordingID := publication.recordingID
	target := publication.capture
	publication.mu.Unlock()
	if recordingID == "" || r.logs == nil {
		return observation
	}
	reader, ok := r.logs.reader.(recordings.WorkerCapturedSummaryReader)
	if !ok {
		return observation
	}
	summary, err := reader.LookupWorkerSessionSummary(ctx, observation.WorkerSessionID)
	item := summary.Capture
	if err != nil || item.Catalog.RecordingID != recordingID ||
		item.Catalog.WorkerSessionID != observation.WorkerSessionID || item.Catalog.FactorySessionID != observation.FactorySessionID ||
		item.Catalog.RecordingGenerationID != target.RecordingGenerationID || item.Catalog.OwnerEpoch != target.OwnerEpoch {
		return observation
	}
	if !capturedTerminalMatchesObservation(item, observation) {
		return observation
	}
	captured, err := capturedHistoryIdentity(item, nil)
	if err != nil || captured == nil || captured.AttemptID != observation.AttemptID || captured.State != observation.State {
		return observation
	}
	applySummaryTerminalCause(summary, captured)
	if captured.StartedAt != nil && captured.EndedAt != nil && captured.Duration != nil {
		observation.StartedAt, observation.EndedAt, observation.Duration = captured.StartedAt, captured.EndedAt, captured.Duration
		observation.DurationBasis = captured.DurationBasis
	}
	observation.ProviderSession, observation.ProviderSessionAvailable = captured.ProviderSession, captured.ProviderSessionAvailable
	observation.Transcript, observation.TokenUsage = captured.Transcript, captured.TokenUsage
	if captured.Failure != nil {
		if observation.Failure == nil {
			observation.Failure = captured.Failure
		} else {
			failure := *observation.Failure
			failure.Kind, failure.Detail = captured.Failure.Kind, captured.Failure.Detail
			if captured.Failure.AgentRunFailureClass != "" {
				failure.AgentRunFailureClass = captured.Failure.AgentRunFailureClass
			}
			observation.Failure = &failure
		}
	}
	observation.TerminalCause = captured.TerminalCause
	observation.RecordingHealth, observation.RecordingHealthReason = captured.RecordingHealth, captured.RecordingHealthReason
	return observation
}

func capturedTerminalMatchesObservation(item recordings.WorkerCapturedCatalogItem, observation workersessions.Observation) bool {
	if item.Terminal == nil || item.Terminal.Position < 1 || uint64(item.Terminal.Position) > item.Catalog.CommittedPosition {
		return false
	}
	matched := false
	for _, record := range item.MetadataRecords {
		if record.ID.Position != item.Terminal.Position {
			continue
		}
		var draft workers.Draft
		var terminal workers.SessionPayload
		opening := workers.SessionPayload{WorkerSessionID: observation.WorkerSessionID, FactorySessionID: observation.FactorySessionID, AttemptID: observation.AttemptID}
		if matched || !uniqueInterruptJSONFields(record.Payload) || json.Unmarshal(record.Payload, &draft) != nil ||
			!uniqueInterruptJSONFields(draft.Payload) || json.Unmarshal(draft.Payload, &terminal) != nil ||
			!capturedTerminalDraftMatches(draft, terminal, opening, string(observation.State)) {
			return false
		}
		matched = true
	}
	return matched
}

// Only the selected committed terminal can establish a terminal association
// or failure. Prefixes and legacy terminals without those facts stay unknown.
func applyCapturedTerminalFacts(observation *workersessions.Observation, item recordings.WorkerCapturedCatalogItem) error {
	if _, err := terminalPhase(observation.State); err != nil {
		return nil
	}
	if item.Terminal == nil || item.Terminal.Position < 1 || uint64(item.Terminal.Position) > item.Catalog.CommittedPosition {
		return nil
	}
	for _, record := range item.MetadataRecords {
		if record.ID.Position != item.Terminal.Position {
			continue
		}
		var draft workers.Draft
		var terminal workers.SessionPayload
		opening := workers.SessionPayload{WorkerSessionID: observation.WorkerSessionID, FactorySessionID: observation.FactorySessionID, AttemptID: observation.AttemptID}
		// Old controlled records carry no optional facts. Their physical terminal
		// attempt can differ from the opening after a retry; leave those facts
		// unknown and let the existing causal-control projection select it.
		if observation.State == workersessions.StateCanceled || observation.State == workersessions.StateTerminated {
			var payload terminalSessionPayload
			if json.Unmarshal(record.Payload, &draft) == nil && json.Unmarshal(draft.Payload, &payload) == nil && payload.FailureCause == "" && payload.Continuation == nil {
				return nil
			}
		}
		if !capturedTerminalMatchesObservation(item, *observation) || json.Unmarshal(record.Payload, &draft) != nil ||
			!uniqueInterruptJSONFields(draft.Payload) || json.Unmarshal(draft.Payload, &terminal) != nil ||
			!capturedTerminalDraftMatches(draft, terminal, opening, string(observation.State)) {
			return workersessions.ErrObservationProjectionUnavailable
		}
		return applyCapturedTerminalPayload(observation, terminal, draft.Payload, item.Health)
	}
	return nil
}

func applyCapturedTerminalPayload(observation *workersessions.Observation, terminal workers.SessionPayload, raw json.RawMessage, health recordings.WorkerRecordingStatus) error {
	if terminal.Continuation != nil {
		ref := providers.SessionRef{Provider: providers.ID(terminal.Continuation.Provider), Kind: terminal.Continuation.Kind, ID: terminal.Continuation.ID}
		if ref.Validate() != nil {
			return workersessions.ErrObservationProjectionUnavailable
		}
		observation.ProviderSession, observation.ProviderSessionAvailable = ref, true
		if health == recordings.WorkerRecordingStatusComplete {
			observation.Transcript = workersessions.TranscriptAvailabilityAvailable
		}
	}
	var failure terminalSessionPayload
	if json.Unmarshal(raw, &failure) != nil {
		return workersessions.ErrObservationProjectionUnavailable
	}
	if observation.State != workersessions.StateCompleted && failure.FailureCause != "" {
		cause := &workersessions.FailureCause{Kind: workersessions.FailureCauseKind(failure.FailureCause), Detail: failure.FailureDetail, AgentRunFailureClass: failure.AgentRunFailureClass}
		if cause.Validate() != nil {
			return workersessions.ErrObservationProjectionUnavailable
		}
		observation.Failure = cause
	}
	return nil
}
