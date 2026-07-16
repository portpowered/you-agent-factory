package requests

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/work"
)

func TestResolveWorkRequestCurrentChainingTraceID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current string
		legacy  string
		want    string
	}{
		{name: "prefers current", current: "chain-current", legacy: "trace-legacy", want: "chain-current"},
		{name: "falls back to legacy", legacy: "trace-legacy", want: "trace-legacy"},
		{name: "empty when neither present", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := ResolveWorkRequestCurrentChainingTraceID(tc.current, tc.legacy); got != tc.want {
				t.Fatalf("ResolveWorkRequestCurrentChainingTraceID(%q, %q) = %q, want %q", tc.current, tc.legacy, got, tc.want)
			}
		})
	}
}

func TestValidateWorkRequestTraceFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current string
		legacy  string
		wantErr error
	}{
		{name: "matching aliases", current: "chain-1", legacy: "chain-1"},
		{name: "current only", current: "chain-1"},
		{name: "legacy only", legacy: "trace-1"},
		{name: "conflicting aliases", current: "chain-1", legacy: "trace-1", wantErr: errConflictingWorkRequestTraceFields},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateWorkRequestTraceFields(tc.current, tc.legacy)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("ValidateWorkRequestTraceFields(%q, %q) error = %v, want %v", tc.current, tc.legacy, err, tc.wantErr)
			}
		})
	}
}

func TestNormalizeWorkRequest_IndependentWorkItemsShareRequestAndTrace(t *testing.T) {
	request := work.WorkRequest{
		RequestID:              "request-1",
		CurrentChainingTraceID: "chain-request-1",
		Type:                   work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{
			{Name: "first", WorkTypeID: "task", Payload: map[string]any{"title": "first"}},
			{Name: "second", WorkTypeID: "task", Payload: map[string]any{"title": "second"}},
		},
	}

	normalized, err := NormalizeWorkRequest(request, work.WorkRequestNormalizeOptions{
		ValidWorkTypes: map[string]bool{"task": true},
	})
	if err != nil {
		t.Fatalf("NormalizeWorkRequest: %v", err)
	}
	assertIndependentWorkIdentity(t, normalized)
	assertIndependentWorkMetadata(t, normalized)
}

func assertIndependentWorkIdentity(t *testing.T, normalized []work.SubmitRequest) {
	t.Helper()

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
}

