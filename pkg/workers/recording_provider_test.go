package workers

import (
	"context"
	"errors"
	"testing"
	"time"

	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	"github.com/portpowered/agent-factory/pkg/interfaces"
)

type recordingProviderFake struct {
	responses []interfaces.InferenceResponse
	errors    []error
	calls     []interfaces.ProviderInferenceRequest
}

func (p *recordingProviderFake) Infer(_ context.Context, req interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, error) {
	p.calls = append(p.calls, interfaces.CloneProviderInferenceRequest(req))
	idx := len(p.calls) - 1
	var resp interfaces.InferenceResponse
	if idx < len(p.responses) {
		resp = p.responses[idx]
	}
	var err error
	if idx < len(p.errors) {
		err = p.errors[idx]
	}
	return resp, err
}

func TestRecordingProvider_Infer_SuccessEmitsRequestAndResponseEventsInOrder(t *testing.T) {
	fake := &recordingProviderFake{
		responses: []interfaces.InferenceResponse{{Content: "provider response"}},
	}
	events := &recordingEvents{}
	provider := NewRecordingProvider(fake, events.record, WithRecordingProviderClock(sequenceClock(
		time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC),
		5*time.Millisecond,
	)))

	resp, err := provider.Infer(context.Background(), recordingProviderDispatch())
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}
	if resp.Content != "provider response" {
		t.Fatalf("response content = %q, want provider response", resp.Content)
	}
	if len(events.items) != 2 {
		t.Fatalf("recorded events = %d, want 2", len(events.items))
	}

	request := assertInferenceRequestEvent(t, events.items[0])
	response := assertInferenceResponseEvent(t, events.items[1])
	if request.Attempt != 1 || response.Attempt != 1 {
		t.Fatalf("attempts = request %d response %d, want 1", request.Attempt, response.Attempt)
	}
	if request.InferenceRequestId != response.InferenceRequestId {
		t.Fatalf("inference request ids differ: %q vs %q", request.InferenceRequestId, response.InferenceRequestId)
	}
	if request.WorkingDirectory != "C:\\repo" || request.Worktree != "feature-worktree" || request.Prompt != "rendered prompt" {
		t.Fatalf("request payload = %#v", request)
	}
	if response.Outcome != factoryapi.InferenceOutcomeSucceeded {
		t.Fatalf("response outcome = %s, want SUCCEEDED", response.Outcome)
	}
	if response.Response == nil || *response.Response != "provider response" {
		t.Fatalf("response text = %#v, want provider response", response.Response)
	}
	if response.DurationMillis != 5 {
		t.Fatalf("durationMillis = %d, want 5", response.DurationMillis)
	}
	assertInferenceEventContext(t, events.items[0].Context)
	assertInferenceEventContext(t, events.items[1].Context)
}

func TestRecordingProvider_Infer_FailureEmitsFailedResponseWithProviderDetails(t *testing.T) {
	providerErr := NewProviderError(interfaces.ProviderErrorTypeTimeout, "provider timed out", nil)
	providerErr.Diagnostics = &interfaces.WorkDiagnostics{
		Command: &interfaces.CommandDiagnostic{ExitCode: 124},
	}
	fake := &recordingProviderFake{errors: []error{providerErr}}
	events := &recordingEvents{}
	provider := NewRecordingProvider(fake, events.record, WithRecordingProviderClock(sequenceClock(
		time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC),
		17*time.Millisecond,
	)))

	_, err := provider.Infer(context.Background(), recordingProviderDispatch())
	if !errors.Is(err, providerErr) {
		t.Fatalf("Infer error = %v, want provider error", err)
	}
	if len(events.items) != 2 {
		t.Fatalf("recorded events = %d, want 2", len(events.items))
	}

	request := assertInferenceRequestEvent(t, events.items[0])
	response := assertInferenceResponseEvent(t, events.items[1])
	if response.InferenceRequestId != request.InferenceRequestId {
		t.Fatalf("response inferenceRequestId = %q, want %q", response.InferenceRequestId, request.InferenceRequestId)
	}
	if response.Outcome != factoryapi.InferenceOutcomeFailed {
		t.Fatalf("response outcome = %s, want FAILED", response.Outcome)
	}
	if response.ErrorClass == nil || *response.ErrorClass != string(interfaces.ProviderErrorTypeTimeout) {
		t.Fatalf("errorClass = %#v, want timeout", response.ErrorClass)
	}
	if response.ExitCode == nil || *response.ExitCode != 124 {
		t.Fatalf("exitCode = %#v, want 124", response.ExitCode)
	}
	if response.Response != nil {
		t.Fatalf("failed response text = %#v, want nil", response.Response)
	}
	if response.DurationMillis != 17 {
		t.Fatalf("durationMillis = %d, want 17", response.DurationMillis)
	}
}

