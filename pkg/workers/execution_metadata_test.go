package workers

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func assertExecutionMetadataEqual(t *testing.T, want, got interfaces.ExecutionMetadata) {
	t.Helper()
	if got.DispatchCreatedTick != want.DispatchCreatedTick {
		t.Fatalf("DispatchCreatedTick = %d, want %d", got.DispatchCreatedTick, want.DispatchCreatedTick)
	}
	if got.CurrentTick != want.CurrentTick {
		t.Fatalf("CurrentTick = %d, want %d", got.CurrentTick, want.CurrentTick)
	}
	if got.RequestID != want.RequestID {
		t.Fatalf("RequestID = %q, want %q", got.RequestID, want.RequestID)
	}
	if got.TraceID != want.TraceID {
		t.Fatalf("TraceID = %q, want %q", got.TraceID, want.TraceID)
	}
	if got.ReplayKey != want.ReplayKey {
		t.Fatalf("ReplayKey = %q, want %q", got.ReplayKey, want.ReplayKey)
	}
	if len(got.WorkIDs) != len(want.WorkIDs) {
		t.Fatalf("WorkIDs length = %d, want %d: %#v", len(got.WorkIDs), len(want.WorkIDs), got.WorkIDs)
	}
	for i := range want.WorkIDs {
		if got.WorkIDs[i] != want.WorkIDs[i] {
			t.Fatalf("WorkIDs[%d] = %q, want %q; full WorkIDs: %#v", i, got.WorkIDs[i], want.WorkIDs[i], got.WorkIDs)
		}
	}
}
