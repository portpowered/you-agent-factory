package chatsessions

import (
	"errors"
	"testing"
	"time"
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

func TestTurnState_IsBusy(t *testing.T) {
	tests := []struct {
		state  TurnState
		isBusy bool
	}{
		{TurnStateAdmitted, true},
		{TurnStateRunning, true},
		{TurnStateCompleted, false},
		{TurnStateFailed, false},
		{TurnStateCanceled, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if got := tt.state.IsBusy(); got != tt.isBusy {
				t.Fatalf("%s.IsBusy() = %v, want %v", tt.state, got, tt.isBusy)
			}
			if tt.state.IsBusy() == tt.state.IsTerminal() {
				t.Fatalf("%s: IsBusy() and IsTerminal() must disagree over every declared TurnState member", tt.state)
			}
		})
	}
}

func TestCloseTargetEpisode(t *testing.T) {
	started := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	closedAt := started.Add(time.Minute)
	open := TargetEpisode{
		Number: 1, State: TargetEpisodeStateOpen,
		Target:    ChatTargetRef{Kind: ChatTargetKindFactory, Ref: "@you/review"},
		StartedAt: started,
	}

	closed, err := CloseTargetEpisode(open, closedAt)
	if err != nil {
		t.Fatalf("CloseTargetEpisode(OPEN): %v", err)
	}
	if closed.State != TargetEpisodeStateClosed {
		t.Fatalf("CloseTargetEpisode: got state %v, want CLOSED", closed.State)
	}
	if closed.ClosedAt == nil || !closed.ClosedAt.Equal(closedAt) {
		t.Fatalf("CloseTargetEpisode: got ClosedAt %v, want %v", closed.ClosedAt, closedAt)
	}
	if closed.Number != open.Number || closed.Target != open.Target {
		t.Fatalf("CloseTargetEpisode: Number/Target must be preserved, got %+v from %+v", closed, open)
	}
	if open.State != TargetEpisodeStateOpen || open.ClosedAt != nil {
		t.Fatalf("CloseTargetEpisode: input must not be mutated, got %+v", open)
	}

	alreadyClosed := closed
	if _, err := CloseTargetEpisode(alreadyClosed, closedAt.Add(time.Minute)); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("CloseTargetEpisode(CLOSED): got %v, want ErrInvalidTransition", err)
	}

	unknown := open
	unknown.State = "BOGUS"
	if _, err := CloseTargetEpisode(unknown, closedAt); !errors.Is(err, ErrUnknownEnumValue) {
		t.Fatalf("CloseTargetEpisode(unknown state): got %v, want ErrUnknownEnumValue", err)
	}
}

