package factorysessions

import (
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

func TestProjectRuntime_LifecycleControlStatusReflectsCanonicalProjection(t *testing.T) {
	now := time.Date(2026, 6, 20, 15, 0, 0, 0, time.UTC)
	runtime := ProjectRuntime(ProjectionContext{
		Session: &LiveSession{ID: "~default", IsDefault: true},
		FactoryCfg: &interfaces.FactoryConfig{
			Name: "legacy-petri",
		},
		Snapshot: &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
			RuntimeStatus:          interfaces.RuntimeStatusIdle,
			FactoryState:           "RUNNING",
			LifecycleControlStatus: string(factoryapi.FactorySessionDurableLifecycleStatusPaused),
		},
		Now: now,
	})
	if runtime.LifecycleControlStatus == nil {
		t.Fatal("lifecycleControlStatus = nil, want PAUSED")
	}
	if *runtime.LifecycleControlStatus != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("lifecycleControlStatus = %q, want PAUSED", *runtime.LifecycleControlStatus)
	}
	if runtime.Progress.FactoryState != string(factoryapi.FactorySessionDurableLifecycleStatusPaused) {
		t.Fatalf("progress.factoryState = %q, want PAUSED", runtime.Progress.FactoryState)
	}
}

func TestProjectRuntime_LifecycleControlStatusUnchangedWithoutControlEvents(t *testing.T) {
	now := time.Date(2026, 6, 20, 15, 5, 0, 0, time.UTC)
	runtime := ProjectRuntime(ProjectionContext{
		Session: &LiveSession{ID: "~default", IsDefault: true},
		FactoryCfg: &interfaces.FactoryConfig{
			Name: "legacy-petri",
		},
		Snapshot: &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
			RuntimeStatus:          interfaces.RuntimeStatusIdle,
			FactoryState:           "RUNNING",
			LifecycleControlStatus: string(factoryapi.FactorySessionDurableLifecycleStatusRunning),
		},
		Now: now,
	})
	if runtime.LifecycleControlStatus == nil || *runtime.LifecycleControlStatus != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("lifecycleControlStatus = %#v, want RUNNING", runtime.LifecycleControlStatus)
	}
}
