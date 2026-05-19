package engine

import (
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
			cp.ConsumedTokens = interfaces.CloneTokens(v.ConsumedTokens)
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

func deepCopyCompletedDispatch(d interfaces.CompletedDispatch) interfaces.CompletedDispatch {
	cp := d
	cp.ProviderSession = interfaces.CloneProviderSessionMetadata(d.ProviderSession)
	cp.ConsumedTokens = interfaces.CloneTokens(d.ConsumedTokens)
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
	cp.ProviderSession = interfaces.CloneProviderSessionMetadata(result.ProviderSession)
	return cp
}

func deepCopyTokenMutationRecord(m interfaces.TokenMutationRecord) interfaces.TokenMutationRecord {
	cp := m
	if m.Token != nil {
		tokenCopy := interfaces.CloneToken(*m.Token)
		cp.Token = &tokenCopy
	}
	return cp
}
