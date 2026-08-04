package mapping

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	acpsdk "github.com/coder/acp-go-sdk"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// ChildAssociation is the bounded, public correlation fact from a canonical
// dispatch-to-Worker-Session association. It is deliberately separate from a
// Worker record: association is what proves that a Worker Session belongs to
// the Factory invocation, while the sequencer-assigned ItemID on the opening
// record is the only ACP tool-call identity this mapper accepts.
type ChildAssociation struct {
	DispatchID      string
	WorkerSessionID string
}

var (
	// ErrMissingChildAssociation reports an opening without the canonical
	// dispatch-to-Worker-Session ownership facts. The mapper never guesses
	// membership from a record's timing, label, or neighbouring records.
	ErrMissingChildAssociation = errors.New("acp: worker child association is required")
	// ErrMissingChildItemID reports an opening whose sequencer identity is
	// absent. A transport-local replacement would break retained/live replay.
	ErrMissingChildItemID = errors.New("acp: worker child opening item id is required")
	// ErrMissingChildParent reports an update that does not name its already
	// delivered parent tool call. It must not be emitted as top-level output.
	ErrMissingChildParent = errors.New("acp: worker child update parent item id is required")
)

const workerToolCallMetaKey = "you.factory.worker_session"

// ProjectWorkerChild projects the sequenced envelope form of an associated
// Worker Session record. The association is stored with the Chat aggregate
// item, so retained and live delivery choose the same child path without a
// transport-local ownership map. Every legal Worker Kind/Phase pair has a
// declared outcome in projectChildRecord; generic Chat projection remains on
// Project and never reaches this function.
func ProjectWorkerChild(item chatsessions.SequencedItem) (*acpsdk.SessionUpdate, error) {
	if item.WorkerSessionAssociation == nil {
		return nil, ErrMissingChildAssociation
	}
	association := ChildAssociation{
		DispatchID:      item.WorkerSessionAssociation.DispatchID,
		WorkerSessionID: item.WorkerSessionAssociation.WorkerSessionID,
	}
	if err := validateChildAssociation(association); err != nil {
		return nil, err
	}
	draft := DraftFromSequencedItem(item)
	if !isLegalKindPhase(draft.Kind, draft.Phase) {
		return nil, fmt.Errorf(
			"%w: kind %q phase %q is not a declared projectable combination",
			ErrMalformedRecord, draft.Kind, draft.Phase,
		)
	}
	if isChildOpening(draft) {
		return ProjectChildOpening(draft, association)
	}
	if strings.TrimSpace(draft.ParentItemID) == "" {
		return nil, ErrMissingChildParent
	}
	return projectChildRecord(draft)
}

func isChildOpening(draft workers.Draft) bool {
	return draft.Kind == workers.KindSession && draft.Phase == workers.PhaseStarted
}

// ProjectChildOpening projects the sequenced SESSION/STARTED record that
// opens an associated Factory Worker Session. The supplied ItemID becomes the
// immutable ACP ToolCallId exactly as stored by Chat Sessions; it is never
// allocated or rewritten here. An opening must be top-level in the aggregate
// (its subsequent Worker records carry this ItemID as ParentItemID).
func ProjectChildOpening(draft workers.Draft, association ChildAssociation) (*acpsdk.SessionUpdate, error) {
	if err := validateChildAssociation(association); err != nil {
		return nil, err
	}
	if draft.Kind != workers.KindSession || draft.Phase != workers.PhaseStarted {
		return nil, fmt.Errorf("%w: child opening must be SESSION/STARTED", ErrMalformedRecord)
	}
	if strings.TrimSpace(draft.ItemID) == "" {
		return nil, ErrMissingChildItemID
	}
	if draft.ParentItemID != "" {
		return nil, fmt.Errorf("%w: child opening must not name a parent", ErrMalformedRecord)
	}
	status, err := childLifecycleStatus(draft)
	if err != nil {
		return nil, err
	}
	if status != "RESERVED" && status != "STARTING" {
		return nil, malformedChildStatus(draft.Phase, status)
	}

	return &acpsdk.SessionUpdate{ToolCall: &acpsdk.SessionUpdateToolCall{
		ToolCallId: acpsdk.ToolCallId(draft.ItemID),
		Title:      "Worker session " + association.WorkerSessionID,
		Kind:       acpsdk.ToolKindExecute,
		Status:     acpsdk.ToolCallStatusPending,
		Meta: map[string]any{workerToolCallMetaKey: map[string]string{
			"dispatchId": association.DispatchID, "workerSessionId": association.WorkerSessionID,
		}},
	}}, nil
}

