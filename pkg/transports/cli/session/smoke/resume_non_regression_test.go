package session_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	fse "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/execution/fixtures"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	sessioncli "github.com/portpowered/infinite-you/pkg/transports/cli/session"
	api "github.com/portpowered/infinite-you/pkg/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

func TestCLIResumeSmokeLane_NonResumeTerminalSessionShowPreservesShippedCLIReadSemantics(t *testing.T) {
	harness := newCLIResumeSmokeSucceededHarness(t)
	sessionID := harness.startSucceededSession(t)

	read := readDurableSessionViaCLI(t, harness.serverURL, sessionID)
	if read.SessionId != sessionID {
		t.Fatalf("sessionId = %q, want %q", read.SessionId, sessionID)
	}
	if read.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("status = %q, want SUCCEEDED", read.Status)
	}
	if read.Lifecycle != nil && read.Lifecycle.ResumedAt != nil {
		t.Fatalf("terminal non-resume read should not expose resumedAt: %#v", read.Lifecycle)
	}
	if read.ResultSummary == nil || read.ResultSummary.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("resultSummary = %#v, want FINAL", read.ResultSummary)
	}

	dispatches := readDispatchesViaCLI(t, harness.serverURL, sessionID)
	if len(dispatches.Dispatches) != 0 {
		t.Fatalf("terminal simple-final dispatches = %#v, want empty", dispatches.Dispatches)
	}
}

func TestCLIResumeSmokeLane_NonResumeFixtureBackedListAndPauseRegression(t *testing.T) {
	catalogPath := filepath.Join("..", "..", "..", "http", "testdata", "durable-session-contract-fixtures.json")
	service, err := fse.NewFakeServiceFromContractFixtures(catalogPath)
	if err != nil {
		t.Fatalf("NewFakeServiceFromContractFixtures: %v", err)
	}

	server := httptest.NewServer(api.NewServer(&testutil.MockFactory{
		DurableExecutionService: service,
	}, 0, zap.NewNop()).Handler())
	t.Cleanup(server.Close)

	var listOut bytes.Buffer
	if err := sessioncli.List(sessioncli.ListConfig{
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
		t.Fatal("expected persisted durable sessions from contract fixtures")
	}
	assertResumeSmokeLaneOutputExcludesForbiddenVocabulary(t, listOut.String())

	sessionID := startFixtureSessionByRequestIDForResumeRegression(t, service, "req-js-run-n-001")

	var pauseOut bytes.Buffer
	if err := sessioncli.Pause(sessioncli.LifecycleControlConfig{
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
	if err := sessioncli.Show(sessioncli.ShowConfig{
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
	if err := sessioncli.Create(sessioncli.CreateConfig{
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
	if err := sessioncli.List(sessioncli.ListConfig{
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
	if err := sessioncli.Show(sessioncli.ShowConfig{
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

func TestCLIResumeSmokeLane_ResumeInspectionStaysOnSharedSessionHTTPSurface(t *testing.T) {
	err := sessioncli.Dispatches(sessioncli.DispatchesConfig{
		Server:    "http://127.0.0.1:1",
		SessionID: "session-beta",
		Output:    &bytes.Buffer{},
	})
	if err == nil || !strings.Contains(err.Error(), "dur-sess-*") {
		t.Fatalf("dispatches on live session id = %v, want durable-session validation error", err)
	}

	projectRoot := setupCLIResumeSmokeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	runtimeService := newCLIResumeRuntimeService(t, projectRoot, fse.ChildExecutorModeFake, nil)
	started, err := runtimeService.StartAsync(context.Background(), fse.StartRequest{
		RequestID: "req-cli-resume-scope-001",
		Source: fse.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "simple-final",
		},
		Args: map[string]any{
			"subject": "workflows",
			"count":   2,
			"prefix":  "you",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	waitForCLIResumeSmokeSessionStatus(t, runtimeService, started.SessionID, fse.LifecycleStatusSucceeded, 15*time.Second)

	server := httptest.NewServer(api.NewServer(&testutil.MockFactory{
		DurableExecutionService: runtimeService,
	}, 0, zap.NewNop()).Handler())
	t.Cleanup(server.Close)

	var out bytes.Buffer
	if err := sessioncli.Dispatches(sessioncli.DispatchesConfig{
		Server:    server.URL,
		SessionID: started.SessionID,
		JSON:      true,
		Output:    &out,
	}); err != nil {
		t.Fatalf("session dispatches durable HTTP: %v", err)
	}

	var listed factoryapi.ListFactorySessionDispatchesResponse
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &listed); err != nil {
		t.Fatalf("decode dispatches JSON: %v\n%s", err, out.String())
	}
	if listed.SessionId != started.SessionID {
		t.Fatalf("dispatch sessionId = %q, want %q", listed.SessionId, started.SessionID)
	}
}

func startFixtureSessionByRequestIDForResumeRegression(t *testing.T, service fse.Service, requestID string) string {
	t.Helper()

	started, err := service.StartAsync(context.Background(), fse.StartRequest{
		RequestID: requestID,
		Source:    fse.Source{Kind: workflowsource.KindFactoryID, FactoryID: "customer-support-triage"},
	})
	if err != nil {
		t.Fatalf("StartAsync %s: %v", requestID, err)
	}
	if started.SessionID == "" {
		t.Fatalf("session id unexpectedly empty for %s", requestID)
	}
	return started.SessionID
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
	for _, term := range fixtures.ForbiddenFixtureVocabularyTerms() {
		if strings.Contains(lower, strings.ToLower(term)) {
			t.Fatalf("output introduced forbidden vocabulary %q:\n%s", term, text)
		}
	}
}
