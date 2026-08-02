package session_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	fse "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	sessioncli "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/session"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestCLIResumeSmokeLane_NonResumeFixtureBackedListAndPauseRegression(t *testing.T) {
	service := &resumeRegressionExecutionScript{}
	server := httptest.NewServer(resumeRegressionHTTPHandler(service))
	t.Cleanup(server.Close)

	var listOut bytes.Buffer
	if err := sessioncli.NewList(testSessionHTTPProtocol(t), resumeSmokeListPreparation())(sessioncli.ListConfig{Context: context.Background(),
		Port:          testServerPort(t, server),
		Scope:         "persisted",
		DurableLister: service.ListSessions,
		JSON:          true,
		Output:        &listOut,
	}); err != nil {
		t.Fatalf("session list persisted: %v", err)
	}

	var listed factoryapi.ListFactorySessionsResponse
	if err := json.Unmarshal(bytes.TrimSpace(listOut.Bytes()), &listed); err != nil {
		t.Fatalf("decode list JSON: %v\n%s", err, listOut.String())
	}
	if listed.DurableSessions == nil || len(*listed.DurableSessions) == 0 {
		t.Fatalf("expected persisted durable sessions from root-contract script: %s", listOut.String())
	}
	assertResumeSmokeLaneOutputExcludesForbiddenVocabulary(t, listOut.String())

	sessionID := resumeRegressionSessionID

	var pauseOut bytes.Buffer
	if err := sessioncli.NewPause(testSessionHTTPProtocol(t))(sessioncli.LifecycleControlConfig{Context: context.Background(),
		Server:    server.URL,
		SessionID: sessionID,
		JSON:      true,
		Output:    &pauseOut,
	}); err != nil {
		t.Fatalf("session pause: %v", err)
	}

	var pauseResponse factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(bytes.TrimSpace(pauseOut.Bytes()), &pauseResponse); err != nil {
		t.Fatalf("decode pause JSON: %v\n%s", err, pauseOut.String())
	}
	if pauseResponse.Operation != factoryapi.FactorySessionLifecycleControlKindPause {
		t.Fatalf("pause operation = %q, want PAUSE", pauseResponse.Operation)
	}
	if pauseResponse.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("pause outcome = %q, want ACCEPTED", pauseResponse.Outcome)
	}

	var showOut bytes.Buffer
	if err := sessioncli.NewShow(testSessionHTTPProtocol(t))(sessioncli.ShowConfig{Context: context.Background(),
		Server:    server.URL,
		SessionID: sessionID,
		JSON:      true,
		Output:    &showOut,
	}); err != nil {
		t.Fatalf("session show after pause: %v", err)
	}

	var shown factoryapi.FactorySessionDurableReadModel
	if err := json.Unmarshal(bytes.TrimSpace(showOut.Bytes()), &shown); err != nil {
		t.Fatalf("decode show JSON: %v\n%s", err, showOut.String())
	}
	if shown.SessionId != sessionID {
		t.Fatalf("show sessionId = %q, want %q", shown.SessionId, sessionID)
	}
	if shown.Status != factoryapi.FactorySessionDurableLifecycleStatusPaused {
		t.Fatalf("show status = %q, want PAUSED", shown.Status)
	}
}

func resumeRegressionHTTPHandler(service *resumeRegressionExecutionScript) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/factory-sessions/"+resumeRegressionSessionID+"/pause":
			result, err := service.Pause(r.Context(), resumeRegressionSessionID, fse.ControlRequest{})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(factoryapi.FactorySessionLifecycleControlResponse{
				SessionId: result.SessionID,
				Operation: factoryapi.FactorySessionLifecycleControlKind(result.Operation),
				Outcome:   factoryapi.FactorySessionLifecycleControlOutcome(result.Outcome),
				Status:    factoryapi.FactorySessionDurableLifecycleStatus(result.Status),
			})
		case r.Method == http.MethodGet && r.URL.Path == "/factory-sessions/"+resumeRegressionSessionID:
			result, err := service.GetSession(r.Context(), resumeRegressionSessionID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(factoryapi.FactorySessionDurableReadModel{
				SessionId: result.SessionID,
				Status:    factoryapi.FactorySessionDurableLifecycleStatus(result.Status),
			})
		default:
			http.NotFound(w, r)
		}
	})
}

