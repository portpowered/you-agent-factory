package service

import (
	"context"
	"encoding/json"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Completed durable attempts use the same host capture clock as archives.
// A live terminal may precede durable admission; that interval proves no end.
func (r *registry) withCapturedTerminalObservation(ctx context.Context, observation workersessions.Observation) workersessions.Observation {
	observation.EndedAt, observation.Duration = nil, nil
	observation.DurationBasis = workersessions.DurationBasisUnavailable
	captured, err := r.GetCapturedObservation(ctx, workersessions.GetObservationByWorkerSessionIDRequest{WorkerSessionID: observation.WorkerSessionID, FactorySessionID: observation.FactorySessionID})
	if err != nil || captured.AttemptID != observation.AttemptID || captured.State != observation.State {
		return observation
	}
	observation.StartedAt, observation.EndedAt, observation.Duration = captured.StartedAt, captured.EndedAt, captured.Duration
	observation.DurationBasis = captured.DurationBasis
	observation.ProviderSession, observation.ProviderSessionAvailable = captured.ProviderSession, captured.ProviderSessionAvailable
	observation.Transcript, observation.TokenUsage = captured.Transcript, captured.TokenUsage
	if captured.Failure != nil || observation.State == workersessions.StateCompleted || observation.State == workersessions.StateFailed {
		observation.Failure = captured.Failure
	}
	observation.TerminalCause = captured.TerminalCause
	observation.RecordingHealth, observation.RecordingHealthReason = captured.RecordingHealth, captured.RecordingHealthReason
	return observation
}

// Only the selected committed terminal can establish a completed association
// or failure. Prefixes and legacy terminals without those facts stay unknown.
func applyCapturedTerminalFacts(observation *workersessions.Observation, item recordings.WorkerCapturedCatalogItem) error {
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
		if json.Unmarshal(record.Payload, &draft) != nil || json.Unmarshal(draft.Payload, &terminal) != nil ||
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
	if observation.State == workersessions.StateFailed && failure.FailureCause != "" {
		cause := &workersessions.FailureCause{Kind: workersessions.FailureCauseKind(failure.FailureCause), Detail: failure.FailureDetail, AgentRunFailureClass: failure.AgentRunFailureClass}
		if cause.Validate() != nil {
			return workersessions.ErrObservationProjectionUnavailable
		}
		observation.Failure = cause
	}
	return nil
}
