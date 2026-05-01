package factory

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/portpowered/agent-factory/pkg/interfaces"
)

func TestNormalizeWorkRequest_IndependentWorkItemsShareRequestAndTrace(t *testing.T) {
	request := interfaces.WorkRequest{
		RequestID:              "request-1",
		CurrentChainingTraceID: "chain-request-1",
		Type:                   interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{
			{Name: "first", WorkTypeID: "task", Payload: map[string]any{"title": "first"}},
			{Name: "second", WorkTypeID: "task", Payload: map[string]any{"title": "second"}},
		},
	}

	normalized, err := NormalizeWorkRequest(request, interfaces.WorkRequestNormalizeOptions{
		ValidWorkTypes: map[string]bool{"task": true},
	})
	if err != nil {
		t.Fatalf("NormalizeWorkRequest: %v", err)
	}
	if len(normalized) != 2 {
		t.Fatalf("normalized count = %d, want 2", len(normalized))
	}
	if normalized[0].RequestID != "request-1" || normalized[1].RequestID != "request-1" {
		t.Fatalf("request IDs = %q/%q, want request-1", normalized[0].RequestID, normalized[1].RequestID)
	}
	if normalized[0].TraceID == "" || normalized[1].TraceID == "" || normalized[0].TraceID != normalized[1].TraceID {
		t.Fatalf("trace IDs should be populated and shared, got %q/%q", normalized[0].TraceID, normalized[1].TraceID)
	}
	if normalized[0].CurrentChainingTraceID != "chain-request-1" || normalized[1].CurrentChainingTraceID != "chain-request-1" {
		t.Fatalf("current chaining trace IDs = %q/%q, want chain-request-1", normalized[0].CurrentChainingTraceID, normalized[1].CurrentChainingTraceID)
	}
	if normalized[0].TraceID != normalized[0].CurrentChainingTraceID || normalized[1].TraceID != normalized[1].CurrentChainingTraceID {
		t.Fatalf("trace IDs and current chaining trace IDs should match, got %#v", normalized)
	}
	if normalized[0].WorkID != "batch-request-1-first" || normalized[1].WorkID != "batch-request-1-second" {
		t.Fatalf("work IDs = %q/%q", normalized[0].WorkID, normalized[1].WorkID)
	}
	if normalized[0].Tags["_work_name"] != "first" || normalized[0].Tags["_work_type"] != "task" {
		t.Fatalf("normalized tags missing work metadata: %#v", normalized[0].Tags)
	}
	if string(normalized[0].Payload) != `{"title":"first"}` {
		t.Fatalf("payload = %s", normalized[0].Payload)
	}
}

func TestNormalizeWorkRequest_DependsOnRelationTargetsRequiredState(t *testing.T) {
	request := interfaces.WorkRequest{
		RequestID: "request-1",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{
			{Name: "build", WorkTypeID: "task", WorkID: "work-build", TraceID: "trace-batch"},
			{Name: "test", WorkTypeID: "task", WorkID: "work-test"},
		},
		Relations: []interfaces.WorkRelation{{
			Type:           interfaces.WorkRelationDependsOn,
			SourceWorkName: "test",
			TargetWorkName: "build",
			RequiredState:  "reviewed",
		}},
	}

	normalized, err := NormalizeWorkRequest(request, interfaces.WorkRequestNormalizeOptions{
		ValidWorkTypes: map[string]bool{"task": true},
	})
	if err != nil {
		t.Fatalf("NormalizeWorkRequest: %v", err)
	}

	dependent := findSubmitRequest(t, normalized, "test")
	if len(dependent.Relations) != 1 {
		t.Fatalf("dependent relation count = %d, want 1", len(dependent.Relations))
	}
	relation := dependent.Relations[0]
	if relation.Type != interfaces.RelationDependsOn || relation.TargetWorkID != "work-build" || relation.RequiredState != "reviewed" {
		t.Fatalf("relation = %#v", relation)
	}
	if dependent.TraceID != "trace-batch" {
		t.Fatalf("dependent trace ID = %q, want trace-batch", dependent.TraceID)
	}
}