// ProjectChildLifecycle projects one sequenced SESSION record after its
// opening. The stored ParentItemID is the sole ToolCallId target. It returns
// no output for lifecycle-only records that leave the ACP status unchanged,
// and a typed error rather than a partially addressed update for malformed
// status or lineage.
func ProjectChildLifecycle(draft workers.Draft) (*acpsdk.SessionUpdate, error) {
	if draft.Kind != workers.KindSession {
		return nil, fmt.Errorf("%w: child lifecycle must be SESSION", ErrMalformedRecord)
	}
	if draft.Phase == workers.PhaseStarted {
		return nil, fmt.Errorf("%w: child opening must use ProjectChildOpening", ErrMalformedRecord)
	}
	if strings.TrimSpace(draft.ParentItemID) == "" {
		return nil, ErrMissingChildParent
	}

	status, err := childLifecycleStatus(draft)
	if err != nil {
		return nil, err
	}
	projected, emit, err := projectChildLifecycleStatus(draft.Phase, status)
	if err != nil || !emit {
		return nil, err
	}
	return childStatusUpdate(draft.ParentItemID, projected), nil
}

func validateChildAssociation(association ChildAssociation) error {
	if strings.TrimSpace(association.DispatchID) == "" || strings.TrimSpace(association.WorkerSessionID) == "" {
		return ErrMissingChildAssociation
	}
	return nil
}

func childLifecycleStatus(draft workers.Draft) (string, error) {
	var payload workers.SessionPayload
	if err := json.Unmarshal(draft.Payload, &payload); err != nil {
		return "", fmt.Errorf("%w: payload must decode as SessionPayload: %v", ErrMalformedRecord, err)
	}
	status := strings.ToUpper(strings.TrimSpace(payload.Status))
	if status == "" {
		return "", fmt.Errorf("%w: child lifecycle status is required", ErrMalformedRecord)
	}
	return status, nil
}

func projectChildLifecycleStatus(phase workers.Phase, status string) (acpsdk.ToolCallStatus, bool, error) {
	switch phase {
	case workers.PhaseUpdated:
		switch status {
		case "RESERVED", "STARTING", "PAUSED":
			return acpsdk.ToolCallStatusPending, false, nil
		case "RUNNING":
			return acpsdk.ToolCallStatusInProgress, true, nil
		default:
			return "", false, malformedChildStatus(phase, status)
		}
	case workers.PhaseCompleted:
		if status != "COMPLETED" {
			return "", false, malformedChildStatus(phase, status)
		}
		return acpsdk.ToolCallStatusCompleted, true, nil
	case workers.PhaseFailed:
		if status != "FAILED" {
			return "", false, malformedChildStatus(phase, status)
		}
		return acpsdk.ToolCallStatusFailed, true, nil
	case workers.PhaseCanceled:
		if status != "CANCELED" && status != "TERMINATED" {
			return "", false, malformedChildStatus(phase, status)
		}
		// ACP has no cancelled terminal status. Failed is its truthful terminal
		// representation: the Worker did not complete successfully.
		return acpsdk.ToolCallStatusFailed, true, nil
	default:
		return "", false, fmt.Errorf("%w: child lifecycle phase %q is not supported", ErrMalformedRecord, phase)
	}
}

func malformedChildStatus(phase workers.Phase, status string) error {
	return fmt.Errorf("%w: status %q is invalid for child lifecycle phase %q", ErrMalformedRecord, status, phase)
}

func childStatusUpdate(parentItemID string, status acpsdk.ToolCallStatus) *acpsdk.SessionUpdate {
	return &acpsdk.SessionUpdate{ToolCallUpdate: &acpsdk.SessionToolCallUpdate{
		ToolCallId: acpsdk.ToolCallId(parentItemID), Status: &status,
	}}
}
