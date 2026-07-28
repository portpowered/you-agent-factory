package run

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"sync"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

const (
	defaultResponseStreamProgressQueueCapacity = 64
	responseStreamProgressDrainTimeout         = 250 * time.Millisecond
)

func TestWriteInvocationError_WritesOneStandardErrorResponseForTerminalFailure(t *testing.T) {
	t.Parallel()

	var stderr strings.Builder
	handled := WriteInvocationError(&stderr, invocationCLIError{
		Code:      "INVOCATION_RUNTIME_FAILURE",
		Message:   "goal execution failed",
		SessionID: "session-failed",
		WorkID:    "work-failed",
	}, false)
	if !handled {
		t.Fatal("terminal invocation failure was not handled")
	}

	var response factoryapi.ErrorResponse
	if err := json.Unmarshal([]byte(stderr.String()), &response); err != nil {
		t.Fatalf("stderr is not one ErrorResponse: %v\n%s", err, stderr.String())
	}
	if response.Family != factoryapi.ErrorFamilyInternalServerError ||
		response.Code != factoryapi.ErrorResponseCode("INVOCATION_RUNTIME_FAILURE") {
		t.Fatalf("ErrorResponse = %#v", response)
	}
	if response.Message != "goal execution failed [session=session-failed workId=work-failed]" {
		t.Fatalf("message = %q", response.Message)
	}
}

func TestMapCurrentFactoryFailureWritesDeclaredStandardErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantCode   factoryapi.ErrorResponseCode
		wantFamily factoryapi.ErrorFamily
	}{
		{
			name:       "missing",
			err:        fmt.Errorf("load Current Factory: %w", fs.ErrNotExist),
			wantCode:   CurrentFactoryNotFoundCode,
			wantFamily: factoryapi.ErrorFamilyNotFound,
		},
		{
			name:       "invalid",
			err:        errors.New("parse Current Factory: malformed JSON"),
			wantCode:   CurrentFactoryInvalidCode,
			wantFamily: factoryapi.ErrorFamilyBadRequest,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var stderr bytes.Buffer
			mapped := MapCurrentFactoryFailure(test.err)
			if !WriteInvocationError(&stderr, mapped, false) {
				t.Fatal("WriteInvocationError did not recognize Current Factory failure")
			}
			var response factoryapi.ErrorResponse
			if err := json.Unmarshal(bytes.TrimSpace(stderr.Bytes()), &response); err != nil {
				t.Fatalf("decode ErrorResponse: %v\n%s", err, stderr.String())
			}
			if response.Code != test.wantCode || response.Family != test.wantFamily {
				t.Fatalf("ErrorResponse = %#v", response)
			}
		})
	}
}

func TestMapServerFailureWritesDeclaredStandardError(t *testing.T) {
	t.Parallel()

	cause := &platformhttpserver.BindError{
		Host: "127.0.0.1", PreferredPort: 65534, Cause: errors.New("address in use"),
	}
	mapped := MapServerFailure(fmt.Errorf("host runtime: %w", cause))
	var stderr bytes.Buffer
	if !WriteInvocationError(&stderr, mapped, false) {
		t.Fatal("WriteInvocationError did not recognize server bind failure")
	}
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal(stderr.Bytes(), &response); err != nil {
		t.Fatalf("decode ErrorResponse: %v\n%s", err, stderr.String())
	}
	if response.Code != factoryapi.ErrorResponseCode(ServerBindFailedCode) ||
		response.Family != factoryapi.ErrorFamilyInternalServerError {
		t.Fatalf("ErrorResponse = %#v", response)
	}
}

func TestMapInvocationFailure_PreservesCancellationCode(t *testing.T) {
	t.Parallel()

	err := MapInvocationFailure(context.Canceled)
	var invocationErr *InvocationError
	if !errors.As(err, &invocationErr) {
		t.Fatalf("error = %T, want InvocationError", err)
	}
	if invocationErr.Code != InvocationErrorCodeCancelled || !errors.Is(err, context.Canceled) {
		t.Fatalf("InvocationError = %#v", invocationErr)
	}
}

