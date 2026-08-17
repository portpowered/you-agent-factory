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

func TestControlLocalMapsAllActionsAndJSONResult(t *testing.T) {
	for _, action := range []workersessions.ControlAction{
		workersessions.ControlActionPause, workersessions.ControlActionResume,
		workersessions.ControlActionCancel, workersessions.ControlActionTerminate,
	} {
		t.Run(strings.ToLower(string(action)), func(t *testing.T) {
			local := &controlLocalFake{results: map[workersessions.ControlAction]workersessions.ControlResult{
				action: {Session: workersessions.Session{ID: "worker-1", State: workersessions.StateRunning}, Action: action, Outcome: workersessions.ControlOutcomeApplied, DispatchID: "dispatch-1"},
			}}
			var output bytes.Buffer
			err := NewControl(nil, local)(ControlConfig{
				Context: context.Background(), WorkerSessionID: "worker-1", Action: action,
				OutputFormat: "json", Output: &output,
			})
			if err != nil {
				t.Fatalf("control error = %v", err)
			}
			var result factoryapi.WorkerSessionControlResponse
			if err := json.Unmarshal(output.Bytes(), &result); err != nil {
				t.Fatalf("decode result: %v; output=%q", err, output.String())
			}
			if result.WorkerSessionId != "worker-1" || workersessions.ControlAction(result.Action) != action ||
				result.Outcome != factoryapi.WorkerSessionControlResponseOutcomeApplied || result.DispatchId != "dispatch-1" {
				t.Fatalf("result = %#v, want exact action and outcome", result)
			}
			if local.request.ID != "worker-1" || local.action != action {
				t.Fatalf("local request = %#v action=%q, want worker-1/%q", local.request, local.action, action)
			}
		})
	}
}

func TestControlLocalHumanResultPreservesNoopTerminalSnapshot(t *testing.T) {
	local := &controlLocalFake{results: map[workersessions.ControlAction]workersessions.ControlResult{
		workersessions.ControlActionCancel: {
			Session: workersessions.Session{ID: "worker-1", State: workersessions.StateCanceled},
			Action:  workersessions.ControlActionCancel, Outcome: workersessions.ControlOutcomeNoop, DispatchID: "dispatch-1",
		},
	}}
	var output bytes.Buffer
	if err := NewControl(nil, local)(ControlConfig{
		Context: context.Background(), WorkerSessionID: "worker-1", Action: workersessions.ControlActionCancel,
		OutputFormat: "human", Output: &output,
	}); err != nil {
		t.Fatalf("human control error = %v", err)
	}
	for _, want := range []string{"Worker Session noop cancel", "Outcome:\tNOOP", "State:\tCANCELED", "Dispatch ID:\tdispatch-1"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("human control output = %q, want %q", output.String(), want)
		}
	}
}

func TestControlRemoteUsesExactActionRouteAndDoesNotFallback(t *testing.T) {
	for _, action := range []workersessions.ControlAction{
		workersessions.ControlActionPause, workersessions.ControlActionResume,
		workersessions.ControlActionCancel, workersessions.ControlActionTerminate,
	} {
		t.Run(strings.ToLower(string(action)), func(t *testing.T) {
			local := &controlLocalFake{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				wantPath := "/worker-sessions/worker-1/" + strings.ToLower(string(action))
				if r.Method != http.MethodPost || r.URL.Path != wantPath {
					t.Fatalf("request = %s %s, want POST %s", r.Method, r.URL.Path, wantPath)
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(factoryapi.WorkerSessionControlResponse{
					WorkerSessionId: "worker-1", Action: factoryapi.WorkerSessionControlResponseAction(action),
					Outcome: factoryapi.WorkerSessionControlResponseOutcomeNoop,
					State:   factoryapi.WorkerSessionControlResponseStateRunning,
				})
			}))
			defer server.Close()
			var output bytes.Buffer
			err := NewControl(testHTTPProtocol(t), local)(ControlConfig{
				Context: context.Background(), Server: server.URL, Remote: true,
				WorkerSessionID: "worker-1", Action: action, OutputFormat: "json", Output: &output,
			})
			if err != nil {
				t.Fatalf("remote control error = %v", err)
			}
			if local.calls != 0 {
				t.Fatalf("remote control local calls = %d, want 0", local.calls)
			}
			var result factoryapi.WorkerSessionControlResponse
			if err := json.Unmarshal(output.Bytes(), &result); err != nil {
				t.Fatalf("decode remote result: %v; output=%q", err, output.String())
			}
			if result.Outcome != factoryapi.WorkerSessionControlResponseOutcomeNoop {
				t.Fatalf("remote result = %#v, want NOOP", result)
			}
		})
	}
}

