package config

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

func containsInferenceThrottleGuard(guard petri.Guard) bool {
	switch typed := guard.(type) {
	case *petri.InferenceThrottleGuard:
		return true
	case *petri.AllGuard:
		for _, nested := range typed.Guards {
			if containsInferenceThrottleGuard(nested) {
				return true
			}
		}
	}
	return false
}

func TestConfigMapping_ValidationAllowsCrossTypeFanout(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			},
			{
				Name: "page",
				States: []interfaces.StateConfig{
					{Name: "complete", Type: interfaces.StateTypeTerminal},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			},
			{
				Name: "asset",
				States: []interfaces.StateConfig{
					{Name: "complete", Type: interfaces.StateTypeTerminal},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "fanout",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "page"},
					{StateName: "complete", WorkTypeName: "asset"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	if _, err := mapper.Map(context.Background(), input); err != nil {
		t.Fatalf("expected cross-type fanout to be allowed, got %v", err)
	}
}

func TestConfigMapping_ValidationRejectsUnknownWorkstationKind(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "processor",
				Kind: "unknown_kind",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for unknown workstation kind")
	}
}

func TestConfigMapping_ValidationRejectsNonexistentWorker(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workers: []interfaces.WorkerConfig{
			{Name: "real-worker"},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:           "processor",
				WorkerTypeName: "ghost-worker",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for workstation referencing non-existent worker")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, `references non-existent worker "ghost-worker"`) {
		t.Errorf("unexpected error message:\ngot: %s\nwant it to mention: references non-existent worker \"ghost-worker\"", errMsg)
	}
}

func TestConfigMapping_ValidationAcceptsValidWorkerReference(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Workers: []interfaces.WorkerConfig{
			{Name: "executor"},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:           "processor",
				WorkerTypeName: "executor",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func cronRequiredInputFactoryConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "ready", Type: interfaces.StateTypeProcessing},
				},
			},
		},
		Workers: []interfaces.WorkerConfig{{Name: "cron-worker"}},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:           "daily-refresh",
				Kind:           interfaces.WorkstationKindCron,
				WorkerTypeName: "cron-worker",
				Cron:           &interfaces.CronConfig{Schedule: "*/30 * * * *"},
				Inputs: []interfaces.IOConfig{
					{StateName: "ready", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
			},
		},
	}
}

func configMapperWorkToken(id string, workType string, state string) *interfaces.Token {
	return &interfaces.Token{
		ID:      id,
		PlaceID: fmt.Sprintf("%s:%s", workType, state),
		Color: interfaces.TokenColor{
			WorkID:     id,
			WorkTypeID: workType,
			DataType:   interfaces.DataTypeWork,
		},
	}
}

func configMapperCronTimeToken(id string, workstation string, dueAt time.Time, expiresAt time.Time) *interfaces.Token {
	return &interfaces.Token{
		ID:      id,
		PlaceID: interfaces.SystemTimePendingPlaceID,
		Color: interfaces.TokenColor{
			WorkID:     id,
			WorkTypeID: interfaces.SystemTimeWorkTypeID,
			DataType:   interfaces.DataTypeWork,
			Tags: map[string]string{
				interfaces.TimeWorkTagKeySource:          interfaces.TimeWorkSourceCron,
				interfaces.TimeWorkTagKeyCronWorkstation: workstation,
				interfaces.TimeWorkTagKeyDueAt:           dueAt.UTC().Format(time.RFC3339Nano),
				interfaces.TimeWorkTagKeyExpiresAt:       expiresAt.UTC().Format(time.RFC3339Nano),
			},
		},
	}
}

// --- assertion helpers ---

func assertEquality(t *testing.T, expectedNet *state.Net, outputNet *state.Net) {
	t.Helper()
	assertPlaceEquality(t, expectedNet, outputNet)
	assertTransitionEquality(t, expectedNet, outputNet)
}

func assertNoTransitionExhaustion(t *testing.T, transitions map[string]*petri.Transition) {
	t.Helper()

	for name, transition := range transitions {
		if transition.Type == petri.TransitionExhaustion {
			t.Fatalf("unexpected TransitionExhaustion transition %q in customer-authored mapping", name)
		}
	}
}

