package service

import (
	"strings"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

// attachLiveProviderAssociation preserves a provider reference only when the
// current process still owns the Worker Session association. The durable
// recording remains the source of lifecycle and output facts; after restart,
// loadObservationState returns false and the same Worker-ID read stays
// provider-neutral.
func (r *registry) attachLiveProviderAssociation(
	id string,
	projected workersessions.Observation,
) workersessions.Observation {
	session, _, ok := r.loadObservationState(id)
	if !ok || session.ProviderSessionAssociation == nil {
		return projected
	}
	association := session.ProviderSessionAssociation
	projected.ProviderSession = association.Reference.Clone()
	projected.ProviderSessionAvailable = true
	if strings.TrimSpace(association.TurnID) != "" {
		projected.TurnID = association.TurnID
	}
	if strings.TrimSpace(association.AttemptID) != "" {
		projected.AttemptID = association.AttemptID
	}
	return projected
}

func mergeLiveObservation(
	durable workersessions.Observation,
	live workersessions.Observation,
) workersessions.Observation {
	mergeLiveObservationIdentity(&durable, live)
	mergeLiveObservationExecution(&durable, live)
	mergeLiveObservationAvailability(&durable, live)
	return durable
}

func mergeLiveObservationIdentity(
	durable *workersessions.Observation,
	live workersessions.Observation,
) {
	if live.PredecessorWorkerSessionID != "" {
		durable.PredecessorWorkerSessionID = live.PredecessorWorkerSessionID
	}
	if live.SuccessorWorkerSessionID != "" {
		durable.SuccessorWorkerSessionID = live.SuccessorWorkerSessionID
	}
	if live.Model != nil {
		durable.Model = live.Model
	}
	if live.ReasoningEffort != nil {
		durable.ReasoningEffort = live.ReasoningEffort
	}
	if live.TokenUsage != nil {
		durable.TokenUsage = live.TokenUsage
	}
	if live.Direct {
		durable.Direct = true
	}
	if strings.TrimSpace(live.FactorySessionID) != "" {
		durable.FactorySessionID = live.FactorySessionID
	}
	if len(live.WorkIDs) > 0 {
		durable.WorkIDs = append([]string(nil), live.WorkIDs...)
	}
}

func mergeLiveObservationExecution(
	durable *workersessions.Observation,
	live workersessions.Observation,
) {
	if live.TurnID != "" {
		durable.TurnID = live.TurnID
	}
	if live.AttemptID != "" {
		durable.AttemptID = live.AttemptID
	}
	if live.State != "" {
		durable.State = live.State
	}
	if live.StartedAt != nil {
		durable.StartedAt = live.StartedAt
	}
	if live.EndedAt != nil {
		durable.EndedAt = live.EndedAt
	}
	if live.Duration != nil {
		durable.Duration = live.Duration
	}
	if live.DurationBasis != "" && live.DurationBasis != workersessions.DurationBasisUnavailable {
		durable.DurationBasis = live.DurationBasis
	}
	if live.Failure != nil {
		durable.Failure = live.Failure
	}
}

func mergeLiveObservationAvailability(
	durable *workersessions.Observation,
	live workersessions.Observation,
) {
	if live.ProviderSessionAvailable {
		durable.ProviderSession = live.ProviderSession.Clone()
		durable.ProviderSessionAvailable = true
	}
	if live.TurnUsage != nil {
		usage := live.TurnUsage.Clone()
		durable.TurnUsage = &usage
	}
	if live.Transcript == workersessions.TranscriptAvailabilityAvailable {
		durable.Transcript = live.Transcript
		durable.Parse = live.Parse.Clone()
	}
}

func (r *registry) attachLiveProviderTranscript(
	id string,
	result workersessions.ReadTranscriptResult,
) workersessions.ReadTranscriptResult {
	session, _, ok := r.loadObservationState(id)
	if !ok || session.ProviderSessionAssociation == nil {
		return result
	}
	association := session.ProviderSessionAssociation
	result.ProviderSession = association.Reference.Clone()
	if strings.TrimSpace(association.TurnID) != "" {
		result.TurnID = association.TurnID
	}
	if strings.TrimSpace(association.AttemptID) != "" {
		result.AttemptID = association.AttemptID
	}
	return result
}
