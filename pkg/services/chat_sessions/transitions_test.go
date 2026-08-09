package chatsessions

import (
	"errors"
	"math"
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

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
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

	if got, err := CloseTargetEpisode(open, time.Time{}); !errors.Is(err, ErrInconsistentValue) {
		t.Fatalf("CloseTargetEpisode(zero closedAt): got (%+v, %v), want ErrInconsistentValue", got, err)
	} else if got != open {
		t.Fatalf("CloseTargetEpisode(zero closedAt): input must not be mutated, got %+v, want %+v", got, open)
	}

	if got, err := CloseTargetEpisode(open, started.Add(-time.Minute)); !errors.Is(err, ErrInconsistentValue) {
		t.Fatalf("CloseTargetEpisode(closedAt before StartedAt): got (%+v, %v), want ErrInconsistentValue", got, err)
	} else if got != open {
		t.Fatalf("CloseTargetEpisode(closedAt before StartedAt): input must not be mutated, got %+v, want %+v", got, open)
	}

	// A structurally corrupt source (OPEN but already carrying a non-nil
	// ClosedAt) must be rejected by prior.Validate() rather than silently
	// repaired by overwriting ClosedAt with the caller's new value.
	corruptOpen := open
	corruptOpen.ClosedAt = &closedAt
	if got, err := CloseTargetEpisode(corruptOpen, closedAt.Add(time.Minute)); !errors.Is(err, ErrInconsistentValue) {
		t.Fatalf("CloseTargetEpisode(OPEN with non-nil ClosedAt): got (%+v, %v), want ErrInconsistentValue", got, err)
	} else if got != corruptOpen {
		t.Fatalf("CloseTargetEpisode(OPEN with non-nil ClosedAt): input must not be mutated, got %+v, want %+v", got, corruptOpen)
	}
}

// openNextTargetEpisodeFixture returns the shared timestamps, targets, and a
// valid CLOSED prior episode used by the TestOpenNextTargetEpisode* family.
func openNextTargetEpisodeFixture() (started, closedAt, nextStarted time.Time, factoryTarget, nextTarget ChatTargetRef, priorFactorySessionID, nextFactorySessionID string, closedPrior TargetEpisode) {
	started = time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	closedAt = started.Add(time.Minute)
	nextStarted = closedAt
	factoryTarget = ChatTargetRef{Kind: ChatTargetKindFactory, Ref: "@you/review"}
	nextTarget = ChatTargetRef{Kind: ChatTargetKindFactory, Ref: "@you/factory-builder"}
	priorFactorySessionID = "fsession-1"
	nextFactorySessionID = "fsession-2"
	closedPrior = TargetEpisode{
		Number: 1, State: TargetEpisodeStateClosed,
		Target: factoryTarget, FactorySessionID: priorFactorySessionID,
		StartedAt: started, ClosedAt: &closedAt,
	}
	return
}

