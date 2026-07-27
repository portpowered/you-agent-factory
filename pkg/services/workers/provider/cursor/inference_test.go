package cursor

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

func TestParseInferenceResult_Success(t *testing.T) {
	stdout := []byte(`{
		"type": "result",
		"subtype": "success",
		"is_error": false,
		"duration_ms": 1234,
		"duration_api_ms": 1100,
		"result": "Done reviewing the repo.",
		"session_id": "c6b62c6f-7ead-4fd6-9922-e952131177ff",
		"request_id": "10e11780-df2f-45dc-a1ff-4540af32e9c0",
		"usage": {
			"inputTokens": 1200,
			"outputTokens": 340,
			"cacheReadTokens": 50,
			"cacheWriteTokens": 10
		}
	}`)

	parsed, err := ParseInferenceResult(string(modelprovider.ProviderCursor), stdout)
	if err != nil {
		t.Fatalf("ParseInferenceResult returned error: %v", err)
	}
	if parsed.Content != "Done reviewing the repo." {
		t.Fatalf("content = %q, want parsed result text", parsed.Content)
	}
	if parsed.ProviderSession == nil {
		t.Fatal("expected provider session metadata")
	}
	if parsed.ProviderSession.Provider != "cursor" {
		t.Fatalf("provider = %q, want cursor", parsed.ProviderSession.Provider)
	}
	if parsed.ProviderSession.Kind != ProviderSessionKindSessionID {
		t.Fatalf("kind = %q, want session_id", parsed.ProviderSession.Kind)
	}
	if parsed.ProviderSession.ID != "c6b62c6f-7ead-4fd6-9922-e952131177ff" {
		t.Fatalf("session id = %q", parsed.ProviderSession.ID)
	}

	assertResponseMetadata(t, parsed.ResponseMetadata, map[string]string{
		ResponseMetadataRequestID:        "10e11780-df2f-45dc-a1ff-4540af32e9c0",
		ResponseMetadataDurationMS:       "1234",
		ResponseMetadataDurationAPIMS:    "1100",
		ResponseMetadataInputTokens:      "1200",
		ResponseMetadataOutputTokens:     "340",
		ResponseMetadataCacheReadTokens:  "50",
		ResponseMetadataCacheWriteTokens: "10",
	})
}

func TestParseInferenceResult_StreamJSONSuccess(t *testing.T) {
	stdout := []byte(
		"{\"type\":\"system\",\"subtype\":\"init\",\"session_id\":\"cursor-stream-session\"}\n" +
			"{\"type\":\"assistant\",\"timestamp_ms\":1,\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"Plan \"}]},\"session_id\":\"cursor-stream-session\"}\n" +
			"{\"type\":\"result\",\"subtype\":\"success\",\"is_error\":false,\"duration_ms\":1234,\"duration_api_ms\":1100,\"result\":\"Plan done\",\"session_id\":\"cursor-stream-session\",\"request_id\":\"req-stream-123\",\"usage\":{\"inputTokens\":12,\"outputTokens\":34}}\n",
	)

	parsed, err := ParseInferenceResult(string(modelprovider.ProviderCursor), stdout)
	if err != nil {
		t.Fatalf("ParseInferenceResult returned error: %v", err)
	}
	if parsed.Content != "Plan done" {
		t.Fatalf("content = %q, want stream result text", parsed.Content)
	}
	if parsed.ProviderSession == nil || parsed.ProviderSession.Provider != "cursor" || parsed.ProviderSession.ID != "cursor-stream-session" {
		t.Fatalf("provider session = %#v, want canonical cursor stream session", parsed.ProviderSession)
	}
	assertResponseMetadata(t, parsed.ResponseMetadata, map[string]string{
		ResponseMetadataRequestID:     "req-stream-123",
		ResponseMetadataDurationMS:    "1234",
		ResponseMetadataDurationAPIMS: "1100",
		ResponseMetadataInputTokens:   "12",
		ResponseMetadataOutputTokens:  "34",
	})
}

