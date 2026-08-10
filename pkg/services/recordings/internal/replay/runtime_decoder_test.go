package replay

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
)

var testFactorySnapshotDecoder factorydefinitions.FactorySnapshotJSONDecoder = func(
	data []byte,
) (*factorydefinitions.FactorySnapshot, error) {
	generated, err := factorymapping.GeneratedFactoryFromOpenAPIJSON(data)
	if err != nil {
		return nil, err
	}
	return factorydefinitions.NewFactorySnapshot(generated)
}

func testRuntimeConfigDecoder(
	snapshot *factorydefinitions.FactorySnapshot,
) (factorydefinitions.ReplayRuntimeConfig, error) {
	var generated factoryapi.Factory
	if err := snapshot.Decode(&generated); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(generated)
	if err != nil {
		return nil, err
	}
	config, err := factorymapping.FactoryConfigFromOpenAPIJSON(payload)
	if err != nil {
		return nil, err
	}
	factoryDir := ""
	if generated.FactoryDirectory != nil {
		factoryDir = *generated.FactoryDirectory
	}
	return runtimefixtures.ReplayRuntimeConfigValue(config, factoryDir), nil
}

// TestBTRCP0ReplayCharacterization_ReconstructsCheckedInSuccess freezes the
// legacy replay contract against a fixed event artifact. The expected order is
// authored here from the recording, rather than derived from a projection.
func TestBTRCP0ReplayCharacterization_ReconstructsCheckedInSuccess(t *testing.T) {
	artifact := loadBTRCP0InferenceArtifact(t)
	assertBTRCP0InferenceTimeline(t, artifact.Events, interfaces.FactoryStateCompleted)

	reduced, err := reduceReplayEvents(artifact, testFactorySnapshotDecoder, testRuntimeConfigDecoder)
	if err != nil {
		t.Fatalf("reduce checked-in replay: %v", err)
	}
	assertBTRCP0ReducedSuccess(t, reduced)

	assertBTRCP0SubmissionSchedule(t, artifact)
	assertBTRCP0ProviderReplay(t, reduced.Dispatches[0].dispatch, artifact)
	assertBTRCP0CompletionDelivery(t, reduced.Dispatches[0].dispatch, artifact)
}

func assertBTRCP0ReducedSuccess(t *testing.T, reduced *replayEventLog) {
	t.Helper()
	if len(reduced.Submissions) != 1 || len(reduced.Dispatches) != 1 || len(reduced.Completions) != 1 {
		t.Fatalf("reduced replay cardinality = submissions:%d dispatches:%d completions:%d, want 1/1/1", len(reduced.Submissions), len(reduced.Dispatches), len(reduced.Completions))
	}
	if got := reduced.Submissions[0].request.RequestID; got != "request-fixture-1" {
		t.Fatalf("replayed request ID = %q, want request-fixture-1", got)
	}
	if got := reduced.Submissions[0].request.Works[0].WorkID; got != "work-fixture-1" {
		t.Fatalf("replayed work ID = %q, want work-fixture-1", got)
	}

	dispatch := reduced.Dispatches[0].dispatch
	if dispatch.Execution.ReplayKey != "process/work-fixture-1" {
		t.Fatalf("replayed dispatch replay key = %q, want process/work-fixture-1", dispatch.Execution.ReplayKey)
	}
	completion := reduced.Completions[0]
	if completion.result.Outcome != workerexecution.OutcomeAccepted || completion.result.Output != "fixture provider response" {
		t.Fatalf("replayed completion = %#v, want accepted fixture provider response", completion.result)
	}
}

func assertBTRCP0SubmissionSchedule(t *testing.T, artifact *interfaces.ReplayArtifact) {
	t.Helper()
	hook, err := NewSubmissionHook(testFactorySnapshotDecoder, testRuntimeConfigDecoder, artifact)
	if err != nil {
		t.Fatalf("NewSubmissionHook: %v", err)
	}
	before, err := hook.OnTick(context.Background(), recordings.ReplaySnapshot{Tick: 0})
	if err != nil {
		t.Fatalf("submission hook before due tick: %v", err)
	}
	if len(before.GeneratedBatches) != 0 || !before.KeepAlive {
		t.Fatalf("submission before due tick = %#v, want no batch and keep-alive", before)
	}
	due, err := hook.OnTick(context.Background(), recordings.ReplaySnapshot{Tick: 1})
	if err != nil {
		t.Fatalf("submission hook at due tick: %v", err)
	}
	if len(due.GeneratedBatches) != 1 || due.GeneratedBatches[0].Request.RequestID != "request-fixture-1" || due.KeepAlive {
		t.Fatalf("submission at due tick = %#v, want one terminal batch", due)
	}
}

