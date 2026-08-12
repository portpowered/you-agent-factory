package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestInvokeLocalAsyncReturnsAfterAdmissionWithoutOpeningObservationStream(t *testing.T) {
	boundary := &invokeLocalFake{
		startResult: workersessions.StartResult{Session: workersessions.Session{
			ID: "local-session", State: workersessions.StateRunning,
		}},
	}
	var output bytes.Buffer
	err := NewInvoke(nil, boundary)(InvokeConfig{
		Context: context.Background(), Output: &output, OutputFormat: "json", Async: true,
		RequestID: "local-request", WorkerSessionID: "local-session", DispatchID: "local-dispatch",
		WorkstationName: "coding", UserMessage: "inspect the repository",
	})
	if err != nil {
		t.Fatalf("local async invoke error = %v", err)
	}
	if boundary.streamCalls != 0 {
		t.Fatalf("async observation stream calls = %d, want 0", boundary.streamCalls)
	}
	if len(boundary.startRequests) != 1 {
		t.Fatalf("start request count = %d, want 1", len(boundary.startRequests))
	}
	request := boundary.startRequests[0]
	if request.RequestID != "local-request" || request.ID != "local-session" {
		t.Fatalf("start identities = (%q, %q), want stable request/session IDs", request.RequestID, request.ID)
	}
	if request.Execution.Execution.Dispatch.DispatchID != "local-dispatch" {
		t.Fatalf("dispatch ID = %q, want local-dispatch", request.Execution.Execution.Dispatch.DispatchID)
	}
	var result invokeResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode async result: %v; output=%q", err, output.String())
	}
	if result.RequestID != "local-request" || result.WorkerSessionID != "local-session" ||
		result.State != "RUNNING" || !result.Accepted || result.Observation == "" {
		t.Fatalf("async result = %#v, want admitted identity/state/guidance", result)
	}
}

func TestInvokeNormalizesDocumentWithExplicitAndPositionalPrecedence(t *testing.T) {
	document := `{"requestId":"document-request","workerSessionId":"document-session","execution":{"workstationName":"document-workstation","dispatch":{"dispatchId":"document-dispatch","workstationName":"document-workstation"},"userMessage":"document message"}}`
	boundary := &invokeLocalFake{startResult: workersessions.StartResult{Session: workersessions.Session{
		ID: "document-session", State: workersessions.StateRunning,
	}}}
	var output bytes.Buffer
	err := NewInvoke(nil, boundary)(InvokeConfig{
		Context: context.Background(), Output: &output, OutputFormat: "json", Async: true,
		ExecutionJSON: document, RequestID: "flag-request", WorkstationName: "flag-workstation",
		Prompt: []string{"follow", "up"},
	})
	if err != nil {
		t.Fatalf("invoke with execution document error = %v", err)
	}
	request := boundary.startRequests[0]
	if request.RequestID != "flag-request" || request.ID != "document-session" {
		t.Fatalf("normalized identities = (%q, %q), want flag request and document session", request.RequestID, request.ID)
	}
	if request.Execution.WorkstationName != "flag-workstation" || request.Execution.Execution.Dispatch.DispatchID != "document-dispatch" {
		t.Fatalf("normalized execution = %#v, want explicit workstation and document dispatch", request.Execution)
	}
	if got := request.Execution.Execution.UserMessage; got != "follow up" {
		t.Fatalf("normalized user message = %q, want positional prompt over document message", got)
	}
}

func TestInvokeUsesNonTTYStdinForMissingUserMessage(t *testing.T) {
	boundary := &invokeLocalFake{startResult: workersessions.StartResult{Session: workersessions.Session{
		ID: "stdin-session", State: workersessions.StateRunning,
	}}}
	var output bytes.Buffer
	err := NewInvoke(nil, boundary)(InvokeConfig{
		Context: context.Background(), Output: &output, OutputFormat: "json", Async: true,
		RequestID: "stdin-request", WorkerSessionID: "stdin-session", DispatchID: "stdin-dispatch",
		WorkstationName: "coding", Stdin: strings.NewReader("piped message\n"),
	})
	if err != nil {
		t.Fatalf("invoke with stdin error = %v", err)
	}
	if got := boundary.startRequests[0].Execution.Execution.UserMessage; got != "piped message" {
		t.Fatalf("stdin user message = %q, want trimmed piped message", got)
	}
}

