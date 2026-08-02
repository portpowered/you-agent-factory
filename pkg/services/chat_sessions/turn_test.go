package chatsessions

import (
	"errors"
	"testing"
)

func TestTurnStateValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		state   TurnState
		wantErr bool
	}{
		{name: "admitted is valid", state: TurnStateAdmitted},
		{name: "running is valid", state: TurnStateRunning},
		{name: "completed is valid", state: TurnStateCompleted},
		{name: "failed is valid", state: TurnStateFailed},
		{name: "canceled is valid", state: TurnStateCanceled},
		{name: "zero value is invalid", state: TurnState(""), wantErr: true},
		{name: "unknown value is invalid", state: TurnState("PAUSED"), wantErr: true},
		{name: "lowercase known value is invalid", state: TurnState("admitted"), wantErr: true},
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
			var invalid *InvalidTurnStateError
			if !errors.As(err, &invalid) {
				t.Fatalf("Validate() error = %v (%T), want *InvalidTurnStateError", err, err)
			}
		})
	}
}

func TestTransitionTurnState(t *testing.T) {
	t.Parallel()

	allKnown := []TurnState{
		TurnStateAdmitted, TurnStateRunning, TurnStateCompleted, TurnStateFailed, TurnStateCanceled,
	}
	legal := map[TurnState]map[TurnState]bool{
		TurnStateAdmitted:  {TurnStateRunning: true, TurnStateCanceled: true},
		TurnStateRunning:   {TurnStateCompleted: true, TurnStateFailed: true, TurnStateCanceled: true},
		TurnStateCompleted: {},
		TurnStateFailed:    {},
		TurnStateCanceled:  {},
	}

	for _, from := range allKnown {
		for _, to := range allKnown {
			from, to := from, to
			wantLegal := legal[from][to]
			t.Run(string(from)+"_to_"+string(to), func(t *testing.T) {
				t.Parallel()

				got, err := TransitionTurnState(from, to)
				if wantLegal {
					if err != nil {
						t.Fatalf("TransitionTurnState(%q, %q) error = %v, want nil", from, to, err)
					}
					if got != to {
						t.Fatalf("TransitionTurnState(%q, %q) = %q, want %q", from, to, got, to)
					}
					return
				}

				if err == nil {
					t.Fatalf("TransitionTurnState(%q, %q) error = nil, want error", from, to)
				}
				if got != from {
					t.Fatalf("TransitionTurnState(%q, %q) = %q, want unchanged %q", from, to, got, from)
				}
				var invalid *InvalidTurnStateTransitionError
				if !errors.As(err, &invalid) {
					t.Fatalf("TransitionTurnState(%q, %q) error = %v (%T), want *InvalidTurnStateTransitionError", from, to, err, err)
				}
				if invalid.From != from || invalid.To != to {
					t.Fatalf("error From/To = %q/%q, want %q/%q", invalid.From, invalid.To, from, to)
				}
			})
		}
	}

	t.Run("zero from is invalid value not invalid transition", func(t *testing.T) {
		t.Parallel()

		got, err := TransitionTurnState(TurnState(""), TurnStateRunning)
		if err == nil {
			t.Fatalf("TransitionTurnState(zero, RUNNING) error = nil, want error")
		}
		if got != TurnState("") {
			t.Fatalf("TransitionTurnState(zero, RUNNING) = %q, want unchanged zero value", got)
		}
		var invalid *InvalidTurnStateError
		if !errors.As(err, &invalid) {
			t.Fatalf("error = %v (%T), want *InvalidTurnStateError", err, err)
		}
	})

	t.Run("unknown to is invalid value not invalid transition", func(t *testing.T) {
		t.Parallel()

		got, err := TransitionTurnState(TurnStateAdmitted, TurnState("PAUSED"))
		if err == nil {
			t.Fatalf("TransitionTurnState(ADMITTED, unknown) error = nil, want error")
		}
		if got != TurnStateAdmitted {
			t.Fatalf("TransitionTurnState(ADMITTED, unknown) = %q, want unchanged ADMITTED", got)
		}
		var invalid *InvalidTurnStateError
		if !errors.As(err, &invalid) {
			t.Fatalf("error = %v (%T), want *InvalidTurnStateError", err, err)
		}
	})

	t.Run("every terminal state has no outbound transition", func(t *testing.T) {
		t.Parallel()

		for _, terminal := range []TurnState{TurnStateCompleted, TurnStateFailed, TurnStateCanceled} {
			for _, to := range allKnown {
				if _, err := TransitionTurnState(terminal, to); err == nil {
					t.Fatalf("TransitionTurnState(%q, %q) error = nil, want error (terminal state must have no outbound transition)", terminal, to)
				}
			}
		}
	})
}

