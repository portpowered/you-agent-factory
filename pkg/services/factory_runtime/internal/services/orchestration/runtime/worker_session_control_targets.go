package runtime

import (
	"sort"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// capturedWorkerSessionControlTargets is the immutable set one committed
// Factory-turn control owns. The Factory Session identity is represented by
// the session-scoped canonical ledger captured below; associations from a
// different Factory Session must therefore be read from a different ledger.
//
// The later fan-out stories retain this value with the committed control. They
// must not call captureAssociatedWorkerSessionTargets again on a retry, or a
// retry could include a child associated after the control linearized.
type capturedWorkerSessionControlTargets struct {
	turnID           string
	workerSessionIDs []string
}

// workerSessionIDsSnapshot returns a detached deterministic target order.
func (c capturedWorkerSessionControlTargets) workerSessionIDsSnapshot() []string {
	return append([]string(nil), c.workerSessionIDs...)
}

// captureAssociatedWorkerSessionTargets selects exactly the Worker Sessions
// canonically associated with turnID at one ledger snapshot. It deliberately
// consumes only the W4 association event rather than tracking dispatches or
// retaining a second association registry. RequestID on that event is the
// Factory invocation's immutable turn correlation supplied by ACP.
func captureAssociatedWorkerSessionTargets(
	ledger recordings.RuntimeLedger,
	turnID string,
) capturedWorkerSessionControlTargets {
	turnID = strings.TrimSpace(turnID)
	if ledger == nil || turnID == "" {
		return capturedWorkerSessionControlTargets{turnID: turnID}
	}
	return selectAssociatedWorkerSessionTargets(ledger.CanonicalEvents(), turnID)
}

func selectAssociatedWorkerSessionTargets(
	events []interfaces.FactoryEvent,
	turnID string,
) capturedWorkerSessionControlTargets {
	turnID = strings.TrimSpace(turnID)
	captured := capturedWorkerSessionControlTargets{turnID: turnID}
	if turnID == "" {
		return captured
	}

	type candidate struct {
		sequence        int
		workerSessionID string
		eventID         string
		index           int
	}
	candidates := make([]candidate, 0)
	for index, event := range events {
		if event.Type != interfaces.FactoryEventTypeDispatchWorkerSessionAssoc ||
			event.Context.DispatchID == nil || strings.TrimSpace(*event.Context.DispatchID) == "" ||
			event.Context.RequestID == nil || strings.TrimSpace(*event.Context.RequestID) != turnID {
			continue
		}
		var association interfaces.DispatchWorkerSessionAssociationEventPayload
		if event.DecodePayload(&association) != nil {
			continue
		}
		workerSessionID := strings.TrimSpace(association.WorkerSessionID)
		if workerSessionID == "" {
			continue
		}
		candidates = append(candidates, candidate{
			sequence:        event.Context.Sequence,
			workerSessionID: workerSessionID,
			eventID:         event.Id,
			index:           index,
		})
	}

	// Canonical sequence is the normal order. The remaining keys make a
	// partially reconstructed or test fixture event stream deterministic too.
	sort.Slice(candidates, func(left, right int) bool {
		if candidates[left].sequence != candidates[right].sequence {
			return candidates[left].sequence < candidates[right].sequence
		}
		if candidates[left].eventID != candidates[right].eventID {
			return candidates[left].eventID < candidates[right].eventID
		}
		if candidates[left].workerSessionID != candidates[right].workerSessionID {
			return candidates[left].workerSessionID < candidates[right].workerSessionID
		}
		return candidates[left].index < candidates[right].index
	})

	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		if _, duplicate := seen[candidate.workerSessionID]; duplicate {
			continue
		}
		seen[candidate.workerSessionID] = struct{}{}
		captured.workerSessionIDs = append(captured.workerSessionIDs, candidate.workerSessionID)
	}
	return captured
}
