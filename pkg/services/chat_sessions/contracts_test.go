package chatsessions_test

import (
	"context"
	"errors"
	"testing"
	"time"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
)

// fakeService is a compile-time-only demonstration that Service is usable by
// an external consumer using nothing but the package's public contracts. It
// is not a candidate production implementation.
type fakeService struct {
	session chatsessions.Session
	turn    chatsessions.Turn
	intent  chatsessions.ControlIntent
}

var _ chatsessions.Service = (*fakeService)(nil)

func (f *fakeService) CreateSession(_ context.Context, req chatsessions.CreateSessionRequest) (chatsessions.CreateSessionResult, error) {
	if err := req.InitialTarget.Validate(); err != nil {
		return chatsessions.CreateSessionResult{}, err
	}
	now := time.Unix(0, 1)
	f.session = chatsessions.Session{
		ID:             "session-1",
		State:          chatsessions.SessionStateCreated,
		SelectedTarget: req.InitialTarget,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	return chatsessions.CreateSessionResult{Session: f.session}, nil
}

func (f *fakeService) GetSession(_ context.Context, req chatsessions.GetSessionRequest) (chatsessions.GetSessionResult, error) {
	if req.SessionID != f.session.ID {
		return chatsessions.GetSessionResult{}, &chatsessions.NotFoundError{Value: "Session", ID: req.SessionID}
	}
	return chatsessions.GetSessionResult{Session: f.session}, nil
}

func (f *fakeService) SetTarget(_ context.Context, req chatsessions.SetTargetRequest) (chatsessions.SetTargetResult, error) {
	if req.ExpectedVersion != f.session.Version {
		return chatsessions.SetTargetResult{}, &chatsessions.ConflictError{
			Value: "Session", ID: req.SessionID, Expected: req.ExpectedVersion, Actual: f.session.Version,
		}
	}
	f.session.SelectedTarget = req.Target
	f.session.Version++
	return chatsessions.SetTargetResult{Session: f.session}, nil
}

// startTurnIfNotBusy is the fakeService.StartTurn admission check, factored
// out so the busy/non-busy behavioral tests can drive it directly against
// every prior-turn shape (nil, admitted, running, or terminal) without first
// having to route a real turn through its full lifecycle.
func (f *fakeService) startTurnIfNotBusy(req chatsessions.StartTurnRequest) (chatsessions.StartTurnResult, error) {
	if req.ExpectedVersion != f.session.Version {
		return chatsessions.StartTurnResult{}, &chatsessions.ConflictError{
			Value: "Session", ID: req.SessionID, Expected: req.ExpectedVersion, Actual: f.session.Version,
		}
	}
	if f.session.ActiveTurnID != "" && f.turn.State.IsBusy() {
		return chatsessions.StartTurnResult{}, &chatsessions.BusyError{
			Value: "Session", ID: req.SessionID,
			ActiveTurnID: f.turn.ID, ActiveTurnState: f.turn.State,
		}
	}
	f.turn = chatsessions.Turn{ID: "turn-1", State: chatsessions.TurnStateAdmitted, RequestID: req.RequestID}
	f.session.ActiveTurnID = f.turn.ID
	f.session.State = chatsessions.SessionStateActive
	return chatsessions.StartTurnResult{Session: f.session, Turn: f.turn}, nil
}

func (f *fakeService) StartTurn(_ context.Context, req chatsessions.StartTurnRequest) (chatsessions.StartTurnResult, error) {
	return f.startTurnIfNotBusy(req)
}

func (f *fakeService) AdvanceTurn(_ context.Context, req chatsessions.AdvanceTurnRequest) (chatsessions.AdvanceTurnResult, error) {
	if req.TurnID != f.turn.ID {
		return chatsessions.AdvanceTurnResult{}, &chatsessions.NotFoundError{Value: "Turn", ID: req.TurnID}
	}
	if err := f.turn.State.CanTransitionTo(req.Next); err != nil {
		return chatsessions.AdvanceTurnResult{}, err
	}
	f.turn.State = req.Next
	return chatsessions.AdvanceTurnResult{Turn: f.turn}, nil
}

func (f *fakeService) Attach(_ context.Context, req chatsessions.AttachRequest) (chatsessions.AttachResult, error) {
	if req.SessionID != f.session.ID {
		return chatsessions.AttachResult{}, &chatsessions.NotFoundError{Value: "Session", ID: req.SessionID}
	}
	return chatsessions.AttachResult{Attachment: chatsessions.Attachment{
		ID: "attachment-1", SessionID: req.SessionID, ConnectionID: req.ConnectionID, Interactive: req.Interactive,
	}}, nil
}

func (f *fakeService) Detach(_ context.Context, req chatsessions.DetachRequest) (chatsessions.DetachResult, error) {
	if req.AttachmentID != "attachment-1" {
		return chatsessions.DetachResult{}, &chatsessions.NotFoundError{Value: "Attachment", ID: req.AttachmentID}
	}
	return chatsessions.DetachResult{}, nil
}

func (f *fakeService) RequestControl(_ context.Context, req chatsessions.RequestControlRequest) (chatsessions.RequestControlResult, error) {
	if err := req.Action.Validate(); err != nil {
		return chatsessions.RequestControlResult{}, err
	}
	if f.session.ActiveTurnID == "" {
		return chatsessions.RequestControlResult{}, &chatsessions.NotFoundError{Value: "Turn", ID: ""}
	}
	if req.ExpectedVersion != f.session.Version {
		return chatsessions.RequestControlResult{}, &chatsessions.ConflictError{
			Value: "Session", ID: req.SessionID, Expected: req.ExpectedVersion, Actual: f.session.Version,
		}
	}
	f.intent = chatsessions.ControlIntent{
		RequestID:     req.RequestID,
		SessionID:     req.SessionID,
		TurnID:        f.session.ActiveTurnID,
		TargetEpisode: f.session.TargetEpisode,
		Action:        req.Action,
		State:         chatsessions.ControlIntentStateRequested,
		RequestedAt:   time.Unix(0, 1),
	}
	return chatsessions.RequestControlResult{Intent: f.intent}, nil
}

// AdvanceControl demonstrates the captured-turn race rule this packet
// requires: once an intent is COMMITTED, its terminal outcome is computed
// with ResolveControlIntentOutcome against the intent's own captured TurnID
// and the session's live active turn -- never taken as a caller-selected
// value -- so a completion can only ever resolve for the turn it captured.
func (f *fakeService) AdvanceControl(_ context.Context, req chatsessions.AdvanceControlRequest) (chatsessions.AdvanceControlResult, error) {
	if req.RequestID != f.intent.RequestID {
		return chatsessions.AdvanceControlResult{}, &chatsessions.NotFoundError{Value: "ControlIntent", ID: req.RequestID.JSONRPCStringID}
	}
	next := req.Next
	if f.intent.State == chatsessions.ControlIntentStateCommitted {
		// f.turn.State is the live state of whichever turn is currently known
		// to the session; it only represents the captured turn's own state
		// when that turn is still the session's active one, which is exactly
		// the case ResolveControlIntentOutcome needs -- when the captured
		// turn is no longer current, the mismatch against currentActiveID
		// resolves to SUPERSEDED before this state value is even consulted.
		next = chatsessions.ResolveControlIntentOutcome(f.intent.TurnID, f.turn.State, f.session.ActiveTurnID)
	}
	if err := f.intent.State.CanTransitionTo(next); err != nil {
		return chatsessions.AdvanceControlResult{}, err
	}
	f.intent.State = next
	return chatsessions.AdvanceControlResult{Intent: f.intent}, nil
}

func newTestSession(t *testing.T, ctx context.Context, svc *fakeService) chatsessions.Session {
	t.Helper()
	created, err := svc.CreateSession(ctx, chatsessions.CreateSessionRequest{
		RequestID:     chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindJSONRPCString, ConnectionID: "conn-1", JSONRPCStringID: "req-1"},
		InitialTarget: chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: "factory:@you/review"},
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if created.Session.ID == "" {
		t.Fatal("CreateSession: expected a non-blank session ID")
	}
	return created.Session
}

func TestFakeService_SessionTargetAndAttachment(t *testing.T) {
	ctx := context.Background()
	svc := &fakeService{}
	session := newTestSession(t, ctx, svc)

	if _, err := svc.GetSession(ctx, chatsessions.GetSessionRequest{SessionID: "does-not-exist"}); !errors.Is(err, chatsessions.ErrNotFound) {
		t.Fatalf("GetSession unknown id: got %v, want ErrNotFound", err)
	}

	retargeted, err := svc.SetTarget(ctx, chatsessions.SetTargetRequest{
		RequestID:       chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindJSONRPCString, ConnectionID: "conn-1", JSONRPCStringID: "req-1b"},
		SessionID:       session.ID,
		ExpectedVersion: session.Version,
		Target:          chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: "factory:@you/factory-builder"},
	})
	if err != nil {
		t.Fatalf("SetTarget: %v", err)
	}
	if retargeted.Session.SelectedTarget.Ref != "factory:@you/factory-builder" {
		t.Fatalf("SetTarget: got target %+v, want factory-builder", retargeted.Session.SelectedTarget)
	}

	attached, err := svc.Attach(ctx, chatsessions.AttachRequest{SessionID: session.ID, ConnectionID: "conn-2", Interactive: true})
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if _, err := svc.Detach(ctx, chatsessions.DetachRequest{SessionID: session.ID, AttachmentID: attached.Attachment.ID}); err != nil {
		t.Fatalf("Detach: %v", err)
	}
	if _, err := svc.Detach(ctx, chatsessions.DetachRequest{SessionID: session.ID, AttachmentID: "does-not-exist"}); !errors.Is(err, chatsessions.ErrNotFound) {
		t.Fatalf("Detach unknown id: got %v, want ErrNotFound", err)
	}
}

