package chatsessions

import (
	"errors"
	"testing"
	"time"
)

func TestControlActionValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		action  ControlAction
		wantErr bool
	}{
		{name: "cancel is valid", action: ControlActionCancel},
		{name: "close is valid", action: ControlActionClose},
		{name: "pause is valid", action: ControlActionPause},
		{name: "resume is valid", action: ControlActionResume},
		{name: "terminate is valid", action: ControlActionTerminate},
		{name: "zero value is invalid", action: ControlAction(""), wantErr: true},
		{name: "unknown value is invalid", action: ControlAction("RESTART"), wantErr: true},
		{name: "lowercase known value is invalid", action: ControlAction("cancel"), wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := test.action.Validate()
			if !test.wantErr {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("Validate() = nil, want error")
			}
			var invalid *InvalidControlActionError
			if !errors.As(err, &invalid) {
				t.Fatalf("Validate() error = %v (%T), want *InvalidControlActionError", err, err)
			}
		})
	}
}

func TestControlActionCheckSupported(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		action      ControlAction
		wantErr     bool
		wantUnsupp  bool
		wantInvalid bool
	}{
		{name: "cancel is supported", action: ControlActionCancel},
		{name: "close is supported", action: ControlActionClose},
		{name: "pause is unsupported", action: ControlActionPause, wantErr: true, wantUnsupp: true},
		{name: "resume is unsupported", action: ControlActionResume, wantErr: true, wantUnsupp: true},
		{name: "terminate is unsupported", action: ControlActionTerminate, wantErr: true, wantUnsupp: true},
		{name: "zero value is invalid not unsupported", action: ControlAction(""), wantErr: true, wantInvalid: true},
		{name: "unknown value is invalid not unsupported", action: ControlAction("RESTART"), wantErr: true, wantInvalid: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := test.action.CheckSupported()
			if !test.wantErr {
				if err != nil {
					t.Fatalf("CheckSupported() = %v, want nil", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("CheckSupported() = nil, want error")
			}

			if test.wantUnsupp {
				var unsupported *UnsupportedControlActionError
				if !errors.As(err, &unsupported) {
					t.Fatalf("CheckSupported() error = %v (%T), want *UnsupportedControlActionError", err, err)
				}
			}
			if test.wantInvalid {
				var invalid *InvalidControlActionError
				if !errors.As(err, &invalid) {
					t.Fatalf("CheckSupported() error = %v (%T), want *InvalidControlActionError", err, err)
				}
			}
		})
	}
}

func TestControlIntentStateValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		state   ControlIntentState
		wantErr bool
	}{
		{name: "requested is valid", state: ControlIntentStateRequested},
		{name: "committed is valid", state: ControlIntentStateCommitted},
		{name: "completed is valid", state: ControlIntentStateCompleted},
		{name: "noop is valid", state: ControlIntentStateNoop},
		{name: "superseded is valid", state: ControlIntentStateSuperseded},
		{name: "zero value is invalid", state: ControlIntentState(""), wantErr: true},
		{name: "unknown value is invalid", state: ControlIntentState("PENDING"), wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := test.state.Validate()
			if !test.wantErr {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("Validate() = nil, want error")
			}
			var invalid *InvalidControlIntentStateError
			if !errors.As(err, &invalid) {
				t.Fatalf("Validate() error = %v (%T), want *InvalidControlIntentStateError", err, err)
			}
		})
	}
}