func TestCLIResumeSmokeLane_NonResumeLiveSessionCreateShowListRegression(t *testing.T) {
	var gotCreatePath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/factory-sessions":
			gotCreatePath = r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(factoryapi.OpenFactorySessionResponse{
				Session: &factoryapi.FactorySessionSummary{
					FactoryDir: "/workspace/fleet/beta",
					FolderPath: "/workspace/fleet",
					Id:         "session-beta",
					Project:    "beta",
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/factory-sessions":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(factoryapi.ListFactorySessionsResponse{
				Sessions: []factoryapi.FactorySessionSummary{
					{
						FactoryDir: "/workspace/fleet/beta",
						FolderPath: "/workspace/fleet",
						Id:         "session-beta",
						Project:    "beta",
					},
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/factory-sessions/session-beta":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(factoryapi.FactorySession{
				Id:         "session-beta",
				FactoryDir: "/workspace/fleet/beta",
				FolderPath: "/workspace/fleet",
				Project:    "beta",
				Runtime: factoryapi.FactorySessionRuntime{
					Status: factoryapi.FactorySessionStatusIDLE,
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	var createOut bytes.Buffer
	if err := sessioncli.NewCreate(testSessionHTTPProtocol(t))(sessioncli.CreateConfig{
		Port:   testServerPort(t, srv),
		Dir:    "/workspace/fleet",
		Output: &createOut,
	}); err != nil {
		t.Fatalf("session create: %v", err)
	}
	if gotCreatePath != "/factory-sessions" {
		t.Fatalf("create path = %q, want /factory-sessions", gotCreatePath)
	}

	var listOut bytes.Buffer
	if err := sessioncli.NewList(testSessionHTTPProtocol(t), resumeSmokeListPreparation())(sessioncli.ListConfig{Context: context.Background(),
		Port:   testServerPort(t, srv),
		Scope:  "live",
		Output: &listOut,
	}); err != nil {
		t.Fatalf("session list live: %v", err)
	}
	if !strings.Contains(listOut.String(), "session-beta") {
		t.Fatalf("live list output missing session-beta:\n%s", listOut.String())
	}

	var showOut bytes.Buffer
	if err := sessioncli.NewShow(testSessionHTTPProtocol(t))(sessioncli.ShowConfig{Context: context.Background(),
		Server:    srv.URL,
		SessionID: "session-beta",
		Output:    &showOut,
	}); err != nil {
		t.Fatalf("session show live: %v", err)
	}
	if !strings.Contains(showOut.String(), "session-beta") {
		t.Fatalf("live show output missing session-beta:\n%s", showOut.String())
	}
}

func resumeSmokeListPreparation() sessioncli.RequestPreparation {
	return resumeSmokeRequestPreparationCallbacks{
		list: func(request fse.ListSessionsRequest) (fse.ListSessionsRequest, error) {
			return request, nil
		},
	}
}

type resumeSmokeRequestPreparationCallbacks struct {
	list func(fse.ListSessionsRequest) (fse.ListSessionsRequest, error)
}

func (resumeSmokeRequestPreparationCallbacks) PrepareStart(
	fse.StartRequest,
) (fse.StartRequest, error) {
	return fse.StartRequest{}, fmt.Errorf("unexpected PrepareStart call")
}

func (resumeSmokeRequestPreparationCallbacks) PrepareControl(
	fse.ControlRequest,
) (fse.ControlRequest, error) {
	return fse.ControlRequest{}, fmt.Errorf("unexpected PrepareControl call")
}

func (resumeSmokeRequestPreparationCallbacks) PrepareApprove(
	fse.ApproveRequest,
) (fse.ApproveRequest, error) {
	return fse.ApproveRequest{}, fmt.Errorf("unexpected PrepareApprove call")
}

func (resumeSmokeRequestPreparationCallbacks) PrepareRetryDispatch(
	fse.RetryDispatchRequest,
) (fse.RetryDispatchRequest, error) {
	return fse.RetryDispatchRequest{}, fmt.Errorf("unexpected PrepareRetryDispatch call")
}

func (resumeSmokeRequestPreparationCallbacks) PrepareInterruptDispatch(
	fse.InterruptDispatchRequest,
) (fse.InterruptDispatchRequest, error) {
	return fse.InterruptDispatchRequest{}, fmt.Errorf("unexpected PrepareInterruptDispatch call")
}

func (callbacks resumeSmokeRequestPreparationCallbacks) PrepareListSessions(
	request fse.ListSessionsRequest,
) (fse.ListSessionsRequest, error) {
	if callbacks.list == nil {
		return fse.ListSessionsRequest{}, fmt.Errorf("unexpected PrepareListSessions call")
	}
	return callbacks.list(request)
}

func (resumeSmokeRequestPreparationCallbacks) PrepareResult(
	fse.ResultRequest,
) (fse.ResultRequest, error) {
	return fse.ResultRequest{}, fmt.Errorf("unexpected PrepareResult call")
}

func (resumeSmokeRequestPreparationCallbacks) PrepareEventReconnect(
	fse.EventReconnectRequest,
) (fse.EventReconnectRequest, error) {
	return fse.EventReconnectRequest{}, fmt.Errorf("unexpected PrepareEventReconnect call")
}

const resumeRegressionSessionID = "dur-sess-js-run-n-001"

type resumeRegressionExecutionScript struct {
	factorysessionwire.DurableExecutionService
	paused bool
}

func (script *resumeRegressionExecutionScript) ListSessions(
	context.Context,
	fse.ListSessionsRequest,
) (fse.ListSessionsResult, error) {
	return fse.ListSessionsResult{
		Scope: fse.SessionListScopePersisted,
		DurableSessions: []fse.DurableSessionListSummary{{
			SessionID: resumeRegressionSessionID, Status: fse.LifecycleStatusRunning,
			OrchestratorKind: "JAVASCRIPT", ResolvedSource: fse.ResolvedSource{Kind: "WORKFLOW_FILE", SourceRef: "workflow/run-n"},
			ResultSummary: &fse.ResultSummary{ResultStatus: "PARTIAL"}, Recoverable: true,
		}},
	}, nil
}

func (script *resumeRegressionExecutionScript) Pause(
	_ context.Context,
	sessionID string,
	_ fse.ControlRequest,
) (fse.LifecycleControlResult, error) {
	script.paused = true
	return fse.LifecycleControlResult{
		SessionID: sessionID, Operation: "PAUSE",
		Outcome: fse.LifecycleControlOutcomeAccepted, Status: fse.LifecycleStatusPaused,
	}, nil
}

func (script *resumeRegressionExecutionScript) GetSession(
	context.Context,
	string,
) (fse.SessionReadResult, error) {
	status := fse.LifecycleStatusRunning
	if script.paused {
		status = fse.LifecycleStatusPaused
	}
	return fse.SessionReadResult{
		SessionID: resumeRegressionSessionID, Status: status, OrchestratorKind: "JAVASCRIPT",
		ResultSummary: &fse.ResultSummary{ResultStatus: "PARTIAL"},
	}, nil
}

func testServerPort(t *testing.T, srv *httptest.Server) int {
	t.Helper()

	var port int
	if _, err := fmt.Sscanf(srv.URL, "http://127.0.0.1:%d", &port); err != nil {
		t.Fatalf("parse test server port: %v", err)
	}
	return port
}

func assertResumeSmokeLaneOutputExcludesForbiddenVocabulary(t *testing.T, text string) {
	t.Helper()
	lower := strings.ToLower(text)
	for _, term := range []string{"DynamicWorkflowRun", "workflow run"} {
		if strings.Contains(lower, strings.ToLower(term)) {
			t.Fatalf("output introduced forbidden vocabulary %q:\n%s", term, text)
		}
	}
}