func TestFakeService_TurnAndControlLifecycle(t *testing.T) {
	ctx := context.Background()
	svc := &fakeService{}
	session := newTestSession(t, ctx, svc)

	started, err := svc.StartTurn(ctx, chatsessions.StartTurnRequest{
		RequestID: chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindJSONRPCString, ConnectionID: "conn-1", JSONRPCStringID: "req-2"},
		SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}

	if _, err := svc.StartTurn(ctx, chatsessions.StartTurnRequest{
		RequestID: chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindJSONRPCString, ConnectionID: "conn-1", JSONRPCStringID: "req-3"},
		SessionID: session.ID,
	}); !errors.Is(err, chatsessions.ErrBusy) {
		t.Fatalf("StartTurn while active: got %v, want ErrBusy", err)
	}

	if _, err := svc.AdvanceTurn(ctx, chatsessions.AdvanceTurnRequest{
		SessionID: session.ID, TurnID: started.Turn.ID, Next: chatsessions.TurnStateCompleted,
	}); !errors.Is(err, chatsessions.ErrInvalidTransition) {
		t.Fatalf("AdvanceTurn ADMITTED->COMPLETED: got %v, want ErrInvalidTransition", err)
	}

	if _, err := svc.RequestControl(ctx, chatsessions.RequestControlRequest{
		RequestID: chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindJSONRPCString, ConnectionID: "conn-1", JSONRPCStringID: "req-4"},
		SessionID: session.ID, ExpectedVersion: 99, Action: chatsessions.ControlActionCancel,
	}); !errors.Is(err, chatsessions.ErrStaleVersion) {
		t.Fatalf("RequestControl stale version: got %v, want ErrStaleVersion", err)
	}

	if _, err := svc.RequestControl(ctx, chatsessions.RequestControlRequest{
		RequestID: chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindJSONRPCString, ConnectionID: "conn-1", JSONRPCStringID: "req-5"},
		SessionID: session.ID, ExpectedVersion: session.Version, Action: chatsessions.ControlActionPause,
	}); !errors.Is(err, chatsessions.ErrUnsupportedControlAction) {
		t.Fatalf("RequestControl PAUSE: got %v, want ErrUnsupportedControlAction", err)
	}

	requested, err := svc.RequestControl(ctx, chatsessions.RequestControlRequest{
		RequestID: chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindJSONRPCString, ConnectionID: "conn-1", JSONRPCStringID: "req-6"},
		SessionID: session.ID, ExpectedVersion: session.Version, Action: chatsessions.ControlActionCancel,
	})
	if err != nil {
		t.Fatalf("RequestControl: %v", err)
	}

	committed, err := svc.AdvanceControl(ctx, chatsessions.AdvanceControlRequest{
		SessionID: session.ID, RequestID: requested.Intent.RequestID, Next: chatsessions.ControlIntentStateCommitted,
	})
	if err != nil {
		t.Fatalf("AdvanceControl REQUESTED->COMMITTED: %v", err)
	}
	if committed.Intent.State != chatsessions.ControlIntentStateCommitted {
		t.Fatalf("AdvanceControl: got state %v, want COMMITTED", committed.Intent.State)
	}

	if _, err := svc.AdvanceControl(ctx, chatsessions.AdvanceControlRequest{
		SessionID: session.ID, RequestID: requested.Intent.RequestID, Next: chatsessions.ControlIntentStateCompleted,
	}); err != nil {
		t.Fatalf("AdvanceControl COMMITTED->COMPLETED: %v", err)
	}
}