func TestTransitionControlIntentState(t *testing.T) {
	t.Parallel()

	allKnown := []ControlIntentState{
		ControlIntentStateRequested, ControlIntentStateCommitted, ControlIntentStateCompleted,
		ControlIntentStateNoop, ControlIntentStateSuperseded,
	}
	legal := map[ControlIntentState]map[ControlIntentState]bool{
		ControlIntentStateRequested: {ControlIntentStateCommitted: true},
		ControlIntentStateCommitted: {
			ControlIntentStateCompleted:  true,
			ControlIntentStateNoop:       true,
			ControlIntentStateSuperseded: true,
		},
		ControlIntentStateCompleted:  {},
		ControlIntentStateNoop:       {},
		ControlIntentStateSuperseded: {},
	}

	for _, from := range allKnown {
		for _, to := range allKnown {
			from, to := from, to
			wantLegal := legal[from][to]
			t.Run(string(from)+"_to_"+string(to), func(t *testing.T) {
				t.Parallel()

				got, err := TransitionControlIntentState(from, to)
				if wantLegal {
					if err != nil {
						t.Fatalf("TransitionControlIntentState(%q, %q) error = %v, want nil", from, to, err)
					}
					if got != to {
						t.Fatalf("TransitionControlIntentState(%q, %q) = %q, want %q", from, to, got, to)
					}
					return
				}

				if err == nil {
					t.Fatalf("TransitionControlIntentState(%q, %q) error = nil, want error", from, to)
				}
				if got != from {
					t.Fatalf("TransitionControlIntentState(%q, %q) = %q, want unchanged %q", from, to, got, from)
				}
				var invalid *InvalidControlIntentStateTransitionError
				if !errors.As(err, &invalid) {
					t.Fatalf("TransitionControlIntentState(%q, %q) error = %v (%T), want *InvalidControlIntentStateTransitionError", from, to, err, err)
				}
			})
		}
	}

	t.Run("zero from is invalid value not invalid transition", func(t *testing.T) {
		t.Parallel()

		got, err := TransitionControlIntentState(ControlIntentState(""), ControlIntentStateCommitted)
		if err == nil {
			t.Fatalf("error = nil, want error")
		}
		if got != ControlIntentState("") {
			t.Fatalf("got = %q, want unchanged zero value", got)
		}
		var invalid *InvalidControlIntentStateError
		if !errors.As(err, &invalid) {
			t.Fatalf("error = %v (%T), want *InvalidControlIntentStateError", err, err)
		}
	})

	t.Run("every terminal state has no outbound transition", func(t *testing.T) {
		t.Parallel()

		for _, terminal := range []ControlIntentState{ControlIntentStateCompleted, ControlIntentStateNoop, ControlIntentStateSuperseded} {
			for _, to := range allKnown {
				if _, err := TransitionControlIntentState(terminal, to); err == nil {
					t.Fatalf("TransitionControlIntentState(%q, %q) error = nil, want error (terminal state must have no outbound transition)", terminal, to)
				}
			}
		}
	})
}

func validControlIntent() ControlIntent {
	return ControlIntent{
		RequestID:       RequestIdentity{OpaqueID: "req-1"},
		SessionID:       "session-1",
		TurnID:          "turn-1",
		TargetEpisode:   1,
		ExpectedVersion: 3,
		Action:          ControlActionCancel,
		State:           ControlIntentStateRequested,
		RequestedAt:     time.Unix(0, 0),
	}
}