func assertBTRCP0ProviderReplay(t *testing.T, dispatch work.WorkDispatch, artifact *interfaces.ReplayArtifact) {
	t.Helper()
	sideEffects, err := NewSideEffects(testFactorySnapshotDecoder, testRuntimeConfigDecoder, artifact)
	if err != nil {
		t.Fatalf("NewSideEffects: %v", err)
	}
	providerResponse, err := sideEffects.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		Dispatch:        work.CloneWorkDispatch(dispatch),
		WorkerType:      dispatch.WorkerType,
		WorkstationType: dispatch.WorkstationName,
	})
	if err != nil {
		t.Fatalf("replayed provider effect: %v", err)
	}
	if providerResponse.Content != "fixture provider response" {
		t.Fatalf("replayed provider content = %q, want fixture provider response", providerResponse.Content)
	}
}

func assertBTRCP0CompletionDelivery(t *testing.T, dispatch work.WorkDispatch, artifact *interfaces.ReplayArtifact) {
	t.Helper()
	deliveryPlan, err := NewCompletionDeliveryPlan(testFactorySnapshotDecoder, testRuntimeConfigDecoder, artifact)
	if err != nil {
		t.Fatalf("NewCompletionDeliveryPlan: %v", err)
	}
	deliveryTick, ok, err := deliveryPlan.DeliveryTickForDispatch(work.CloneWorkDispatch(dispatch))
	if err != nil || !ok || deliveryTick != 3 {
		t.Fatalf("replayed completion delivery = tick:%d matched:%t error:%v, want tick 3", deliveryTick, ok, err)
	}
	planned, ok, err := deliveryPlan.PlannedResultForDispatch(dispatch)
	if err != nil || !ok {
		t.Fatalf("replayed terminal result = %#v matched:%t error:%v, want recorded result", planned, ok, err)
	}
	if planned.Outcome != workerexecution.OutcomeAccepted || planned.Output != "fixture provider response" {
		t.Fatalf("planned terminal result = %#v, want accepted fixture provider response", planned)
	}
}

// TestBTRCP0ReplayCharacterization_ReconstructsFailureAndTypedDivergence
// freezes both a recorded failure result and the typed stop condition used
// when the observed dispatch no longer matches the recording.
func TestBTRCP0ReplayCharacterization_ReconstructsFailureAndTypedDivergence(t *testing.T) {
	artifact := loadBTRCP0InferenceArtifact(t)
	mutateBTRCP0Failure(t, artifact)
	assertBTRCP0InferenceTimeline(t, artifact.Events, interfaces.FactoryStateFailed)

	reduced, err := reduceReplayEvents(artifact, testFactorySnapshotDecoder, testRuntimeConfigDecoder)
	if err != nil {
		t.Fatalf("reduce failed replay: %v", err)
	}
	if reduced.Completions[0].result.Outcome != workerexecution.OutcomeFailed ||
		reduced.Completions[0].result.Error != "fixture provider rejected the request" {
		t.Fatalf("failed replay completion = %#v, want typed provider failure", reduced.Completions[0].result)
	}
	if reduced.Completions[0].result.FailureMetadata == nil ||
		reduced.Completions[0].result.FailureMetadata.Type != workerexecution.WorkFailureTypePermanentBadRequest {
		t.Fatalf("failed replay metadata = %#v, want permanent_bad_request", reduced.Completions[0].result.FailureMetadata)
	}

	sideEffects, err := NewSideEffects(testFactorySnapshotDecoder, testRuntimeConfigDecoder, artifact)
	if err != nil {
		t.Fatalf("NewSideEffects for failed replay: %v", err)
	}
	_, err = sideEffects.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		Dispatch:        work.CloneWorkDispatch(reduced.Dispatches[0].dispatch),
		WorkerType:      reduced.Dispatches[0].dispatch.WorkerType,
		WorkstationType: reduced.Dispatches[0].dispatch.WorkstationName,
	})
	var providerErr *workerexecution.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Type != workerexecution.WorkFailureTypePermanentBadRequest {
		t.Fatalf("failed replay provider error = %v, want typed permanent_bad_request ProviderError", err)
	}

	deliveryPlan, err := NewCompletionDeliveryPlan(testFactorySnapshotDecoder, testRuntimeConfigDecoder, artifact)
	if err != nil {
		t.Fatalf("NewCompletionDeliveryPlan for failed replay: %v", err)
	}
	observed := work.CloneWorkDispatch(reduced.Dispatches[0].dispatch)
	observed.TransitionID = "unexpected-transition"
	_, _, err = deliveryPlan.DeliveryTickForDispatch(observed)
	var divergence *DivergenceError
	if !errors.As(err, &divergence) || divergence.Report.Category != DivergenceCategoryDispatchMismatch ||
		divergence.Report.ExpectedEventID != "factory-event/dispatch-created/dispatch-fixture-1" {
		t.Fatalf("replay divergence = %v, want typed dispatch mismatch for recorded dispatch", err)
	}
}

