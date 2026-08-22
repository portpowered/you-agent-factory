package session

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestLifecycleControlPause_CLIJSONMatchesAPIResponse(t *testing.T) {
	service := acceptedPauseEquivalenceScript()
	serverURL := serverURLForLifecycleEquivalence(t, service)

	apiResponse, status := postLifecycleControl(
		t,
		serverURL,
		"dur-sess-api-pause-001",
		"pause",
	)
	if status != 200 {
		t.Fatalf("API pause status = %d, want 200", status)
	}

	var cliOut bytes.Buffer
	if err := NewPause(testHTTPProtocol(t))(LifecycleControlConfig{Context: context.Background(),
		Server:    serverURL,
		SessionID: "dur-sess-cli-pause-001",
		JSON:      true,
		Output:    &cliOut,
	}); err != nil {
		t.Fatalf("CLI pause: %v", err)
	}

	var cliResponse factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(cliOut.Bytes(), &cliResponse); err != nil {
		t.Fatalf("decode CLI JSON: %v\n%s", err, cliOut.String())
	}
	assertLifecycleControlEquivalence(t, apiResponse, cliResponse)
}

func TestLocalLifecycleControl_JSONPreservesCanonicalResponse(t *testing.T) {
	result := lifecycleEquivalenceResult(
		"dur-sess-local-pause-001",
		factorysessionexecution.LifecycleControlPause,
		factorysessionexecution.LifecycleControlOutcomeAccepted,
		factorysessionexecution.LifecycleStatusPaused,
	)
	result.EffectivePolicyHash = "sha256:policy"
	result.ApprovalPreviewID = "preview-1"
	result.DispatchID = "dispatch-1"
	result.RetryDispatchID = "dispatch-1"
	result.Detail = "pause accepted"
	result.Links = factorysessionexecution.LifecycleControlLinks{
		Session:    "/factory-sessions/dur-sess-local-pause-001",
		Status:     "/factory-sessions/dur-sess-local-pause-001",
		Results:    "/factory-sessions/dur-sess-local-pause-001/results",
		Dispatches: "/factory-sessions/dur-sess-local-pause-001/dispatches",
		Artifacts:  "/factory-sessions/dur-sess-local-pause-001/artifacts",
		Events:     "/factory-sessions/dur-sess-local-pause-001/events",
	}

	var out bytes.Buffer
	operation := func(_ context.Context, sessionID string, request factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
		if sessionID != result.SessionID {
			t.Fatalf("sessionId = %q, want %q", sessionID, result.SessionID)
		}
		if request.RequestID != "control-1" || request.Reason != "operator pause" {
			t.Fatalf("control request = %#v, want request metadata", request)
		}
		return result, nil
	}
	if err := NewLocalPause(operation)(LifecycleControlConfig{
		Context: context.Background(), SessionID: result.SessionID, RequestID: "control-1",
		Reason: "operator pause", JSON: true, Output: &out,
	}); err != nil {
		t.Fatalf("local pause: %v", err)
	}

	var got factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode local response: %v\n%s", err, out.String())
	}
	want := localLifecycleControlResponse(result)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("local response = %#v, want canonical mapping %#v", got, want)
	}
}