func TestControlIntentValidate(t *testing.T) {
	t.Parallel()

	t.Run("fully populated intent with a supported action is valid", func(t *testing.T) {
		t.Parallel()

		if err := validControlIntent().Validate(); err != nil {
			t.Fatalf("Validate() = %v, want nil", err)
		}
	})

	t.Run("invalid request identity is rejected", func(t *testing.T) {
		t.Parallel()

		intent := validControlIntent()
		intent.RequestID = RequestIdentity{}
		err := intent.Validate()
		if err == nil {
			t.Fatalf("Validate() = nil, want error")
		}
		var invalid *InvalidRequestIdentityError
		if !errors.As(err, &invalid) {
			t.Fatalf("Validate() error = %v (%T), want *InvalidRequestIdentityError", err, err)
		}
	})

	t.Run("missing session id is rejected", func(t *testing.T) {
		t.Parallel()

		intent := validControlIntent()
		intent.SessionID = ""
		err := intent.Validate()
		if err == nil {
			t.Fatalf("Validate() = nil, want error")
		}
		var invalid *InvalidControlIntentError
		if !errors.As(err, &invalid) {
			t.Fatalf("Validate() error = %v (%T), want *InvalidControlIntentError", err, err)
		}
		if invalid.Reason != ControlIntentInvalidMissingSessionID {
			t.Fatalf("Reason = %q, want %q", invalid.Reason, ControlIntentInvalidMissingSessionID)
		}
	})

	t.Run("missing turn id is rejected", func(t *testing.T) {
		t.Parallel()

		intent := validControlIntent()
		intent.TurnID = ""
		err := intent.Validate()
		if err == nil {
			t.Fatalf("Validate() = nil, want error")
		}
		var invalid *InvalidControlIntentError
		if !errors.As(err, &invalid) {
			t.Fatalf("Validate() error = %v (%T), want *InvalidControlIntentError", err, err)
		}
		if invalid.Reason != ControlIntentInvalidMissingTurnID {
			t.Fatalf("Reason = %q, want %q", invalid.Reason, ControlIntentInvalidMissingTurnID)
		}
	})

	t.Run("unsupported action is rejected", func(t *testing.T) {
		t.Parallel()

		intent := validControlIntent()
		intent.Action = ControlActionPause
		err := intent.Validate()
		if err == nil {
			t.Fatalf("Validate() = nil, want error")
		}
		var unsupported *UnsupportedControlActionError
		if !errors.As(err, &unsupported) {
			t.Fatalf("Validate() error = %v (%T), want *UnsupportedControlActionError", err, err)
		}
	})

	t.Run("close is a supported action", func(t *testing.T) {
		t.Parallel()

		intent := validControlIntent()
		intent.Action = ControlActionClose
		if err := intent.Validate(); err != nil {
			t.Fatalf("Validate() = %v, want nil", err)
		}
	})
}

func TestCommitControlIntent(t *testing.T) {
	t.Parallel()

	t.Run("valid requested intent commits", func(t *testing.T) {
		t.Parallel()

		intent := validControlIntent()
		got, err := CommitControlIntent(intent)
		if err != nil {
			t.Fatalf("CommitControlIntent() error = %v, want nil", err)
		}
		if got.State != ControlIntentStateCommitted {
			t.Fatalf("got.State = %q, want COMMITTED", got.State)
		}
		if got.TurnID != intent.TurnID {
			t.Fatalf("got.TurnID = %q, want unchanged %q", got.TurnID, intent.TurnID)
		}
		if intent.State != ControlIntentStateRequested {
			t.Fatalf("input intent.State mutated to %q, want unchanged REQUESTED", intent.State)
		}
	})

	t.Run("invalid intent is rejected without state mutation", func(t *testing.T) {
		t.Parallel()

		intent := validControlIntent()
		intent.SessionID = ""
		got, err := CommitControlIntent(intent)
		if err == nil {
			t.Fatalf("CommitControlIntent() error = nil, want error")
		}
		if got != intent {
			t.Fatalf("got = %+v, want unchanged %+v", got, intent)
		}
	})

	t.Run("already committed intent cannot commit again", func(t *testing.T) {
		t.Parallel()

		intent := validControlIntent()
		intent.State = ControlIntentStateCommitted
		got, err := CommitControlIntent(intent)
		if err == nil {
			t.Fatalf("CommitControlIntent() error = nil, want error")
		}
		var invalid *InvalidControlIntentStateTransitionError
		if !errors.As(err, &invalid) {
			t.Fatalf("error = %v (%T), want *InvalidControlIntentStateTransitionError", err, err)
		}
		if got != intent {
			t.Fatalf("got = %+v, want unchanged %+v", got, intent)
		}
	})
}

