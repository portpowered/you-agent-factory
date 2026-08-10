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

type remoteInvocationOperationFunc func(context.Context, RemoteInvocationRequest) (factoryapi.InvocationResponse, error)

func (fn remoteInvocationOperationFunc) InvokeFactory(ctx context.Context, request RemoteInvocationRequest) (factoryapi.InvocationResponse, error) {
	return fn(ctx, request)
}