func TestHumanFactoryEventRenderer_CustomerLifecycleGolden(t *testing.T) {
	t.Parallel()

	phaseName := "synthesize"
	label := "release review"
	providerResponse := "SECRET_PROVIDER_RESPONSE"
	providerOutput := "SECRET_PROVIDER_OUTPUT"
	events := []factorydefinitions.FactoryEvent{
		canonicalFactoryEventWithPayload(1, factorydefinitions.FactoryEventTypeWorkRequest, work.WorkRequestEventPayload{
			Works: []work.WorkRequestEventWork{{Name: "Review release"}},
		}),
		canonicalFactoryEventWithPayload(2, factorydefinitions.FactoryEventTypeSessionStarted, factorydefinitions.FactorySessionStartedEventPayload{}),
		canonicalFactoryEventWithPayload(3, factorydefinitions.FactoryEventTypeDispatchQueued, factorydefinitions.DispatchQueuedEventPayload{Label: &label}),
		canonicalFactoryEventWithPayload(4, factorydefinitions.FactoryEventTypeDispatchRequest, factorydefinitions.DispatchRequestEventPayload{TransitionID: "release review"}),
		canonicalFactoryEventWithPayload(5, factorydefinitions.FactoryEventTypeInferenceRequest, workerexecution.InferenceRequestEventPayload{Attempt: 1}),
		canonicalFactoryEventWithPayload(6, factorydefinitions.FactoryEventTypeInferenceResponse, workerexecution.InferenceResponseEventPayload{
			Attempt: 1, Outcome: workerexecution.InferenceOutcomeSucceeded, Response: &providerResponse,
		}),
		canonicalFactoryEventWithPayload(7, factorydefinitions.FactoryEventTypeDispatchResponse, workerexecution.DispatchResponseEventPayload{
			TransitionID: "release review", Outcome: workerexecution.OutcomeAccepted, Output: &providerOutput,
		}),
		canonicalFactoryEventWithPayload(8, factorydefinitions.FactoryEventTypeOrchestratorPhaseChanged, factorydefinitions.OrchestratorPhaseChangedEventPayload{
			PhaseStatus: factorydefinitions.OrchestratorPhaseStatusActive,
		}),
		canonicalFactoryEventWithPayload(9, factorydefinitions.FactoryEventTypeOrchestratorCheckpointWritten, factorydefinitions.OrchestratorCheckpointWrittenEventPayload{
			Label: "draft-ready", ResumabilityStatus: factorydefinitions.CheckpointResumabilityStatusResumable,
		}),
		canonicalFactoryEventWithPayload(10, factorydefinitions.FactoryEventTypeSessionResultUpdated, factorydefinitions.FactorySessionResultUpdatedEventPayload{
			ResultStatus: factorydefinitions.FactorySessionResultStatusFinal,
		}),
		canonicalFactoryEventWithPayload(11, factorydefinitions.FactoryEventTypeSessionCompleted, factorydefinitions.FactorySessionCompletedEventPayload{
			FinalStatus: factorydefinitions.FactorySessionLifecycleStatusSucceeded,
		}),
	}
	events[7].Context.PhaseName = &phaseName

	var output strings.Builder
	renderer := openTestHumanFactoryEventRenderer(t, &output, testResponsePresentation())
	renderer.PresentFactoryEvents(events)
	if err := renderer.WriteFinalInvocationResult(apisurface.FactoryInvocationResult{
		Status:        factorydefinitions.InvocationTerminalStatusCompleted,
		PrimaryResult: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "approved"}},
	}); err != nil {
		t.Fatalf("writeFinalInvocationResult: %v", err)
	}

	want := "[1] work accepted: Review release\n" +
		"[2] Factory Session started\n" +
		"[3] workstation queued: release review\n" +
		"[4] workstation started: release review\n" +
		"[5] inference started (attempt 1)\n" +
		"[6] inference completed (attempt 1)\n" +
		"[7] workstation completed: release review\n" +
		"[8] workflow phase synthesize: ACTIVE\n" +
		"[9] workflow checkpoint written: draft-ready (RESUMABLE)\n" +
		"[10] final output updated: FINAL\n" +
		"[11] Factory Session completed: SUCCEEDED\n\n" +
		responseStreamPrimaryResultHeader + "\napproved"
	if got := output.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	for _, forbidden := range []string{providerResponse, providerOutput, "INFERENCE_RESPONSE"} {
		if strings.Contains(output.String(), forbidden) {
			t.Fatalf("human lifecycle exposed private provider value %q: %s", forbidden, output.String())
		}
	}
}

