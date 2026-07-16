package provider

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/work"
)

type recordingProviderFake struct {
	responses []workerexecution.InferenceResponse
	errors    []error
	calls     []workerexecution.ProviderInferenceRequest
}

func (p *recordingProviderFake) Infer(_ context.Context, req workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	p.calls = append(p.calls, workerexecution.CloneProviderInferenceRequest(req))
	idx := len(p.calls) - 1
	var resp workerexecution.InferenceResponse
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
		responses: []workerexecution.InferenceResponse{{Content: "provider response"}},
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
	assertInferenceEventContext(t, events.items[0])
	assertInferenceEventContext(t, events.items[1])
}

func TestRecordingProvider_Infer_NormalizesEventTimesToUTC(t *testing.T) {
	localZone := time.FixedZone("Provider/Local", 3*60*60)
	events := &recordingEvents{}
	provider := NewRecordingProvider(
		&recordingProviderFake{responses: []workerexecution.InferenceResponse{{Content: "provider response"}}},
		events.record,
		WithRecordingProviderClock(sequenceClock(
			time.Date(2026, 4, 18, 15, 0, 0, 0, localZone),
			25*time.Millisecond,
		)),
	)

	_, err := provider.Infer(context.Background(), recordingProviderDispatch())
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}
	if len(events.items) != 2 {
		t.Fatalf("recorded events = %d, want 2", len(events.items))
	}
	assertProviderEventTimeJSON(t, events.items[0], "2026-04-18T12:00:00Z")
	assertProviderEventTimeJSON(t, events.items[1], "2026-04-18T12:00:00.025Z")
}