func TestInvokeLocalWaitsOnAuthoritativeObservationAndRendersOnlyProviderOutput(t *testing.T) {
	boundary := &invokeLocalFake{
		startResult: workersessions.StartResult{Session: workersessions.Session{
			ID: "local-session", State: workersessions.StateRunning,
		}},
		deliveries: []workersessions.ObservationDelivery{
			invokeObservationDelivery(workersessions.ObservationDeliveryRecord, "worker.session.running", `{"state":"RUNNING"}`),
			invokeObservationDelivery(workersessions.ObservationDeliveryTerminal, "worker.session.completed", `{"state":"COMPLETED","output":"provider output"}`),
		},
	}
	var output bytes.Buffer
	err := NewInvoke(nil, boundary)(InvokeConfig{
		Context: context.Background(), Output: &output,
		RequestID: "local-request", WorkerSessionID: "local-session", DispatchID: "local-dispatch",
		WorkstationName: "coding", UserMessage: "finish the change",
	})
	if err != nil {
		t.Fatalf("local synchronous invoke error = %v", err)
	}
	if got := output.String(); got != "provider output\n" {
		t.Fatalf("synchronous output = %q, want provider output without envelope", got)
	}
	if len(boundary.startRequests) != 1 || boundary.streamCalls != 1 {
		t.Fatalf("local calls = start:%d stream:%d, want one each", len(boundary.startRequests), boundary.streamCalls)
	}
	if !boundary.closed {
		t.Fatal("synchronous invoke did not close observation subscription")
	}
}

func TestInvokeRemoteAsyncPostsExactlySelectedServerAndDoesNotStream(t *testing.T) {
	var postCount, getCount int
	var received factoryapi.WorkerSessionStartRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/worker-sessions":
			postCount++
			if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
				t.Fatalf("decode start request: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(factoryapi.WorkerSessionStartResponse{
				RequestId: "remote-request", WorkerSessionId: "remote-session", Accepted: true,
				State: factoryapi.WorkerSessionStartResponseState("RUNNING"), EventTopic: "worker-session.remote-session",
			})
		case r.Method == http.MethodGet:
			getCount++
			t.Errorf("async invoke opened an observation stream at %s", r.URL.Path)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	var output bytes.Buffer
	err := NewInvoke(testHTTPProtocol(t), nil)(InvokeConfig{
		Context: context.Background(), Server: server.URL, Remote: true, Output: &output,
		OutputFormat: "json", Async: true, RequestID: "remote-request", WorkerSessionID: "remote-session",
		DispatchID: "remote-dispatch", WorkstationName: "coding", UserMessage: "remote work",
	})
	if err != nil {
		t.Fatalf("remote async invoke error = %v", err)
	}
	if postCount != 1 || getCount != 0 {
		t.Fatalf("remote request counts = POST:%d GET:%d, want POST:1 GET:0", postCount, getCount)
	}
	if received.RequestId != "remote-request" || received.WorkerSessionId != "remote-session" {
		t.Fatalf("remote request identities = (%q, %q), want supplied identities", received.RequestId, received.WorkerSessionId)
	}
	var result invokeResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode remote async result: %v; output=%q", err, output.String())
	}
	if result.WorkerSessionID != "remote-session" || result.State != "RUNNING" || !result.Accepted {
		t.Fatalf("remote async result = %#v, want admitted response", result)
	}
}

