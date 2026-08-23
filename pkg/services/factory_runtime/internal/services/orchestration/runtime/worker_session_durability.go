package runtime

import (
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func completedFlushWatermarkReader(
	ledger recordings.RuntimeLedger,
) recordings.CompletedFlushWatermarkReader {
	if ledger == nil {
		return nil
	}
	reader, _ := ledger.(recordings.CompletedFlushWatermarkReader)
	return reader
}

// annotateRecordedFact attaches the canonical stream identity to the state
// cursor before the detached observation crosses the runtime boundary.
func (s *recordedWorkerSessionObservation) annotateRecordedFact(
	fact recordedDispatchObservation,
) recordedDispatchObservation {
	if s == nil || s.ledger == nil {
		return fact
	}
	fact.streamGenerationID = strings.TrimSpace(s.ledger.StreamGenerationID())
	return fact
}

// applyConfirmation samples one completed-flush watermark for the response.
// The process-local registry remains the fallback source, so every observation
// starts UNCONFIRMED and can become CONFIRMED only when its recorded state
// cursor is known and covered by the generation-matching watermark.
func (s *recordedWorkerSessionObservation) applyConfirmation(
	observations []workersessions.Observation,
) {
	if len(observations) == 0 {
		return
	}
	for index := range observations {
		observations[index].ConfirmationState = workersessions.ConfirmationStateUnconfirmed
	}
	if s == nil || s.ledger == nil || s.durability == nil {
		return
	}
	generationID := strings.TrimSpace(s.ledger.StreamGenerationID())
	if generationID == "" {
		return
	}
	watermark, ok := s.durability.CompletedFlushWatermark(generationID)
	if !ok || strings.TrimSpace(watermark.StreamGenerationID) != generationID {
		return
	}
	for index := range observations {
		observation := &observations[index]
		if observation.StateSequenceKnown &&
			observation.StreamGenerationID == generationID &&
			observation.StateSequence <= int64(watermark.Sequence) {
			observation.ConfirmationState = workersessions.ConfirmationStateConfirmed
		}
	}
}

func (s *recordedWorkerSessionObservation) confirmedObservation(
	observation workersessions.Observation,
) workersessions.Observation {
	values := []workersessions.Observation{observation}
	s.applyConfirmation(values)
	return values[0]
}

// recordedDispatchStateCursor returns the latest canonical dispatch lifecycle
// event for the dispatch. It is the cursor responsible for the projected
// Worker Session state or terminal outcome, rather than merely the association
// event that made the Worker Session addressable.
func recordedDispatchStateCursor(
	events []interfaces.FactoryEvent,
	dispatchID string,
) (int64, bool) {
	dispatchID = strings.TrimSpace(dispatchID)
	if dispatchID == "" {
		return 0, false
	}
	var (
		sequence int64
		found    bool
	)
	for _, event := range cloneAndSortFactoryEvents(events) {
		if stringPointerValue(event.Context.DispatchID) != dispatchID {
			continue
		}
		switch event.Type {
		case interfaces.FactoryEventTypeDispatchRequest,
			interfaces.FactoryEventTypeDispatchWorkerSessionAssoc,
			interfaces.FactoryEventTypeDispatchQueued,
			interfaces.FactoryEventTypeDispatchResponse,
			interfaces.FactoryEventTypeDispatchInterrupted,
			interfaces.FactoryEventTypeDispatchReconciled:
			sequence = int64(event.Context.Sequence)
			found = true
		}
	}
	return sequence, found
}
