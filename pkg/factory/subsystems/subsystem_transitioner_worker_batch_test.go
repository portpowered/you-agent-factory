package subsystems

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
)

func TestTransitioner_WorkerEmittedGeneratedSubmissionBatchCreatesGeneratedWork(t *testing.T) {
	now := time.Date(2026, time.April, 16, 22, 0, 0, 0, time.UTC)
	net := workerBatchTestNet()
	transitioner := NewTransitioner(net, nil, WithTransitionerClock(func() time.Time { return now }))
	output := `{"request":{"type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"child","tags":{"priority":"high"}},{"name":"review","workTypeName":"child"}],"relations":[{"type":"DEPENDS_ON","sourceWorkName":"review","targetWorkName":"draft"}]}}`
	result := executeWorkerBatchTransition(t, transitioner, workerBatchSnapshot(output))
	batch, requestID := assertGeneratedWorkerBatchMetadata(t, result)
	normalized := normalizeGeneratedWorkerBatch(t, batch)
	first := assertGeneratedWorkerBatchSubmissions(t, requestID, batch.Metadata.Source, normalized)
	assertRepeatedGeneratedWorkerBatchRequestID(t, transitioner, output, requestID)
	assertGeneratedWorkerBatchOutcome(t, result, first, normalized[1])
}

func TestTransitioner_WorkerEmittedGeneratedSubmissionBatchPreservesCanonicalChainingTrace(t *testing.T) {
	now := time.Date(2026, time.April, 16, 22, 10, 0, 0, time.UTC)
	net := workerBatchTestNet()
	transitioner := NewTransitioner(net, nil, WithTransitionerClock(func() time.Time { return now }))
	output := `{"request":{"type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"child"}]}}`
	snapshot := workerBatchSnapshot(output)
	snapshot.Dispatches["dispatch-1"].ConsumedTokens[0].Color.CurrentChainingTraceID = "chain-source"
	snapshot.Dispatches["dispatch-1"].ConsumedTokens[0].Color.TraceID = "trace-source"

	result := executeWorkerBatchTransition(t, transitioner, snapshot)
	batch := result.GeneratedBatches[0]
	normalized, err := factory.NormalizeGeneratedSubmissionBatch(batch, interfaces.WorkRequestNormalizeOptions{
		ValidWorkTypes: map[string]bool{"task": true, "child": true},
	})
	if err != nil {
		t.Fatalf("NormalizeGeneratedSubmissionBatch: %v", err)
	}
	if len(normalized) != 1 {
		t.Fatalf("normalized submissions = %d, want 1", len(normalized))
	}
	if normalized[0].CurrentChainingTraceID != "chain-source" {
		t.Fatalf("generated current chaining trace ID = %q, want chain-source", normalized[0].CurrentChainingTraceID)
	}
	if normalized[0].ChainingTraceDepth != 2 {
		t.Fatalf("generated chaining trace depth = %d, want 2", normalized[0].ChainingTraceDepth)
	}
	if len(normalized[0].PreviousChainingTraceIDs) != 1 || normalized[0].PreviousChainingTraceIDs[0] != "chain-source" {
		t.Fatalf("generated previous chaining trace IDs = %#v, want [chain-source]", normalized[0].PreviousChainingTraceIDs)
	}
	if normalized[0].TraceID != "trace-source" {
		t.Fatalf("generated legacy trace ID = %q, want trace-source", normalized[0].TraceID)
	}
}

func executeWorkerBatchTransition(
	t *testing.T,
	transitioner *TransitionerSubsystem,
	snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net],
) *interfaces.TickResult {
	t.Helper()

	result, err := transitioner.Execute(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected transitioner result")
	}
	return result
}

func assertGeneratedWorkerBatchMetadata(t *testing.T, result *interfaces.TickResult) (interfaces.GeneratedSubmissionBatch, string) {
	t.Helper()

	if len(result.Mutations) != 0 {
		t.Fatalf("mutation count = %d, want engine-owned generated work only", len(result.Mutations))
	}
	if len(result.GeneratedBatches) != 1 {
		t.Fatalf("generated batches = %d, want 1", len(result.GeneratedBatches))
	}
	batch := result.GeneratedBatches[0]
	requestID := batch.Request.RequestID
	if requestID == "" {
		t.Fatal("expected deterministic generated request ID")
	}
	if batch.Metadata.Source != "worker-output:dispatch-1" {
		t.Fatalf("batch source = %q, want worker-output:dispatch-1", batch.Metadata.Source)
	}
	return batch, requestID
}