func assertGuardedLoopBreakerTransition(t *testing.T, transition *petri.Transition, inputPlace string, outputPlace string, watchedTransition string, maxVisits int) {
	t.Helper()
	if transition == nil {
		t.Fatal("expected guarded loop-breaker transition to exist")
	}
	if transition.Type != petri.TransitionNormal {
		t.Fatalf("guarded loop-breaker type = %s, want %s", transition.Type, petri.TransitionNormal)
	}
	if len(transition.InputArcs) != 1 {
		t.Fatalf("guarded loop-breaker input arcs = %d, want 1", len(transition.InputArcs))
	}
	if transition.InputArcs[0].PlaceID != inputPlace {
		t.Fatalf("guarded loop-breaker input place = %q, want %q", transition.InputArcs[0].PlaceID, inputPlace)
	}
	guard, ok := transition.InputArcs[0].Guard.(*petri.VisitCountGuard)
	if !ok {
		t.Fatalf("expected VisitCountGuard on guarded loop breaker, got %T", transition.InputArcs[0].Guard)
	}
	if guard.TransitionID != watchedTransition {
		t.Fatalf("guarded loop-breaker guard transition = %q, want %s", guard.TransitionID, watchedTransition)
	}
	if guard.MaxVisits != maxVisits {
		t.Fatalf("guarded loop-breaker guard max visits = %d, want %d", guard.MaxVisits, maxVisits)
	}
	if len(transition.OutputArcs) != 1 {
		t.Fatalf("guarded loop-breaker output arcs = %d, want 1", len(transition.OutputArcs))
	}
	if transition.OutputArcs[0].PlaceID != outputPlace {
		t.Fatalf("guarded loop-breaker output place = %q, want %q", transition.OutputArcs[0].PlaceID, outputPlace)
	}
}

func assertPlaceEquality(t *testing.T, expectedNet *state.Net, outputNet *state.Net) {
	t.Helper()
	for placeName, place := range expectedNet.Places {
		outputTarget := outputNet.Places[placeName]
		if outputTarget == nil {
			t.Errorf("failed to find a corresponding place for input place %s", placeName)
			continue
		}
		assertEqualPlaces(t, place, outputTarget)
	}

	for placeName := range outputNet.Places {
		if expectedNet.Places[placeName] == nil {
			t.Errorf("declared place that was not expected %s", placeName)
		}
	}
}

func assertEqualPlaces(t *testing.T, place, outputTarget *petri.Place) {
	t.Helper()
	if place.ID != outputTarget.ID {
		t.Errorf("ids not matching, expected %s, output %s", place.ID, outputTarget.ID)
	}
	if place.State != outputTarget.State {
		t.Errorf("states not matching, expected %q, output %q", place.State, outputTarget.State)
	}
	if place.TypeID != outputTarget.TypeID {
		t.Errorf("types not matching, expected %q, output %q", place.TypeID, outputTarget.TypeID)
	}
}

func assertEqualTransitions(t *testing.T, expected, output *petri.Transition) {
	t.Helper()
	if expected.ID != output.ID {
		t.Errorf("ids not matching, expected %s, output %s", expected.ID, output.ID)
	}
	if expected.Name != output.Name {
		t.Errorf("names not matching, expected %s, output %s", expected.Name, output.Name)
	}
	if expected.WorkerType != output.WorkerType {
		t.Errorf("worker types not matching, expected %s, output %s", expected.WorkerType, output.WorkerType)
	}
	assertArcEquality(t, "input", expected.InputArcs, output.InputArcs)
	assertArcEquality(t, "output", expected.OutputArcs, output.OutputArcs)
	assertArcEquality(t, "rejection", expected.RejectionArcs, output.RejectionArcs)
	assertArcEquality(t, "failure", expected.FailureArcs, output.FailureArcs)
}

func assertArcEquality(t *testing.T, arcType string, expected, output []petri.Arc) {
	t.Helper()
	expecteds := make(map[string]*petri.Arc)
	outputs := make(map[string]*petri.Arc)
	for _, expectedArc := range expected {
		id := getArcID(expectedArc)
		expecteds[id] = &expectedArc
	}
	for _, outputArc := range output {
		id := getArcID(outputArc)
		outputs[id] = &outputArc
	}

	for id, arc := range expecteds {
		outputArc := outputs[id]
		if outputArc == nil {
			t.Errorf("failed to find a corresponding %s arc for %s", arcType, id)
			continue
		}
		assertEqualArcs(t, arc, outputArc)
	}
	for id := range outputs {
		if expecteds[id] == nil {
			t.Errorf("declared %s arc that was not expected %s", arcType, id)
		}
	}
}

func getArcID(arc petri.Arc) string {
	return fmt.Sprintf("%s-%s", arc.PlaceID, arc.TransitionID)
}

func assertEqualArcs(t *testing.T, expected, output *petri.Arc) {
	t.Helper()
	if expected.Name != output.Name {
		t.Errorf("names not matching, expected %s, output %s", expected.Name, output.Name)
	}
	if expected.PlaceID != output.PlaceID {
		t.Errorf("place ids not matching, expected %s, output %s", expected.PlaceID, output.PlaceID)
	}
	if expected.TransitionID != output.TransitionID {
		t.Errorf("transition ids not matching, expected %s, output %s", expected.TransitionID, output.TransitionID)
	}
}

func assertTransitionEquality(t *testing.T, expectedNet *state.Net, outputNet *state.Net) {
	t.Helper()
	for name, expected := range expectedNet.Transitions {
		output := outputNet.Transitions[name]
		if output == nil {
			t.Errorf("failed to find a corresponding transition for %s", name)
			continue
		}
		assertEqualTransitions(t, expected, output)
	}
	for name := range outputNet.Transitions {
		if expectedNet.Transitions[name] == nil {
			t.Errorf("declared transition that was not expected %s", name)
		}
	}
}