func TestNormalizeWorkRequest_ParentChildRelationTargetsParentAndCoexistsWithDependsOn(t *testing.T) {
	request := interfaces.WorkRequest{
		RequestID: "request-parent-child-1",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{
			{Name: "parent", WorkTypeID: "task", WorkID: "work-parent", TraceID: "trace-parent-child"},
			{Name: "prerequisite", WorkTypeID: "task", WorkID: "work-prerequisite"},
			{Name: "child", WorkTypeID: "task", WorkID: "work-child"},
		},
		Relations: []interfaces.WorkRelation{
			{
				Type:           interfaces.WorkRelationParentChild,
				SourceWorkName: "child",
				TargetWorkName: "parent",
			},
			{
				Type:           interfaces.WorkRelationDependsOn,
				SourceWorkName: "child",
				TargetWorkName: "prerequisite",
			},
		},
	}

	normalized, err := NormalizeWorkRequest(request, interfaces.WorkRequestNormalizeOptions{
		ValidWorkTypes: map[string]bool{"task": true},
	})
	if err != nil {
		t.Fatalf("NormalizeWorkRequest: %v", err)
	}

	child := findSubmitRequest(t, normalized, "child")
	if len(child.Relations) != 2 {
		t.Fatalf("child relation count = %d, want 2", len(child.Relations))
	}

	var foundParentChild bool
	var foundDependsOn bool
	for _, relation := range child.Relations {
		switch relation.Type {
		case interfaces.RelationParentChild:
			foundParentChild = true
			if relation.TargetWorkID != "work-parent" {
				t.Fatalf("parent-child target = %q, want work-parent", relation.TargetWorkID)
			}
		case interfaces.RelationDependsOn:
			foundDependsOn = true
			if relation.TargetWorkID != "work-prerequisite" {
				t.Fatalf("depends_on target = %q, want work-prerequisite", relation.TargetWorkID)
			}
			if relation.RequiredState != "complete" {
				t.Fatalf("depends_on required_state = %q, want complete", relation.RequiredState)
			}
		default:
			t.Fatalf("unexpected relation = %#v", relation)
		}
	}
	if !foundParentChild {
		t.Fatal("missing parent-child relation")
	}
	if !foundDependsOn {
		t.Fatal("missing depends_on relation")
	}
	if child.TraceID != "trace-parent-child" {
		t.Fatalf("child trace ID = %q, want trace-parent-child", child.TraceID)
	}
}

func TestNormalizeWorkRequest_RejectsMultipleParentChildParentsForOneChild(t *testing.T) {
	_, err := NormalizeWorkRequest(interfaces.WorkRequest{
		RequestID: "request-parent-child-conflict",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{
			{Name: "parent-a", WorkTypeID: "story-set"},
			{Name: "parent-b", WorkTypeID: "story-set"},
			{Name: "story-a", WorkTypeID: "story"},
		},
		Relations: []interfaces.WorkRelation{
			{Type: interfaces.WorkRelationParentChild, SourceWorkName: "story-a", TargetWorkName: "parent-a"},
			{Type: interfaces.WorkRelationParentChild, SourceWorkName: "story-a", TargetWorkName: "parent-b"},
		},
	}, interfaces.WorkRequestNormalizeOptions{
		ValidWorkTypes: map[string]bool{"story-set": true, "story": true},
	})
	if err == nil || !strings.Contains(err.Error(), "multiple PARENT_CHILD parents") {
		t.Fatalf("expected multiple parent rejection, got %v", err)
	}
}

func TestNormalizeWorkRequest_FillsMissingWorkTypeFromContext(t *testing.T) {
	normalized, err := NormalizeWorkRequest(interfaces.WorkRequest{
		RequestID: "request-1",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works:     []interfaces.Work{{Name: "inferred"}},
	}, interfaces.WorkRequestNormalizeOptions{
		DefaultWorkTypeID: "task",
		ValidWorkTypes:    map[string]bool{"task": true},
	})
	if err != nil {
		t.Fatalf("NormalizeWorkRequest: %v", err)
	}
	if normalized[0].WorkTypeID != "task" {
		t.Fatalf("work type = %q, want task", normalized[0].WorkTypeID)
	}
}

