package conductor_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/conductor"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

func TestConductorStampsCorrelationBeforeDestination(t *testing.T) {
	t.Parallel()

	providers, recording := newLimitedCapabilityRegistry(t)
	recording.invoke = func(ctx context.Context, request inference.InvocationRequest, writer inference.ResponseWriter) error {
		event := runEvent(t, "provider-wrong-run-id", string(recording.identity), workers.PhaseStarted)
		if err := writer.WriteEvent(ctx, event); err != nil {
			return err
		}
		return writer.Close(ctx, inference.FailedCompletion(inference.NewFailure(inference.FailureInput{
			Kind:    inference.FailureDependency,
			Message: "fixture closed after stamped progress",
		})))
	}
	destination := &orderedDestination{}
	err := conductor.New(providers).Invoke(
		context.Background(),
		"conductor.fixture",
		acceptedRequest("inv-stamp"),
		destination,
	)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if len(destination.drafts) != 1 {
		t.Fatalf("destination drafts = %d, want 1", len(destination.drafts))
	}
	if got := destination.drafts[0].Draft().RunID; got != "inv-stamp" {
		t.Fatalf("destination RunID = %q, want conductor-owned invocation id", got)
	}
}

func TestConductorRejectsInvalidWorkersDraftContract(t *testing.T) {
	t.Parallel()

	providers, recording := newLimitedCapabilityRegistry(t)
	var observedWrite error
	recording.invoke = func(ctx context.Context, request inference.InvocationRequest, writer inference.ResponseWriter) error {
		invalid := runEvent(t, request.InvocationID(), string(recording.identity), workers.PhaseDelta)
		observedWrite = writer.WriteEvent(ctx, invalid)
		return observedWrite
	}
	destination := &orderedDestination{}
	err := conductor.New(providers).Invoke(
		context.Background(),
		"conductor.fixture",
		acceptedRequest("inv-invalid-draft"),
		destination,
	)
	if observedWrite == nil {
		t.Fatal("WriteEvent() error = nil, want Workers Draft contract rejection")
	}
	if err == nil {
		t.Fatal("Invoke() error = nil, want Draft contract rejection")
	}
	if len(destination.drafts) != 0 || destination.closes != 0 {
		t.Fatalf("destination received output after invalid draft: drafts=%d closes=%d", len(destination.drafts), destination.closes)
	}
}

func TestConductorPreservesDraftEmissionOrder(t *testing.T) {
	t.Parallel()

	providers, recording := newLimitedCapabilityRegistry(t)
	recording.invoke = func(ctx context.Context, request inference.InvocationRequest, writer inference.ResponseWriter) error {
		provider := string(recording.identity)
		for _, phase := range []workers.Phase{workers.PhaseStarted, workers.PhaseCompleted} {
			if err := writer.WriteEvent(ctx, runEvent(t, request.InvocationID(), provider, phase)); err != nil {
				return err
			}
		}
		if err := writer.WriteEvent(ctx, messageEvent(t, request.InvocationID(), provider, "message-1", "ordered")); err != nil {
			return err
		}
		return writer.Close(ctx, inference.SuccessfulCompletion(inference.NewResponse(inference.ResponseInput{
			Content: "ordered",
		})))
	}
	destination := &orderedDestination{}
	if err := conductor.New(providers).Invoke(
		context.Background(),
		"conductor.fixture",
		acceptedRequest("inv-order"),
		destination,
	); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	want := []string{"RUN:STARTED", "RUN:COMPLETED", "MESSAGE:COMPLETED", "CLOSE"}
	if got := destination.order(); !equalStrings(got, want) {
		t.Fatalf("destination order = %v, want %v", got, want)
	}
}

func TestConductorStopsAfterDestinationWriteFailure(t *testing.T) {
	t.Parallel()

	providers, recording := newLimitedCapabilityRegistry(t)
	sinkErr := errors.New("response sink failed")
	destination := &failingDestination{err: sinkErr}
	var lateWrite, lateClose error
	recording.invoke = func(ctx context.Context, request inference.InvocationRequest, writer inference.ResponseWriter) error {
		provider := string(recording.identity)
		writeErr := writer.WriteEvent(ctx, runEvent(t, request.InvocationID(), provider, workers.PhaseStarted))
		lateWrite = writer.WriteEvent(ctx, runEvent(t, request.InvocationID(), provider, workers.PhaseCompleted))
		lateClose = writer.Close(ctx, inference.FailedCompletion(inference.NewFailure(inference.FailureInput{
			Kind:    inference.FailureDependency,
			Message: "should not replace writer failure",
		})))
		return writeErr
	}

	err := conductor.New(providers).Invoke(
		context.Background(),
		"conductor.fixture",
		acceptedRequest("inv-writer-failure"),
		destination,
	)
	if !errors.Is(err, sinkErr) {
		t.Fatalf("Invoke() error = %v, want preserved destination failure", err)
	}
	if lateWrite == nil {
		t.Fatal("late WriteEvent() error = nil, want rejection after writer failure")
	}
	if lateClose == nil {
		t.Fatal("late Close() error = nil, want rejection after writer failure")
	}
	if destination.writes != 1 || destination.closes != 0 {
		t.Fatalf("destination calls = %d writes, %d closes; want one failed write and no close", destination.writes, destination.closes)
	}
}