func normalizeGeneratedWorkerBatch(t *testing.T, batch interfaces.GeneratedSubmissionBatch) []interfaces.SubmitRequest {
	t.Helper()

	normalized, err := factory.NormalizeGeneratedSubmissionBatch(batch, interfaces.WorkRequestNormalizeOptions{
		ValidWorkTypes: map[string]bool{"task": true, "child": true},
	})
	if err != nil {
		t.Fatalf("NormalizeGeneratedSubmissionBatch: %v", err)
	}
	return normalized
}

func assertGeneratedWorkerBatchSubmissions(
	t *testing.T,
	requestID string,
	source string,
	normalized []interfaces.SubmitRequest,
) interfaces.SubmitRequest {
	t.Helper()

	record := factory.WorkRequestRecordFromSubmitRequests(requestID, source, normalized)
	if len(record.Relations) != 1 {
		t.Fatalf("request relation count = %d, want 1", len(record.Relations))
	}
	if len(normalized) != 2 {
		t.Fatalf("normalized submissions = %d, want 2", len(normalized))
	}

	first := normalized[0]
	if first.RequestID != requestID {
		t.Fatalf("generated request ID = %q, want %q", first.RequestID, requestID)
	}
	if first.TraceID != "trace-source" || first.CurrentChainingTraceID != "trace-source" {
		t.Fatalf("first generated trace fields = %#v, want trace-source", first)
	}
	if first.ChainingTraceDepth != 2 {
		t.Fatalf("first generated chaining trace depth = %d, want 2", first.ChainingTraceDepth)
	}
	if len(first.PreviousChainingTraceIDs) != 1 || first.PreviousChainingTraceIDs[0] != "trace-source" {
		t.Fatalf("generated previous chaining trace IDs = %#v, want [trace-source]", first.PreviousChainingTraceIDs)
	}
	if first.Tags["tenant"] != "port" || first.Tags["priority"] != "high" {
		t.Fatalf("generated tags = %#v, want source and item tags", first.Tags)
	}
	if first.Tags["_parent_work_id"] != "work-source" || first.Tags["_parent_request_id"] != "request-source" {
		t.Fatalf("generated lineage tags = %#v", first.Tags)
	}
	if first.Tags["_source_dispatch_id"] != "dispatch-1" || first.Tags["_source_transition_id"] != "t1" {
		t.Fatalf("generated execution tags = %#v", first.Tags)
	}
	return first
}

func assertRepeatedGeneratedWorkerBatchRequestID(
	t *testing.T,
	transitioner *TransitionerSubsystem,
	output string,
	requestID string,
) {
	t.Helper()

	repeated := executeWorkerBatchTransition(t, transitioner, workerBatchSnapshot(output))
	if repeated.GeneratedBatches[0].Request.RequestID != requestID {
		t.Fatalf("generated request ID = %q, want deterministic %q", repeated.GeneratedBatches[0].Request.RequestID, requestID)
	}
}

func assertGeneratedWorkerBatchOutcome(
	t *testing.T,
	result *interfaces.TickResult,
	first interfaces.SubmitRequest,
	second interfaces.SubmitRequest,
) {
	t.Helper()

	if second.CurrentChainingTraceID != "trace-source" {
		t.Fatalf("second generated current chaining trace ID = %q, want trace-source", second.CurrentChainingTraceID)
	}
	if second.ChainingTraceDepth != 2 {
		t.Fatalf("second generated chaining trace depth = %d, want 2", second.ChainingTraceDepth)
	}
	if len(second.PreviousChainingTraceIDs) != 1 || second.PreviousChainingTraceIDs[0] != "trace-source" {
		t.Fatalf("second generated previous chaining trace IDs = %#v, want [trace-source]", second.PreviousChainingTraceIDs)
	}
	if len(second.Relations) != 1 || second.Relations[0].TargetWorkID != first.WorkID {
		t.Fatalf("generated dependency relation = %#v, want target %q", second.Relations, first.WorkID)
	}
	if result.CompletedDispatches[0].Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("completed outcome = %s, want ACCEPTED", result.CompletedDispatches[0].Outcome)
	}
}

