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

func (f *fakeService) StartTurn(_ context.Context, req chatsessions.StartTurnRequest) (chatsessions.StartTurnResult, error) {
	if f.session.ActiveTurnID != "" {
		return chatsessions.StartTurnResult{}, &chatsessions.BusyError{Value: "Session", ID: req.SessionID}
	}
	f.turn = chatsessions.Turn{ID: "turn-1", State: chatsessions.TurnStateAdmitted, RequestID: req.RequestID}
	f.session.ActiveTurnID = f.turn.ID
	f.session.State = chatsessions.SessionStateActive
	return chatsessions.StartTurnResult{Session: f.session, Turn: f.turn}, nil
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

func (f *fakeService) AdvanceControl(_ context.Context, req chatsessions.AdvanceControlRequest) (chatsessions.AdvanceControlResult, error) {
	if req.RequestID != f.intent.RequestID {
		return chatsessions.AdvanceControlResult{}, &chatsessions.NotFoundError{Value: "ControlIntent", ID: req.RequestID.RequestToken}
	}
	if err := f.intent.State.CanTransitionTo(req.Next); err != nil {
		return chatsessions.AdvanceControlResult{}, err
	}
	f.intent.State = req.Next
	return chatsessions.AdvanceControlResult{Intent: f.intent}, nil
}

func newTestSession(t *testing.T, ctx context.Context, svc *fakeService) chatsessions.Session {
	t.Helper()
	created, err := svc.CreateSession(ctx, chatsessions.CreateSessionRequest{
		RequestID:     chatsessions.RequestIdentity{ConnectionID: "conn-1", RequestToken: "req-1"},
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
		RequestID:       chatsessions.RequestIdentity{ConnectionID: "conn-1", RequestToken: "req-1b"},
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
		RequestID: chatsessions.RequestIdentity{ConnectionID: "conn-1", RequestToken: "req-2"},
		SessionID: session.ID,
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}

	if _, err := svc.StartTurn(ctx, chatsessions.StartTurnRequest{
		RequestID: chatsessions.RequestIdentity{ConnectionID: "conn-1", RequestToken: "req-3"},
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
		RequestID: chatsessions.RequestIdentity{ConnectionID: "conn-1", RequestToken: "req-4"},
		SessionID: session.ID, ExpectedVersion: 99, Action: chatsessions.ControlActionCancel,
	}); !errors.Is(err, chatsessions.ErrStaleVersion) {
		t.Fatalf("RequestControl stale version: got %v, want ErrStaleVersion", err)
	}

	if _, err := svc.RequestControl(ctx, chatsessions.RequestControlRequest{
		RequestID: chatsessions.RequestIdentity{ConnectionID: "conn-1", RequestToken: "req-5"},
		SessionID: session.ID, ExpectedVersion: session.Version, Action: chatsessions.ControlActionPause,
	}); !errors.Is(err, chatsessions.ErrUnsupportedControlAction) {
		t.Fatalf("RequestControl PAUSE: got %v, want ErrUnsupportedControlAction", err)
	}

	requested, err := svc.RequestControl(ctx, chatsessions.RequestControlRequest{
		RequestID: chatsessions.RequestIdentity{ConnectionID: "conn-1", RequestToken: "req-6"},
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