func TestOpenNextTargetEpisode(t *testing.T) {
	started := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	closedAt := started.Add(time.Minute)
	nextStarted := closedAt
	factoryTarget := ChatTargetRef{Kind: ChatTargetKindFactory, Ref: "@you/review"}
	nextTarget := ChatTargetRef{Kind: ChatTargetKindFactory, Ref: "@you/factory-builder"}

	closedPrior := TargetEpisode{
		Number: 1, State: TargetEpisodeStateClosed,
		Target: factoryTarget, StartedAt: started, ClosedAt: &closedAt,
	}

	next, err := OpenNextTargetEpisode(closedPrior, nextTarget, nextStarted)
	if err != nil {
		t.Fatalf("OpenNextTargetEpisode(CLOSED prior): %v", err)
	}
	if next.Number != closedPrior.Number+1 {
		t.Fatalf("OpenNextTargetEpisode: got Number %d, want %d", next.Number, closedPrior.Number+1)
	}
	if next.State != TargetEpisodeStateOpen {
		t.Fatalf("OpenNextTargetEpisode: got state %v, want OPEN", next.State)
	}
	if next.Target != nextTarget {
		t.Fatalf("OpenNextTargetEpisode: got target %+v, want %+v", next.Target, nextTarget)
	}
	if closedPrior.Target != factoryTarget || closedPrior.Number != 1 || closedPrior.State != TargetEpisodeStateClosed {
		t.Fatalf("OpenNextTargetEpisode: prior episode's identity must never be rewritten, got %+v", closedPrior)
	}

	openPrior := closedPrior
	openPrior.State = TargetEpisodeStateOpen
	openPrior.ClosedAt = nil
	_, openErr := OpenNextTargetEpisode(openPrior, nextTarget, nextStarted)
	if !errors.Is(openErr, ErrTargetEpisodeNotClosed) {
		t.Fatalf("OpenNextTargetEpisode(OPEN prior): got %v, want ErrTargetEpisodeNotClosed", openErr)
	}
	var notClosed *TargetEpisodeNotClosedError
	if !errors.As(openErr, &notClosed) {
		t.Fatalf("OpenNextTargetEpisode(OPEN prior): want *TargetEpisodeNotClosedError, got %T", openErr)
	}

	for _, state := range []TargetEpisodeState{"", "BOGUS"} {
		invalidPrior := closedPrior
		invalidPrior.State = state
		_, err := OpenNextTargetEpisode(invalidPrior, nextTarget, nextStarted)
		if !errors.Is(err, ErrUnknownEnumValue) {
			t.Fatalf("OpenNextTargetEpisode(prior state %q): got %v, want ErrUnknownEnumValue", state, err)
		}
		if errors.Is(err, ErrTargetEpisodeNotClosed) {
			t.Fatalf("OpenNextTargetEpisode(prior state %q): must classify as invalid-state, not ErrTargetEpisodeNotClosed", state)
		}
	}

	if _, err := OpenNextTargetEpisode(closedPrior, ChatTargetRef{}, nextStarted); !errors.Is(err, ErrUnknownEnumValue) {
		t.Fatalf("OpenNextTargetEpisode(invalid target): got %v, want ErrUnknownEnumValue", err)
	}
}

func TestResolveControlIntentOutcome(t *testing.T) {
	const capturedTurn = "turn-1"
	const otherTurn = "turn-2"

	tests := []struct {
		name              string
		capturedTurnState TurnState
		currentActiveID   string
		want              ControlIntentState
	}{
		{"still current and running -> completed", TurnStateRunning, capturedTurn, ControlIntentStateCompleted},
		{"still current and admitted -> completed", TurnStateAdmitted, capturedTurn, ControlIntentStateCompleted},
		{"still current but already completed -> noop", TurnStateCompleted, capturedTurn, ControlIntentStateNoop},
		{"still current but already failed -> noop", TurnStateFailed, capturedTurn, ControlIntentStateNoop},
		{"still current but already canceled -> noop", TurnStateCanceled, capturedTurn, ControlIntentStateNoop},
		{"no longer current, captured was running -> superseded", TurnStateRunning, otherTurn, ControlIntentStateSuperseded},
		{"no longer current, captured was terminal -> superseded, not noop", TurnStateCompleted, otherTurn, ControlIntentStateSuperseded},
		{"no longer current, no active turn at all -> superseded", TurnStateRunning, "", ControlIntentStateSuperseded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveControlIntentOutcome(capturedTurn, tt.capturedTurnState, tt.currentActiveID)
			if got != tt.want {
				t.Fatalf("ResolveControlIntentOutcome(%q, %s, %q) = %v, want %v",
					capturedTurn, tt.capturedTurnState, tt.currentActiveID, got, tt.want)
			}
		})
	}

	t.Run("never rebinds to a later admitted turn", func(t *testing.T) {
		got := ResolveControlIntentOutcome(capturedTurn, TurnStateAdmitted, otherTurn)
		if got != ControlIntentStateSuperseded {
			t.Fatalf("a later admitted turn must never be completed by an older intent: got %v, want SUPERSEDED", got)
		}
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
