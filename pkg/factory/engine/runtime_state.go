package engine

import (
	"maps"

	"github.com/portpowered/infinite-you/pkg/buffers"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

// RuntimeState is the unified mutable state container for the engine loop.
// All per-tick state lives here so it can be snapshotted atomically.
type RuntimeState struct {
	Marking              *petri.Marking                              `json:"marking"`
	Dispatches           map[string]*interfaces.DispatchEntry        `json:"dispatches"`
	InFlightCount        int                                         `json:"in_flight_count"` // accurate count even when Dispatches map has key collisions
	Results              []interfaces.WorkResult                     `json:"results"`
	ResultBuffer         *buffers.TypedBuffer[interfaces.WorkResult] `json:"-"`
	DispatchHistory      []interfaces.CompletedDispatch              `json:"dispatch_history"`
	ActiveThrottlePauses []interfaces.ActiveThrottlePause            `json:"active_throttle_pauses,omitempty"`
	TickCount            int                                         `json:"tick_count"`
}

// Snapshot produces an immutable deep copy of the RuntimeState.
func (rs *RuntimeState) Snapshot() interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	snap := interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		TickCount:     rs.TickCount,
		InFlightCount: rs.InFlightCount,
	}

	// Deep copy marking via its own Snapshot method.
	if rs.Marking != nil {
		snap.Marking = rs.Marking.Snapshot()
	}

	// Deep copy dispatches.
	if rs.Dispatches != nil {
		snap.Dispatches = make(map[string]*interfaces.DispatchEntry, len(rs.Dispatches))
		for k, v := range rs.Dispatches {
			cp := *v
			if v.ConsumedTokens != nil {
				cp.ConsumedTokens = make([]interfaces.Token, len(v.ConsumedTokens))
				for i := range v.ConsumedTokens {
					cp.ConsumedTokens[i] = deepCopyToken(v.ConsumedTokens[i])
				}
			}
			if v.HeldMutations != nil {
				cp.HeldMutations = make([]interfaces.MarkingMutation, len(v.HeldMutations))
				copy(cp.HeldMutations, v.HeldMutations)
			}
			snap.Dispatches[k] = &cp
		}
	}

	// Deep copy results.
	if rs.Results != nil {
		snap.Results = make([]interfaces.WorkResult, len(rs.Results))
		for i := range rs.Results {
			snap.Results[i] = deepCopyWorkResult(rs.Results[i])
		}
	}

	// Deep copy dispatch history.
	if rs.DispatchHistory != nil {
		snap.DispatchHistory = make([]interfaces.CompletedDispatch, len(rs.DispatchHistory))
		for i := range rs.DispatchHistory {
			snap.DispatchHistory[i] = deepCopyCompletedDispatch(rs.DispatchHistory[i])
		}
	}

	if rs.ActiveThrottlePauses != nil {
		snap.ActiveThrottlePauses = make([]interfaces.ActiveThrottlePause, len(rs.ActiveThrottlePauses))
		copy(snap.ActiveThrottlePauses, rs.ActiveThrottlePauses)
	}

	return snap
}

// deepCopyTokenHistory creates a deep copy of a petri.TokenHistory.
func deepCopyTokenHistory(h interfaces.TokenHistory) interfaces.TokenHistory {
	cp := h
	if h.TotalVisits != nil {
		cp.TotalVisits = make(map[string]int, len(h.TotalVisits))
		maps.Copy(cp.TotalVisits, h.TotalVisits)
	}
	if h.ConsecutiveFailures != nil {
		cp.ConsecutiveFailures = make(map[string]int, len(h.ConsecutiveFailures))
		maps.Copy(cp.ConsecutiveFailures, h.ConsecutiveFailures)
	}
	if h.PlaceVisits != nil {
		cp.PlaceVisits = make(map[string]int, len(h.PlaceVisits))
		maps.Copy(cp.PlaceVisits, h.PlaceVisits)
	}
	if h.FailureLog != nil {
		cp.FailureLog = make([]interfaces.FailureRecord, len(h.FailureLog))
		copy(cp.FailureLog, h.FailureLog)
	}
	return cp
}

func deepCopyToken(t interfaces.Token) interfaces.Token {
	cp := t
	if t.Color.Tags != nil {
		cp.Color.Tags = make(map[string]string, len(t.Color.Tags))
		maps.Copy(cp.Color.Tags, t.Color.Tags)
	}
	if t.Color.Relations != nil {
		cp.Color.Relations = make([]interfaces.Relation, len(t.Color.Relations))
		copy(cp.Color.Relations, t.Color.Relations)
	}
	if t.Color.Payload != nil {
		cp.Color.Payload = make([]byte, len(t.Color.Payload))
		copy(cp.Color.Payload, t.Color.Payload)
	}
	cp.History = deepCopyTokenHistory(t.History)
	return cp
}

func deepCopyCompletedDispatch(d interfaces.CompletedDispatch) interfaces.CompletedDispatch {
	cp := d
	cp.ProviderSession = cloneProviderSession(d.ProviderSession)
	if d.ConsumedTokens != nil {
		cp.ConsumedTokens = make([]interfaces.Token, len(d.ConsumedTokens))
		for i := range d.ConsumedTokens {
			cp.ConsumedTokens[i] = deepCopyToken(d.ConsumedTokens[i])
		}
	}
	if d.OutputMutations != nil {
		cp.OutputMutations = make([]interfaces.TokenMutationRecord, len(d.OutputMutations))
		for i := range d.OutputMutations {
			cp.OutputMutations[i] = deepCopyTokenMutationRecord(d.OutputMutations[i])
		}
	}
	return cp
}

func deepCopyWorkResult(result interfaces.WorkResult) interfaces.WorkResult {
	cp := result
	cp.ProviderSession = cloneProviderSession(result.ProviderSession)
	return cp
}

func cloneProviderSession(session *interfaces.ProviderSessionMetadata) *interfaces.ProviderSessionMetadata {
	if session == nil {
		return nil
	}
	clone := *session
	return &clone
}

func deepCopyTokenMutationRecord(m interfaces.TokenMutationRecord) interfaces.TokenMutationRecord {
	cp := m
	if m.Token != nil {
		tokenCopy := deepCopyToken(*m.Token)
		cp.Token = &tokenCopy
	}
	return cp
}