func TestLocalLifecycleControl_RejectionRendersRemoteEquivalentOutcome(t *testing.T) {
	const sessionID = "dur-sess-local-terminal-001"
	var out bytes.Buffer
	err := NewLocalPause(func(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
		return factorysessionexecution.LifecycleControlResult{}, &factorysessionexecution.ControlError{
			Operation: factorysessionexecution.LifecycleControlPause,
			Outcome:   factorysessionexecution.LifecycleControlOutcomeTerminalSession,
			Status:    factorysessionexecution.LifecycleStatusSucceeded,
			Message:   "terminal session",
			Links: factorysessionexecution.LifecycleControlLinks{
				Session: "/factory-sessions/" + sessionID,
				Status:  "/factory-sessions/" + sessionID,
			},
		}
	})(LifecycleControlConfig{
		Context: context.Background(), SessionID: sessionID, JSON: true, Output: &out,
	})

	var rejected *LifecycleControlRejectedError
	if !errors.As(err, &rejected) {
		t.Fatalf("local error = %v, want LifecycleControlRejectedError", err)
	}
	if rejected.Response.SessionId != sessionID ||
		rejected.Response.Operation != factoryapi.FactorySessionLifecycleControlKindPause ||
		rejected.Response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeTerminalSession ||
		rejected.Response.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("rejected response = %#v, want terminal pause for %s", rejected.Response, sessionID)
	}
	if rejected.Response.Detail == nil || *rejected.Response.Detail != "terminal session" {
		t.Fatalf("rejected detail = %#v, want terminal session", rejected.Response.Detail)
	}

	var output factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(out.Bytes(), &output); err != nil {
		t.Fatalf("decode local rejection: %v\n%s", err, out.String())
	}
	if !reflect.DeepEqual(output, rejected.Response) {
		t.Fatalf("rendered rejection = %#v, want returned rejection %#v", output, rejected.Response)
	}
}

func TestLocalLifecycleControl_NotFoundUsesStableDiagnosticWithoutMutation(t *testing.T) {
	calls := 0
	var out bytes.Buffer
	err := NewLocalCancel(func(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
		calls++
		return factorysessionexecution.LifecycleControlResult{}, factorysessionexecution.ErrDurableSessionNotFound
	})(LifecycleControlConfig{
		Context: context.Background(), SessionID: "dur-sess-missing-001", Output: &out,
	})
	if err == nil || !strings.Contains(err.Error(), `factory session "dur-sess-missing-001" not found`) {
		t.Fatalf("local not-found error = %v, want stable session diagnostic", err)
	}
	if calls != 1 {
		t.Fatalf("local operation calls = %d, want 1", calls)
	}
	if out.Len() != 0 {
		t.Fatalf("not-found stdout = %q, want empty", out.String())
	}
}

func TestLocalLifecycleControls_RenderAllOperationsForExactSession(t *testing.T) {
	tests := []struct {
		name      string
		operation factorysessionexecution.LifecycleControlKind
		status    factorysessionexecution.LifecycleStatus
		invoke    func(localLifecycleOperation) func(LifecycleControlConfig) error
	}{
		{
			name: "pause", operation: factorysessionexecution.LifecycleControlPause,
			status: factorysessionexecution.LifecycleStatusPaused, invoke: NewLocalPause,
		},
		{
			name: "resume", operation: factorysessionexecution.LifecycleControlResume,
			status: factorysessionexecution.LifecycleStatusRunning, invoke: NewLocalResume,
		},
		{
			name: "cancel", operation: factorysessionexecution.LifecycleControlCancel,
			status: factorysessionexecution.LifecycleStatusCanceling, invoke: NewLocalCancel,
		},
		{
			name: "terminate", operation: factorysessionexecution.LifecycleControlTerminate,
			status: factorysessionexecution.LifecycleStatusTerminated, invoke: NewLocalTerminate,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			const sessionID = "dur-sess-local-exact-001"
			var out bytes.Buffer
			operation := func(_ context.Context, gotSessionID string, _ factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
				if gotSessionID != sessionID {
					t.Fatalf("sessionId = %q, want %q", gotSessionID, sessionID)
				}
				return lifecycleEquivalenceResult(sessionID, test.operation, factorysessionexecution.LifecycleControlOutcomeAccepted, test.status), nil
			}
			if err := test.invoke(operation)(LifecycleControlConfig{
				Context: context.Background(), SessionID: sessionID, JSON: true, Output: &out,
			}); err != nil {
				t.Fatalf("local %s: %v", test.name, err)
			}

			var response factoryapi.FactorySessionLifecycleControlResponse
			if err := json.Unmarshal(out.Bytes(), &response); err != nil {
				t.Fatalf("decode local %s response: %v\n%s", test.name, err, out.String())
			}
			if response.SessionId != sessionID ||
				response.Operation != factoryapi.FactorySessionLifecycleControlKind(test.operation) ||
				response.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted ||
				response.Status != factoryapi.FactorySessionDurableLifecycleStatus(test.status) {
				t.Fatalf("local %s response = %#v, want exact accepted outcome", test.name, response)
			}
		})
	}
}

