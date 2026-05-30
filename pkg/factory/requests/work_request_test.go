package requests

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
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

// pkgmaintcheck:ignore-cyclomatic-complexity this normalization contract test keeps shared-request, trace, and relation assertions inline for readability.
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

func TestSubmitResultFromNormalized_PopulatesPerWorkIdentifiers(t *testing.T) {
	normalized := []interfaces.SubmitRequest{
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
	request := interfaces.WorkRequest{
		RequestID: "request-legacy-trace",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{
			{Name: "first", WorkTypeID: "task", TraceID: "trace-legacy"},
			{Name: "second", WorkTypeID: "task"},
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
	if normalized[0].TraceID != "trace-legacy" || normalized[0].CurrentChainingTraceID != "trace-legacy" {
		t.Fatalf("first normalized trace fields = %#v, want legacy trace fallback", normalized[0])
	}
	if normalized[1].TraceID != "trace-legacy" || normalized[1].CurrentChainingTraceID != "trace-legacy" {
		t.Fatalf("second normalized trace fields = %#v, want propagated legacy trace fallback", normalized[1])
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
	wantErr := `work_request: relations[1] assigns multiple PARENT_CHILD parents to "story-a" ("parent-a" and "parent-b")`
	if err == nil || err.Error() != wantErr {
		t.Fatalf("error = %v, want %q", err, wantErr)
	}
}

func TestNormalizeWorkRequest_RejectsDuplicateDependsOnRelationWithNormalizedRequiredState(t *testing.T) {
	_, err := NormalizeWorkRequest(interfaces.WorkRequest{
		RequestID: "request-duplicate-depends-on",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{
			{Name: "build", WorkTypeID: "task"},
			{Name: "review", WorkTypeID: "task"},
		},
		Relations: []interfaces.WorkRelation{
			{
				Type:           interfaces.WorkRelationDependsOn,
				SourceWorkName: "review",
				TargetWorkName: "build",
				RequiredState:  "reviewed",
			},
			{
				Type:           interfaces.WorkRelationDependsOn,
				SourceWorkName: "review",
				TargetWorkName: "build",
				RequiredState:  "reviewed",
			},
		},
	}, interfaces.WorkRequestNormalizeOptions{
		ValidWorkTypes:    map[string]bool{"task": true},
		ValidStatesByType: map[string]map[string]bool{"task": {"reviewed": true, "complete": true}},
	})
	wantErr := `work_request: relations[1] duplicates relations[0] ("DEPENDS_ON" "review" -> "build" with requiredState "reviewed")`
	if err == nil || err.Error() != wantErr {
		t.Fatalf("error = %v, want %q", err, wantErr)
	}
}

func TestNormalizeWorkRequest_RejectsDuplicateParentChildRelationWithoutRequiredStateSuffix(t *testing.T) {
	_, err := NormalizeWorkRequest(interfaces.WorkRequest{
		RequestID: "request-duplicate-parent-child",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{
			{Name: "parent", WorkTypeID: "task"},
			{Name: "child", WorkTypeID: "task"},
		},
		Relations: []interfaces.WorkRelation{
			{Type: interfaces.WorkRelationParentChild, SourceWorkName: "child", TargetWorkName: "parent"},
			{Type: interfaces.WorkRelationParentChild, SourceWorkName: "child", TargetWorkName: "parent"},
		},
	}, interfaces.WorkRequestNormalizeOptions{
		ValidWorkTypes: map[string]bool{"task": true},
	})
	wantErr := `work_request: relations[1] duplicates relations[0] ("PARENT_CHILD" "child" -> "parent")`
	if err == nil || err.Error() != wantErr {
		t.Fatalf("error = %v, want %q", err, wantErr)
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
	if err == nil || !strings.Contains(err.Error(), `references unknown requiredState "queued"`) {
		t.Fatalf("expected requiredState validation error, got %v", err)
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

func TestNormalizeWorkRequest_RejectsValidationFailures_WorkArrayAndEndpoints(t *testing.T) {
	runNormalizeWorkRequestValidationTests(t, []normalizeValidationTestCase{
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
			name: "missing source endpoint",
			request: interfaces.WorkRequest{
				RequestID: "request-1",
				Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
				Works:     []interfaces.Work{{Name: "first", WorkTypeID: "task"}},
				Relations: []interfaces.WorkRelation{{Type: interfaces.WorkRelationDependsOn, TargetWorkName: "first"}},
			},
			wantErr: "missing sourceWorkName",
		},
		{
			name: "blank source endpoint",
			request: interfaces.WorkRequest{
				RequestID: "request-1",
				Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
				Works:     []interfaces.Work{{Name: "first", WorkTypeID: "task"}},
				Relations: []interfaces.WorkRelation{{Type: interfaces.WorkRelationDependsOn, SourceWorkName: "   ", TargetWorkName: "first"}},
			},
			wantErr: "missing sourceWorkName",
		},
		{
			name: "missing target endpoint",
			request: interfaces.WorkRequest{
				RequestID: "request-1",
				Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
				Works:     []interfaces.Work{{Name: "first", WorkTypeID: "task"}},
				Relations: []interfaces.WorkRelation{{Type: interfaces.WorkRelationDependsOn, SourceWorkName: "first"}},
			},
			wantErr: "missing targetWorkName",
		},
		{
			name: "unknown source endpoint",
			request: interfaces.WorkRequest{
				RequestID: "request-1",
				Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
				Works:     []interfaces.Work{{Name: "first", WorkTypeID: "task"}},
				Relations: []interfaces.WorkRelation{{Type: interfaces.WorkRelationDependsOn, SourceWorkName: "missing", TargetWorkName: "first"}},
			},
			wantErr: "unknown sourceWorkName",
		},
		{
			name: "unknown target endpoint",
			request: interfaces.WorkRequest{
				RequestID: "request-1",
				Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
				Works:     []interfaces.Work{{Name: "first", WorkTypeID: "task"}},
				Relations: []interfaces.WorkRelation{{Type: interfaces.WorkRelationDependsOn, SourceWorkName: "first", TargetWorkName: "missing"}},
			},
			wantErr: "unknown targetWorkName",
		},
	})
}

func TestNormalizeWorkRequest_RejectsValidationFailures_RelationSemantics(t *testing.T) {
	runNormalizeWorkRequestValidationTests(t, []normalizeValidationTestCase{
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
			name: "self dependency",
			request: interfaces.WorkRequest{
				RequestID: "request-1",
				Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
				Works:     []interfaces.Work{{Name: "first", WorkTypeID: "task"}},
				Relations: []interfaces.WorkRelation{{Type: interfaces.WorkRelationDependsOn, SourceWorkName: "first", TargetWorkName: "first"}},
			},
			wantErr: "self-dependency",
		},
		{
			name: "self parenting",
			request: interfaces.WorkRequest{
				RequestID: "request-1",
				Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
				Works:     []interfaces.Work{{Name: "first", WorkTypeID: "task"}},
				Relations: []interfaces.WorkRelation{{Type: interfaces.WorkRelationParentChild, SourceWorkName: "first", TargetWorkName: "first"}},
			},
			wantErr: "self-parenting",
		},
		{
			name: "duplicate parent child relation",
			request: interfaces.WorkRequest{
				RequestID: "request-1",
				Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
				Works:     []interfaces.Work{{Name: "parent", WorkTypeID: "task"}, {Name: "child", WorkTypeID: "task"}},
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
				Works:     []interfaces.Work{{Name: "parent", WorkTypeID: "task"}, {Name: "child", WorkTypeID: "task"}},
				Relations: []interfaces.WorkRelation{{
					Type:           interfaces.WorkRelationParentChild,
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
			request: interfaces.WorkRequest{
				RequestID: "request-1",
				Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
				Works:     []interfaces.Work{{Name: "first", WorkTypeID: "missing"}},
			},
			wantErr: "unknown work type",
		},
	})
}

type normalizeValidationTestCase struct {
	name    string
	request interfaces.WorkRequest
	wantErr string
}

func runNormalizeWorkRequestValidationTests(t *testing.T, tests []normalizeValidationTestCase) {
	t.Helper()

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
	if len(normalized[0].Content) != 1 {
		t.Fatalf("content count = %d, want 1", len(normalized[0].Content))
	}
	if normalized[0].Content[0].Type != interfaces.WorkContentPartTypeText || normalized[0].Content[0].Text != "plain text" {
		t.Fatalf("content = %#v, want canonical text part", normalized[0].Content)
	}
}

func TestNormalizeWorkRequest_PrefersExplicitContentForLegacyTextPayload(t *testing.T) {
	normalized, err := NormalizeWorkRequest(interfaces.WorkRequest{
		RequestID: "request-explicit-content",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{{
			Name:       "raw",
			WorkTypeID: "task",
			Content: []interfaces.WorkContentPart{
				{Type: interfaces.WorkContentPartTypeText, Text: "plain "},
				{Type: interfaces.WorkContentPartTypeText, Text: "text"},
			},
			Payload: "plain text",
		}},
	}, interfaces.WorkRequestNormalizeOptions{ValidWorkTypes: map[string]bool{"task": true}})
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
	normalized, err := NormalizeWorkRequest(interfaces.WorkRequest{
		RequestID: "request-content-only",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{{
			Name:       "raw",
			WorkTypeID: "task",
			Content: []interfaces.WorkContentPart{
				{Type: interfaces.WorkContentPartTypeText, Text: "alpha"},
				{Type: interfaces.WorkContentPartTypeText, Text: "\nbeta"},
			},
		}},
	}, interfaces.WorkRequestNormalizeOptions{ValidWorkTypes: map[string]bool{"task": true}})
	if err != nil {
		t.Fatalf("NormalizeWorkRequest: %v", err)
	}
	if string(normalized[0].Payload) != "alpha\nbeta" {
		t.Fatalf("payload = %q, want alpha\\nbeta", normalized[0].Payload)
	}
}

func TestNormalizeWorkRequest_RejectsConflictingExplicitContentAndPayload(t *testing.T) {
	_, err := NormalizeWorkRequest(interfaces.WorkRequest{
		RequestID: "request-content-conflict",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{{
			Name:       "raw",
			WorkTypeID: "task",
			Content: []interfaces.WorkContentPart{
				{Type: interfaces.WorkContentPartTypeText, Text: "plain text"},
			},
			Payload: "different text",
		}},
	}, interfaces.WorkRequestNormalizeOptions{ValidWorkTypes: map[string]bool{"task": true}})
	if err == nil || !strings.Contains(err.Error(), "payload conflicts with explicit content") {
		t.Fatalf("expected content conflict error, got %v", err)
	}
}

func TestNormalizeWorkRequest_RejectsPayloadAlongsideImageContent(t *testing.T) {
	_, err := NormalizeWorkRequest(interfaces.WorkRequest{
		RequestID: "request-image-conflict",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{{
			Name:       "raw",
			WorkTypeID: "task",
			Content: []interfaces.WorkContentPart{
				{Type: interfaces.WorkContentPartTypeImage, File: "fixtures/example.png"},
			},
			Payload: "caption",
		}},
	}, interfaces.WorkRequestNormalizeOptions{ValidWorkTypes: map[string]bool{"task": true}})
	if err == nil || !strings.Contains(err.Error(), "payload cannot be combined with image-only canonical content") {
		t.Fatalf("expected image/payload conflict error, got %v", err)
	}
}

func TestNormalizeWorkRequest_ExtendedContentKeepsLegacyTextProjection(t *testing.T) {
	normalized, err := NormalizeWorkRequest(interfaces.WorkRequest{
		RequestID: "request-model-content",
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{{
			Name:       "tts",
			WorkTypeID: "task",
			Content: []interfaces.WorkContentPart{
				{Type: interfaces.WorkContentPartTypeText, Text: "Synthesize"},
				{Type: interfaces.WorkContentPartTypeAudio, File: "artifacts/output.wav"},
				{Type: interfaces.WorkContentPartTypeJSON, JSON: json.RawMessage(`{"voice":"alloy"}`)},
			},
		}},
	}, interfaces.WorkRequestNormalizeOptions{ValidWorkTypes: map[string]bool{"task": true}})
	if err != nil {
		t.Fatalf("NormalizeWorkRequest: %v", err)
	}
	if len(normalized) != 1 {
		t.Fatalf("normalized count = %d, want 1", len(normalized))
	}
	if string(normalized[0].Payload) != "Synthesize" {
		t.Fatalf("payload = %q, want text-only legacy projection", normalized[0].Payload)
	}
	if len(normalized[0].Content) != 3 || normalized[0].Content[1].Type != interfaces.WorkContentPartTypeAudio || normalized[0].Content[2].Type != interfaces.WorkContentPartTypeJSON {
		t.Fatalf("content = %#v, want preserved extended content", normalized[0].Content)
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