func TestInvokeRemoteWaitConsumesOneTerminalSSEFrame(t *testing.T) {
	var getCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(factoryapi.WorkerSessionStartResponse{
				RequestId: "remote-request", WorkerSessionId: "remote-session", Accepted: true,
				State: factoryapi.WorkerSessionStartResponseState("RUNNING"),
			})
			return
		}
		getCount++
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"delivery\":\"RECORD\",\"event\":{\"schemaId\":\"worker.session.running\",\"payload\":{\"state\":\"RUNNING\"}}}\n\n")
		_, _ = io.WriteString(w, "data: {\"delivery\":\"TERMINAL\",\"event\":{\"schemaId\":\"worker.session.completed\",\"payload\":{\"state\":\"COMPLETED\",\"output\":\"remote output\"}}}\n\n")
	}))
	defer server.Close()

	var output bytes.Buffer
	err := NewInvoke(testHTTPProtocol(t), nil)(InvokeConfig{
		Context: context.Background(), Server: server.URL, Remote: true, Output: &output,
		RequestID: "remote-request", WorkerSessionID: "remote-session", DispatchID: "remote-dispatch",
		WorkstationName: "coding", UserMessage: "remote wait",
	})
	if err != nil {
		t.Fatalf("remote synchronous invoke error = %v", err)
	}
	if getCount != 1 || output.String() != "remote output\n" {
		t.Fatalf("remote stream count/output = (%d, %q), want one terminal stream and raw output", getCount, output.String())
	}
}

func TestInvokeRemoteConnectionFailureRedactsSelectedEndpoint(t *testing.T) {
	protocol := &invokeProtocolStub{err: errors.New("dial failed")}
	var output bytes.Buffer
	err := NewInvoke(protocol, nil)(InvokeConfig{
		Context: context.Background(), Server: "https://example.test?token=secret", Remote: true,
		Output: &output, RequestID: "request", WorkerSessionID: "session", DispatchID: "dispatch",
		WorkstationName: "coding", UserMessage: "remote failure",
	})
	if err == nil {
		t.Fatal("remote connection failure error = nil, want stable failure")
	}
	var typed *CLIError
	if !errors.As(err, &typed) || typed.Code != "FACTORY_UNREACHABLE" {
		t.Fatalf("error = %v, want FACTORY_UNREACHABLE", err)
	}
	if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "/private") || strings.Contains(err.Error(), "token") {
		t.Fatalf("remote failure leaked endpoint details: %v", err)
	}
	if protocol.requestURL == "" || !strings.HasSuffix(protocol.requestURL, "/worker-sessions") {
		t.Fatalf("request URL = %q, want selected server top-level endpoint", protocol.requestURL)
	}
}

func TestInvokeConfigValidationCoversRequiredInputs(t *testing.T) {
	for _, test := range []struct {
		name   string
		config InvokeConfig
		want   string
	}{
		{name: "missing context", config: InvokeConfig{Output: &bytes.Buffer{}}, want: "context is required"},
		{name: "missing output", config: InvokeConfig{Context: context.Background()}, want: "output writer is required"},
		{name: "negative retry", config: InvokeConfig{Context: context.Background(), Output: &bytes.Buffer{}, RetryMaxAttempts: -1}, want: "WORKER_SESSION_RETRY_INVALID"},
		{name: "remote protocol missing", config: InvokeConfig{Context: context.Background(), Output: &bytes.Buffer{}, Remote: true}, want: "WORKER_SESSION_HTTP_UNAVAILABLE"},
		{name: "local boundary missing", config: InvokeConfig{Context: context.Background(), Output: &bytes.Buffer{}}, want: "WORKER_SESSION_LOCAL_UNAVAILABLE"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateInvokeConfig(test.config)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateInvokeConfig() = %v, want %q", err, test.want)
			}
		})
	}
	valid := InvokeConfig{Context: context.Background(), Output: &bytes.Buffer{}, Local: &invokeLocalFake{}}
	if err := validateInvokeConfig(valid); err != nil {
		t.Fatalf("validateInvokeConfig(valid) = %v, want nil", err)
	}
}

