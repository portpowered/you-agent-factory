package service

import (
	"context"
	"errors"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
)

func TestRuntimeHostAcceptedDependenciesCapacityRefusalThenReleasedStopPreservesPeer(t *testing.T) {
	t.Parallel()
	selected, err := (models.RuntimeScopeRef{}).Parse("selected")
	if err != nil {
		t.Fatal(err)
	}
	peer, err := (models.RuntimeScopeRef{}).Parse("peer")
	if err != nil {
		t.Fatal(err)
	}
	selectedKey, peerKey := runtimeSlotKey(selected, "llm"), runtimeSlotKey(peer, "llm")
	selectedProcess, peerProcess := &capacityProcess{endpoint: "selected"}, &capacityProcess{endpoint: "peer"}
	state := NewSlotState()
	state.runtimeSlots[selectedKey] = &supervisedRuntime{state: supervisedStateReady, endpoint: selectedProcess.endpoint, process: selectedProcess}
	state.runtimeSlots[peerKey] = &supervisedRuntime{state: supervisedStateReady, endpoint: peerProcess.endpoint, process: peerProcess}
	state.capacityHolders[selectedKey] = 1
	state.capacityHolders[peerKey] = 1
	host := New(capacityScopes{}, capacityAssets{}, nil, state, nil, nil, nil, nil, nil, models.AssetHostPlatform{}, nil, nil, nil, nil, 0, 0).(*service)
	t.Cleanup(func() {
		if err := host.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	request := models.StopModelHostRequest{Scope: selected, Name: "llm"}
	if result, err := host.StopModelHost(t.Context(), request); !errors.Is(err, models.ErrHostCapacityExhausted) || result.Outcome == models.HostStopStopped {
		t.Fatalf("occupied stop = %#v, %v", result, err)
	}
	if selectedProcess.stops != 0 || peerProcess.stops != 0 {
		t.Fatal("capacity refusal stopped an owned process")
	}
	// Supply the released-holder fact at the component boundary. Lease admission
	// and release are observed separately through the canonical functional graph.
	state.mu.Lock()
	delete(state.capacityHolders, selectedKey)
	state.mu.Unlock()
	if result, err := host.StopModelHost(t.Context(), request); err != nil || result.Outcome != models.HostStopStopped {
		t.Fatalf("released stop = %#v, %v", result, err)
	}
	if result, err := host.StopModelHost(t.Context(), request); err != nil || result.Outcome != models.HostStopAlreadyStopped || selectedProcess.stops != 1 {
		t.Fatalf("repeated stop = %#v, %v, stops=%d", result, err, selectedProcess.stops)
	}
	if endpoint, err := host.InvocationEndpoint(t.Context(), peer, "llm"); err != nil || endpoint != "peer" || peerProcess.stops != 0 {
		t.Fatalf("peer endpoint=%s err=%v stops=%d", endpoint, err, peerProcess.stops)
	}
}

type capacityScopes struct{ runtimescopes.Service }

func (capacityScopes) Resolve(runtimescopes.Reference) (models.RuntimeBinding, error) {
	return models.RuntimeBinding{RuntimeConfig: func() *models.RuntimeConfig { return &models.RuntimeConfig{} }}, nil
}

type capacityAssets struct{ scopedassets.Service }

func (capacityAssets) InspectRuntimeCache(context.Context, models.InspectModelAssetsRequest) (scopedassets.RuntimeCacheInspection, error) {
	return scopedassets.RuntimeCacheInspection{Supported: true, Installed: true}, nil
}

type capacityProcess struct {
	endpoint string
	stops    int
}

func (p *capacityProcess) HealthEndpoint() string     { return p.endpoint }
func (p *capacityProcess) Wait() error                { panic("component did not launch a wait goroutine") }
func (p *capacityProcess) Stop(context.Context) error { p.stops++; return nil }