func TestParseInferenceResult_StreamJSONIgnoresMalformedAndUnknownLinesBeforeResult(t *testing.T) {
	stdout := []byte(
		"{not json}\n" +
			"{\"type\":\"mystery\"}\n" +
			"{\"type\":\"result\",\"subtype\":\"success\",\"is_error\":false,\"result\":\"Plan done\",\"session_id\":\"cursor-stream-session\"}\n",
	)

	parsed, err := ParseInferenceResult(string(modelprovider.ProviderCursor), stdout)
	if err != nil {
		t.Fatalf("ParseInferenceResult returned error: %v", err)
	}
	if parsed.Content != "Plan done" {
		t.Fatalf("content = %q, want stream result text", parsed.Content)
	}
	if parsed.ProviderSession == nil || parsed.ProviderSession.ID != "cursor-stream-session" {
		t.Fatalf("provider session = %#v, want canonical cursor stream session", parsed.ProviderSession)
	}
}

func TestParseInferenceResult_StreamJSONUsesLastTerminalResult(t *testing.T) {
	stdout := []byte(
		"{\"type\":\"result\",\"subtype\":\"api_error\",\"is_error\":true,\"result\":\"old server failure\",\"session_id\":\"old-session\"}\n" +
			"{\"type\":\"assistant\",\"message\":{\"content\":[{\"type\":\"text\",\"text\":\"ignored\"}]}}\n" +
			"{\"type\":\"result\",\"subtype\":\"rate_limit_error\",\"is_error\":true,\"result\":\"Cursor capacity is busy\",\"session_id\":\"final-session\"}\n",
	)

	parsed, failure := ParseInferenceResult(string(modelprovider.ProviderCursor), stdout)
	if parsed != nil {
		t.Fatalf("parsed result = %#v, want no successful response", parsed)
	}
	if failure == nil || failure.Type != workerexecution.WorkFailureTypeThrottled || failure.Message != cursorThrottleFailureMessage {
		t.Fatalf("failure = %#v, want final throttling result", failure)
	}
	if failure.ProviderSession == nil || failure.ProviderSession.ID != "final-session" {
		t.Fatalf("provider session = %#v, want final-session", failure.ProviderSession)
	}
}

func TestParseInferenceResult_MissingSessionID(t *testing.T) {
	stdout := []byte(`{
		"type": "result",
		"subtype": "success",
		"is_error": false,
		"result": "done",
		"session_id": ""
	}`)

	_, err := ParseInferenceResult(string(modelprovider.ProviderCursor), stdout)
	if err == nil {
		t.Fatal("expected parse error for missing session_id")
	}
	if err.Type != workerexecution.WorkFailureTypeUnknown {
		t.Fatalf("error type = %q, want unknown", err.Type)
	}
}

func TestParseInferenceResult_InvalidSessionID(t *testing.T) {
	stdout := []byte(`{
		"type": "result",
		"subtype": "success",
		"is_error": false,
		"result": "done",
		"session_id": "../cursor-session"
	}`)

	_, err := ParseInferenceResult(string(modelprovider.ProviderCursor), stdout)
	if err == nil {
		t.Fatal("expected parse error for invalid session_id")
	}
	if err.Type != workerexecution.WorkFailureTypeUnknown {
		t.Fatalf("error type = %q, want unknown", err.Type)
	}
}

func TestParseInferenceResult_StreamJSONRejectsInvalidReplacement(t *testing.T) {
	stdout := []byte(
		"{\"type\":\"system\",\"subtype\":\"init\",\"session_id\":\"cursor-stream-session\"}\n" +
			"{\"type\":\"result\",\"subtype\":\"success\",\"is_error\":false,\"result\":\"Plan done\",\"session_id\":\"../cursor-stream-session\"}\n",
	)

	parsed, err := ParseInferenceResult(string(modelprovider.ProviderCursor), stdout)
	if err != nil {
		t.Fatalf("ParseInferenceResult() failure = %#v, want accepted initialization session", err)
	}
	if parsed.ProviderSession == nil || parsed.ProviderSession.ID != "cursor-stream-session" {
		t.Fatalf("provider session = %#v, want initialization session", parsed.ProviderSession)
	}
}

func TestParseInferenceResult_MalformedJSON(t *testing.T) {
	_, err := ParseInferenceResult(string(modelprovider.ProviderCursor), []byte(`{not json`))
	if err == nil {
		t.Fatal("expected parse error for malformed JSON")
	}
	if err.Type != workerexecution.WorkFailureTypeUnknown {
		t.Fatalf("error type = %q, want unknown", err.Type)
	}
}

func TestParseInferenceResult_UnexpectedType(t *testing.T) {
	stdout := []byte(`{"type":"assistant","subtype":"success","result":"hi","session_id":"sess-1"}`)

	_, err := ParseInferenceResult(string(modelprovider.ProviderCursor), stdout)
	if err == nil {
		t.Fatal("expected parse error for unexpected type")
	}
	if err.Type != workerexecution.WorkFailureTypeUnknown {
		t.Fatalf("error type = %q, want unknown", err.Type)
	}
}

