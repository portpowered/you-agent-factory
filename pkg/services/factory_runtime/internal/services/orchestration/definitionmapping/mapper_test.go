package definitionmapping

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
)

func TestReopenPolicyCompilesToDetachedOpeningState(t *testing.T) {
	t.Parallel()
	mapper, err := New(uuid.NewString)
	if err != nil {
		t.Fatal(err)
	}
	rule := &interfaces.StateReopenConfig{State: "ready", MaxWaits: 3, ExhaustedState: "failed"}
	cfg := &interfaces.FactoryConfig{WorkTypes: []interfaces.WorkTypeConfig{{Name: "mission", States: []interfaces.StateConfig{
		{Name: "ready", Type: interfaces.StateTypeProcessing},
		{Name: "waiting", Type: interfaces.StateTypeProcessing, OnReopen: rule},
		{Name: "failed", Type: interfaces.StateTypeFailed},
	}}}}
	first := mapper.convertToWorkTypes(cfg)["mission"].States[1].OnReopen
	second := mapper.convertToWorkTypes(cfg)["mission"].States[1].OnReopen
	if first == nil || second == nil || *first != *rule || *second != *rule {
		t.Fatalf("compiled rule was lost: first=%#v second=%#v", first, second)
	}
	first.MaxWaits = 1
	if rule.MaxWaits != 3 || second.MaxWaits != 3 {
		t.Fatal("opening mutated authored policy or another opening")
	}
	if mapper.convertToWorkTypes(cfg)["mission"].States[0].OnReopen != nil {
		t.Fatal("unconfigured state acquired a reopen policy")
	}
}

func TestSharedMapperKeepsConcurrentOpeningStateDetached(t *testing.T) {
	t.Parallel()
	mapper, err := New(uuid.NewString)
	if err != nil {
		t.Fatal(err)
	}
	config := &interfaces.FactoryConfig{
		Resources: []interfaces.ResourceConfig{{ID: "gpu", Name: "GPU pool", Capacity: 2}},
	}
	type result struct {
		net *state.Net
		err error
	}
	results := make(chan result, 2)
	for range 2 {
		go func() {
			binding, err := mapper.Bind(t.Context(), config)
			results <- result{net: orchestration.PetriNet(binding), err: err}
		}()
	}
	first, second := <-results, <-results
	for _, mapped := range []result{first, second} {
		if mapped.err != nil {
			t.Fatalf("Bind: %v", mapped.err)
		}
		if mapped.net.Resources["gpu"] == nil || mapped.net.Resources["gpu"].Capacity != 2 {
			t.Fatalf("mapped resources = %#v, want selected GPU capacity", mapped.net.Resources)
		}
	}
	first.net.Resources["gpu"].Capacity = 1
	delete(first.net.Places, "gpu:available")
	if second.net.Resources["gpu"].Capacity != 2 || second.net.Places["gpu:available"] == nil {
		t.Fatal("changing one opening's resource topology changed its peer")
	}
	if config.Resources[0].Capacity != 2 {
		t.Fatal("mapping changed the caller's resource definition")
	}
}

func TestResourceMappingUsesStableIDForMutableDisplayName(t *testing.T) {
	var nextID int
	mapper, err := New(func() string {
		nextID++
		return fmt.Sprintf("arc-%d", nextID)
	})
	if err != nil {
		t.Fatalf("New mapper: %v", err)
	}
	config := &interfaces.FactoryConfig{
		WorkTypes: []interfaces.WorkTypeConfig{{
			Name: "task",
			States: []interfaces.StateConfig{
				{Name: "init", Type: interfaces.StateTypeInitial},
				{Name: "done", Type: interfaces.StateTypeTerminal},
			},
		}},
		Resources: []interfaces.ResourceConfig{{ID: "gpu-1", Name: "GPU pool", Capacity: 2}},
		Workers:   []interfaces.FactoryWorkerConfig{{Name: "worker"}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name: "run", WorkerTypeName: "worker",
			Inputs:    []interfaces.IOConfig{{WorkTypeName: "task", StateName: "init"}},
			Outputs:   []interfaces.IOConfig{{WorkTypeName: "task", StateName: "done"}},
			Resources: []interfaces.ResourceConfig{{Name: "GPU pool", Capacity: 1}},
		}},
	}
	net, err := mapper.Map(context.Background(), config)
	if err != nil {
		t.Fatalf("Map: %v", err)
	}
	if net.Resources["gpu-1"] == nil || net.Resources["gpu-1"].Name != "GPU pool" {
		t.Fatalf("resources = %#v, want stable gpu-1 with display name", net.Resources)
	}
	if _, ok := net.Resources["GPU pool"]; ok {
		t.Fatal("display name was used as the resource map key")
	}
	if net.Places["gpu-1:available"] == nil {
		t.Fatal("stable resource availability place is missing")
	}
	transition := net.Transitions["run"]
	if transition == nil || len(transition.InputArcs) < 2 || transition.InputArcs[1].PlaceID != "gpu-1:available" {
		t.Fatalf("resource input arcs = %#v, want gpu-1:available", transition)
	}
}

func TestCombinedTransitionResourceUsageDeduplicatesWorkerAndWorkstationRequirement(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		Workers: []interfaces.FactoryWorkerConfig{{
			Name: "executor",
			Resources: []interfaces.ResourceConfig{{
				Name: "agent-slot", Capacity: 1,
			}},
		}},
	}
	got := combinedTransitionResourceUsage(cfg, interfaces.FactoryWorkstationConfig{
		WorkerTypeName: "executor",
		Resources: []interfaces.ResourceConfig{{
			Name: "agent-slot", Capacity: 1,
		}},
	})
	if len(got) != 1 || got[0].Name != "agent-slot" || got[0].Capacity != 1 {
		t.Fatalf("combined resources = %#v, want one aligned agent-slot requirement", got)
	}
}

func TestCombinedTransitionResourceUsageUsesStricterRepeatedRequirement(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		Workers: []interfaces.FactoryWorkerConfig{{
			Name: "executor",
			Resources: []interfaces.ResourceConfig{{
				Name: "gpu", Capacity: 1,
			}},
		}},
	}
	got := combinedTransitionResourceUsage(cfg, interfaces.FactoryWorkstationConfig{
		WorkerTypeName: "executor",
		Resources: []interfaces.ResourceConfig{{
			Name: "gpu", Capacity: 2,
		}},
	})
	if len(got) != 1 || got[0].Capacity != 2 {
		t.Fatalf("combined resources = %#v, want stricter capacity 2", got)
	}
}