func assertIndependentWorkMetadata(t *testing.T, normalized []work.SubmitRequest) {
	t.Helper()

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

func TestSubmitResultFromNormalized_PopulatesPerWorkIdentifiers(t *testing.T) {
	normalized := []work.SubmitRequest{
		{RequestID: "request-1", WorkID: "batch-request-1-first", Name: "first", WorkTypeID: "task", TraceID: "trace-1"},
		{RequestID: "request-1", WorkID: "batch-request-1-second", Name: "second", WorkTypeID: "review", TraceID: "trace-1"},
	}
	result := SubmitResultFromNormalized("request-1", normalized)
	if result.RequestID != "request-1" || result.TraceID != "trace-1" || !result.Accepted {
		t.Fatalf("result = %#v, want accepted request metadata", result)
	}
	if len(result.Works) != 2 {
		t.Fatalf("works = %#v, want 2 items", result.Works)
	}
	if result.Works[0].Name != "first" || result.Works[0].WorkTypeName != "task" || result.Works[0].WorkID != "batch-request-1-first" {
		t.Fatalf("works[0] = %#v, want first/task/batch-request-1-first", result.Works[0])
	}
	if result.Works[1].Name != "second" || result.Works[1].WorkTypeName != "review" || result.Works[1].WorkID != "batch-request-1-second" {
		t.Fatalf("works[1] = %#v, want second/review/batch-request-1-second", result.Works[1])
	}
}

func TestNormalizeWorkRequest_LegacyTraceIDPropagatesStableCurrentChainingTrace(t *testing.T) {
	request := work.WorkRequest{
		RequestID: "request-legacy-trace",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{
			{Name: "first", WorkTypeID: "task", TraceID: "trace-legacy"},
			{Name: "second", WorkTypeID: "task"},
		},
	}

	normalized, err := NormalizeWorkRequest(request, work.WorkRequestNormalizeOptions{
		ValidWorkTypes: map[string]bool{"task": true},
	})
	if err != nil {
		t.Fatalf("NormalizeWorkRequest: %v", err)
	}
	if len(normalized) != 2 {
		t.Fatalf("normalized count = %d, want 2", len(normalized))
	}
	if normalized[0].TraceID != "trace-legacy" || normalized[0].CurrentChainingTraceID != "trace-legacy" {
		t.Fatalf("first normalized trace fields = %#v, want legacy trace fallback", normalized[0])
	}
	if normalized[1].TraceID != "trace-legacy" || normalized[1].CurrentChainingTraceID != "trace-legacy" {
		t.Fatalf("second normalized trace fields = %#v, want propagated legacy trace fallback", normalized[1])
	}
}

func TestNormalizeWorkRequest_DependsOnRelationTargetsRequiredState(t *testing.T) {
	request := work.WorkRequest{
		RequestID: "request-1",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{
			{Name: "build", WorkTypeID: "task", WorkID: "work-build", TraceID: "trace-batch"},
			{Name: "test", WorkTypeID: "task", WorkID: "work-test"},
		},
		Relations: []work.WorkRelation{{
			Type:           work.WorkRelationDependsOn,
			SourceWorkName: "test",
			TargetWorkName: "build",
			RequiredState:  "reviewed",
		}},
	}

	normalized, err := NormalizeWorkRequest(request, work.WorkRequestNormalizeOptions{
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
	if relation.Type != work.RelationDependsOn || relation.TargetWorkID != "work-build" || relation.RequiredState != "reviewed" {
		t.Fatalf("relation = %#v", relation)
	}
	if dependent.TraceID != "trace-batch" {
		t.Fatalf("dependent trace ID = %q, want trace-batch", dependent.TraceID)
	}
}

func TestNormalizeWorkRequest_ParentChildRelationTargetsParentAndCoexistsWithDependsOn(t *testing.T) {
	request := work.WorkRequest{
		RequestID: "request-parent-child-1",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{
			{Name: "parent", WorkTypeID: "task", WorkID: "work-parent", TraceID: "trace-parent-child"},
			{Name: "prerequisite", WorkTypeID: "task", WorkID: "work-prerequisite"},
			{Name: "child", WorkTypeID: "task", WorkID: "work-child"},
		},
		Relations: []work.WorkRelation{
			{
				Type:           work.WorkRelationParentChild,
				SourceWorkName: "child",
				TargetWorkName: "parent",
			},
			{
				Type:           work.WorkRelationDependsOn,
				SourceWorkName: "child",
				TargetWorkName: "prerequisite",
			},
		},
	}

	normalized, err := NormalizeWorkRequest(request, work.WorkRequestNormalizeOptions{
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
		case work.RelationParentChild:
			foundParentChild = true
			if relation.TargetWorkID != "work-parent" {
				t.Fatalf("parent-child target = %q, want work-parent", relation.TargetWorkID)
			}
		case work.RelationDependsOn:
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
	_, err := NormalizeWorkRequest(work.WorkRequest{
		RequestID: "request-parent-child-conflict",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{
			{Name: "parent-a", WorkTypeID: "story-set"},
			{Name: "parent-b", WorkTypeID: "story-set"},
			{Name: "story-a", WorkTypeID: "story"},
		},
		Relations: []work.WorkRelation{
			{Type: work.WorkRelationParentChild, SourceWorkName: "story-a", TargetWorkName: "parent-a"},
			{Type: work.WorkRelationParentChild, SourceWorkName: "story-a", TargetWorkName: "parent-b"},
		},
	}, work.WorkRequestNormalizeOptions{
		ValidWorkTypes: map[string]bool{"story-set": true, "story": true},
	})
	wantErr := `work_request: relations[1] assigns multiple PARENT_CHILD parents to "story-a" ("parent-a" and "parent-b")`
	if err == nil || err.Error() != wantErr {
		t.Fatalf("error = %v, want %q", err, wantErr)
	}
}

func TestNormalizeWorkRequest_RejectsDuplicateDependsOnRelationWithNormalizedRequiredState(t *testing.T) {
	_, err := NormalizeWorkRequest(work.WorkRequest{
		RequestID: "request-duplicate-depends-on",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{
			{Name: "build", WorkTypeID: "task"},
			{Name: "review", WorkTypeID: "task"},
		},
		Relations: []work.WorkRelation{
			{
				Type:           work.WorkRelationDependsOn,
				SourceWorkName: "review",
				TargetWorkName: "build",
				RequiredState:  "reviewed",
			},
			{
				Type:           work.WorkRelationDependsOn,
				SourceWorkName: "review",
				TargetWorkName: "build",
				RequiredState:  "reviewed",
			},
		},
	}, work.WorkRequestNormalizeOptions{
		ValidWorkTypes:    map[string]bool{"task": true},
		ValidStatesByType: map[string]map[string]bool{"task": {"reviewed": true, "complete": true}},
	})
	wantErr := `work_request: relations[1] duplicates relations[0] ("DEPENDS_ON" "review" -> "build" with requiredState "reviewed")`
	if err == nil || err.Error() != wantErr {
		t.Fatalf("error = %v, want %q", err, wantErr)
	}
}

func TestNormalizeWorkRequest_RejectsDuplicateParentChildRelationWithoutRequiredStateSuffix(t *testing.T) {
	_, err := NormalizeWorkRequest(work.WorkRequest{
		RequestID: "request-duplicate-parent-child",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{
			{Name: "parent", WorkTypeID: "task"},
			{Name: "child", WorkTypeID: "task"},
		},
		Relations: []work.WorkRelation{
			{Type: work.WorkRelationParentChild, SourceWorkName: "child", TargetWorkName: "parent"},
			{Type: work.WorkRelationParentChild, SourceWorkName: "child", TargetWorkName: "parent"},
		},
	}, work.WorkRequestNormalizeOptions{
		ValidWorkTypes: map[string]bool{"task": true},
	})
	wantErr := `work_request: relations[1] duplicates relations[0] ("PARENT_CHILD" "child" -> "parent")`
	if err == nil || err.Error() != wantErr {
		t.Fatalf("error = %v, want %q", err, wantErr)
	}
}

func TestNormalizeWorkRequest_FillsMissingWorkTypeFromContext(t *testing.T) {
	normalized, err := NormalizeWorkRequest(work.WorkRequest{
		RequestID: "request-1",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works:     []work.Work{{Name: "inferred"}},
	}, work.WorkRequestNormalizeOptions{
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
	normalized, err := NormalizeWorkRequest(work.WorkRequest{
		RequestID: "request-state",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name:       "draft",
			WorkTypeID: "task",
			State:      "queued",
		}},
	}, work.WorkRequestNormalizeOptions{
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
	_, err := NormalizeWorkRequest(work.WorkRequest{
		RequestID: "request-1",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works:     []work.Work{{Name: "conflict", WorkTypeID: "other"}},
	}, work.WorkRequestNormalizeOptions{
		DefaultWorkTypeID: "task",
		ValidWorkTypes:    map[string]bool{"task": true, "other": true},
	})
	if err == nil || !strings.Contains(err.Error(), "conflicts with context work type") {
		t.Fatalf("expected work type conflict error, got %v", err)
	}
}

func TestNormalizeWorkRequest_RejectsUnknownExplicitState(t *testing.T) {
	_, err := NormalizeWorkRequest(work.WorkRequest{
		RequestID: "request-invalid-state",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name:       "draft",
			WorkTypeID: "task",
			State:      "queued",
		}},
	}, work.WorkRequestNormalizeOptions{
		ValidWorkTypes:    map[string]bool{"task": true},
		ValidStatesByType: map[string]map[string]bool{"task": {"init": true, "complete": true}},
	})
	if err == nil || !strings.Contains(err.Error(), `references unknown state "queued"`) {
		t.Fatalf("expected state validation error, got %v", err)
	}
}

func TestNormalizeWorkRequest_RejectsUnknownDependencyRequiredState(t *testing.T) {
	_, err := NormalizeWorkRequest(work.WorkRequest{
		RequestID: "request-invalid-required-state",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{
			{Name: "draft", WorkTypeID: "task"},
			{Name: "review", WorkTypeID: "task"},
		},
		Relations: []work.WorkRelation{{
			Type:           work.WorkRelationDependsOn,
			SourceWorkName: "review",
			TargetWorkName: "draft",
			RequiredState:  "queued",
		}},
	}, work.WorkRequestNormalizeOptions{
		ValidWorkTypes:    map[string]bool{"task": true},
		ValidStatesByType: map[string]map[string]bool{"task": {"init": true, "complete": true}},
	})
	if err == nil || !strings.Contains(err.Error(), `references unknown requiredState "queued"`) {
		t.Fatalf("expected requiredState validation error, got %v", err)
	}
}

func TestNormalizeWorkRequest_RejectsDependencyCycle(t *testing.T) {
	_, err := NormalizeWorkRequest(work.WorkRequest{
		RequestID: "request-1",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{
			{Name: "first", WorkTypeID: "task"},
			{Name: "second", WorkTypeID: "task"},
		},
		Relations: []work.WorkRelation{
			{Type: work.WorkRelationDependsOn, SourceWorkName: "first", TargetWorkName: "second"},
			{Type: work.WorkRelationDependsOn, SourceWorkName: "second", TargetWorkName: "first"},
		},
	}, work.WorkRequestNormalizeOptions{ValidWorkTypes: map[string]bool{"task": true}})
	if err == nil || !strings.Contains(err.Error(), "dependency cycle") {
		t.Fatalf("expected dependency cycle error, got %v", err)
	}
}

func TestNormalizeWorkRequest_RejectsValidationFailures_WorkArrayAndEndpoints(t *testing.T) {
	runNormalizeWorkRequestValidationTests(t, []normalizeValidationTestCase{
		{
			name:    "empty work list",
			request: work.WorkRequest{RequestID: "request-1", Type: work.WorkRequestTypeFactoryRequestBatch},
			wantErr: "works array must contain at least one item",
		},
		{
			name: "duplicate work names",
			request: work.WorkRequest{
				RequestID: "request-1",
				Type:      work.WorkRequestTypeFactoryRequestBatch,
				Works:     []work.Work{{Name: "same", WorkTypeID: "task"}, {Name: "same", WorkTypeID: "task"}},
			},
			wantErr: "duplicate name",
		},
		{
			name: "missing source endpoint",
			request: work.WorkRequest{
				RequestID: "request-1",
				Type:      work.WorkRequestTypeFactoryRequestBatch,
				Works:     []work.Work{{Name: "first", WorkTypeID: "task"}},
				Relations: []work.WorkRelation{{Type: work.WorkRelationDependsOn, TargetWorkName: "first"}},
			},
			wantErr: "missing sourceWorkName",
		},
		{
			name: "blank source endpoint",
			request: work.WorkRequest{
				RequestID: "request-1",
				Type:      work.WorkRequestTypeFactoryRequestBatch,
				Works:     []work.Work{{Name: "first", WorkTypeID: "task"}},
				Relations: []work.WorkRelation{{Type: work.WorkRelationDependsOn, SourceWorkName: "   ", TargetWorkName: "first"}},
			},
			wantErr: "missing sourceWorkName",
		},
		{
			name: "missing target endpoint",
			request: work.WorkRequest{
				RequestID: "request-1",
				Type:      work.WorkRequestTypeFactoryRequestBatch,
				Works:     []work.Work{{Name: "first", WorkTypeID: "task"}},
				Relations: []work.WorkRelation{{Type: work.WorkRelationDependsOn, SourceWorkName: "first"}},
			},
			wantErr: "missing targetWorkName",
		},
		{
			name: "unknown source endpoint",
			request: work.WorkRequest{
				RequestID: "request-1",
				Type:      work.WorkRequestTypeFactoryRequestBatch,
				Works:     []work.Work{{Name: "first", WorkTypeID: "task"}},
				Relations: []work.WorkRelation{{Type: work.WorkRelationDependsOn, SourceWorkName: "missing", TargetWorkName: "first"}},
			},
			wantErr: "unknown sourceWorkName",
		},
		{
			name: "unknown target endpoint",
			request: work.WorkRequest{
				RequestID: "request-1",
				Type:      work.WorkRequestTypeFactoryRequestBatch,
				Works:     []work.Work{{Name: "first", WorkTypeID: "task"}},
				Relations: []work.WorkRelation{{Type: work.WorkRelationDependsOn, SourceWorkName: "first", TargetWorkName: "missing"}},
			},
			wantErr: "unknown targetWorkName",
		},
	})
}

func TestNormalizeWorkRequest_RejectsValidationFailures_RelationSemantics(t *testing.T) {
	runNormalizeWorkRequestValidationTests(t, []normalizeValidationTestCase{
		{
			name: "unknown relation type",
			request: work.WorkRequest{
				RequestID: "request-1",
				Type:      work.WorkRequestTypeFactoryRequestBatch,
				Works:     []work.Work{{Name: "first", WorkTypeID: "task"}, {Name: "second", WorkTypeID: "task"}},
				Relations: []work.WorkRelation{{
					Type:           work.WorkRelationType("INVALID"),
					SourceWorkName: "first",
					TargetWorkName: "second",
				}},
			},
			wantErr: "unsupported type",
		},
		{
			name: "self dependency",
			request: work.WorkRequest{
				RequestID: "request-1",
				Type:      work.WorkRequestTypeFactoryRequestBatch,
				Works:     []work.Work{{Name: "first", WorkTypeID: "task"}},
				Relations: []work.WorkRelation{{Type: work.WorkRelationDependsOn, SourceWorkName: "first", TargetWorkName: "first"}},
			},
			wantErr: "self-dependency",
		},
		{
			name: "self parenting",
			request: work.WorkRequest{
				RequestID: "request-1",
				Type:      work.WorkRequestTypeFactoryRequestBatch,
				Works:     []work.Work{{Name: "first", WorkTypeID: "task"}},
				Relations: []work.WorkRelation{{Type: work.WorkRelationParentChild, SourceWorkName: "first", TargetWorkName: "first"}},
			},
			wantErr: "self-parenting",
		},
		{
			name: "duplicate parent child relation",
			request: work.WorkRequest{
				RequestID: "request-1",
				Type:      work.WorkRequestTypeFactoryRequestBatch,
				Works:     []work.Work{{Name: "parent", WorkTypeID: "task"}, {Name: "child", WorkTypeID: "task"}},
				Relations: []work.WorkRelation{
					{Type: work.WorkRelationParentChild, SourceWorkName: "child", TargetWorkName: "parent"},
					{Type: work.WorkRelationParentChild, SourceWorkName: "child", TargetWorkName: "parent"},
				},
			},
			wantErr: "duplicates relations[0]",
		},
		{
			name: "parent child required state",
			request: work.WorkRequest{
				RequestID: "request-1",
				Type:      work.WorkRequestTypeFactoryRequestBatch,
				Works:     []work.Work{{Name: "parent", WorkTypeID: "task"}, {Name: "child", WorkTypeID: "task"}},
				Relations: []work.WorkRelation{{
					Type:           work.WorkRelationParentChild,
					SourceWorkName: "child",
					TargetWorkName: "parent",
					RequiredState:  "complete",
				}},
			},
			wantErr: "must not set requiredState",
		},
	})
}

func TestNormalizeWorkRequest_RejectsValidationFailures_WorkTypeValidation(t *testing.T) {
	runNormalizeWorkRequestValidationTests(t, []normalizeValidationTestCase{
		{
			name: "unknown work type",
			request: work.WorkRequest{
				RequestID: "request-1",
				Type:      work.WorkRequestTypeFactoryRequestBatch,
				Works:     []work.Work{{Name: "first", WorkTypeID: "missing"}},
			},
			wantErr: "unknown work type",
		},
	})
}

type normalizeValidationTestCase struct {
	name    string
	request work.WorkRequest
	wantErr string
}

func runNormalizeWorkRequestValidationTests(t *testing.T, tests []normalizeValidationTestCase) {
	t.Helper()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NormalizeWorkRequest(tc.request, work.WorkRequestNormalizeOptions{
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
	normalized, err := NormalizeWorkRequest(work.WorkRequest{
		RequestID: "request-1",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works:     []work.Work{{Name: "raw", WorkTypeID: "task", Payload: raw}},
	}, work.WorkRequestNormalizeOptions{ValidWorkTypes: map[string]bool{"task": true}})
	if err != nil {
		t.Fatalf("NormalizeWorkRequest: %v", err)
	}
	if string(normalized[0].Payload) != `{"key":"value"}` {
		t.Fatalf("payload = %s", normalized[0].Payload)
	}
}

func TestNormalizeWorkRequest_ExecutionIDTagPropagatesToSubmitRequest(t *testing.T) {
	normalized, err := NormalizeWorkRequest(work.WorkRequest{
		RequestID: "request-exec-id",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name:        "chapter",
			WorkTypeID:  "chapter",
			ExecutionID: "exec-guard-propagation",
		}},
	}, work.WorkRequestNormalizeOptions{ValidWorkTypes: map[string]bool{"chapter": true}})
	if err != nil {
		t.Fatalf("NormalizeWorkRequest: %v", err)
	}
	if normalized[0].ExecutionID != "exec-guard-propagation" {
		t.Fatalf("execution ID = %q, want exec-guard-propagation", normalized[0].ExecutionID)
	}
	if normalized[0].Tags["_execution_id"] != "exec-guard-propagation" {
		t.Fatalf("execution tag = %q, want exec-guard-propagation", normalized[0].Tags["_execution_id"])
	}
}

func TestNormalizeWorkRequest_AcceptsStringPayloadAsRawText(t *testing.T) {
	normalized, err := NormalizeWorkRequest(work.WorkRequest{
		RequestID: "request-string-payload",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works:     []work.Work{{Name: "raw", WorkTypeID: "task", Payload: "plain text"}},
	}, work.WorkRequestNormalizeOptions{ValidWorkTypes: map[string]bool{"task": true}})
	if err != nil {
		t.Fatalf("NormalizeWorkRequest: %v", err)
	}
	if string(normalized[0].Payload) != "plain text" {
		t.Fatalf("payload = %q, want plain text", normalized[0].Payload)
	}
	if len(normalized[0].Content) != 1 {
		t.Fatalf("content count = %d, want 1", len(normalized[0].Content))
	}
	if normalized[0].Content[0].Type != work.WorkContentPartTypeText || normalized[0].Content[0].Text != "plain text" {
		t.Fatalf("content = %#v, want canonical text part", normalized[0].Content)
	}
}

func TestNormalizeWorkRequest_PrefersExplicitContentForLegacyTextPayload(t *testing.T) {
	normalized, err := NormalizeWorkRequest(work.WorkRequest{
		RequestID: "request-explicit-content",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name:       "raw",
			WorkTypeID: "task",
			Content: []work.WorkContentPart{
				{Type: work.WorkContentPartTypeText, Text: "plain "},
				{Type: work.WorkContentPartTypeText, Text: "text"},
			},
			Payload: "plain text",
		}},
	}, work.WorkRequestNormalizeOptions{ValidWorkTypes: map[string]bool{"task": true}})
	if err != nil {
		t.Fatalf("NormalizeWorkRequest: %v", err)
	}
	if len(normalized[0].Content) != 2 {
		t.Fatalf("content count = %d, want 2", len(normalized[0].Content))
	}
	if string(normalized[0].Payload) != "plain text" {
		t.Fatalf("payload = %q, want plain text", normalized[0].Payload)
	}
}

func TestNormalizeWorkRequest_DerivesLegacyPayloadFromExplicitTextContent(t *testing.T) {
	normalized, err := NormalizeWorkRequest(work.WorkRequest{
		RequestID: "request-content-only",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name:       "raw",
			WorkTypeID: "task",
			Content: []work.WorkContentPart{
				{Type: work.WorkContentPartTypeText, Text: "alpha"},
				{Type: work.WorkContentPartTypeText, Text: "\nbeta"},
			},
		}},
	}, work.WorkRequestNormalizeOptions{ValidWorkTypes: map[string]bool{"task": true}})
	if err != nil {
		t.Fatalf("NormalizeWorkRequest: %v", err)
	}
	if string(normalized[0].Payload) != "alpha\nbeta" {
		t.Fatalf("payload = %q, want alpha\\nbeta", normalized[0].Payload)
	}
}

func TestNormalizeWorkRequest_RejectsConflictingExplicitContentAndPayload(t *testing.T) {
	_, err := NormalizeWorkRequest(work.WorkRequest{
		RequestID: "request-content-conflict",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name:       "raw",
			WorkTypeID: "task",
			Content: []work.WorkContentPart{
				{Type: work.WorkContentPartTypeText, Text: "plain text"},
			},
			Payload: "different text",
		}},
	}, work.WorkRequestNormalizeOptions{ValidWorkTypes: map[string]bool{"task": true}})
	if err == nil || !strings.Contains(err.Error(), "payload conflicts with explicit content") {
		t.Fatalf("expected content conflict error, got %v", err)
	}
}