func TestControlRemoteFailureDoesNotFallbackAndMapsStableError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, `{"message":"control conflicts with current state","code":"WORKER_SESSION_CONTROL_CONFLICT"}`)
	}))
	defer server.Close()

	local := &controlLocalFake{}
	var output bytes.Buffer
	err := NewControl(testHTTPProtocol(t), local)(ControlConfig{
		Context: context.Background(), Server: server.URL, Remote: true,
		WorkerSessionID: "worker-1", Action: workersessions.ControlActionResume,
		OutputFormat: "json", Output: &output,
	})
	if err == nil || !strings.Contains(err.Error(), "control conflicts") {
		t.Fatalf("remote control error = %v, want server conflict", err)
	}
	if local.calls != 0 {
		t.Fatalf("remote failure local calls = %d, want 0", local.calls)
	}
	var payload struct {
		Code string `json:"code"`
	}
	if decodeErr := json.Unmarshal(output.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("decode remote error: %v; output=%q", decodeErr, output.String())
	}
	if payload.Code != "WORKER_SESSION_CONTROL_CONFLICT" {
		t.Fatalf("remote error code = %q, want control conflict", payload.Code)
	}
}

// The inspection commands (list/show/read/stream) are always addressed to the
// factory server, so every Worker Session an operator can see belongs to that
// server. A control addressed by the same stable Worker Session ID must reach
// the same owner instead of reporting NOT_FOUND from an in-process root that
// never ran the session.
func TestControlLocalNotFoundReAddressesTheSameWorkerSessionIDToTheFactoryServer(t *testing.T) {
	for _, action := range []workersessions.ControlAction{
		workersessions.ControlActionCancel, workersessions.ControlActionTerminate,
		workersessions.ControlActionPause, workersessions.ControlActionResume,
	} {
		t.Run(strings.ToLower(string(action)), func(t *testing.T) {
			local := &controlLocalFake{errs: map[workersessions.ControlAction]error{
				action: workersessions.ErrSessionNotFound,
			}}
			var requestedPath string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestedPath = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(factoryapi.WorkerSessionControlResponse{
					WorkerSessionId: "worker-1",
					Action:          factoryapi.WorkerSessionControlResponseAction(action),
					Outcome:         factoryapi.WorkerSessionControlResponseOutcomeApplied,
					State:           factoryapi.WorkerSessionControlResponseStateCanceled,
					DispatchId:      "dispatch-1",
				})
			}))
			defer server.Close()

			var output bytes.Buffer
			err := NewControl(testHTTPProtocol(t), local)(ControlConfig{
				Context: context.Background(), Server: server.URL,
				WorkerSessionID: "worker-1", Action: action,
				OutputFormat: "json", Output: &output,
			})
			if err != nil {
				t.Fatalf("control after local not-found error = %v; output=%q", err, output.String())
			}
			if local.calls != 1 || local.request.ID != "worker-1" {
				t.Fatalf("local calls = %d request = %#v, want one local attempt for worker-1", local.calls, local.request)
			}
			wantPath := "/worker-sessions/worker-1/" + strings.ToLower(string(action))
			if requestedPath != wantPath {
				t.Fatalf("server path = %q, want %q", requestedPath, wantPath)
			}
			var result factoryapi.WorkerSessionControlResponse
			if decodeErr := json.Unmarshal(output.Bytes(), &result); decodeErr != nil {
				t.Fatalf("decode result: %v; output=%q", decodeErr, output.String())
			}
			if result.WorkerSessionId != "worker-1" ||
				result.Outcome != factoryapi.WorkerSessionControlResponseOutcomeApplied ||
				result.State != factoryapi.WorkerSessionControlResponseStateCanceled {
				t.Fatalf("result = %#v, want the server's applied CANCELED snapshot", result)
			}
		})
	}
}

// A control that the local root does not own and that has no factory server to
// re-address must still report the honest NOT_FOUND rather than inventing a
// success.
func TestControlLocalNotFoundWithoutAFactoryServerStillReportsNotFound(t *testing.T) {
	local := &controlLocalFake{errs: map[workersessions.ControlAction]error{
		workersessions.ControlActionCancel: workersessions.ErrSessionNotFound,
	}}
	var output bytes.Buffer
	err := NewControl(nil, local)(ControlConfig{
		Context: context.Background(), WorkerSessionID: "worker-1",
		Action: workersessions.ControlActionCancel, OutputFormat: "json", Output: &output,
	})
	assertCLIErrorCode(t, err, "NOT_FOUND")
}