func loadBTRCP0InferenceArtifact(t *testing.T) *interfaces.ReplayArtifact {
	t.Helper()
	artifact, err := Load(
		testReplayStorage(),
		filepath.FromSlash("testdata/inference-events.replay.json"),
		testFactorySnapshotDecoder,
	)
	if err != nil {
		t.Fatalf("load checked-in inference replay: %v", err)
	}
	return artifact
}

func assertBTRCP0InferenceTimeline(t *testing.T, events []interfaces.FactoryEvent, terminalState interfaces.FactoryState) {
	t.Helper()
	wantIDs := []string{
		"factory-event/run-started",
		"factory-event/work-request/request-fixture-1",
		"factory-event/dispatch-created/dispatch-fixture-1",
		"factory-event/inference-request/inference-request-fixture-1",
		"factory-event/inference-response/inference-request-fixture-1",
		"factory-event/dispatch-completed/dispatch-fixture-1",
		"factory-event/run-finished",
	}
	wantTypes := []interfaces.FactoryEventType{
		interfaces.FactoryEventTypeRunRequest,
		interfaces.FactoryEventTypeWorkRequest,
		interfaces.FactoryEventTypeDispatchRequest,
		interfaces.FactoryEventTypeInferenceRequest,
		interfaces.FactoryEventTypeInferenceResponse,
		interfaces.FactoryEventTypeDispatchResponse,
		interfaces.FactoryEventTypeRunResponse,
	}
	wantTicks := []int{0, 1, 2, 2, 2, 3, 3}
	if len(events) != len(wantIDs) {
		t.Fatalf("replay event count = %d, want %d", len(events), len(wantIDs))
	}
	for index, event := range events {
		if event.Id != wantIDs[index] || event.Type != wantTypes[index] || event.Context.Sequence != index || event.Context.Tick != wantTicks[index] {
			t.Fatalf("replay event[%d] = id:%q type:%q sequence:%d tick:%d, want id:%q type:%q sequence:%d tick:%d", index, event.Id, event.Type, event.Context.Sequence, event.Context.Tick, wantIDs[index], wantTypes[index], index, wantTicks[index])
		}
	}
	var runResponse interfaces.RunResponseEventPayload
	if err := events[len(events)-1].DecodePayload(&runResponse); err != nil {
		t.Fatalf("decode replay terminal event: %v", err)
	}
	if runResponse.State == nil || *runResponse.State != terminalState {
		t.Fatalf("replay terminal state = %#v, want %q", runResponse.State, terminalState)
	}
}

func mutateBTRCP0Failure(t *testing.T, artifact *interfaces.ReplayArtifact) {
	t.Helper()
	for index := range artifact.Events {
		event := mustGeneratedReplayEvent(t, artifact.Events[index])
		if event.Type == factoryapi.FactoryEventTypeDispatchResponse {
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil {
				t.Fatalf("decode dispatch response for failed replay: %v", err)
			}
			payload.Outcome = factoryapi.WorkOutcomeFailed
			payload.Error = btrcP0StringPtr("fixture provider rejected the request")
			family := factoryapi.WorkFailureFamily(workerexecution.WorkFailureFamilyTerminal)
			failureType := factoryapi.WorkFailureType(workerexecution.WorkFailureTypePermanentBadRequest)
			payload.ProviderFailure = &factoryapi.ProviderFailureMetadata{Family: &family, Type: &failureType}
			var union factoryapi.FactoryEvent_Payload
			if err := union.FromDispatchResponseEventPayload(payload); err != nil {
				t.Fatalf("encode failed dispatch response: %v", err)
			}
			event.Payload = union
			converted, err := interfaces.NewFactoryEvent(event)
			if err != nil {
				t.Fatalf("convert failed dispatch response: %v", err)
			}
			artifact.Events[index] = converted
		}
		if event.Type == factoryapi.FactoryEventTypeRunResponse {
			var payload interfaces.RunResponseEventPayload
			if err := artifact.Events[index].DecodePayload(&payload); err != nil {
				t.Fatalf("decode run response for failed replay: %v", err)
			}
			state := interfaces.FactoryStateFailed
			payload.State = &state
			encoded, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("encode failed run response: %v", err)
			}
			artifact.Events[index].Payload = encoded
		}
	}
}

func btrcP0StringPtr(value string) *string { return &value }
