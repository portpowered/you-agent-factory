package factory

import (
	"encoding/json"
	"testing"

	"github.com/portpowered/agent-factory/pkg/interfaces"
)

func TestWorkstationResponsePayload_SerializesProjectionFields(t *testing.T) {
	payload := marshalPayloadObject(t, interfaces.WorkstationResponsePayload{
		DispatchID:     "dispatch-1",
		TransitionID:   "transition-1",
		Workstation:    interfaces.FactoryWorkstationRef{ID: "transition-1", Name: "review"},
		Result:         interfaces.WorkstationResult{Outcome: "ACCEPTED", Output: "done"},
		DurationMillis: 1500,
		Outputs: []interfaces.WorkstationOutput{{
			Type:      "MOVE",
			TokenID:   "token-1",
			FromPlace: "story:init",
			ToPlace:   "story:complete",
			WorkItem: &interfaces.FactoryWorkItem{
				ID:          "work-1",
				WorkTypeID:  "story",
				DisplayName: "Implement event stream",
				TraceID:     "trace-1",
			},
		}},
		TraceData:       &interfaces.FactoryTraceData{TraceID: "trace-1", WorkIDs: []string{"work-1"}},
		ProviderSession: &interfaces.ProviderSessionMetadata{Provider: "codex", Kind: "response_id", ID: "resp-1"},
		TerminalWork: &interfaces.FactoryTerminalWork{
			WorkItem: interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "story"},
			Status:   "completed",
		},
	})
	assertJSONField(t, payload, "dispatch_id", "dispatch-1")
	assertJSONField(t, payload, "transition_id", "transition-1")
	assertJSONField(t, payload, "duration_millis", float64(1500))
	assertJSONObject(t, payload, "workstation")
	assertJSONObject(t, payload, "result")
	assertJSONArray(t, payload, "outputs")
	assertJSONObject(t, payload, "trace_data")
	assertJSONObject(t, payload, "provider_session")
	assertJSONObject(t, payload, "terminal_work")
}

func TestInitialStructurePayload_SerializesContractFields(t *testing.T) {
	payload := interfaces.InitialStructurePayload{
		Resources:    []interfaces.FactoryResource{{ID: "agent-slots", Name: "Agent slots", Capacity: 2}},
		Constraints:  []interfaces.FactoryConstraint{{ID: "max-visits", Type: "global_limit", Values: map[string]string{"max_total_visits": "3"}}},
		Workers:      []interfaces.FactoryWorker{{ID: "executor", Provider: "codex-cli", ModelProvider: "codex", Model: "gpt-5.4"}},
		WorkTypes:    []interfaces.FactoryWorkType{{ID: "story", States: []interfaces.FactoryStateDefinition{{Value: "init", Category: "INITIAL"}}}},
		Workstations: []interfaces.FactoryWorkstation{{ID: "execute", Name: "execute-story", WorkerID: "executor"}},
		Places:       []interfaces.FactoryPlace{{ID: "story:init", TypeID: "story", State: "init", Category: "INITIAL"}},
		Relations:    []interfaces.FactoryRelation{{Type: "DEPENDS_ON", SourceWorkID: "work-2", TargetWorkID: "work-1", RequiredState: "complete"}},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal InitialStructurePayload: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal InitialStructurePayload: %v", err)
	}

	for _, field := range []string{"resources", "constraints", "workers", "work_types", "workstations", "places", "relations"} {
		assertJSONArray(t, decoded, field)
	}
}