// TestFakeService_StartTurnAdmissionBusyCases proves StartTurn admission is
// busy only for an ADMITTED or RUNNING prior turn, not for a terminal or
// absent one, and that the returned *BusyError carries the blocking turn's
// own identity and state.
func TestFakeService_StartTurnAdmissionBusyCases(t *testing.T) {
	ctx := context.Background()

	t.Run("no prior turn is not busy", func(t *testing.T) {
		svc := &fakeService{}
		session := newTestSession(t, ctx, svc)
		if _, err := svc.StartTurn(ctx, chatsessions.StartTurnRequest{
			RequestID: chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindJSONRPCString, ConnectionID: "conn-1", JSONRPCStringID: "req-1"},
			SessionID: session.ID,
		}); err != nil {
			t.Fatalf("StartTurn with no prior turn: got %v, want nil", err)
		}
	})

	for _, tt := range []struct {
		name  string
		state chatsessions.TurnState
		busy  bool
	}{
		{"admitted prior turn is busy", chatsessions.TurnStateAdmitted, true},
		{"running prior turn is busy", chatsessions.TurnStateRunning, true},
		{"completed prior turn is not busy", chatsessions.TurnStateCompleted, false},
		{"failed prior turn is not busy", chatsessions.TurnStateFailed, false},
		{"canceled prior turn is not busy", chatsessions.TurnStateCanceled, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeService{}
			session := newTestSession(t, ctx, svc)
			if _, err := svc.StartTurn(ctx, chatsessions.StartTurnRequest{
				RequestID: chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindJSONRPCString, ConnectionID: "conn-1", JSONRPCStringID: "req-1"},
				SessionID: session.ID,
			}); err != nil {
				t.Fatalf("initial StartTurn: %v", err)
			}
			svc.turn.State = tt.state

			_, err := svc.StartTurn(ctx, chatsessions.StartTurnRequest{
				RequestID: chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindJSONRPCString, ConnectionID: "conn-1", JSONRPCStringID: "req-2"},
				SessionID: session.ID,
			})
			if !tt.busy {
				if err != nil {
					t.Fatalf("StartTurn with %s prior turn: got %v, want nil", tt.state, err)
				}
				return
			}
			if !errors.Is(err, chatsessions.ErrBusy) {
				t.Fatalf("StartTurn with %s prior turn: got %v, want ErrBusy", tt.state, err)
			}
			var busyErr *chatsessions.BusyError
			if !errors.As(err, &busyErr) {
				t.Fatalf("StartTurn with %s prior turn: got %v, want *BusyError", tt.state, err)
			}
			if busyErr.ActiveTurnID != "turn-1" || busyErr.ActiveTurnState != tt.state {
				t.Fatalf("BusyError = %+v, want ActiveTurnID=turn-1 ActiveTurnState=%s", busyErr, tt.state)
			}
		})
	}
}