// A local control failure that is not a missing session is the local root's
// authoritative answer and must never be retried against the factory server.
func TestControlLocalConflictIsNotReAddressedToTheFactoryServer(t *testing.T) {
	local := &controlLocalFake{errs: map[workersessions.ControlAction]error{
		workersessions.ControlActionResume: workersessions.ErrProviderSessionAssociationMissing,
	}}
	serverCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		serverCalls++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	var output bytes.Buffer
	err := NewControl(testHTTPProtocol(t), local)(ControlConfig{
		Context: context.Background(), Server: server.URL, WorkerSessionID: "worker-1",
		Action: workersessions.ControlActionResume, OutputFormat: "json", Output: &output,
	})
	assertCLIErrorCode(t, err, "WORKER_SESSION_CONTROL_CONFLICT")
	if serverCalls != 0 {
		t.Fatalf("server calls = %d, want 0", serverCalls)
	}
}

type controlLocalFake struct {
	results map[workersessions.ControlAction]workersessions.ControlResult
	errs    map[workersessions.ControlAction]error
	action  workersessions.ControlAction
	request workersessions.ControlRequest
	calls   int
}

func (f *controlLocalFake) control(action workersessions.ControlAction, request workersessions.ControlRequest) (workersessions.ControlResult, error) {
	f.calls++
	f.action, f.request = action, request
	if err := f.errs[action]; err != nil {
		return workersessions.ControlResult{}, err
	}
	return f.results[action], nil
}

func (f *controlLocalFake) Pause(_ context.Context, request workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return f.control(workersessions.ControlActionPause, request)
}

func (f *controlLocalFake) Resume(_ context.Context, request workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return f.control(workersessions.ControlActionResume, request)
}

func (f *controlLocalFake) Cancel(_ context.Context, request workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return f.control(workersessions.ControlActionCancel, request)
}

func (f *controlLocalFake) Terminate(_ context.Context, request workersessions.ControlRequest) (workersessions.ControlResult, error) {
	return f.control(workersessions.ControlActionTerminate, request)
}

var _ LocalControlBoundary = (*controlLocalFake)(nil)
var _ clihttp.Protocol = (*invokeProtocolStub)(nil)

func TestMapInvokeServiceErrorPreservesRemoteClassificationInSyncAndAsyncModes(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code string
	}{
		{name: "invalid", err: workersessions.ErrInvalidExecutionRequest, code: "WORKER_SESSION_INVOKE_INVALID"},
		{name: "request id conflict", err: workersessions.ErrStartRequestIDConflict, code: "WORKER_SESSION_START_REQUEST_ID_CONFLICT"},
		{name: "identity conflict", err: workersessions.ErrSessionNotStartable, code: "WORKER_SESSION_NOT_STARTABLE"},
		{name: "admission", err: workersessions.ErrStartAdmissionFailed, code: "WORKER_SESSION_ADMISSION_FAILED"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, async := range []bool{false, true} {
				t.Run(map[bool]string{false: "sync", true: "async"}[async], func(t *testing.T) {
					got := mapInvokeServiceError(test.err, async)
					assertCLIErrorCode(t, got, test.code)
				})
			}
		})
	}
}

func TestMapContinueServiceErrorPreservesRemoteClassificationInSyncAndAsyncModes(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code string
	}{
		{name: "unknown source", err: workersessions.ErrContinuationSourceNotFound, code: "NOT_FOUND"},
		{name: "active source", err: workersessions.ErrContinuationSourceActive, code: "WORKER_SESSION_CONTINUATION_CONFLICT"},
		{name: "request id conflict", err: workersessions.ErrContinuationRequestIDConflict, code: "WORKER_SESSION_CONTINUATION_REQUEST_ID_CONFLICT"},
		{name: "successor conflict", err: workersessions.ErrContinuationSuccessorConflict, code: "WORKER_SESSION_CONTINUATION_CONFLICT"},
		{name: "admission", err: workersessions.ErrContinuationNotAccepted, code: "WORKER_SESSION_CONTINUATION_ADMISSION_FAILED"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, async := range []bool{false, true} {
				t.Run(map[bool]string{false: "sync", true: "async"}[async], func(t *testing.T) {
					got := mapContinueServiceError(test.err, async)
					assertCLIErrorCode(t, got, test.code)
				})
			}
		})
	}
}

func assertCLIErrorCode(t *testing.T, err error, want string) {
	t.Helper()
	var cliErr *CLIError
	if !errors.As(err, &cliErr) {
		t.Fatalf("error = %v, want CLIError(%s)", err, want)
	}
	if cliErr.Code != want {
		t.Fatalf("CLI error code = %q, want %q", cliErr.Code, want)
	}
}
