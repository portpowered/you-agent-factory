package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/events"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	controlSourceType         events.SourceType     = "worker_session_control"
	controlRequestSourceEvent events.SourceEventID  = "request"
	controlOutcomeSourceEvent events.SourceEventID  = "outcome"
	controlRequestSourceSeq   events.SourceSequence = 1
	controlOutcomeSourceSeq   events.SourceSequence = 2
)

// controlHistoryGate keeps one control bracket ahead of the terminal
// publication boundary. It is separate from publication.mu because a
// Workers callback may synchronously publish a terminal record while the
// control operation is still waiting for that callback to return.
type controlHistoryGate struct {
	mu      sync.Mutex
	pending bool
	closed  bool
	done    chan struct{}
}

func (gate *controlHistoryGate) acquire() bool {
	for {
		gate.mu.Lock()
		if gate.closed {
			gate.mu.Unlock()
			return false
		}
		if !gate.pending {
			gate.pending = true
			gate.done = make(chan struct{})
			gate.mu.Unlock()
			return true
		}
		wait := gate.done
		gate.mu.Unlock()
		<-wait
	}
}

func (gate *controlHistoryGate) close() {
	for {
		gate.mu.Lock()
		if !gate.pending {
			gate.closed = true
			gate.mu.Unlock()
			return
		}
		wait := gate.done
		gate.mu.Unlock()
		<-wait
	}
}

func (gate *controlHistoryGate) release() {
	gate.mu.Lock()
	if !gate.pending {
		gate.mu.Unlock()
		return
	}
	wait := gate.done
	gate.pending = false
	gate.done = nil
	gate.mu.Unlock()
	close(wait)
}

type controlHistoryReservation struct {
	pub         *publication
	sessionID   string
	action      workersessions.ControlAction
	requestID   string
	correlation string
	dispatchID  string
	turnID      string
	supervision *supervision
	finishOnce  sync.Once
}

// beginControlHistory commits the request half of a control bracket before
// the caller can invoke Workers. A control racing the in-process terminal
// transition may still be recorded while the terminal publication window is
// open; the terminal gate then orders the bracket before the terminal record.
func (r *registry) beginControlHistory(
	ctx context.Context,
	id string,
	action workersessions.ControlAction,
	requestID string,
) (*controlHistoryReservation, error) {
	session, supervision, err := r.controlTarget(id)
	if err != nil {
		return nil, err
	}
	pub := r.publicationFor(id)
	if pub == nil {
		// Small lifecycle unit registries intentionally omit Events and the
		// publication map. Preserve their control semantics; production
		// reservations always install a publication before exposing a session.
		return nil, nil
	}
	if !pub.control.acquire() {
		return nil, nil
	}

	pub.mu.Lock()
	open := pub.open
	dispatchID := ""
	turnID := pub.turnID
	if supervision != nil {
		supervision.mu.Lock()
		dispatchID = strings.TrimSpace(supervision.dispatchID)
		turnID = strings.TrimSpace(supervision.turnID)
		supervision.mu.Unlock()
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		requestID = controlFallbackRequestID(action, id, dispatchID)
	}
	if _, exists := pub.completedControls[controlReplayKey(action, requestID)]; exists {
		pub.mu.Unlock()
		pub.control.release()
		return nil, nil
	}
	pub.mu.Unlock()
	if !open {
		pub.control.release()
		return nil, nil
	}

	reservation := &controlHistoryReservation{
		pub:         pub,
		sessionID:   id,
		action:      action,
		requestID:   requestID,
		correlation: controlCorrelation(action, id, requestID, dispatchID),
		dispatchID:  dispatchID,
		turnID:      turnID,
		supervision: supervision,
	}
	if err := r.appendControlRecord(controlContext(ctx), reservation, workersessions.ControlRecordTypeRequest, "", dispatchID, session.State); err != nil {
		pub.control.release()
		return nil, err
	}
	if supervision != nil {
		supervision.mu.Lock()
		if supervision.controlHistory == nil {
			supervision.controlHistory = reservation
			reservation.supervision = supervision
		}
		supervision.mu.Unlock()
	}
	return reservation, nil
}

func controlContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return context.WithoutCancel(ctx)
}

func controlFallbackRequestID(action workersessions.ControlAction, sessionID, dispatchID string) string {
	return strings.Join([]string{string(action), sessionID, dispatchID}, "/")
}

func controlReplayKey(action workersessions.ControlAction, requestID string) string {
	return string(action) + "\x00" + strings.TrimSpace(requestID)
}

func controlCorrelation(action workersessions.ControlAction, sessionID, requestID, dispatchID string) string {
	value := strings.Join([]string{string(action), sessionID, requestID, dispatchID}, "\x00")
	digest := sha256.Sum256([]byte(value))
	return "control-" + hex.EncodeToString(digest[:])
}