func TestWorkInputAndStateChangePayloads_SerializeContractFields(t *testing.T) {
	inputPayload := marshalPayloadObject(t, interfaces.WorkInputPayload{
		TokenID:   "tok-story-1",
		WorkItem:  interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "story", DisplayName: "Story 1", TraceID: "trace-1"},
		Relations: []interfaces.FactoryRelation{{Type: "DEPENDS_ON", TargetWorkID: "work-0", RequiredState: "complete"}},
	})
	assertJSONField(t, inputPayload, "token_id", "tok-story-1")
	assertJSONObject(t, inputPayload, "work_item")
	assertJSONArray(t, inputPayload, "relations")

	statePayload := marshalPayloadObject(t, interfaces.FactoryStateChangePayload{
		PreviousState: "IDLE",
		State:         "ACTIVE",
		Reason:        "work submitted",
	})
	assertJSONField(t, statePayload, "previous_state", "IDLE")
	assertJSONField(t, statePayload, "state", "ACTIVE")
	assertJSONField(t, statePayload, "reason", "work submitted")
}

func TestWorkRequestAndRelationshipPayloads_SerializeContractFields(t *testing.T) {
	requestPayload := marshalPayloadObject(t, interfaces.WorkRequestPayload{
		RequestID: "request-1",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		TraceID:   "trace-1",
		Source:    "external-submit",
		RelationContext: []interfaces.WorkRelation{{
			Type:           interfaces.WorkRelationDependsOn,
			SourceWorkName: "story-2",
			TargetWorkName: "story-1",
		}},
		ParentLineage: []string{"request-parent", "work-parent"},
		WorkItems:     []interfaces.FactoryWorkItem{{ID: "work-1", WorkTypeID: "story", DisplayName: "Story 1", TraceID: "trace-1"}},
	})
	assertJSONField(t, requestPayload, "request_id", "request-1")
	assertJSONField(t, requestPayload, "type", "FACTORY_REQUEST_BATCH")
	assertJSONField(t, requestPayload, "trace_id", "trace-1")
	assertJSONField(t, requestPayload, "source", "external-submit")
	assertJSONArray(t, requestPayload, "relation_context")
	assertJSONArray(t, requestPayload, "parent_lineage")
	assertJSONArray(t, requestPayload, "work_items")

	relationshipPayload := marshalPayloadObject(t, interfaces.RelationshipChangePayload{
		RequestID: "request-1",
		TraceID:   "trace-1",
		Relation: interfaces.FactoryRelation{
			Type:           "DEPENDS_ON",
			SourceWorkID:   "work-2",
			SourceWorkName: "second",
			TargetWorkID:   "work-1",
			TargetWorkName: "first",
			RequiredState:  "complete",
			RequestID:      "request-1",
			TraceID:        "trace-1",
		},
	})
	assertJSONField(t, relationshipPayload, "request_id", "request-1")
	assertJSONField(t, relationshipPayload, "trace_id", "trace-1")
	relation := assertJSONObject(t, relationshipPayload, "relation")
	assertJSONField(t, relation, "type", "DEPENDS_ON")
	assertJSONField(t, relation, "sourceWorkId", "work-2")
	assertJSONField(t, relation, "sourceWorkName", "second")
	assertJSONField(t, relation, "targetWorkId", "work-1")
	assertJSONField(t, relation, "targetWorkName", "first")
	assertJSONField(t, relation, "requiredState", "complete")
}

func marshalPayloadObject(t *testing.T, payload any) map[string]any {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	return decoded
}

func assertJSONField(t *testing.T, object map[string]any, field string, want any) {
	t.Helper()
	got, ok := object[field]
	if !ok {
		t.Fatalf("missing JSON field %q in %#v", field, object)
	}
	if got != want {
		t.Fatalf("JSON field %q = %#v, want %#v", field, got, want)
	}
}

func assertJSONObject(t *testing.T, object map[string]any, field string) map[string]any {
	t.Helper()
	got, ok := object[field]
	if !ok {
		t.Fatalf("missing JSON object field %q in %#v", field, object)
	}
	value, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("JSON field %q = %#v, want object", field, got)
	}
	return value
}

func assertJSONArray(t *testing.T, object map[string]any, field string) []any {
	t.Helper()
	got, ok := object[field]
	if !ok {
		t.Fatalf("missing JSON array field %q in %#v", field, object)
	}
	value, ok := got.([]any)
	if !ok {
		t.Fatalf("JSON field %q = %#v, want array", field, got)
	}
	return value
}
