package run

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

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	contentcontract "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
)

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
	t.Run("context is required", func(t *testing.T) {
		_, err := remoteInvocationClient{transport: &remoteProtocolStub{}}.InvokeFactory(nil, RemoteInvocationRequest{})
		if err == nil || !strings.Contains(err.Error(), "context is required") {
			t.Fatalf("error = %v, want required-context error", err)
		}
	})

	t.Run("protocol is required", func(t *testing.T) {
		_, err := remoteInvocationClient{}.InvokeFactory(context.Background(), RemoteInvocationRequest{})
		if err == nil || !strings.Contains(err.Error(), "CLI HTTP protocol is required") {
			t.Fatalf("error = %v, want required-protocol error", err)
		}
	})

	t.Run("endpoint must be valid", func(t *testing.T) {
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
	})

	t.Run("request must be JSON encodable", func(t *testing.T) {
		bad := map[string]any{"function": func() {}}
		_, err := remoteInvocationClient{transport: &remoteProtocolStub{}}.InvokeFactory(context.Background(), RemoteInvocationRequest{
			Server:  "http://selected.test",
			Request: factoryapi.InvocationRequest{Args: &bad},
		})
		if err == nil || !strings.Contains(err.Error(), "marshal remote invocation request") {
			t.Fatalf("error = %v, want marshal error", err)
		}
	})

	t.Run("nil HTTP response is rejected", func(t *testing.T) {
		_, err := remoteInvocationClient{transport: &remoteProtocolStub{
			response: clihttp.Response{},
		}}.InvokeFactory(context.Background(), RemoteInvocationRequest{Server: "http://selected.test"})
		if err == nil || !strings.Contains(err.Error(), "HTTP response is unavailable") {
			t.Fatalf("error = %v, want missing-response error", err)
		}
	})

	t.Run("non API error status remains actionable", func(t *testing.T) {
		_, err := remoteInvocationClient{transport: &remoteProtocolStub{
			response: clihttp.Response{HTTP: &http.Response{
				StatusCode: http.StatusBadGateway,
				Body:       io.NopCloser(strings.NewReader("not an API error")),
			}},
		}}.InvokeFactory(context.Background(), RemoteInvocationRequest{Server: "http://selected.test"})
		if err == nil || !strings.Contains(err.Error(), "(502)") {
			t.Fatalf("error = %v, want HTTP status error", err)
		}
	})

	t.Run("success does not require a response body", func(t *testing.T) {
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
	})
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

func TestSafeRemoteEndpointRedactsCredentialsAndPreservesInvalidInput(t *testing.T) {
	if got := safeRemoteEndpoint("https://user:secret@selected.test/path"); got != "https://selected.test/path" {
		t.Fatalf("safe endpoint = %q, want credentials removed", got)
	}
	if got := safeRemoteEndpoint("http://[::1"); got != "http://[::1" {
		t.Fatalf("invalid safe endpoint = %q, want original input", got)
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