func TestReadInvokeRequestCoversInputFailures(t *testing.T) {
	for _, test := range []struct {
		name   string
		config InvokeConfig
		want   string
	}{
		{name: "stdin document missing", config: InvokeConfig{ExecutionJSON: "-"}, want: "WORKER_SESSION_INPUT_MISSING"},
		{name: "file reader missing", config: InvokeConfig{ExecutionJSON: "request.json"}, want: "WORKER_SESSION_INPUT_FAILED"},
		{name: "file read failure", config: InvokeConfig{ExecutionJSON: "request.json", ReadFile: func(string) ([]byte, error) { return nil, errors.New("read failed") }}, want: "WORKER_SESSION_INPUT_FAILED"},
		{name: "malformed JSON", config: InvokeConfig{ExecutionJSON: "{"}, want: "WORKER_SESSION_INPUT_INVALID"},
		{name: "trailing JSON", config: InvokeConfig{ExecutionJSON: "{} {}"}, want: "WORKER_SESSION_INPUT_INVALID"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := readInvokeRequest(test.config)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("readInvokeRequest() = %v, want %q", err, test.want)
			}
		})
	}
	fromFile, err := readInvokeRequest(InvokeConfig{
		ExecutionJSON: "request.json",
		ReadFile: func(string) ([]byte, error) {
			return []byte(`{"requestId":"request","workerSessionId":"session","execution":{"workstationName":"review","dispatch":{"dispatchId":"dispatch","workstationName":"review"},"userMessage":"message"}}`), nil
		},
	})
	if err != nil {
		t.Fatalf("readInvokeRequest(file) = %v, want nil", err)
	}
	if fromFile.RequestId != "request" {
		t.Fatalf("readInvokeRequest(file) request ID = %q, want request", fromFile.RequestId)
	}
}

func TestApplyInvokeOverridesPreservesExplicitFields(t *testing.T) {
	request := factoryapi.WorkerSessionStartRequest{}
	if err := applyInvokeOverrides(&request, InvokeConfig{
		RequestID: " request ", WorkerSessionID: " session ", WorkstationName: " review ", DispatchID: " dispatch ",
		WorkerType: " type ", RunnerID: " runner ", Provider: " provider ", Model: " model ", ReasoningEffort: " high ",
		SystemPrompt: "system", UserMessage: "user", RetryMaxAttempts: 3,
	}); err != nil {
		t.Fatalf("applyInvokeOverrides() = %v, want nil", err)
	}
	assertInvokeOverrides(t, request)
}

func assertInvokeOverrides(t *testing.T, request factoryapi.WorkerSessionStartRequest) {
	t.Helper()
	assertInvokeIdentityAndRouting(t, request)
	assertInvokeMessages(t, request)
	assertInvokeRunnerFields(t, request)
	assertInvokeModelFields(t, request)
	assertInvokeRetry(t, request)
}

func assertInvokeIdentityAndRouting(t *testing.T, request factoryapi.WorkerSessionStartRequest) {
	t.Helper()
	if request.RequestId != "request" || request.WorkerSessionId != "session" {
		t.Fatalf("request identity = %q/%q, want request/session", request.RequestId, request.WorkerSessionId)
	}
	if request.Execution.WorkstationName != "review" || request.Execution.Dispatch.DispatchId != "dispatch" || request.Execution.Dispatch.WorkstationName != "review" {
		t.Fatalf("dispatch/workstation = %#v/%q, want dispatch/review", request.Execution.Dispatch, request.Execution.WorkstationName)
	}
}

func assertInvokeMessages(t *testing.T, request factoryapi.WorkerSessionStartRequest) {
	t.Helper()
	if request.Execution.UserMessage == nil || *request.Execution.UserMessage != "user" || request.Execution.SystemPrompt == nil || *request.Execution.SystemPrompt != "system" {
		t.Fatalf("messages = %#v/%#v, want system/user", request.Execution.SystemPrompt, request.Execution.UserMessage)
	}
}

func assertInvokeRunnerFields(t *testing.T, request factoryapi.WorkerSessionStartRequest) {
	t.Helper()
	if request.Execution.WorkerType == nil || *request.Execution.WorkerType != "type" || request.Execution.RunnerId == nil || *request.Execution.RunnerId != "runner" {
		t.Fatalf("runner fields = %#v/%#v, want type/runner", request.Execution.WorkerType, request.Execution.RunnerId)
	}
}

func assertInvokeModelFields(t *testing.T, request factoryapi.WorkerSessionStartRequest) {
	t.Helper()
	if request.Execution.ModelProvider == nil || *request.Execution.ModelProvider != "provider" || request.Execution.Model == nil || *request.Execution.Model != "model" {
		t.Fatalf("model fields = %#v/%#v, want provider/model", request.Execution.ModelProvider, request.Execution.Model)
	}
}

