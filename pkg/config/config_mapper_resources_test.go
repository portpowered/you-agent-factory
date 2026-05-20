package config

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

// portos:func-length-exception owner=agent-factory reason=legacy-resource-fixture review=2026-07-18 removal=split-resource-config-fixture-before-next-resource-change
func TestConfigMapping_ResourceUsage(t *testing.T) {
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
		Resources: []interfaces.ResourceConfig{
			{Name: "gpu", Capacity: 2},
		},
		Workers: []interfaces.WorkerConfig{
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
				Resources: []interfaces.ResourceConfig{
					{Name: "gpu", Capacity: 1},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	outputNet, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("failed to map config: %v", err)
	}

	// Verify resource place was created with correct ID and state.
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

	// Verify transition has consume and release arcs.
	tr := outputNet.Transitions["processor"]
	if tr == nil {
		t.Fatal("expected transition 'processor' to exist")
	}

	// Should have 2 input arcs: normal input + resource consume.
	if len(tr.InputArcs) != 2 {
		t.Fatalf("expected 2 input arcs, got %d", len(tr.InputArcs))
	}
	consumeArc := tr.InputArcs[1]
	if consumeArc.PlaceID != "gpu:available" {
		t.Errorf("consume arc place: expected 'gpu:available', got %q", consumeArc.PlaceID)
	}
	if consumeArc.Name != "gpu:consume:processor" {
		t.Errorf("consume arc name: expected 'gpu:consume:processor', got %q", consumeArc.Name)
	}
	if consumeArc.Mode != interfaces.ArcModeConsume {
		t.Errorf("consume arc mode: expected CONSUME, got %d", consumeArc.Mode)
	}
	if consumeArc.Cardinality.Mode != petri.CardinalityN || consumeArc.Cardinality.Count != 1 {
		t.Errorf("consume arc cardinality: expected N(1), got %d(%d)", consumeArc.Cardinality.Mode, consumeArc.Cardinality.Count)
	}

	// Should have 2 output arcs: normal output + resource release.
	if len(tr.OutputArcs) != 2 {
		t.Fatalf("expected 2 output arcs, got %d", len(tr.OutputArcs))
	}
	releaseArc := tr.OutputArcs[1]
	if releaseArc.PlaceID != "gpu:available" {
		t.Errorf("release arc place: expected 'gpu:available', got %q", releaseArc.PlaceID)
	}
	if releaseArc.Name != "gpu:release:processor" {
		t.Errorf("release arc name: expected 'gpu:release:processor', got %q", releaseArc.Name)
	}
	if releaseArc.Cardinality.Mode != petri.CardinalityN || releaseArc.Cardinality.Count != 1 {
		t.Errorf("release arc cardinality: expected N(1), got %d(%d)", releaseArc.Cardinality.Mode, releaseArc.Cardinality.Count)
	}
}

// portos:func-length-exception owner=agent-factory reason=legacy-resource-fixture review=2026-07-18 removal=split-shared-resource-fixture-before-next-resource-change
func TestConfigMapping_TwoWorkstationsSharingResource(t *testing.T) {
	input := &interfaces.FactoryConfig{
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
		Resources: []interfaces.ResourceConfig{
			{Name: "gpu", Capacity: 1},
		},
		Workers: []interfaces.WorkerConfig{
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
				Resources: []interfaces.ResourceConfig{
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
				Resources: []interfaces.ResourceConfig{
					{Name: "gpu", Capacity: 1},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	outputNet, err := mapper.Map(context.Background(), input)
	if err != nil {
		t.Fatalf("failed to map config: %v", err)
	}

	// Both transitions should have resource consume and release arcs.
	for _, name := range []string{"step1", "step2"} {
		tr := outputNet.Transitions[name]
		if tr == nil {
			t.Fatalf("expected transition %q to exist", name)
		}
		// 1 normal input + 1 consume = 2 input arcs.
		if len(tr.InputArcs) != 2 {
			t.Errorf("transition %q: expected 2 input arcs, got %d", name, len(tr.InputArcs))
		}
		// 1 normal output + 1 release = 2 output arcs.
		if len(tr.OutputArcs) != 2 {
			t.Errorf("transition %q: expected 2 output arcs, got %d", name, len(tr.OutputArcs))
		}

		// Verify consume arc references the shared resource.
		consumeArc := tr.InputArcs[1]
		if consumeArc.PlaceID != "gpu:available" {
			t.Errorf("transition %q: consume arc place expected 'gpu:available', got %q", name, consumeArc.PlaceID)
		}

		// Verify release arc references the shared resource.
		releaseArc := tr.OutputArcs[1]
		if releaseArc.PlaceID != "gpu:available" {
			t.Errorf("transition %q: release arc place expected 'gpu:available', got %q", name, releaseArc.PlaceID)
		}
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
				Resources: []interfaces.ResourceConfig{
					{Name: "nonexistent-gpu", Capacity: 1},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for resource_usage referencing non-existent resource")
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
		Resources: []interfaces.ResourceConfig{
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
				Resources: []interfaces.ResourceConfig{
					{Name: "gpu", Capacity: 0},
				},
			},
		},
	}

	mapper := ConfigMapper{}
	_, err := mapper.Map(context.Background(), input)
	if err == nil {
		t.Fatal("expected validation error for resource_usage with zero count")
	}
}