func TestBindServiceDelegatesToInjectedOperations(t *testing.T) {
	t.Parallel()

	calls := 0
	service := Bind(Operations{
		List: func(ListConfig) error {
			calls++
			return nil
		},
		Show:      func(ShowConfig) error { calls++; return nil },
		Pause:     func(LifecycleControlConfig) error { calls++; return nil },
		Resume:    func(LifecycleControlConfig) error { calls++; return nil },
		Cancel:    func(LifecycleControlConfig) error { calls++; return nil },
		Terminate: func(LifecycleControlConfig) error { calls++; return nil },
		Create:    func(CreateConfig) error { calls++; return nil },
		Delete:    func(DeleteConfig) error { calls++; return nil },
	})
	if err := service.List(ListConfig{Context: context.Background()}); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	for name, call := range map[string]func() error{
		"show":      func() error { return service.Show(ShowConfig{}) },
		"pause":     func() error { return service.Pause(LifecycleControlConfig{}) },
		"resume":    func() error { return service.Resume(LifecycleControlConfig{}) },
		"cancel":    func() error { return service.Cancel(LifecycleControlConfig{}) },
		"terminate": func() error { return service.Terminate(LifecycleControlConfig{}) },
		"create":    func() error { return service.Create(CreateConfig{}) },
		"delete":    func() error { return service.Delete(DeleteConfig{}) },
	} {
		if err := call(); err != nil {
			t.Fatalf("%s error = %v", name, err)
		}
	}
	if calls != 8 {
		t.Fatalf("calls = %d, want 8", calls)
	}
}

func TestBindServiceRequiresInjectedOperations(t *testing.T) {
	t.Parallel()

	service := Bind(Operations{})
	for name, call := range map[string]func() error{
		"list":      func() error { return service.List(ListConfig{}) },
		"show":      func() error { return service.Show(ShowConfig{}) },
		"pause":     func() error { return service.Pause(LifecycleControlConfig{}) },
		"resume":    func() error { return service.Resume(LifecycleControlConfig{}) },
		"cancel":    func() error { return service.Cancel(LifecycleControlConfig{}) },
		"terminate": func() error { return service.Terminate(LifecycleControlConfig{}) },
		"create":    func() error { return service.Create(CreateConfig{}) },
		"delete":    func() error { return service.Delete(DeleteConfig{}) },
	} {
		if err := call(); err == nil {
			t.Fatalf("%s error = nil, want required-edge failure", name)
		}
	}
}

func TestLifecycleControlPause_CLIJSONMatchesAPINoOpResponse(t *testing.T) {
	const sessionID = "dur-sess-pause-noop-001"
	service := lifecycleEquivalenceScript{
		pause: func(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
			return lifecycleEquivalenceResult(sessionID, "PAUSE", factorysessionexecution.LifecycleControlOutcomeNoOp, factorysessionexecution.LifecycleStatusPaused), nil
		},
	}
	serverURL := serverURLForLifecycleEquivalence(t, service)

	apiResponse, status := postLifecycleControl(t, serverURL, sessionID, "pause")
	if status != 200 {
		t.Fatalf("API no-op pause status = %d, want 200", status)
	}
	if apiResponse.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeNoOp {
		t.Fatalf("API outcome = %q, want NO_OP", apiResponse.Outcome)
	}

	var cliOut bytes.Buffer
	if err := NewPause(testHTTPProtocol(t))(LifecycleControlConfig{Context: context.Background(),
		Server:    serverURL,
		SessionID: sessionID,
		JSON:      true,
		Output:    &cliOut,
	}); err != nil {
		t.Fatalf("CLI pause no-op: %v", err)
	}

	var cliResponse factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(cliOut.Bytes(), &cliResponse); err != nil {
		t.Fatalf("decode CLI JSON: %v\n%s", err, cliOut.String())
	}
	assertLifecycleControlEquivalence(t, apiResponse, cliResponse)
}