func TestTransitioner_WorkerEmittedFactoryRequestBatchReleasesConsumedResources(t *testing.T) {
	now := time.Date(2026, time.April, 17, 0, 0, 0, 0, time.UTC)
	net := workerBatchTestNet()
	net.Places["agent-slot:available"] = &petri.Place{ID: "agent-slot:available", TypeID: "agent-slot", State: "available"}
	net.Resources = map[string]*state.ResourceDef{
		"agent-slot": {ID: "agent-slot", Capacity: 1},
	}
	net.Transitions["t1"].InputArcs = []petri.Arc{
		{ID: "task-in", PlaceID: "task:init"},
		{ID: "slot-in", PlaceID: "agent-slot:available"},
	}
	net.Transitions["t1"].OutputArcs = []petri.Arc{
		{ID: "accepted", PlaceID: "task:complete"},
		{ID: "slot-out", PlaceID: "agent-slot:available"},
	}
	transitioner := NewTransitioner(net, nil, WithTransitionerClock(func() time.Time { return now }))
	output := `{"request":{"type":"FACTORY_REQUEST_BATCH","works":[{"name":"follow-up","workTypeName":"child"}]}}`
	snapshot := workerBatchSnapshot(output)
	snapshot.Dispatches["dispatch-1"].ConsumedTokens = append(snapshot.Dispatches["dispatch-1"].ConsumedTokens, interfaces.Token{
		ID:        "agent-slot:resource:0",
		PlaceID:   "agent-slot:available",
		CreatedAt: now.Add(-time.Hour),
		EnteredAt: now.Add(-time.Hour),
		Color: interfaces.TokenColor{
			WorkID:     "agent-slot:0",
			WorkTypeID: "agent-slot",
			DataType:   interfaces.DataTypeResource,
		},
	})

	result, err := transitioner.Execute(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected transitioner result")
	}

	var generatedWork int
	var releasedResource *interfaces.Token
	for i := range result.Mutations {
		mutation := result.Mutations[i]
		if mutation.NewToken == nil {
			continue
		}
		switch mutation.NewToken.Color.DataType {
		case interfaces.DataTypeWork:
			if mutation.ToPlace == "child:init" {
				generatedWork++
			}
			if mutation.ToPlace == "task:complete" {
				t.Fatalf("worker-emitted batch should replace normal accepted work output, got mutation %#v", mutation)
			}
		case interfaces.DataTypeResource:
			if mutation.ToPlace == "agent-slot:available" {
				releasedResource = mutation.NewToken
			}
		}
	}
	if generatedWork != 0 {
		t.Fatalf("generated work mutations = %d, want engine-owned generated work", generatedWork)
	}
	if releasedResource == nil {
		t.Fatalf("resource release mutation missing from %#v", result.Mutations)
	}
	if releasedResource.ID != "agent-slot:resource:0" {
		t.Fatalf("released resource token ID = %q, want consumed token identity", releasedResource.ID)
	}
	if !releasedResource.EnteredAt.Equal(now) {
		t.Fatalf("released resource EnteredAt = %v, want %v", releasedResource.EnteredAt, now)
	}
	if len(result.GeneratedBatches) != 1 {
		t.Fatalf("generated batches = %d, want 1", len(result.GeneratedBatches))
	}
}

func TestTransitioner_AcceptedTransitionReleasesAllConsumedResourceUnitsForCardinalityNArc(t *testing.T) {
	now := time.Date(2026, time.April, 17, 2, 0, 0, 0, time.UTC)
	net := workerBatchTestNet()
	net.Places["agent-slot:available"] = &petri.Place{ID: "agent-slot:available", TypeID: "agent-slot", State: "available"}
	net.Resources = map[string]*state.ResourceDef{
		"agent-slot": {ID: "agent-slot", Capacity: 2},
	}
	net.Transitions["t1"].InputArcs = []petri.Arc{
		{ID: "task-in", PlaceID: "task:init"},
		{ID: "slot-in", PlaceID: "agent-slot:available", Cardinality: petri.ArcCardinality{Mode: petri.CardinalityN, Count: 2}},
	}
	net.Transitions["t1"].OutputArcs = []petri.Arc{
		{ID: "accepted", PlaceID: "task:complete"},
		{ID: "slot-out", PlaceID: "agent-slot:available", Cardinality: petri.ArcCardinality{Mode: petri.CardinalityN, Count: 2}},
	}

	transitioner := NewTransitioner(net, nil, WithTransitionerClock(func() time.Time { return now }))
	snapshot := workerBatchSnapshot("accepted")
	snapshot.Dispatches["dispatch-1"].ConsumedTokens = append(snapshot.Dispatches["dispatch-1"].ConsumedTokens,
		interfaces.Token{
			ID:        "agent-slot:resource:0",
			PlaceID:   "agent-slot:available",
			CreatedAt: now.Add(-time.Hour),
			EnteredAt: now.Add(-time.Hour),
			Color: interfaces.TokenColor{
				WorkID:     "agent-slot:0",
				WorkTypeID: "agent-slot",
				DataType:   interfaces.DataTypeResource,
			},
		},
		interfaces.Token{
			ID:        "agent-slot:resource:1",
			PlaceID:   "agent-slot:available",
			CreatedAt: now.Add(-time.Hour),
			EnteredAt: now.Add(-time.Hour),
			Color: interfaces.TokenColor{
				WorkID:     "agent-slot:1",
				WorkTypeID: "agent-slot",
				DataType:   interfaces.DataTypeResource,
			},
		},
	)

	result, err := transitioner.Execute(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected transitioner result")
	}

	releasedIDs := make([]string, 0, 2)
	for i := range result.Mutations {
		mutation := result.Mutations[i]
		if mutation.NewToken == nil || mutation.NewToken.Color.DataType != interfaces.DataTypeResource {
			continue
		}
		if mutation.ToPlace != "agent-slot:available" {
			t.Fatalf("resource release ToPlace = %q, want agent-slot:available", mutation.ToPlace)
		}
		releasedIDs = append(releasedIDs, mutation.NewToken.ID)
	}
	if !reflect.DeepEqual(releasedIDs, []string{"agent-slot:resource:0", "agent-slot:resource:1"}) {
		t.Fatalf("released resource IDs = %#v, want both consumed resource units restored", releasedIDs)
	}
}

