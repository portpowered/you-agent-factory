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
		ArtifactVerification: &workers.ExpectedArtifactVerification{
			Code: workers.WorkFailureTypeExpectedArtifactsUnsatisfied,
			Entries: []workers.ExpectedArtifactVerificationEntry{{
				Name:    "report",
				Pattern: "reports/*.json",
				Reason:  workers.ExpectedArtifactVerificationReasonEmpty,
			}},
		},
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

func TestWorkstationResult_PreservesStructuredResultPresenceAndNativeShape(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   any
		present bool
		wantKey bool
	}{
		{
			name:    "object",
			value:   map[string]any{"items": []any{json.Number("1"), "two"}},
			present: true,
			wantKey: true,
		},
		{name: "null", value: nil, present: true, wantKey: true},
		{name: "absent", value: nil, present: false, wantKey: false},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			original := workers.WorkstationResult{
				Outcome:                 string(workers.OutcomeAccepted),
				StructuredResult:        test.value,
				StructuredResultPresent: test.present,
			}
			encoded, err := json.Marshal(original)
			if err != nil {
				t.Fatalf("marshal workstation result: %v", err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &fields); err != nil {
				t.Fatalf("decode workstation result: %v", err)
			}
			_, gotKey := fields["structuredResult"]
			if gotKey != test.wantKey {
				t.Fatalf("structuredResult field present = %t in %s, want %t", gotKey, encoded, test.wantKey)
			}

			var roundTripped workers.WorkstationResult
			if err := json.Unmarshal(encoded, &roundTripped); err != nil {
				t.Fatalf("unmarshal workstation result: %v", err)
			}
			if roundTripped.StructuredResultPresent != test.present || !reflect.DeepEqual(roundTripped.StructuredResult, test.value) {
				t.Fatalf("round trip structured result = %#v (present=%t), want %#v (present=%t)", roundTripped.StructuredResult, roundTripped.StructuredResultPresent, test.value, test.present)
			}
		})
	}
}

func TestColorCheckpointPreservesSnakeCaseStructuredResult(t *testing.T) {
	t.Parallel()

	original := workers.Color{
		WorkID:                  "work-1",
		StructuredResult:        map[string]any{"ready": true},
		StructuredResultPresent: true,
	}
	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal token color: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("decode token color: %v", err)
	}
	if _, ok := fields["structured_result"]; !ok {
		t.Fatalf("structured_result omitted from %s", encoded)
	}
	if _, wrongKey := fields["structuredResult"]; wrongKey {
		t.Fatalf("public structuredResult key leaked into token checkpoint: %s", encoded)
	}

	var roundTripped workers.Color
	if err := json.Unmarshal(encoded, &roundTripped); err != nil {
		t.Fatalf("unmarshal token color: %v", err)
	}
	if !roundTripped.StructuredResultPresent || !reflect.DeepEqual(roundTripped.StructuredResult, original.StructuredResult) {
		t.Fatalf("round trip color = %#v, want structured result %#v", roundTripped, original.StructuredResult)
	}
}
