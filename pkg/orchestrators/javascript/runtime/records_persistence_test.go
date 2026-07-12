package workflowruntime

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRuntimeRecordJSON_CheckpointRoundTripPreservesTypedResumeData(t *testing.T) {
	original := RuntimeRecord{
		Sequence: 7,
		Kind:     RecordKindCheckpoint,
		Checkpoint: &CheckpointRecord{
			ID:      "checkpoint-7",
			Label:   "after-plan",
			Summary: "resume after planning",
			State:   map[string]any{"next": "dispatch", "ordinal": float64(3)},
		},
	}

	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal checkpoint record: %v", err)
	}
	var decoded RuntimeRecord
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal checkpoint record: %v", err)
	}
	if decoded.Kind != RecordKindCheckpoint || decoded.Sequence != 7 || decoded.Checkpoint == nil {
		t.Fatalf("decoded record = %#v, want typed checkpoint at sequence 7", decoded)
	}
	if decoded.Checkpoint.ID != "checkpoint-7" || decoded.Checkpoint.State["next"] != "dispatch" {
		t.Fatalf("decoded checkpoint = %#v, want original identity and resume state", decoded.Checkpoint)
	}
}

func TestRuntimeRecordJSON_RejectsUnknownAndMismatchedTypedRecords(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		message string
	}{
		{name: "unknown kind", raw: `{"sequence":1,"kind":"petri_transition","checkpoint":{"id":"checkpoint-1"}}`, message: `unsupported kind "petri_transition"`},
		{name: "missing payload", raw: `{"sequence":1,"kind":"checkpoint"}`, message: "matching payload is required"},
		{name: "foreign payload", raw: `{"sequence":1,"kind":"checkpoint","checkpoint":{"id":"checkpoint-1"},"phase":{"name":"plan"}}`, message: "unexpected phase payload"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var record RuntimeRecord
			err := json.Unmarshal([]byte(test.raw), &record)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("error = %v, want message containing %q", err, test.message)
			}
		})
	}
}
