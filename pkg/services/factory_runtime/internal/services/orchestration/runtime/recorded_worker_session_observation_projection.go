package runtime

import (
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func recordedObservationFromFact(fact recordedDispatchObservation, clock factory.Clock) workersessions.Observation {
	state := fact.state
	if state == "" {
		state = workersessions.StateStarting
	}
	observation := workersessions.Observation{
		WorkerSessionID:          fact.workerSessionID,
		Model:                    cloneRecordedString(fact.model),
		ReasoningEffort:          cloneRecordedString(fact.reasoningEffort),
		ProviderSessionAvailable: fact.provider != nil && fact.provider.ID != "",
		WorkIDs:                  append([]string(nil), fact.workIDs...),
		TurnID:                   fact.turnID,
		AttemptID:                fact.dispatchID,
		State:                    state,
		DurationBasis:            workersessions.DurationBasisUnavailable,
		Transcript:               workersessions.TranscriptAvailabilityUnavailable,
	}
	if fact.provider != nil {
		observation.ProviderSession = providerSessionRef(*fact.provider)
	}
	if !fact.startedAt.IsZero() {
		started := fact.startedAt.UTC()
		observation.StartedAt = &started
		if fact.endedAt != nil {
			ended := fact.endedAt.UTC()
			observation.EndedAt = &ended
			duration := ended.Sub(started)
			if duration < 0 {
				duration = 0
			}
			observation.Duration = &duration
			observation.DurationBasis = workersessions.DurationBasisRecordedTimestamps
		} else if !state.Terminal() && clock != nil {
			duration := clock.Now().Sub(started)
			if duration < 0 {
				duration = 0
			}
			observation.Duration = &duration
			observation.DurationBasis = workersessions.DurationBasisActiveClock
		}
	}
	if fact.failure != nil {
		failure := *fact.failure
		observation.Failure = &failure
	}
	return observation
}