func TestNormalizeWorkRequest_RejectsPayloadAlongsideImageContent(t *testing.T) {
	_, err := NormalizeWorkRequest(work.WorkRequest{
		RequestID: "request-image-conflict",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name:       "raw",
			WorkTypeID: "task",
			Content: []work.WorkContentPart{
				{Type: work.WorkContentPartTypeImage, File: "fixtures/example.png"},
			},
			Payload: "caption",
		}},
	}, work.WorkRequestNormalizeOptions{ValidWorkTypes: map[string]bool{"task": true}})
	if err == nil || !strings.Contains(err.Error(), "payload cannot be combined with image-only canonical content") {
		t.Fatalf("expected image/payload conflict error, got %v", err)
	}
}

func TestNormalizeWorkRequest_ExtendedContentKeepsLegacyTextProjection(t *testing.T) {
	normalized, err := NormalizeWorkRequest(work.WorkRequest{
		RequestID: "request-model-content",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name:       "tts",
			WorkTypeID: "task",
			Content: []work.WorkContentPart{
				{Type: work.WorkContentPartTypeText, Text: "Synthesize"},
				{Type: work.WorkContentPartTypeAudio, File: "artifacts/output.wav"},
				{Type: work.WorkContentPartTypeJSON, JSON: json.RawMessage(`{"voice":"alloy"}`)},
			},
		}},
	}, work.WorkRequestNormalizeOptions{ValidWorkTypes: map[string]bool{"task": true}})
	if err != nil {
		t.Fatalf("NormalizeWorkRequest: %v", err)
	}
	if len(normalized) != 1 {
		t.Fatalf("normalized count = %d, want 1", len(normalized))
	}
	if string(normalized[0].Payload) != "Synthesize" {
		t.Fatalf("payload = %q, want text-only legacy projection", normalized[0].Payload)
	}
	if len(normalized[0].Content) != 3 || normalized[0].Content[1].Type != work.WorkContentPartTypeAudio || normalized[0].Content[2].Type != work.WorkContentPartTypeJSON {
		t.Fatalf("content = %#v, want preserved extended content", normalized[0].Content)
	}
	if normalized[0].Content[1].URL != "file://artifacts/output.wav" {
		t.Fatalf("audio url = %q, want legacy file normalized to url", normalized[0].Content[1].URL)
	}
	if normalized[0].Content[1].File != "" {
		t.Fatalf("audio file = %q, want empty canonical file field", normalized[0].Content[1].File)
	}
}

