package diagnostics_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type observedHTTPCall struct {
	method      string
	path        string
	contentType string
	body        []byte
}

func TestRemoteFactoryReplaceCurrentUsesJSONPutAndRendersSuccess(t *testing.T) {
	var calls []observedHTTPCall
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			http.Error(writer, "could not read request", http.StatusBadRequest)
			return
		}
		calls = append(calls, observedHTTPCall{
			method:      request.Method,
			path:        request.URL.Path,
			contentType: request.Header.Get("Content-Type"),
			body:        body,
		})

		if request.URL.Path != "/factory-sessions/~default/factory" {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case http.MethodGet:
			_ = json.NewEncoder(writer).Encode(factoryapi.Factory{Name: "remote"})
		case http.MethodPut:
			if request.Header.Get("Content-Type") != "application/json" {
				http.Error(writer, "missing JSON content type", http.StatusBadRequest)
				return
			}
			var payload factoryapi.SaveFactoryForSessionRequest
			if err := json.Unmarshal(body, &payload); err != nil || payload.Factory.Name != "remote" {
				http.Error(writer, "invalid replacement payload", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(writer).Encode(factoryapi.Factory{Name: "remote"})
		default:
			http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(server.Close)

	process := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
	inputs := remoteInputs(t, "you", "--json", "--server", server.URL, "factory", "replace-current")
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(factory replace-current) error = %v\nstdout=%q\nstderr=%q", err, inputs.Stdout(), inputs.Stderr())
	}
	if inputs.Stderr() != "" {
		t.Fatalf("successful remote replacement stderr = %q, want empty", inputs.Stderr())
	}
	var result factoryapi.Factory
	if err := json.Unmarshal([]byte(inputs.Stdout()), &result); err != nil {
		t.Fatalf("decode successful replacement stdout: %v\nstdout=%q", err, inputs.Stdout())
	}
	if result.Name != "remote" {
		t.Fatalf("successful replacement result name = %q, want remote", result.Name)
	}
	if len(calls) != 2 {
		t.Fatalf("observed HTTP calls = %#v, want GET followed by PUT", calls)
	}
	if calls[0].method != http.MethodGet || calls[1].method != http.MethodPut {
		t.Fatalf("observed HTTP methods = %#v, want GET followed by PUT", calls)
	}
	if calls[1].contentType != "application/json" {
		t.Fatalf("PUT content type = %q, want application/json", calls[1].contentType)
	}
}

func TestRemoteSubmitUsesCreatedResponseAndPreservesPayloadContract(t *testing.T) {
	var received factoryapi.SubmitWorkRequest
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/factory-sessions/~default/work" {
			http.NotFound(writer, request)
			return
		}
		if request.Header.Get("Content-Type") != "application/json" {
			http.Error(writer, "missing JSON content type", http.StatusBadRequest)
			return
		}
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			http.Error(writer, "invalid submit payload", http.StatusBadRequest)
			return
		}
		name := "remote-submit"
		workType := "task"
		workID := "work-remote-submit"
		sessionID := "~default"
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(writer).Encode(factoryapi.SubmitWorkResponse{
			Name:         &name,
			WorkTypeName: &workType,
			WorkId:       &workID,
			SessionId:    &sessionID,
			TraceId:      "trace-remote-submit",
		})
	}))
	t.Cleanup(server.Close)

	payloadPath := filepath.Join(t.TempDir(), "request.md")
	if err := os.WriteFile(payloadPath, []byte("remote payload"), 0o644); err != nil {
		t.Fatalf("write submit payload: %v", err)
	}

	process := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
	inputs := remoteInputs(t,
		"you", "--json", "--server", server.URL,
		"submit", "--name", "remote-submit", "--work-type-name", "task", "--payload", payloadPath,
	)
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(submit) error = %v\nstdout=%q\nstderr=%q", err, inputs.Stdout(), inputs.Stderr())
	}
	if inputs.Stderr() != "" {
		t.Fatalf("successful remote submit stderr = %q, want empty", inputs.Stderr())
	}
	var result struct {
		WorkID       string `json:"workId"`
		Name         string `json:"name"`
		WorkTypeName string `json:"workTypeName"`
		TraceID      string `json:"traceId"`
		SessionID    string `json:"sessionId"`
	}
	if err := json.Unmarshal([]byte(inputs.Stdout()), &result); err != nil {
		t.Fatalf("decode submit stdout: %v\nstdout=%q", err, inputs.Stdout())
	}
	if result.WorkID != "work-remote-submit" || result.Name != "remote-submit" ||
		result.WorkTypeName != "task" || result.TraceID != "trace-remote-submit" || result.SessionID != "~default" {
		t.Fatalf("submit result = %#v, want created response fields", result)
	}
	if received.Name == nil || *received.Name != "remote-submit" || received.WorkTypeName != "task" {
		t.Fatalf("received submit request = %#v, want name and work type", received)
	}
	payload, err := json.Marshal(received.Payload)
	if err != nil {
		t.Fatalf("marshal received payload: %v", err)
	}
	if string(payload) != `"remote payload"` {
		t.Fatalf("received payload = %s, want JSON string payload", payload)
	}
}