func TestRecordingProvider_Infer_FailureEmitsFailedResponseWithProviderDetails(t *testing.T) {
	providerErr := NewProviderError(workerexecution.WorkFailureTypeTimeout, "provider timed out", nil)
	providerErr.Diagnostics = &workerexecution.WorkDiagnostics{
		Command: &workerexecution.CommandDiagnostic{ExitCode: 124},
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
	if response.FailureDetail == nil || response.FailureDetail.Reason != factoryapi.WorkFailureTypeTimeout {
		t.Fatalf("failureDetail = %#v, want timeout", response.FailureDetail)
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
		diagnostics  *workerexecution.WorkDiagnostics
		wantExitCode *int
	}{
		{
			name:         "omits without command diagnostics",
			diagnostics:  nil,
			wantExitCode: nil,
		},
		{
			name: "omits zero exit code",
			diagnostics: &workerexecution.WorkDiagnostics{
				Command: &workerexecution.CommandDiagnostic{ExitCode: 0},
			},
			wantExitCode: nil,
		},
		{
			name: "emits nonzero exit code",
			diagnostics: &workerexecution.WorkDiagnostics{
				Command: &workerexecution.CommandDiagnostic{ExitCode: 23},
			},
			wantExitCode: intPtr(23),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			providerErr := NewProviderError(workerexecution.WorkFailureTypeTimeout, "provider timed out", nil)
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

func TestRecordingProvider_Infer_SuccessPreservesProviderSessionAndSafeDiagnostics(t *testing.T) {
	respSession := &workerexecution.ProviderSessionMetadata{Provider: "claude", Kind: "session_id", ID: "sess-123"}
	fake := &recordingProviderFake{
		responses: []workerexecution.InferenceResponse{{
			Content:         "provider response",
			ProviderSession: respSession,
			Diagnostics: &workerexecution.WorkDiagnostics{
				Provider: &workerexecution.ProviderDiagnostic{
					Provider: "claude",
					Model:    "claude-sonnet-4",
					RequestMetadata: map[string]string{
						"worker_type": "worker-a",
						"unsafe":      "drop-me",
					},
					ResponseMetadata: map[string]string{
						"source": "provider",
						"unsafe": "drop-me",
					},
				},
			},
		}},
	}
	events := &recordingEvents{}
	provider := NewRecordingProvider(fake, events.record, WithRecordingProviderClock(sequenceClock(
		time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC),
		5*time.Millisecond,
	)))

	_, err := provider.Infer(context.Background(), recordingProviderDispatch())
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}

	response := assertInferenceResponseEvent(t, events.items[1])
	if response.ExitCode != nil {
		t.Fatalf("exitCode = %#v, want nil", response.ExitCode)
	}
	assertProviderSessionPayload(t, response.ProviderSession, "claude", "session_id", "sess-123")
	assertInferenceProviderDiagnostics(
		t,
		response.Diagnostics,
		"claude",
		"claude-sonnet-4",
		map[string]string{
			"worker_type":       "worker-a",
			"worktree":          "feature-worktree",
			"working_directory": "C:\\repo",
		},
		map[string]string{
			"content_bytes":             "17",
			"provider_session_id":       "sess-123",
			"provider_session_kind":     "session_id",
			"provider_session_provider": "claude",
			"retry_count":               "0",
			"source":                    "provider",
		},
	)
}

func TestRecordingProvider_Infer_CursorSessionMetadataIsCanonicalizedInEvents(t *testing.T) {
	respSession := &workerexecution.ProviderSessionMetadata{
		Provider: string(modelprovider.Cursor),
		Kind:     "session_id",
		ID:       "cursor-session-123",
	}
	fake := &recordingProviderFake{
		responses: []workerexecution.InferenceResponse{{
			Content:         "provider response",
			ProviderSession: respSession,
			Diagnostics: &workerexecution.WorkDiagnostics{
				Provider: &workerexecution.ProviderDiagnostic{
					Provider: string(modelprovider.Cursor),
					Model:    "gpt-5",
				},
			},
		}},
	}
	events := &recordingEvents{}
	provider := NewRecordingProvider(fake, events.record, WithRecordingProviderClock(sequenceClock(
		time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC),
		5*time.Millisecond,
	)))

	_, err := provider.Infer(context.Background(), recordingProviderDispatch())
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}

	response := assertInferenceResponseEvent(t, events.items[1])
	assertProviderSessionPayload(t, response.ProviderSession, "cursor", "session_id", "cursor-session-123")
	assertInferenceProviderDiagnostics(
		t,
		response.Diagnostics,
		string(modelprovider.Cursor),
		"gpt-5",
		map[string]string{
			"worker_type":       "worker-a",
			"worktree":          "feature-worktree",
			"working_directory": "C:\\repo",
		},
		map[string]string{
			"content_bytes":             "17",
			"provider_session_id":       "cursor-session-123",
			"provider_session_kind":     "session_id",
			"provider_session_provider": "cursor",
			"retry_count":               "0",
		},
	)
}