func TestNormalizeWorkRequest_NormalizesLegacyFileOnlyImageContent(t *testing.T) {
	normalized, err := NormalizeWorkRequest(work.WorkRequest{
		RequestID: "request-legacy-file",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name:       "raw",
			WorkTypeID: "task",
			Content: []work.WorkContentPart{
				{Type: work.WorkContentPartTypeImage, File: "fixtures/example.png"},
			},
		}},
	}, work.WorkRequestNormalizeOptions{ValidWorkTypes: map[string]bool{"task": true}})
	if err != nil {
		t.Fatalf("NormalizeWorkRequest: %v", err)
	}
	if len(normalized) != 1 || len(normalized[0].Content) != 1 {
		t.Fatalf("normalized = %#v, want one image part", normalized)
	}
	if normalized[0].Content[0].URL != "file://fixtures/example.png" {
		t.Fatalf("url = %q, want file://fixtures/example.png", normalized[0].Content[0].URL)
	}
	if normalized[0].Content[0].File != "" {
		t.Fatalf("file = %q, want empty canonical file field", normalized[0].Content[0].File)
	}
}

func findSubmitRequest(t *testing.T, requests []work.SubmitRequest, name string) work.SubmitRequest {
	t.Helper()
	for _, request := range requests {
		if request.Name == name {
			return request
		}
	}
	t.Fatalf("submit request named %q not found in %#v", name, requests)
	return work.SubmitRequest{}
}
