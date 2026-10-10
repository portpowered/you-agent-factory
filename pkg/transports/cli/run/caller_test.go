package run

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestFactoryCallerRemoteAdmission(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"start", "invoke"} {
		for _, outcome := range []string{"success", "absent", "malformed", "refused", "transport", "api-error"} {
			t.Run(mode+"/"+outcome, func(t *testing.T) { t.Parallel(); testFactoryCallerRemote(t, mode, outcome) })
		}
	}
}

func testFactoryCallerRemote(t *testing.T, mode, outcome string) {
	t.Helper()
	token := strings.Repeat("A", 43)
	caller := &workersessions.CallerIdentity{WorkerSessionID: "exact/caller", Token: token}
	if outcome == "absent" {
		caller = nil
	}
	if outcome == "malformed" {
		caller.Token = "malformed"
	}
	var diagnostics bytes.Buffer
	protocol := &factoryCallerProtocol{t: t, mode: mode, outcome: outcome, token: token}
	client := remoteInvocationClient{transport: protocol}
	var err error
	var wrapper any
	if mode == "start" {
		request := RemoteInvocationRequest{Server: "http://selected.test", Request: factoryapi.FactorySessionExecutionRequest{RequestId: "request"}, Caller: caller, Diagnostics: &diagnostics, Verbose: true}
		wrapper = request
		_, err = client.StartFactorySession(t.Context(), request)
	} else {
		request := RemoteExistingSessionInvocationRequest{Server: "http://selected.test", SessionID: "selected", Caller: caller, Diagnostics: &diagnostics, Verbose: true}
		wrapper = request
		_, err = client.InvokeFactorySession(t.Context(), request)
	}
	data, marshalErr := json.Marshal(wrapper)
	if marshalErr != nil || strings.Contains(string(data)+diagnostics.String(), token) {
		t.Fatal("caller leaked into serialized request or diagnostics")
	}
	assertFactoryCallerRemoteResult(t, outcome, err, protocol.calls, token)
}

func assertFactoryCallerRemoteResult(t *testing.T, outcome string, err error, calls int, token string) {
	t.Helper()
	if outcome == "malformed" || outcome == "refused" {
		var typed *InvocationError
		if !errors.Is(err, workersessions.ErrCallerInvalid) || !errors.As(err, &typed) || typed.Code != "WORKER_SESSION_CALLER_INVALID" {
			t.Fatal("caller refusal lost typed identity")
		}
	} else if outcome == "success" || outcome == "absent" {
		if err != nil {
			t.Fatal(err)
		}
	} else if err == nil {
		t.Fatal("transport/API failure reported success")
	}
	if outcome == "malformed" {
		if calls != 0 {
			t.Fatal("malformed caller reached transport")
		}
	} else if calls != 1 {
		t.Fatal("admission did not use selected transport once")
	}
	assertFactoryCallerErrorPrivacy(t, err, token)
}

func assertFactoryCallerErrorPrivacy(t *testing.T, err error, token string) {
	t.Helper()
	if err != nil {
		if strings.Contains(err.Error(), token) {
			t.Fatal("error disclosed token")
		}
		var typed *InvocationError
		if errors.As(err, &typed) && typed.Cause != nil && strings.Contains(typed.Cause.Error(), token) {
			t.Fatal("error cause disclosed token")
		}
	}
}

type factoryCallerProtocol struct {
	remoteProtocolStub
	t                    *testing.T
	mode, outcome, token string
	calls                int
}

func (protocol *factoryCallerProtocol) Execute(request *http.Request) (clihttp.Response, error) {
	protocol.calls++
	if request.URL.Host != "selected.test" {
		protocol.t.Fatal("caller changed selected host")
	}
	if protocol.outcome == "absent" {
		if request.Header.Get("Authorization") != "" || request.Header.Get("X-You-Worker-Session-Id") != "" {
			protocol.t.Fatal("absent caller invented headers")
		}
	} else if request.Header.Get("Authorization") != "Bearer "+protocol.token || request.Header.Get("X-You-Worker-Session-Id") != "exact/caller" {
		protocol.t.Fatal("exact caller headers changed")
	}
	data, err := io.ReadAll(request.Body)
	if err != nil || strings.Contains(string(data), protocol.token) {
		protocol.t.Fatal("caller entered generated body")
	}
	if protocol.outcome == "transport" {
		return clihttp.Response{}, errors.New("credential: " + protocol.token)
	}
	status, body := http.StatusOK, `{"requestId":"request","status":"COMPLETED","sessionId":"selected"}`
	if protocol.mode == "start" {
		status, body = http.StatusAccepted, `{"status":"ACCEPTED","sessionId":"selected"}`
	}
	if protocol.outcome == "refused" {
		status, body = http.StatusForbidden, `{"code":"WORKER_SESSION_CALLER_INVALID","message":"credential: `+protocol.token+`"}`
	}
	if protocol.outcome == "api-error" {
		status, body = http.StatusInternalServerError, `{"code":"INTERNAL_ERROR","message":"credential: `+protocol.token+`"}`
	}
	return clihttp.Response{HTTP: &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}}, nil
}
