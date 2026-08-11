package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
)

func TestResourceCapacityDecisionMatrix(t *testing.T) {
	tests := []struct {
		name        string
		current     int
		requested   int
		inUse       int
		wantErr     error
		wantOutcome factory.ResourceCapacityOutcome
		wantAvail   int
	}{
		{name: "increase", current: 2, requested: 4, wantOutcome: factory.ResourceCapacityOutcomeApplied, wantAvail: 4},
		{name: "equal is no-op", current: 2, requested: 2, wantOutcome: factory.ResourceCapacityOutcomeNoOp, wantAvail: 2},
		{name: "decrease removes idle units", current: 4, requested: 2, wantOutcome: factory.ResourceCapacityOutcomeApplied, wantAvail: 2},
		{name: "negative rejected", current: 2, requested: -1, wantErr: factory.ErrResourceCapacityValidation, wantAvail: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			eng := newResourceCapacityTestEngine(test.current)
			if test.inUse > 0 {
				markResourceUnitsInUse(eng, test.inUse)
			}
			before := eng.GetMarking()
			result, err := eng.SetResourceCapacity(context.Background(), factory.ResourceCapacityRequest{
				ResourceID: "gpu-slot", RequestedCapacity: test.requested,
			})
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("SetResourceCapacity error = %v, want %v", err, test.wantErr)
				}
				beforeMarking := before
				afterMarking := eng.GetMarking()
				if got := len(afterMarking.TokensInPlace("gpu-slot:available")); got != len(beforeMarking.TokensInPlace("gpu-slot:available")) {
					t.Fatalf("available token count after rejection = %d, want %d", got, len(beforeMarking.TokensInPlace("gpu-slot:available")))
				}
				return
			}
			if err != nil {
				t.Fatalf("SetResourceCapacity: %v", err)
			}
			if result.Outcome != test.wantOutcome || result.AvailableCount != test.wantAvail {
				t.Fatalf("result = %#v, want outcome %s and available %d", result, test.wantOutcome, test.wantAvail)
			}
			if got := eng.state.Resources["gpu-slot"].Capacity; got != test.requested {
				t.Fatalf("effective capacity = %d, want %d", got, test.requested)
			}
		})
	}
}

func TestResourceCapacityRejectsReductionBelowInUseWithoutMutation(t *testing.T) {
	eng := newResourceCapacityTestEngine(3)
	markResourceUnitsInUse(eng, 2)
	before := eng.GetRuntimeStateSnapshot()

	result, err := eng.SetResourceCapacity(context.Background(), factory.ResourceCapacityRequest{
		ResourceID: "gpu-slot", RequestedCapacity: 1,
	})
	if !errors.Is(err, factory.ErrResourceCapacityInUse) {
		t.Fatalf("SetResourceCapacity error = %v, want ErrResourceCapacityInUse", err)
	}
	if result.EffectiveCapacity != 3 {
		t.Fatalf("rejected result effective capacity = %d, want 3", result.EffectiveCapacity)
	}
	after := eng.GetRuntimeStateSnapshot()
	if eng.state.Resources["gpu-slot"].Capacity != 3 || len(after.Marking.Tokens) != len(before.Marking.Tokens) {
		t.Fatalf("runtime changed after capacity-in-use rejection: before=%d after=%d capacity=%d", len(before.Marking.Tokens), len(after.Marking.Tokens), eng.state.Resources["gpu-slot"].Capacity)
	}
}

func TestResourceCapacityZeroBlocksUntilRaised(t *testing.T) {
	eng := newResourceCapacityTestEngine(2)
	if _, err := eng.SetResourceCapacity(context.Background(), factory.ResourceCapacityRequest{ResourceID: "gpu-slot", RequestedCapacity: 0}); err != nil {
		t.Fatalf("set zero capacity: %v", err)
	}
	marking := eng.GetMarking()
	if got := len(marking.TokensInPlace("gpu-slot:available")); got != 0 {
		t.Fatalf("available tokens at zero capacity = %d, want 0", got)
	}
	if _, err := eng.SetResourceCapacity(context.Background(), factory.ResourceCapacityRequest{ResourceID: "gpu-slot", RequestedCapacity: 1}); err != nil {
		t.Fatalf("raise zero capacity: %v", err)
	}
	marking = eng.GetMarking()
	if got := len(marking.TokensInPlace("gpu-slot:available")); got != 1 {
		t.Fatalf("available tokens after raise = %d, want 1", got)
	}
}

func TestResourceCapacityAdmissionBlocksUntilReleased(t *testing.T) {
	eng := newResourceCapacityTestEngine(1)
	release, err := eng.AcquireResourceCapacityAdmission(context.Background())
	if err != nil {
		t.Fatalf("acquire admission: %v", err)
	}
	defer release()

	done := make(chan error, 1)
	go func() {
		_, err := eng.SetResourceCapacity(context.Background(), factory.ResourceCapacityRequest{ResourceID: "gpu-slot", RequestedCapacity: 2})
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("capacity mutation completed before release: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	release()
	if err := <-done; err != nil {
		t.Fatalf("capacity mutation after release: %v", err)
	}
}

func newResourceCapacityTestEngine(capacity int) *FactoryEngine {
	net := buildTestNet()
	net.Resources = map[string]*state.ResourceDef{
		"gpu-slot": {ID: "gpu-slot", Name: "GPU slot", Capacity: capacity},
	}
	place, tokens := state.GenerateResourcePlaces(net.Resources["gpu-slot"], time.Unix(0, 0))
	net.Places[place.ID] = place
	marking := petri.NewMarking(net.ID)
	for _, token := range tokens {
		marking.AddToken(token)
	}
	return newTestFactoryEngine(net, marking, nil)
}

func markResourceUnitsInUse(eng *FactoryEngine, count int) {
	marking := eng.GetMarking()
	available := marking.TokensInPlace("gpu-slot:available")
	if count > len(available) {
		count = len(available)
	}
	eng.mu.Lock()
	defer eng.mu.Unlock()
	for _, token := range available[:count] {
		eng.runtimeState.Marking.RemoveToken(token.ID)
	}
	consumed := make([]factorytoken.Token, 0, count)
	for _, token := range available[:count] {
		token.PlaceID = "gpu-slot:held"
		consumed = append(consumed, token)
	}
	eng.runtimeState.Dispatches["dispatch-resource"] = &interfaces.DispatchEntry{ConsumedTokens: consumed}
}
