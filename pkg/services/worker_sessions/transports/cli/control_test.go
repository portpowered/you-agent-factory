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

type controlLocalFake struct {
	results map[workersessions.ControlAction]workersessions.ControlResult
	action  workersessions.ControlAction
	request workersessions.ControlRequest
	calls   int
}

func (f *controlLocalFake) control(action workersessions.ControlAction, request workersessions.ControlRequest) (workersessions.ControlResult, error) {
	f.calls++
	f.action, f.request = action, request
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