func TestRunFlagConflictRendersCodedBadRequest(t *testing.T) {
	inputs := remoteInputs(t, "you", "--json", "run", "--resume", "resume.json", "--no-record")
	process := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
	if err := process.Execute(inputs.Input); err == nil {
		t.Fatal("Process.Execute(run flag conflict) error = nil, want rejected flags")
	}
	if inputs.Stdout() != "" {
		t.Fatalf("run flag conflict stdout = %q, want empty", inputs.Stdout())
	}
	response := decodeFirstErrorResponse(t, inputs.Stderr())
	if response.Code != factoryapi.ErrorResponseCode("CLI_FLAG_CONFLICT") ||
		response.Family != factoryapi.ErrorFamilyBadRequest ||
		response.Message != "--resume cannot be used with --no-record" {
		t.Fatalf("run flag conflict diagnostic = %#v, want coded bad-request response", response)
	}
}

func TestRemoteTransportFailureUsesSafeFallbackAndDebugMetadata(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	serverURL := server.URL
	server.Close()

	process := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
	inputs := remoteInputs(t, "you", "--json", "--debug", "--server", serverURL, "session", "show", "missing")
	if err := process.Execute(inputs.Input); err == nil {
		t.Fatal("Process.Execute(unreachable remote session) error = nil, want transport failure")
	}
	if inputs.Stdout() != "" {
		t.Fatalf("unreachable remote session stdout = %q, want empty", inputs.Stdout())
	}
	response := decodeFirstErrorResponse(t, inputs.Stderr())
	if response.Code != factoryapi.ErrorResponseCode("CLI_COMMAND_FAILED") ||
		response.Family != factoryapi.ErrorFamilyInternalServerError ||
		response.Message != "command failed" {
		t.Fatalf("transport failure diagnostic = %#v, want safe fallback", response)
	}
	for _, want := range []string{
		"debug: http method=GET",
		"url=" + serverURL + "/factory-sessions/missing",
		"status=<unavailable>",
	} {
		if !strings.Contains(inputs.Stderr(), want) {
			t.Fatalf("transport failure debug diagnostic = %q, want %q", inputs.Stderr(), want)
		}
	}
}

func TestRemoteRunPreservesStructuredAdmissionDiagnostic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/factory-sessions/async" {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(writer).Encode(factoryapi.ErrorResponse{
			Code:    factoryapi.ErrorResponseCode("REMOTE_ADMISSION_REJECTED"),
			Family:  factoryapi.ErrorFamilyInternalServerError,
			Message: "remote admission is unavailable",
		})
	}))
	t.Cleanup(server.Close)

	factoryPath := filepath.Join(t.TempDir(), "remote-factory.json")
	if err := os.WriteFile(factoryPath, []byte(remoteDiagnosticFactoryJSON), 0o600); err != nil {
		t.Fatalf("write remote run factory: %v", err)
	}

	process := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
	inputs := remoteInputs(t,
		"you", "--json", "--remote", "--server", server.URL,
		"run", "--factory", factoryPath, "--no-record", "remote request",
	)
	if err := process.Execute(inputs.Input); err == nil {
		t.Fatal("Process.Execute(remote run) error = nil, want admission failure")
	}
	if inputs.Stdout() != "" {
		t.Fatalf("remote run admission stdout = %q, want empty", inputs.Stdout())
	}
	response := decodeFirstErrorResponse(t, inputs.Stderr())
	if response.Code != factoryapi.ErrorResponseCode("REMOTE_DURABLE_START_FAILED") ||
		response.Family != factoryapi.ErrorFamilyInternalServerError ||
		!strings.Contains(response.Message, "remote admission is unavailable") {
		t.Fatalf("remote run admission diagnostic = %#v, want preserved failure context", response)
	}
}

