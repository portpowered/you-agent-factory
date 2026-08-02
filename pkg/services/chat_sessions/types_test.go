package chatsessions

import (
	"errors"
	"testing"
	"time"
)

func TestChatTargetKind_Validate(t *testing.T) {
	tests := []struct {
		name    string
		kind    ChatTargetKind
		wantErr error
	}{
		{"factory", ChatTargetKindFactory, nil},
		{"worker", ChatTargetKindWorker, nil},
		{"zero", ChatTargetKind(""), ErrUnknownEnumValue},
		{"unknown", ChatTargetKind("BOGUS"), ErrUnknownEnumValue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.kind.Validate()
			assertSentinel(t, err, tt.wantErr)
		})
	}
}

func TestChatTargetRef_Validate(t *testing.T) {
	tests := []struct {
		name    string
		ref     ChatTargetRef
		wantErr error
	}{
		{"valid factory", ChatTargetRef{Kind: ChatTargetKindFactory, Ref: "@you/review"}, nil},
		{"valid worker", ChatTargetRef{Kind: ChatTargetKindWorker, Ref: "worker-1"}, nil},
		{"unknown kind", ChatTargetRef{Kind: "BOGUS", Ref: "x"}, ErrUnknownEnumValue},
		{"blank ref", ChatTargetRef{Kind: ChatTargetKindFactory, Ref: ""}, ErrRequiredValue},
		{"zero value", ChatTargetRef{}, ErrUnknownEnumValue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.ref.Validate()
			assertSentinel(t, err, tt.wantErr)
		})
	}
}

func TestRequestIdentity_Validate(t *testing.T) {
	tests := []struct {
		name    string
		id      RequestIdentity
		wantErr error
	}{
		{"valid", RequestIdentity{ConnectionID: "conn-1", RequestToken: "req-1"}, nil},
		{"blank connection", RequestIdentity{RequestToken: "req-1"}, ErrRequiredValue},
		{"blank token", RequestIdentity{ConnectionID: "conn-1"}, ErrRequiredValue},
		{"zero value", RequestIdentity{}, ErrRequiredValue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.id.Validate()
			assertSentinel(t, err, tt.wantErr)
		})
	}
}

func TestSessionState_Validate(t *testing.T) {
	tests := []struct {
		name    string
		state   SessionState
		wantErr error
	}{
		{"created", SessionStateCreated, nil},
		{"active", SessionStateActive, nil},
		{"closed", SessionStateClosed, nil},
		{"zero", SessionState(""), ErrUnknownEnumValue},
		{"unknown", SessionState("BOGUS"), ErrUnknownEnumValue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.state.Validate()
			assertSentinel(t, err, tt.wantErr)
		})
	}
	if !SessionStateClosed.IsTerminal() {
		t.Fatalf("SessionStateClosed.IsTerminal() = false, want true")
	}
	for _, s := range []SessionState{SessionStateCreated, SessionStateActive} {
		if s.IsTerminal() {
			t.Fatalf("%s.IsTerminal() = true, want false", s)
		}
	}
}

func validSession() Session {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	return Session{
		ID:             "session-1",
		State:          SessionStateActive,
		SelectedTarget: ChatTargetRef{Kind: ChatTargetKindFactory, Ref: "@you/review"},
		TargetEpisode:  1,
		ActiveTurnID:   "turn-1",
		Version:        1,
		StreamHead:     10,
		CreatedAt:      now,
		UpdatedAt:      now.Add(time.Minute),
	}
}