func assertInvokeRetry(t *testing.T, request factoryapi.WorkerSessionStartRequest) {
	t.Helper()
	if request.Execution.ReasoningEffort == nil || *request.Execution.ReasoningEffort != "high" {
		t.Fatalf("reasoning effort = %#v, want high", request.Execution.ReasoningEffort)
	}
	if request.Retry == nil || request.Retry.MaxAttempts == nil || *request.Retry.MaxAttempts != 3 {
		t.Fatalf("retry = %#v, want max attempts 3", request.Retry)
	}
}

func TestEnsureInvokeIdentitiesUsesInjectedGenerator(t *testing.T) {
	request := factoryapi.WorkerSessionStartRequest{RequestId: "request", WorkerSessionId: "session"}
	request.Execution.Dispatch.DispatchId = "dispatch"
	if err := ensureInvokeIdentities(&request, nil); err != nil {
		t.Fatalf("ensureInvokeIdentities(populated) = %v, want nil", err)
	}
	missing := factoryapi.WorkerSessionStartRequest{}
	if err := ensureInvokeIdentities(&missing, nil); err == nil || !strings.Contains(err.Error(), "WORKER_SESSION_IDENTITY_UNAVAILABLE") {
		t.Fatalf("ensureInvokeIdentities(missing, nil) = %v, want identity error", err)
	}
	ids := []string{"generated-request", "generated-session", "generated-dispatch"}
	index := 0
	if err := ensureInvokeIdentities(&missing, func() string { value := ids[index]; index++; return value }); err != nil {
		t.Fatalf("ensureInvokeIdentities(generated) = %v, want nil", err)
	}
	assertGeneratedInvokeIdentities(t, missing, ids)
}

func assertGeneratedInvokeIdentities(t *testing.T, request factoryapi.WorkerSessionStartRequest, ids []string) {
	t.Helper()
	if request.RequestId != ids[0] || request.WorkerSessionId != ids[1] {
		t.Fatalf("generated request/session IDs = %q/%q, want %q/%q", request.RequestId, request.WorkerSessionId, ids[0], ids[1])
	}
	if request.Execution.Dispatch.DispatchId != ids[2] {
		t.Fatalf("generated dispatch ID = %q, want %q", request.Execution.Dispatch.DispatchId, ids[2])
	}
	if request.Execution.Dispatch.Execution == nil || request.Execution.Dispatch.Execution.RequestId == nil {
		t.Fatalf("generated dispatch execution = %#v, want request metadata", request.Execution.Dispatch.Execution)
	}
	if *request.Execution.Dispatch.Execution.RequestId != ids[0] {
		t.Fatalf("generated dispatch request ID = %q, want %q", *request.Execution.Dispatch.Execution.RequestId, ids[0])
	}
}

func TestInvokeCaptureEventProjectsOutputAndError(t *testing.T) {
	capture := invokeCapture{}
	captureEvent(&capture, "worker.session.running", json.RawMessage(`{"state":"RUNNING","kind":"MESSAGE","phase":"COMPLETED","payload":{"contentBlocks":[{"kind":"TEXT","text":"provider output"},{"kind":"JSON","structuredOutput":{"answer":42}}]}}`))
	if capture.State != "RUNNING" || capture.Output != "provider output" || !capture.HasOutput || !capture.HasStructured {
		t.Fatalf("captureEvent() = %#v, want state/output/structured result", capture)
	}
	captureEvent(&capture, "worker.session.failed", json.RawMessage(`{"error":"safe failure"}`))
	if capture.Error != "safe failure" {
		t.Fatalf("captureEvent(error) = %#v, want safe failure", capture)
	}
	errorCapture := invokeCapture{}
	captureEvent(&errorCapture, "worker.output", json.RawMessage(`{"kind":"ERROR","phase":"COMPLETED","payload":{"message":"draft failure"}}`))
	if errorCapture.Error != "draft failure" {
		t.Fatalf("captureEvent(ERROR draft) = %#v, want draft failure", errorCapture)
	}
}

func TestInferInvokeStateCoversKnownSchemas(t *testing.T) {
	for _, test := range []struct {
		schema string
		want   string
	}{
		{schema: "worker.canceled", want: "CANCELED"}, {schema: "worker.terminated", want: "TERMINATED"},
		{schema: "worker.failed", want: "FAILED"}, {schema: "worker.completed", want: "COMPLETED"}, {schema: "worker.running", want: ""},
	} {
		if got := inferInvokeState(test.schema); got != test.want {
			t.Errorf("inferInvokeState(%q) = %q, want %q", test.schema, got, test.want)
		}
	}
}