func TestLifecycleControlResume_CLIJSONMatchesAPIResponse(t *testing.T) {
	service := lifecycleEquivalenceScript{
		pause: acceptedPauseEquivalenceScript().pause,
		resume: func(_ context.Context, sessionID string, _ factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
			return lifecycleEquivalenceResult(sessionID, "RESUME", factorysessionexecution.LifecycleControlOutcomeAccepted, factorysessionexecution.LifecycleStatusRunning), nil
		},
	}
	serverURL := serverURLForLifecycleEquivalence(t, service)
	sessionID := "dur-sess-api-resume-001"

	if _, status := postLifecycleControl(t, serverURL, sessionID, "pause"); status != 200 {
		t.Fatalf("API pause status = %d, want 200", status)
	}

	apiResponse, status := postLifecycleControl(t, serverURL, sessionID, "resume")
	if status != 200 {
		t.Fatalf("API resume status = %d, want 200", status)
	}

	cliSessionID := "dur-sess-cli-resume-001"
	if _, status := postLifecycleControl(t, serverURL, cliSessionID, "pause"); status != 200 {
		t.Fatalf("API pause cli session status = %d, want 200", status)
	}

	var cliOut bytes.Buffer
	if err := NewResume(testHTTPProtocol(t))(LifecycleControlConfig{Context: context.Background(),
		Server:    serverURL,
		SessionID: cliSessionID,
		JSON:      true,
		Output:    &cliOut,
	}); err != nil {
		t.Fatalf("CLI resume: %v", err)
	}

	var cliResponse factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(cliOut.Bytes(), &cliResponse); err != nil {
		t.Fatalf("decode CLI JSON: %v\n%s", err, cliOut.String())
	}
	assertLifecycleControlEquivalence(t, apiResponse, cliResponse)
}

func TestLifecycleControlPause_CLIJSONMatchesAPITerminalSessionRejection(t *testing.T) {
	service := lifecycleEquivalenceScript{
		pause: func(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
			return factorysessionexecution.LifecycleControlResult{}, &factorysessionexecution.ControlError{
				Operation: "PAUSE",
				Outcome:   factorysessionexecution.LifecycleControlOutcomeTerminalSession,
				Status:    factorysessionexecution.LifecycleStatusSucceeded,
				Message:   "terminal session",
			}
		},
	}
	serverURL := serverURLForLifecycleEquivalence(t, service)
	sessionID := "dur-sess-petri-success-001"

	apiResponse, status := postLifecycleControl(t, serverURL, sessionID, "pause")
	if status != 409 {
		t.Fatalf("API pause status = %d, want 409", status)
	}
	if apiResponse.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeTerminalSession {
		t.Fatalf("API outcome = %q, want TERMINAL_SESSION", apiResponse.Outcome)
	}

	var cliOut bytes.Buffer
	err := NewPause(testHTTPProtocol(t))(LifecycleControlConfig{Context: context.Background(),
		Server:    serverURL,
		SessionID: sessionID,
		JSON:      true,
		Output:    &cliOut,
	})
	var rejected *LifecycleControlRejectedError
	if !errors.As(err, &rejected) {
		t.Fatalf("CLI error = %v, want LifecycleControlRejectedError", err)
	}

	var cliResponse factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(cliOut.Bytes(), &cliResponse); err != nil {
		t.Fatalf("decode CLI JSON: %v\n%s", err, cliOut.String())
	}
	assertLifecycleControlEquivalence(t, apiResponse, cliResponse)
}

