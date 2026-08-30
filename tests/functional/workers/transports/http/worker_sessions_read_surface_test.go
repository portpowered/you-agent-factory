package http_test

import (
	"encoding/json"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestWorkerSessionHTTPReadDuringFactoryWork exercises the customer-facing
// Factory Session route while a real dispatch is held at the provider command
// edge. The route must remain readable during the in-flight recording and must
// return the same scoped attempt after the recording becomes terminal.
func TestWorkerSessionHTTPReadDuringFactoryWork(t *testing.T) {
	dir := support.ScaffoldSingleStepFactory(t, "worker-sessions-read-surface")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "test-model"))

	gate := make(chan struct{})
	runner := newFunctionalWorkerGate(gate)
	recordPath := filepath.Join(t.TempDir(), "worker-sessions-read-surface.json")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		Args:                      []string{"--record", recordPath},
		WaitForServiceModeRuntime: true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})
	t.Cleanup(func() { server.Stop(t) })

	emptyFleet := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, server.URL()+"/worker-sessions")
	if emptyFleet.Sessions == nil || len(emptyFleet.Sessions) != 0 {
		t.Fatalf("empty fleet Worker Session list = %#v, want non-nil empty collection", emptyFleet)
	}

	opened := support.OpenFactorySessionAt(t, server.URL(), dir)
	if opened.Session == nil || opened.Session.Id == "" {
		t.Fatalf("opened Factory Session = %#v, want resolved session identity", opened)
	}
	sessionID := opened.Session.Id

	name := "worker-sessions-read-surface-work"
	submitted := support.SubmitSessionWorkAt(t, server.URL(), sessionID, factoryapi.SubmitWorkRequest{
		Name:         &name,
		WorkTypeName: "task",
		Payload:      map[string]string{"title": "read the in-flight Worker Session"},
	})
	workID := support.StringPointerValue(submitted.WorkId)
	if workID == "" {
		t.Fatalf("submitted Work response = %#v, want Work ID", submitted)
	}

	runner.waitStarted(t)
	inFlight := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, workerSessionsListURL(server.URL(), sessionID, workID))
	cliInFlight := executeWorkerSessionsListJSON(t, server, sessionID, workID)
	assertWorkerSessionReadIdentityEqual(t, "in-flight", cliInFlight, inFlight)
	if len(inFlight.Sessions) != 1 {
		t.Fatalf("in-flight Worker Session list = %#v, want one scoped observation", inFlight)
	}
	fleet := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, server.URL()+"/worker-sessions")
	if len(fleet.Sessions) != 1 || fleet.Sessions[0].WorkerSessionId != inFlight.Sessions[0].WorkerSessionId {
		t.Fatalf("in-flight fleet Worker Session list = %#v, want the scoped observation", fleet)
	}
	if inFlight.Sessions[0].WorkerSessionId == "" || len(inFlight.Sessions[0].WorkIds) != 1 || inFlight.Sessions[0].WorkIds[0] != workID {
		t.Fatalf("in-flight Worker Session observation = %#v, want requested Work correlation", inFlight.Sessions[0])
	}
	if inFlight.Sessions[0].State != factoryapi.WorkerSessionObservationStateStarting && inFlight.Sessions[0].State != factoryapi.WorkerSessionObservationStateRunning {
		t.Fatalf("in-flight Worker Session state = %q, want STARTING or RUNNING", inFlight.Sessions[0].State)
	}

	close(gate)
	runner.waitCompleted(t)
	support.WaitForSessionTerminalStatus(t, server.URL(), sessionID, 10*time.Second)
	completed := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, workerSessionsListURL(server.URL(), sessionID, workID))
	cliCompleted := executeWorkerSessionsListJSON(t, server, sessionID, workID)
	assertWorkerSessionReadIdentityEqual(t, "completed", cliCompleted, completed)
	if len(completed.Sessions) != 1 || completed.Sessions[0].WorkerSessionId != inFlight.Sessions[0].WorkerSessionId {
		t.Fatalf("completed Worker Session list = %#v, want the same single attempt", completed)
	}
	completedFleet := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, server.URL()+"/worker-sessions")
	if len(completedFleet.Sessions) != 1 || completedFleet.Sessions[0].WorkerSessionId != inFlight.Sessions[0].WorkerSessionId {
		t.Fatalf("completed fleet Worker Session list = %#v, want the same single attempt", completedFleet)
	}
	if completed.Sessions[0].State != factoryapi.WorkerSessionObservationStateCompleted {
		t.Fatalf("completed Worker Session state = %q, want COMPLETED", completed.Sessions[0].State)
	}
}

