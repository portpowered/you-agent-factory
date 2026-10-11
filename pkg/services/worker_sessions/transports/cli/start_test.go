package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestPromptStartSelectedHost(t *testing.T) {
	t.Parallel()
	for _, timeout := range []*int64{nil, startTestMillis(12345)} {
		t.Run("deadline", func(t *testing.T) {
			t.Parallel()
			var received factoryapi.WorkerSessionStartRequest
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/worker-sessions" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
					t.Error(err)
				}
				w.WriteHeader(http.StatusAccepted)
				_, _ = io.WriteString(w, `{"requestId":"request","workerSessionId":"worker","accepted":true,"state":"RUNNING","eventTopic":"worker-session.worker"}`)
			}))
			defer server.Close()
			var output bytes.Buffer
			err := BindInvoke(testHTTPProtocol(t), nil, unexpectedWorkerID, nil)(InvokeConfig{
				Context: context.Background(), Server: server.URL, PromptStart: true,
				RequestID: "request", WorkerSessionID: "worker", Prompt: []string{"inspect", "code"},
				Provider: "codex", Model: "opaque-model", ReasoningEffort: "low", WorkingRoot: "host-root",
				TimeoutMillis: timeout, Output: &output, JSON: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			assertStartPromptRequest(t, received)
			if (received.TimeoutMillis == nil) != (timeout == nil) || timeout != nil && *received.TimeoutMillis != *timeout {
				t.Fatalf("execution deadline changed: %#v", received.TimeoutMillis)
			}
			if !strings.Contains(output.String(), `"workerSessionId":"worker"`) || !strings.Contains(output.String(), `"state":"RUNNING"`) {
				t.Fatalf("admission = %s", output.String())
			}
		})
	}
}

func assertStartPromptRequest(t *testing.T, received factoryapi.WorkerSessionStartRequest) {
	t.Helper()
	if received.Execution != nil || received.Retry != nil {
		t.Fatalf("prompt form contains resolved execution: %#v", received)
	}
	for _, field := range []struct {
		value *string
		want  string
	}{
		{received.Prompt, "inspect code"}, {received.Provider, "codex"}, {received.Model, "opaque-model"},
		{received.ReasoningEffort, "low"}, {received.WorkingRoot, "host-root"},
	} {
		if field.value == nil || *field.value != field.want {
			t.Fatalf("prompt field = %v, want %q", field.value, field.want)
		}
	}
}

func TestPromptStartRejectsInvalidBeforeHTTP(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		prompt  string
		timeout *int64
	}{
		{" ", nil}, {"work", startTestMillis(0)}, {"work", startTestMillis(-1)}, {"work", startTestMillis(9223372036855)},
	} {
		var output bytes.Buffer
		err := BindInvoke(testHTTPProtocol(t), nil, unexpectedWorkerID, nil)(InvokeConfig{
			Context: context.Background(), PromptStart: true, Server: "http://must-not-run.invalid",
			RequestID: "request", WorkerSessionID: "worker", Prompt: []string{test.prompt}, TimeoutMillis: test.timeout, Output: &output, JSON: true,
		})
		if err == nil || !strings.Contains(output.String(), "WORKER_SESSION_START_INVALID") {
			t.Fatalf("invalid request: %v %s", err, output.String())
		}
	}
}

func startTestMillis(value int64) *int64 { return &value }

func TestPromptStartOmittedRouteAndGeneratedIdentities(t *testing.T) {
	t.Parallel()
	sequence := []string{"generated-request", "generated-worker"}
	request, err := normalizePromptStart(InvokeConfig{
		Stdin:      strings.NewReader("  stdin prompt  "),
		GenerateID: func() string { value := sequence[0]; sequence = sequence[1:]; return value },
	})
	if err != nil {
		t.Fatal(err)
	}
	if request.API.RequestId != "generated-request" || request.API.WorkerSessionId != "generated-worker" || *request.API.Prompt != "stdin prompt" {
		t.Fatalf("generated prompt form = %#v", request.API)
	}
	if request.API.Provider != nil || request.API.Model != nil || request.API.ReasoningEffort != nil || request.API.TimeoutMillis != nil || request.API.Execution != nil {
		t.Fatalf("CLI supplied policy defaults: %#v", request.API)
	}
}

func TestPromptStartRouteRefusalPreservesTypedError(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"code":"SUBAGENT_ROUTE_INVALID","message":"configure defaults.subagent on the selected host"}`)
	}))
	defer server.Close()
	var output bytes.Buffer
	err := BindInvoke(testHTTPProtocol(t), nil, unexpectedWorkerID, nil)(InvokeConfig{
		Context: context.Background(), Server: server.URL, PromptStart: true,
		RequestID: "request", WorkerSessionID: "worker", Prompt: []string{"work"}, JSON: true, Output: &output,
	})
	if err == nil || !strings.Contains(output.String(), "SUBAGENT_ROUTE_INVALID") || !strings.Contains(output.String(), "defaults.subagent") {
		t.Fatalf("route refusal = %v %s", err, output.String())
	}
}