func TestTransitioner_RawWorkerEmittedFactoryRequestBatchRoutesAsAcceptedOutput(t *testing.T) {
	now := time.Date(2026, time.April, 18, 1, 0, 0, 0, time.UTC)
	net := workerBatchTestNet()
	transitioner := NewTransitioner(net, nil, WithTransitionerClock(func() time.Time { return now }))
	output := `{"type":"FACTORY_REQUEST_BATCH","works":[{"name":"raw","work_type_name":"child"}]}`
	snapshot := workerBatchSnapshot(output)

	result, err := transitioner.Execute(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected transitioner result")
	}
	if len(result.GeneratedBatches) != 0 {
		t.Fatalf("generated batches = %d, want raw worker output treated as ordinary output", len(result.GeneratedBatches))
	}
	if len(result.Mutations) != 1 || result.Mutations[0].ToPlace != "task:complete" {
		t.Fatalf("mutations = %#v, want ordinary accepted-output mutation", result.Mutations)
	}
	token := result.Mutations[0].NewToken
	if token == nil {
		t.Fatal("accepted output mutation missing token")
	}
	if string(token.Color.Payload) != output {
		t.Fatalf("accepted output payload = %q, want raw JSON", token.Color.Payload)
	}
	if result.CompletedDispatches[0].Outcome != interfaces.OutcomeAccepted {
		t.Fatalf("completed outcome = %s, want ACCEPTED", result.CompletedDispatches[0].Outcome)
	}
}

func TestTransitioner_WorkerEmittedGeneratedSubmissionBatchUsesBatchMetadataSource(t *testing.T) {
	now := time.Date(2026, time.April, 18, 0, 0, 0, 0, time.UTC)
	net := workerBatchTestNet()
	transitioner := NewTransitioner(net, nil, WithTransitionerClock(func() time.Time { return now }))
	output := `{"request":{"requestId":"metadata-request","type":"FACTORY_REQUEST_BATCH","works":[{"name":"generated","workId":"work-generated","workTypeName":"child","payload":"generated"}]},"metadata":{"source":"generator:unit-test","parentLineage":["request-parent","work-parent"]},"submissions":[{"name":"generated","workId":"work-generated","targetState":"complete","executionId":"exec-child","tags":{"runtime":"true"}}]}`
	snapshot := workerBatchSnapshot(output)

	result, err := transitioner.Execute(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected transitioner result")
	}
	if len(result.GeneratedBatches) != 1 {
		t.Fatalf("generated batches = %d, want 1", len(result.GeneratedBatches))
	}
	batch := result.GeneratedBatches[0]
	if batch.Metadata.Source != "generator:unit-test" {
		t.Fatalf("batch source = %q, want generator:unit-test", batch.Metadata.Source)
	}
	if batch.Request.RequestID != "metadata-request" {
		t.Fatalf("work request id = %q, want metadata-request", batch.Request.RequestID)
	}
	if len(batch.Metadata.ParentLineage) != 2 {
		t.Fatalf("parent lineage = %#v, want metadata preserved", batch.Metadata.ParentLineage)
	}
	normalized, err := factory.NormalizeGeneratedSubmissionBatch(batch, interfaces.WorkRequestNormalizeOptions{
		ValidWorkTypes: map[string]bool{"task": true, "child": true},
	})
	if err != nil {
		t.Fatalf("NormalizeGeneratedSubmissionBatch: %v", err)
	}
	if len(normalized) != 1 {
		t.Fatalf("normalized submissions = %d, want 1", len(normalized))
	}
	if normalized[0].TargetState != "complete" {
		t.Fatalf("work input target state = %q, want complete", normalized[0].TargetState)
	}
	if normalized[0].ExecutionID != "exec-child" {
		t.Fatalf("work input execution id = %q, want exec-child", normalized[0].ExecutionID)
	}
	if got := normalized[0].Tags["runtime"]; got != "true" {
		t.Fatalf("runtime tag = %q, want true", got)
	}
}