func TestSession_Validate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(Session) Session
		wantErr error
	}{
		{"valid", func(s Session) Session { return s }, nil},
		{"blank id", func(s Session) Session { s.ID = ""; return s }, ErrRequiredValue},
		{"unknown state", func(s Session) Session { s.State = "BOGUS"; return s }, ErrUnknownEnumValue},
		{"invalid target", func(s Session) Session { s.SelectedTarget = ChatTargetRef{}; return s }, ErrUnknownEnumValue},
		{"zero created at", func(s Session) Session { s.CreatedAt = time.Time{}; return s }, ErrRequiredValue},
		{"zero updated at", func(s Session) Session { s.UpdatedAt = time.Time{}; return s }, ErrRequiredValue},
		{"updated before created", func(s Session) Session {
			s.UpdatedAt = s.CreatedAt.Add(-time.Minute)
			return s
		}, ErrInconsistentValue},
		{"created with active turn", func(s Session) Session {
			s.State = SessionStateCreated
			s.ActiveTurnID = "turn-1"
			return s
		}, ErrInconsistentValue},
		{"created without active turn is valid", func(s Session) Session {
			s.State = SessionStateCreated
			s.ActiveTurnID = ""
			return s
		}, nil},
		{"closed without active turn is valid", func(s Session) Session {
			s.State = SessionStateClosed
			s.ActiveTurnID = ""
			return s
		}, nil},
		{"zero value", func(Session) Session { return Session{} }, ErrRequiredValue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.mutate(validSession()).Validate()
			assertSentinel(t, err, tt.wantErr)
		})
	}
}

func TestTargetEpisodeState_Validate(t *testing.T) {
	tests := []struct {
		name    string
		state   TargetEpisodeState
		wantErr error
	}{
		{"open", TargetEpisodeStateOpen, nil},
		{"closed", TargetEpisodeStateClosed, nil},
		{"zero", TargetEpisodeState(""), ErrUnknownEnumValue},
		{"unknown", TargetEpisodeState("BOGUS"), ErrUnknownEnumValue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.state.Validate()
			assertSentinel(t, err, tt.wantErr)
		})
	}
	if !TargetEpisodeStateClosed.IsTerminal() || TargetEpisodeStateOpen.IsTerminal() {
		t.Fatalf("TargetEpisodeState.IsTerminal() mismatch")
	}
}

func validTargetEpisode() TargetEpisode {
	started := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	return TargetEpisode{
		Number:           1,
		State:            TargetEpisodeStateOpen,
		Target:           ChatTargetRef{Kind: ChatTargetKindFactory, Ref: "@you/review"},
		FactorySessionID: "fs-1",
		StartedAt:        started,
	}
}

func TestTargetEpisode_Validate(t *testing.T) {
	started := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	closedAfter := started.Add(time.Minute)
	closedBefore := started.Add(-time.Minute)

	tests := []struct {
		name    string
		mutate  func(TargetEpisode) TargetEpisode
		wantErr error
	}{
		{"valid open", func(e TargetEpisode) TargetEpisode { return e }, nil},
		{"valid closed", func(e TargetEpisode) TargetEpisode {
			e.State = TargetEpisodeStateClosed
			e.ClosedAt = &closedAfter
			return e
		}, nil},
		{"unknown state", func(e TargetEpisode) TargetEpisode { e.State = "BOGUS"; return e }, ErrUnknownEnumValue},
		{"invalid target", func(e TargetEpisode) TargetEpisode { e.Target = ChatTargetRef{}; return e }, ErrUnknownEnumValue},
		{"zero started at", func(e TargetEpisode) TargetEpisode { e.StartedAt = time.Time{}; return e }, ErrRequiredValue},
		{"open with closed at set", func(e TargetEpisode) TargetEpisode {
			e.ClosedAt = &closedAfter
			return e
		}, ErrInconsistentValue},
		{"closed without closed at", func(e TargetEpisode) TargetEpisode {
			e.State = TargetEpisodeStateClosed
			return e
		}, ErrInconsistentValue},
		{"closed at before started at", func(e TargetEpisode) TargetEpisode {
			e.State = TargetEpisodeStateClosed
			e.ClosedAt = &closedBefore
			return e
		}, ErrInconsistentValue},
		{"zero value", func(TargetEpisode) TargetEpisode { return TargetEpisode{} }, ErrUnknownEnumValue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.mutate(validTargetEpisode()).Validate()
			assertSentinel(t, err, tt.wantErr)
		})
	}
}

func TestTurnState_Validate(t *testing.T) {
	tests := []struct {
		name       string
		state      TurnState
		wantErr    error
		isTerminal bool
	}{
		{"admitted", TurnStateAdmitted, nil, false},
		{"running", TurnStateRunning, nil, false},
		{"completed", TurnStateCompleted, nil, true},
		{"failed", TurnStateFailed, nil, true},
		{"canceled", TurnStateCanceled, nil, true},
		{"zero", TurnState(""), ErrUnknownEnumValue, false},
		{"unknown", TurnState("BOGUS"), ErrUnknownEnumValue, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.state.Validate()
			assertSentinel(t, err, tt.wantErr)
			if got := tt.state.IsTerminal(); got != tt.isTerminal {
				t.Fatalf("%s.IsTerminal() = %v, want %v", tt.state, got, tt.isTerminal)
			}
		})
	}
}

