package factory

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestWorkRequestRecordFromSubmitRequests_UsesSharedTraceFallback(t *testing.T) {
	record := WorkRequestRecordFromSubmitRequests("request-record", "api", []interfaces.SubmitRequest{{
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

// pkgmaintcheck:ignore-cyclomatic-complexity this canonical batch-contract test keeps cross-item payload, relation, and trace assertions visible in one flow.
func TestWorkRequestFromSubmitRequests_PreservesCanonicalBatchContract(t *testing.T) {
	requests := []interfaces.SubmitRequest{
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
			Relations:                []interfaces.Relation{{Type: interfaces.RelationDependsOn, TargetWorkID: "work-2", RequiredState: "complete"}},
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
	if workRequest.Type != interfaces.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("work request type = %q, want %q", workRequest.Type, interfaces.WorkRequestTypeFactoryRequestBatch)
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

	first := workRequest.Works[0]
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

	second := workRequest.Works[1]
	if second.Name != "draft-2" {
		t.Fatalf("second work name = %q, want draft-2", second.Name)
	}
	if second.RequestID != "request-shared" {
		t.Fatalf("second request ID = %q, want request-shared", second.RequestID)
	}
	if second.CurrentChainingTraceID != "trace-second" {
		t.Fatalf("second current chaining trace ID = %q, want trace-second", second.CurrentChainingTraceID)
	}

	requests[0].Payload[0] = 'X'
	requests[0].Tags["scope"] = "mutated"
	requests[0].Relations[0].TargetWorkID = "mutated"
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
	requests := []interfaces.SubmitRequest{
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
	if workRequest.Type != interfaces.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("work request type = %q, want %q", workRequest.Type, interfaces.WorkRequestTypeFactoryRequestBatch)
	}
	if workRequest.RequestID != "" {
		t.Fatalf("work request ID = %q, want empty", workRequest.RequestID)
	}
	if len(workRequest.Works) != 0 {
		t.Fatalf("work count = %d, want 0", len(workRequest.Works))
	}
}
