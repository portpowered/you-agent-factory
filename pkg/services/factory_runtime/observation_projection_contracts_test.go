package factory

import (
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestDashboardEngineStateSnapshotPublishesRuntimeRootVocabulary(t *testing.T) {
	t.Parallel()

	uptime := 11 * time.Second
	snapshot := DashboardEngineStateSnapshot(
		"RUNNING",
		interfaces.RuntimeStatusActive,
		11,
		uptime,
	)
	if snapshot.FactoryState != "RUNNING" {
		t.Fatalf("factory state = %q, want RUNNING", snapshot.FactoryState)
	}
	if snapshot.RuntimeStatus != interfaces.RuntimeStatusActive {
		t.Fatalf("runtime status = %q, want ACTIVE", snapshot.RuntimeStatus)
	}
	if snapshot.TickCount != 11 {
		t.Fatalf("tick count = %d, want 11", snapshot.TickCount)
	}
	if snapshot.Uptime != uptime {
		t.Fatalf("uptime = %v, want %v", snapshot.Uptime, uptime)
	}
}