func validTurn() Turn {
	return Turn{
		ID:               "turn-1",
		Episode:          1,
		State:            TurnStateAdmitted,
		RequestID:        RequestIdentity{ConnectionID: "conn-1", RequestToken: "req-1"},
		StartSequence:    0,
		TerminalSequence: 0,
	}
}

func TestTurn_Validate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(Turn) Turn
		wantErr error
	}{
		{"valid admitted", func(t Turn) Turn { return t }, nil},
		{"valid running", func(t Turn) Turn { t.State = TurnStateRunning; return t }, nil},
		{"valid completed", func(t Turn) Turn {
			t.State = TurnStateRunning
			t.StartSequence = 5
			t.State = TurnStateCompleted
			t.TerminalSequence = 9
			return t
		}, nil},
		{"valid canceled before start", func(t Turn) Turn {
			t.State = TurnStateCanceled
			t.TerminalSequence = 1
			return t
		}, nil},
		{"blank id", func(t Turn) Turn { t.ID = ""; return t }, ErrRequiredValue},
		{"unknown state", func(t Turn) Turn { t.State = "BOGUS"; return t }, ErrUnknownEnumValue},
		{"invalid request id", func(t Turn) Turn { t.RequestID = RequestIdentity{}; return t }, ErrRequiredValue},
		{"terminal without terminal sequence", func(t Turn) Turn {
			t.State = TurnStateCompleted
			return t
		}, ErrInconsistentValue},
		{"terminal sequence before start", func(t Turn) Turn {
			t.State = TurnStateCompleted
			t.StartSequence = 10
			t.TerminalSequence = 3
			return t
		}, ErrInconsistentValue},
		{"non terminal with terminal sequence", func(t Turn) Turn {
			t.TerminalSequence = 4
			return t
		}, ErrInconsistentValue},
		{"zero value", func(Turn) Turn { return Turn{} }, ErrRequiredValue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.mutate(validTurn()).Validate()
			assertSentinel(t, err, tt.wantErr)
		})
	}
}

func validAttachment() Attachment {
	return Attachment{
		ID:            "attach-1",
		SessionID:     "session-1",
		ConnectionID:  "conn-1",
		AfterSequence: 0,
		Interactive:   true,
	}
}

func TestAttachment_Validate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(Attachment) Attachment
		wantErr error
	}{
		{"valid", func(a Attachment) Attachment { return a }, nil},
		{"blank id", func(a Attachment) Attachment { a.ID = ""; return a }, ErrRequiredValue},
		{"blank session id", func(a Attachment) Attachment { a.SessionID = ""; return a }, ErrRequiredValue},
		{"blank connection id", func(a Attachment) Attachment { a.ConnectionID = ""; return a }, ErrRequiredValue},
		{"zero after sequence is valid", func(a Attachment) Attachment { a.AfterSequence = 0; return a }, nil},
		{"zero value", func(Attachment) Attachment { return Attachment{} }, ErrRequiredValue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.mutate(validAttachment()).Validate()
			assertSentinel(t, err, tt.wantErr)
		})
	}
}

func TestControlAction_Validate(t *testing.T) {
	tests := []struct {
		name          string
		action        ControlAction
		wantErr       error
		supportedInL1 bool
	}{
		{"cancel", ControlActionCancel, nil, true},
		{"close", ControlActionClose, nil, true},
		{"pause", ControlActionPause, ErrUnsupportedControlAction, false},
		{"resume", ControlActionResume, ErrUnsupportedControlAction, false},
		{"terminate", ControlActionTerminate, ErrUnsupportedControlAction, false},
		{"zero", ControlAction(""), ErrUnknownEnumValue, false},
		{"unknown", ControlAction("BOGUS"), ErrUnknownEnumValue, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.action.Validate()
			assertSentinel(t, err, tt.wantErr)
			if got := tt.action.SupportedInL1(); got != tt.supportedInL1 {
				t.Fatalf("%s.SupportedInL1() = %v, want %v", tt.action, got, tt.supportedInL1)
			}
		})
	}
}