func TestInvokeObservationPayloadHelpersRejectInvalidValues(t *testing.T) {
	if _, ok := eventPayloadObject(nil); ok {
		t.Fatal("eventPayloadObject(nil) = ok, want false")
	}
	if _, ok := eventPayloadObject(json.RawMessage("null")); ok {
		t.Fatal("eventPayloadObject(null) = ok, want false")
	}
	if _, ok := eventPayloadObject(json.RawMessage("not-json")); ok {
		t.Fatal("eventPayloadObject(malformed) = ok, want false")
	}
	if derefString(nil) != "" {
		t.Fatal("derefString(nil) was non-empty")
	}
}

func TestInvokeTerminalErrorClassifiesStates(t *testing.T) {
	for _, state := range []string{"COMPLETED", "FAILED", "CANCELED", "TERMINATED", "", "UNKNOWN"} {
		err := terminalInvokeError(invokeCapture{State: state})
		if state == "COMPLETED" && err != nil {
			t.Fatalf("terminalInvokeError(COMPLETED) = %v, want nil", err)
		}
		if state != "COMPLETED" && err == nil {
			t.Fatalf("terminalInvokeError(%q) = nil, want error", state)
		}
	}
}

func TestInvokeObservationFramesClassifyTerminalAndFailure(t *testing.T) {
	for _, test := range []struct {
		delivery string
		payload  string
		wantDone bool
		wantErr  bool
	}{
		{delivery: "RECORD", payload: `{"delivery":"RECORD"}`},
		{delivery: "TERMINAL", payload: `{"delivery":"TERMINAL","event":{"schemaId":"worker.session.completed","payload":{"state":"COMPLETED"}}}`, wantDone: true},
		{delivery: "SOURCE_FAILURE", payload: `{"delivery":"SOURCE_FAILURE","errorCode":"STREAM_GAP","errorMessage":"gap"}`, wantErr: true},
		{delivery: "SOURCE_FAILURE", payload: `{"delivery":"SOURCE_FAILURE"}`, wantErr: true},
		{delivery: "REPLAY_SUMMARY", payload: `{"delivery":"REPLAY_SUMMARY"}`},
	} {
		t.Run(test.delivery, func(t *testing.T) {
			got := invokeCapture{}
			done, err := applyRemoteObservationFrame(&got, []byte(test.payload))
			if done != test.wantDone || (err != nil) != test.wantErr {
				t.Fatalf("applyRemoteObservationFrame() = done:%t err:%v, want done:%t err:%t", done, err, test.wantDone, test.wantErr)
			}
		})
	}
	if _, err := applyRemoteObservationFrame(&invokeCapture{}, []byte("not-json")); err == nil {
		t.Fatal("applyRemoteObservationFrame(malformed) = nil, want typed error")
	}
}

func TestInvokeOutputAndRemoteErrorHelpersCoverStableMappings(t *testing.T) {
	for _, test := range []struct {
		name string
		fn   func() error
	}{
		{name: "remote canceled", fn: func() error {
			return remoteInvokeTransportError(InvokeConfig{Context: context.Background()}, context.Canceled)
		}},
		{name: "remote stream canceled", fn: func() error {
			return remoteInvokeStreamTransportError(InvokeConfig{Context: context.Background()}, context.Canceled)
		}},
		{name: "remote stream read canceled", fn: func() error {
			return remoteStreamReadError(InvokeConfig{Context: context.Background()}, context.Canceled)
		}},
		{name: "remote stream closed", fn: func() error { return remoteStreamClosedError() }},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.fn(); err == nil {
				t.Fatal("helper returned nil, want stable CLI error")
			}
		})
	}
	for _, code := range []string{"BAD_REQUEST", "CONFLICT", "INTERNAL_ERROR", "", "OTHER"} {
		response := &http.Response{StatusCode: http.StatusBadRequest, Body: io.NopCloser(strings.NewReader(`{"code":"` + code + `","message":"bad"}`))}
		if err := remoteInvokeHTTPError(response, response.StatusCode); err == nil {
			t.Fatalf("remoteInvokeHTTPError(%q) = nil", code)
		}
	}
	if err := remoteInvokeStreamHTTPError(&http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader("not-json"))}, http.StatusBadGateway); err == nil {
		t.Fatal("remoteInvokeStreamHTTPError() = nil, want stable error")
	}

	var output bytes.Buffer
	if err := writeInvokeResult(InvokeConfig{Output: &output}, false, invokeResult{RequestID: "request", WorkerSessionID: "session", State: "RUNNING", Observation: "observe"}, false); err != nil || !strings.Contains(output.String(), "Worker Session admitted") {
		t.Fatalf("writeInvokeResult(async human) = %v, output=%q", err, output.String())
	}
	output.Reset()
	if err := writeInvokeResult(InvokeConfig{Output: &output}, false, invokeResult{WorkerSessionID: "session", State: "COMPLETED", Observation: "observe"}, true); err != nil || !strings.Contains(output.String(), "Worker Session session completed") {
		t.Fatalf("writeInvokeResult(sync human) = %v, output=%q", err, output.String())
	}
	if got := safeRemoteServer(":bad"); got != "<remote>" {
		t.Fatalf("safeRemoteServer(invalid) = %q, want <remote>", got)
	}
}

