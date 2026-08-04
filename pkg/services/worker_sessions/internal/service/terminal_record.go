package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/events"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Fixed identity Start uses to commit the one terminal SESSION record onto
// workersessions.Topic(id), scoped by the same worker_session_lifecycle
// SourceType and SourceID as the W3-001 opening record but a distinct
// SourceSequence/SourceEventID: commitTerminal already guarantees at most one
// caller ever reaches this identity for a given id, so no richer sequencing
// is required here either.
const (
	terminalSourceSequence events.SourceSequence = 2
	terminalSourceEventID  events.SourceEventID  = "terminal"
)

// terminalSessionPayload is the SESSION KindSession payload committed for the
// W3 terminal record. It always decodes as a valid workers.SessionPayload
// (Status is the one field that type declares), so validation and any reader
// that only knows about workers.SessionPayload continue to work unchanged;
// FailureCause/FailureDetail are additive fields present only on a FAILED
// terminal record, carrying the already-computed, already-safe
// FailureCause.Kind/Detail worker_sessions itself derives (see classify.go
// and safe_detail.go) rather than any new free-form text.
type terminalSessionPayload struct {
	Status        string `json:"status,omitempty"`
	FailureCause  string `json:"failureCause,omitempty"`
	FailureDetail string `json:"failureDetail,omitempty"`
}

// terminalPhase is the pure mapping from a committed Worker Session State to
// its terminal projection Phase: COMPLETED and FAILED map one-to-one, and the
// W1 CANCELED/TERMINATED states -- neither reachable through Start until W6
// adds controls -- share the existing PhaseCanceled pair per the W3 scope
// note that no new phase is introduced. Any other state has no terminal
// projection.
func terminalPhase(state workersessions.State) (workers.Phase, error) {
	switch state {
	case workersessions.StateCompleted:
		return workers.PhaseCompleted, nil
	case workersessions.StateFailed:
		return workers.PhaseFailed, nil
	case workersessions.StateCanceled, workersessions.StateTerminated:
		return workers.PhaseCanceled, nil
	default:
		return "", fmt.Errorf("worker sessions: state %q has no terminal projection phase", state)
	}
}

// terminalDraft is the pure mapping from a committed Worker Session
// State+TerminalResult to the one KindSession terminal workers.Draft W3
// projects after prior output. terminalDraft is side-effect free so every
// state, including the CANCELED/TERMINATED cases Start cannot yet produce, is
// directly unit-testable without a live session or Events call.
func terminalDraft(state workersessions.State, result workersessions.TerminalResult, attemptID string) (workers.Draft, error) {
	phase, err := terminalPhase(state)
	if err != nil {
		return workers.Draft{}, err
	}
	payload := terminalSessionPayload{Status: string(state)}
	if result.Cause != nil {
		payload.FailureCause = string(result.Cause.Kind)
		payload.FailureDetail = result.Cause.Detail
	}
	// terminalSessionPayload has only string fields, so json.Marshal cannot
	// fail here; the error is intentionally discarded rather than defended
	// against.
	payloadJSON, _ := json.Marshal(payload)
	return workers.Draft{
		Kind:       workers.KindSession,
		Phase:      phase,
		Payload:    payloadJSON,
		DispatchID: attemptID,
	}, nil
}

// publishTerminalRecord commits the one terminal KindSession workers.Draft
// onto workersessions.Topic(id), derived from state+result, through the same
// appendDraft helper publishOpeningRecord and PublishRecord already share. A
// non-nil return means no record was committed; the caller must not rewrite
// the already-committed canonical W2 terminal Session on this failure -- see
// publishTerminalRecordOrLog.
func (r *registry) publishTerminalRecord(ctx context.Context, id, attemptID string, state workersessions.State, result workersessions.TerminalResult) error {
	draft, err := terminalDraft(state, result, attemptID)
	if err != nil {
		return err
	}
	identity := events.AppendIdentity{
		SourceType:     lifecycleSourceType,
		SourceID:       events.SourceID(id),
		SourceSequence: terminalSourceSequence,
		SourceEventID:  terminalSourceEventID,
	}
	_, err = r.appendDraft(ctx, workersessions.Topic(id), identity, workerDraftSchemaID, draft)
	return err
}

// publishTerminalRecordOrLog calls publishTerminalRecord and, on failure,
// logs it explicitly rather than propagating it: the caller already holds the
// real committed terminal Session (commitTerminal's return value) and must
// return that value unchanged. A terminal-record publication failure is never
// reported as success and never silently replaced with fabricated history.
func (r *registry) publishTerminalRecordOrLog(ctx context.Context, id, attemptID string, state workersessions.State, result workersessions.TerminalResult) {
	if err := r.publishTerminalRecord(ctx, id, attemptID, state, result); err != nil {
		r.logger.Info(
			"worker session terminal record publication failed",
			"sessionID", id,
			"attemptID", attemptID,
			"state", string(state),
			"outcome", "publish_failed",
		)
	}
}