func TestControlIntentState_Validate(t *testing.T) {
	tests := []struct {
		name       string
		state      ControlIntentState
		wantErr    error
		isTerminal bool
	}{
		{"requested", ControlIntentStateRequested, nil, false},
		{"committed", ControlIntentStateCommitted, nil, false},
		{"completed", ControlIntentStateCompleted, nil, true},
		{"noop", ControlIntentStateNoop, nil, true},
		{"superseded", ControlIntentStateSuperseded, nil, true},
		{"zero", ControlIntentState(""), ErrUnknownEnumValue, false},
		{"unknown", ControlIntentState("BOGUS"), ErrUnknownEnumValue, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.state.Validate()
			assertSentinel(t, err, tt.wantErr)
			if got := tt.state.IsTerminal(); got != tt.isTerminal {
				t.Fatalf("%s.IsTerminal() = %v, want %v", tt.state, got, tt.isTerminal)
			}
		})
	}
	if ControlIntentStateNoop == ControlIntentStateSuperseded {
		t.Fatalf("NOOP and SUPERSEDED must remain distinct outcomes")
	}
}

func validControlIntent() ControlIntent {
	return ControlIntent{
		RequestID:       RequestIdentity{ConnectionID: "conn-1", RequestToken: "req-1"},
		SessionID:       "session-1",
		TurnID:          "turn-1",
		TargetEpisode:   1,
		ExpectedVersion: 1,
		Action:          ControlActionCancel,
		State:           ControlIntentStateRequested,
		RequestedAt:     time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC),
	}
}

func TestControlIntent_Validate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(ControlIntent) ControlIntent
		wantErr error
	}{
		{"valid", func(c ControlIntent) ControlIntent { return c }, nil},
		{"invalid request id", func(c ControlIntent) ControlIntent { c.RequestID = RequestIdentity{}; return c }, ErrRequiredValue},
		{"blank session id", func(c ControlIntent) ControlIntent { c.SessionID = ""; return c }, ErrRequiredValue},
		{"blank turn id", func(c ControlIntent) ControlIntent { c.TurnID = ""; return c }, ErrRequiredValue},
		{"unsupported action", func(c ControlIntent) ControlIntent { c.Action = ControlActionPause; return c }, ErrUnsupportedControlAction},
		{"unknown action", func(c ControlIntent) ControlIntent { c.Action = "BOGUS"; return c }, ErrUnknownEnumValue},
		{"unknown state", func(c ControlIntent) ControlIntent { c.State = "BOGUS"; return c }, ErrUnknownEnumValue},
		{"zero requested at", func(c ControlIntent) ControlIntent { c.RequestedAt = time.Time{}; return c }, ErrRequiredValue},
		{"zero value", func(ControlIntent) ControlIntent { return ControlIntent{} }, ErrRequiredValue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.mutate(validControlIntent()).Validate()
			assertSentinel(t, err, tt.wantErr)
		})
	}
}

// TestValidationError_ClassifiableWithoutStringParsing proves callers can use
// errors.As/errors.Is instead of matching Error() text, including through a
// nested field wrap.
func TestValidationError_ClassifiableWithoutStringParsing(t *testing.T) {
	err := ChatTargetRef{}.Validate()
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("errors.As(%v, *ValidationError) = false, want true", err)
	}
	if ve.Value != "ChatTargetRef" || ve.Field != "Kind" {
		t.Fatalf("ValidationError = %+v, want Value=ChatTargetRef Field=Kind", ve)
	}
	if !errors.Is(err, ErrUnknownEnumValue) {
		t.Fatalf("errors.Is(%v, ErrUnknownEnumValue) = false, want true", err)
	}
}

func assertSentinel(t *testing.T, err, wantSentinel error) {
	t.Helper()
	if wantSentinel == nil {
		if err != nil {
			t.Fatalf("Validate() = %v, want nil", err)
		}
		return
	}
	if err == nil {
		t.Fatalf("Validate() = nil, want error wrapping %v", wantSentinel)
	}
	if !errors.Is(err, wantSentinel) {
		t.Fatalf("Validate() = %v, want error wrapping %v", err, wantSentinel)
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("Validate() = %v, want *ValidationError", err)
	}
}
