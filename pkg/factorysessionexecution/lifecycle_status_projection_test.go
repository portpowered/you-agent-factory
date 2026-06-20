package factorysessionexecution

import "testing"

func TestProjectedLifecycleControlStatus_PrefersCanonicalBracketStatus(t *testing.T) {
	status := ProjectedLifecycleControlStatus("PAUSED", "RUNNING")
	if status != LifecycleStatusPaused {
		t.Fatalf("status = %q, want PAUSED", status)
	}
}

func TestProjectedLifecycleControlStatus_FallsBackToFactoryRuntimeState(t *testing.T) {
	if got := ProjectedLifecycleControlStatus("", "PAUSED"); got != LifecycleStatusPaused {
		t.Fatalf("status = %q, want PAUSED", got)
	}
	if got := ProjectedLifecycleControlStatus("", "RUNNING"); got != LifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", got)
	}
}
