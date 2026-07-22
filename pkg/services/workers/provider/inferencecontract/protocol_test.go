package inferencecontract_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	contract "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

type invocationFunc func(context.Context, contract.InvocationRequest, contract.ResponseWriter) error

type lifecycleIntegration struct {
	invoke invocationFunc
}

func (i lifecycleIntegration) Identity() contract.Identity { return "customer.lifecycle" }
func (i lifecycleIntegration) MaximumCapabilities() contract.CapabilitySet {
	return contract.NewCapabilitySet(contract.CapabilityPromptSubmission)
}
func (i lifecycleIntegration) Discover(context.Context) (contract.Discovery, error) {
	return contract.NewDiscovery(contract.ReadinessReady), nil
}
func (i lifecycleIntegration) Capabilities(context.Context, contract.InvocationRequest) (contract.CapabilitySet, error) {
	return i.MaximumCapabilities(), nil
}
func (i lifecycleIntegration) Invoke(ctx context.Context, request contract.InvocationRequest, writer contract.ResponseWriter) error {
	return i.invoke(ctx, request, writer)
}

func TestExecuteInvocationFlushesValidTerminalTailBeforeClose(t *testing.T) {
	t.Parallel()
	destination := &orderedWriter{}
	integration := lifecycleIntegration{invoke: func(ctx context.Context, request contract.InvocationRequest, writer contract.ResponseWriter) error {
		if err := writer.WriteEvent(ctx, event(t, request.InvocationID(), workers.KindRun, workers.PhaseStarted, "", runPayload(t, "started"))); err != nil {
			return err
		}
		if err := writer.WriteEvent(ctx, event(t, request.InvocationID(), workers.KindMessage, workers.PhaseCompleted, "message-1", messagePayload(t, "answer"))); err != nil {
			return err
		}
		if err := writer.WriteEvent(ctx, event(t, request.InvocationID(), workers.KindRun, workers.PhaseCompleted, "", runPayload(t, "completed"))); err != nil {
			return err
		}
		return writer.Close(ctx, contract.SuccessfulCompletion(contract.NewResponse(contract.ResponseInput{Content: "answer"})))
	}}

	if err := contract.ExecuteInvocation(context.Background(), integration, request(), destination); err != nil {
		t.Fatalf("ExecuteInvocation() error = %v", err)
	}
	want := []string{"RUN:STARTED", "MESSAGE:COMPLETED", "RUN:COMPLETED", "CLOSE"}
	if !equalStrings(destination.order, want) {
		t.Fatalf("destination order = %v, want %v", destination.order, want)
	}
}

func TestExecuteInvocationRejectsMalformedEventLifecycles(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		rule   string
		invoke invocationFunc
	}{
		{name: "cross invocation", rule: "invocation_correlation", invoke: writeOne(eventFactory(t, "other", workers.KindRun, workers.PhaseStarted, "", runPayload(t, "started")))},
		{name: "delta before start", rule: "update_before_start", invoke: writeOne(eventFactory(t, "invocation-1", workers.KindMessage, workers.PhaseDelta, "message-1", messageDeltaPayload(t, "part")))},
		{name: "duplicate terminal", rule: "duplicate_terminal", invoke: func(ctx context.Context, request contract.InvocationRequest, writer contract.ResponseWriter) error {
			completed := event(t, request.InvocationID(), workers.KindMessage, workers.PhaseCompleted, "message-1", messagePayload(t, "answer"))
			if err := writer.WriteEvent(ctx, completed); err != nil {
				return err
			}
			return writer.WriteEvent(ctx, completed)
		}},
		{name: "incomplete lifecycle", rule: "incomplete_lifecycle", invoke: func(ctx context.Context, request contract.InvocationRequest, writer contract.ResponseWriter) error {
			if err := writer.WriteEvent(ctx, event(t, request.InvocationID(), workers.KindRun, workers.PhaseStarted, "", runPayload(t, "started"))); err != nil {
				return err
			}
			return writer.Close(ctx, contract.SuccessfulCompletion(contract.NewResponse(contract.ResponseInput{Content: "answer"})))
		}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := contract.ExecuteInvocation(context.Background(), lifecycleIntegration{invoke: test.invoke}, request(), &orderedWriter{})
			assertProtocolRule(t, err, test.rule)
		})
	}
}

