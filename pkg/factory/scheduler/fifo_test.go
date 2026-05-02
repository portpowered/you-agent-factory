package scheduler

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestFIFOScheduler_TwoTransitionsCompetingForSameToken(t *testing.T) {
	sched := NewFIFOScheduler()

	token := interfaces.Token{ID: "tok-1", PlaceID: "p1"}
	enabled := []interfaces.EnabledTransition{
		{
			TransitionID: "tr-1",
			WorkerType:   "worker-a",
			Bindings:     map[string][]interfaces.Token{"input": {token}},
		},
		{
			TransitionID: "tr-2",
			WorkerType:   "worker-b",
			Bindings:     map[string][]interfaces.Token{"input": {token}},
		},
	}

	decisions := sched.Select(enabled, nil)

	if len(decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(decisions))
	}
	if decisions[0].TransitionID != "tr-1" {
		t.Errorf("expected tr-1 (first in order), got %s", decisions[0].TransitionID)
	}
	if decisions[0].WorkerType != "worker-a" {
		t.Errorf("expected worker-a, got %s", decisions[0].WorkerType)
	}
	if len(decisions[0].ConsumeTokens) != 1 || decisions[0].ConsumeTokens[0] != "tok-1" {
		t.Errorf("expected consume [tok-1], got %v", decisions[0].ConsumeTokens)
	}
}

func TestFIFOScheduler_TwoTransitionsIndependentTokens(t *testing.T) {
	sched := NewFIFOScheduler()

	tok1 := interfaces.Token{ID: "tok-1", PlaceID: "p1"}
	tok2 := interfaces.Token{ID: "tok-2", PlaceID: "p2"}
	enabled := []interfaces.EnabledTransition{
		{
			TransitionID: "tr-1",
			WorkerType:   "worker-a",
			Bindings:     map[string][]interfaces.Token{"input": {tok1}},
		},
		{
			TransitionID: "tr-2",
			WorkerType:   "worker-b",
			Bindings:     map[string][]interfaces.Token{"input": {tok2}},
		},
	}

	decisions := sched.Select(enabled, nil)

	if len(decisions) != 2 {
		t.Fatalf("expected 2 decisions, got %d", len(decisions))
	}
	if decisions[0].TransitionID != "tr-1" {
		t.Errorf("first decision should be tr-1, got %s", decisions[0].TransitionID)
	}
	if decisions[1].TransitionID != "tr-2" {
		t.Errorf("second decision should be tr-2, got %s", decisions[1].TransitionID)
	}
}

func TestFIFOScheduler_EmptyEnabled(t *testing.T) {
	sched := NewFIFOScheduler()
	decisions := sched.Select(nil, nil)
	if len(decisions) != 0 {
		t.Fatalf("expected 0 decisions for empty enabled list, got %d", len(decisions))
	}
}

func TestFIFOScheduler_MultipleBindingsPartialConflict(t *testing.T) {
	sched := NewFIFOScheduler()

	sharedTok := interfaces.Token{ID: "shared", PlaceID: "p1"}
	uniqueTok := interfaces.Token{ID: "unique", PlaceID: "p2"}
	otherTok := interfaces.Token{ID: "other", PlaceID: "p3"}

	enabled := []interfaces.EnabledTransition{
		{
			TransitionID: "tr-1",
			WorkerType:   "worker-a",
			Bindings: map[string][]interfaces.Token{
				"code":   {sharedTok},
				"review": {uniqueTok},
			},
		},
		{
			TransitionID: "tr-2",
			WorkerType:   "worker-b",
			Bindings: map[string][]interfaces.Token{
				"input": {sharedTok}, // conflicts with tr-1's "code" binding
			},
		},
		{
			TransitionID: "tr-3",
			WorkerType:   "worker-c",
			Bindings: map[string][]interfaces.Token{
				"input": {otherTok}, // no conflict
			},
		},
	}

	decisions := sched.Select(enabled, nil)

	if len(decisions) != 2 {
		t.Fatalf("expected 2 decisions (tr-1 and tr-3), got %d", len(decisions))
	}
	if decisions[0].TransitionID != "tr-1" {
		t.Errorf("first decision should be tr-1, got %s", decisions[0].TransitionID)
	}
	if decisions[1].TransitionID != "tr-3" {
		t.Errorf("second decision should be tr-3, got %s", decisions[1].TransitionID)
	}
}

func TestFIFOScheduler_CardinalityAllMultipleTokens(t *testing.T) {
	sched := NewFIFOScheduler()

	tok1 := interfaces.Token{ID: "tok-1", PlaceID: "p1"}
	tok2 := interfaces.Token{ID: "tok-2", PlaceID: "p1"}
	tok3 := interfaces.Token{ID: "tok-3", PlaceID: "p2"}

	enabled := []interfaces.EnabledTransition{
		{
			TransitionID: "tr-1",
			WorkerType:   "worker-a",
			Bindings:     map[string][]interfaces.Token{"all-items": {tok1, tok2}},
		},
		{
			TransitionID: "tr-2",
			WorkerType:   "worker-b",
			Bindings:     map[string][]interfaces.Token{"single": {tok1}}, // tok-1 already claimed
		},
		{
			TransitionID: "tr-3",
			WorkerType:   "worker-c",
			Bindings:     map[string][]interfaces.Token{"other": {tok3}}, // no conflict
		},
	}

	decisions := sched.Select(enabled, nil)

	if len(decisions) != 2 {
		t.Fatalf("expected 2 decisions (tr-1 and tr-3), got %d", len(decisions))
	}
	if len(decisions[0].ConsumeTokens) != 2 {
		t.Errorf("tr-1 should consume 2 tokens, got %d", len(decisions[0].ConsumeTokens))
	}
}

func TestFIFOScheduler_CompileTimeInterface(t *testing.T) {
	var _ Scheduler = (*FIFOScheduler)(nil)
}
