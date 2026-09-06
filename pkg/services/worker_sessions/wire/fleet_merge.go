package wire

import (
	"time"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

// mergeFleetObservation combines duplicate views of one Worker Session. A
// Factory-scoped decorator may own canonical identity and recording health,
// while the process-local registry still owns live timing or a provider
// association. Keep the first source's authoritative non-empty facts and
// fill only gaps from the other source so catalog order cannot erase useful
// optional projection data.
func mergeFleetObservation(
	current workersessions.Observation,
	candidate workersessions.Observation,
) workersessions.Observation {
	merged := current.Clone()
	mergeFleetIdentity(&merged, candidate)
	mergeFleetTiming(&merged, candidate)
	mergeFleetHealth(&merged, candidate)
	return merged
}

func mergeFleetIdentity(
	merged *workersessions.Observation,
	candidate workersessions.Observation,
) {
	if merged.PredecessorWorkerSessionID == "" {
		merged.PredecessorWorkerSessionID = candidate.PredecessorWorkerSessionID
	}
	if merged.SuccessorWorkerSessionID == "" {
		merged.SuccessorWorkerSessionID = candidate.SuccessorWorkerSessionID
	}
	if merged.Model == nil {
		merged.Model = cloneFleetString(candidate.Model)
	}
	if merged.ReasoningEffort == nil {
		merged.ReasoningEffort = cloneFleetString(candidate.ReasoningEffort)
	}
	if merged.FactorySessionID == "" {
		merged.FactorySessionID = candidate.FactorySessionID
	}
	if !merged.ProviderSessionAvailable && candidate.ProviderSessionAvailable {
		merged.ProviderSession = candidate.ProviderSession.Clone()
		merged.ProviderSessionAvailable = true
	}
	if len(merged.WorkIDs) == 0 && len(candidate.WorkIDs) > 0 {
		merged.WorkIDs = append([]string(nil), candidate.WorkIDs...)
	}
	if merged.TurnID == "" {
		merged.TurnID = candidate.TurnID
	}
	if merged.AttemptID == "" {
		merged.AttemptID = candidate.AttemptID
	}
}

func mergeFleetTiming(
	merged *workersessions.Observation,
	candidate workersessions.Observation,
) {
	if merged.StartedAt == nil {
		merged.StartedAt = cloneFleetTime(candidate.StartedAt)
	}
	if merged.EndedAt == nil {
		merged.EndedAt = cloneFleetTime(candidate.EndedAt)
	}
	if merged.Duration == nil {
		merged.Duration = cloneFleetDuration(candidate.Duration)
	}
	if (merged.DurationBasis == "" || merged.DurationBasis == workersessions.DurationBasisUnavailable) && candidate.DurationBasis != workersessions.DurationBasisUnavailable {
		merged.DurationBasis = candidate.DurationBasis
	}
	if merged.TokenUsage == nil && candidate.TokenUsage != nil {
		usage := candidate.TokenUsage.Clone()
		merged.TokenUsage = &usage
	}
	if merged.TurnUsage == nil && candidate.TurnUsage != nil {
		usage := candidate.TurnUsage.Clone()
		merged.TurnUsage = &usage
	}
	if (merged.Transcript == "" || merged.Transcript == workersessions.TranscriptAvailabilityUnavailable) && candidate.Transcript != workersessions.TranscriptAvailabilityUnavailable {
		merged.Transcript = candidate.Transcript
		merged.Parse = candidate.Parse.Clone()
	}
}

func mergeFleetHealth(
	merged *workersessions.Observation,
	candidate workersessions.Observation,
) {
	if merged.RecordingHealth == "" {
		merged.RecordingHealth = candidate.RecordingHealth
		merged.RecordingHealthReason = candidate.RecordingHealthReason
	}
	if merged.Failure == nil && candidate.Failure != nil {
		failure := *candidate.Failure
		merged.Failure = &failure
	}
	if merged.ConfirmationState != workersessions.ConfirmationStateConfirmed && candidate.ConfirmationState == workersessions.ConfirmationStateConfirmed {
		merged.ConfirmationState = candidate.ConfirmationState
	}
}

func cloneFleetString(value *string) *string {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneFleetTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

func cloneFleetDuration(value *time.Duration) *time.Duration {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
