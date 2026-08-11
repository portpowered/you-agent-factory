package run

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	contentcontract "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_NamedFactoryModelNotReadyKeepsStdoutEmpty(t *testing.T) {
	preserveRunGlobals(t)

	text := "hi there"
	var output bytes.Buffer
	core, observedLogs := observer.New(zap.InfoLevel)

	openTestInvocationRunner = func(_ context.Context, cfg *testRuntimeSelections, _ serviceedges.Edges) (sessionInvocationRunner, error) {
		return stubInvocationService{
			run: func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
			invoke: func(_ context.Context, _ string, _ factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				return apisurface.FactoryInvocationResult{
					RequestID: "request-tts-not-ready",
					TraceID:   "trace-tts-not-ready",
					Status:    interfaces.InvocationTerminalStatusFailed,
					ErrorCode: interfaces.TTSInvocationErrorCodeModelNotReady,
					Message:   "model not available: required assets missing",
				}, nil
			},
		}, nil
	}

	err := Run(context.Background(), RunConfig{
		Dir:                      "/tmp/builtin-tts",
		NamedFactoryName:         "@you/tts",
		InvocationPositionalText: &text,
		StdinIsTTY:               func() bool { return true },
		Output:                   &output,
		Port:                     7437,
		Logger:                   zap.New(core),
	})
	if err == nil {
		t.Fatal("expected model-not-ready invocation failure")
	}
	if !strings.Contains(err.Error(), interfaces.TTSInvocationErrorCodeModelNotReady) {
		t.Fatalf("error = %q, want %s", err.Error(), interfaces.TTSInvocationErrorCodeModelNotReady)
	}
	if output.Len() != 0 {
		t.Fatalf("stdout = %q, want empty without success metadata", output.String())
	}

	startLogs := observedLogs.FilterMessage("packaged tts invocation started").All()
	if len(startLogs) != 1 {
		t.Fatalf("packaged start logs = %d, want 1", len(startLogs))
	}
	if got := startLogs[0].ContextMap()["tts_backend"]; got == "" {
		t.Fatal("expected tts_backend field in packaged start log")
	}
}

func TestRun_NamedFactoryGenerationFailureKeepsStdoutEmpty(t *testing.T) {
	preserveRunGlobals(t)

	text := "hi there"
	var output bytes.Buffer

	openTestInvocationRunner = func(_ context.Context, _ *testRuntimeSelections, _ serviceedges.Edges) (sessionInvocationRunner, error) {
		return stubInvocationService{
			run: func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
			invoke: func(_ context.Context, _ string, _ factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				return apisurface.FactoryInvocationResult{
					RequestID: "request-tts-failed",
					TraceID:   "trace-tts-failed",
					Status:    interfaces.InvocationTerminalStatusFailed,
					ErrorCode: interfaces.TTSInvocationErrorCodeGenerationFailed,
					Message:   "omnivoice invoke failed: exit status 1",
				}, nil
			},
		}, nil
	}

	err := Run(context.Background(), RunConfig{
		Dir:                      "/tmp/builtin-tts",
		NamedFactoryName:         "@you/tts",
		InvocationPositionalText: &text,
		StdinIsTTY:               func() bool { return true },
		Output:                   &output,
		Port:                     7437,
	})
	if err == nil {
		t.Fatal("expected generation failure")
	}
	if !strings.Contains(err.Error(), interfaces.TTSInvocationErrorCodeGenerationFailed) {
		t.Fatalf("error = %q, want %s", err.Error(), interfaces.TTSInvocationErrorCodeGenerationFailed)
	}
	if output.Len() != 0 {
		t.Fatalf("stdout = %q, want empty without success metadata", output.String())
	}
}

