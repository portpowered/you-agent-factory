package session

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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

func TestBindServiceDelegatesToInjectedOperations(t *testing.T) {
	t.Parallel()

	calls := 0
	service := Bind(Operations{
		List: func(ListConfig) error {
			calls++
			return nil
		},
		Show:           func(ShowConfig) error { calls++; return nil },
		Pause:          func(LifecycleControlConfig) error { calls++; return nil },
		Resume:         func(LifecycleControlConfig) error { calls++; return nil },
		ListDispatches: func(DispatchesConfig) error { calls++; return nil },
		Create:         func(CreateConfig) error { calls++; return nil },
		Delete:         func(DeleteConfig) error { calls++; return nil },
	})
	if err := service.List(ListConfig{Context: context.Background()}); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	for name, call := range map[string]func() error{
		"show":       func() error { return service.Show(ShowConfig{}) },
		"pause":      func() error { return service.Pause(LifecycleControlConfig{}) },
		"resume":     func() error { return service.Resume(LifecycleControlConfig{}) },
		"dispatches": func() error { return service.ListDispatches(DispatchesConfig{}) },
		"create":     func() error { return service.Create(CreateConfig{}) },
		"delete":     func() error { return service.Delete(DeleteConfig{}) },
	} {
		if err := call(); err != nil {
			t.Fatalf("%s error = %v", name, err)
		}
	}
	if calls != 7 {
		t.Fatalf("calls = %d, want 7", calls)
	}
}

func TestBindServiceRequiresInjectedOperations(t *testing.T) {
	t.Parallel()

	service := Bind(Operations{})
	for name, call := range map[string]func() error{
		"list":       func() error { return service.List(ListConfig{}) },
		"show":       func() error { return service.Show(ShowConfig{}) },
		"pause":      func() error { return service.Pause(LifecycleControlConfig{}) },
		"resume":     func() error { return service.Resume(LifecycleControlConfig{}) },
		"dispatches": func() error { return service.ListDispatches(DispatchesConfig{}) },
		"create":     func() error { return service.Create(CreateConfig{}) },
		"delete":     func() error { return service.Delete(DeleteConfig{}) },
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