func TestParseInferenceResult_ErrorSubtype(t *testing.T) {
	oversizedResult := strings.Repeat("x", FailureMessageLimit+20)
	stdout := []byte(`{
		"type": "result",
		"subtype": "error",
		"is_error": true,
		"result": "` + oversizedResult + `",
		"session_id": "sess-1"
	}`)

	_, err := ParseInferenceResult(string(modelprovider.ProviderCursor), stdout)
	if err == nil {
		t.Fatal("expected parse error for error subtype")
	}
	if err.Type != workerexecution.WorkFailureTypeUnknown {
		t.Fatalf("error type = %q, want unknown", err.Type)
	}
	if strings.Contains(err.Message, oversizedResult) {
		t.Fatalf("error message = %q, should not include full oversized result", err.Message)
	}
	if err.Message != cursorUnknownFailureMessage {
		t.Fatalf("error message = %q, want canonical unknown guidance", err.Message)
	}
}

func TestBoundedCommandOutputExcerpt_TruncatesWhenOverLimit(t *testing.T) {
	const limit = 8
	long := []byte("0123456789abcdef")
	got := BoundedCommandOutputExcerpt(long, limit)
	want := "01234567..."
	if got != want {
		t.Fatalf("excerpt = %q, want %q", got, want)
	}
}

func TestBoundedText_PreservesSpacingForPublishedAssistantText(t *testing.T) {
	got := boundedText(" hi", 2)
	if got != " h..." {
		t.Fatalf("boundedText() = %q, want preserved leading spacing with truncation", got)
	}
}

func TestWithCommandOutputExcerpts_AttachesBoundedStdoutAndStderr(t *testing.T) {
	stdout := []byte(strings.Repeat("a", CommandOutputExcerptLimit+10))
	stderr := []byte("rate limited\n")
	diagnostics := WithCommandOutputExcerpts(nil, stdout, stderr)
	if diagnostics == nil || diagnostics.Provider == nil {
		t.Fatal("expected provider diagnostics with excerpts")
	}
	if got := diagnostics.Provider.ResponseMetadata[ResponseMetadataStdoutExcerpt]; len(got) != CommandOutputExcerptLimit+3 {
		t.Fatalf("stdout excerpt len = %d, want %d with ellipsis", len(got), CommandOutputExcerptLimit+3)
	}
	if got := diagnostics.Provider.ResponseMetadata[ResponseMetadataStderrExcerpt]; got != "rate limited" {
		t.Fatalf("stderr excerpt = %q, want rate limited", got)
	}
}

func assertResponseMetadata(t *testing.T, metadata map[string]string, want map[string]string) {
	t.Helper()
	for key, wantValue := range want {
		if got := metadata[key]; got != wantValue {
			t.Fatalf("response metadata[%q] = %q, want %q", key, got, wantValue)
		}
	}
}

