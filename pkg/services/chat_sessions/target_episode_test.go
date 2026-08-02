package chatsessions

import (
	"errors"
	"testing"
	"time"
)

func TestTargetEpisodeStateValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		state   TargetEpisodeState
		wantErr bool
	}{
		{name: "open is valid", state: TargetEpisodeStateOpen},
		{name: "closed is valid", state: TargetEpisodeStateClosed},
		{name: "zero value is invalid", state: TargetEpisodeState(""), wantErr: true},
		{name: "unknown value is invalid", state: TargetEpisodeState("PENDING"), wantErr: true},
		{name: "lowercase known value is invalid", state: TargetEpisodeState("open"), wantErr: true},
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
			var invalid *InvalidTargetEpisodeStateError
			if !errors.As(err, &invalid) {
				t.Fatalf("Validate() error = %v (%T), want *InvalidTargetEpisodeStateError", err, err)
			}
		})
	}
}

func TestTransitionTargetEpisodeState(t *testing.T) {
	t.Parallel()

	allKnown := []TargetEpisodeState{TargetEpisodeStateOpen, TargetEpisodeStateClosed}
	legal := map[TargetEpisodeState]map[TargetEpisodeState]bool{
		TargetEpisodeStateOpen:   {TargetEpisodeStateClosed: true},
		TargetEpisodeStateClosed: {},
	}

	for _, from := range allKnown {
		for _, to := range allKnown {
			from, to := from, to
			wantLegal := legal[from][to]
			t.Run(string(from)+"_to_"+string(to), func(t *testing.T) {
				t.Parallel()

				got, err := TransitionTargetEpisodeState(from, to)
				if wantLegal {
					if err != nil {
						t.Fatalf("TransitionTargetEpisodeState(%q, %q) error = %v, want nil", from, to, err)
					}
					if got != to {
						t.Fatalf("TransitionTargetEpisodeState(%q, %q) = %q, want %q", from, to, got, to)
					}
					return
				}

				if err == nil {
					t.Fatalf("TransitionTargetEpisodeState(%q, %q) error = nil, want error", from, to)
				}
				if got != from {
					t.Fatalf("TransitionTargetEpisodeState(%q, %q) = %q, want unchanged %q", from, to, got, from)
				}
				var invalid *InvalidTargetEpisodeStateTransitionError
				if !errors.As(err, &invalid) {
					t.Fatalf("TransitionTargetEpisodeState(%q, %q) error = %v (%T), want *InvalidTargetEpisodeStateTransitionError", from, to, err, err)
				}
			})
		}
	}

	t.Run("zero from is invalid value not invalid transition", func(t *testing.T) {
		t.Parallel()

		got, err := TransitionTargetEpisodeState(TargetEpisodeState(""), TargetEpisodeStateClosed)
		if err == nil {
			t.Fatalf("error = nil, want error")
		}
		if got != TargetEpisodeState("") {
			t.Fatalf("got = %q, want unchanged zero value", got)
		}
		var invalid *InvalidTargetEpisodeStateError
		if !errors.As(err, &invalid) {
			t.Fatalf("error = %v (%T), want *InvalidTargetEpisodeStateError", err, err)
		}
	})

	t.Run("unknown to is invalid value not invalid transition", func(t *testing.T) {
		t.Parallel()

		got, err := TransitionTargetEpisodeState(TargetEpisodeStateOpen, TargetEpisodeState("PENDING"))
		if err == nil {
			t.Fatalf("error = nil, want error")
		}
		if got != TargetEpisodeStateOpen {
			t.Fatalf("got = %q, want unchanged OPEN", got)
		}
		var invalid *InvalidTargetEpisodeStateError
		if !errors.As(err, &invalid) {
			t.Fatalf("error = %v (%T), want *InvalidTargetEpisodeStateError", err, err)
		}
	})
}