func TestJSONFactoryEventRenderer_EmitsDiscriminatedSafeNDJSON(t *testing.T) {
	const providerCanary = "PRIVATE_PROVIDER_CHUNK_71f2"
	providerResponse := providerCanary
	events := append(canonicalJavaScriptFactoryEvents(), canonicalFactoryEventWithPayload(
		4,
		factorydefinitions.FactoryEventTypeInferenceResponse,
		workerexecution.InferenceResponseEventPayload{
			Diagnostics:     json.RawMessage(`{"schemaVersion":"agent-factory.response-event.v1","textDelta":"PRIVATE_PROVIDER_CHUNK_71f2"}`),
			ProviderSession: &workerexecution.ProviderSessionMetadata{Provider: "codex", ID: providerCanary},
			Response:        &providerResponse,
		},
	))

	var output strings.Builder
	renderer := openTestJSONFactoryEventRenderer(t, &output, testResponsePresentation())
	renderer.PresentFactoryEvents(events)
	if err := renderer.WriteFinalInvocationResult(apisurface.FactoryInvocationResult{
		RequestID: "request-ndjson", Status: factorydefinitions.InvocationTerminalStatusCompleted,
		PrimaryResult: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "complete"}},
	}); err != nil {
		t.Fatalf("writeFinalInvocationResult: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != len(events)+1 {
		t.Fatalf("NDJSON records = %d, want %d:\n%s", len(lines), len(events)+1, output.String())
	}
	for index, line := range lines[:len(events)] {
		assertFactoryEventNDJSONRecord(t, line, events[index], index)
	}
	assertInvocationResultNDJSONRecord(t, lines[len(lines)-1])
	for _, forbidden := range []string{providerCanary, "textDelta", "providerSession", "FactoryResponseEvent"} {
		if strings.Contains(output.String(), forbidden) {
			t.Fatalf("NDJSON exposed provider-only value %q:\n%s", forbidden, output.String())
		}
	}
	if !strings.Contains(string(events[len(events)-1].Payload), providerCanary) {
		t.Fatal("presentation redaction mutated canonical Factory Event history")
	}
}

func assertFactoryEventNDJSONRecord(
	t *testing.T, line string, want factorydefinitions.FactoryEvent, index int,
) {
	t.Helper()
	var record map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &record); err != nil {
		t.Fatalf("decode event record %d: %v", index, err)
	}
	if len(record) != 2 || string(record["recordType"]) != `"factory_event"` || len(record["event"]) == 0 {
		t.Fatalf("event record %d has invalid discriminator shape: %s", index, line)
	}
	var event factorydefinitions.FactoryEvent
	if err := json.Unmarshal(record["event"], &event); err != nil {
		t.Fatalf("decode event %d: %v", index, err)
	}
	if event.Context.Sequence != want.Context.Sequence || event.Context.SessionSequence == nil ||
		*event.Context.SessionSequence != *want.Context.SessionSequence {
		t.Fatalf("event %d sequence context changed: %#v", index, event.Context)
	}
}

func assertInvocationResultNDJSONRecord(t *testing.T, line string) {
	t.Helper()
	var record map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &record); err != nil {
		t.Fatalf("decode terminal record: %v", err)
	}
	if len(record) != 2 || string(record["recordType"]) != `"invocation_result"` || len(record["response"]) == 0 {
		t.Fatalf("terminal record has invalid discriminator shape: %s", line)
	}
}

func TestJSONFactoryEventRenderer_WithoutTerminalWritesOnlyFactoryEvents(t *testing.T) {
	var output strings.Builder
	renderer := openTestJSONFactoryEventRenderer(t, &output, testResponsePresentation())
	renderer.PresentFactoryEvents(canonicalJavaScriptFactoryEvents())
	renderer.StopProgressRendering()

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("NDJSON records = %d, want 3 Factory Events:\n%s", len(lines), output.String())
	}
	for _, line := range lines {
		if !strings.Contains(line, `"recordType":"factory_event"`) || strings.Contains(line, "invocation_result") {
			t.Fatalf("unexpected no-terminal record: %s", line)
		}
	}
}

