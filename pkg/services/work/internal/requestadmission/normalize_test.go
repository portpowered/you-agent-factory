// backendsizecheck:ignore-file service-ownership migration preserves this consolidated surface until a dedicated responsibility split removes the exemption.
// pkgmaintcheck:ignore-file-lines service-ownership migration preserves this consolidated file; split responsibilities and remove this exemption.
package requestadmission

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
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
		{name: "conflicting aliases", current: "chain-1", legacy: "trace-1", wantErr: ErrConflictingWorkRequestTraceFields},
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

func TestSingleWorkTargetPreparationOwnsNormalization(t *testing.T) {
	t.Parallel()

	prepare := NewSingleWorkTargetPreparation()
	target, err := prepare(Request{
		RequestID: "request-1",
		Type:      RequestTypeFactoryRequestBatch,
		Works: []Work{{
			WorkID:     "work-1",
			Name:       "draft",
			WorkTypeID: "chapter",
			Payload:    "write it",
		}},
	})
	if err != nil {
		t.Fatalf("PrepareSingleWorkTarget: %v", err)
	}
	if target != (SingleWorkTarget{WorkID: "work-1", WorkTypeID: "chapter"}) {
		t.Fatalf("target = %#v", target)
	}
}

func TestSingleWorkTargetPreparationRejectsMultipleWorkItems(t *testing.T) {
	t.Parallel()

	_, err := NewSingleWorkTargetPreparation()(Request{
		RequestID: "request-1",
		Type:      RequestTypeFactoryRequestBatch,
		Works: []Work{
			{WorkID: "work-1", Name: "first", WorkTypeID: "chapter", Payload: "one"},
			{WorkID: "work-2", Name: "second", WorkTypeID: "chapter", Payload: "two"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "exactly one work item") {
		t.Fatalf("PrepareSingleWorkTarget error = %v, want exact-one validation", err)
	}
}

func TestSingleWorkTargetPreparationRejectsIdentityReconstruction(t *testing.T) {
	t.Parallel()

	_, err := NewSingleWorkTargetPreparation()(Request{
		Type: RequestTypeFactoryRequestBatch,
		Works: []Work{{
			Name:       "draft",
			WorkTypeID: "chapter",
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "Work ID is required") {
		t.Fatalf("PrepareSingleWorkTarget error = %v, want stable Work identity failure", err)
	}
}

func TestNormalizeWorkRequest_IndependentWorkItemsShareRequestAndTrace(t *testing.T) {
	request := Request{
		RequestID:              "request-1",
		CurrentChainingTraceID: "chain-request-1",
		Type:                   RequestTypeFactoryRequestBatch,
		Works: []Work{
			{Name: "first", WorkTypeID: "task", Payload: map[string]any{"title": "first"}},
			{Name: "second", WorkTypeID: "task", Payload: map[string]any{"title": "second"}},
		},
	}

	normalized, err := NormalizeWorkRequest(request, NormalizeOptions{
		ValidWorkTypes: map[string]bool{"task": true},
	})
	if err != nil {
		t.Fatalf("NormalizeWorkRequest: %v", err)
	}
	assertIndependentWorkIdentity(t, normalized)
	assertIndependentWorkMetadata(t, normalized)
}

func assertIndependentWorkIdentity(t *testing.T, normalized []SubmitRequest) {
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

func assertIndependentWorkMetadata(t *testing.T, normalized []SubmitRequest) {
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
	normalized := []SubmitRequest{
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
	request := Request{
		RequestID: "request-legacy-trace",
		Type:      RequestTypeFactoryRequestBatch,
		Works: []Work{
			{Name: "first", WorkTypeID: "task", TraceID: "trace-legacy"},
			{Name: "second", WorkTypeID: "task"},
		},
	}

	normalized, err := NormalizeWorkRequest(request, NormalizeOptions{
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
	request := Request{
		RequestID: "request-1",
		Type:      RequestTypeFactoryRequestBatch,
		Works: []Work{
			{Name: "build", WorkTypeID: "task", WorkID: "work-build", TraceID: "trace-batch"},
			{Name: "test", WorkTypeID: "task", WorkID: "work-test"},
		},
		Relations: []WorkRelation{{
			Type:           WorkRelationDependsOn,
			SourceWorkName: "test",
			TargetWorkName: "build",
			RequiredState:  "reviewed",
		}},
	}

	normalized, err := NormalizeWorkRequest(request, NormalizeOptions{
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
	if relation.Type != RelationDependsOn || relation.TargetWorkID != "work-build" || relation.RequiredState != "reviewed" {
		t.Fatalf("relation = %#v", relation)
	}
	if dependent.TraceID != "trace-batch" {
		t.Fatalf("dependent trace ID = %q, want trace-batch", dependent.TraceID)
	}
}

func TestNormalizeWorkRequest_ParentChildRelationTargetsParentAndCoexistsWithDependsOn(t *testing.T) {
	request := Request{
		RequestID: "request-parent-child-1",
		Type:      RequestTypeFactoryRequestBatch,
		Works: []Work{
			{Name: "parent", WorkTypeID: "task", WorkID: "work-parent", TraceID: "trace-parent-child"},
			{Name: "prerequisite", WorkTypeID: "task", WorkID: "work-prerequisite"},
			{Name: "child", WorkTypeID: "task", WorkID: "work-child"},
		},
		Relations: []WorkRelation{
			{
				Type:           WorkRelationParentChild,
				SourceWorkName: "child",
				TargetWorkName: "parent",
			},
			{
				Type:           WorkRelationDependsOn,
				SourceWorkName: "child",
				TargetWorkName: "prerequisite",
			},
		},
	}

	normalized, err := NormalizeWorkRequest(request, NormalizeOptions{
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
		case RelationParentChild:
			foundParentChild = true
			if relation.TargetWorkID != "work-parent" {
				t.Fatalf("parent-child target = %q, want work-parent", relation.TargetWorkID)
			}
		case RelationDependsOn:
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
	_, err := NormalizeWorkRequest(Request{
		RequestID: "request-parent-child-conflict",
		Type:      RequestTypeFactoryRequestBatch,
		Works: []Work{
			{Name: "parent-a", WorkTypeID: "story-set"},
			{Name: "parent-b", WorkTypeID: "story-set"},
			{Name: "story-a", WorkTypeID: "story"},
		},
		Relations: []WorkRelation{
			{Type: WorkRelationParentChild, SourceWorkName: "story-a", TargetWorkName: "parent-a"},
			{Type: WorkRelationParentChild, SourceWorkName: "story-a", TargetWorkName: "parent-b"},
		},
	}, NormalizeOptions{
		ValidWorkTypes: map[string]bool{"story-set": true, "story": true},
	})
	wantErr := `work_request: relations[1] assigns multiple PARENT_CHILD parents to "story-a" ("parent-a" and "parent-b")`
	if err == nil || err.Error() != wantErr {
		t.Fatalf("error = %v, want %q", err, wantErr)
	}
}

func TestNormalizeWorkRequest_RejectsDuplicateDependsOnRelationWithNormalizedRequiredState(t *testing.T) {
	_, err := NormalizeWorkRequest(Request{
		RequestID: "request-duplicate-depends-on",
		Type:      RequestTypeFactoryRequestBatch,
		Works: []Work{
			{Name: "build", WorkTypeID: "task"},
			{Name: "review", WorkTypeID: "task"},
		},
		Relations: []WorkRelation{
			{
				Type:           WorkRelationDependsOn,
				SourceWorkName: "review",
				TargetWorkName: "build",
				RequiredState:  "reviewed",
			},
			{
				Type:           WorkRelationDependsOn,
				SourceWorkName: "review",
				TargetWorkName: "build",
				RequiredState:  "reviewed",
			},
		},
	}, NormalizeOptions{
		ValidWorkTypes:    map[string]bool{"task": true},
		ValidStatesByType: map[string]map[string]bool{"task": {"reviewed": true, "complete": true}},
	})
	wantErr := `work_request: relations[1] duplicates relations[0] ("DEPENDS_ON" "review" -> "build" with requiredState "reviewed")`
	if err == nil || err.Error() != wantErr {
		t.Fatalf("error = %v, want %q", err, wantErr)
	}
}

func TestNormalizeWorkRequest_RejectsDuplicateParentChildRelationWithoutRequiredStateSuffix(t *testing.T) {
	_, err := NormalizeWorkRequest(Request{
		RequestID: "request-duplicate-parent-child",
		Type:      RequestTypeFactoryRequestBatch,
		Works: []Work{
			{Name: "parent", WorkTypeID: "task"},
			{Name: "child", WorkTypeID: "task"},
		},
		Relations: []WorkRelation{
			{Type: WorkRelationParentChild, SourceWorkName: "child", TargetWorkName: "parent"},
			{Type: WorkRelationParentChild, SourceWorkName: "child", TargetWorkName: "parent"},
		},
	}, NormalizeOptions{
		ValidWorkTypes: map[string]bool{"task": true},
	})
	wantErr := `work_request: relations[1] duplicates relations[0] ("PARENT_CHILD" "child" -> "parent")`
	if err == nil || err.Error() != wantErr {
		t.Fatalf("error = %v, want %q", err, wantErr)
	}
}

func TestNormalizeWorkRequest_FillsMissingWorkTypeFromContext(t *testing.T) {
	normalized, err := NormalizeWorkRequest(Request{
		RequestID: "request-1",
		Type:      RequestTypeFactoryRequestBatch,
		Works:     []Work{{Name: "inferred"}},
	}, NormalizeOptions{
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
	normalized, err := NormalizeWorkRequest(Request{
		RequestID: "request-state",
		Type:      RequestTypeFactoryRequestBatch,
		Works: []Work{{
			Name:       "draft",
			WorkTypeID: "task",
			State:      "queued",
		}},
	}, NormalizeOptions{
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
	_, err := NormalizeWorkRequest(Request{
		RequestID: "request-1",
		Type:      RequestTypeFactoryRequestBatch,
		Works:     []Work{{Name: "conflict", WorkTypeID: "other"}},
	}, NormalizeOptions{
		DefaultWorkTypeID: "task",
		ValidWorkTypes:    map[string]bool{"task": true, "other": true},
	})
	if err == nil || !strings.Contains(err.Error(), "conflicts with context work type") {
		t.Fatalf("expected work type conflict error, got %v", err)
	}
}

func TestNormalizeWorkRequest_RejectsUnknownExplicitState(t *testing.T) {
	_, err := NormalizeWorkRequest(Request{
		RequestID: "request-invalid-state",
		Type:      RequestTypeFactoryRequestBatch,
		Works: []Work{{
			Name:       "draft",
			WorkTypeID: "task",
			State:      "queued",
		}},
	}, NormalizeOptions{
		ValidWorkTypes:    map[string]bool{"task": true},
		ValidStatesByType: map[string]map[string]bool{"task": {"init": true, "complete": true}},
	})
	if err == nil || !strings.Contains(err.Error(), `references unknown state "queued"`) {
		t.Fatalf("expected state validation error, got %v", err)
	}
}

func TestNormalizeWorkRequest_RejectsUnknownDependencyRequiredState(t *testing.T) {
	_, err := NormalizeWorkRequest(Request{
		RequestID: "request-invalid-required-state",
		Type:      RequestTypeFactoryRequestBatch,
		Works: []Work{
			{Name: "draft", WorkTypeID: "task"},
			{Name: "review", WorkTypeID: "task"},
		},
		Relations: []WorkRelation{{
			Type:           WorkRelationDependsOn,
			SourceWorkName: "review",
			TargetWorkName: "draft",
			RequiredState:  "queued",
		}},
	}, NormalizeOptions{
		ValidWorkTypes:    map[string]bool{"task": true},
		ValidStatesByType: map[string]map[string]bool{"task": {"init": true, "complete": true}},
	})
	if err == nil || !strings.Contains(err.Error(), `references unknown requiredState "queued"`) {
		t.Fatalf("expected requiredState validation error, got %v", err)
	}
}

func TestNormalizeWorkRequest_RejectsDependencyCycle(t *testing.T) {
	_, err := NormalizeWorkRequest(Request{
		RequestID: "request-1",
		Type:      RequestTypeFactoryRequestBatch,
		Works: []Work{
			{Name: "first", WorkTypeID: "task"},
			{Name: "second", WorkTypeID: "task"},
		},
		Relations: []WorkRelation{
			{Type: WorkRelationDependsOn, SourceWorkName: "first", TargetWorkName: "second"},
			{Type: WorkRelationDependsOn, SourceWorkName: "second", TargetWorkName: "first"},
		},
	}, NormalizeOptions{ValidWorkTypes: map[string]bool{"task": true}})
	if err == nil || !strings.Contains(err.Error(), "dependency cycle") {
		t.Fatalf("expected dependency cycle error, got %v", err)
	}
}

func TestNormalizeWorkRequest_RejectsValidationFailures_WorkArrayAndEndpoints(t *testing.T) {
	runNormalizeWorkRequestValidationTests(t, []normalizeValidationTestCase{
		{
			name:    "empty work list",
			request: Request{RequestID: "request-1", Type: RequestTypeFactoryRequestBatch},
			wantErr: "works array must contain at least one item",
		},
		{
			name: "duplicate work names",
			request: Request{
				RequestID: "request-1",
				Type:      RequestTypeFactoryRequestBatch,
				Works:     []Work{{Name: "same", WorkTypeID: "task"}, {Name: "same", WorkTypeID: "task"}},
			},
			wantErr: "duplicate name",
		},
		{
			name: "missing source endpoint",
			request: Request{
				RequestID: "request-1",
				Type:      RequestTypeFactoryRequestBatch,
				Works:     []Work{{Name: "first", WorkTypeID: "task"}},
				Relations: []WorkRelation{{Type: WorkRelationDependsOn, TargetWorkName: "first"}},
			},
			wantErr: "missing sourceWorkName",
		},
		{
			name: "blank source endpoint",
			request: Request{
				RequestID: "request-1",
				Type:      RequestTypeFactoryRequestBatch,
				Works:     []Work{{Name: "first", WorkTypeID: "task"}},
				Relations: []WorkRelation{{Type: WorkRelationDependsOn, SourceWorkName: "   ", TargetWorkName: "first"}},
			},
			wantErr: "missing sourceWorkName",
		},
		{
			name: "missing target endpoint",
			request: Request{
				RequestID: "request-1",
				Type:      RequestTypeFactoryRequestBatch,
				Works:     []Work{{Name: "first", WorkTypeID: "task"}},
				Relations: []WorkRelation{{Type: WorkRelationDependsOn, SourceWorkName: "first"}},
			},
			wantErr: "missing targetWorkName",
		},
		{
			name: "unknown source endpoint",
			request: Request{
				RequestID: "request-1",
				Type:      RequestTypeFactoryRequestBatch,
				Works:     []Work{{Name: "first", WorkTypeID: "task"}},
				Relations: []WorkRelation{{Type: WorkRelationDependsOn, SourceWorkName: "missing", TargetWorkName: "first"}},
			},
			wantErr: "unknown sourceWorkName",
		},
		{
			name: "unknown target endpoint",
			request: Request{
				RequestID: "request-1",
				Type:      RequestTypeFactoryRequestBatch,
				Works:     []Work{{Name: "first", WorkTypeID: "task"}},
				Relations: []WorkRelation{{Type: WorkRelationDependsOn, SourceWorkName: "first", TargetWorkName: "missing"}},
			},
			wantErr: "unknown targetWorkName",
		},
	})
}

func TestNormalizeWorkRequest_RejectsValidationFailures_RelationSemantics(t *testing.T) {
	runNormalizeWorkRequestValidationTests(t, []normalizeValidationTestCase{
		{
			name: "unknown relation type",
			request: Request{
				RequestID: "request-1",
				Type:      RequestTypeFactoryRequestBatch,
				Works:     []Work{{Name: "first", WorkTypeID: "task"}, {Name: "second", WorkTypeID: "task"}},
				Relations: []WorkRelation{{
					Type:           WorkRelationType("INVALID"),
					SourceWorkName: "first",
					TargetWorkName: "second",
				}},
			},
			wantErr: "unsupported type",
		},
		{
			name: "self dependency",
			request: Request{
				RequestID: "request-1",
				Type:      RequestTypeFactoryRequestBatch,
				Works:     []Work{{Name: "first", WorkTypeID: "task"}},
				Relations: []WorkRelation{{Type: WorkRelationDependsOn, SourceWorkName: "first", TargetWorkName: "first"}},
			},
			wantErr: "self-dependency",
		},
		{
			name: "self parenting",
			request: Request{
				RequestID: "request-1",
				Type:      RequestTypeFactoryRequestBatch,
				Works:     []Work{{Name: "first", WorkTypeID: "task"}},
				Relations: []WorkRelation{{Type: WorkRelationParentChild, SourceWorkName: "first", TargetWorkName: "first"}},
			},
			wantErr: "self-parenting",
		},
		{
			name: "duplicate parent child relation",
			request: Request{
				RequestID: "request-1",
				Type:      RequestTypeFactoryRequestBatch,
				Works:     []Work{{Name: "parent", WorkTypeID: "task"}, {Name: "child", WorkTypeID: "task"}},
				Relations: []WorkRelation{
					{Type: WorkRelationParentChild, SourceWorkName: "child", TargetWorkName: "parent"},
					{Type: WorkRelationParentChild, SourceWorkName: "child", TargetWorkName: "parent"},
				},
			},
			wantErr: "duplicates relations[0]",
		},
		{
			name: "parent child required state",
			request: Request{
				RequestID: "request-1",
				Type:      RequestTypeFactoryRequestBatch,
				Works:     []Work{{Name: "parent", WorkTypeID: "task"}, {Name: "child", WorkTypeID: "task"}},
				Relations: []WorkRelation{{
					Type:           WorkRelationParentChild,
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
			request: Request{
				RequestID: "request-1",
				Type:      RequestTypeFactoryRequestBatch,
				Works:     []Work{{Name: "first", WorkTypeID: "missing"}},
			},
			wantErr: "unknown work type",
		},
	})
}

type normalizeValidationTestCase struct {
	name    string
	request Request
	wantErr string
}

func runNormalizeWorkRequestValidationTests(t *testing.T, tests []normalizeValidationTestCase) {
	t.Helper()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NormalizeWorkRequest(tc.request, NormalizeOptions{
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
	normalized, err := NormalizeWorkRequest(Request{
		RequestID: "request-1",
		Type:      RequestTypeFactoryRequestBatch,
		Works:     []Work{{Name: "raw", WorkTypeID: "task", Payload: raw}},
	}, NormalizeOptions{ValidWorkTypes: map[string]bool{"task": true}})
	if err != nil {
		t.Fatalf("NormalizeWorkRequest: %v", err)
	}
	if string(normalized[0].Payload) != `{"key":"value"}` {
		t.Fatalf("payload = %s", normalized[0].Payload)
	}
}

func TestNormalizeWorkRequest_ExecutionIDTagPropagatesToSubmitRequest(t *testing.T) {
	normalized, err := NormalizeWorkRequest(Request{
		RequestID: "request-exec-id",
		Type:      RequestTypeFactoryRequestBatch,
		Works: []Work{{
			Name:        "chapter",
			WorkTypeID:  "chapter",
			ExecutionID: "exec-guard-propagation",
		}},
	}, NormalizeOptions{ValidWorkTypes: map[string]bool{"chapter": true}})
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
	normalized, err := NormalizeWorkRequest(Request{
		RequestID: "request-string-payload",
		Type:      RequestTypeFactoryRequestBatch,
		Works:     []Work{{Name: "raw", WorkTypeID: "task", Payload: "plain text"}},
	}, NormalizeOptions{ValidWorkTypes: map[string]bool{"task": true}})
	if err != nil {
		t.Fatalf("NormalizeWorkRequest: %v", err)
	}
	if string(normalized[0].Payload) != "plain text" {
		t.Fatalf("payload = %q, want plain text", normalized[0].Payload)
	}
	if len(normalized[0].Content) != 1 {
		t.Fatalf("content count = %d, want 1", len(normalized[0].Content))
	}
	if normalized[0].Content[0].Type != ContentPartTypeText || normalized[0].Content[0].Text != "plain text" {
		t.Fatalf("content = %#v, want canonical text part", normalized[0].Content)
	}
}

func TestNormalizeWorkRequest_PrefersExplicitContentForLegacyTextPayload(t *testing.T) {
	normalized, err := NormalizeWorkRequest(Request{
		RequestID: "request-explicit-content",
		Type:      RequestTypeFactoryRequestBatch,
		Works: []Work{{
			Name:       "raw",
			WorkTypeID: "task",
			Content: []ContentPart{
				{Type: ContentPartTypeText, Text: "plain "},
				{Type: ContentPartTypeText, Text: "text"},
			},
			Payload: "plain text",
		}},
	}, NormalizeOptions{ValidWorkTypes: map[string]bool{"task": true}})
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
	normalized, err := NormalizeWorkRequest(Request{
		RequestID: "request-content-only",
		Type:      RequestTypeFactoryRequestBatch,
		Works: []Work{{
			Name:       "raw",
			WorkTypeID: "task",
			Content: []ContentPart{
				{Type: ContentPartTypeText, Text: "alpha"},
				{Type: ContentPartTypeText, Text: "\nbeta"},
			},
		}},
	}, NormalizeOptions{ValidWorkTypes: map[string]bool{"task": true}})
	if err != nil {
		t.Fatalf("NormalizeWorkRequest: %v", err)
	}
	if string(normalized[0].Payload) != "alpha\nbeta" {
		t.Fatalf("payload = %q, want alpha\\nbeta", normalized[0].Payload)
	}
}

func TestNormalizeWorkRequest_RejectsConflictingExplicitContentAndPayload(t *testing.T) {
	_, err := NormalizeWorkRequest(Request{
		RequestID: "request-content-conflict",
		Type:      RequestTypeFactoryRequestBatch,
		Works: []Work{{
			Name:       "raw",
			WorkTypeID: "task",
			Content: []ContentPart{
				{Type: ContentPartTypeText, Text: "plain text"},
			},
			Payload: "different text",
		}},
	}, NormalizeOptions{ValidWorkTypes: map[string]bool{"task": true}})
	if err == nil || !strings.Contains(err.Error(), "payload conflicts with explicit content") {
		t.Fatalf("expected content conflict error, got %v", err)
	}
}

func TestNormalizeWorkRequest_RejectsPayloadAlongsideImageContent(t *testing.T) {
	_, err := NormalizeWorkRequest(Request{
		RequestID: "request-image-conflict",
		Type:      RequestTypeFactoryRequestBatch,
		Works: []Work{{
			Name:       "raw",
			WorkTypeID: "task",
			Content: []ContentPart{
				{Type: ContentPartTypeImage, File: "fixtures/example.png"},
			},
			Payload: "caption",
		}},
	}, NormalizeOptions{ValidWorkTypes: map[string]bool{"task": true}})
	if err == nil || !strings.Contains(err.Error(), "payload cannot be combined with image-only canonical content") {
		t.Fatalf("expected image/payload conflict error, got %v", err)
	}
}

func TestNormalizeWorkRequest_ExtendedContentKeepsLegacyTextProjection(t *testing.T) {
	normalized, err := NormalizeWorkRequest(Request{
		RequestID: "request-model-content",
		Type:      RequestTypeFactoryRequestBatch,
		Works: []Work{{
			Name:       "tts",
			WorkTypeID: "task",
			Content: []ContentPart{
				{Type: ContentPartTypeText, Text: "Synthesize"},
				{Type: ContentPartTypeAudio, File: "artifacts/output.wav"},
				{Type: ContentPartTypeJSON, JSON: json.RawMessage(`{"voice":"alloy"}`)},
			},
		}},
	}, NormalizeOptions{ValidWorkTypes: map[string]bool{"task": true}})
	if err != nil {
		t.Fatalf("NormalizeWorkRequest: %v", err)
	}
	if len(normalized) != 1 {
		t.Fatalf("normalized count = %d, want 1", len(normalized))
	}
	if string(normalized[0].Payload) != "Synthesize" {
		t.Fatalf("payload = %q, want text-only legacy projection", normalized[0].Payload)
	}
	if len(normalized[0].Content) != 3 || normalized[0].Content[1].Type != ContentPartTypeAudio || normalized[0].Content[2].Type != ContentPartTypeJSON {
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
	normalized, err := NormalizeWorkRequest(Request{
		RequestID: "request-legacy-file",
		Type:      RequestTypeFactoryRequestBatch,
		Works: []Work{{
			Name:       "raw",
			WorkTypeID: "task",
			Content: []ContentPart{
				{Type: ContentPartTypeImage, File: "fixtures/example.png"},
			},
		}},
	}, NormalizeOptions{ValidWorkTypes: map[string]bool{"task": true}})
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

func TestNormalizeWorkRequest_UsesInjectedOpaqueIdentityForMissingRequestID(t *testing.T) {
	generated := 0
	normalized, err := NormalizeWorkRequest(Request{
		Type:  RequestTypeFactoryRequestBatch,
		Works: []Work{{Name: "work", WorkTypeID: "task"}},
	}, NormalizeOptions{
		ValidWorkTypes: map[string]bool{"task": true},
		IDGenerator: func() string {
			generated++
			return "opaque-id"
		},
	})
	if err != nil {
		t.Fatalf("NormalizeWorkRequest: %v", err)
	}
	if generated != 1 {
		t.Fatalf("ID generator calls = %d, want 1", generated)
	}
	if normalized[0].RequestID != "request-opaque-id" || normalized[0].TraceID != "trace-request-opaque-id" {
		t.Fatalf("generated identities = (%q, %q)", normalized[0].RequestID, normalized[0].TraceID)
	}
}

func TestNormalizeWorkRequest_FailsClosedWithoutRequiredIDGenerator(t *testing.T) {
	_, err := NormalizeWorkRequest(Request{
		Type:  RequestTypeFactoryRequestBatch,
		Works: []Work{{Name: "work", WorkTypeID: "task"}},
	}, NormalizeOptions{ValidWorkTypes: map[string]bool{"task": true}})
	if err == nil || !strings.Contains(err.Error(), "ID generator is required") {
		t.Fatalf("error = %v, want missing ID generator failure", err)
	}
}

func TestNormalizeWorkRequest_DoesNotGenerateWhenRequestIdentityIsSupplied(t *testing.T) {
	normalized, err := NormalizeWorkRequest(Request{
		RequestID: "request-customer",
		Type:      RequestTypeFactoryRequestBatch,
		Works:     []Work{{Name: "work", WorkTypeID: "task"}},
	}, NormalizeOptions{
		ValidWorkTypes: map[string]bool{"task": true},
		IDGenerator: func() string {
			t.Fatal("ID generator called for an identified request")
			return "unreachable"
		},
	})
	if err != nil {
		t.Fatalf("NormalizeWorkRequest: %v", err)
	}
	if normalized[0].RequestID != "request-customer" || normalized[0].TraceID != "trace-request-customer" {
		t.Fatalf("preserved identities = (%q, %q)", normalized[0].RequestID, normalized[0].TraceID)
	}
}

// The FactoryRequestBatch scenarios were relocated from
// tests/functional/guards_batch/factory_request_batch_test.go. They exercise
// Work-owned normalization policy directly, so their canonical home is the
// Work owner rather than a functional test that constructs service internals.
func TestFactoryRequestBatch_InvalidStructureRejected(t *testing.T) {
	for _, tc := range []struct {
		name, payload, wantErr string
	}{
		{"empty work array", `{"requestId":"invalid-1","type":"FACTORY_REQUEST_BATCH","works":[]}`, "works array must contain at least one item"},
		{"missing name field", `{"requestId":"invalid-2","type":"FACTORY_REQUEST_BATCH","works":[{"workTypeName":"task"}]}`, "missing required name"},
		{"missing work type field", `{"requestId":"invalid-3","type":"FACTORY_REQUEST_BATCH","works":[{"name":"foo"}]}`, "missing workTypeName"},
		{"duplicate work names", `{"requestId":"invalid-4","type":"FACTORY_REQUEST_BATCH","works":[{"workTypeName":"task","name":"dup"},{"workTypeName":"task","name":"dup"}]}`, "duplicate name"},
		{"unknown work type", `{"requestId":"invalid-5","type":"FACTORY_REQUEST_BATCH","works":[{"workTypeName":"nonexistent","name":"foo"}]}`, "unknown work type"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertInvalidFactoryRequestBatchPayload(t, tc.payload, tc.wantErr)
		})
	}
}

func TestFactoryRequestBatch_InvalidRelationsRejected(t *testing.T) {
	for _, tc := range []struct {
		name, payload, wantErr string
	}{
		{"unknown source in relation", `{"requestId":"invalid-6","type":"FACTORY_REQUEST_BATCH","works":[{"workTypeName":"task","name":"a"}],"relations":[{"type":"DEPENDS_ON","sourceWorkName":"missing","targetWorkName":"a"}]}`, "unknown sourceWorkName"},
		{"unknown target in relation", `{"requestId":"invalid-7","type":"FACTORY_REQUEST_BATCH","works":[{"workTypeName":"task","name":"a"}],"relations":[{"type":"DEPENDS_ON","sourceWorkName":"a","targetWorkName":"missing"}]}`, "unknown targetWorkName"},
		{"self-referencing dependency", `{"requestId":"invalid-8","type":"FACTORY_REQUEST_BATCH","works":[{"workTypeName":"task","name":"a"}],"relations":[{"type":"DEPENDS_ON","sourceWorkName":"a","targetWorkName":"a"}]}`, "self-dependency"},
		{"self-parenting relation", `{"requestId":"invalid-9","type":"FACTORY_REQUEST_BATCH","works":[{"workTypeName":"task","name":"a"}],"relations":[{"type":"PARENT_CHILD","sourceWorkName":"a","targetWorkName":"a"}]}`, "self-parenting"},
		{"duplicate parent-child relation", `{"requestId":"invalid-10","type":"FACTORY_REQUEST_BATCH","works":[{"workTypeName":"task","name":"parent"},{"workTypeName":"task","name":"child"}],"relations":[{"type":"PARENT_CHILD","sourceWorkName":"child","targetWorkName":"parent"},{"type":"PARENT_CHILD","sourceWorkName":"child","targetWorkName":"parent"}]}`, "duplicates relations[0]"},
		{"invalid dependency required_state", `{"requestId":"invalid-11","type":"FACTORY_REQUEST_BATCH","works":[{"workTypeName":"task","name":"draft"},{"workTypeName":"task","name":"review"}],"relations":[{"type":"DEPENDS_ON","sourceWorkName":"review","targetWorkName":"draft","requiredState":"queued"}]}`, "unknown requiredState"},
		{"unsupported relation type", `{"requestId":"invalid-12","type":"FACTORY_REQUEST_BATCH","works":[{"workTypeName":"task","name":"a"},{"workTypeName":"task","name":"b"}],"relations":[{"type":"INVALID","sourceWorkName":"a","targetWorkName":"b"}]}`, "unsupported type"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertInvalidFactoryRequestBatchPayload(t, tc.payload, tc.wantErr)
		})
	}
}

func TestFactoryRequestBatch_InvalidJSONRejected(t *testing.T) {
	assertInvalidFactoryRequestBatchPayload(t, `{not json}`, "invalid character")
}

func TestFactoryRequestBatch_BatchSubmissionAtomic(t *testing.T) {
	validWorkTypes := map[string]bool{"task": true}
	invalid := Request{
		RequestID: "request-atomic-invalid", Type: RequestTypeFactoryRequestBatch,
		Works: []Work{{WorkTypeID: "task", Name: "valid-item"}, {WorkTypeID: "task"}},
	}
	if _, err := NormalizeWorkRequest(invalid, NormalizeOptions{ValidWorkTypes: validWorkTypes}); err == nil {
		t.Fatal("expected validation error for batch with invalid item, got nil")
	}
	valid := Request{
		RequestID: "request-atomic-1", Type: RequestTypeFactoryRequestBatch,
		Works: []Work{{WorkTypeID: "task", Name: "item-1"}, {WorkTypeID: "task", Name: "item-2"}, {WorkTypeID: "task", Name: "item-3"}},
	}
	expanded, err := NormalizeWorkRequest(valid, NormalizeOptions{ValidWorkTypes: validWorkTypes})
	if err != nil {
		t.Fatalf("NormalizeWorkRequest failed: %v", err)
	}
	if len(expanded) != 3 {
		t.Fatalf("expected 3 expanded requests, got %d", len(expanded))
	}
	for _, request := range expanded {
		if request.WorkID == "" || !strings.HasPrefix(request.WorkID, "batch-request-atomic-1-") {
			t.Fatalf("expanded WorkID = %q, want batch-request-atomic-1- prefix", request.WorkID)
		}
	}
}

func assertInvalidFactoryRequestBatchPayload(t *testing.T, payload, wantErr string) {
	t.Helper()
	var request Request
	err := json.Unmarshal([]byte(payload), &request)
	if err == nil {
		_, err = NormalizeWorkRequest(request, NormalizeOptions{
			ValidWorkTypes: map[string]bool{"task": true},
			ValidStatesByType: map[string]map[string]bool{
				"task": {"init": true, "complete": true},
			},
		})
	}
	if err == nil || !strings.Contains(err.Error(), wantErr) {
		t.Fatalf("expected error containing %q, got %v", wantErr, err)
	}
}

func findSubmitRequest(t *testing.T, requests []SubmitRequest, name string) SubmitRequest {
	t.Helper()
	for _, request := range requests {
		if request.Name == name {
			return request
		}
	}
	t.Fatalf("submit request named %q not found in %#v", name, requests)
	return SubmitRequest{}
}