// TestFakeService_StaleVersionConflictsDoNotMutateState proves a stale
// ExpectedVersion on SetTarget or StartTurn returns a *ConflictError
// carrying the expected/actual facts and leaves the Session unchanged.
func TestFakeService_StaleVersionConflictsDoNotMutateState(t *testing.T) {
	ctx := context.Background()

	t.Run("SetTarget", func(t *testing.T) {
		svc := &fakeService{}
		session := newTestSession(t, ctx, svc)
		before := svc.session

		_, err := svc.SetTarget(ctx, chatsessions.SetTargetRequest{
			RequestID:       chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindJSONRPCString, ConnectionID: "conn-1", JSONRPCStringID: "req-1"},
			SessionID:       session.ID,
			ExpectedVersion: session.Version + 1,
			Target:          chatsessions.ChatTargetRef{Kind: chatsessions.ChatTargetKindFactory, Ref: "@you/factory-builder"},
		})
		var conflict *chatsessions.ConflictError
		if !errors.As(err, &conflict) {
			t.Fatalf("SetTarget stale version: got %v, want *ConflictError", err)
		}
		if conflict.Expected != session.Version+1 || conflict.Actual != session.Version {
			t.Fatalf("ConflictError = %+v, want Expected=%d Actual=%d", conflict, session.Version+1, session.Version)
		}
		if svc.session != before {
			t.Fatalf("SetTarget stale version must not mutate Session, got %+v, want %+v", svc.session, before)
		}
	})

	t.Run("StartTurn", func(t *testing.T) {
		svc := &fakeService{}
		session := newTestSession(t, ctx, svc)
		before := svc.session

		_, err := svc.StartTurn(ctx, chatsessions.StartTurnRequest{
			RequestID:       chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindJSONRPCString, ConnectionID: "conn-1", JSONRPCStringID: "req-1"},
			SessionID:       session.ID,
			ExpectedVersion: session.Version + 1,
		})
		var conflict *chatsessions.ConflictError
		if !errors.As(err, &conflict) {
			t.Fatalf("StartTurn stale version: got %v, want *ConflictError", err)
		}
		if conflict.Expected != session.Version+1 || conflict.Actual != session.Version {
			t.Fatalf("ConflictError = %+v, want Expected=%d Actual=%d", conflict, session.Version+1, session.Version)
		}
		if svc.session != before {
			t.Fatalf("StartTurn stale version must not mutate Session, got %+v, want %+v", svc.session, before)
		}
	})
}

