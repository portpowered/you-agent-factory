package chatsessions

import (
	"errors"
	"testing"
)

func assertNoTransitionErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("CanTransitionTo() = %v, want nil", err)
	}
}

func assertInvalidTransition(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("CanTransitionTo() = nil, want error wrapping %v", ErrInvalidTransition)
	}
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("CanTransitionTo() = %v, want error wrapping %v", err, ErrInvalidTransition)
	}
	var te *TransitionError
	if !errors.As(err, &te) {
		t.Fatalf("CanTransitionTo() = %v, want *TransitionError", err)
	}
}

func assertInvalidState(t *testing.T, err error) {
	t.Helper()
	assertSentinel(t, err, ErrUnknownEnumValue)
	var te *TransitionError
	if errors.As(err, &te) {
		t.Fatalf("CanTransitionTo() = %v, want invalid-state outcome, not *TransitionError", err)
	}
}

func TestSessionState_CanTransitionTo(t *testing.T) {
	members := []SessionState{SessionStateCreated, SessionStateActive, SessionStateClosed}
	legal := map[[2]SessionState]bool{
		{SessionStateCreated, SessionStateActive}: true,
		{SessionStateCreated, SessionStateClosed}: true,
		{SessionStateActive, SessionStateClosed}:  true,
	}
	for _, from := range members {
		for _, to := range members {
			from, to := from, to
			t.Run(string(from)+"->"+string(to), func(t *testing.T) {
				err := from.CanTransitionTo(to)
				if legal[[2]SessionState{from, to}] {
					assertNoTransitionErr(t, err)
				} else {
					assertInvalidTransition(t, err)
				}
			})
		}
	}
	t.Run("terminal CLOSED rejects every transition", func(t *testing.T) {
		for _, to := range members {
			assertInvalidTransition(t, SessionStateClosed.CanTransitionTo(to))
		}
	})
	t.Run("unknown from state", func(t *testing.T) {
		assertInvalidState(t, SessionState("BOGUS").CanTransitionTo(SessionStateActive))
	})
	t.Run("unknown to state", func(t *testing.T) {
		assertInvalidState(t, SessionStateCreated.CanTransitionTo(SessionState("BOGUS")))
	})
}

func TestTargetEpisodeState_CanTransitionTo(t *testing.T) {
	members := []TargetEpisodeState{TargetEpisodeStateOpen, TargetEpisodeStateClosed}
	legal := map[[2]TargetEpisodeState]bool{
		{TargetEpisodeStateOpen, TargetEpisodeStateClosed}: true,
	}
	for _, from := range members {
		for _, to := range members {
			from, to := from, to
			t.Run(string(from)+"->"+string(to), func(t *testing.T) {
				err := from.CanTransitionTo(to)
				if legal[[2]TargetEpisodeState{from, to}] {
					assertNoTransitionErr(t, err)
				} else {
					assertInvalidTransition(t, err)
				}
			})
		}
	}
	t.Run("closed never returns to open", func(t *testing.T) {
		assertInvalidTransition(t, TargetEpisodeStateClosed.CanTransitionTo(TargetEpisodeStateOpen))
	})
	t.Run("unknown from state", func(t *testing.T) {
		assertInvalidState(t, TargetEpisodeState("BOGUS").CanTransitionTo(TargetEpisodeStateClosed))
	})
	t.Run("unknown to state", func(t *testing.T) {
		assertInvalidState(t, TargetEpisodeStateOpen.CanTransitionTo(TargetEpisodeState("BOGUS")))
	})
}

func TestTurnState_CanTransitionTo(t *testing.T) {
	members := []TurnState{
		TurnStateAdmitted, TurnStateRunning, TurnStateCompleted, TurnStateFailed, TurnStateCanceled,
	}
	legal := map[[2]TurnState]bool{
		{TurnStateAdmitted, TurnStateRunning}:  true,
		{TurnStateAdmitted, TurnStateCanceled}: true,
		{TurnStateRunning, TurnStateCompleted}: true,
		{TurnStateRunning, TurnStateFailed}:    true,
		{TurnStateRunning, TurnStateCanceled}:  true,
	}
	for _, from := range members {
		for _, to := range members {
			from, to := from, to
			t.Run(string(from)+"->"+string(to), func(t *testing.T) {
				err := from.CanTransitionTo(to)
				if legal[[2]TurnState{from, to}] {
					assertNoTransitionErr(t, err)
				} else {
					assertInvalidTransition(t, err)
				}
			})
		}
	}
	t.Run("terminal states reject every transition", func(t *testing.T) {
		for _, terminal := range []TurnState{TurnStateCompleted, TurnStateFailed, TurnStateCanceled} {
			for _, to := range members {
				assertInvalidTransition(t, terminal.CanTransitionTo(to))
			}
		}
	})
	t.Run("unknown from state", func(t *testing.T) {
		assertInvalidState(t, TurnState("BOGUS").CanTransitionTo(TurnStateRunning))
	})
	t.Run("unknown to state", func(t *testing.T) {
		assertInvalidState(t, TurnStateAdmitted.CanTransitionTo(TurnState("BOGUS")))
	})
}

func TestControlIntentState_CanTransitionTo(t *testing.T) {
	members := []ControlIntentState{
		ControlIntentStateRequested, ControlIntentStateCommitted,
		ControlIntentStateCompleted, ControlIntentStateNoop, ControlIntentStateSuperseded,
	}
	legal := map[[2]ControlIntentState]bool{
		{ControlIntentStateRequested, ControlIntentStateCommitted}:  true,
		{ControlIntentStateCommitted, ControlIntentStateCompleted}:  true,
		{ControlIntentStateCommitted, ControlIntentStateNoop}:       true,
		{ControlIntentStateCommitted, ControlIntentStateSuperseded}: true,
	}
	for _, from := range members {
		for _, to := range members {
			from, to := from, to
			t.Run(string(from)+"->"+string(to), func(t *testing.T) {
				err := from.CanTransitionTo(to)
				if legal[[2]ControlIntentState{from, to}] {
					assertNoTransitionErr(t, err)
				} else {
					assertInvalidTransition(t, err)
				}
			})
		}
	}
	t.Run("terminal outcomes reject every transition and remain distinct", func(t *testing.T) {
		for _, terminal := range []ControlIntentState{
			ControlIntentStateCompleted, ControlIntentStateNoop, ControlIntentStateSuperseded,
		} {
			for _, to := range members {
				assertInvalidTransition(t, terminal.CanTransitionTo(to))
			}
		}
		if ControlIntentStateNoop == ControlIntentStateSuperseded {
			t.Fatalf("NOOP and SUPERSEDED must remain distinct outcomes")
		}
	})
	t.Run("unknown from state", func(t *testing.T) {
		assertInvalidState(t, ControlIntentState("BOGUS").CanTransitionTo(ControlIntentStateCommitted))
	})
	t.Run("unknown to state", func(t *testing.T) {
		assertInvalidState(t, ControlIntentStateRequested.CanTransitionTo(ControlIntentState("BOGUS")))
	})
}

func TestControlAction_L1Support(t *testing.T) {
	tests := []struct {
		name          string
		action        ControlAction
		wantErr       error
		wantSupported bool
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
			assertSentinel(t, tt.action.Validate(), tt.wantErr)
			if got := tt.action.SupportedInL1(); got != tt.wantSupported {
				t.Fatalf("SupportedInL1() = %v, want %v", got, tt.wantSupported)
			}
		})
	}
}
