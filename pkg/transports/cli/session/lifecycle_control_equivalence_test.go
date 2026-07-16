package session

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/factory/sessions/execution"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	api "github.com/portpowered/infinite-you/pkg/transports/http"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
)

func TestLifecycleControlPause_CLIJSONMatchesAPIResponse(t *testing.T) {
	service := newLifecycleEquivalenceFakeService(t)
	serverURL := serverURLForLifecycleEquivalence(t, service)

	apiResponse, status := postLifecycleControl(
		t,
		serverURL,
		startFixtureSessionByRequestID(t, service, "req-js-run-n-001"),
		"pause",
	)
	if status != 200 {
		t.Fatalf("API pause status = %d, want 200", status)
	}

	var cliOut bytes.Buffer
	if err := Pause(LifecycleControlConfig{
		Server:    serverURL,
		SessionID: startFixtureSessionByRequestID(t, service, "req-petri-run-001"),
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

func TestLifecycleControlPause_CLIJSONMatchesAPINoOpResponse(t *testing.T) {
	service := newLifecycleEquivalenceFakeService(t)
	row := startRunningSessionForLifecycleEquivalence(t, service)
	serverURL := serverURLForLifecycleEquivalence(t, service)

	if _, status := postLifecycleControl(t, serverURL, row.SessionID, "pause"); status != 200 {
		t.Fatalf("initial API pause status = %d, want 200", status)
	}
	apiResponse, status := postLifecycleControl(t, serverURL, row.SessionID, "pause")
	if status != 200 {
		t.Fatalf("API no-op pause status = %d, want 200", status)
	}
	if apiResponse.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeNoOp {
		t.Fatalf("API outcome = %q, want NO_OP", apiResponse.Outcome)
	}

	var cliOut bytes.Buffer
	if err := Pause(LifecycleControlConfig{
		Server:    serverURL,
		SessionID: row.SessionID,
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
	service := newLifecycleEquivalenceFakeService(t)
	serverURL := serverURLForLifecycleEquivalence(t, service)
	sessionID := startFixtureSessionByRequestID(t, service, "req-js-run-n-001")

	if _, status := postLifecycleControl(t, serverURL, sessionID, "pause"); status != 200 {
		t.Fatalf("API pause status = %d, want 200", status)
	}

	apiResponse, status := postLifecycleControl(t, serverURL, sessionID, "resume")
	if status != 200 {
		t.Fatalf("API resume status = %d, want 200", status)
	}

	cliSessionID := startFixtureSessionByRequestID(t, service, "req-petri-run-001")
	if _, status := postLifecycleControl(t, serverURL, cliSessionID, "pause"); status != 200 {
		t.Fatalf("API pause cli session status = %d, want 200", status)
	}

	var cliOut bytes.Buffer
	if err := Resume(LifecycleControlConfig{
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
	service := newLifecycleEquivalenceFakeService(t)
	_, err := service.StartSync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-petri-success-001",
		Source:    factorysessionexecution.Source{Kind: workflowsource.KindFactoryID, FactoryID: "customer-support-triage"},
	})
	if err != nil {
		t.Fatalf("StartSync terminal session: %v", err)
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
	err = Pause(LifecycleControlConfig{
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
	apiServerURL := liveLifecycleEquivalenceServerURL(t, &testutil.MockFactory{
		State:          interfaces.FactoryStateRunning,
		FactorySession: factoryapi.FactorySession{Id: "~default"},
	})

	apiResponse, status := postLifecycleControl(t, apiServerURL, "~default", "pause")
	if status != 200 {
		t.Fatalf("API pause status = %d, want 200", status)
	}

	cliServerURL := liveLifecycleEquivalenceServerURL(t, &testutil.MockFactory{
		State:          interfaces.FactoryStateRunning,
		FactorySession: factoryapi.FactorySession{Id: "~default"},
	})

	var cliOut bytes.Buffer
	if err := Pause(LifecycleControlConfig{
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
	apiServerURL := liveLifecycleEquivalenceServerURL(t, &testutil.MockFactory{
		State:          interfaces.FactoryStatePaused,
		FactorySession: factoryapi.FactorySession{Id: "session-beta"},
	})

	apiResponse, status := postLifecycleControl(t, apiServerURL, "session-beta", "resume")
	if status != 200 {
		t.Fatalf("API resume status = %d, want 200", status)
	}

	cliServerURL := liveLifecycleEquivalenceServerURL(t, &testutil.MockFactory{
		State:          interfaces.FactoryStatePaused,
		FactorySession: factoryapi.FactorySession{Id: "session-beta"},
	})

	var cliOut bytes.Buffer
	if err := Resume(LifecycleControlConfig{
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

func newLifecycleEquivalenceFakeService(t *testing.T) *factorysessionexecution.FakeService {
	t.Helper()
	service, err := factorysessionexecution.NewFakeServiceFromContractFixtures(
		filepath.Join("..", "..", "http", "testdata", "durable-session-contract-fixtures.json"),
	)
	if err != nil {
		t.Fatalf("NewFakeServiceFromContractFixtures: %v", err)
	}
	return service
}

func serverURLForLifecycleEquivalence(t *testing.T, service factorysessionexecution.Service) string {
	t.Helper()
	logger, _ := zap.NewDevelopment()
	srv := api.NewServer(&testutil.MockFactory{DurableExecutionService: service}, 8080, logger)
	server := httptest.NewServer(srv.Handler())
	t.Cleanup(server.Close)
	return server.URL
}

func liveLifecycleEquivalenceServerURL(t *testing.T, factory *testutil.MockFactory) string {
	t.Helper()
	logger, _ := zap.NewDevelopment()
	srv := api.NewServer(factory, 8080, logger)
	server := httptest.NewServer(srv.Handler())
	t.Cleanup(server.Close)
	return server.URL
}

func startRunningSessionForLifecycleEquivalence(t *testing.T, service *factorysessionexecution.FakeService) struct {
	SessionID string
} {
	t.Helper()
	started, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-js-run-n-001",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowFile,
			WorkflowFile: ".claude/workflows/run-n.yaml",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync running session: %v", err)
	}
	return struct{ SessionID string }{SessionID: started.SessionID}
}

func startFixtureSessionByRequestID(
	t *testing.T,
	service *factorysessionexecution.FakeService,
	requestID string,
) string {
	t.Helper()
	started, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: requestID,
		Source:    factorysessionexecution.Source{Kind: workflowsource.KindFactoryID, FactoryID: "customer-support-triage"},
	})
	if err != nil {
		t.Fatalf("StartAsync fixture session %q: %v", requestID, err)
	}
	return started.SessionID
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
