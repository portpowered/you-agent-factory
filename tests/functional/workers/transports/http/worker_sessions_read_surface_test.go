package http_test

import (
	"context"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
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
	if len(inFlight.Sessions) != 1 {
		t.Fatalf("in-flight Worker Session list = %#v, want one scoped observation", inFlight)
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
	if len(completed.Sessions) != 1 || completed.Sessions[0].WorkerSessionId != inFlight.Sessions[0].WorkerSessionId {
		t.Fatalf("completed Worker Session list = %#v, want the same single attempt", completed)
	}
	if completed.Sessions[0].State != factoryapi.WorkerSessionObservationStateCompleted {
		t.Fatalf("completed Worker Session state = %q, want COMPLETED", completed.Sessions[0].State)
	}
}

func TestWorkerSessionHTTPFleetListIncludesDirectAndFactoryObservations(t *testing.T) {
	dir := support.ScaffoldSingleStepFactory(t, "worker-sessions-fleet-read-surface")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "test-model"))

	gate := make(chan struct{})
	runner := newFunctionalWorkerGate(gate)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})
	t.Cleanup(func() { server.Stop(t) })

	opened := support.OpenFactorySessionAt(t, server.URL(), dir)
	if opened.Session == nil || opened.Session.Id == "" {
		t.Fatalf("opened Factory Session = %#v, want resolved session identity", opened)
	}
	workName := "fleet-factory-work"
	submitted := support.SubmitSessionWorkAt(t, server.URL(), opened.Session.Id, factoryapi.SubmitWorkRequest{
		Name:         &workName,
		WorkTypeName: "task",
		Payload:      map[string]string{"title": "list both Worker Session origins"},
	})
	workID := support.StringPointerValue(submitted.WorkId)
	if workID == "" {
		t.Fatalf("submitted Work response = %#v, want Work ID", submitted)
	}

	runner.waitStarted(t)
	direct := postDirectWorkerSession(t, context.Background(), server.URL(), "fleet-direct-request", "fleet-direct-session", "fleet-direct-dispatch")
	direct.Body.Close()
	if direct.StatusCode != http.StatusAccepted {
		t.Fatalf("POST direct Worker Session status = %d, want 202", direct.StatusCode)
	}
	close(gate)
	runner.waitCompleted(t)
	support.WaitForSessionTerminalStatus(t, server.URL(), opened.Session.Id, 10*time.Second)

	listed := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, strings.TrimSuffix(server.URL(), "/")+"/worker-sessions")
	if len(listed.Sessions) != 2 {
		t.Fatalf("fleet Worker Session list = %#v, want direct and Factory observations", listed)
	}
	ids := make([]string, 0, len(listed.Sessions))
	seenOrigins := make(map[string]bool, len(listed.Sessions))
	for _, session := range listed.Sessions {
		ids = append(ids, session.WorkerSessionId)
		if session.WorkerSessionId == "fleet-direct-session" {
			if !session.Direct {
				t.Fatalf("direct fleet observation = %#v, want direct=true", session)
			}
			seenOrigins["direct"] = true
			continue
		}
		if len(session.WorkIds) != 1 || session.WorkIds[0] != workID || session.WorkId == nil || *session.WorkId != workID || session.WorkName == nil || *session.WorkName != workName {
			t.Fatalf("Factory fleet observation = %#v, want Work %q", session, workID)
		}
		if session.Direct {
			t.Fatalf("Factory fleet observation = %#v, want direct=false", session)
		}
		seenOrigins["factory"] = true
	}
	wantIDs := append([]string(nil), ids...)
	sort.Strings(wantIDs)
	if strings.Join(ids, "\x00") != strings.Join(wantIDs, "\x00") {
		t.Fatalf("fleet Worker Session IDs = %#v, want deterministic ascending order", ids)
	}
	if !seenOrigins["direct"] || !seenOrigins["factory"] {
		t.Fatalf("fleet Worker Session origins = %#v, want direct and Factory", seenOrigins)
	}
}

func workerSessionsListURL(baseURL, sessionID, workID string) string {
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/worker-sessions"
	if workID == "" {
		return endpoint
	}
	return endpoint + "?workId=" + url.QueryEscape(workID)
}