func TestConductorRejectsLateWriteAfterClose(t *testing.T) {
	t.Parallel()

	providers, recording := newLimitedCapabilityRegistry(t)
	destination := &orderedDestination{}
	var lateWrite error
	recording.invoke = func(ctx context.Context, request inference.InvocationRequest, writer inference.ResponseWriter) error {
		if err := writer.Close(ctx, inference.SuccessfulCompletion(inference.NewResponse(inference.ResponseInput{
			Content: "done",
		}))); err != nil {
			return err
		}
		lateWrite = writer.WriteEvent(ctx, messageEvent(t, request.InvocationID(), string(recording.identity), "message-late", "late"))
		return lateWrite
	}

	err := conductor.New(providers).Invoke(
		context.Background(),
		"conductor.fixture",
		acceptedRequest("inv-late-write"),
		destination,
	)
	if lateWrite == nil {
		t.Fatal("late WriteEvent() error = nil, want rejection after close")
	}
	if err == nil {
		t.Fatal("Invoke() error = nil, want late-write rejection")
	}
	if destination.closes != 1 || len(destination.drafts) != 0 {
		t.Fatalf("destination drafts=%d closes=%d, want close without competing progress", len(destination.drafts), destination.closes)
	}
	if destination.completion == nil || destination.completion.Response() == nil {
		t.Fatalf("destination completion = %#v, want preserved successful close", destination.completion)
	}
}

func acceptedRequest(invocationID string) inference.InvocationRequest {
	return inference.NewInvocationRequest(inference.InvocationInput{
		InvocationID: invocationID,
		Required:     inference.NewCapabilitySet(inference.CapabilityPromptSubmission),
	})
}

type orderedDestination struct {
	drafts     []inference.EventDraft
	completion *inference.Completion
	closes     int
}

func (w *orderedDestination) WriteEvent(_ context.Context, event inference.EventDraft) error {
	w.drafts = append(w.drafts, event)
	return nil
}

func (w *orderedDestination) Close(_ context.Context, completion inference.Completion) error {
	w.closes++
	w.completion = &completion
	return nil
}

func (w *orderedDestination) order() []string {
	out := make([]string, 0, len(w.drafts)+w.closes)
	for _, event := range w.drafts {
		draft := event.Draft()
		out = append(out, string(draft.Kind)+":"+string(draft.Phase))
	}
	if w.closes > 0 {
		out = append(out, "CLOSE")
	}
	return out
}

type failingDestination struct {
	err    error
	writes int
	closes int
}

func (w *failingDestination) WriteEvent(context.Context, inference.EventDraft) error {
	w.writes++
	return w.err
}

func (w *failingDestination) Close(context.Context, inference.Completion) error {
	w.closes++
	return nil
}

func runEvent(t *testing.T, runID, provider string, phase workers.Phase) inference.EventDraft {
	t.Helper()
	payload, err := json.Marshal(workers.RunPayload{Status: string(phase)})
	if err != nil {
		t.Fatalf("marshal run payload: %v", err)
	}
	event, err := inference.NewEventDraft(inference.EventDraftInput{
		RunID:   runID,
		Kind:    workers.KindRun,
		Phase:   phase,
		Payload: payload,
		Provenance: workers.Provenance{
			Delivery:        workers.DeliveryNativeStream,
			Fidelity:        workers.FidelityNormalized,
			NativeEventType: "fixture.run",
			Provider:        provider,
			Representation:  workers.RepresentationSnapshot,
		},
	})
	if err != nil {
		t.Fatalf("NewEventDraft() error = %v", err)
	}
	return event
}

func messageEvent(t *testing.T, runID, provider, itemID, content string) inference.EventDraft {
	t.Helper()
	payload, err := json.Marshal(workers.MessagePayload{
		Role: "assistant",
		ContentBlocks: []workers.ContentBlock{{
			Kind: workers.ContentBlockText,
			Text: content,
		}},
	})
	if err != nil {
		t.Fatalf("marshal message payload: %v", err)
	}
	event, err := inference.NewEventDraft(inference.EventDraftInput{
		RunID:   runID,
		Kind:    workers.KindMessage,
		Phase:   workers.PhaseCompleted,
		ItemID:  itemID,
		Payload: payload,
		Provenance: workers.Provenance{
			Delivery:        workers.DeliveryNativeFinal,
			Fidelity:        workers.FidelityFinalOnly,
			NativeEventType: "fixture.message",
			Provider:        provider,
			Representation:  workers.RepresentationSnapshot,
		},
	})
	if err != nil {
		t.Fatalf("NewEventDraft() error = %v", err)
	}
	return event
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