func TestRecordingProvider_Infer_FailureExitCodeEmissionMatchesDiagnosticPolicy(t *testing.T) {
	testCases := []struct {
		name         string
		diagnostics  *interfaces.WorkDiagnostics
		wantExitCode *int
	}{
		{
			name:         "omits without command diagnostics",
			diagnostics:  nil,
			wantExitCode: nil,
		},
		{
			name: "omits zero exit code",
			diagnostics: &interfaces.WorkDiagnostics{
				Command: &interfaces.CommandDiagnostic{ExitCode: 0},
			},
			wantExitCode: nil,
		},
		{
			name: "emits nonzero exit code",
			diagnostics: &interfaces.WorkDiagnostics{
				Command: &interfaces.CommandDiagnostic{ExitCode: 23},
			},
			wantExitCode: intPtr(23),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			providerErr := NewProviderError(interfaces.ProviderErrorTypeTimeout, "provider timed out", nil)
			providerErr.Diagnostics = tc.diagnostics
			fake := &recordingProviderFake{errors: []error{providerErr}}
			events := &recordingEvents{}
			provider := NewRecordingProvider(fake, events.record, WithRecordingProviderClock(sequenceClock(
				time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC),
				17*time.Millisecond,
			)))

			_, err := provider.Infer(context.Background(), recordingProviderDispatch())
			if !errors.Is(err, providerErr) {
				t.Fatalf("Infer error = %v, want provider error", err)
			}
			if len(events.items) != 2 {
				t.Fatalf("recorded events = %d, want 2", len(events.items))
			}

			response := assertInferenceResponseEvent(t, events.items[1])
			if tc.wantExitCode == nil {
				if response.ExitCode != nil {
					t.Fatalf("exitCode = %#v, want nil", response.ExitCode)
				}
				return
			}
			if response.ExitCode == nil || *response.ExitCode != *tc.wantExitCode {
				t.Fatalf("exitCode = %#v, want %d", response.ExitCode, *tc.wantExitCode)
			}
		})
	}
}

func TestRecordingProvider_Infer_MultipleAttemptsIncrementAndKeepUniqueRequestIDs(t *testing.T) {
	start := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	fake := &recordingProviderFake{
		errors: []error{
			NewProviderError(interfaces.ProviderErrorTypeInternalServerError, "provider 500", nil),
			NewProviderError(interfaces.ProviderErrorTypeTimeout, "provider timeout", nil),
			nil,
		},
		responses: []interfaces.InferenceResponse{
			{},
			{},
			{Content: "recovered"},
		},
	}
	events := &recordingEvents{}
	provider := NewRecordingProvider(fake, events.record, WithRecordingProviderClock(sequenceClock(
		start,
		time.Millisecond,
		start.Add(2*time.Millisecond),
		3*time.Millisecond,
		start.Add(4*time.Millisecond),
		5*time.Millisecond,
	)))

	dispatch := recordingProviderDispatch()
	for i := 0; i < 3; i++ {
		_, _ = provider.Infer(context.Background(), dispatch)
	}
	if len(events.items) != 6 {
		t.Fatalf("recorded events = %d, want 6", len(events.items))
	}

	seenIDs := map[string]bool{}
	for attempt := 1; attempt <= 3; attempt++ {
		request := assertInferenceRequestEvent(t, events.items[(attempt-1)*2])
		response := assertInferenceResponseEvent(t, events.items[(attempt-1)*2+1])
		if request.Attempt != attempt || response.Attempt != attempt {
			t.Fatalf("attempt %d payloads = request %d response %d", attempt, request.Attempt, response.Attempt)
		}
		if request.InferenceRequestId != response.InferenceRequestId {
			t.Fatalf("attempt %d request id mismatch", attempt)
		}
		if seenIDs[request.InferenceRequestId] {
			t.Fatalf("duplicate inferenceRequestId %q", request.InferenceRequestId)
		}
		seenIDs[request.InferenceRequestId] = true
	}
}

