package factory

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestWorkRequestJSONUsesWorkTypeNameContract(t *testing.T) {
	var request interfaces.WorkRequest
	if err := json.Unmarshal([]byte(`{
		"requestId": "request-json",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [
			{"name": "draft", "workTypeName": "task", "state": "queued", "payload": {"title": "Draft"}}
		]
	}`), &request); err != nil {
		t.Fatalf("Unmarshal WorkRequest: %v", err)
	}
	if request.Works[0].WorkTypeID != "task" {
		t.Fatalf("WorkTypeID = %q, want task", request.Works[0].WorkTypeID)
	}
	if request.Works[0].State != "queued" {
		t.Fatalf("State = %q, want queued", request.Works[0].State)
	}
	request.CurrentChainingTraceID = "chain-json"
	request.Works[0].CurrentChainingTraceID = "chain-work-json"

	data, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("Marshal WorkRequest: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal marshaled WorkRequest: %v", err)
	}
	works := raw["works"].([]any)
	work := works[0].(map[string]any)
	if got := work["workTypeName"]; got != "task" {
		t.Fatalf("workTypeName = %#v, want task in %s", got, data)
	}
	if got := work["state"]; got != "queued" {
		t.Fatalf("state = %#v, want queued in %s", got, data)
	}
	if got := raw["currentChainingTraceId"]; got != "chain-json" {
		t.Fatalf("currentChainingTraceId = %#v, want chain-json in %s", got, data)
	}
	if got := work["currentChainingTraceId"]; got != "chain-work-json" {
		t.Fatalf("work currentChainingTraceId = %#v, want chain-work-json in %s", got, data)
	}
	if _, ok := work["work_type_id"]; ok {
		t.Fatalf("marshaled WorkRequest must not expose work_type_id: %s", data)
	}
	if _, ok := work["target_state"]; ok {
		t.Fatalf("marshaled WorkRequest must not expose target_state: %s", data)
	}
}

func TestParseCanonicalWorkRequestJSON_RejectsConflictingCurrentChainingTraceID(t *testing.T) {
	_, err := ParseCanonicalWorkRequestJSON([]byte(`{
		"requestId": "request-json-conflict",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [
			{
				"name": "draft",
				"workTypeName": "task",
				"currentChainingTraceId": "chain-a",
				"traceId": "trace-b"
			}
		]
	}`))
	if err == nil || !strings.Contains(err.Error(), "currentChainingTraceId and traceId must match") {
		t.Fatalf("expected conflicting chaining trace rejection, got %v", err)
	}
}

func TestParseCanonicalWorkRequestJSON_RejectsRequestLevelConflictingCurrentChainingTraceID(t *testing.T) {
	_, err := ParseCanonicalWorkRequestJSON([]byte(`{
		"requestId": "request-json-root-conflict",
		"type": "FACTORY_REQUEST_BATCH",
		"currentChainingTraceId": "chain-a",
		"traceId": "trace-b",
		"works": [
			{
				"name": "draft",
				"workTypeName": "task"
			}
		]
	}`))
	if err == nil || err.Error() != "work request batch currentChainingTraceId and traceId must match when both are provided" {
		t.Fatalf("ParseCanonicalWorkRequestJSON error = %v, want request-level conflict rejection", err)
	}
}

func TestParseCanonicalWorkRequestJSON_RejectsLegacyConflictingCurrentChainingTraceID(t *testing.T) {
	_, err := ParseCanonicalWorkRequestJSON([]byte(`{
		"requestId": "request-json-legacy-conflict",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [
			{
				"name": "draft",
				"workTypeName": "task",
				"current_chaining_trace_id": "chain-a",
				"trace_id": "trace-b"
			}
		]
	}`))
	if err == nil || err.Error() != "work request batch works[0] currentChainingTraceId and traceId must match when both are provided" {
		t.Fatalf("ParseCanonicalWorkRequestJSON error = %v, want legacy conflict rejection", err)
	}
}

func TestParseCanonicalWorkRequestJSON_RejectsRetiredAliases(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		wantErr string
	}{
		{
			name: "top level work type id",
			data: `{
				"requestId": "request-json-top-level-alias",
				"type": "FACTORY_REQUEST_BATCH",
				"work_type_id": "task",
				"works": [{"name": "draft", "workTypeName": "task"}]
			}`,
			wantErr: "work request batch uses retired work_type_id field; use workTypeName",
		},
		{
			name: "nested work type id",
			data: `{
				"requestId": "request-json-work-alias",
				"type": "FACTORY_REQUEST_BATCH",
				"works": [{"name": "draft", "work_type_id": "task"}]
			}`,
			wantErr: "work request batch works[0] uses retired work_type_id field; use workTypeName",
		},
		{
			name: "top level target state",
			data: `{
				"name": "draft",
				"workTypeName": "task",
				"target_state": "queued"
			}`,
			wantErr: "work request batch uses retired target_state field; use state",
		},
		{
			name: "nested target state",
			data: `{
				"requestId": "request-json-target-state-alias",
				"type": "FACTORY_REQUEST_BATCH",
				"works": [{"name": "draft", "workTypeName": "task", "target_state": "queued"}]
			}`,
			wantErr: "work request batch works[0] uses retired target_state field; use state",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseCanonicalWorkRequestJSON([]byte(tc.data))
			if err == nil || err.Error() != tc.wantErr {
				t.Fatalf("ParseCanonicalWorkRequestJSON error = %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestParseCanonicalWorkRequestJSON_AcceptsMatchingCurrentChainingTraceIDAliases(t *testing.T) {
	request, err := ParseCanonicalWorkRequestJSON([]byte(`{
		"requestId": "request-json-match",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [
			{
				"name": "draft",
				"workTypeName": "task",
				"currentChainingTraceId": "chain-a",
				"traceId": "chain-a"
			}
		]
	}`))
	if err != nil {
		t.Fatalf("ParseCanonicalWorkRequestJSON: %v", err)
	}
	if len(request.Works) != 1 {
		t.Fatalf("works = %d, want 1", len(request.Works))
	}
	if request.Works[0].CurrentChainingTraceID != "chain-a" {
		t.Fatalf("current chaining trace ID = %q, want chain-a", request.Works[0].CurrentChainingTraceID)
	}
	if request.Works[0].TraceID != "chain-a" {
		t.Fatalf("trace ID = %q, want chain-a", request.Works[0].TraceID)
	}
}