func TestOpenNextTargetEpisode_SuccessFromClosedPrior(t *testing.T) {
	_, _, nextStarted, factoryTarget, nextTarget, priorFactorySessionID, nextFactorySessionID, closedPrior := openNextTargetEpisodeFixture()

	next, err := OpenNextTargetEpisode(closedPrior, nextTarget, nextFactorySessionID, nextStarted)
	if err != nil {
		t.Fatalf("OpenNextTargetEpisode(CLOSED prior): %v", err)
	}
	if err := next.Validate(); err != nil {
		t.Fatalf("OpenNextTargetEpisode(CLOSED prior): result fails its own Validate(): %v", err)
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
	if next.FactorySessionID != nextFactorySessionID || next.FactorySessionID == closedPrior.FactorySessionID {
		t.Fatalf("OpenNextTargetEpisode: got FactorySessionID %q, want the new %q distinct from prior's %q",
			next.FactorySessionID, nextFactorySessionID, closedPrior.FactorySessionID)
	}
	if closedPrior.Target != factoryTarget || closedPrior.Number != 1 || closedPrior.State != TargetEpisodeStateClosed || closedPrior.FactorySessionID != priorFactorySessionID {
		t.Fatalf("OpenNextTargetEpisode: prior episode's identity must never be rewritten, got %+v", closedPrior)
	}
}

func TestOpenNextTargetEpisode_OpenPriorReportsNotClosed(t *testing.T) {
	_, _, nextStarted, _, nextTarget, _, nextFactorySessionID, closedPrior := openNextTargetEpisodeFixture()

	openPrior := closedPrior
	openPrior.State = TargetEpisodeStateOpen
	openPrior.ClosedAt = nil
	_, openErr := OpenNextTargetEpisode(openPrior, nextTarget, nextFactorySessionID, nextStarted)
	if !errors.Is(openErr, ErrTargetEpisodeNotClosed) {
		t.Fatalf("OpenNextTargetEpisode(OPEN prior): got %v, want ErrTargetEpisodeNotClosed", openErr)
	}
	var notClosed *TargetEpisodeNotClosedError
	if !errors.As(openErr, &notClosed) {
		t.Fatalf("OpenNextTargetEpisode(OPEN prior): want *TargetEpisodeNotClosedError, got %T", openErr)
	}
}

func TestOpenNextTargetEpisode_InvalidPriorStateRejected(t *testing.T) {
	_, _, nextStarted, _, nextTarget, _, nextFactorySessionID, closedPrior := openNextTargetEpisodeFixture()

	for _, state := range []TargetEpisodeState{"", "BOGUS"} {
		invalidPrior := closedPrior
		invalidPrior.State = state
		_, err := OpenNextTargetEpisode(invalidPrior, nextTarget, nextFactorySessionID, nextStarted)
		if !errors.Is(err, ErrUnknownEnumValue) {
			t.Fatalf("OpenNextTargetEpisode(prior state %q): got %v, want ErrUnknownEnumValue", state, err)
		}
		if errors.Is(err, ErrTargetEpisodeNotClosed) {
			t.Fatalf("OpenNextTargetEpisode(prior state %q): must classify as invalid-state, not ErrTargetEpisodeNotClosed", state)
		}
	}
}

func TestOpenNextTargetEpisode_InvalidCandidateValuesRejected(t *testing.T) {
	_, closedAt, nextStarted, _, nextTarget, _, nextFactorySessionID, closedPrior := openNextTargetEpisodeFixture()

	if _, err := OpenNextTargetEpisode(closedPrior, ChatTargetRef{}, nextFactorySessionID, nextStarted); !errors.Is(err, ErrUnknownEnumValue) {
		t.Fatalf("OpenNextTargetEpisode(invalid target): got %v, want ErrUnknownEnumValue", err)
	}

	if got, err := OpenNextTargetEpisode(closedPrior, nextTarget, nextFactorySessionID, time.Time{}); !errors.Is(err, ErrRequiredValue) {
		t.Fatalf("OpenNextTargetEpisode(zero startedAt): got (%+v, %v), want ErrRequiredValue", got, err)
	} else if got != (TargetEpisode{}) {
		t.Fatalf("OpenNextTargetEpisode(zero startedAt): must return a zero-value result on error, got %+v", got)
	}

	if got, err := OpenNextTargetEpisode(closedPrior, nextTarget, nextFactorySessionID, closedAt.Add(-time.Minute)); !errors.Is(err, ErrInconsistentValue) {
		t.Fatalf("OpenNextTargetEpisode(startedAt before prior.ClosedAt): got (%+v, %v), want ErrInconsistentValue", got, err)
	} else if got != (TargetEpisode{}) {
		t.Fatalf("OpenNextTargetEpisode(startedAt before prior.ClosedAt): must return a zero-value result on error, got %+v", got)
	}
}

// A prior.Number already at the maximum representable uint64 must be
// rejected before prior.Number+1 is ever computed, so the next episode
// number can never silently wrap to 0 and collide with the very first
// episode's identity.
func TestOpenNextTargetEpisode_ExhaustedNumberRejected(t *testing.T) {
	_, _, nextStarted, _, nextTarget, _, nextFactorySessionID, closedPrior := openNextTargetEpisodeFixture()

	exhaustedPrior := closedPrior
	exhaustedPrior.Number = math.MaxUint64

	got, err := OpenNextTargetEpisode(exhaustedPrior, nextTarget, nextFactorySessionID, nextStarted)
	if !errors.Is(err, ErrTargetEpisodeNumberExhausted) {
		t.Fatalf("OpenNextTargetEpisode(prior.Number == MaxUint64): got (%+v, %v), want ErrTargetEpisodeNumberExhausted", got, err)
	}
	if got != (TargetEpisode{}) {
		t.Fatalf("OpenNextTargetEpisode(prior.Number == MaxUint64): must return a zero-value result on error, got %+v", got)
	}
	if exhaustedPrior.Number != math.MaxUint64 {
		t.Fatalf("OpenNextTargetEpisode(prior.Number == MaxUint64): prior must not be mutated, got Number %d", exhaustedPrior.Number)
	}
}

// A CLOSED prior.State is not, by itself, proof the rest of prior is a
// consistent history fact. Each of these corrupt-source cases must be
// rejected by prior.Validate() rather than silently accepted while building
// the next episode.
func TestOpenNextTargetEpisode_CorruptClosedPriorRejected(t *testing.T) {
	started, closedAt, nextStarted, factoryTarget, nextTarget, priorFactorySessionID, nextFactorySessionID, _ := openNextTargetEpisodeFixture()

	backdated := started.Add(-time.Minute)
	corruptPriors := []struct {
		name  string
		prior TargetEpisode
		want  error
	}{
		{
			name: "CLOSED prior with nil ClosedAt",
			prior: TargetEpisode{
				Number: 1, State: TargetEpisodeStateClosed,
				Target: factoryTarget, FactorySessionID: priorFactorySessionID,
				StartedAt: started, ClosedAt: nil,
			},
			want: ErrInconsistentValue,
		},
		{
			name: "CLOSED prior with invalid Target",
			prior: TargetEpisode{
				Number: 1, State: TargetEpisodeStateClosed,
				Target: ChatTargetRef{}, FactorySessionID: priorFactorySessionID,
				StartedAt: started, ClosedAt: &closedAt,
			},
			want: ErrUnknownEnumValue,
		},
		{
			name: "CLOSED prior with zero StartedAt",
			prior: TargetEpisode{
				Number: 1, State: TargetEpisodeStateClosed,
				Target: factoryTarget, FactorySessionID: priorFactorySessionID,
				StartedAt: time.Time{}, ClosedAt: &closedAt,
			},
			want: ErrRequiredValue,
		},
		{
			name: "CLOSED prior with ClosedAt before StartedAt",
			prior: TargetEpisode{
				Number: 1, State: TargetEpisodeStateClosed,
				Target: factoryTarget, FactorySessionID: priorFactorySessionID,
				StartedAt: started, ClosedAt: &backdated,
			},
			want: ErrInconsistentValue,
		},
	}

	for _, tt := range corruptPriors {
		t.Run(tt.name, func(t *testing.T) {
			beforePrior := tt.prior
			got, err := OpenNextTargetEpisode(tt.prior, nextTarget, nextFactorySessionID, nextStarted)
			if !errors.Is(err, tt.want) {
				t.Fatalf("OpenNextTargetEpisode(%s): got %v, want error wrapping %v", tt.name, err, tt.want)
			}
			if got != (TargetEpisode{}) {
				t.Fatalf("OpenNextTargetEpisode(%s): must return a zero-value result on error, got %+v", tt.name, got)
			}
			if tt.prior != beforePrior {
				t.Fatalf("OpenNextTargetEpisode(%s): prior must not be mutated, got %+v, want %+v", tt.name, tt.prior, beforePrior)
			}
		})
	}
}

// A structurally corrupt OPEN prior (e.g. already carrying a non-nil
// ClosedAt) must be rejected by prior.Validate() with its own typed error,
// not misreported as merely *TargetEpisodeNotClosedError.
func TestOpenNextTargetEpisode_CorruptOpenPriorRejected(t *testing.T) {
	started, closedAt, nextStarted, factoryTarget, nextTarget, priorFactorySessionID, nextFactorySessionID, _ := openNextTargetEpisodeFixture()

	corruptOpen := TargetEpisode{
		Number: 1, State: TargetEpisodeStateOpen,
		Target: factoryTarget, FactorySessionID: priorFactorySessionID,
		StartedAt: started, ClosedAt: &closedAt,
	}
	beforePrior := corruptOpen
	got, err := OpenNextTargetEpisode(corruptOpen, nextTarget, nextFactorySessionID, nextStarted)
	if !errors.Is(err, ErrInconsistentValue) {
		t.Fatalf("OpenNextTargetEpisode(OPEN with non-nil ClosedAt): got %v, want ErrInconsistentValue", err)
	}
	if errors.Is(err, ErrTargetEpisodeNotClosed) {
		t.Fatalf("OpenNextTargetEpisode(OPEN with non-nil ClosedAt): must classify as invalid-value, not ErrTargetEpisodeNotClosed")
	}
	if got != (TargetEpisode{}) {
		t.Fatalf("OpenNextTargetEpisode(OPEN with non-nil ClosedAt): must return a zero-value result on error, got %+v", got)
	}
	if corruptOpen != beforePrior {
		t.Fatalf("OpenNextTargetEpisode(OPEN with non-nil ClosedAt): prior must not be mutated, got %+v, want %+v", corruptOpen, beforePrior)
	}
}

const (
	resolveOutcomeCapturedTurn = "turn-1"
	resolveOutcomeOtherTurn    = "turn-2"
)

func TestResolveControlIntentOutcome_TableDriven(t *testing.T) {
	tests := []struct {
		name              string
		capturedTurnState TurnState
		mostRecentTurnID  string
		want              ControlIntentState
	}{
		{"still most recent and running -> completed", TurnStateRunning, resolveOutcomeCapturedTurn, ControlIntentStateCompleted},
		{"still most recent and admitted -> completed", TurnStateAdmitted, resolveOutcomeCapturedTurn, ControlIntentStateCompleted},
		{"still most recent but already completed -> noop", TurnStateCompleted, resolveOutcomeCapturedTurn, ControlIntentStateNoop},
		{"still most recent but already failed -> noop", TurnStateFailed, resolveOutcomeCapturedTurn, ControlIntentStateNoop},
		{"still most recent but already canceled -> noop", TurnStateCanceled, resolveOutcomeCapturedTurn, ControlIntentStateNoop},
		{"no longer most recent, captured was running -> superseded", TurnStateRunning, resolveOutcomeOtherTurn, ControlIntentStateSuperseded},
		{"no longer most recent, captured was terminal -> superseded, not noop", TurnStateCompleted, resolveOutcomeOtherTurn, ControlIntentStateSuperseded},
		{"no longer most recent, no turn admitted yet -> superseded", TurnStateRunning, "", ControlIntentStateSuperseded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveControlIntentOutcome(resolveOutcomeCapturedTurn, tt.capturedTurnState, tt.mostRecentTurnID)
			if err != nil {
				t.Fatalf("ResolveControlIntentOutcome(%q, %s, %q) returned error %v, want nil",
					resolveOutcomeCapturedTurn, tt.capturedTurnState, tt.mostRecentTurnID, err)
			}
			if got != tt.want {
				t.Fatalf("ResolveControlIntentOutcome(%q, %s, %q) = %v, want %v",
					resolveOutcomeCapturedTurn, tt.capturedTurnState, tt.mostRecentTurnID, got, tt.want)
			}
		})
	}

	t.Run("never rebinds to a later admitted turn", func(t *testing.T) {
		got, err := ResolveControlIntentOutcome(resolveOutcomeCapturedTurn, TurnStateAdmitted, resolveOutcomeOtherTurn)
		if err != nil {
			t.Fatalf("ResolveControlIntentOutcome: got error %v, want nil", err)
		}
		if got != ControlIntentStateSuperseded {
			t.Fatalf("a later admitted turn must never be completed by an older intent: got %v, want SUPERSEDED", got)
		}
	})
}

