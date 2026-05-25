package executor

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func assertExecutionMetadataEqual(t *testing.T, want, got interfaces.ExecutionMetadata) {
	t.Helper()
	if want.RequestID != got.RequestID {
		t.Fatalf("RequestID = %q, want %q", got.RequestID, want.RequestID)
	}
	if want.TraceID != got.TraceID {
		t.Fatalf("TraceID = %q, want %q", got.TraceID, want.TraceID)
	}
	if len(want.WorkIDs) != len(got.WorkIDs) {
		t.Fatalf("WorkIDs length = %d, want %d", len(got.WorkIDs), len(want.WorkIDs))
	}
	for i := range want.WorkIDs {
		if want.WorkIDs[i] != got.WorkIDs[i] {
			t.Fatalf("WorkIDs[%d] = %q, want %q", i, got.WorkIDs[i], want.WorkIDs[i])
		}
	}
}