func TestLifecycleControlPause_DefaultLiveSessionCLIJSONMatchesAPIResponse(t *testing.T) {
	apiServerURL := liveLifecycleEquivalenceServerURL(
		t,
		scriptedLiveLifecycleMock("~default", factorysessionexecution.LifecycleControlPause),
	)

	apiResponse, status := postLifecycleControl(t, apiServerURL, "~default", "pause")
	if status != 200 {
		t.Fatalf("API pause status = %d, want 200", status)
	}

	cliServerURL := liveLifecycleEquivalenceServerURL(
		t,
		scriptedLiveLifecycleMock("~default", factorysessionexecution.LifecycleControlPause),
	)

	var cliOut bytes.Buffer
	if err := NewPause(testHTTPProtocol(t))(LifecycleControlConfig{Context: context.Background(),
		Server: cliServerURL,
		JSON:   true,
		Output: &cliOut,
	}); err != nil {
		t.Fatalf("CLI default pause: %v", err)
	}

	var cliResponse factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(cliOut.Bytes(), &cliResponse); err != nil {
		t.Fatalf("decode CLI JSON: %v\n%s", err, cliOut.String())
	}
	assertLifecycleControlEquivalence(t, apiResponse, cliResponse)
}

func TestLifecycleControlResume_NamedLiveSessionCLIJSONMatchesAPIResponse(t *testing.T) {
	apiServerURL := liveLifecycleEquivalenceServerURL(
		t,
		scriptedLiveLifecycleMock("session-beta", factorysessionexecution.LifecycleControlResume),
	)

	apiResponse, status := postLifecycleControl(t, apiServerURL, "session-beta", "resume")
	if status != 200 {
		t.Fatalf("API resume status = %d, want 200", status)
	}

	cliServerURL := liveLifecycleEquivalenceServerURL(
		t,
		scriptedLiveLifecycleMock("session-beta", factorysessionexecution.LifecycleControlResume),
	)

	var cliOut bytes.Buffer
	if err := NewResume(testHTTPProtocol(t))(LifecycleControlConfig{Context: context.Background(),
		Server:    cliServerURL,
		SessionID: "session-beta",
		JSON:      true,
		Output:    &cliOut,
	}); err != nil {
		t.Fatalf("CLI named live resume: %v", err)
	}

	var cliResponse factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(cliOut.Bytes(), &cliResponse); err != nil {
		t.Fatalf("decode CLI JSON: %v\n%s", err, cliOut.String())
	}
	assertLifecycleControlEquivalence(t, apiResponse, cliResponse)
}

type lifecycleEquivalenceScript struct {
	factorysessionwire.DurableExecutionService
	pause  func(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error)
	resume func(context.Context, string, factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error)
}

func scriptedLiveLifecycleMock(
	sessionID string,
	operation factorysessionexecution.LifecycleControlKind,
) lifecycleEquivalenceScript {
	result := factorysessionexecution.LifecycleControlResult{
		SessionID: sessionID,
		Operation: operation,
		Outcome:   factorysessionexecution.LifecycleControlOutcomeAccepted,
	}
	mock := lifecycleEquivalenceScript{}
	switch operation {
	case factorysessionexecution.LifecycleControlPause:
		result.Status = factorysessionexecution.LifecycleStatusPaused
		mock.pause = func(
			context.Context,
			string,
			factorysessionexecution.ControlRequest,
		) (factorysessionexecution.LifecycleControlResult, error) {
			return result, nil
		}
	case factorysessionexecution.LifecycleControlResume:
		result.Status = factorysessionexecution.LifecycleStatusRunning
		mock.resume = func(
			context.Context,
			string,
			factorysessionexecution.ControlRequest,
		) (factorysessionexecution.LifecycleControlResult, error) {
			return result, nil
		}
	}
	return mock
}

func (script lifecycleEquivalenceScript) Pause(ctx context.Context, sessionID string, request factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
	return script.pause(ctx, sessionID, request)
}

func (script lifecycleEquivalenceScript) Resume(ctx context.Context, sessionID string, request factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
	return script.resume(ctx, sessionID, request)
}

func acceptedPauseEquivalenceScript() lifecycleEquivalenceScript {
	return lifecycleEquivalenceScript{
		pause: func(_ context.Context, sessionID string, _ factorysessionexecution.ControlRequest) (factorysessionexecution.LifecycleControlResult, error) {
			return lifecycleEquivalenceResult(sessionID, "PAUSE", factorysessionexecution.LifecycleControlOutcomeAccepted, factorysessionexecution.LifecycleStatusPaused), nil
		},
	}
}

