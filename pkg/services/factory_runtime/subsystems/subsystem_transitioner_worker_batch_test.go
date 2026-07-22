package subsystems

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/token"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestTransitioner_WorkerEmittedGeneratedSubmissionBatchCreatesGeneratedWork(t *testing.T) {
	now := time.Date(2026, time.April, 16, 22, 0, 0, 0, time.UTC)
	net := workerBatchTestNet()
	transitioner := NewTransitioner(net, nil, func() time.Time { return now }, testTokenTransformer(net), nil, nil, nil, testWorkPropagationPolicy())
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
	transitioner := NewTransitioner(net, nil, func() time.Time { return now }, testTokenTransformer(net), nil, nil, nil, testWorkPropagationPolicy())
	output := `{"request":{"type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"child"}]}}`
	snapshot := workerBatchSnapshot(output)
	snapshot.Dispatches["dispatch-1"].ConsumedTokens[0].Color.CurrentChainingTraceID = "chain-source"
	snapshot.Dispatches["dispatch-1"].ConsumedTokens[0].Color.TraceID = "trace-source"

	result := executeWorkerBatchTransition(t, transitioner, snapshot)
	batch := result.GeneratedBatches[0]
	normalized, err := work.NormalizeGeneratedSubmissionBatch(batch, work.WorkRequestNormalizeOptions{
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

func hasMutationToPlace(mutations []interfaces.MarkingMutation, placeID string) bool {
	for i := range mutations {
		if mutations[i].ToPlace == placeID {
			return true
		}
	}
	return false
}

func assertGeneratedWorkerBatchMetadata(t *testing.T, result *interfaces.TickResult) (work.GeneratedSubmissionBatch, string) {
	t.Helper()

	if len(result.Mutations) != 1 || result.Mutations[0].ToPlace != "task:complete" {
		t.Fatalf("mutations = %#v, want ordinary parent output routing", result.Mutations)
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

func normalizeGeneratedWorkerBatch(t *testing.T, batch work.GeneratedSubmissionBatch) []work.SubmitRequest {
	t.Helper()

	normalized, err := work.NormalizeGeneratedSubmissionBatch(batch, work.WorkRequestNormalizeOptions{
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
	normalized []work.SubmitRequest,
) work.SubmitRequest {
	t.Helper()

	record := work.WorkRequestRecordFromSubmitRequests(requestID, source, normalized)
	if len(record.Relations) != 3 {
		t.Fatalf("request relation count = %d, want dependency plus two parent relations", len(record.Relations))
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
	if !hasRuntimeRelation(first.Relations, work.RelationParentChild, "work-source") {
		t.Fatalf("first generated relations = %#v, want parent work-source", first.Relations)
	}
	return first
}

func hasRuntimeRelation(relations []work.Relation, relationType work.RelationType, targetWorkID string) bool {
	for i := range relations {
		if relations[i].Type == relationType && relations[i].TargetWorkID == targetWorkID {
			return true
		}
	}
	return false
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
	first work.SubmitRequest,
	second work.SubmitRequest,
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
	if !hasRuntimeRelation(second.Relations, work.RelationDependsOn, first.WorkID) {
		t.Fatalf("generated relations = %#v, want dependency target %q", second.Relations, first.WorkID)
	}
	if !hasRuntimeRelation(second.Relations, work.RelationParentChild, "work-source") {
		t.Fatalf("generated relations = %#v, want parent work-source", second.Relations)
	}
	if result.CompletedDispatches[0].Outcome != workerexecution.OutcomeAccepted {
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
	transitioner := NewTransitioner(net, nil, func() time.Time { return now }, testTokenTransformer(net), nil, nil, nil, testWorkPropagationPolicy())
	output := `{"request":{"type":"FACTORY_REQUEST_BATCH","works":[{"name":"follow-up","workTypeName":"child"}]}}`
	snapshot := workerBatchSnapshot(output)
	snapshot.Dispatches["dispatch-1"].ConsumedTokens = append(snapshot.Dispatches["dispatch-1"].ConsumedTokens, factorytoken.Token{
		ID:        "agent-slot:resource:0",
		PlaceID:   "agent-slot:available",
		CreatedAt: now.Add(-time.Hour),
		EnteredAt: now.Add(-time.Hour),
		Color: factorytoken.Color{
			WorkID:     "agent-slot:0",
			WorkTypeID: "agent-slot",
			DataType:   factorytoken.DataTypeResource,
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
	var releasedResource *factorytoken.Token
	for i := range result.Mutations {
		mutation := result.Mutations[i]
		if mutation.NewToken == nil {
			continue
		}
		switch mutation.NewToken.Color.DataType {
		case factorytoken.DataTypeWork:
			if mutation.ToPlace == "child:init" {
				generatedWork++
			}
		case factorytoken.DataTypeResource:
			if mutation.ToPlace == "agent-slot:available" {
				releasedResource = mutation.NewToken
			}
		}
	}
	if generatedWork != 0 {
		t.Fatalf("generated work mutations = %d, want engine-owned generated work", generatedWork)
	}
	if !hasMutationToPlace(result.Mutations, "task:complete") {
		t.Fatalf("ordinary accepted output mutation missing from %#v", result.Mutations)
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

	transitioner := NewTransitioner(net, nil, func() time.Time { return now }, testTokenTransformer(net), nil, nil, nil, testWorkPropagationPolicy())
	snapshot := workerBatchSnapshot("accepted")
	snapshot.Dispatches["dispatch-1"].ConsumedTokens = append(snapshot.Dispatches["dispatch-1"].ConsumedTokens,
		factorytoken.Token{
			ID:        "agent-slot:resource:0",
			PlaceID:   "agent-slot:available",
			CreatedAt: now.Add(-time.Hour),
			EnteredAt: now.Add(-time.Hour),
			Color: factorytoken.Color{
				WorkID:     "agent-slot:0",
				WorkTypeID: "agent-slot",
				DataType:   factorytoken.DataTypeResource,
			},
		},
		factorytoken.Token{
			ID:        "agent-slot:resource:1",
			PlaceID:   "agent-slot:available",
			CreatedAt: now.Add(-time.Hour),
			EnteredAt: now.Add(-time.Hour),
			Color: factorytoken.Color{
				WorkID:     "agent-slot:1",
				WorkTypeID: "agent-slot",
				DataType:   factorytoken.DataTypeResource,
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
		if mutation.NewToken == nil || mutation.NewToken.Color.DataType != factorytoken.DataTypeResource {
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
	transitioner := NewTransitioner(net, nil, func() time.Time { return now }, testTokenTransformer(net), nil, nil, nil, testWorkPropagationPolicy())
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
	if result.CompletedDispatches[0].Outcome != workerexecution.OutcomeAccepted {
		t.Fatalf("completed outcome = %s, want ACCEPTED", result.CompletedDispatches[0].Outcome)
	}
}

func TestTransitioner_WorkerEmittedGeneratedSubmissionBatchUsesBatchMetadataSource(t *testing.T) {
	now := time.Date(2026, time.April, 18, 0, 0, 0, 0, time.UTC)
	net := workerBatchTestNet()
	transitioner := NewTransitioner(net, nil, func() time.Time { return now }, testTokenTransformer(net), nil, nil, nil, testWorkPropagationPolicy())
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
	normalized, err := work.NormalizeGeneratedSubmissionBatch(batch, work.WorkRequestNormalizeOptions{
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
	transitioner := NewTransitioner(net, nil, func() time.Time { return now }, testTokenTransformer(net), nil, nil, nil, testWorkPropagationPolicy())
	snapshot := workerBatchSnapshot(`{"request":{"requestId":"bad-request","type":"FACTORY_REQUEST_BATCH","works":[]}}`)

	result, err := transitioner.Execute(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result == nil || len(result.CompletedDispatches) != 1 {
		t.Fatalf("completed dispatches = %#v, want 1", result)
	}
	completed := result.CompletedDispatches[0]
	if completed.Outcome != workerexecution.OutcomeFailed {
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