// testResponsePresentation is the service-root test edge used by transport
// encoding tests. Queue and attachment invariants are owned and tested in the
// Factory Visualization package.
func testResponsePresentation() factoryvisualization.ResponsePresentation {
	return fakeResponsePresentation{}
}

type fakeResponsePresentation struct{}

func (fakeResponsePresentation) OpenBestEffortOutput(writer io.Writer) factoryvisualization.Output {
	return &fakePresentationOutput{writer: writer, capacity: defaultResponseStreamProgressQueueCapacity}
}

func (fakeResponsePresentation) OpenLosslessOutput(writer io.Writer) factoryvisualization.Output {
	return &fakePresentationOutput{writer: writer}
}

func (fakeResponsePresentation) OpenBestEffortFactoryEventStream(
	writer io.Writer,
	encode factoryvisualization.FactoryEventEncoder,
) interface {
	PresentFactoryEvents([]factorydefinitions.FactoryEvent)
	Finalize(factoryvisualization.FinalResponseWriter) (bool, error)
	CloseAndDrain() error
} {
	return &fakeFactoryEventStream{
		output: &fakePresentationOutput{writer: writer, capacity: defaultResponseStreamProgressQueueCapacity},
		encode: encode,
	}
}

func (fakeResponsePresentation) OpenLosslessFactoryEventStream(
	writer io.Writer,
	encode factoryvisualization.FactoryEventEncoder,
) interface {
	PresentFactoryEvents([]factorydefinitions.FactoryEvent)
	Finalize(factoryvisualization.FinalResponseWriter) (bool, error)
	CloseAndDrain() error
} {
	return &fakeFactoryEventStream{
		output: &fakePresentationOutput{writer: writer},
		encode: encode,
	}
}

type fakeFactoryEventStream struct {
	mu           sync.Mutex
	output       factoryvisualization.Output
	encode       factoryvisualization.FactoryEventEncoder
	progressSeen bool
	finalized    bool
	finalErr     error
}

func (s *fakeFactoryEventStream) PresentFactoryEvents(events []factorydefinitions.FactoryEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finalized {
		return
	}
	for _, event := range events {
		payload, ok := s.encode(event)
		if !ok || len(payload) == 0 {
			continue
		}
		if err := s.output.Enqueue(payload); err == nil {
			s.progressSeen = true
		}
	}
}

func (s *fakeFactoryEventStream) Finalize(write factoryvisualization.FinalResponseWriter) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finalized {
		return false, s.finalErr
	}
	s.finalized = true
	if err := s.output.CloseAndDrain(); err != nil {
		s.finalErr = err
		return true, err
	}
	s.finalErr = s.output.WithWriterExclusive(func(writer io.Writer) error {
		return write(writer, s.progressSeen)
	})
	return true, s.finalErr
}

func (s *fakeFactoryEventStream) CloseAndDrain() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.finalized {
		s.finalized = true
		s.finalErr = s.output.CloseAndDrain()
	}
	return s.finalErr
}

type fakePresentationOutput struct {
	mu       sync.Mutex
	writer   io.Writer
	capacity int
	pending  [][]byte
	dropped  int
	closed   bool
}

func (o *fakePresentationOutput) Enqueue(payload []byte) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return errors.New("fake response presentation output is closed")
	}
	if o.capacity == 0 {
		_, err := o.writer.Write(append(append([]byte(nil), payload...), '\n'))
		return err
	}
	if o.capacity > 0 && len(o.pending) >= o.capacity {
		o.dropped++
		return errors.New("fake response presentation output backlog is full")
	}
	o.pending = append(o.pending, append([]byte(nil), payload...))
	return nil
}

func (o *fakePresentationOutput) CloseAndDrain() error {
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return nil
	}
	o.closed = true
	pending := append([][]byte(nil), o.pending...)
	o.pending = nil
	o.mu.Unlock()
	for _, payload := range pending {
		if _, err := o.writer.Write(append(payload, '\n')); err != nil {
			return err
		}
	}
	return nil
}

func (o *fakePresentationOutput) WithWriterExclusive(write func(io.Writer) error) error {
	return write(o.writer)
}

func (o *fakePresentationOutput) Dropped() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.dropped
}