func TestSuccessStdoutJSON(t *testing.T) {
	encoded := SuccessStdoutJSON("hello", "sess-1")
	var payload resultPayload
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload.Result != "hello" || payload.SessionID != "sess-1" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestIntegrationPublishesCursorStreamInOrderAndClosesOnce(t *testing.T) {
	stdout := []byte(strings.Join([]string{
		`{"type":"system","subtype":"init","session_id":"cursor-integration"}`,
		`{"type":"assistant","timestamp_ms":1,"message":{"role":"assistant","content":[{"type":"text","text":"Plan " }]},"session_id":"cursor-integration"}`,
		`{"type":"tool_call","subtype":"started","call_id":"call-1","tool_call":{"readToolCall":{"args":{"path":"README.md"}}},"session_id":"cursor-integration"}`,
		`{"type":"tool_call","subtype":"completed","call_id":"call-1","tool_call":{"readToolCall":{"result":{"success":{"bytes":12}}}},"session_id":"cursor-integration"}`,
		`{"type":"result","subtype":"success","is_error":false,"result":"Plan done","session_id":"cursor-integration"}`,
	}, "\n"))
	runner := &cursorIntegrationRunner{
		chunks: [][]byte{stdout[:83], stdout[83:211], stdout[211:]},
		result: workerprocess.CommandResult{Stdout: stdout},
	}
	writer := &cursorIntegrationWriter{}
	integration := NewIntegration(IntegrationDependencies{
		CommandRunner: runner, OperatingSystem: "linux",
	})
	if integration.Identity() != "cursor" {
		t.Fatalf("Identity() = %q, want cursor", integration.Identity())
	}
	discovery, err := integration.Discover(context.Background())
	if err != nil || discovery.Readiness() != inference.ReadinessReady {
		t.Fatalf("Discover() = %#v, %v; want ready", discovery, err)
	}
	request := inference.NewInvocationRequest(inference.InvocationInput{
		InvocationID: "inv-cursor-stream",
		UserMessage:  "inspect the repository",
	})
	capabilities, err := integration.Capabilities(context.Background(), request)
	if err != nil {
		t.Fatalf("Capabilities() error = %v", err)
	}
	assertCursorIntegrationCapabilities(t, capabilities, integration.MaximumCapabilities())

	err = integration.Invoke(context.Background(), request, writer)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}

	assertCursorIntegrationEvents(t, writer)
	if writer.completion.Response() == nil || writer.completion.Response().Content() != "Plan done" {
		t.Fatalf("completion = %#v, want authoritative Plan done response", writer.completion)
	}
	if session := writer.completion.Response().ProviderSession(); session == nil || session.ID() != "cursor-integration" {
		t.Fatalf("provider session = %#v, want cursor-integration", session)
	}
}

func assertCursorIntegrationCapabilities(
	t *testing.T,
	capabilities inference.CapabilitySet,
	maximum inference.CapabilitySet,
) {
	t.Helper()
	for _, required := range []inference.Capability{
		inference.CapabilityPromptSubmission, inference.CapabilitySessionResume,
		inference.CapabilityNativeStreaming, inference.CapabilityMessageSnapshots,
		inference.CapabilityToolLifecycle, inference.CapabilityToolOutputDeltas,
		inference.CapabilityUsage, inference.CapabilityStableItemIDs,
	} {
		if !capabilities.Has(required) || !maximum.Has(required) {
			t.Fatalf("capabilities = %v maximum = %v, want %q", capabilities.Values(), maximum.Values(), required)
		}
	}
	if capabilities.Has(inference.CapabilityMessageDeltas) {
		t.Fatalf("capabilities = %v, must preserve manifest without message_deltas advertisement", capabilities.Values())
	}
}

func assertCursorIntegrationEvents(t *testing.T, writer *cursorIntegrationWriter) {
	t.Helper()
	if writer.closeCalls != 1 || writer.writeAfterClose != 0 {
		t.Fatalf("writer close calls = %d, writes after close = %d; want 1 and 0", writer.closeCalls, writer.writeAfterClose)
	}
	if len(writer.events) != 6 {
		t.Fatalf("events = %#v, want run boundaries around assistant, tool lifecycle, and terminal snapshot", writer.events)
	}
	wantKinds := []workerexecution.Kind{
		workerexecution.KindRun, workerexecution.KindMessage, workerexecution.KindTool,
		workerexecution.KindTool, workerexecution.KindMessage, workerexecution.KindRun,
	}
	for index, event := range writer.events {
		draft := event.Draft()
		if draft.Kind != wantKinds[index] || draft.RunID != "inv-cursor-stream" {
			t.Fatalf("event[%d] = %#v, want kind %q and correlated run", index, draft, wantKinds[index])
		}
	}
}

func TestIntegrationWriterBackpressureStopsPublicationWithoutClosing(t *testing.T) {
	stdout := []byte(
		`{"type":"assistant","timestamp_ms":1,"message":{"content":[{"type":"text","text":"private draft"}]},"session_id":"cursor-backpressure"}` + "\n" +
			`{"type":"result","subtype":"success","is_error":false,"result":"done","session_id":"cursor-backpressure"}`,
	)
	writerErr := errors.New("writer backpressure")
	writer := &cursorIntegrationWriter{writeErr: writerErr}

	err := NewIntegration(IntegrationDependencies{
		CommandRunner:   &cursorIntegrationRunner{chunks: [][]byte{stdout}, result: workerprocess.CommandResult{Stdout: stdout}},
		OperatingSystem: "linux",
	}).Invoke(context.Background(), inference.NewInvocationRequest(inference.InvocationInput{
		InvocationID: "inv-cursor-backpressure", UserMessage: "run",
	}), writer)

	if !errors.Is(err, writerErr) {
		t.Fatalf("Invoke() error = %v, want writer backpressure", err)
	}
	if len(writer.events) != 0 || writer.closeCalls != 0 || writer.writeCalls != 1 {
		t.Fatalf("writer = %#v, want one rejected write, no accepted events, and no close", writer)
	}
}