func TestCloseTargetEpisode(t *testing.T) {
	t.Parallel()

	target := ChatTargetRef{Kind: ChatTargetKindFactory, Ref: "@you/factory-builder"}
	started := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	open := TargetEpisode{
		Number:           1,
		State:            TargetEpisodeStateOpen,
		Target:           target,
		FactorySessionID: "factory-session-1",
		StartedAt:        started,
	}

	closedAt := started.Add(time.Hour)
	closed, err := CloseTargetEpisode(open, closedAt)
	if err != nil {
		t.Fatalf("CloseTargetEpisode() error = %v, want nil", err)
	}
	if closed.State != TargetEpisodeStateClosed {
		t.Fatalf("closed.State = %q, want CLOSED", closed.State)
	}
	if closed.ClosedAt == nil || !closed.ClosedAt.Equal(closedAt) {
		t.Fatalf("closed.ClosedAt = %v, want %v", closed.ClosedAt, closedAt)
	}
	if closed.Number != open.Number || closed.Target != open.Target || closed.FactorySessionID != open.FactorySessionID {
		t.Fatalf("closed identity fields changed: got %+v, want Number/Target/FactorySessionID from %+v", closed, open)
	}
	if open.State != TargetEpisodeStateOpen || open.ClosedAt != nil {
		t.Fatalf("input episode was mutated: %+v", open)
	}

	t.Run("already closed episode rejects re-close", func(t *testing.T) {
		t.Parallel()

		_, err := CloseTargetEpisode(closed, closedAt.Add(time.Hour))
		if err == nil {
			t.Fatalf("CloseTargetEpisode() error = nil, want error")
		}
		var invalid *InvalidTargetEpisodeStateTransitionError
		if !errors.As(err, &invalid) {
			t.Fatalf("error = %v (%T), want *InvalidTargetEpisodeStateTransitionError", err, err)
		}
	})
}

func TestOpenNextTargetEpisode(t *testing.T) {
	t.Parallel()

	original := ChatTargetRef{Kind: ChatTargetKindFactory, Ref: "@you/factory-builder"}
	started := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	closedAt := started.Add(time.Hour)
	closed := TargetEpisode{
		Number:           1,
		State:            TargetEpisodeStateClosed,
		Target:           original,
		FactorySessionID: "factory-session-1",
		StartedAt:        started,
		ClosedAt:         &closedAt,
	}

	nextTarget := ChatTargetRef{Kind: ChatTargetKindFactory, Ref: "@you/factory-reviewer"}
	nextStarted := closedAt
	next, err := OpenNextTargetEpisode(closed, nextTarget, "factory-session-2", nextStarted)
	if err != nil {
		t.Fatalf("OpenNextTargetEpisode() error = %v, want nil", err)
	}
	if next.Number != closed.Number+1 {
		t.Fatalf("next.Number = %d, want %d", next.Number, closed.Number+1)
	}
	if next.State != TargetEpisodeStateOpen {
		t.Fatalf("next.State = %q, want OPEN", next.State)
	}
	if next.Target != nextTarget {
		t.Fatalf("next.Target = %+v, want %+v", next.Target, nextTarget)
	}
	if closed.Target != original {
		t.Fatalf("prior episode Target was rewritten: got %+v, want %+v", closed.Target, original)
	}

	t.Run("open prior rejects opening next episode", func(t *testing.T) {
		t.Parallel()

		open := TargetEpisode{Number: 1, State: TargetEpisodeStateOpen, Target: original}
		_, err := OpenNextTargetEpisode(open, nextTarget, "factory-session-2", nextStarted)
		if err == nil {
			t.Fatalf("OpenNextTargetEpisode() error = nil, want error")
		}
		var invalid *TargetEpisodeNotClosedError
		if !errors.As(err, &invalid) {
			t.Fatalf("error = %v (%T), want *TargetEpisodeNotClosedError", err, err)
		}
	})

	t.Run("invalid next target is rejected", func(t *testing.T) {
		t.Parallel()

		_, err := OpenNextTargetEpisode(closed, ChatTargetRef{Kind: ChatTargetKindFactory, Ref: ""}, "factory-session-2", nextStarted)
		if err == nil {
			t.Fatalf("OpenNextTargetEpisode() error = nil, want error")
		}
		var invalid *InvalidChatTargetRefError
		if !errors.As(err, &invalid) {
			t.Fatalf("error = %v (%T), want *InvalidChatTargetRefError", err, err)
		}
	})
}
