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

type invokeLocalFake struct {
	startResult   workersessions.StartResult
	startRequests []workersessions.StartRequest
	deliveries    []workersessions.ObservationDelivery
	streamCalls   int
	closed        bool
}

func (fake *invokeLocalFake) Start(_ context.Context, request workersessions.StartRequest) (workersessions.StartResult, error) {
	fake.startRequests = append(fake.startRequests, request)
	return fake.startResult, nil
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