func TestIntegrationMalformedOrIncompleteStreamClosesWithSafeFailure(t *testing.T) {
	for _, stdout := range [][]byte{
		[]byte(`{"type":"assistant","timestamp_ms":1,"message":{"content":[{"type":"text","text":"private prompt /Users/alice/key"}]}}`),
		[]byte(`{"type":"result","subtype":"success","is_error":false,"result":"private prompt","session_id":`),
	} {
		writer := &cursorIntegrationWriter{}
		err := NewIntegration(IntegrationDependencies{
			CommandRunner: &cursorIntegrationRunner{
				chunks: [][]byte{stdout}, result: workerprocess.CommandResult{Stdout: stdout},
			},
			OperatingSystem: "linux",
		}).Invoke(context.Background(), inference.NewInvocationRequest(inference.InvocationInput{
			InvocationID: "inv-cursor-invalid-stream", UserMessage: "secret request",
		}), writer)
		if err != nil {
			t.Fatalf("Invoke() error = %v", err)
		}
		if writer.closeCalls != 1 || writer.completion.Failure() == nil {
			t.Fatalf("writer = %#v, want one failed close", writer)
		}
		message := writer.completion.Failure().Message()
		for _, unsafe := range []string{"private prompt", "/Users/alice", "secret request"} {
			if strings.Contains(message, unsafe) {
				t.Fatalf("failure message leaked %q: %q", unsafe, message)
			}
		}
	}
}

func TestIntegrationPreservesOrReplacesResumedProviderSession(t *testing.T) {
	requested := inference.NewProviderSession(
		"cursor", ProviderSessionKindSessionID, "cursor-requested", nil,
	)
	tests := []struct {
		name   string
		stdout string
		wantID string
	}{
		{
			name:   "preserves requested session without replacement",
			stdout: `{"type":"result","subtype":"success","is_error":false,"result":"continued","session_id":""}`,
			wantID: "cursor-requested",
		},
		{
			name: "accepted initialization replaces requested session",
			stdout: strings.Join([]string{
				`{"type":"system","subtype":"init","session_id":"cursor-replacement"}`,
				`{"type":"result","subtype":"success","is_error":false,"result":"continued","session_id":""}`,
			}, "\n"),
			wantID: "cursor-replacement",
		},
		{
			name: "invalid replacement preserves requested session",
			stdout: strings.Join([]string{
				`{"type":"system","subtype":"init","session_id":"../invalid"}`,
				`{"type":"result","subtype":"success","is_error":false,"result":"continued","session_id":""}`,
			}, "\n"),
			wantID: "cursor-requested",
		},
		{
			name: "unaccepted record cannot replace requested session",
			stdout: strings.Join([]string{
				`{"type":"assistant","session_id":"cursor-unaccepted"}`,
				`{"type":"result","subtype":"success","is_error":false,"result":"continued","session_id":""}`,
			}, "\n"),
			wantID: "cursor-requested",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stdout := []byte(test.stdout)
			writer := &cursorIntegrationWriter{}
			err := NewIntegration(IntegrationDependencies{
				CommandRunner: &cursorIntegrationRunner{
					chunks: [][]byte{stdout},
					result: workerprocess.CommandResult{Stdout: stdout},
				},
				OperatingSystem: "linux",
			}).Invoke(context.Background(), inference.NewInvocationRequest(inference.InvocationInput{
				InvocationID:    "inv-cursor-resume",
				UserMessage:     "continue",
				ProviderSession: &requested,
			}), writer)
			if err != nil {
				t.Fatalf("Invoke() error = %v", err)
			}
			response := writer.completion.Response()
			if response == nil || response.ProviderSession() == nil ||
				response.ProviderSession().ID() != test.wantID {
				t.Fatalf("completion response = %#v, want session %q", response, test.wantID)
			}
			if writer.closeCalls != 1 {
				t.Fatalf("close calls = %d, want 1", writer.closeCalls)
			}
		})
	}
}

