package workers_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestWorkstationResult_RoundTripsDetachedExecutionFields(t *testing.T) {
	t.Parallel()

	original := workers.WorkstationResult{
		Outcome:                     string(workers.OutcomeRejected),
		Output:                      "partial",
		Feedback:                    "needs revision",
		SelectedClassificationLabel: "blocked",
		FailureDetail: &workers.FailureDetail{
			Reason:  workers.WorkFailureTypeThrottled,
			Message: "provider unavailable",
		},
		FailureMetadata: &workers.WorkFailureMetadata{
			Family: workers.WorkFailureFamilyThrottle,
			Type:   workers.WorkFailureTypeThrottled,
		},
	}

	cloned := workers.CloneWorkstationResult(original)
	if !reflect.DeepEqual(cloned, original) {
		t.Fatalf("clone = %#v, want %#v", cloned, original)
	}

	encoded, err := json.Marshal(cloned)
	if err != nil {
		t.Fatalf("marshal workstation result: %v", err)
	}
	var roundTripped workers.WorkstationResult
	if err := json.Unmarshal(encoded, &roundTripped); err != nil {
		t.Fatalf("unmarshal workstation result: %v", err)
	}
	if roundTripped.Outcome != original.Outcome ||
		roundTripped.FailureDetail == nil ||
		roundTripped.FailureDetail.Reason != workers.WorkFailureTypeThrottled {
		t.Fatalf("round trip = %#v, want outcome and failure detail from %#v", roundTripped, original)
	}
}