// TestFakeService_ControlIntentCapturedTurnRaceOutcomes proves the three
// captured-turn race outcomes a committed ControlIntent can resolve to:
// COMPLETED for a still-current, still-active captured turn; NOOP for a
// still-current captured turn that reached a terminal state before the
// intent completed; and SUPERSEDED for a captured turn that is no longer the
// session's active turn, regardless of that captured turn's own state -- and
// that none of the three ever rebinds the intent's TurnID.
func TestFakeService_ControlIntentCapturedTurnRaceOutcomes(t *testing.T) {
	ctx := context.Background()

	commit := func(t *testing.T, svc *fakeService, sessionID string) chatsessions.ControlIntent {
		t.Helper()
		requested, err := svc.RequestControl(ctx, chatsessions.RequestControlRequest{
			RequestID: chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindJSONRPCString, ConnectionID: "conn-1", JSONRPCStringID: "req-control"},
			SessionID: sessionID, ExpectedVersion: svc.session.Version, Action: chatsessions.ControlActionCancel,
		})
		if err != nil {
			t.Fatalf("RequestControl: %v", err)
		}
		committed, err := svc.AdvanceControl(ctx, chatsessions.AdvanceControlRequest{
			SessionID: sessionID, RequestID: requested.Intent.RequestID, Next: chatsessions.ControlIntentStateCommitted,
		})
		if err != nil {
			t.Fatalf("AdvanceControl REQUESTED->COMMITTED: %v", err)
		}
		return committed.Intent
	}

	t.Run("still current and active resolves to completed", func(t *testing.T) {
		svc := &fakeService{}
		session := newTestSession(t, ctx, svc)
		if _, err := svc.StartTurn(ctx, chatsessions.StartTurnRequest{
			RequestID: chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindJSONRPCString, ConnectionID: "conn-1", JSONRPCStringID: "req-turn"},
			SessionID: session.ID,
		}); err != nil {
			t.Fatalf("StartTurn: %v", err)
		}
		committed := commit(t, svc, session.ID)

		result, err := svc.AdvanceControl(ctx, chatsessions.AdvanceControlRequest{
			SessionID: session.ID, RequestID: committed.RequestID, Next: chatsessions.ControlIntentStateCompleted,
		})
		if err != nil {
			t.Fatalf("AdvanceControl: %v", err)
		}
		if result.Intent.State != chatsessions.ControlIntentStateCompleted {
			t.Fatalf("got state %v, want COMPLETED", result.Intent.State)
		}
		if result.Intent.TurnID != committed.TurnID {
			t.Fatalf("outcome rebound TurnID: got %q, want %q", result.Intent.TurnID, committed.TurnID)
		}
	})

	t.Run("still current but already terminal resolves to noop", func(t *testing.T) {
		svc := &fakeService{}
		session := newTestSession(t, ctx, svc)
		if _, err := svc.StartTurn(ctx, chatsessions.StartTurnRequest{
			RequestID: chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindJSONRPCString, ConnectionID: "conn-1", JSONRPCStringID: "req-turn"},
			SessionID: session.ID,
		}); err != nil {
			t.Fatalf("StartTurn: %v", err)
		}
		committed := commit(t, svc, session.ID)

		// The captured turn reaches a terminal state before the intent
		// completes, while remaining the session's active turn (this
		// fakeService, like a real implementation would on Detach/Advance,
		// does not clear ActiveTurnID purely from a Turn's own transition).
		svc.turn.State = chatsessions.TurnStateCompleted

		result, err := svc.AdvanceControl(ctx, chatsessions.AdvanceControlRequest{
			SessionID: session.ID, RequestID: committed.RequestID, Next: chatsessions.ControlIntentStateCompleted,
		})
		if err != nil {
			t.Fatalf("AdvanceControl: %v", err)
		}
		if result.Intent.State != chatsessions.ControlIntentStateNoop {
			t.Fatalf("got state %v, want NOOP", result.Intent.State)
		}
		if result.Intent.TurnID != committed.TurnID {
			t.Fatalf("outcome rebound TurnID: got %q, want %q", result.Intent.TurnID, committed.TurnID)
		}
	})

	t.Run("no longer current resolves to superseded even if captured turn is non-terminal", func(t *testing.T) {
		svc := &fakeService{}
		session := newTestSession(t, ctx, svc)
		if _, err := svc.StartTurn(ctx, chatsessions.StartTurnRequest{
			RequestID: chatsessions.RequestIdentity{Kind: chatsessions.RequestIdentityKindJSONRPCString, ConnectionID: "conn-1", JSONRPCStringID: "req-turn"},
			SessionID: session.ID,
		}); err != nil {
			t.Fatalf("StartTurn: %v", err)
		}
		committed := commit(t, svc, session.ID)

		// Simulate a later turn becoming the session's active turn after
		// this intent already captured the prior one.
		svc.session.ActiveTurnID = "turn-2"

		result, err := svc.AdvanceControl(ctx, chatsessions.AdvanceControlRequest{
			SessionID: session.ID, RequestID: committed.RequestID, Next: chatsessions.ControlIntentStateCompleted,
		})
		if err != nil {
			t.Fatalf("AdvanceControl: %v", err)
		}
		if result.Intent.State != chatsessions.ControlIntentStateSuperseded {
			t.Fatalf("got state %v, want SUPERSEDED", result.Intent.State)
		}
		if result.Intent.TurnID != committed.TurnID || result.Intent.TurnID == "turn-2" {
			t.Fatalf("a superseded outcome must never rebind to the later turn: got TurnID %q, captured %q", result.Intent.TurnID, committed.TurnID)
		}
	})
}