func TestIntegrationPreservesExecutionResumeProviderSession(t *testing.T) {
	stdout := []byte(
		`{"type":"result","subtype":"success","is_error":false,"result":"continued","session_id":""}`,
	)
	writer := &cursorIntegrationWriter{}
	err := NewIntegration(IntegrationDependencies{
		CommandRunner: &cursorIntegrationRunner{
			chunks: [][]byte{stdout},
			result: workerprocess.CommandResult{Stdout: stdout},
		},
		OperatingSystem: "linux",
	}).Invoke(context.Background(), inference.NewInvocationRequest(inference.InvocationInput{
		InvocationID: "inv-cursor-product-resume",
		UserMessage:  "continue",
		Execution: workerexecution.ProviderInferenceRequest{
			SessionID: "cursor-product-session",
		},
	}), writer)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	response := writer.completion.Response()
	if response == nil || response.ProviderSession() == nil ||
		response.ProviderSession().ID() != "cursor-product-session" {
		t.Fatalf("completion response = %#v, want execution resume session", response)
	}
}

func TestIntegrationRetainsObservedProviderSessionOnRetryableFailure(t *testing.T) {
	requested := inference.NewProviderSession(
		"cursor", ProviderSessionKindSessionID, "cursor-requested", nil,
	)
	tests := []struct {
		name   string
		stdout string
		wantID string
	}{
		{
			name:   "requested session survives without replacement",
			stdout: `{"type":"result","subtype":"rate_limit_error","is_error":true,"result":"capacity unavailable","session_id":""}`,
			wantID: "cursor-requested",
		},
		{
			name: "observed session replaces requested session",
			stdout: strings.Join([]string{
				`{"type":"system","subtype":"init","session_id":"cursor-retry"}`,
				`{"type":"result","subtype":"rate_limit_error","is_error":true,"result":"capacity unavailable","session_id":""}`,
			}, "\n"),
			wantID: "cursor-retry",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stdout := []byte(test.stdout)
			writer := &cursorIntegrationWriter{}
			err := NewIntegration(IntegrationDependencies{
				CommandRunner: &cursorIntegrationRunner{
					chunks: [][]byte{stdout},
					result: workerprocess.CommandResult{Stdout: stdout, ExitCode: 1},
				},
				OperatingSystem: "linux",
			}).Invoke(context.Background(), inference.NewInvocationRequest(inference.InvocationInput{
				InvocationID:    "inv-cursor-retry",
				UserMessage:     "continue",
				ProviderSession: &requested,
			}), writer)
			if err != nil {
				t.Fatalf("Invoke() error = %v", err)
			}
			failure := writer.completion.Failure()
			if failure == nil || !failure.Retryable() || failure.ProviderSession() == nil ||
				failure.ProviderSession().ID() != test.wantID {
				t.Fatalf("completion failure = %#v, want retryable %s session", failure, test.wantID)
			}
			if writer.closeCalls != 1 {
				t.Fatalf("close calls = %d, want 1", writer.closeCalls)
			}
		})
	}
}

func TestIntegrationConcurrentInvocationsKeepPublicationAndClosureIsolated(t *testing.T) {
	const invocations = 16
	stdout := SuccessStdoutJSON("done", "cursor-concurrent")
	integration := NewIntegration(IntegrationDependencies{
		CommandRunner:   &cursorIntegrationRunner{chunks: [][]byte{stdout}, result: workerprocess.CommandResult{Stdout: stdout}},
		OperatingSystem: "linux",
	})

	var wait sync.WaitGroup
	errorsByInvocation := make(chan error, invocations)
	for index := 0; index < invocations; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			writer := &cursorIntegrationWriter{}
			err := integration.Invoke(context.Background(), inference.NewInvocationRequest(inference.InvocationInput{
				InvocationID: "inv-cursor-concurrent", UserMessage: "run",
			}), writer)
			if err == nil && writer.closeCalls != 1 {
				err = errors.New("writer was not closed exactly once")
			}
			errorsByInvocation <- err
		}()
	}
	wait.Wait()
	close(errorsByInvocation)
	for err := range errorsByInvocation {
		if err != nil {
			t.Fatal(err)
		}
	}
}

type cursorIntegrationFailureCase struct {
	name        string
	stdout      string
	stderr      string
	exitCode    int
	commandErr  error
	wantKind    inference.FailureKind
	wantMessage string
	wantRetry   bool
	rejected    []string
}

