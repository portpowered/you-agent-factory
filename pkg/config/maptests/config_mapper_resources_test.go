package maptests

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryresource "github.com/portpowered/infinite-you/pkg/factory/resource"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
)

func TestConfigMapping_ResourceUsage(t *testing.T) {
	mapper := testConfigMapper{}
	outputNet, err := mapper.Map(context.Background(), resourceUsageFactoryConfig())
	if err != nil {
		t.Fatalf("failed to map config: %v", err)
	}
	assertMappedResourcePlace(t, outputNet)
	assertMappedResourceTransition(t, outputNet, "processor")
}

func resourceUsageFactoryConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Resources: []factoryresource.Config{
			{Name: "gpu", Capacity: 2},
		},
		Workers: []workerconfig.Config{
			{Name: "gpu-worker"},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:           "processor",
				WorkerTypeName: "gpu-worker",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
				Resources: []factoryresource.Config{
					{Name: "gpu", Capacity: 1},
				},
			},
		},
	}
}

func TestConfigMapping_TwoWorkstationsSharingResource(t *testing.T) {
	mapper := testConfigMapper{}
	outputNet, err := mapper.Map(context.Background(), twoWorkstationsSharingResourceFactoryConfig())
	if err != nil {
		t.Fatalf("failed to map config: %v", err)
	}
	assertMappedResourcePlace(t, outputNet)
	for _, name := range []string{"step1", "step2"} {
		assertMappedResourceTransition(t, outputNet, name)
	}
}

func twoWorkstationsSharingResourceFactoryConfig() *interfaces.FactoryConfig {
	return &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{
			{
				Name: "task",
				States: []interfaces.StateConfig{
					{Name: "init", Type: interfaces.StateTypeInitial},
					{Name: "processing", Type: interfaces.StateTypeProcessing},
					{Name: "complete", Type: interfaces.StateTypeTerminal},
				},
			},
		},
		Resources: []factoryresource.Config{
			{Name: "gpu", Capacity: 1},
		},
		Workers: []workerconfig.Config{
			{Name: "worker-a"},
			{Name: "worker-b"},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name:           "step1",
				WorkerTypeName: "worker-a",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "processing", WorkTypeName: "task"},
				},
				Resources: []factoryresource.Config{
					{Name: "gpu", Capacity: 1},
				},
			},
			{
				Name:           "step2",
				WorkerTypeName: "worker-b",
				Inputs: []interfaces.IOConfig{
					{StateName: "processing", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
				Resources: []factoryresource.Config{
					{Name: "gpu", Capacity: 1},
				},
			},
		},
	}
}

func TestConfigMapping_ValidationRejectsNonexistentResource(t *testing.T) {
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
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
				Resources: []factoryresource.Config{
					{Name: "nonexistent-gpu", Capacity: 1},
				},
			},
		},
	}

	mapper := testConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for resource_usage referencing non-existent resource")
	}
}

func assertMappedResourcePlace(t *testing.T, outputNet *state.Net) {
	t.Helper()

	resourcePlace := outputNet.Places["gpu:available"]
	if resourcePlace == nil {
		t.Fatal("expected resource place 'gpu:available' to exist")
	}
	if resourcePlace.TypeID != "gpu" {
		t.Errorf("resource place type: expected 'gpu', got %q", resourcePlace.TypeID)
	}
	if resourcePlace.State != "available" {
		t.Errorf("resource place state: expected 'available', got %q", resourcePlace.State)
	}
}

func assertMappedResourceTransition(t *testing.T, outputNet *state.Net, transitionName string) {
	t.Helper()

	tr := outputNet.Transitions[transitionName]
	if tr == nil {
		t.Fatalf("expected transition %q to exist", transitionName)
	}
	if len(tr.InputArcs) != 2 {
		t.Fatalf("expected 2 input arcs, got %d", len(tr.InputArcs))
	}
	consumeArc := tr.InputArcs[1]
	if consumeArc.PlaceID != "gpu:available" {
		t.Errorf("consume arc place: expected 'gpu:available', got %q", consumeArc.PlaceID)
	}
	if consumeArc.Name != "gpu:consume:"+transitionName {
		t.Errorf("consume arc name: expected %q, got %q", "gpu:consume:"+transitionName, consumeArc.Name)
	}
	if consumeArc.Mode != interfaces.ArcModeConsume {
		t.Errorf("consume arc mode: expected CONSUME, got %d", consumeArc.Mode)
	}
	if consumeArc.Cardinality.Mode != petri.CardinalityN || consumeArc.Cardinality.Count != 1 {
		t.Errorf("consume arc cardinality: expected N(1), got %d(%d)", consumeArc.Cardinality.Mode, consumeArc.Cardinality.Count)
	}
	if len(tr.OutputArcs) != 2 {
		t.Fatalf("expected 2 output arcs, got %d", len(tr.OutputArcs))
	}
	releaseArc := tr.OutputArcs[1]
	if releaseArc.PlaceID != "gpu:available" {
		t.Errorf("release arc place: expected 'gpu:available', got %q", releaseArc.PlaceID)
	}
	if releaseArc.Name != "gpu:release:"+transitionName {
		t.Errorf("release arc name: expected %q, got %q", "gpu:release:"+transitionName, releaseArc.Name)
	}
	if releaseArc.Cardinality.Mode != petri.CardinalityN || releaseArc.Cardinality.Count != 1 {
		t.Errorf("release arc cardinality: expected N(1), got %d(%d)", releaseArc.Cardinality.Mode, releaseArc.Cardinality.Count)
	}
}