func TestExecuteInvocationEnforcesStableToolCorrelation(t *testing.T) {
	t.Parallel()
	integration := lifecycleIntegration{invoke: func(ctx context.Context, request contract.InvocationRequest, writer contract.ResponseWriter) error {
		if err := writer.WriteEvent(ctx, event(t, request.InvocationID(), workers.KindTool, workers.PhaseStarted, "tool-item", toolPayload(t, "tool-1", "lookup", "running"))); err != nil {
			return err
		}
		if err := writer.WriteEvent(ctx, event(t, request.InvocationID(), workers.KindTool, workers.PhaseDelta, "tool-item", toolDeltaPayload(t, "tool-1", "partial"))); err != nil {
			return err
		}
		return writer.WriteEvent(ctx, event(t, request.InvocationID(), workers.KindTool, workers.PhaseCompleted, "tool-item", toolPayload(t, "tool-1", "different", "completed")))
	}}
	err := contract.ExecuteInvocation(context.Background(), integration, request(), &orderedWriter{})
	assertProtocolRule(t, err, "tool_correlation")
}

func TestExecuteInvocationRejectsInvalidDraftAndCompletionValues(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		rule   string
		invoke invocationFunc
	}{
		{
			name: "invalid provenance",
			rule: "provenance_provider",
			invoke: func(ctx context.Context, request contract.InvocationRequest, writer contract.ResponseWriter) error {
				draft := event(t, request.InvocationID(), workers.KindRun, workers.PhaseStarted, "", runPayload(t, "started"))
				input := draft.Draft()
				input.Provenance.Provider = "different.provider"
				altered, err := contract.NewEventDraft(contract.EventDraftInput{
					RunID: input.RunID, Kind: input.Kind, Phase: input.Phase, Payload: input.Payload,
					ProviderSessionRef: input.ProviderSessionRef, Provenance: input.Provenance,
				})
				if err != nil {
					return err
				}
				return writer.WriteEvent(ctx, altered)
			},
		},
		{
			name: "unbounded payload",
			rule: "payload_bound",
			invoke: func(ctx context.Context, request contract.InvocationRequest, writer contract.ResponseWriter) error {
				return writer.WriteEvent(ctx, event(t, request.InvocationID(), workers.KindTool, workers.PhaseDelta, "tool-1", toolDeltaPayload(t, "tool-1", string(make([]byte, 70*1024)))))
			},
		},
		{
			name: "empty completion",
			rule: "completion_outcome",
			invoke: func(ctx context.Context, _ contract.InvocationRequest, writer contract.ResponseWriter) error {
				return writer.Close(ctx, contract.Completion{})
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := contract.ExecuteInvocation(context.Background(), lifecycleIntegration{invoke: test.invoke}, request(), &orderedWriter{})
			assertProtocolRule(t, err, test.rule)
		})
	}
}

func TestExecuteInvocationRejectsFinalResultDisagreement(t *testing.T) {
	t.Parallel()
	destination := &orderedWriter{}
	integration := lifecycleIntegration{invoke: func(ctx context.Context, request contract.InvocationRequest, writer contract.ResponseWriter) error {
		if err := writer.WriteEvent(ctx, event(t, request.InvocationID(), workers.KindMessage, workers.PhaseCompleted, "message-1", messagePayload(t, "streamed"))); err != nil {
			return err
		}
		return writer.Close(ctx, contract.SuccessfulCompletion(contract.NewResponse(contract.ResponseInput{Content: "different"})))
	}}
	err := contract.ExecuteInvocation(context.Background(), integration, request(), destination)
	assertProtocolRule(t, err, "final_result_agreement")
	if len(destination.order) != 0 {
		t.Fatalf("destination received contradictory terminal state: %v", destination.order)
	}
}

func TestExecuteInvocationNormalizesErrorBeforeClose(t *testing.T) {
	t.Parallel()
	destination := &orderedWriter{}
	providerErr := errors.New("native payload with secret")
	integration := lifecycleIntegration{invoke: func(ctx context.Context, request contract.InvocationRequest, writer contract.ResponseWriter) error {
		if err := writer.WriteEvent(ctx, event(t, request.InvocationID(), workers.KindRun, workers.PhaseStarted, "", runPayload(t, "started"))); err != nil {
			return err
		}
		return providerErr
	}}
	err := contract.ExecuteInvocation(context.Background(), integration, request(), destination)
	if !errors.Is(err, providerErr) {
		t.Fatalf("ExecuteInvocation() error = %v, want provider error", err)
	}
	if destination.completion == nil || destination.completion.Failure() == nil {
		t.Fatal("destination did not receive normalized failure")
	}
	failure := destination.completion.Failure()
	if failure.Kind() != contract.FailureUnknown || failure.Message() != "provider invocation failed" {
		t.Fatalf("failure = %q %q", failure.Kind(), failure.Message())
	}
	if want := []string{"RUN:STARTED", "CLOSE"}; !equalStrings(destination.order, want) {
		t.Fatalf("destination order = %v, want %v", destination.order, want)
	}
}