func TestIntegrationNormalizesCursorFailureCategoriesWithoutNativeDetail(t *testing.T) {
	t.Parallel()

	cases := []cursorIntegrationFailureCase{
		{
			name: "authentication", stdout: cursorFailureRecord(
				"authentication_error", "Authorization: Bearer customer-token", "cursor-auth",
			),
			exitCode: 1, wantKind: inference.FailureAuthentication,
			wantMessage: cursorAuthFailureMessage,
			rejected:    []string{"Authorization", "customer-token"},
		},
		{
			name: "invalid request", stdout: cursorFailureRecord(
				"invalid_request_error", "private prompt selected unsupported model", "cursor-invalid",
			),
			exitCode: 1, wantKind: inference.FailureInvalidRequest,
			wantMessage: cursorBadRequestFailureMessage,
			rejected:    []string{"private prompt", "unsupported model"},
		},
		{
			name: "throttled", stdout: cursorFailureRecord(
				"rate_limit_error", "capacity response included full transcript", "cursor-throttle",
			),
			stderr: "authentication failed", exitCode: 1,
			wantKind: inference.FailureThrottled, wantMessage: cursorThrottleFailureMessage,
			wantRetry: true, rejected: []string{"full transcript", "authentication failed"},
		},
		{
			name: "temporary service", stdout: cursorFailureRecord(
				"api_error", "provider unavailable at /Users/alice/private/project", "cursor-service",
			),
			exitCode: 1, wantKind: inference.FailureUnknown,
			wantMessage: cursorServerFailureMessage, wantRetry: true,
			rejected: []string{"/Users/alice", "private"},
		},
		{
			name: "structured timeout", stdout: cursorFailureRecord(
				"error", "deadline exceeded while sending user prompt", "cursor-timeout",
			),
			exitCode: 1, wantKind: inference.FailureTimeout,
			wantMessage: cursorTimeoutFailureMessage, wantRetry: true,
			rejected: []string{"user prompt", "deadline exceeded"},
		},
	}

	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			assertCursorIntegrationFailure(t, testCase)
		})
	}
}

func TestIntegrationNormalizesMalformedUnknownAndCommandDeadlineFailures(t *testing.T) {
	t.Parallel()

	cases := []cursorIntegrationFailureCase{
		{
			name:     "malformed output",
			stdout:   `{"type":"assistant","message":{"content":[{"type":"text","text":"private transcript /Users/alice/key"}]}`,
			wantKind: inference.FailureMalformedOutput, wantMessage: cursorUnknownFailureMessage,
			rejected: []string{"private transcript", "/Users/alice"},
		},
		{
			name:     "unknown nonzero exit",
			stderr:   "tool payload: password=hunter2 at /home/alice/private",
			exitCode: 17, wantKind: inference.FailureUnknown,
			wantMessage: cursorUnknownFailureMessage,
			rejected:    []string{"hunter2", "/home/alice", "tool payload"},
		},
		{
			name:       "command deadline",
			commandErr: context.DeadlineExceeded,
			wantKind:   inference.FailureTimeout, wantMessage: cursorTimeoutFailureMessage,
			wantRetry: true,
		},
		{
			name: "timeout exit code", exitCode: 124,
			wantKind: inference.FailureTimeout, wantMessage: cursorTimeoutFailureMessage,
			wantRetry: true,
		},
	}

	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			assertCursorIntegrationFailure(t, testCase)
		})
	}
}

func TestIntegrationPropagatesCancellationForProtocolNormalization(t *testing.T) {
	t.Parallel()

	writer := &cursorIntegrationWriter{}
	err := inference.ExecuteInvocation(
		context.Background(),
		NewIntegration(IntegrationDependencies{
			CommandRunner:   &cursorIntegrationRunner{err: context.Canceled},
			OperatingSystem: "linux",
		}),
		cursorFailureInvocation("inv-cursor-canceled"),
		writer,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ExecuteInvocation() error = %v, want context.Canceled", err)
	}
	if writer.closeCalls != 1 || writer.writeAfterClose != 0 {
		t.Fatalf("writer closes=%d writes-after-close=%d, want 1 and 0", writer.closeCalls, writer.writeAfterClose)
	}
	failure := writer.completion.Failure()
	if failure == nil || failure.Kind() != inference.FailureCanceled ||
		failure.Message() != "provider invocation was canceled" || failure.Retryable() {
		t.Fatalf("failure = %#v, want canonical non-retryable cancellation", failure)
	}
}