func TestNormalizeWorkRequest_ForwardsExplicitPublicState(t *testing.T) {
	normalized, err := NormalizeWorkRequest(interfaces.WorkRequest{
		RequestID: "request-state",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{{
			Name:       "draft",
			WorkTypeID: "task",
			State:      "queued",
		}},
	}, interfaces.WorkRequestNormalizeOptions{
		ValidWorkTypes:    map[string]bool{"task": true},
		ValidStatesByType: map[string]map[string]bool{"task": {"queued": true, "complete": true}},
	})
	if err != nil {
		t.Fatalf("NormalizeWorkRequest: %v", err)
	}
	if normalized[0].TargetState != "queued" {
		t.Fatalf("target state = %q, want queued", normalized[0].TargetState)
	}
}

func TestNormalizeWorkRequest_RejectsWorkTypeConflict(t *testing.T) {
	_, err := NormalizeWorkRequest(interfaces.WorkRequest{
		RequestID: "request-1",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works:     []interfaces.Work{{Name: "conflict", WorkTypeID: "other"}},
	}, interfaces.WorkRequestNormalizeOptions{
		DefaultWorkTypeID: "task",
		ValidWorkTypes:    map[string]bool{"task": true, "other": true},
	})
	if err == nil || !strings.Contains(err.Error(), "conflicts with context work type") {
		t.Fatalf("expected work type conflict error, got %v", err)
	}
}

func TestNormalizeWorkRequest_RejectsUnknownExplicitState(t *testing.T) {
	_, err := NormalizeWorkRequest(interfaces.WorkRequest{
		RequestID: "request-invalid-state",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{{
			Name:       "draft",
			WorkTypeID: "task",
			State:      "queued",
		}},
	}, interfaces.WorkRequestNormalizeOptions{
		ValidWorkTypes:    map[string]bool{"task": true},
		ValidStatesByType: map[string]map[string]bool{"task": {"init": true, "complete": true}},
	})
	if err == nil || !strings.Contains(err.Error(), `references unknown state "queued"`) {
		t.Fatalf("expected state validation error, got %v", err)
	}
}

func TestNormalizeWorkRequest_RejectsUnknownDependencyRequiredState(t *testing.T) {
	_, err := NormalizeWorkRequest(interfaces.WorkRequest{
		RequestID: "request-invalid-required-state",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{
			{Name: "draft", WorkTypeID: "task"},
			{Name: "review", WorkTypeID: "task"},
		},
		Relations: []interfaces.WorkRelation{{
			Type:           interfaces.WorkRelationDependsOn,
			SourceWorkName: "review",
			TargetWorkName: "draft",
			RequiredState:  "queued",
		}},
	}, interfaces.WorkRequestNormalizeOptions{
		ValidWorkTypes:    map[string]bool{"task": true},
		ValidStatesByType: map[string]map[string]bool{"task": {"init": true, "complete": true}},
	})
	if err == nil || !strings.Contains(err.Error(), `references unknown required_state "queued"`) {
		t.Fatalf("expected required_state validation error, got %v", err)
	}
}

func TestNormalizeWorkRequest_RejectsDependencyCycle(t *testing.T) {
	_, err := NormalizeWorkRequest(interfaces.WorkRequest{
		RequestID: "request-1",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{
			{Name: "first", WorkTypeID: "task"},
			{Name: "second", WorkTypeID: "task"},
		},
		Relations: []interfaces.WorkRelation{
			{Type: interfaces.WorkRelationDependsOn, SourceWorkName: "first", TargetWorkName: "second"},
			{Type: interfaces.WorkRelationDependsOn, SourceWorkName: "second", TargetWorkName: "first"},
		},
	}, interfaces.WorkRequestNormalizeOptions{ValidWorkTypes: map[string]bool{"task": true}})
	if err == nil || !strings.Contains(err.Error(), "dependency cycle") {
		t.Fatalf("expected dependency cycle error, got %v", err)
	}
}

