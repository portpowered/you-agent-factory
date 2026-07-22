package work

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestFactoryRequestBatchPreparationOwnsCanonicalDecodeAndAdmission(t *testing.T) {
	t.Parallel()

	data := []byte(`{"requestId":"batch-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"alpha","workTypeName":"task"}]}`)
	prepared, err := NewFactoryRequestBatchPreparation().PrepareFactoryRequestBatch(context.Background(), data)
	if err != nil {
		t.Fatalf("PrepareFactoryRequestBatch() error = %v", err)
	}
	if prepared.Request.RequestID != "batch-1" || len(prepared.Request.Works) != 1 {
		t.Fatalf("prepared request = %#v", prepared.Request)
	}
	data[0] = '['
	if string(prepared.CanonicalJSON) != `{"requestId":"batch-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"alpha","workTypeName":"task"}]}` {
		t.Fatalf("canonical JSON aliases caller storage: %q", prepared.CanonicalJSON)
	}
}

func TestFactoryRequestBatchPreparationRejectsInvalidCanonicalBatches(t *testing.T) {
	t.Parallel()

	prepare := NewFactoryRequestBatchPreparation()
	tests := []struct {
		name, data, want string
	}{
		{name: "invalid JSON", data: `{not-json`, want: "invalid character"},
		{name: "wrong type", data: `{"requestId":"batch-1","type":"SINGLE_WORK","works":[{"name":"alpha","workTypeName":"task"}]}`, want: `batch type must be "FACTORY_REQUEST_BATCH"`},
		{name: "missing request id", data: `{"requestId":"  ","type":"FACTORY_REQUEST_BATCH","works":[{"name":"alpha","workTypeName":"task"}]}`, want: "batch requestId is required"},
		{name: "empty works", data: `{"requestId":"batch-1","type":"FACTORY_REQUEST_BATCH","works":[]}`, want: "batch works must contain at least one item"},
		{name: "retired alias", data: `{"requestId":"batch-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"alpha","work_type_id":"task"}]}`, want: "works[0].work_type_id is not supported; use workTypeName"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := prepare.PrepareFactoryRequestBatch(context.Background(), []byte(tt.data))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("PrepareFactoryRequestBatch() error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestFactoryRequestBatchPreparationFailsClosedWithoutLiveContext(t *testing.T) {
	t.Parallel()

	prepare := NewFactoryRequestBatchPreparation()
	if _, err := prepare.PrepareFactoryRequestBatch(nil, nil); err == nil || err.Error() != "Factory Request Batch preparation context is required" {
		t.Fatalf("nil-context error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := prepare.PrepareFactoryRequestBatch(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled-context error = %v, want context.Canceled", err)
	}
}

func TestWorkRequestRecordFromSubmitRequests_UsesSharedTraceFallback(t *testing.T) {
	record := WorkRequestRecordFromSubmitRequests("request-record", "api", []SubmitRequest{{
		WorkID:      "work-1",
		WorkTypeID:  "task",
		Name:        "draft",
		TraceID:     "trace-legacy",
		TargetState: "queued",
	}})

	if record.TraceID != "trace-legacy" {
		t.Fatalf("record trace ID = %q, want trace-legacy", record.TraceID)
	}
	if len(record.WorkItems) != 1 {
		t.Fatalf("work item count = %d, want 1", len(record.WorkItems))
	}
	if record.WorkItems[0].CurrentChainingTraceID != "trace-legacy" {
		t.Fatalf("record current chaining trace ID = %q, want trace-legacy", record.WorkItems[0].CurrentChainingTraceID)
	}
	if record.WorkItems[0].TraceID != "trace-legacy" {
		t.Fatalf("record work item trace ID = %q, want trace-legacy", record.WorkItems[0].TraceID)
	}
}

func TestWorkRequestFromSubmitRequests_PreservesCanonicalBatchContract(t *testing.T) {
	requests := []SubmitRequest{
		{
			RequestID:                "request-shared",
			WorkID:                   "work-1",
			Name:                     "draft",
			WorkTypeID:               "task",
			CurrentChainingTraceID:   "chain-current",
			PreviousChainingTraceIDs: []string{"chain-prev"},
			TraceID:                  "trace-legacy",
			Payload:                  []byte(`{"title":"first"}`),
			Tags:                     map[string]string{"scope": "alpha"},
			TargetState:              "queued",
			ExecutionID:              "exec-1",
			Relations:                []Relation{{Type: RelationDependsOn, TargetWorkID: "work-2", RequiredState: "complete"}},
			InvocationArguments:      &InvocationArguments{Arguments: map[string]InvocationArgument{"input": {Values: []string{"first"}}}},
		},
		{
			RequestID:   "request-shared",
			WorkID:      "work-2",
			WorkTypeID:  "task",
			TraceID:     "trace-second",
			Payload:     []byte(`{"title":"second"}`),
			Tags:        map[string]string{"_work_name": "draft"},
			TargetState: "running",
			ExecutionID: "exec-2",
		},
	}

	workRequest := WorkRequestFromSubmitRequests(requests)
	assertCanonicalBatchEnvelope(t, workRequest)
	assertCanonicalFirstWork(t, workRequest.Works[0])
	assertCanonicalSecondWork(t, workRequest.Works[1])

	requests[0].Payload[0] = 'X'
	requests[0].Tags["scope"] = "mutated"
	requests[0].Relations[0].TargetWorkID = "mutated"
	requests[0].InvocationArguments.Arguments["input"] = InvocationArgument{Values: []string{"mutated"}}
	assertCanonicalFirstWorkClones(t, workRequest.Works[0])
	if got := workRequest.Works[0].InvocationArguments.Arguments["input"].Values; !reflect.DeepEqual(got, []string{"first"}) {
		t.Fatalf("invocation arguments = %#v, want detached input argument", got)
	}
}

func assertCanonicalBatchEnvelope(t *testing.T, workRequest WorkRequest) {
	t.Helper()

	if workRequest.Type != WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("work request type = %q, want %q", workRequest.Type, WorkRequestTypeFactoryRequestBatch)
	}
	if workRequest.RequestID != "request-shared" {
		t.Fatalf("work request ID = %q, want request-shared", workRequest.RequestID)
	}
	if workRequest.CurrentChainingTraceID != "chain-current" {
		t.Fatalf("work request current chaining trace ID = %q, want chain-current", workRequest.CurrentChainingTraceID)
	}
	if len(workRequest.Works) != 2 {
		t.Fatalf("work count = %d, want 2", len(workRequest.Works))
	}
}

func assertCanonicalFirstWork(t *testing.T, first Work) {
	t.Helper()

	if first.Name != "draft" {
		t.Fatalf("first work name = %q, want draft", first.Name)
	}
	if first.RequestID != "request-shared" {
		t.Fatalf("first request ID = %q, want request-shared", first.RequestID)
	}
	if first.CurrentChainingTraceID != "chain-current" {
		t.Fatalf("first current chaining trace ID = %q, want chain-current", first.CurrentChainingTraceID)
	}
	if string(first.Payload.([]byte)) != `{"title":"first"}` {
		t.Fatalf("first payload = %s", first.Payload)
	}
	if first.Tags["scope"] != "alpha" {
		t.Fatalf("first tags = %#v, want preserved scope", first.Tags)
	}
	if first.ExecutionID != "exec-1" {
		t.Fatalf("first execution ID = %q, want exec-1", first.ExecutionID)
	}
	if len(first.RuntimeRelations) != 1 || first.RuntimeRelations[0].TargetWorkID != "work-2" {
		t.Fatalf("first runtime relations = %#v", first.RuntimeRelations)
	}
}

func assertCanonicalSecondWork(t *testing.T, second Work) {
	t.Helper()

	if second.Name != "draft-2" {
		t.Fatalf("second work name = %q, want draft-2", second.Name)
	}
	if second.RequestID != "request-shared" {
		t.Fatalf("second request ID = %q, want request-shared", second.RequestID)
	}
	if second.CurrentChainingTraceID != "trace-second" {
		t.Fatalf("second current chaining trace ID = %q, want trace-second", second.CurrentChainingTraceID)
	}
}

func assertCanonicalFirstWorkClones(t *testing.T, first Work) {
	t.Helper()

	if string(first.Payload.([]byte)) != `{"title":"first"}` {
		t.Fatalf("first payload should be cloned, got %s", first.Payload)
	}
	if first.Tags["scope"] != "alpha" {
		t.Fatalf("first tags should be cloned, got %#v", first.Tags)
	}
	if first.RuntimeRelations[0].TargetWorkID != "work-2" {
		t.Fatalf("first runtime relations should be cloned, got %#v", first.RuntimeRelations)
	}
}

func TestWorkRequestFromSubmitRequests_LegacyTraceFallbackAndRequestIDInheritance(t *testing.T) {
	requests := []SubmitRequest{
		{
			RequestID:  "request-shared",
			WorkID:     "work-1",
			Name:       "first",
			WorkTypeID: "task",
			TraceID:    "trace-request-legacy",
		},
		{
			WorkID:     "work-2",
			Name:       "second",
			WorkTypeID: "task",
			TraceID:    "trace-work-legacy",
		},
	}

	workRequest := WorkRequestFromSubmitRequests(requests)
	if workRequest.RequestID != "request-shared" {
		t.Fatalf("work request ID = %q, want request-shared", workRequest.RequestID)
	}
	if workRequest.CurrentChainingTraceID != "trace-request-legacy" {
		t.Fatalf("work request current chaining trace ID = %q, want trace-request-legacy", workRequest.CurrentChainingTraceID)
	}
	if len(workRequest.Works) != 2 {
		t.Fatalf("work count = %d, want 2", len(workRequest.Works))
	}

	first := workRequest.Works[0]
	if first.RequestID != "request-shared" {
		t.Fatalf("first request ID = %q, want request-shared", first.RequestID)
	}
	if first.CurrentChainingTraceID != "trace-request-legacy" {
		t.Fatalf("first current chaining trace ID = %q, want trace-request-legacy", first.CurrentChainingTraceID)
	}

	second := workRequest.Works[1]
	if second.RequestID != "request-shared" {
		t.Fatalf("second request ID = %q, want inherited request-shared", second.RequestID)
	}
	if second.CurrentChainingTraceID != "trace-work-legacy" {
		t.Fatalf("second current chaining trace ID = %q, want trace-work-legacy", second.CurrentChainingTraceID)
	}
}

func TestWorkRequestFromSubmitRequests_EmptyBatchReturnsCanonicalEnvelope(t *testing.T) {
	workRequest := WorkRequestFromSubmitRequests(nil)
	if workRequest.Type != WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("work request type = %q, want %q", workRequest.Type, WorkRequestTypeFactoryRequestBatch)
	}
	if workRequest.RequestID != "" {
		t.Fatalf("work request ID = %q, want empty", workRequest.RequestID)
	}
	if len(workRequest.Works) != 0 {
		t.Fatalf("work count = %d, want 0", len(workRequest.Works))
	}
}

func TestWorkRequestFromSubmitRequests_EmptyMutableInputsNormalizeToNil(t *testing.T) {
	workRequest := WorkRequestFromSubmitRequests([]SubmitRequest{{
		RequestID:  "request-shared",
		WorkID:     "work-1",
		Name:       "draft",
		WorkTypeID: "task",
		Payload:    []byte{},
		Tags:       map[string]string{},
		Relations:  []Relation{},
	}})

	if len(workRequest.Works) != 1 {
		t.Fatalf("work count = %d, want 1", len(workRequest.Works))
	}

	first := workRequest.Works[0]
	payload, ok := first.Payload.([]byte)
	if !ok {
		t.Fatalf("first payload type = %T, want []byte", first.Payload)
	}
	if payload != nil {
		t.Fatalf("first payload = %#v, want nil slice for empty input", payload)
	}
	if first.Tags != nil {
		t.Fatalf("first tags = %#v, want nil map for empty input", first.Tags)
	}
	if first.RuntimeRelations != nil {
		t.Fatalf("first runtime relations = %#v, want nil slice for empty input", first.RuntimeRelations)
	}
}

func TestWorkRequestJSONUsesWorkTypeNameContract(t *testing.T) {
	var request WorkRequest
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
	if err == nil || err.Error() != "currentChainingTraceId and traceId must match when both are provided" {
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
	if err == nil || err.Error() != "works[0].currentChainingTraceId and traceId must match when both are provided" {
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
			wantErr: "work_type_id is not supported; use workTypeName",
		},
		{
			name: "nested work type id",
			data: `{
				"requestId": "request-json-work-alias",
				"type": "FACTORY_REQUEST_BATCH",
				"works": [{"name": "draft", "work_type_id": "task"}]
			}`,
			wantErr: "works[0].work_type_id is not supported; use workTypeName",
		},
		{
			name: "top level target state",
			data: `{
				"name": "draft",
				"workTypeName": "task",
				"target_state": "queued"
			}`,
			wantErr: "target_state is not supported; use state",
		},
		{
			name: "nested target state",
			data: `{
				"requestId": "request-json-target-state-alias",
				"type": "FACTORY_REQUEST_BATCH",
				"works": [{"name": "draft", "workTypeName": "task", "target_state": "queued"}]
			}`,
			wantErr: "works[0].target_state is not supported; use state",
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