func TestExecuteInvocationStopsAfterDestinationWriteFailure(t *testing.T) {
	t.Parallel()
	sinkErr := errors.New("response sink failed")
	destination := &failingWriter{err: sinkErr}
	var lateWrite, lateClose error
	integration := lifecycleIntegration{invoke: func(ctx context.Context, request contract.InvocationRequest, writer contract.ResponseWriter) error {
		writeErr := writer.WriteEvent(ctx, event(t, request.InvocationID(), workers.KindRun, workers.PhaseStarted, "", runPayload(t, "started")))
		lateWrite = writer.WriteEvent(ctx, event(t, request.InvocationID(), workers.KindRun, workers.PhaseCompleted, "", runPayload(t, "completed")))
		lateClose = writer.Close(ctx, contract.FailedCompletion(contract.NewFailure(contract.FailureInput{
			Kind: contract.FailureDependency, Message: "response sink failed",
		})))
		return writeErr
	}}

	err := contract.ExecuteInvocation(context.Background(), integration, request(), destination)
	if !errors.Is(err, sinkErr) {
		t.Fatalf("ExecuteInvocation() error = %v, want response sink error", err)
	}
	assertProtocolRule(t, lateWrite, "write_after_close")
	assertProtocolRule(t, lateClose, "duplicate_close")
	if destination.writes != 1 || destination.closes != 0 {
		t.Fatalf("destination calls = %d writes, %d closes; want one failed write and no close", destination.writes, destination.closes)
	}
}

func TestExecuteInvocationPreservesIgnoredDestinationFailure(t *testing.T) {
	t.Parallel()
	sinkErr := errors.New("response sink failed")
	integration := lifecycleIntegration{invoke: func(ctx context.Context, request contract.InvocationRequest, writer contract.ResponseWriter) error {
		_ = writer.WriteEvent(ctx, event(t, request.InvocationID(), workers.KindRun, workers.PhaseStarted, "", runPayload(t, "started")))
		return nil
	}}

	err := contract.ExecuteInvocation(context.Background(), integration, request(), &failingWriter{err: sinkErr})
	if !errors.Is(err, sinkErr) {
		t.Fatalf("ExecuteInvocation() error = %v, want ignored response sink error", err)
	}
}

func TestExecuteInvocationPreservesIgnoredProtocolValidationFailure(t *testing.T) {
	t.Parallel()
	destination := &orderedWriter{}
	var writeErr, closeErr error
	integration := lifecycleIntegration{invoke: func(ctx context.Context, request contract.InvocationRequest, writer contract.ResponseWriter) error {
		writeErr = writer.WriteEvent(ctx, event(t, "other-invocation", workers.KindRun, workers.PhaseStarted, "", runPayload(t, "started")))
		closeErr = writer.Close(ctx, contract.SuccessfulCompletion(contract.NewResponse(contract.ResponseInput{Content: "answer"})))
		return nil
	}}

	err := contract.ExecuteInvocation(context.Background(), integration, request(), destination)
	assertProtocolRule(t, writeErr, "invocation_correlation")
	assertProtocolRule(t, closeErr, "duplicate_close")
	assertProtocolRule(t, err, "invocation_correlation")
	if len(destination.order) != 0 || destination.closes != 0 {
		t.Fatalf("destination received output after invalid draft: order = %v, closes = %d", destination.order, destination.closes)
	}
}

func TestExecuteInvocationRejectsMissingAndDuplicateClose(t *testing.T) {
	t.Parallel()
	t.Run("missing", func(t *testing.T) {
		destination := &orderedWriter{}
		err := contract.ExecuteInvocation(context.Background(), lifecycleIntegration{invoke: func(context.Context, contract.InvocationRequest, contract.ResponseWriter) error {
			return nil
		}}, request(), destination)
		assertProtocolRule(t, err, "missing_close")
		if destination.completion == nil || destination.completion.Failure().Kind() != contract.FailureMalformedOutput {
			t.Fatalf("completion = %#v, want malformed output failure", destination.completion)
		}
	})
	t.Run("duplicate", func(t *testing.T) {
		var secondClose error
		completion := contract.SuccessfulCompletion(contract.NewResponse(contract.ResponseInput{Content: "answer"}))
		err := contract.ExecuteInvocation(context.Background(), lifecycleIntegration{invoke: func(ctx context.Context, _ contract.InvocationRequest, writer contract.ResponseWriter) error {
			if err := writer.Close(ctx, completion); err != nil {
				return err
			}
			secondClose = writer.Close(ctx, completion)
			return secondClose
		}}, request(), &orderedWriter{})
		assertProtocolRule(t, secondClose, "duplicate_close")
		assertProtocolRule(t, err, "duplicate_close")
	})
}

