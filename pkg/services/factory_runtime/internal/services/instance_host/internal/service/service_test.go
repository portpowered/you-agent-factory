package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	instancehost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host"
)

func TestNewConstructsInertHost(t *testing.T) {
	t.Parallel()

	clock := clockwork.NewFakeClock()
	host, err := newHostWithLifecycle(clock, platformclock.Real{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if host == nil {
		t.Fatal("New() = nil, want inert instance host")
	}

	concrete, ok := host.(*Host)
	if !ok {
		t.Fatalf("New() type = %T, want *Host", host)
	}
	if concrete.lifecycle == nil {
		t.Fatal("constructed host has nil lifecycle delegate")
	}
	if concrete.handles == nil {
		t.Fatal("constructed host did not allocate handle state")
	}
	if len(concrete.handles) != 0 {
		t.Fatalf("constructed host handles = %d, want empty inert registry", len(concrete.handles))
	}
}

func newHostWithLifecycle(clock factoryruntime.Clock, scheduler platformclock.TimerSource) (instancehost.Service, error) {
	lifecycle, err := factoryhost.NewLifecycleService(clock, scheduler)
	if err != nil {
		return nil, err
	}
	return New(clock, scheduler, lifecycle)
}

func TestScopeSharesControlsAndPreservesIndependentFactClocks(t *testing.T) {
	t.Parallel()
	host := newTestHost(t)
	times := []time.Time{time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2012, 2, 2, 0, 0, 0, 0, time.UTC)}
	scopes := []instancehost.Service{host.Scope(clockwork.NewFakeClockAt(times[0])), host.Scope(clockwork.NewFakeClockAt(times[1]))}
	runs := make([]factoryruntime.RuntimeRun, 2)
	recordings := []*terminalRecording{{}, {}}
	for i, identity := range []string{"scope-a", "scope-b"} {
		engine := newLifecycleControlFactory(t, interfaces.FactoryStateRunning)
		bundle := testBundle(engine, identity)
		bundle.Recording = recordings[i]
		run, err := scopes[i].Start(context.Background(), bundle)
		if err != nil {
			t.Fatal(err)
		}
		runs[i] = run
		t.Cleanup(func() { _ = scopes[i].Stop(run) })
	}
	// A control through the process authority reaches a handle registered by a
	// scoped invocation; a sibling scope can resume that same physical handle.
	paused, err := host.Pause(context.Background(), runs[0])
	if err != nil || paused.Outcome != factoryruntime.ControlOutcomeAccepted {
		t.Fatalf("Pause = %v, %v", paused, err)
	}
	resumed, err := scopes[1].Resume(context.Background(), runs[0])
	if err != nil || resumed.Outcome != factoryruntime.ControlOutcomeAccepted {
		t.Fatalf("Resume = %v, %v", resumed, err)
	}
	if err := scopes[0].Stop(runs[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := host.Pause(context.Background(), runs[0]); !errors.Is(err, factoryruntime.ErrNotRunning) {
		t.Fatalf("stale Pause = %v", err)
	}
	// Selected cancellation leaves the sibling controllable and its fact clock
	// unchanged. Neither fact clock schedules the process readiness wait.
	paused, err = host.Pause(context.Background(), runs[1])
	if err != nil || paused.Outcome != factoryruntime.ControlOutcomeAccepted {
		t.Fatalf("peer Pause = %v, %v", paused, err)
	}
	if err := scopes[1].Stop(runs[1]); err != nil {
		t.Fatal(err)
	}
	for i, recording := range recordings {
		if recording.finalizeCalls != 1 || !recording.finishedAt.Equal(times[i]) {
			t.Fatalf("scope %d finalization = %d at %s; want once at %s", i, recording.finalizeCalls, recording.finishedAt, times[i])
		}
	}
}
