package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
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
	result, err := host.StopModelHost(t.Context(), request)
	assertCapacityStopOutcome(t, result, err, models.HostStopStopped)
	if result, err := host.StopModelHost(t.Context(), request); err != nil || result.Outcome != models.HostStopAlreadyStopped || selectedProcess.stops != 1 {
		t.Fatalf("repeated stop = %#v, %v, stops=%d", result, err, selectedProcess.stops)
	}
	if endpoint, err := host.InvocationEndpoint(t.Context(), peer, "llm"); err != nil || endpoint != "peer" || peerProcess.stops != 0 {
		t.Fatalf("peer endpoint=%s err=%v stops=%d", endpoint, err, peerProcess.stops)
	}
}

func assertCapacityStopOutcome(t *testing.T, result models.StopModelHostResult, err error, want models.HostStopOutcome) {
	t.Helper()
	if err != nil || result.Outcome != want {
		t.Fatalf("released stop = %#v, %v, want %s", result, err, want)
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

func TestRuntimeHostAcceptedDependenciesSelectedIdleSchedulerPreservesActivePeer(t *testing.T) {
	t.Parallel()
	selected, _ := (models.RuntimeScopeRef{}).Parse("selected")
	peer, _ := (models.RuntimeScopeRef{}).Parse("peer")
	state := NewSlotState()
	selectedProcess := &schedulerProcess{stopped: make(chan struct{})}
	peerProcess := &schedulerProcess{stopped: make(chan struct{})}
	state.runtimeSlots[runtimeSlotKey(selected, "llm")] = &supervisedRuntime{state: supervisedStateReady, process: selectedProcess}
	state.runtimeSlots[runtimeSlotKey(peer, "llm")] = &supervisedRuntime{state: supervisedStateReady, process: peerProcess}
	clock := &schedulerClock{timer: &schedulerTimer{events: make(chan time.Time, 1)}}
	coordinator := NewSlotCoordinator(state, capacityScopes{}, clock, nil, nil, time.Minute)
	coordinator.OnLeaseCapacityAcquired(selected, "llm")
	coordinator.OnLeaseCapacityAcquired(peer, "llm")
	coordinator.OnLeaseCapacityReleased(selected, "llm")
	if clock.duration != time.Minute || selectedProcess.stops.Load() != 0 {
		t.Fatalf("release scheduled duration=%s stops=%d", clock.duration, selectedProcess.stops.Load())
	}
	clock.timer.events <- clock.Now().Add(time.Minute)
	// The injected timer drives cleanup and the stopped channel observes it.
	// This generous ceiling only bounds a failure on a loaded host.
	select {
	case <-selectedProcess.stopped:
	case <-time.After(30 * time.Second):
		t.Fatal("selected scheduler did not stop its process")
	}
	state.mu.Lock()
	_, selectedRemains := state.runtimeSlots[runtimeSlotKey(selected, "llm")]
	peerRemains := state.runtimeSlots[runtimeSlotKey(peer, "llm")]
	peerHolders := state.capacityHolders[runtimeSlotKey(peer, "llm")]
	state.mu.Unlock()
	if selectedRemains || selectedProcess.stops.Load() != 1 || peerRemains == nil || peerHolders != 1 || peerProcess.stops.Load() != 0 {
		t.Fatal("selected deadline did not unload exactly its idle slot and preserve active peer")
	}
	// No process was launched by this component witness; Stop is its owned effect.
	// Canonical functional scenarios exercise actual lease admission and release.
}

type schedulerClock struct {
	duration time.Duration
	timer    *schedulerTimer
}

func (*schedulerClock) Now() time.Time { return time.Unix(0, 0) }
func (c *schedulerClock) NewTimer(duration time.Duration) modelseffects.HostTimer {
	c.duration = duration
	return c.timer
}

type schedulerTimer struct{ events chan time.Time }

func (t *schedulerTimer) C() <-chan time.Time { return t.events }
func (*schedulerTimer) Stop() bool            { return true }

type schedulerProcess struct {
	stops   atomic.Int32
	stopped chan struct{}
}

func (*schedulerProcess) HealthEndpoint() string { return "controlled" }
func (*schedulerProcess) Wait() error            { panic("no process launched by coordinator witness") }
func (p *schedulerProcess) Stop(context.Context) error {
	if p.stops.Add(1) == 1 {
		close(p.stopped)
	}
	return nil
}