func TestRecordingProvider_Infer_FailureZeroExitCodeStillPreservesProviderSessionAndSafeDiagnostics(t *testing.T) {
	providerErr := NewProviderErrorWithSession(
		workerexecution.WorkFailureTypeTimeout,
		"provider timed out",
		nil,
		&workerexecution.ProviderSessionMetadata{Provider: "claude", Kind: "session_id", ID: "sess-456"},
	)
	providerErr.Diagnostics = &workerexecution.WorkDiagnostics{
		Provider: &workerexecution.ProviderDiagnostic{
			Provider: "claude",
			Model:    "claude-sonnet-4",
			ResponseMetadata: map[string]string{
				"source": "stderr",
				"unsafe": "drop-me",
			},
		},
		Command: &workerexecution.CommandDiagnostic{ExitCode: 0},
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

	response := assertInferenceResponseEvent(t, events.items[1])
	if response.ExitCode != nil {
		t.Fatalf("exitCode = %#v, want nil", response.ExitCode)
	}
	assertProviderSessionPayload(t, response.ProviderSession, "claude", "session_id", "sess-456")
	assertInferenceProviderDiagnostics(
		t,
		response.Diagnostics,
		"claude",
		"claude-sonnet-4",
		map[string]string{
			"worker_type":       "worker-a",
			"worktree":          "feature-worktree",
			"working_directory": "C:\\repo",
		},
		map[string]string{
			"retry_count": "0",
			"source":      "stderr",
		},
	)
}

func TestRecordingProvider_Infer_MultipleAttemptsIncrementAndKeepUniqueRequestIDs(t *testing.T) {
	start := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	fake := &recordingProviderFake{
		errors: []error{
			NewProviderError(workerexecution.WorkFailureTypeInternalServerError, "provider 500", nil),
			NewProviderError(workerexecution.WorkFailureTypeTimeout, "provider timeout", nil),
			nil,
		},
		responses: []workerexecution.InferenceResponse{
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

func TestRecordingProvider_Infer_RetryableFailureKeepsAttemptCounterUntilTerminalOutcome(t *testing.T) {
	start := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	fake := &recordingProviderFake{
		errors: []error{
			NewProviderError(workerexecution.WorkFailureTypeTimeout, "provider timed out", nil),
			NewProviderError(workerexecution.WorkFailureTypePermanentBadRequest, "prompt invalid", nil),
			nil,
		},
		responses: []workerexecution.InferenceResponse{
			{},
			{},
			{Content: "fresh dispatch"},
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

	firstFailure := assertInferenceResponseEvent(t, events.items[1])
	secondFailure := assertInferenceResponseEvent(t, events.items[3])
	finalSuccess := assertInferenceResponseEvent(t, events.items[5])

	if firstFailure.Attempt != 1 || secondFailure.Attempt != 2 || finalSuccess.Attempt != 1 {
		t.Fatalf("attempt sequence = [%d %d %d], want [1 2 1]", firstFailure.Attempt, secondFailure.Attempt, finalSuccess.Attempt)
	}
	if firstFailure.FailureDetail == nil || firstFailure.FailureDetail.Reason != factoryapi.WorkFailureTypeTimeout {
		t.Fatalf("first failure detail = %#v, want timeout", firstFailure.FailureDetail)
	}
	if secondFailure.FailureDetail == nil || secondFailure.FailureDetail.Reason != factoryapi.WorkFailureTypePermanentBadRequest {
		t.Fatalf("second failure detail = %#v, want permanent bad request", secondFailure.FailureDetail)
	}
	if finalSuccess.Outcome != factoryapi.InferenceOutcomeSucceeded {
		t.Fatalf("final success outcome = %s, want SUCCEEDED", finalSuccess.Outcome)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this recording-provider regression keeps the misconfigured-provider event contract together in one scenario.
func TestRecordingProvider_Infer_MissingInnerProviderEmitsMisconfiguredFailureEvent(t *testing.T) {
	events := &recordingEvents{}
	provider := NewRecordingProvider(nil, events.record, WithRecordingProviderClock(sequenceClock(
		time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC),
		11*time.Millisecond,
	)))

	_, err := provider.Infer(context.Background(), recordingProviderDispatch())
	if err == nil {
		t.Fatal("expected Infer to fail")
	}

	var providerErr *ProviderError
	ok := errors.As(err, &providerErr)
	if !ok {
		t.Fatalf("expected ProviderError, got %T", err)
	}
	if providerErr.Type != workerexecution.WorkFailureTypeMisconfigured {
		t.Fatalf("provider error type = %q, want %q", providerErr.Type, workerexecution.WorkFailureTypeMisconfigured)
	}
	if providerErr.Message != "recording provider requires an inner provider" {
		t.Fatalf("provider error message = %q", providerErr.Message)
	}
	if len(events.items) != 2 {
		t.Fatalf("recorded events = %d, want 2", len(events.items))
	}

	response := assertInferenceResponseEvent(t, events.items[1])
	if response.Outcome != factoryapi.InferenceOutcomeFailed {
		t.Fatalf("response outcome = %s, want FAILED", response.Outcome)
	}
	if response.FailureDetail == nil || response.FailureDetail.Reason != factoryapi.WorkFailureTypeMisconfigured {
		t.Fatalf("failureDetail = %#v, want misconfigured", response.FailureDetail)
	}
	if response.ExitCode != nil {
		t.Fatalf("exitCode = %#v, want nil", response.ExitCode)
	}
	if response.Diagnostics == nil || response.Diagnostics.Provider == nil {
		t.Fatalf("diagnostics = %#v, want provider diagnostics", response.Diagnostics)
	}
	responseMetadata := recordingProviderStringMapValue(response.Diagnostics.Provider.ResponseMetadata)
	if responseMetadata["retry_count"] != "0" {
		t.Fatalf("retry_count = %q, want 0", responseMetadata["retry_count"])
	}
	requestMetadata := recordingProviderStringMapValue(response.Diagnostics.Provider.RequestMetadata)
	if requestMetadata["worker_type"] != "worker-a" || requestMetadata["working_directory"] != "C:\\repo" || requestMetadata["worktree"] != "feature-worktree" {
		t.Fatalf("request metadata = %#v, want worker_type/working_directory/worktree", requestMetadata)
	}
}

func recordingProviderDispatch() workerexecution.ProviderInferenceRequest {
	dispatch := work.WorkDispatch{
		DispatchID:   "dispatch-1",
		TransitionID: "transition-1",
		WorkerType:   "worker-a",
		Execution: work.ExecutionMetadata{
			DispatchCreatedTick: 7,
			CurrentTick:         8,
			RequestID:           "request-1",
			TraceID:             "trace-1",
			WorkIDs:             []string{"work-1", "work-2"},
		},
	}
	return workerexecution.ProviderInferenceRequest{
		Dispatch:         dispatch,
		WorkerType:       dispatch.WorkerType,
		WorkingDirectory: "C:\\repo",
		Worktree:         "feature-worktree",
		UserMessage:      "rendered prompt",
	}
}

type recordingEvents struct {
	items []workerexecution.InferenceEvent
}

func (r *recordingEvents) record(event workerexecution.InferenceEvent) {
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

func assertInferenceRequestEvent(t *testing.T, event workerexecution.InferenceEvent) factoryapi.InferenceRequestEventPayload {
	t.Helper()
	if event.Kind != workerexecution.InferenceEventKindRequest || event.Request == nil || event.Response != nil {
		t.Fatalf("event = %#v, want inference request", event)
	}
	encoded, err := json.Marshal(event.Request)
	if err != nil {
		t.Fatalf("request payload encode: %v", err)
	}
	var payload factoryapi.InferenceRequestEventPayload
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("request payload decode: %v", err)
	}
	return payload
}

func assertInferenceResponseEvent(t *testing.T, event workerexecution.InferenceEvent) factoryapi.InferenceResponseEventPayload {
	t.Helper()
	if event.Kind != workerexecution.InferenceEventKindResponse || event.Response == nil || event.Request != nil {
		t.Fatalf("event = %#v, want inference response", event)
	}
	encoded, err := json.Marshal(event.Response)
	if err != nil {
		t.Fatalf("response payload encode: %v", err)
	}
	var payload factoryapi.InferenceResponseEventPayload
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("response payload decode: %v", err)
	}
	return payload
}

func assertInferenceEventContext(t *testing.T, event workerexecution.InferenceEvent) {
	t.Helper()
	if event.Tick != 8 {
		t.Fatalf("tick = %d, want 8", event.Tick)
	}
	if event.DispatchID != "dispatch-1" {
		t.Fatalf("dispatchId = %q, want dispatch-1", event.DispatchID)
	}
	if event.RequestID != "request-1" {
		t.Fatalf("requestId = %q, want request-1", event.RequestID)
	}
	if len(event.TraceIDs) != 1 || event.TraceIDs[0] != "trace-1" {
		t.Fatalf("traceIds = %#v, want trace-1", event.TraceIDs)
	}
	if len(event.WorkIDs) != 2 || event.WorkIDs[0] != "work-1" || event.WorkIDs[1] != "work-2" {
		t.Fatalf("workIds = %#v, want work-1/work-2", event.WorkIDs)
	}
}

func assertProviderEventTimeJSON(t *testing.T, event workerexecution.InferenceEvent, want string) {
	t.Helper()
	if event.EventTime.Location() != time.UTC {
		t.Fatalf("event time location = %v, want UTC", event.EventTime.Location())
	}
	if got := event.EventTime.Format(time.RFC3339Nano); got != want {
		t.Fatalf("event time = %q, want %q", got, want)
	}
}

func assertProviderSessionPayload(t *testing.T, session *factoryapi.ProviderSessionMetadata, provider, kind, id string) {
	t.Helper()
	if session == nil {
		t.Fatal("providerSession = nil, want payload")
	}
	if recordingProviderStringValue(session.Provider) != provider ||
		recordingProviderStringValue(session.Kind) != kind ||
		recordingProviderStringValue(session.Id) != id {
		t.Fatalf("providerSession = %#v, want provider=%q kind=%q id=%q", session, provider, kind, id)
	}
}

func assertInferenceProviderDiagnostics(
	t *testing.T,
	diagnostics *factoryapi.SafeWorkDiagnostics,
	provider string,
	model string,
	wantRequest map[string]string,
	wantResponse map[string]string,
) {
	t.Helper()
	if diagnostics == nil || diagnostics.Provider == nil {
		t.Fatalf("diagnostics = %#v, want provider diagnostics", diagnostics)
	}
	if recordingProviderStringValue(diagnostics.Provider.Provider) != provider ||
		recordingProviderStringValue(diagnostics.Provider.Model) != model {
		t.Fatalf("provider diagnostics = %#v, want provider=%q model=%q", diagnostics.Provider, provider, model)
	}

	requestMetadata := recordingProviderStringMapValue(diagnostics.Provider.RequestMetadata)
	for key, want := range wantRequest {
		if requestMetadata[key] != want {
			t.Fatalf("request metadata[%q] = %q, want %q", key, requestMetadata[key], want)
		}
	}
	if _, ok := requestMetadata["unsafe"]; ok {
		t.Fatalf("request metadata leaked unsafe key: %#v", requestMetadata)
	}

	responseMetadata := recordingProviderStringMapValue(diagnostics.Provider.ResponseMetadata)
	for key, want := range wantResponse {
		if responseMetadata[key] != want {
			t.Fatalf("response metadata[%q] = %q, want %q", key, responseMetadata[key], want)
		}
	}
	if _, ok := responseMetadata["unsafe"]; ok {
		t.Fatalf("response metadata leaked unsafe key: %#v", responseMetadata)
	}
}

func recordingProviderStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func recordingProviderStringMapValue(values *factoryapi.StringMap) map[string]string {
	if values == nil {
		return nil
	}
	out := make(map[string]string, len(*values))
	for key, value := range *values {
		out[key] = value
	}
	return out
}

func TestInferenceProgressPublishingCommandRunner_NormalizesCodexStructuredEvents(t *testing.T) {
	scriptPath := writeProviderOutputFixture(t, filepath.Join(t.TempDir(), "codex"), []byte(
		"{\"event\":\"session.created\",\"session_id\":\"sess-codex-1\"}\n"+
			"{\"type\":\"response.output_text.delta\",\"delta\":\"hello from delta\"}\n"+
			"{\"type\":\"response.completed\",\"response\":{\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"hello final\"}]}]}}\n",
	), []byte("planning update\n"), 0)

	var published []InferenceProgressFragment
	var publishedMu sync.Mutex
	runner := NewInferenceProgressPublishingCommandRunner(func(fragment InferenceProgressFragment) {
		publishedMu.Lock()
		published = append(published, fragment)
		publishedMu.Unlock()
	}, nil)

	_, err := runner.Run(context.Background(), CommandRequest{
		Command:         scriptPath,
		DispatchID:      "dispatch-codex-json-1",
		WorkstationName: "review",
		Execution: work.ExecutionMetadata{
			WorkIDs: []string{"work-codex-json-1"},
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	publishedMu.Lock()
	defer publishedMu.Unlock()
	if len(published) != 4 {
		t.Fatalf("published fragments = %#v, want 4 normalized fragments", published)
	}

	startedFragment := fragmentByType(published, NormalizedEventTypeStarted)
	deltaFragment := fragmentByType(published, NormalizedEventTypeTextDelta)
	finalFragment := fragmentByType(published, NormalizedEventTypeFinalText)
	progressFragment := fragmentByType(published, NormalizedEventTypeProgress)

	assertCodexStartedFragment(t, startedFragment, "sess-codex-1", "work-codex-json-1")
	assertCodexResponseFragment(t, deltaFragment, NormalizedEventTypeTextDelta, "hello from delta")
	assertCodexResponseFragment(t, finalFragment, NormalizedEventTypeFinalText, "hello final")
	assertCodexProgressFragment(t, progressFragment, "planning update")
	if finalFragment.ProviderSessionRef == nil || finalFragment.ProviderSessionRef.ID != "sess-codex-1" {
		t.Fatalf("final provider session = %#v, want session propagated", finalFragment.ProviderSessionRef)
	}
}

func TestInferenceProgressPublishingCommandRunner_MapsUnknownAndMalformedCodexEventsToBoundedDiagnostics(t *testing.T) {
	scriptPath := writeProviderOutputFixture(t, filepath.Join(t.TempDir(), "codex"), []byte(
		"{\"event\":\"session.created\",\"session_id\":\"sess-codex-2\"}\n"+
			"{\"type\":\"response.mystery\",\"message\":\"secret-token-123 should never be retained\"}\n"+
			"{\"type\":\"response.progress\"\n"+
			"event: response.output_text.delta\n\n"+
			"event: response.output_text.delta\n"+
			"data: {\"delta\":\"hello after malformed frames\"}\n\n",
	), nil, 0)

	var published []InferenceProgressFragment
	var publishedMu sync.Mutex
	runner := NewInferenceProgressPublishingCommandRunner(func(fragment InferenceProgressFragment) {
		publishedMu.Lock()
		published = append(published, fragment)
		publishedMu.Unlock()
	}, nil)

	_, err := runner.Run(context.Background(), CommandRequest{
		Command:         scriptPath,
		DispatchID:      "dispatch-codex-json-2",
		WorkstationName: "review",
		Execution: work.ExecutionMetadata{
			WorkIDs: []string{"work-codex-json-2"},
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	publishedMu.Lock()
	defer publishedMu.Unlock()
	if len(published) != 5 {
		t.Fatalf("published fragments = %#v, want 5 fragments with bounded unknown diagnostics", published)
	}

	assertCodexStartedFragment(t, &published[0], "sess-codex-2", "work-codex-json-2")
	assertUnknownCodexDiagnostic(t, &published[1], "response.mystery", codexDiagnosticUnknownEvent)
	assertUnknownCodexDiagnostic(t, &published[2], "", codexDiagnosticMalformedJSON)
	assertUnknownCodexDiagnostic(t, &published[3], "response.output_text.delta", codexDiagnosticIncompleteSSE)
	assertCodexResponseFragment(t, &published[4], NormalizedEventTypeTextDelta, "hello after malformed frames")
	if published[4].ProviderSessionRef == nil || published[4].ProviderSessionRef.ID != "sess-codex-2" {
		t.Fatalf("final provider session = %#v, want session carried across malformed frames", published[4].ProviderSessionRef)
	}
}

func TestInferenceProgressPublishingCommandRunner_MapsFailureCancelAndTruncation(t *testing.T) {
	// Do not run in parallel: Linux CI can return "text file busy" when executing
	// the freshly written shell script under heavy parallel package load.

	progressPayload := strings.Repeat("p", codexRetainedProgressBytes+73)
	deltaPayload := strings.Repeat("d", codexRetainedTextBytes+29)
	finalPayload := strings.Repeat("f", codexRetainedTextBytes+41)
	failurePayload := strings.Repeat("e", codexRetainedProgressBytes+17)
	cancelPayload := strings.Repeat("c", codexRetainedProgressBytes+9)

	scriptPath := writeProviderOutputFixture(t, filepath.Join(t.TempDir(), "codex"), []byte(
		"{\"event\":\"session.created\",\"session_id\":\"sess-codex-3\"}\n"+
			"{\"type\":\"response.progress\",\"message\":\""+progressPayload+"\"}\n"+
			"{\"type\":\"response.output_text.delta\",\"delta\":\""+deltaPayload+"\"}\n"+
			"{\"type\":\"response.completed\",\"response\":{\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\""+finalPayload+"\"}]}]}}\n"+
			"{\"type\":\"response.failed\",\"error\":\""+failurePayload+"\"}\n"+
			"{\"type\":\"response.canceled\",\"status\":\""+cancelPayload+"\"}\n",
	), nil, 0)

	var published []InferenceProgressFragment
	var publishedMu sync.Mutex
	runner := NewInferenceProgressPublishingCommandRunner(func(fragment InferenceProgressFragment) {
		publishedMu.Lock()
		published = append(published, fragment)
		publishedMu.Unlock()
	}, nil)

	_, err := runner.Run(context.Background(), CommandRequest{
		Command:         scriptPath,
		DispatchID:      "dispatch-codex-json-3",
		WorkstationName: "review",
		Execution: work.ExecutionMetadata{
			WorkIDs: []string{"work-codex-json-3"},
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	publishedMu.Lock()
	defer publishedMu.Unlock()
	if len(published) != 6 {
		t.Fatalf("published fragments = %#v, want 6 normalized fragments", published)
	}

	assertCodexStartedFragment(t, &published[0], "sess-codex-3", "work-codex-json-3")
	assertCodexBoundedFragment(t, &published[1], ProgressFragmentKind, NormalizedEventTypeProgress, "response.progress", progressPayload, codexRetainedProgressBytes)
	assertCodexBoundedFragment(t, &published[2], ResponseFragmentKind, NormalizedEventTypeTextDelta, "response.output_text.delta", deltaPayload, codexRetainedTextBytes)
	assertCodexBoundedFragment(t, &published[3], ResponseFragmentKind, NormalizedEventTypeFinalText, "response.completed", finalPayload, codexRetainedTextBytes)
	assertCodexBoundedFragment(t, &published[4], ProgressFragmentKind, NormalizedEventTypeFailed, "response.failed", failurePayload, codexRetainedProgressBytes)
	assertCodexBoundedFragment(t, &published[5], ProgressFragmentKind, NormalizedEventTypeCanceled, "response.canceled", cancelPayload, codexRetainedProgressBytes)

	if published[5].ProviderSessionRef == nil || published[5].ProviderSessionRef.ID != "sess-codex-3" {
		t.Fatalf("cancel provider session = %#v, want session propagated", published[5].ProviderSessionRef)
	}
}

func fragmentByType(published []InferenceProgressFragment, fragmentType string) *InferenceProgressFragment {
	for i := range published {
		if published[i].Type == fragmentType {
			return &published[i]
		}
	}
	return nil
}

func assertCodexStartedFragment(t *testing.T, fragment *InferenceProgressFragment, sessionID string, workID string) {
	t.Helper()
	if fragment == nil || fragment.ExternalEventType != "session.created" {
		t.Fatalf("started fragment = %#v, want session.created", fragment)
	}
	if fragment.ProviderSessionRef == nil || fragment.ProviderSessionRef.ID != sessionID {
		t.Fatalf("start provider session = %#v, want %s", fragment.ProviderSessionRef, sessionID)
	}
	if got := fragment.Metadata[codexMetadataRunnerIDKey]; got != "codex" {
		t.Fatalf("start metadata runner_id = %q, want codex", got)
	}
	if got := fragment.Metadata[codexMetadataWorkstationKey]; got != "review" {
		t.Fatalf("start metadata workstation_name = %q, want review", got)
	}
	if got := fragment.Metadata[codexMetadataWorkIDKey]; got != workID {
		t.Fatalf("start metadata work_id = %q, want %q", got, workID)
	}
}

func assertCodexResponseFragment(t *testing.T, fragment *InferenceProgressFragment, fragmentType string, payload string) {
	t.Helper()
	if fragment == nil || fragment.Kind != ResponseFragmentKind || fragment.Type != fragmentType || fragment.Payload != payload {
		t.Fatalf("response fragment = %#v, want %s payload %q", fragment, fragmentType, payload)
	}
}

func assertCodexProgressFragment(t *testing.T, fragment *InferenceProgressFragment, payload string) {
	t.Helper()
	if fragment == nil || fragment.Kind != ProgressFragmentKind || fragment.Payload != payload {
		t.Fatalf("progress fragment = %#v, want progress payload %q", fragment, payload)
	}
}

func assertUnknownCodexDiagnostic(t *testing.T, fragment *InferenceProgressFragment, externalEventType string, diagnosticClass string) {
	t.Helper()
	if fragment == nil || fragment.Type != NormalizedEventTypeUnknown || fragment.Kind != ProgressFragmentKind {
		t.Fatalf("unknown fragment = %#v, want UNKNOWN progress fragment", fragment)
	}
	if fragment.ExternalEventType != externalEventType {
		t.Fatalf("unknown external event = %q, want %q", fragment.ExternalEventType, externalEventType)
	}
	if fragment.Payload != "codex event omitted" || strings.Contains(fragment.Payload, "secret-token-123") {
		t.Fatalf("unknown payload = %q, want bounded omitted diagnostic", fragment.Payload)
	}
	if got := fragment.Metadata[codexMetadataDiagnosticKey]; got != diagnosticClass {
		t.Fatalf("diagnostic_class = %q, want %q", got, diagnosticClass)
	}
	if diagnosticClass == codexDiagnosticUnknownEvent && (fragment.Metadata[codexMetadataRawSHA256Key] == "" || fragment.Metadata[codexMetadataRawBytesKey] == "") {
		t.Fatalf("unknown metadata = %#v, want raw digest metadata", fragment.Metadata)
	}
}

func assertCodexBoundedFragment(
	t *testing.T,
	fragment *InferenceProgressFragment,
	kind string,
	fragmentType string,
	externalEventType string,
	originalPayload string,
	retainedBytes int,
) {
	t.Helper()
	if fragment == nil {
		t.Fatalf("fragment = nil, want %s %s", kind, fragmentType)
	}
	if fragment.Kind != kind || fragment.Type != fragmentType {
		t.Fatalf("fragment kind/type = (%q, %q), want (%q, %q)", fragment.Kind, fragment.Type, kind, fragmentType)
	}
	if fragment.ExternalEventType != externalEventType {
		t.Fatalf("external event type = %q, want %q", fragment.ExternalEventType, externalEventType)
	}
	if len([]byte(fragment.Payload)) != retainedBytes {
		t.Fatalf("payload bytes = %d, want %d", len([]byte(fragment.Payload)), retainedBytes)
	}
	if fragment.Payload != originalPayload[:retainedBytes] {
		t.Fatalf("payload retained wrong prefix length: got %d bytes", len([]byte(fragment.Payload)))
	}
	if got := fragment.Metadata[codexMetadataTextBytesKey]; got != strconv.Itoa(len([]byte(originalPayload))) {
		t.Fatalf("text_bytes = %q, want %d", got, len([]byte(originalPayload)))
	}
	if got := fragment.Metadata[codexMetadataTruncatedKey]; got != "true" {
		t.Fatalf("payload_truncated = %q, want true", got)
	}
}
