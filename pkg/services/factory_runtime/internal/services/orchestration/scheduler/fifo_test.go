package scheduler

import (
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestFIFOScheduler_TwoTransitionsCompetingForSameToken(t *testing.T) {
	sched := NewFIFOScheduler()

	token := workerexecution.Token{ID: "tok-1"}
	enabled := []interfaces.EnabledTransition{
		{
			TransitionID: "tr-1",
			WorkerType:   "worker-a",
			Bindings:     map[string][]workerexecution.Token{"input": {token}},
		},
		{
			TransitionID: "tr-2",
			WorkerType:   "worker-b",
			Bindings:     map[string][]workerexecution.Token{"input": {token}},
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

	tok1 := workerexecution.Token{ID: "tok-1"}
	tok2 := workerexecution.Token{ID: "tok-2"}
	enabled := []interfaces.EnabledTransition{
		{
			TransitionID: "tr-1",
			WorkerType:   "worker-a",
			Bindings:     map[string][]workerexecution.Token{"input": {tok1}},
		},
		{
			TransitionID: "tr-2",
			WorkerType:   "worker-b",
			Bindings:     map[string][]workerexecution.Token{"input": {tok2}},
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

	sharedTok := workerexecution.Token{ID: "shared"}
	uniqueTok := workerexecution.Token{ID: "unique"}
	otherTok := workerexecution.Token{ID: "other"}

	enabled := []interfaces.EnabledTransition{
		{
			TransitionID: "tr-1",
			WorkerType:   "worker-a",
			Bindings: map[string][]workerexecution.Token{
				"code":   {sharedTok},
				"review": {uniqueTok},
			},
		},
		{
			TransitionID: "tr-2",
			WorkerType:   "worker-b",
			Bindings: map[string][]workerexecution.Token{
				"input": {sharedTok}, // conflicts with tr-1's "code" binding
			},
		},
		{
			TransitionID: "tr-3",
			WorkerType:   "worker-c",
			Bindings: map[string][]workerexecution.Token{
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

	tok1 := workerexecution.Token{ID: "tok-1"}
	tok2 := workerexecution.Token{ID: "tok-2"}
	tok3 := workerexecution.Token{ID: "tok-3"}

	enabled := []interfaces.EnabledTransition{
		{
			TransitionID: "tr-1",
			WorkerType:   "worker-a",
			Bindings:     map[string][]workerexecution.Token{"all-items": {tok1, tok2}},
		},
		{
			TransitionID: "tr-2",
			WorkerType:   "worker-b",
			Bindings:     map[string][]workerexecution.Token{"single": {tok1}}, // tok-1 already claimed
		},
		{
			TransitionID: "tr-3",
			WorkerType:   "worker-c",
			Bindings:     map[string][]workerexecution.Token{"other": {tok3}}, // no conflict
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

func TestFIFOScheduler_IncludesObservedBindingsWithoutConsumingThem(t *testing.T) {
	sched := NewFIFOScheduler()
	parent := workerexecution.Token{ID: "parent", State: "waiting"}
	childA := workerexecution.Token{ID: "child-a", State: "complete"}
	childB := workerexecution.Token{ID: "child-b", State: "complete"}

	decisions := sched.Select([]interfaces.EnabledTransition{{
		TransitionID: "merge",
		WorkerType:   "merger",
		Bindings: map[string][]workerexecution.Token{
			"parent":   {parent},
			"children": {childA, childB},
		},
		ArcModes: map[string]interfaces.ArcMode{"children": interfaces.ArcModeObserve},
	}}, nil)

	if len(decisions) != 1 {
		t.Fatalf("decisions = %d, want 1", len(decisions))
	}
	decision := decisions[0]
	if strings.Join(decision.InputTokens, ",") != "child-a,child-b,parent" {
		t.Fatalf("input tokens = %v, want observed children and consumed parent", decision.InputTokens)
	}
	if strings.Join(decision.ConsumeTokens, ",") != "parent" {
		t.Fatalf("consume tokens = %v, want only parent", decision.ConsumeTokens)
	}
	if strings.Join(decision.InputBindings["children"], ",") != "child-a,child-b" {
		t.Fatalf("child input bindings = %v, want both observed children", decision.InputBindings["children"])
	}
}

func TestFIFOScheduler_CompileTimeInterface(t *testing.T) {
	var _ Scheduler = (*FIFOScheduler)(nil)
}