func (r *registry) finishControlHistory(
	reservation *controlHistoryReservation,
	outcome workersessions.ControlOutcome,
	dispatchID string,
	state workersessions.State,
) {
	if reservation == nil {
		return
	}
	reservation.finishOnce.Do(func() {
		if strings.TrimSpace(dispatchID) == "" {
			dispatchID = reservation.dispatchID
		}
		ctx := r.serverOwnedContext()
		reservation.pub.mu.Lock()
		err := error(nil)
		if reservation.pub.open {
			err = r.appendControlRecordLocked(ctx, reservation, workersessions.ControlRecordTypeOutcome, outcome, dispatchID, state)
		} else {
			err = workersessions.ErrPublicationNotOpen
		}
		if reservation.pub.completedControls == nil {
			reservation.pub.completedControls = make(map[string]struct{})
		}
		reservation.pub.completedControls[controlReplayKey(reservation.action, reservation.requestID)] = struct{}{}
		reservation.pub.mu.Unlock()
		if err != nil && !errors.Is(err, workersessions.ErrPublicationNotOpen) {
			r.logger.Info(
				"worker session control history outcome publication failed",
				"sessionID", reservation.sessionID,
				"action", string(reservation.action),
				"outcome", string(outcome),
				"error", err.Error(),
			)
		}
		if reservation.supervision != nil {
			reservation.supervision.mu.Lock()
			if reservation.supervision.controlHistory == reservation {
				reservation.supervision.controlHistory = nil
			}
			reservation.supervision.mu.Unlock()
		}
		reservation.pub.control.release()
	})
}

func (r *registry) appendControlRecord(
	ctx context.Context,
	reservation *controlHistoryReservation,
	recordType workersessions.ControlRecordType,
	outcome workersessions.ControlOutcome,
	dispatchID string,
	state workersessions.State,
) error {
	reservation.pub.mu.Lock()
	defer reservation.pub.mu.Unlock()
	if !reservation.pub.open {
		return workersessions.ErrPublicationNotOpen
	}
	return r.appendControlRecordLocked(ctx, reservation, recordType, outcome, dispatchID, state)
}

func (r *registry) appendControlRecordLocked(
	ctx context.Context,
	reservation *controlHistoryReservation,
	recordType workersessions.ControlRecordType,
	outcome workersessions.ControlOutcome,
	dispatchID string,
	state workersessions.State,
) error {
	payload := workersessions.ControlRecordPayload{
		RecordType:      recordType,
		Action:          reservation.action,
		Outcome:         outcome,
		RequestID:       reservation.requestID,
		CorrelationID:   reservation.correlation,
		WorkerSessionID: reservation.sessionID,
		DispatchID:      strings.TrimSpace(dispatchID),
		AttemptID:       strings.TrimSpace(dispatchID),
		State:           state,
	}
	payloadJSON, _ := json.Marshal(payload)
	provenance := lifecycleProvenance("")
	provenance.NativeEventSubtype = "worker_session.control." + strings.ToLower(string(recordType))
	draft := workers.Draft{
		Kind:       workers.KindSession,
		Phase:      workers.PhaseUpdated,
		Provenance: provenance,
		Payload:    payloadJSON,
		DispatchID: strings.TrimSpace(dispatchID),
		TurnID:     reservation.turnID,
	}
	sequence := controlRequestSourceSeq
	eventID := controlRequestSourceEvent
	if recordType == workersessions.ControlRecordTypeOutcome {
		sequence = controlOutcomeSourceSeq
		eventID = controlOutcomeSourceEvent
	}
	identity := events.AppendIdentity{
		SourceType:     controlSourceType,
		SourceID:       events.SourceID(reservation.correlation),
		SourceSequence: sequence,
		SourceEventID:  eventID,
	}
	_, err := r.appendDraft(ctx, workersessions.Topic(reservation.sessionID), identity, workerDraftSchemaID, draft)
	return err
}

func controlOutcomeFromDispatch(
	action workersessions.ControlAction,
	result workers.WorkstationDispatchResult,
	dispatchErr error,
) workersessions.ControlOutcome {
	if dispatchErr != nil && !dispatchCanceled(result, dispatchErr) {
		return workersessions.ControlOutcomeFailed
	}
	if dispatchCanceled(result, dispatchErr) {
		if action == workersessions.ControlActionPause || action == workersessions.ControlActionCancel || action == workersessions.ControlActionTerminate || action == workersessions.ControlActionInterrupt {
			return workersessions.ControlOutcomeApplied
		}
	}
	return workersessions.ControlOutcomeNoop
}

func controlResultOutcome(result workersessions.ControlResult, operationErr error) workersessions.ControlOutcome {
	if result.Outcome != "" {
		return result.Outcome
	}
	if operationErr != nil {
		return workersessions.ControlOutcomeFailed
	}
	return workersessions.ControlOutcomeNoop
}

func controlReservationFor(supervision *supervision) *controlHistoryReservation {
	if supervision == nil {
		return nil
	}
	supervision.mu.Lock()
	defer supervision.mu.Unlock()
	return supervision.controlHistory
}