// TestAttachment_IndependentAcrossSameSessionAndPosition proves two valid
// Attachments sharing SessionID and AfterSequence retain independent IDs,
// ConnectionIDs, and Interactive flags, and that advancing one attachment's
// cursor does not move the other's.
func TestAttachment_IndependentAcrossSameSessionAndPosition(t *testing.T) {
	first := chatsessions.Attachment{
		ID: "attach-1", SessionID: "session-1", ConnectionID: "conn-1", AfterSequence: 5, Interactive: true,
	}
	second := chatsessions.Attachment{
		ID: "attach-2", SessionID: "session-1", ConnectionID: "conn-2", AfterSequence: 5, Interactive: false,
	}
	if err := first.Validate(); err != nil {
		t.Fatalf("first.Validate(): %v", err)
	}
	if err := second.Validate(); err != nil {
		t.Fatalf("second.Validate(): %v", err)
	}
	if first.ID == second.ID || first.ConnectionID == second.ConnectionID || first.Interactive == second.Interactive {
		t.Fatalf("attachments sharing SessionID/AfterSequence must retain independent ID/ConnectionID/Interactive, got %+v and %+v", first, second)
	}

	second.AfterSequence = 9
	if first.AfterSequence != 5 {
		t.Fatalf("advancing one attachment's cursor must not move another's, got first.AfterSequence=%d", first.AfterSequence)
	}
}
