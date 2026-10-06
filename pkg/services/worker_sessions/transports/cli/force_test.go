package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func forceCLIConfig(output io.Writer) ControlConfig {
	return ControlConfig{Context: context.Background(), WorkerSessionID: "worker-1",
		Action: workersessions.ControlActionTerminate, Force: true, RequestID: "kill-1",
		ExpectedAttemptID: "attempt-1", OutputFormat: "json", Output: output}
}

func TestForceCLILocalExactTupleAndResult(t *testing.T) {
	t.Parallel()
	local := &controlLocalFake{results: map[workersessions.ControlAction]workersessions.ControlResult{
		workersessions.ControlActionTerminate: {Session: workersessions.Session{ID: "worker-1", State: workersessions.StateTerminated},
			Action: workersessions.ControlActionTerminate, Outcome: workersessions.ControlOutcomeApplied, Forced: true},
	}}
	var output bytes.Buffer
	if err := NewControl(nil, local)(forceCLIConfig(&output)); err != nil {
		t.Fatal(err)
	}
	if local.calls != 1 || local.request.ID != "worker-1" || !local.request.Force ||
		local.request.RequestID != "kill-1" || local.request.ExpectedAttemptID != "attempt-1" {
		t.Fatalf("request = %#v, calls = %d", local.request, local.calls)
	}
	var result factoryapi.WorkerSessionControlResponse
	if err := json.Unmarshal(output.Bytes(), &result); err != nil || result.Forced == nil || !*result.Forced {
		t.Fatalf("force result = %s, error = %v", output.Bytes(), err)
	}
}

func TestForceCLIInvalidTupleHasNoEffects(t *testing.T) {
	t.Parallel()
	for _, change := range []func(*ControlConfig){
		func(c *ControlConfig) { c.RequestID = "" },
		func(c *ControlConfig) { c.ExpectedAttemptID = " " },
		func(c *ControlConfig) { c.Force = false },
		func(c *ControlConfig) { c.Action = workersessions.ControlActionCancel },
	} {
		local := &controlLocalFake{}
		config := forceCLIConfig(io.Discard)
		change(&config)
		err := NewControl(nil, local)(config)
		assertCLIErrorCode(t, err, "WORKER_SESSION_CONTROL_INVALID")
		if local.calls != 0 {
			t.Fatalf("invalid tuple dispatched %d controls", local.calls)
		}
	}
}

func TestForceCLIRemoteExactBodyAndFailClosedResult(t *testing.T) {
	t.Parallel()
	for _, forced := range []string{`,"forced":true`, ``, `,"forced":false`} {
		t.Run(forced, func(t *testing.T) {
			t.Parallel()
			calls := 0
			protocol := &forceProtocolStub{execute: func(request *http.Request) (clihttp.Response, error) {
				calls++
				assertForceCLIHTTPRequest(t, request)
				return clihttp.Response{HTTP: &http.Response{StatusCode: http.StatusOK,
					Body: io.NopCloser(bytes.NewBufferString(`{"workerSessionId":"worker-1","action":"TERMINATE","outcome":"UNSUPPORTED","state":"RUNNING"` + forced + `}`))}}, nil
			}}
			local := &controlLocalFake{}
			config := forceCLIConfig(io.Discard)
			config.Remote, config.Server = true, "http://factory.test"
			err := NewControl(protocol, local)(config)
			if forced == `,"forced":true` {
				if err != nil {
					t.Fatal(err)
				}
			} else {
				assertCLIErrorCode(t, err, "WORKER_SESSION_CONTROL_FAILED")
			}
			if calls != 1 || local.calls != 0 {
				t.Fatalf("remote/local calls = %d/%d", calls, local.calls)
			}
		})
	}
}

type forceProtocolStub struct {
	invokeProtocolStub
	execute func(*http.Request) (clihttp.Response, error)
}

func (stub *forceProtocolStub) Execute(request *http.Request) (clihttp.Response, error) {
	return stub.execute(request)
}

func assertForceCLIHTTPRequest(t *testing.T, request *http.Request) {
	t.Helper()
	if request.Method != http.MethodPost || request.URL.Path != "/worker-sessions/worker-1/terminate" || request.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("request = %s %s, headers = %v", request.Method, request.URL, request.Header)
	}
	var body factoryapi.TerminateWorkerSessionJSONRequestBody
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Force == nil || !*body.Force || body.RequestId == nil || *body.RequestId != "kill-1" ||
		body.ExpectedAttemptId == nil || *body.ExpectedAttemptId != "attempt-1" {
		t.Fatalf("force body = %#v", body)
	}
}

func TestForceCLIUnconfirmedDeadlineIsFailure(t *testing.T) {
	t.Parallel()
	err := mapControlServiceError(errors.Join(workersessions.ErrForceTerminationUnconfirmed, context.DeadlineExceeded))
	assertCLIErrorCode(t, err, "WORKER_SESSION_CONTROL_FAILED")
}
