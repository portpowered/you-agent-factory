package runtime

import (
	"strings"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func mergeRecordedObservations(recorded, live []workersessions.Observation) []workersessions.Observation {
	if len(recorded) == 0 && len(live) == 0 {
		return nil
	}

	// Recorded facts remain authoritative for an overlapping Worker Session,
	// while the live registry can contain a session whose association has not
	// reached the durable projection yet. Clone both sources so the read
	// decorator never mutates a service-owned observation while reconciling the
	// two views.
	merged := make([]workersessions.Observation, 0, len(recorded)+len(live))
	seen := make(map[string]struct{}, len(recorded)+len(live))
	liveBySession := make(map[string]workersessions.Observation, len(live))
	for _, observation := range live {
		liveBySession[observation.WorkerSessionID] = observation.Clone()
	}

	for _, recordedObservation := range recorded {
		if _, alreadyAdded := seen[recordedObservation.WorkerSessionID]; alreadyAdded {
			continue
		}
		seen[recordedObservation.WorkerSessionID] = struct{}{}
		mergedObservation := recordedObservation.Clone()
		if liveObservation, ok := liveBySession[recordedObservation.WorkerSessionID]; ok {
			mergeLiveObservation(&mergedObservation, liveObservation)
		}
		merged = append(merged, mergedObservation)
	}

	for _, liveObservation := range live {
		if _, alreadyAdded := seen[liveObservation.WorkerSessionID]; alreadyAdded {
			continue
		}
		seen[liveObservation.WorkerSessionID] = struct{}{}
		merged = append(merged, liveObservation.Clone())
	}
	sortObservationAttempts(merged)
	return merged
}

func mergeLiveObservation(recorded *workersessions.Observation, live workersessions.Observation) {
	if recorded == nil {
		return
	}
	mergeLiveObservationIdentity(recorded, live)
	mergeLiveObservationLifecycle(recorded, live)
	mergeLiveObservationDetails(recorded, live)
}

func mergeLiveObservationIdentity(recorded *workersessions.Observation, live workersessions.Observation) {
	if recorded.PredecessorWorkerSessionID == "" {
		recorded.PredecessorWorkerSessionID = live.PredecessorWorkerSessionID
	}
	if recorded.SuccessorWorkerSessionID == "" {
		recorded.SuccessorWorkerSessionID = live.SuccessorWorkerSessionID
	}
	if recorded.FactorySessionID == "" {
		recorded.FactorySessionID = live.FactorySessionID
	}
	recorded.Direct = live.Direct
	if len(recorded.WorkIDs) == 0 && len(live.WorkIDs) > 0 {
		recorded.WorkIDs = append([]string(nil), live.WorkIDs...)
	}
}

func mergeLiveObservationLifecycle(recorded *workersessions.Observation, live workersessions.Observation) {
	if live.StartedAt != nil {
		started := *live.StartedAt
		recorded.StartedAt = &started
	}
	if recorded.EndedAt == nil && live.EndedAt != nil {
		ended := *live.EndedAt
		recorded.EndedAt = &ended
	}
	if recorded.Duration == nil && live.Duration != nil {
		duration := *live.Duration
		recorded.Duration = &duration
		if recorded.DurationBasis == workersessions.DurationBasisUnavailable {
			recorded.DurationBasis = live.DurationBasis
		}
	}
}

func mergeLiveObservationDetails(recorded *workersessions.Observation, live workersessions.Observation) {
	if live.Model != nil && strings.TrimSpace(*live.Model) != "" {
		recorded.Model = cloneRecordedString(live.Model)
	}
	if live.ReasoningEffort != nil && strings.TrimSpace(*live.ReasoningEffort) != "" {
		recorded.ReasoningEffort = cloneRecordedString(live.ReasoningEffort)
	}
	if live.ProviderSessionAvailable {
		recorded.ProviderSession = live.ProviderSession.Clone()
		recorded.ProviderSessionAvailable = true
	}
	if live.TokenUsage != nil {
		clone := live.TokenUsage.Clone()
		recorded.TokenUsage = &clone
	}
	if live.Transcript != workersessions.TranscriptAvailabilityUnavailable {
		recorded.Transcript = live.Transcript
		recorded.Parse = live.Parse.Clone()
	}
	if recorded.Failure == nil && live.Failure != nil {
		failure := *live.Failure
		recorded.Failure = &failure
	}
}
