package apiserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	sessioncli "github.com/portpowered/infinite-you/pkg/cli/session"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

func TestLifecycleControlPause_CLIJSONMatchesAPIResponse(t *testing.T) {
	service := newAPILifecycleFakeService(t)
	serverURL := serverURLForLifecycle(t, service)

	apiResponse, status := postFactorySessionLifecycleControl(
		t,
		serverURL,
		startFixtureSessionByRequestID(t, service, "req-js-run-n-001"),
		"pause",
		nil,
	)
	if status != 200 {
		t.Fatalf("API pause status = %d, want 200", status)
	}

	var cliOut bytes.Buffer
	if err := sessioncli.Pause(sessioncli.LifecycleControlConfig{
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
	service := newAPILifecycleFakeService(t)
	row := startAPIRunningSessionForControl(t, service)
	serverURL := serverURLForLifecycle(t, service)

	if _, status := postFactorySessionLifecycleControl(t, serverURL, row.SessionID, "pause", nil); status != 200 {
		t.Fatalf("initial API pause status = %d, want 200", status)
	}
	apiResponse, status := postFactorySessionLifecycleControl(t, serverURL, row.SessionID, "pause", nil)
	if status != 200 {
		t.Fatalf("API no-op pause status = %d, want 200", status)
	}
	if apiResponse.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeNoOp {
		t.Fatalf("API outcome = %q, want NO_OP", apiResponse.Outcome)
	}

	var cliOut bytes.Buffer
	if err := sessioncli.Pause(sessioncli.LifecycleControlConfig{
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
	service := newAPILifecycleFakeService(t)
	serverURL := serverURLForLifecycle(t, service)
	sessionID := startFixtureSessionByRequestID(t, service, "req-js-run-n-001")

	if _, status := postFactorySessionLifecycleControl(t, serverURL, sessionID, "pause", nil); status != 200 {
		t.Fatalf("API pause status = %d, want 200", status)
	}

	apiResponse, status := postFactorySessionLifecycleControl(t, serverURL, sessionID, "resume", nil)
	if status != 200 {
		t.Fatalf("API resume status = %d, want 200", status)
	}

	cliSessionID := startFixtureSessionByRequestID(t, service, "req-petri-run-001")
	if _, status := postFactorySessionLifecycleControl(t, serverURL, cliSessionID, "pause", nil); status != 200 {
		t.Fatalf("API pause cli session status = %d, want 200", status)
	}

	var cliOut bytes.Buffer
	if err := sessioncli.Resume(sessioncli.LifecycleControlConfig{
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
	service := newAPILifecycleFakeService(t)
	_, err := service.StartSync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-petri-success-001",
		Source:    factorysessionexecution.Source{Kind: workflowsource.KindFactoryID, FactoryID: "customer-support-triage"},
	})
	if err != nil {
		t.Fatalf("StartSync terminal session: %v", err)
	}
	serverURL := serverURLForLifecycle(t, service)
	sessionID := "dur-sess-petri-success-001"

	apiResponse, status := postFactorySessionLifecycleControl(t, serverURL, sessionID, "pause", nil)
	if status != 409 {
		t.Fatalf("API pause status = %d, want 409", status)
	}
	if apiResponse.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeTerminalSession {
		t.Fatalf("API outcome = %q, want TERMINAL_SESSION", apiResponse.Outcome)
	}

	var cliOut bytes.Buffer
	err = sessioncli.Pause(sessioncli.LifecycleControlConfig{
		Server:    serverURL,
		SessionID: sessionID,
		JSON:      true,
		Output:    &cliOut,
	})
	var rejected *sessioncli.LifecycleControlRejectedError
	if !errors.As(err, &rejected) {
		t.Fatalf("CLI error = %v, want LifecycleControlRejectedError", err)
	}

	var cliResponse factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(cliOut.Bytes(), &cliResponse); err != nil {
		t.Fatalf("decode CLI JSON: %v\n%s", err, cliOut.String())
	}
	assertLifecycleControlEquivalence(t, apiResponse, cliResponse)
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