func TestResolveControlIntentOutcome(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		capturedTurnID    string
		capturedTurnState TurnState
		currentActiveID   string
		want              ControlIntentState
	}{
		{
			name:              "current non-terminal captured turn completes",
			capturedTurnID:    "turn-1",
			capturedTurnState: TurnStateRunning,
			currentActiveID:   "turn-1",
			want:              ControlIntentStateCompleted,
		},
		{
			name:              "current already-terminal captured turn is a noop",
			capturedTurnID:    "turn-1",
			capturedTurnState: TurnStateCompleted,
			currentActiveID:   "turn-1",
			want:              ControlIntentStateNoop,
		},
		{
			name:              "no-longer-current captured turn is superseded even if it looks non-terminal",
			capturedTurnID:    "turn-1",
			capturedTurnState: TurnStateRunning,
			currentActiveID:   "turn-2",
			want:              ControlIntentStateSuperseded,
		},
		{
			name:              "no-longer-current terminal captured turn is still superseded, not noop",
			capturedTurnID:    "turn-1",
			capturedTurnState: TurnStateCanceled,
			currentActiveID:   "turn-2",
			want:              ControlIntentStateSuperseded,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := ResolveControlIntentOutcome(test.capturedTurnID, test.capturedTurnState, test.currentActiveID)
			if err != nil {
				t.Fatalf("ResolveControlIntentOutcome() error = %v, want nil", err)
			}
			if got != test.want {
				t.Fatalf("ResolveControlIntentOutcome() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestCompleteControlIntent(t *testing.T) {
	t.Parallel()

	t.Run("committed intent for the current running captured turn completes", func(t *testing.T) {
		t.Parallel()

		intent := validControlIntent()
		intent.State = ControlIntentStateCommitted
		got, err := CompleteControlIntent(intent, TurnStateRunning, intent.TurnID)
		if err != nil {
			t.Fatalf("CompleteControlIntent() error = %v, want nil", err)
		}
		if got.State != ControlIntentStateCompleted {
			t.Fatalf("got.State = %q, want COMPLETED", got.State)
		}
		if intent.State != ControlIntentStateCommitted {
			t.Fatalf("input intent.State mutated to %q, want unchanged COMMITTED", intent.State)
		}
	})

	t.Run("committed intent for an already-terminal captured turn is a noop", func(t *testing.T) {
		t.Parallel()

		intent := validControlIntent()
		intent.State = ControlIntentStateCommitted
		got, err := CompleteControlIntent(intent, TurnStateFailed, intent.TurnID)
		if err != nil {
			t.Fatalf("CompleteControlIntent() error = %v, want nil", err)
		}
		if got.State != ControlIntentStateNoop {
			t.Fatalf("got.State = %q, want NOOP", got.State)
		}
	})

	t.Run("committed intent whose captured turn is no longer current is superseded", func(t *testing.T) {
		t.Parallel()

		intent := validControlIntent()
		intent.State = ControlIntentStateCommitted
		got, err := CompleteControlIntent(intent, TurnStateRunning, "turn-2-later")
		if err != nil {
			t.Fatalf("CompleteControlIntent() error = %v, want nil", err)
		}
		if got.State != ControlIntentStateSuperseded {
			t.Fatalf("got.State = %q, want SUPERSEDED", got.State)
		}
	})

	t.Run("a requested (not committed) intent cannot be completed", func(t *testing.T) {
		t.Parallel()

		intent := validControlIntent()
		got, err := CompleteControlIntent(intent, TurnStateRunning, intent.TurnID)
		if err == nil {
			t.Fatalf("CompleteControlIntent() error = nil, want error")
		}
		var invalid *InvalidControlIntentStateTransitionError
		if !errors.As(err, &invalid) {
			t.Fatalf("error = %v (%T), want *InvalidControlIntentStateTransitionError", err, err)
		}
		if got != intent {
			t.Fatalf("got = %+v, want unchanged %+v", got, intent)
		}
	})

	t.Run("a superseded outcome never rebinds to the later current turn's identity", func(t *testing.T) {
		t.Parallel()

		intent := validControlIntent()
		intent.State = ControlIntentStateCommitted
		got, err := CompleteControlIntent(intent, TurnStateRunning, "turn-2-later")
		if err != nil {
			t.Fatalf("CompleteControlIntent() error = %v, want nil", err)
		}
		if got.TurnID != intent.TurnID {
			t.Fatalf("got.TurnID = %q, want captured turn id unchanged %q (never rebound to later turn)", got.TurnID, intent.TurnID)
		}
	})
}
