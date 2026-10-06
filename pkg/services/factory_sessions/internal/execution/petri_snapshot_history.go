package factorysessionexecution

import (
	"errors"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"go.uber.org/zap"
)

// coalescePetriMutationHistory retains a recovery baseline, not a second event
// ledger. A full CREATE/MOVE token replaces all earlier state for that identity.
// Keep a following CONSUME as well: its tokenless retirement must not resurrect
// the baseline, and terminal summaries may still need the baseline's identity.
// Records without a known baseline remain lossless for legacy compatibility.
func coalescePetriMutationHistory(mutations []interfaces.TokenMutationRecord) []interfaces.TokenMutationRecord {
	latestFull := make(map[string]int)
	for index, mutation := range mutations {
		if mutation.Token == nil || mutation.Token.ID == "" {
			continue
		}
		if mutation.Type == interfaces.MutationCreate || mutation.Type == interfaces.MutationMove {
			latestFull[petriMutationTokenID(mutation)] = index
		}
	}
	retained := make([]interfaces.TokenMutationRecord, 0, len(latestFull))
	for index, mutation := range mutations {
		if baseline, known := latestFull[petriMutationTokenID(mutation)]; known && index < baseline {
			continue
		}
		retained = append(retained, clonePetriMutationRecord(mutation))
	}
	return retained
}

type pendingPetriHistory struct {
	mutations []interfaces.TokenMutationRecord
	summaries []PetriTokenSummary
}

// applyPendingPetriHistory overlays only unpublished token state. Lifecycle,
// events, dispatches, and other projections still come from the last good state.
// Callers hold s.mu throughout candidate construction and publication.
func (s *JavaScriptRuntimeService) applyPendingPetriHistory(candidate *runtimeSessionState) {
	if pending, ok := s.pendingPetriHistory[candidate.session.SessionID]; ok {
		candidate.petriMutations = clonePetriMutations(pending.mutations)
		candidate.petriSummaries = clonePetriTokenSummaries(pending.summaries)
	}
}

func (s *JavaScriptRuntimeService) persistPetriCandidate(candidate runtimeSessionState) error {
	id := candidate.session.SessionID
	err := s.persistSessionSnapshot(candidate)
	var sizeErr *SnapshotSizeLimitError
	if errors.As(err, &sizeErr) {
		_, degraded := s.pendingPetriHistory[id]
		if s.pendingPetriHistory == nil {
			s.pendingPetriHistory = make(map[string]pendingPetriHistory)
		}
		s.pendingPetriHistory[id] = pendingPetriHistory{
			mutations: clonePetriMutations(candidate.petriMutations),
			summaries: clonePetriTokenSummaries(candidate.petriSummaries),
		}
		if !degraded && s.persistenceWarningLogger != nil {
			s.persistenceWarningLogger.Error("durable Factory Session persistence degraded",
				zap.String("code", "durable_session_snapshot_size_limit"),
				zap.String("session_id", id), zap.Int("observed_bytes", sizeErr.ActualBytes),
				zap.Int("max_bytes", sizeErr.MaxBytes), zap.Bool("persistence_degraded", true))
		}
		return err
	}
	if err != nil {
		return err
	}
	if _, degraded := s.pendingPetriHistory[id]; degraded {
		delete(s.pendingPetriHistory, id)
		if s.persistenceWarningLogger != nil {
			s.persistenceWarningLogger.Warn("durable Factory Session persistence recovered",
				zap.String("code", "durable_session_snapshot_recovered"),
				zap.String("session_id", id), zap.Bool("persistence_degraded", false))
		}
	}
	return nil
}