type invokeLocalFake struct {
	startResult      workersessions.StartResult
	startRequests    []workersessions.StartRequest
	continueResult   workersessions.ContinueResult
	continueErr      error
	continueRequests []workersessions.ContinueRequest
	deliveries       []workersessions.ObservationDelivery
	streamCalls      int
	closed           bool
}

func (fake *invokeLocalFake) Start(_ context.Context, request workersessions.StartRequest) (workersessions.StartResult, error) {
	fake.startRequests = append(fake.startRequests, request)
	return fake.startResult, nil
}

func (fake *invokeLocalFake) Continue(_ context.Context, request workersessions.ContinueRequest) (workersessions.ContinueResult, error) {
	fake.continueRequests = append(fake.continueRequests, request)
	return fake.continueResult, fake.continueErr
}

func (fake *invokeLocalFake) StreamObservationsByWorkerSessionID(_ context.Context, _ workersessions.StreamObservationsByWorkerSessionIDRequest) (workersessions.ObservationSubscription, error) {
	fake.streamCalls++
	index := 0
	return workersessions.ObservationSubscription{
		NextFunc: func(context.Context) workersessions.ObservationDelivery {
			if index >= len(fake.deliveries) {
				return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryClosed}
			}
			delivery := fake.deliveries[index]
			index++
			return delivery
		},
		CloseFunc: func() { fake.closed = true },
	}, nil
}

func invokeObservationDelivery(kind workersessions.ObservationDeliveryKind, schemaID, payload string) workersessions.ObservationDelivery {
	return workersessions.ObservationDelivery{Kind: kind, Event: workersessions.ObservationEvent{
		SchemaID: schemaID, Payload: json.RawMessage(payload),
	}}
}

type invokeProtocolStub struct {
	err        error
	requestURL string
}

func (stub *invokeProtocolStub) Execute(request *http.Request) (clihttp.Response, error) {
	stub.requestURL = request.URL.String()
	return clihttp.Response{}, stub.err
}

func (stub *invokeProtocolStub) GetJSON(context.Context, string, any) (clihttp.Response, error) {
	return clihttp.Response{}, stub.err
}

func (stub *invokeProtocolStub) PostJSON(context.Context, string, io.Reader, any) (clihttp.Response, error) {
	return clihttp.Response{}, stub.err
}

func (stub *invokeProtocolStub) PostJSONCreated(context.Context, string, io.Reader, any) (clihttp.Response, error) {
	return clihttp.Response{}, stub.err
}

func (stub *invokeProtocolStub) PutJSON(context.Context, string, io.Reader, any) (clihttp.Response, error) {
	return clihttp.Response{}, stub.err
}

func (stub *invokeProtocolStub) PutJSONCreated(context.Context, string, io.Reader, any) (clihttp.Response, error) {
	return clihttp.Response{}, stub.err
}

var _ LocalInvokeBoundary = (*invokeLocalFake)(nil)
var _ clihttp.Protocol = (*invokeProtocolStub)(nil)