func lifecycleEquivalenceResult(
	sessionID string,
	operation factorysessionexecution.LifecycleControlKind,
	outcome factorysessionexecution.LifecycleControlOutcome,
	status factorysessionexecution.LifecycleStatus,
) factorysessionexecution.LifecycleControlResult {
	return factorysessionexecution.LifecycleControlResult{
		SessionID: sessionID,
		Operation: operation,
		Outcome:   outcome,
		Status:    status,
		Links: factorysessionexecution.LifecycleControlLinks{
			Session: "/factory-sessions/" + sessionID,
			Status:  "/factory-sessions/" + sessionID,
			Results: "/factory-sessions/" + sessionID + "/results",
		},
	}
}

func serverURLForLifecycleEquivalence(t *testing.T, service factorysessionwire.DurableExecutionService) string {
	t.Helper()
	server := httptest.NewServer(lifecycleEquivalenceHTTPHandler(service))
	t.Cleanup(server.Close)
	return server.URL
}

func liveLifecycleEquivalenceServerURL(t *testing.T, service factorysessionwire.DurableExecutionService) string {
	t.Helper()
	return serverURLForLifecycleEquivalence(t, service)
}

func lifecycleEquivalenceHTTPHandler(service factorysessionwire.DurableExecutionService) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if r.Method != http.MethodPost || len(parts) != 3 || parts[0] != "factory-sessions" {
			http.NotFound(w, r)
			return
		}
		sessionID, operation := parts[1], parts[2]
		var (
			result factorysessionexecution.LifecycleControlResult
			err    error
		)
		switch operation {
		case "pause":
			result, err = service.Pause(r.Context(), sessionID, factorysessionexecution.ControlRequest{})
		case "resume":
			result, err = service.Resume(r.Context(), sessionID, factorysessionexecution.ControlRequest{})
		default:
			http.NotFound(w, r)
			return
		}

		statusCode := http.StatusOK
		response := factoryapi.FactorySessionLifecycleControlResponse{
			SessionId: result.SessionID,
			Operation: factoryapi.FactorySessionLifecycleControlKind(result.Operation),
			Outcome:   factoryapi.FactorySessionLifecycleControlOutcome(result.Outcome),
			Status:    factoryapi.FactorySessionDurableLifecycleStatus(result.Status),
		}
		var rejected *factorysessionexecution.ControlError
		if errors.As(err, &rejected) {
			statusCode = http.StatusConflict
			response.SessionId = sessionID
			response.Operation = factoryapi.FactorySessionLifecycleControlKind(rejected.Operation)
			response.Outcome = factoryapi.FactorySessionLifecycleControlOutcome(rejected.Outcome)
			response.Status = factoryapi.FactorySessionDurableLifecycleStatus(rejected.Status)
		} else if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(response)
	})
}

func postLifecycleControl(
	t *testing.T,
	serverURL, sessionID, operation string,
) (factoryapi.FactorySessionLifecycleControlResponse, int) {
	t.Helper()
	url := serverURL + "/factory-sessions/" + sessionID + "/" + operation
	resp, err := http.Post(url, "application/json", http.NoBody)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	var response factoryapi.FactorySessionLifecycleControlResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode lifecycle control response: %v", err)
	}
	return response, resp.StatusCode
}

func assertLifecycleControlEquivalence(
	t *testing.T,
	apiResponse, cliResponse factoryapi.FactorySessionLifecycleControlResponse,
) {
	t.Helper()
	if cliResponse.Operation != apiResponse.Operation {
		t.Fatalf("CLI operation = %q, API operation = %q", cliResponse.Operation, apiResponse.Operation)
	}
	if cliResponse.Outcome != apiResponse.Outcome {
		t.Fatalf("CLI outcome = %q, API outcome = %q", cliResponse.Outcome, apiResponse.Outcome)
	}
	if cliResponse.Status != apiResponse.Status {
		t.Fatalf("CLI status = %q, API status = %q", cliResponse.Status, apiResponse.Status)
	}
}