// An invalid captured TurnState is rejected with a typed error before any
// outcome is selected, both when the captured turn is still the most recent
// one and when an identity mismatch would otherwise resolve to SUPERSEDED --
// the mismatch must never hide the invalid captured state.
func TestResolveControlIntentOutcome_InvalidCapturedStateRejected(t *testing.T) {
	for _, state := range []TurnState{"", "BOGUS"} {
		t.Run("still most recent: "+string(state), func(t *testing.T) {
			got, err := ResolveControlIntentOutcome(resolveOutcomeCapturedTurn, state, resolveOutcomeCapturedTurn)
			if !errors.Is(err, ErrUnknownEnumValue) {
				t.Fatalf("ResolveControlIntentOutcome(captured state %q): got %v, want ErrUnknownEnumValue", state, err)
			}
			if got != "" {
				t.Fatalf("ResolveControlIntentOutcome(captured state %q): got outcome %v, want zero value on error", state, got)
			}
		})

		t.Run("no longer most recent: "+string(state), func(t *testing.T) {
			got, err := ResolveControlIntentOutcome(resolveOutcomeCapturedTurn, state, resolveOutcomeOtherTurn)
			if !errors.Is(err, ErrUnknownEnumValue) {
				t.Fatalf("ResolveControlIntentOutcome(captured state %q, mismatched id): got %v, want ErrUnknownEnumValue", state, err)
			}
			if got != "" {
				t.Fatalf("ResolveControlIntentOutcome(captured state %q, mismatched id): got outcome %v, want zero value on error", state, got)
			}
		})
	}
}