func TestTransitioner_MalformedWorkerEmittedFactoryRequestBatchFailsDispatch(t *testing.T) {
	now := time.Date(2026, time.April, 16, 22, 5, 0, 0, time.UTC)
	net := workerBatchTestNet()
	transitioner := NewTransitioner(net, nil, WithTransitionerClock(func() time.Time { return now }))
	snapshot := workerBatchSnapshot(`{"request":{"requestId":"bad-request","type":"FACTORY_REQUEST_BATCH","works":[]}}`)

	result, err := transitioner.Execute(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result == nil || len(result.CompletedDispatches) != 1 {
		t.Fatalf("completed dispatches = %#v, want 1", result)
	}
	completed := result.CompletedDispatches[0]
	if completed.Outcome != interfaces.OutcomeFailed {
		t.Fatalf("completed outcome = %s, want FAILED", completed.Outcome)
	}
	if !strings.Contains(completed.Reason, "worker-emitted work request batch") {
		t.Fatalf("completed reason = %q, want worker-emitted validation prefix", completed.Reason)
	}
	if !strings.Contains(completed.Reason, "works array must contain at least one item") {
		t.Fatalf("completed reason = %q, want validation failure", completed.Reason)
	}
	if len(result.Mutations) != 1 || result.Mutations[0].ToPlace != "task:failed" {
		t.Fatalf("failure mutations = %#v, want failed arc", result.Mutations)
	}
	if len(result.GeneratedBatches) != 0 {
		t.Fatalf("generated batches = %#v, want none", result.GeneratedBatches)
	}
}

func workerBatchTestNet() *state.Net {
	return &state.Net{
		Places: map[string]*petri.Place{
			"task:init":      {ID: "task:init", TypeID: "task", State: "init"},
			"task:complete":  {ID: "task:complete", TypeID: "task", State: "complete"},
			"task:failed":    {ID: "task:failed", TypeID: "task", State: "failed"},
			"child:init":     {ID: "child:init", TypeID: "child", State: "init"},
			"child:complete": {ID: "child:complete", TypeID: "child", State: "complete"},
		},
		WorkTypes: map[string]*state.WorkType{
			"task": {
				ID: "task",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
					{Value: "complete", Category: state.StateCategoryTerminal},
					{Value: "failed", Category: state.StateCategoryFailed},
				},
			},
			"child": {
				ID: "child",
				States: []state.StateDefinition{
					{Value: "init", Category: state.StateCategoryInitial},
					{Value: "complete", Category: state.StateCategoryTerminal},
				},
			},
		},
		Transitions: map[string]*petri.Transition{
			"t1": {
				ID: "t1",
				OutputArcs: []petri.Arc{
					{ID: "accepted", PlaceID: "task:complete"},
				},
				FailureArcs: []petri.Arc{
					{ID: "failed", PlaceID: "task:failed"},
				},
			},
		},
	}
}

func workerBatchSnapshot(output string) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	return &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		Dispatches: map[string]*interfaces.DispatchEntry{
			"dispatch-1": {
				DispatchID:   "dispatch-1",
				TransitionID: "t1",
				ConsumedTokens: []interfaces.Token{{
					ID:        "tok-source",
					PlaceID:   "task:init",
					CreatedAt: time.Date(2026, time.April, 16, 21, 0, 0, 0, time.UTC),
					Color: interfaces.TokenColor{
						Name:       "source",
						RequestID:  "request-source",
						WorkID:     "work-source",
						WorkTypeID: "task",
						DataType:   interfaces.DataTypeWork,
						TraceID:    "trace-source",
						Tags:       map[string]string{"tenant": "port"},
					},
				}},
			},
		},
		Results: []interfaces.WorkResult{{
			DispatchID:   "dispatch-1",
			TransitionID: "t1",
			Outcome:      interfaces.OutcomeAccepted,
			Output:       output,
		}},
	}
}