func TestProtocolWriterRejectsWriteAfterCloseWithoutReplacingCompletion(t *testing.T) {
	t.Parallel()
	destination := &orderedWriter{}
	var lateWrite error
	integration := lifecycleIntegration{invoke: func(ctx context.Context, request contract.InvocationRequest, writer contract.ResponseWriter) error {
		if err := writer.Close(ctx, contract.SuccessfulCompletion(contract.NewResponse(contract.ResponseInput{Content: "answer"}))); err != nil {
			return err
		}
		lateWrite = writer.WriteEvent(ctx, event(t, request.InvocationID(), workers.KindMessage, workers.PhaseCompleted, "message-1", messagePayload(t, "answer")))
		return lateWrite
	}}
	err := contract.ExecuteInvocation(context.Background(), integration, request(), destination)
	assertProtocolRule(t, lateWrite, "write_after_close")
	assertProtocolRule(t, err, "write_after_close")
	if destination.closes != 1 || destination.completion.Response() == nil {
		t.Fatalf("destination completion = %#v, closes = %d", destination.completion, destination.closes)
	}
}

type orderedWriter struct {
	order      []string
	completion *contract.Completion
	closes     int
}

type failingWriter struct {
	err    error
	writes int
	closes int
}

func (w *failingWriter) WriteEvent(context.Context, contract.EventDraft) error {
	w.writes++
	return w.err
}

func (w *failingWriter) Close(context.Context, contract.Completion) error {
	w.closes++
	return nil
}

func (w *orderedWriter) WriteEvent(_ context.Context, event contract.EventDraft) error {
	draft := event.Draft()
	w.order = append(w.order, string(draft.Kind)+":"+string(draft.Phase))
	return nil
}
func (w *orderedWriter) Close(_ context.Context, completion contract.Completion) error {
	w.order = append(w.order, "CLOSE")
	w.closes++
	w.completion = &completion
	return nil
}

func request() contract.InvocationRequest {
	return contract.NewInvocationRequest(contract.InvocationInput{InvocationID: "invocation-1", Model: "fixture", UserMessage: "hello"})
}

func event(t *testing.T, runID string, kind workers.Kind, phase workers.Phase, itemID string, payload []byte) contract.EventDraft {
	t.Helper()
	event, err := contract.NewEventDraft(contract.EventDraftInput{
		RunID: runID, Kind: kind, Phase: phase, ItemID: itemID, Payload: payload,
		ProviderSessionRef: "session-1", TurnID: "turn-1",
		Provenance: workers.Provenance{Delivery: workers.DeliveryNativeStream, Fidelity: workers.FidelityNormalized, NativeEventType: "fixture", Provider: "customer.lifecycle", Representation: workers.RepresentationSnapshot},
	})
	if err != nil {
		t.Fatal(err)
	}
	return event
}

func eventFactory(t *testing.T, runID string, kind workers.Kind, phase workers.Phase, itemID string, payload []byte) func(contract.InvocationRequest) contract.EventDraft {
	return func(contract.InvocationRequest) contract.EventDraft {
		return event(t, runID, kind, phase, itemID, payload)
	}
}

func writeOne(factory func(contract.InvocationRequest) contract.EventDraft) invocationFunc {
	return func(ctx context.Context, request contract.InvocationRequest, writer contract.ResponseWriter) error {
		return writer.WriteEvent(ctx, factory(request))
	}
}

func runPayload(t *testing.T, status string) []byte {
	return marshal(t, workers.RunPayload{Status: status})
}
func messagePayload(t *testing.T, content string) []byte {
	return marshal(t, workers.MessagePayload{Role: "assistant", ContentBlocks: []workers.ContentBlock{{Kind: workers.ContentBlockText, Text: content}}})
}
func messageDeltaPayload(t *testing.T, content string) []byte {
	return marshal(t, workers.MessageDeltaPayload{ContentBlockIndex: 0, ContentBlockKind: workers.ContentBlockText, TextDelta: content})
}
func toolPayload(t *testing.T, id, name, status string) []byte {
	return marshal(t, workers.ToolPayload{ToolCallID: id, ToolName: name, Status: status})
}
func toolDeltaPayload(t *testing.T, id, output string) []byte {
	return marshal(t, workers.ToolDeltaPayload{ToolCallID: id, OutputDelta: output})
}
func marshal(t *testing.T, value any) []byte {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func assertProtocolRule(t *testing.T, err error, rule string) {
	t.Helper()
	var protocolErr *contract.ProtocolError
	if !errors.As(err, &protocolErr) || protocolErr.Rule != rule {
		t.Fatalf("error = %v, want protocol rule %q", err, rule)
	}
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