func TestIntegrationContextDeadlineWinsOverCommandCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	writer := &cursorIntegrationWriter{}
	err := inference.ExecuteInvocation(
		ctx,
		NewIntegration(IntegrationDependencies{
			CommandRunner:   &cursorIntegrationRunner{err: context.Canceled},
			OperatingSystem: "linux",
		}),
		cursorFailureInvocation("inv-cursor-deadline"),
		writer,
	)
	if err != nil {
		t.Fatalf("ExecuteInvocation() error = %v", err)
	}
	failure := writer.completion.Failure()
	if writer.closeCalls != 1 || failure == nil ||
		failure.Kind() != inference.FailureTimeout ||
		failure.Message() != cursorTimeoutFailureMessage ||
		!failure.Retryable() {
		t.Fatalf(
			"completion=%#v closes=%d, want one canonical retryable timeout",
			writer.completion,
			writer.closeCalls,
		)
	}
}

func assertCursorIntegrationFailure(t *testing.T, want cursorIntegrationFailureCase) {
	t.Helper()
	stdout := []byte(want.stdout)
	writer := &cursorIntegrationWriter{}
	err := inference.ExecuteInvocation(
		context.Background(),
		NewIntegration(IntegrationDependencies{
			CommandRunner: &cursorIntegrationRunner{
				chunks: [][]byte{stdout},
				result: workerprocess.CommandResult{
					Stdout: stdout, Stderr: []byte(want.stderr), ExitCode: want.exitCode,
				},
				err: want.commandErr,
			},
			OperatingSystem: "linux",
		}),
		cursorFailureInvocation("inv-cursor-"+strings.ReplaceAll(want.name, " ", "-")),
		writer,
	)
	if err != nil {
		t.Fatalf("ExecuteInvocation() error = %v", err)
	}
	if writer.closeCalls != 1 || writer.writeAfterClose != 0 ||
		writer.completion.Response() != nil {
		t.Fatalf(
			"completion=%#v closes=%d writes-after-close=%d, want one failed close",
			writer.completion, writer.closeCalls, writer.writeAfterClose,
		)
	}
	failure := writer.completion.Failure()
	if failure == nil || failure.Kind() != want.wantKind ||
		failure.Message() != want.wantMessage || failure.Retryable() != want.wantRetry {
		t.Fatalf(
			"failure=%#v, want kind=%q message=%q retryable=%v",
			failure, want.wantKind, want.wantMessage, want.wantRetry,
		)
	}
	visible := failure.Message()
	for _, event := range writer.events {
		visible += string(event.Draft().Payload)
	}
	for _, rejected := range want.rejected {
		if strings.Contains(visible, rejected) {
			t.Fatalf("customer-visible output leaked %q: %q", rejected, visible)
		}
	}
}

func cursorFailureRecord(subtype, result, sessionID string) string {
	return `{"type":"result","subtype":"` + subtype +
		`","is_error":true,"result":"` + result +
		`","session_id":"` + sessionID + `"}`
}

func cursorFailureInvocation(id string) inference.InvocationRequest {
	return inference.NewInvocationRequest(inference.InvocationInput{
		InvocationID: id,
		UserMessage:  "private customer prompt",
	})
}

type cursorIntegrationRunner struct {
	chunks [][]byte
	result workerprocess.CommandResult
	err    error
}

func (r *cursorIntegrationRunner) Run(context.Context, workerprocess.CommandRequest) (workerprocess.CommandResult, error) {
	return r.result, r.err
}

func (r *cursorIntegrationRunner) RunStreaming(
	_ context.Context,
	_ workerprocess.CommandRequest,
	observe workerprocess.OutputChunkObserver,
) (workerprocess.CommandResult, error) {
	for _, chunk := range r.chunks {
		observe(workerprocess.OutputStreamStdout, append([]byte(nil), chunk...))
	}
	return r.result, r.err
}

type cursorIntegrationWriter struct {
	mu              sync.Mutex
	events          []inference.EventDraft
	completion      inference.Completion
	writeErr        error
	writeCalls      int
	closeCalls      int
	writeAfterClose int
}

func (w *cursorIntegrationWriter) WriteEvent(_ context.Context, event inference.EventDraft) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.writeCalls++
	if w.closeCalls > 0 {
		w.writeAfterClose++
	}
	if w.writeErr != nil {
		return w.writeErr
	}
	w.events = append(w.events, event)
	return nil
}

func (w *cursorIntegrationWriter) Close(_ context.Context, completion inference.Completion) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closeCalls++
	w.completion = completion
	return nil
}