func TestRemoteSessionShowPreservesStructuredServerDiagnostic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/factory-sessions/missing" {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(writer).Encode(factoryapi.ErrorResponse{
			Code:    factoryapi.ErrorResponseCodeNOTFOUND,
			Family:  factoryapi.ErrorFamilyNotFound,
			Message: "remote session is unavailable",
		})
	}))
	t.Cleanup(server.Close)

	process := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
	for _, debug := range []bool{false, true} {
		name := "normal"
		args := []string{"you", "--json"}
		if debug {
			name = "debug"
			args = append(args, "--debug")
		}
		args = append(args, "--server", server.URL, "session", "show", "missing")

		t.Run(name, func(t *testing.T) {
			inputs := remoteInputs(t, args...)
			err := process.Execute(inputs.Input)
			if err == nil {
				t.Fatal("Process.Execute(session show) error = nil, want server failure")
			}
			if inputs.Stdout() != "" {
				t.Fatalf("server failure stdout = %q, want empty", inputs.Stdout())
			}
			response := decodeFirstErrorResponse(t, inputs.Stderr())
			if response.Code != factoryapi.ErrorResponseCodeNOTFOUND ||
				response.Family != factoryapi.ErrorFamilyNotFound ||
				response.Message != "remote session is unavailable" {
				t.Fatalf("server failure diagnostic = %#v, want preserved NOT_FOUND response", response)
			}
			if !debug {
				var apiError *clihttp.APIError
				if !errors.As(err, &apiError) {
					t.Fatalf("server failure error = %T, want clihttp.APIError", err)
				}
				fallback := *apiError
				fallback.DisplayMessage = ""
				if got := fallback.Error(); got != response.Message {
					t.Fatalf("APIError fallback message = %q, want %q", got, response.Message)
				}
			}
			if debug {
				for _, want := range []string{
					"debug: http method=GET",
					"status=404",
					"url=" + server.URL + "/factory-sessions/missing",
				} {
					if !strings.Contains(inputs.Stderr(), want) {
						t.Fatalf("debug diagnostic = %q, want %q", inputs.Stderr(), want)
					}
				}
			}
		})
	}
}

func TestRemoteFactoryShowMalformedResponseUsesSafeDebugDiagnostic(t *testing.T) {
	const opaqueResponseMarker = "opaque-response-secret"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/factory-sessions/~default/factory" {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(writer, `{"name":"`+opaqueResponseMarker+`"`)
	}))
	t.Cleanup(server.Close)

	process := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
	inputs := remoteInputs(t, "you", "--debug", "--server", server.URL, "factory", "show")
	if err := process.Execute(inputs.Input); err == nil {
		t.Fatal("Process.Execute(factory show) error = nil, want malformed-response failure")
	}
	if inputs.Stdout() != "" {
		t.Fatalf("malformed-response stdout = %q, want empty", inputs.Stdout())
	}
	response := decodeFirstErrorResponse(t, inputs.Stderr())
	if response.Code != factoryapi.ErrorResponseCode("CLI_COMMAND_FAILED") ||
		response.Family != factoryapi.ErrorFamilyInternalServerError ||
		response.Message != "command failed" {
		t.Fatalf("malformed-response diagnostic = %#v, want safe CLI_COMMAND_FAILED fallback", response)
	}
	for _, want := range []string{
		"debug: http method=GET",
		"status=200",
		"url=" + server.URL + "/factory-sessions/~default/factory",
	} {
		if !strings.Contains(inputs.Stderr(), want) {
			t.Fatalf("malformed-response debug diagnostic = %q, want %q", inputs.Stderr(), want)
		}
	}
	if strings.Contains(inputs.Stderr(), opaqueResponseMarker) {
		t.Fatalf("malformed-response debug diagnostic leaked opaque response body: %q", inputs.Stderr())
	}
}

func remoteInputs(t *testing.T, args ...string) *support.CapturedInputs {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), args)
	home := t.TempDir()
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = t.TempDir()
	return inputs
}

func decodeFirstErrorResponse(t *testing.T, stderr string) factoryapi.ErrorResponse {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(stderr), "\n") {
		var response factoryapi.ErrorResponse
		if err := json.Unmarshal([]byte(line), &response); err == nil && response.Code != "" {
			return response
		}
	}
	t.Fatalf("decode CLI error response: stderr=%q", stderr)
	return factoryapi.ErrorResponse{}
}

const remoteDiagnosticFactoryJSON = `{
  "name": "remote-diagnostics",
  "invocationSignature": {
    "parameters": [{
      "name": "prompt",
      "required": true,
      "bindings": [{"kind": "POSITIONAL", "position": 1}]
    }]
  },
  "workTypes": [{
    "name": "task",
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "complete", "type": "TERMINAL"},
      {"name": "failed", "type": "FAILED"}
    ]
  }],
  "workers": [{"name": "processor"}],
  "workstations": [{
    "name": "process",
    "inputs": [{"workType": "task", "state": "init"}],
    "outputs": [{"workType": "task", "state": "complete"}],
    "onFailure": [{"workType": "task", "state": "failed"}],
    "worker": "processor"
  }]
}`
