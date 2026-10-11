package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
)

func TestInvokeCallerEnvironmentValidation(t *testing.T) {
	t.Parallel()
	token := strings.Repeat("A", 43)
	for _, test := range []struct {
		name    string
		env     map[string]string
		invalid bool
	}{
		{name: "absent"},
		{name: "exact pair", env: map[string]string{"YOU_WORKER_SESSION_ID": "exact/caller", "YOU_WORKER_SESSION_TOKEN": token}},
		{name: "id only", env: map[string]string{"YOU_WORKER_SESSION_ID": "exact/caller"}, invalid: true},
		{name: "token only", env: map[string]string{"YOU_WORKER_SESSION_TOKEN": token}, invalid: true},
		{name: "empty pair", env: map[string]string{"YOU_WORKER_SESSION_ID": "", "YOU_WORKER_SESSION_TOKEN": ""}, invalid: true},
		{name: "malformed token", env: map[string]string{"YOU_WORKER_SESSION_ID": "exact/caller", "YOU_WORKER_SESSION_TOKEN": "bad"}, invalid: true},
		{name: "inexact id", env: map[string]string{"YOU_WORKER_SESSION_ID": " exact/caller", "YOU_WORKER_SESSION_TOKEN": token}, invalid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			caller, err := invokeCaller(InvokeConfig{LookupEnv: func(key string) (string, bool) {
				value, present := test.env[key]
				return value, present
			}})
			if test.invalid {
				if !errors.Is(err, workersessions.ErrCallerInvalid) || caller != nil {
					t.Fatal("malformed caller was not refused")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if test.env == nil {
				if caller != nil {
					t.Fatal("absent credentials invented a caller")
				}
			} else if caller == nil || caller.WorkerSessionID != "exact/caller" || caller.Token != token {
				t.Fatal("caller pair changed during decoding")
			}
		})
	}
}

func TestInvokeCallerLocalAdmissionAndDetachment(t *testing.T) {
	t.Parallel()
	caller := &workersessions.CallerIdentity{WorkerSessionID: "exact/caller", Token: strings.Repeat("A", 43)}
	boundary := &invokeLocalFake{startResult: workersessions.StartResult{Session: workersessions.Session{ID: "child", State: workersessions.StateRunning}}}
	config := callerInvokeConfig()
	config.Local, config.Caller = boundary, caller
	if err := invoke(config); err != nil {
		t.Fatal(err)
	}
	admitted := boundary.startRequests[0].Caller
	if admitted == caller || admitted == nil || *admitted != *caller {
		t.Fatal("admission did not receive a detached exact caller")
	}
	caller.Token = "changed"
	if admitted.Token == caller.Token {
		t.Fatal("caller mutation changed admitted credentials")
	}
	encoded, err := json.Marshal(boundary.startRequests[0])
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(admitted.Token)) {
		t.Fatal("execution request serialized the caller token")
	}
}

func TestInvokeMalformedCallerDoesNotReachAdmission(t *testing.T) {
	t.Parallel()
	boundary := &invokeLocalFake{}
	config := callerInvokeConfig()
	config.Local = boundary
	config.LookupEnv = func(key string) (string, bool) { return "exact/caller", key == "YOU_WORKER_SESSION_ID" }
	err := invoke(config)
	if !errors.Is(err, workersessions.ErrCallerInvalid) || len(boundary.startRequests) != 0 {
		t.Fatal("partial credentials reached admission or lost typed refusal")
	}
	boundary.startErr = workersessions.ErrCallerInvalid
	config.LookupEnv = nil
	config.Caller = &workersessions.CallerIdentity{WorkerSessionID: "exact/caller", Token: strings.Repeat("A", 43)}
	err = invoke(config)
	var typed *CLIError
	if !errors.As(err, &typed) || typed.Code != "WORKER_SESSION_CALLER_INVALID" {
		t.Fatal("owner caller refusal lost its code")
	}
}

func TestInvokeCallerRemoteHeadersAndPrivacy(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"success", "transport failure", "owner refusal"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			token := strings.Repeat("A", 43)
			protocol := &callerInvokeProtocol{token: token, mode: mode, t: t}
			var output, diagnostics bytes.Buffer
			config := callerInvokeConfig()
			config.Remote, config.Server, config.HTTP = true, "http://selected.test:7437", protocol
			config.Output, config.Diagnostics, config.Debug = &output, &diagnostics, true
			config.LookupEnv = func(key string) (string, bool) {
				switch key {
				case "YOU_WORKER_SESSION_ID":
					return "exact/caller", true
				case "YOU_WORKER_SESSION_TOKEN":
					return token, true
				default:
					return "", false
				}
			}
			err := invoke(config)
			if (mode == "success") != (err == nil) {
				t.Fatal("unexpected admission outcome")
			}
			if strings.Contains(output.String()+diagnostics.String(), token) {
				t.Fatal("CLI output disclosed caller credential")
			}
			if err != nil {
				for cause := err; cause != nil; cause = errors.Unwrap(cause) {
					if strings.Contains(cause.Error(), token) {
						t.Fatal("returned diagnostic cause disclosed caller credential")
					}
				}
			}
		})
	}
}

func callerInvokeConfig() InvokeConfig {
	return InvokeConfig{Context: context.Background(), Output: io.Discard, OutputFormat: "json", Async: true,
		RequestID: "request", WorkerSessionID: "child", DispatchID: "dispatch", WorkstationName: "coding", UserMessage: "work"}
}

type callerInvokeProtocol struct {
	invokeProtocolStub
	token, mode string
	t           *testing.T
}

func (protocol *callerInvokeProtocol) Execute(request *http.Request) (clihttp.Response, error) {
	if request.URL.Host != "selected.test:7437" || request.URL.Path != "/worker-sessions" {
		protocol.t.Fatal("caller admission did not use the selected server")
	}
	if request.Header.Get("X-You-Worker-Session-Id") != "exact/caller" || request.Header.Get("Authorization") != "Bearer "+protocol.token {
		protocol.t.Fatal("caller admission did not forward exact headers")
	}
	body, err := io.ReadAll(request.Body)
	if err != nil {
		protocol.t.Fatal(err)
	}
	if bytes.Contains(body, []byte(protocol.token)) {
		protocol.t.Fatal("HTTP body disclosed caller credential")
	}
	if protocol.mode == "transport failure" {
		return clihttp.Response{}, errors.New("transport echoed " + protocol.token)
	}
	status, response := http.StatusAccepted, `{"workerSessionId":"child","accepted":true,"state":"RUNNING"}`
	if protocol.mode == "owner refusal" {
		status = http.StatusForbidden
		response = `{"code":"WORKER_SESSION_CALLER_INVALID","message":"refused ` + protocol.token + `"}`
	}
	return clihttp.Response{HTTP: &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(response))}}, nil
}