// portos:func-length-exception owner=agent-factory reason=table-driven-validation-matrix review=2026-07-19 removal=split-validation-cases-before-next-work-request-contract-change
func TestNormalizeWorkRequest_RejectsValidationFailures(t *testing.T) {
	tests := []struct {
		name    string
		request interfaces.WorkRequest
		wantErr string
	}{
		{
			name:    "empty work list",
			request: interfaces.WorkRequest{RequestID: "request-1", Type: interfaces.WorkRequestTypeFactoryRequestBatch},
			wantErr: "works array must contain at least one item",
		},
		{
			name: "duplicate work names",
			request: interfaces.WorkRequest{
				RequestID: "request-1",
				Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
				Works:     []interfaces.Work{{Name: "same", WorkTypeID: "task"}, {Name: "same", WorkTypeID: "task"}},
			},
			wantErr: "duplicate name",
		},
		{
			name: "unknown relation type",
			request: interfaces.WorkRequest{
				RequestID: "request-1",
				Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
				Works:     []interfaces.Work{{Name: "first", WorkTypeID: "task"}, {Name: "second", WorkTypeID: "task"}},
				Relations: []interfaces.WorkRelation{{
					Type:           interfaces.WorkRelationType("INVALID"),
					SourceWorkName: "first",
					TargetWorkName: "second",
				}},
			},
			wantErr: "unsupported type",
		},
		{
			name: "missing source endpoint",
			request: interfaces.WorkRequest{
				RequestID: "request-1",
				Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
				Works:     []interfaces.Work{{Name: "first", WorkTypeID: "task"}},
				Relations: []interfaces.WorkRelation{{
					Type:           interfaces.WorkRelationDependsOn,
					TargetWorkName: "first",
				}},
			},
			wantErr: "missing source_work_name",
		},
		{
			name: "blank source endpoint",
			request: interfaces.WorkRequest{
				RequestID: "request-1",
				Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
				Works:     []interfaces.Work{{Name: "first", WorkTypeID: "task"}},
				Relations: []interfaces.WorkRelation{{
					Type:           interfaces.WorkRelationDependsOn,
					SourceWorkName: "   ",
					TargetWorkName: "first",
				}},
			},
			wantErr: "missing source_work_name",
		},
		{
			name: "missing target endpoint",
			request: interfaces.WorkRequest{
				RequestID: "request-1",
				Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
				Works:     []interfaces.Work{{Name: "first", WorkTypeID: "task"}},
				Relations: []interfaces.WorkRelation{{
					Type:           interfaces.WorkRelationDependsOn,
					SourceWorkName: "first",
				}},
			},
			wantErr: "missing target_work_name",
		},
		{
			name: "unknown source endpoint",
			request: interfaces.WorkRequest{
				RequestID: "request-1",
				Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
				Works:     []interfaces.Work{{Name: "first", WorkTypeID: "task"}},
				Relations: []interfaces.WorkRelation{{
					Type:           interfaces.WorkRelationDependsOn,
					SourceWorkName: "missing",
					TargetWorkName: "first",
				}},
			},
			wantErr: "unknown source_work_name",
		},
		{
			name: "unknown target endpoint",
			request: interfaces.WorkRequest{
				RequestID: "request-1",
				Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
				Works:     []interfaces.Work{{Name: "first", WorkTypeID: "task"}},
				Relations: []interfaces.WorkRelation{{
					Type:           interfaces.WorkRelationDependsOn,
					SourceWorkName: "first",
					TargetWorkName: "missing",
				}},
			},
			wantErr: "unknown target_work_name",
		},
		{
			name: "self dependency",
			request: interfaces.WorkRequest{
				RequestID: "request-1",
				Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
				Works:     []interfaces.Work{{Name: "first", WorkTypeID: "task"}},
				Relations: []interfaces.WorkRelation{{
					Type:           interfaces.WorkRelationDependsOn,
					SourceWorkName: "first",
					TargetWorkName: "first",
				}},
			},
			wantErr: "self-dependency",
		},
		{
			name: "self parenting",
			request: interfaces.WorkRequest{
				RequestID: "request-1",
				Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
				Works:     []interfaces.Work{{Name: "first", WorkTypeID: "task"}},
				Relations: []interfaces.WorkRelation{{
					Type:           interfaces.WorkRelationParentChild,
					SourceWorkName: "first",
					TargetWorkName: "first",
				}},
			},
			wantErr: "self-parenting",
		},
		{
			name: "duplicate parent child relation",
			request: interfaces.WorkRequest{
				RequestID: "request-1",
				Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
				Works: []interfaces.Work{
					{Name: "parent", WorkTypeID: "task"},
					{Name: "child", WorkTypeID: "task"},
				},
				Relations: []interfaces.WorkRelation{
					{Type: interfaces.WorkRelationParentChild, SourceWorkName: "child", TargetWorkName: "parent"},
					{Type: interfaces.WorkRelationParentChild, SourceWorkName: "child", TargetWorkName: "parent"},
				},
			},
			wantErr: "duplicates relations[0]",
		},
		{
			name: "parent child required state",
			request: interfaces.WorkRequest{
				RequestID: "request-1",
				Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
				Works: []interfaces.Work{
					{Name: "parent", WorkTypeID: "task"},
					{Name: "child", WorkTypeID: "task"},
				},
				Relations: []interfaces.WorkRelation{{
					Type:           interfaces.WorkRelationParentChild,
					SourceWorkName: "child",
					TargetWorkName: "parent",
					RequiredState:  "complete",
				}},
			},
			wantErr: "must not set required_state",
		},
		{
			name: "unknown work type",
			request: interfaces.WorkRequest{
				RequestID: "request-1",
				Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
				Works:     []interfaces.Work{{Name: "first", WorkTypeID: "missing"}},
			},
			wantErr: "unknown work type",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NormalizeWorkRequest(tc.request, interfaces.WorkRequestNormalizeOptions{
				ValidWorkTypes: map[string]bool{"task": true},
			})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestNormalizeWorkRequest_AcceptsRawJSONPayload(t *testing.T) {
	raw := json.RawMessage(`{"key":"value"}`)
	normalized, err := NormalizeWorkRequest(interfaces.WorkRequest{
		RequestID: "request-1",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works:     []interfaces.Work{{Name: "raw", WorkTypeID: "task", Payload: raw}},
	}, interfaces.WorkRequestNormalizeOptions{ValidWorkTypes: map[string]bool{"task": true}})
	if err != nil {
		t.Fatalf("NormalizeWorkRequest: %v", err)
	}
	if string(normalized[0].Payload) != `{"key":"value"}` {
		t.Fatalf("payload = %s", normalized[0].Payload)
	}
}

func TestNormalizeWorkRequest_AcceptsStringPayloadAsRawText(t *testing.T) {
	normalized, err := NormalizeWorkRequest(interfaces.WorkRequest{
		RequestID: "request-string-payload",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works:     []interfaces.Work{{Name: "raw", WorkTypeID: "task", Payload: "plain text"}},
	}, interfaces.WorkRequestNormalizeOptions{ValidWorkTypes: map[string]bool{"task": true}})
	if err != nil {
		t.Fatalf("NormalizeWorkRequest: %v", err)
	}
	if string(normalized[0].Payload) != "plain text" {
		t.Fatalf("payload = %q, want plain text", normalized[0].Payload)
	}
}

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

func findSubmitRequest(t *testing.T, requests []interfaces.SubmitRequest, name string) interfaces.SubmitRequest {
	t.Helper()
	for _, request := range requests {
		if request.Name == name {
			return request
		}
	}
	t.Fatalf("submit request named %q not found in %#v", name, requests)
	return interfaces.SubmitRequest{}
}