func recordingProviderDispatch() interfaces.ProviderInferenceRequest {
	dispatch := interfaces.WorkDispatch{
		DispatchID:   "dispatch-1",
		TransitionID: "transition-1",
		WorkerType:   "worker-a",
		Execution: interfaces.ExecutionMetadata{
			DispatchCreatedTick: 7,
			CurrentTick:         8,
			RequestID:           "request-1",
			TraceID:             "trace-1",
			WorkIDs:             []string{"work-1", "work-2"},
		},
	}
	return interfaces.ProviderInferenceRequest{
		Dispatch:         dispatch,
		WorkerType:       dispatch.WorkerType,
		WorkingDirectory: "C:\\repo",
		Worktree:         "feature-worktree",
		UserMessage:      "rendered prompt",
	}
}

type recordingEvents struct {
	items []factoryapi.FactoryEvent
}

func (r *recordingEvents) record(event factoryapi.FactoryEvent) {
	r.items = append(r.items, event)
}

func sequenceClock(values ...any) func() time.Time {
	times := make([]time.Time, 0, len(values))
	var current time.Time
	for _, value := range values {
		switch typed := value.(type) {
		case time.Time:
			current = typed
		case time.Duration:
			current = current.Add(typed)
		}
		times = append(times, current)
	}
	idx := 0
	return func() time.Time {
		if idx >= len(times) {
			return times[len(times)-1]
		}
		value := times[idx]
		idx++
		return value
	}
}

func assertInferenceRequestEvent(t *testing.T, event factoryapi.FactoryEvent) factoryapi.InferenceRequestEventPayload {
	t.Helper()
	if event.SchemaVersion != factoryapi.AgentFactoryEventV1 {
		t.Fatalf("schemaVersion = %q, want %q", event.SchemaVersion, factoryapi.AgentFactoryEventV1)
	}
	if event.Type != factoryapi.FactoryEventTypeInferenceRequest {
		t.Fatalf("event type = %s, want INFERENCE_REQUEST", event.Type)
	}
	payload, err := event.Payload.AsInferenceRequestEventPayload()
	if err != nil {
		t.Fatalf("request payload decode: %v", err)
	}
	return payload
}

func assertInferenceResponseEvent(t *testing.T, event factoryapi.FactoryEvent) factoryapi.InferenceResponseEventPayload {
	t.Helper()
	if event.SchemaVersion != factoryapi.AgentFactoryEventV1 {
		t.Fatalf("schemaVersion = %q, want %q", event.SchemaVersion, factoryapi.AgentFactoryEventV1)
	}
	if event.Type != factoryapi.FactoryEventTypeInferenceResponse {
		t.Fatalf("event type = %s, want INFERENCE_RESPONSE", event.Type)
	}
	payload, err := event.Payload.AsInferenceResponseEventPayload()
	if err != nil {
		t.Fatalf("response payload decode: %v", err)
	}
	return payload
}

func assertInferenceEventContext(t *testing.T, context factoryapi.FactoryEventContext) {
	t.Helper()
	if context.Tick != 8 {
		t.Fatalf("context tick = %d, want 8", context.Tick)
	}
	if context.DispatchId == nil || *context.DispatchId != "dispatch-1" {
		t.Fatalf("context dispatchId = %#v, want dispatch-1", context.DispatchId)
	}
	if context.RequestId == nil || *context.RequestId != "request-1" {
		t.Fatalf("context requestId = %#v, want request-1", context.RequestId)
	}
	if context.TraceIds == nil || len(*context.TraceIds) != 1 || (*context.TraceIds)[0] != "trace-1" {
		t.Fatalf("context traceIds = %#v, want trace-1", context.TraceIds)
	}
	if context.WorkIds == nil || len(*context.WorkIds) != 2 || (*context.WorkIds)[0] != "work-1" || (*context.WorkIds)[1] != "work-2" {
		t.Fatalf("context workIds = %#v, want work-1/work-2", context.WorkIds)
	}
}