func assertWorkerSessionReadIdentityEqual(
	t *testing.T,
	phase string,
	cliResponse, restResponse factoryapi.ListWorkerSessionsResponse,
) {
	t.Helper()
	if len(cliResponse.Sessions) != len(restResponse.Sessions) {
		t.Fatalf("%s CLI and REST Worker Session counts differ: CLI=%d REST=%d", phase, len(cliResponse.Sessions), len(restResponse.Sessions))
	}
	for index := range cliResponse.Sessions {
		cliObservation := cliResponse.Sessions[index]
		restObservation := restResponse.Sessions[index]
		if cliObservation.WorkerSessionId != restObservation.WorkerSessionId ||
			cliObservation.AttemptId != restObservation.AttemptId ||
			cliObservation.State != restObservation.State ||
			cliObservation.WorkId == nil || restObservation.WorkId == nil ||
			*cliObservation.WorkId != *restObservation.WorkId ||
			len(cliObservation.WorkIds) != len(restObservation.WorkIds) {
			t.Fatalf("%s CLI and REST Worker Session identity at index %d differs:\nCLI: %#v\nREST: %#v", phase, index, cliObservation, restObservation)
		}
		for workIndex := range cliObservation.WorkIds {
			if cliObservation.WorkIds[workIndex] != restObservation.WorkIds[workIndex] {
				t.Fatalf("%s CLI and REST Work IDs at observation index %d differ:\nCLI: %#v\nREST: %#v", phase, index, cliObservation.WorkIds, restObservation.WorkIds)
			}
		}
		if (cliObservation.FactorySessionId == nil) != (restObservation.FactorySessionId == nil) ||
			(cliObservation.FactorySessionId != nil && *cliObservation.FactorySessionId != *restObservation.FactorySessionId) {
			t.Fatalf("%s CLI and REST Factory Session identity at index %d differs: CLI=%#v REST=%#v", phase, index, cliObservation.FactorySessionId, restObservation.FactorySessionId)
		}
	}
}

func workerSessionsListURL(baseURL, sessionID, workID string) string {
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/worker-sessions"
	if workID == "" {
		return endpoint
	}
	return endpoint + "?workId=" + url.QueryEscape(workID)
}

func executeWorkerSessionsListJSON(
	t *testing.T,
	server *support.FunctionalAPIServer,
	sessionID, workID string,
) factoryapi.ListWorkerSessionsResponse {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "worker-sessions", "list", "--session", sessionID,
		"--work-id", workID, "--server", server.URL(), "--output", "json",
	})
	if err := server.Execute(t, inputs.Input); err != nil {
		t.Fatalf("JSON Worker Sessions list for session %q Work %q: %v\nstderr:\n%s", sessionID, workID, err, inputs.Stderr())
	}
	var response factoryapi.ListWorkerSessionsResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stdout())), &response); err != nil {
		t.Fatalf("decode JSON Worker Sessions list for session %q Work %q: %v\nstdout:\n%s", sessionID, workID, err, inputs.Stdout())
	}
	return response
}

func executeDefaultWorkerSessionsListJSON(
	t *testing.T,
	server *support.FunctionalAPIServer,
	workID string,
) factoryapi.ListWorkerSessionsResponse {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "worker-sessions", "list", "--work-id", workID,
		"--server", server.URL(), "--output", "json",
	})
	if err := server.Execute(t, inputs.Input); err != nil {
		t.Fatalf("default JSON Worker Sessions list for Work %q: %v\nstderr:\n%s", workID, err, inputs.Stderr())
	}
	var response factoryapi.ListWorkerSessionsResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stdout())), &response); err != nil {
		t.Fatalf("decode default JSON Worker Sessions list for Work %q: %v\nstdout:\n%s", workID, err, inputs.Stdout())
	}
	return response
}