// A blank capturedTurnID is invalid under ControlIntent.Validate() and must
// reject the outcome before an identity comparison (still most recent or
// mismatched) could otherwise resolve a meaningless outcome.
func TestResolveControlIntentOutcome_BlankCapturedTurnIDRejected(t *testing.T) {
	t.Run("still most recent", func(t *testing.T) {
		got, err := ResolveControlIntentOutcome("", TurnStateRunning, "")
		if !errors.Is(err, ErrRequiredValue) {
			t.Fatalf("ResolveControlIntentOutcome(blank captured id, still most recent): got %v, want ErrRequiredValue", err)
		}
		if got != "" {
			t.Fatalf("ResolveControlIntentOutcome(blank captured id, still most recent): got outcome %v, want zero value on error", got)
		}
	})

	t.Run("no longer most recent", func(t *testing.T) {
		got, err := ResolveControlIntentOutcome("", TurnStateRunning, resolveOutcomeOtherTurn)
		if !errors.Is(err, ErrRequiredValue) {
			t.Fatalf("ResolveControlIntentOutcome(blank captured id, mismatched): got %v, want ErrRequiredValue", err)
		}
		if got != "" {
			t.Fatalf("ResolveControlIntentOutcome(blank captured id, mismatched): got outcome %v, want zero value on error", got)
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
