package chatsessions

import (
	"errors"
	"testing"
)

func TestSessionStateValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		state   SessionState
		wantErr bool
	}{
		{name: "created is valid", state: SessionStateCreated},
		{name: "active is valid", state: SessionStateActive},
		{name: "closed is valid", state: SessionStateClosed},
		{name: "zero value is invalid", state: SessionState(""), wantErr: true},
		{name: "unknown value is invalid", state: SessionState("PAUSED"), wantErr: true},
		{name: "lowercase known value is invalid", state: SessionState("created"), wantErr: true},
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
			var invalid *InvalidSessionStateError
			if !errors.As(err, &invalid) {
				t.Fatalf("Validate() error = %v (%T), want *InvalidSessionStateError", err, err)
			}
		})
	}
}

func TestTransitionSessionState(t *testing.T) {
	t.Parallel()

	allKnown := []SessionState{SessionStateCreated, SessionStateActive, SessionStateClosed}
	legal := map[SessionState]map[SessionState]bool{
		SessionStateCreated: {SessionStateActive: true, SessionStateClosed: true},
		SessionStateActive:  {SessionStateClosed: true},
		SessionStateClosed:  {},
	}

	for _, from := range allKnown {
		for _, to := range allKnown {
			from, to := from, to
			wantLegal := legal[from][to]
			t.Run(string(from)+"_to_"+string(to), func(t *testing.T) {
				t.Parallel()

				got, err := TransitionSessionState(from, to)
				if wantLegal {
					if err != nil {
						t.Fatalf("TransitionSessionState(%q, %q) error = %v, want nil", from, to, err)
					}
					if got != to {
						t.Fatalf("TransitionSessionState(%q, %q) = %q, want %q", from, to, got, to)
					}
					return
				}

				if err == nil {
					t.Fatalf("TransitionSessionState(%q, %q) error = nil, want error", from, to)
				}
				if got != from {
					t.Fatalf("TransitionSessionState(%q, %q) = %q, want unchanged %q", from, to, got, from)
				}
				var invalid *InvalidSessionStateTransitionError
				if !errors.As(err, &invalid) {
					t.Fatalf("TransitionSessionState(%q, %q) error = %v (%T), want *InvalidSessionStateTransitionError", from, to, err, err)
				}
				if invalid.From != from || invalid.To != to {
					t.Fatalf("error From/To = %q/%q, want %q/%q", invalid.From, invalid.To, from, to)
				}
			})
		}
	}

	t.Run("zero from is invalid value not invalid transition", func(t *testing.T) {
		t.Parallel()

		got, err := TransitionSessionState(SessionState(""), SessionStateActive)
		if err == nil {
			t.Fatalf("TransitionSessionState(zero, ACTIVE) error = nil, want error")
		}
		if got != SessionState("") {
			t.Fatalf("TransitionSessionState(zero, ACTIVE) = %q, want unchanged zero value", got)
		}
		var invalid *InvalidSessionStateError
		if !errors.As(err, &invalid) {
			t.Fatalf("error = %v (%T), want *InvalidSessionStateError", err, err)
		}
	})

	t.Run("unknown to is invalid value not invalid transition", func(t *testing.T) {
		t.Parallel()

		got, err := TransitionSessionState(SessionStateCreated, SessionState("PAUSED"))
		if err == nil {
			t.Fatalf("TransitionSessionState(CREATED, unknown) error = nil, want error")
		}
		if got != SessionStateCreated {
			t.Fatalf("TransitionSessionState(CREATED, unknown) = %q, want unchanged CREATED", got)
		}
		var invalid *InvalidSessionStateError
		if !errors.As(err, &invalid) {
			t.Fatalf("error = %v (%T), want *InvalidSessionStateError", err, err)
		}
	})
}