func TestCheckTurnAdmission(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		prior    *Turn
		wantBusy bool
	}{
		{name: "absent prior turn is not busy", prior: nil, wantBusy: false},
		{name: "admitted prior turn is busy", prior: &Turn{ID: "turn-1", State: TurnStateAdmitted}, wantBusy: true},
		{name: "running prior turn is busy", prior: &Turn{ID: "turn-1", State: TurnStateRunning}, wantBusy: true},
		{name: "completed prior turn is not busy", prior: &Turn{ID: "turn-1", State: TurnStateCompleted}, wantBusy: false},
		{name: "failed prior turn is not busy", prior: &Turn{ID: "turn-1", State: TurnStateFailed}, wantBusy: false},
		{name: "canceled prior turn is not busy", prior: &Turn{ID: "turn-1", State: TurnStateCanceled}, wantBusy: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := CheckTurnAdmission("session-1", test.prior)
			if !test.wantBusy {
				if err != nil {
					t.Fatalf("CheckTurnAdmission() = %v, want nil", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("CheckTurnAdmission() = nil, want *TurnBusyError")
			}
			var busy *TurnBusyError
			if !errors.As(err, &busy) {
				t.Fatalf("CheckTurnAdmission() error = %v (%T), want *TurnBusyError", err, err)
			}
			if busy.SessionID != "session-1" {
				t.Fatalf("busy.SessionID = %q, want session-1", busy.SessionID)
			}
			if busy.ActiveTurnID != test.prior.ID {
				t.Fatalf("busy.ActiveTurnID = %q, want %q", busy.ActiveTurnID, test.prior.ID)
			}
			if busy.ActiveTurnState != test.prior.State {
				t.Fatalf("busy.ActiveTurnState = %q, want %q", busy.ActiveTurnState, test.prior.State)
			}
		})
	}
}

func TestCheckVersion(t *testing.T) {
	t.Parallel()

	t.Run("matching version is valid", func(t *testing.T) {
		t.Parallel()

		if err := CheckVersion(3, 3); err != nil {
			t.Fatalf("CheckVersion(3, 3) = %v, want nil", err)
		}
	})

	t.Run("mismatched version is a typed conflict", func(t *testing.T) {
		t.Parallel()

		err := CheckVersion(3, 4)
		if err == nil {
			t.Fatalf("CheckVersion(3, 4) = nil, want error")
		}
		var conflict *VersionConflictError
		if !errors.As(err, &conflict) {
			t.Fatalf("CheckVersion(3, 4) error = %v (%T), want *VersionConflictError", err, err)
		}
		if conflict.Expected != 3 || conflict.Actual != 4 {
			t.Fatalf("conflict = %+v, want Expected=3 Actual=4", conflict)
		}
	})
}

func TestTransitionTurnStateChecked(t *testing.T) {
	t.Parallel()

	t.Run("matching version applies the legal transition", func(t *testing.T) {
		t.Parallel()

		turn := Turn{ID: "turn-1", State: TurnStateAdmitted}
		got, err := TransitionTurnStateChecked(turn, TurnStateRunning, 5, 5)
		if err != nil {
			t.Fatalf("TransitionTurnStateChecked() error = %v, want nil", err)
		}
		if got.State != TurnStateRunning {
			t.Fatalf("got.State = %q, want RUNNING", got.State)
		}
		if got.ID != turn.ID {
			t.Fatalf("got.ID = %q, want unchanged %q", got.ID, turn.ID)
		}
		if turn.State != TurnStateAdmitted {
			t.Fatalf("input turn.State mutated to %q, want unchanged ADMITTED", turn.State)
		}
	})

	t.Run("mismatched version returns a conflict and leaves turn unchanged", func(t *testing.T) {
		t.Parallel()

		turn := Turn{ID: "turn-1", State: TurnStateAdmitted}
		got, err := TransitionTurnStateChecked(turn, TurnStateRunning, 5, 6)
		if err == nil {
			t.Fatalf("TransitionTurnStateChecked() error = nil, want *VersionConflictError")
		}
		var conflict *VersionConflictError
		if !errors.As(err, &conflict) {
			t.Fatalf("error = %v (%T), want *VersionConflictError", err, err)
		}
		if got != turn {
			t.Fatalf("got = %+v, want unchanged %+v", got, turn)
		}
	})

	t.Run("matching version but illegal transition returns transition error and leaves turn unchanged", func(t *testing.T) {
		t.Parallel()

		turn := Turn{ID: "turn-1", State: TurnStateCompleted}
		got, err := TransitionTurnStateChecked(turn, TurnStateRunning, 5, 5)
		if err == nil {
			t.Fatalf("TransitionTurnStateChecked() error = nil, want *InvalidTurnStateTransitionError")
		}
		var invalid *InvalidTurnStateTransitionError
		if !errors.As(err, &invalid) {
			t.Fatalf("error = %v (%T), want *InvalidTurnStateTransitionError", err, err)
		}
		if got != turn {
			t.Fatalf("got = %+v, want unchanged %+v", got, turn)
		}
	})
}