func TestConfigMapping_ValidationRejectsInvalidResourceCount(t *testing.T) {
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
		Resources: []factoryresource.Config{
			{Name: "gpu", Capacity: 2},
		},
		Workstations: []interfaces.FactoryWorkstationConfig{
			{
				Name: "processor",
				Inputs: []interfaces.IOConfig{
					{StateName: "init", WorkTypeName: "task"},
				},
				Outputs: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
				Resources: []factoryresource.Config{
					{Name: "gpu", Capacity: 0},
				},
			},
		},
	}

	mapper := testConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for resource_usage with zero count")
	}
}

func TestConfigMapping_WorkerResourcesBecomeEnforcedTransitionRequirements(t *testing.T) {
	input := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "complete", Type: interfaces.StateTypeTerminal},
			},
		}},
		Resources: []factoryresource.Config{{Name: "local-model", Capacity: 2}},
		Workers: []workerconfig.Config{{
			Name:      "tts-worker",
			Resources: []factoryresource.Config{{Name: "local-model", Capacity: 1}},
		}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "speak",
			WorkerTypeName: "tts-worker",
			Inputs:         []interfaces.IOConfig{{StateName: "init", WorkTypeName: "task"}},
			Outputs:        []interfaces.IOConfig{{StateName: "complete", WorkTypeName: "task"}},
		}},
	}

	mapper := testConfigMapper{}
	outputNet, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("failed to map config: %v", err)
	}

	tr := outputNet.Transitions["speak"]
	if tr == nil {
		t.Fatal("expected transition 'speak' to exist")
	}
	if len(tr.InputArcs) != 2 {
		t.Fatalf("expected worker resource consume arc to be added, got %d input arcs", len(tr.InputArcs))
	}
	if len(tr.OutputArcs) != 2 {
		t.Fatalf("expected worker resource release arc to be added, got %d output arcs", len(tr.OutputArcs))
	}
	if got := tr.InputArcs[1].PlaceID; got != "local-model:available" {
		t.Fatalf("worker resource consume place = %q, want local-model:available", got)
	}
	if got := tr.InputArcs[1].Cardinality.Count; got != 1 {
		t.Fatalf("worker resource consume count = %d, want 1", got)
	}
}

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

	mapper := testConfigMapper{}
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

	mapper := testConfigMapper{}
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
		Workers: []workerconfig.Config{
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

	mapper := testConfigMapper{}
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
		Workers: []workerconfig.Config{
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

	mapper := testConfigMapper{}
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
					{Name: "complete", Type: interfaces.StateTypeTerminal},
					{Name: "failed", Type: interfaces.StateTypeFailed},
				},
			},
		},
		Workers: []workerconfig.Config{{Name: "cron-worker"}},
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
				OnContinue: []interfaces.IOConfig{
					{StateName: "complete", WorkTypeName: "task"},
				},
			},
		},
	}
}

func configMapperWorkToken(id string, workType string, state string) *factorytoken.Token {
	return &factorytoken.Token{
		ID:      id,
		PlaceID: fmt.Sprintf("%s:%s", workType, state),
		Color: factorytoken.Color{
			WorkID:     id,
			WorkTypeID: workType,
			DataType:   factorytoken.DataTypeWork,
		},
	}
}

func configMapperCronTimeToken(id string, workstation string, dueAt time.Time, expiresAt time.Time) *factorytoken.Token {
	return &factorytoken.Token{
		ID:      id,
		PlaceID: interfaces.SystemTimePendingPlaceID,
		Color: factorytoken.Color{
			WorkID:     id,
			WorkTypeID: interfaces.SystemTimeWorkTypeID,
			DataType:   factorytoken.DataTypeWork,
			Tags: map[string]string{
				interfaces.TimeWorkTagKeySource:          interfaces.TimeWorkSourceCron,
				interfaces.TimeWorkTagKeyCronWorkstation: workstation,
				interfaces.TimeWorkTagKeyDueAt:           dueAt.UTC().Format(time.RFC3339Nano),
				interfaces.TimeWorkTagKeyExpiresAt:       expiresAt.UTC().Format(time.RFC3339Nano),
			},
		},
	}
}

func assertEquality(t *testing.T, expectedNet *state.Net, outputNet *state.Net) {
	t.Helper()
	assertPlaceEquality(t, expectedNet, outputNet)
	assertTransitionEquality(t, expectedNet, outputNet)
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