func TestRun_NamedFactoryStdinInvocationWritesMetadataPrimaryResult(t *testing.T) {
	preserveRunGlobals(t)

	stdinText := "hi there"
	metadataJSON := `{"artifactPath":"/tmp/speech.wav","mediaType":"audio/wav","backend":"OMNIVOICE_Q4_K_M/LLAMACPP"}`
	var output bytes.Buffer

	openTestInvocationRunner = func(_ context.Context, cfg *testRuntimeSelections, _ serviceedges.Edges) (sessionInvocationRunner, error) {
		return stubInvocationService{
			run: func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
			invoke: func(_ context.Context, sessionID string, request factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				if sessionID != defaultFactorySessionID {
					t.Fatalf("sessionID = %q, want %q", sessionID, defaultFactorySessionID)
				}
				if got := extractInvocationText(t, &request); got != stdinText {
					t.Fatalf("invocation text = %q, want %q", got, stdinText)
				}
				return apisurface.FactoryInvocationResult{
					RequestID: "request-tts-stdin",
					TraceID:   "trace-tts-stdin",
					Status:    interfaces.InvocationTerminalStatusCompleted,
					PrimaryResult: []work.WorkContentPart{{
						Type: work.WorkContentPartTypeText,
						Text: metadataJSON,
					}},
				}, nil
			},
		}, nil
	}

	err := Run(context.Background(), RunConfig{
		Dir:                 "/tmp/builtin-tts",
		NamedFactoryName:    "@you/tts",
		InvocationStdinText: &stdinText,
		StdinIsTTY:          func() bool { return true },
		Output:              &output,
		Port:                7437,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := output.String(); got != metadataJSON {
		t.Fatalf("stdout = %q, want packaged TTS metadata JSON", got)
	}

	var metadata interfaces.TTSInvocationMetadata
	if err := json.Unmarshal([]byte(output.String()), &metadata); err != nil {
		t.Fatalf("metadata JSON: %v", err)
	}
	if metadata.ArtifactPath != "/tmp/speech.wav" || metadata.MediaType != "audio/wav" || metadata.Backend == "" {
		t.Fatalf("metadata = %#v, want artifact path, media type, and backend", metadata)
	}
}

func TestRun_FactoryInvocationWritesPrimaryTextOnly(t *testing.T) {
	preserveRunGlobals(t)

	text := "Fix the lint issues"
	var output bytes.Buffer
	var captured *testRuntimeSelections

	openTestInvocationRunner = func(_ context.Context, cfg *testRuntimeSelections, edges serviceedges.Edges) (sessionInvocationRunner, error) {
		captured = cfg
		_ = edges
		return stubInvocationService{
			run: func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
			invoke: func(_ context.Context, sessionID string, request factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				if sessionID != defaultFactorySessionID {
					t.Fatalf("sessionID = %q, want %q", sessionID, defaultFactorySessionID)
				}
				if got := extractInvocationText(t, &request); got != text {
					t.Fatalf("invocation text = %q, want %q", got, text)
				}
				return apisurface.FactoryInvocationResult{
					RequestID: "request-123",
					TraceID:   "trace-123",
					Status:    interfaces.InvocationTerminalStatusCompleted,
					PrimaryResult: []work.WorkContentPart{{
						Type: work.WorkContentPartTypeText,
						Text: "final output",
					}},
				}, nil
			},
		}, nil
	}

	err := Run(context.Background(), RunConfig{
		FactoryConfigPath:        "/tmp/factory.json",
		InvocationPositionalText: &text,
		StdinIsTTY:               func() bool { return true },
		Output:                   &output,
		Port:                     7437,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := output.String(); got != "final output" {
		t.Fatalf("stdout = %q, want only primary result text", got)
	}
	if captured == nil {
		t.Fatal("expected invocation run to build a service config")
	}
	if captured.RuntimeMode != interfaces.RuntimeModeService {
		t.Fatalf("runtime mode = %q, want service", captured.RuntimeMode)
	}
	if captured.WorkFile != "" {
		t.Fatalf("work file = %q, want empty for invocation mode", captured.WorkFile)
	}
}

func TestRun_FactoryInvocationFailureKeepsStdoutEmpty(t *testing.T) {
	preserveRunGlobals(t)

	text := "Fix the lint issues"
	var output bytes.Buffer

	openTestInvocationRunner = func(_ context.Context, cfg *testRuntimeSelections, _ serviceedges.Edges) (sessionInvocationRunner, error) {
		return stubInvocationService{
			run: func(ctx context.Context) error {
				<-ctx.Done()
				return nil
			},
			invoke: func(_ context.Context, _ string, _ factoryapi.InvocationRequest) (apisurface.FactoryInvocationResult, error) {
				return apisurface.FactoryInvocationResult{
					RequestID: "request-123",
					TraceID:   "trace-123",
					Status:    interfaces.InvocationTerminalStatusFailed,
					ErrorCode: "INVOCATION_PRIMARY_RESULT_UNRESOLVED",
					Message:   "primary result could not be resolved",
				}, nil
			},
		}, nil
	}

	err := Run(context.Background(), RunConfig{
		FactoryConfigPath:        "/tmp/factory.json",
		InvocationPositionalText: &text,
		StdinIsTTY:               func() bool { return true },
		Output:                   &output,
		Port:                     7437,
	})
	if err == nil {
		t.Fatal("expected invocation failure")
	}
	if !strings.Contains(err.Error(), "INVOCATION_PRIMARY_RESULT_UNRESOLVED") {
		t.Fatalf("error = %q, want stable unresolved code", err.Error())
	}
	if output.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on invocation failure", output.String())
	}
}

func TestRunRemoteInvocationUsesSelectedEndpointAndNormalizedRequest(t *testing.T) {
	var gotRequest factoryapi.InvocationRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/selected/factory-sessions/~default/invocations" {
			t.Fatalf("path = %q, want selected invocation endpoint", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotRequest); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		response := factoryapi.InvocationResponse{
			RequestId: "request-remote",
			TraceId:   "trace-remote",
			Status:    factoryapi.InvocationTerminalStatusCompleted,
			PrimaryResult: contentcontract.GeneratedPtrFromParts([]work.WorkContentPart{{
				Type: work.WorkContentPartTypeText, Text: "remote result",
			}}),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	transport, err := clihttp.NewProtocol(server.Client(), platformclock.Real{})
	if err != nil {
		t.Fatalf("NewProtocol: %v", err)
	}
	text := "same normalized prompt"
	var output bytes.Buffer
	err = RunRemoteInvocation(context.Background(), RunConfig{
		Dir:                      "factory",
		InvocationPositionalText: &text,
		JSONOutput:               true,
		InvocationOutputMode:     InvocationOutputPrimaryResult,
		Output:                   &output,
	}, server.URL+"/selected", NewRemoteInvocation(transport))
	if err != nil {
		t.Fatalf("RunRemoteInvocation: %v", err)
	}
	parts := contentcontract.PartsFromGenerated(gotRequest.Content)
	if len(parts) != 1 || parts[0].Text != text {
		t.Fatalf("remote request content = %#v, want normalized prompt", gotRequest.Content)
	}
	var gotResponse factoryapi.InvocationResponse
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &gotResponse); err != nil {
		t.Fatalf("decode CLI response: %v; output=%q", err, output.String())
	}
	if gotResponse.RequestId != "request-remote" || gotResponse.TraceId != "trace-remote" {
		t.Fatalf("CLI response identity = (%q, %q)", gotResponse.RequestId, gotResponse.TraceId)
	}
}

func TestRunRemoteInvocationPassesPreparedArguments(t *testing.T) {
	text := "local adapter must not run"
	prepared := work.PreparedInvocationInput{
		NormalizedArguments: &work.NormalizedArguments{
			Arguments: map[string]work.NormalizedArgument{
				"prompt": {Values: []string{text}},
			},
		},
	}
	var got RemoteInvocationRequest
	remote := remoteInvocationOperationFunc(func(_ context.Context, request RemoteInvocationRequest) (factoryapi.InvocationResponse, error) {
		got = request
		return factoryapi.InvocationResponse{
			RequestId: "request-arguments", TraceId: "trace-arguments",
			Status: factoryapi.InvocationTerminalStatusCompleted,
			PrimaryResult: contentcontract.GeneratedPtrFromParts([]work.WorkContentPart{{
				Type: work.WorkContentPartTypeText, Text: "ok",
			}}),
		}, nil
	})
	var output bytes.Buffer
	err := RunRemoteInvocation(context.Background(), RunConfig{
		Dir:                      "factory",
		PreparedInvocationInput:  &prepared,
		InvocationOutputMode:     InvocationOutputPrimaryResult,
		InvocationOutputExplicit: true,
		Output:                   &output,
	}, "http://selected.test", remote)
	if err != nil {
		t.Fatalf("RunRemoteInvocation: %v", err)
	}
	if got.Server != "http://selected.test" {
		t.Fatalf("remote server = %q, want selected server", got.Server)
	}
	if got.Request.Args == nil || (*got.Request.Args)["prompt"] != text {
		t.Fatalf("remote normalized arguments = %#v, want prompt=%q", got.Request.Args, text)
	}
}

func TestRemoteInvocationFailureDoesNotLeakRequestOrRetryLocally(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"code":"UNAVAILABLE","message":"remote service unavailable","detail":"sensitive request"}`)
	}))
	defer server.Close()
	transport, err := clihttp.NewProtocol(server.Client(), platformclock.Real{})
	if err != nil {
		t.Fatalf("NewProtocol: %v", err)
	}
	secret := "do not echo this payload"
	text := secret
	err = RunRemoteInvocation(context.Background(), RunConfig{
		Dir:                      "factory",
		InvocationPositionalText: &text,
		Output:                   io.Discard,
	}, server.URL, NewRemoteInvocation(transport))
	if err == nil {
		t.Fatal("RunRemoteInvocation error = nil, want remote failure")
	}
	if !strings.Contains(err.Error(), server.URL) || !strings.Contains(err.Error(), "remote service unavailable") {
		t.Fatalf("error = %q, want selected endpoint and stable remote message", err)
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "sensitive request") {
		t.Fatalf("error leaked sensitive request data: %q", err)
	}
	if errors.Is(err, context.Canceled) {
		t.Fatalf("remote failure unexpectedly mapped to cancellation: %v", err)
	}
}

func TestRemoteInvocationClientRejectsInvalidInputsAndResponses(t *testing.T) {
	t.Run("context is required", testRemoteInvocationRequiresContext)
	t.Run("protocol is required", testRemoteInvocationRequiresProtocol)
	t.Run("endpoint must be valid", testRemoteInvocationRejectsInvalidEndpoint)
	t.Run("request must be JSON encodable", testRemoteInvocationRejectsUnencodableRequest)
	t.Run("nil HTTP response is rejected", testRemoteInvocationRejectsMissingResponse)
	t.Run("non API error status remains actionable", testRemoteInvocationReportsNonAPIError)
	t.Run("success does not require a response body", testRemoteInvocationAcceptsEmptySuccessBody)
}

func testRemoteInvocationRequiresContext(t *testing.T) {
	_, err := remoteInvocationClient{transport: &remoteProtocolStub{}}.InvokeFactory(nil, RemoteInvocationRequest{})
	if err == nil || !strings.Contains(err.Error(), "context is required") {
		t.Fatalf("error = %v, want required-context error", err)
	}
}

func testRemoteInvocationRequiresProtocol(t *testing.T) {
	_, err := remoteInvocationClient{}.InvokeFactory(context.Background(), RemoteInvocationRequest{})
	if err == nil || !strings.Contains(err.Error(), "CLI HTTP protocol is required") {
		t.Fatalf("error = %v, want required-protocol error", err)
	}
}

func testRemoteInvocationRejectsInvalidEndpoint(t *testing.T) {
	stub := &remoteProtocolStub{}
	_, err := remoteInvocationClient{transport: stub}.InvokeFactory(context.Background(), RemoteInvocationRequest{
		Server: "http://[::1",
	})
	if err == nil || !strings.Contains(err.Error(), "endpoint") {
		t.Fatalf("error = %v, want invalid-endpoint error", err)
	}
	if stub.called {
		t.Fatal("invalid endpoint called the HTTP protocol")
	}
}

func testRemoteInvocationRejectsUnencodableRequest(t *testing.T) {
	bad := map[string]any{"function": func() {}}
	_, err := remoteInvocationClient{transport: &remoteProtocolStub{}}.InvokeFactory(context.Background(), RemoteInvocationRequest{
		Server:  "http://selected.test",
		Request: factoryapi.InvocationRequest{Args: &bad},
	})
	if err == nil || !strings.Contains(err.Error(), "marshal remote invocation request") {
		t.Fatalf("error = %v, want marshal error", err)
	}
}

func testRemoteInvocationRejectsMissingResponse(t *testing.T) {
	_, err := remoteInvocationClient{transport: &remoteProtocolStub{
		response: clihttp.Response{},
	}}.InvokeFactory(context.Background(), RemoteInvocationRequest{Server: "http://selected.test"})
	if err == nil || !strings.Contains(err.Error(), "HTTP response is unavailable") {
		t.Fatalf("error = %v, want missing-response error", err)
	}
}

func testRemoteInvocationReportsNonAPIError(t *testing.T) {
	_, err := remoteInvocationClient{transport: &remoteProtocolStub{
		response: clihttp.Response{HTTP: &http.Response{
			StatusCode: http.StatusBadGateway,
			Body:       io.NopCloser(strings.NewReader("not an API error")),
		}},
	}}.InvokeFactory(context.Background(), RemoteInvocationRequest{Server: "http://selected.test"})
	if err == nil || !strings.Contains(err.Error(), "(502)") {
		t.Fatalf("error = %v, want HTTP status error", err)
	}
}

func testRemoteInvocationAcceptsEmptySuccessBody(t *testing.T) {
	stub := &remoteProtocolStub{response: clihttp.Response{HTTP: &http.Response{StatusCode: http.StatusOK}}}
	response, err := remoteInvocationClient{transport: stub}.InvokeFactory(context.Background(), RemoteInvocationRequest{
		Server:    "https://selected.test",
		SessionID: " session-beta ",
	})
	if err != nil {
		t.Fatalf("InvokeFactory: %v", err)
	}
	if response.Status != "" || !strings.HasSuffix(stub.url, "/factory-sessions/session-beta/invocations") {
		t.Fatalf("response/url = %#v/%q, want empty response and scoped session URL", response, stub.url)
	}
}

func TestRunRemoteInvocationReportsTerminalStatesAndInputErrors(t *testing.T) {
	text := "remote terminal state"
	failedCode := factoryapi.InvocationResponseErrorCode("REMOTE_FAILED")
	sessionID, workID, workName, workState := "session-1", "work-1", "work name", "failed"
	message := "remote terminal failure"
	response := factoryapi.InvocationResponse{
		ErrorCode: &failedCode,
		Message:   &message,
		RequestId: "request-1",
		SessionId: &sessionID,
		Status:    factoryapi.InvocationTerminalStatusFailed,
		TraceId:   "trace-1",
		WorkId:    &workID,
		WorkName:  &workName,
		WorkState: &workState,
	}

	t.Run("nil operation is rejected", func(t *testing.T) {
		err := RunRemoteInvocation(context.Background(), RunConfig{}, "", nil)
		if err == nil || !strings.Contains(err.Error(), "operation is required") {
			t.Fatalf("error = %v, want required-operation error", err)
		}
	})

	t.Run("missing invocation input has stable code", func(t *testing.T) {
		err := RunRemoteInvocation(context.Background(), RunConfig{Dir: "factory"}, "", remoteInvocationOperationFunc(func(context.Context, RemoteInvocationRequest) (factoryapi.InvocationResponse, error) {
			t.Fatal("remote operation called without invocation input")
			return factoryapi.InvocationResponse{}, nil
		}))
		var invocationErr *InvocationError
		if !errors.As(err, &invocationErr) || invocationErr.Code != RemoteInvocationInputRequiredCode {
			t.Fatalf("error = %v, want %s", err, RemoteInvocationInputRequiredCode)
		}
	})

	t.Run("missing terminal status is rejected", func(t *testing.T) {
		err := RunRemoteInvocation(context.Background(), RunConfig{Dir: "factory", InvocationPositionalText: &text}, "", remoteInvocationOperationFunc(func(context.Context, RemoteInvocationRequest) (factoryapi.InvocationResponse, error) {
			return factoryapi.InvocationResponse{}, nil
		}))
		if err == nil || !strings.Contains(err.Error(), "no terminal status") {
			t.Fatalf("error = %v, want missing-status error", err)
		}
	})

	t.Run("terminal failure preserves response fields", func(t *testing.T) {
		got := invocationResultFromRemoteResponse(response)
		if got.ErrorCode != string(failedCode) || got.Message != message || got.SessionID != sessionID || got.WorkID != workID || got.WorkName != workName || got.WorkState != workState {
			t.Fatalf("mapped response = %#v, want response metadata", got)
		}
		var output bytes.Buffer
		err := RunRemoteInvocation(context.Background(), RunConfig{
			Dir:                      "factory",
			InvocationPositionalText: &text,
			JSONOutput:               true,
			Output:                   &output,
		}, "", remoteInvocationOperationFunc(func(context.Context, RemoteInvocationRequest) (factoryapi.InvocationResponse, error) {
			return response, nil
		}))
		if err == nil || output.Len() == 0 {
			t.Fatalf("error/output = %v/%q, want terminal failure and rendered JSON", err, output.String())
		}
	})
}

func TestSafeRemoteEndpointRedactsCredentialsAndFailsClosedForInvalidInput(t *testing.T) {
	if got := safeRemoteEndpoint("https://user:secret@selected.test/path"); got != "https://selected.test/path" {
		t.Fatalf("safe endpoint = %q, want credentials removed", got)
	}
	for _, endpoint := range []string{"http://[::1", "http://user:secret@[::1"} {
		if got := safeRemoteEndpoint(endpoint); got != invalidRemoteEndpointLabel {
			t.Fatalf("invalid safe endpoint %q = %q, want fixed redacted label", endpoint, got)
		}
	}
}

type remoteProtocolStub struct {
	response clihttp.Response
	err      error
	called   bool
	url      string
}

func (stub *remoteProtocolStub) Execute(*http.Request) (clihttp.Response, error) {
	return stub.response, stub.err
}

func (stub *remoteProtocolStub) GetJSON(context.Context, string, any) (clihttp.Response, error) {
	return stub.response, stub.err
}

func (stub *remoteProtocolStub) PostJSON(_ context.Context, url string, _ io.Reader, _ any) (clihttp.Response, error) {
	stub.called = true
	stub.url = url
	return stub.response, stub.err
}

func (stub *remoteProtocolStub) PostJSONCreated(context.Context, string, io.Reader, any) (clihttp.Response, error) {
	return stub.response, stub.err
}

func (stub *remoteProtocolStub) PutJSON(context.Context, string, io.Reader, any) (clihttp.Response, error) {
	return stub.response, stub.err
}

func (stub *remoteProtocolStub) PutJSONCreated(context.Context, string, io.Reader, any) (clihttp.Response, error) {
	return stub.response, stub.err
}

type remoteInvocationOperationFunc func(context.Context, RemoteInvocationRequest) (factoryapi.InvocationResponse, error)

func (fn remoteInvocationOperationFunc) InvokeFactory(ctx context.Context, request RemoteInvocationRequest) (factoryapi.InvocationResponse, error) {
	return fn(ctx, request)
}
