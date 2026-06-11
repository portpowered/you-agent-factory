package factorysession_test

import (
	"context"
	"path/filepath"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

// pkgmaintcheck:ignore-cyclomatic-complexity this consumer test keeps fake-service projection assertions together across apisurface mappers.
func TestFakeServiceConsumer_StartAsync_ProjectsCoreScenarioOutcomes(t *testing.T) {
	fixturesPath := filepath.Join("..", "..", "api", "testdata", "durable-session-contract-fixtures.json")
	service, err := factorysessionexecution.NewFakeServiceFromContractFixtures(fixturesPath)
	if err != nil {
		t.Fatalf("NewFakeServiceFromContractFixtures: %v", err)
	}

	cases := []struct {
		name       string
		scenarioID string
		requestID  string
		sessionID  string
		status     factoryapi.FactorySessionDurableLifecycleStatus
	}{
		{"success", "petri-succeeded-one-dispatch", "", "dur-sess-petri-success-001", factoryapi.FactorySessionDurableLifecycleStatusSucceeded},
		{"running", "petri-running-one-dispatch", "", "dur-sess-petri-run-001", factoryapi.FactorySessionDurableLifecycleStatusRunning},
		{"failed-with-partial", "javascript-failed-with-partial", "", "dur-sess-js-failed-partial-001", factoryapi.FactorySessionDurableLifecycleStatusFailed},
		{"interrupted", "", "req-js-interrupted-001", "dur-sess-js-interrupted-001", factoryapi.FactorySessionDurableLifecycleStatusInterrupted},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var request factorysessionexecution.StartRequest
			if tc.scenarioID != "" {
				scenario := findScenario(t, loadDurableFixtureCatalog(t), tc.scenarioID)
				executionRequest, ok := scenario["executionRequest"].(map[string]any)
				if !ok {
					t.Fatal("missing executionRequest fixture")
				}
				var err error
				request, err = factorysession.StartRequestFromAPI(decodeExecutionRequest(t, executionRequest))
				if err != nil {
					t.Fatalf("StartRequestFromAPI: %v", err)
				}
			} else {
				request = factorysessionexecution.StartRequest{
					RequestID: tc.requestID,
					Source: factorysessionexecution.Source{
						Kind:      workflowsource.KindFactoryID,
						FactoryID: "customer-support-triage",
					},
				}
			}
			started, err := service.StartAsync(context.Background(), request)
			if err != nil {
				t.Fatalf("StartAsync: %v", err)
			}
			mapped := factorysession.AsyncStartResponseToAPI(started)
			if mapped.SessionId != tc.sessionID {
				t.Fatalf("sessionId = %q, want %q", mapped.SessionId, tc.sessionID)
			}
			if mapped.Status != tc.status {
				t.Fatalf("status = %q, want %q", mapped.Status, tc.status)
			}
			if mapped.Links.Session == nil || *mapped.Links.Session == "" {
				t.Fatal("session inspection link missing")
			}
		})
	}
}

func TestFakeServiceConsumer_ListAndDetail_ProjectsScopedSummaries(t *testing.T) {
	fixturesPath := filepath.Join("..", "..", "api", "testdata", "durable-session-contract-fixtures.json")
	service, err := factorysessionexecution.NewFakeServiceFromContractFixtures(fixturesPath)
	if err != nil {
		t.Fatalf("NewFakeServiceFromContractFixtures: %v", err)
	}

	start := func(requestID string) {
		t.Helper()
		_, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
			RequestID: requestID,
			Source: factorysessionexecution.Source{
				Kind:      workflowsource.KindFactoryID,
				FactoryID: "customer-support-triage",
			},
		})
		if err != nil {
			t.Fatalf("StartAsync(%q): %v", requestID, err)
		}
	}
	start("req-petri-run-001")
	start("req-petri-success-001")
	start("req-js-interrupted-001")

	liveScope := factoryapi.FactorySessionListScopeLive
	liveReq, err := factorysession.ListSessionsRequestFromAPI(factoryapi.ListFactorySessionsParams{Scope: &liveScope})
	if err != nil {
		t.Fatalf("ListSessionsRequestFromAPI live: %v", err)
	}
	live, err := service.ListSessions(context.Background(), liveReq)
	if err != nil {
		t.Fatalf("ListSessions live: %v", err)
	}
	liveAPI := factorysession.ListSessionsResponseToAPI(live)
	if liveAPI.DurableSessions != nil {
		t.Fatalf("live response durableSessions = %#v, want omitted", liveAPI.DurableSessions)
	}

	persistedScope := factoryapi.FactorySessionListScopePersisted
	persistedReq, err := factorysession.ListSessionsRequestFromAPI(factoryapi.ListFactorySessionsParams{Scope: &persistedScope})
	if err != nil {
		t.Fatalf("ListSessionsRequestFromAPI persisted: %v", err)
	}
	persisted, err := service.ListSessions(context.Background(), persistedReq)
	if err != nil {
		t.Fatalf("ListSessions persisted: %v", err)
	}
	persistedAPI := factorysession.ListSessionsResponseToAPI(persisted)
	if persistedAPI.DurableSessions == nil || len(*persistedAPI.DurableSessions) == 0 {
		t.Fatal("persisted durableSessions missing")
	}

	read, err := service.GetSession(context.Background(), "dur-sess-petri-success-001")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	mappedRead := factorysession.SessionReadResponseToAPI(read)
	if mappedRead.Progress == nil || mappedRead.Progress.TotalDispatches == nil || *mappedRead.Progress.TotalDispatches == 0 {
		t.Fatalf("detail progress missing dispatch count: %#v", mappedRead.Progress)
	}

	var successSummary factoryapi.FactorySessionDurableSummary
	for _, row := range *persistedAPI.DurableSessions {
		if row.SessionId == "dur-sess-petri-success-001" {
			successSummary = row
			break
		}
	}
	if successSummary.SessionId == "" {
		t.Fatal("persisted list missing success session")
	}
	if mappedRead.ResultSummary == nil || successSummary.ResultSummary == nil ||
		mappedRead.ResultSummary.ResultStatus != successSummary.ResultSummary.ResultStatus {
		t.Fatalf("result summary mismatch: detail=%#v list=%#v", mappedRead.ResultSummary, successSummary.ResultSummary)
	}
	if successSummary.Progress == nil || mappedRead.Progress == nil ||
		successSummary.Progress.TotalDispatches == nil || mappedRead.Progress.TotalDispatches == nil ||
		*successSummary.Progress.TotalDispatches != *mappedRead.Progress.TotalDispatches {
		t.Fatalf("dispatch count mismatch: detail=%#v list=%#v", mappedRead.Progress, successSummary.Progress)
	}
	if successSummary.ArtifactCount == nil || mappedRead.ArtifactRefs == nil ||
		*successSummary.ArtifactCount != len(*mappedRead.ArtifactRefs) {
		t.Fatalf("artifact count mismatch: detail=%d list=%v", len(*mappedRead.ArtifactRefs), successSummary.ArtifactCount)
	}
}

func TestFakeServiceConsumer_ProjectsFixtureThroughApisurfaceMappers(t *testing.T) {
	fixturesPath := filepath.Join("..", "..", "api", "testdata", "durable-session-contract-fixtures.json")
	service, err := factorysessionexecution.NewFakeServiceFromContractFixtures(fixturesPath)
	if err != nil {
		t.Fatalf("NewFakeServiceFromContractFixtures: %v", err)
	}

	scenario := findScenario(t, loadDurableFixtureCatalog(t), "petri-succeeded-one-dispatch")
	executionRequest, ok := scenario["executionRequest"].(map[string]any)
	if !ok {
		t.Fatal("missing executionRequest fixture")
	}
	request, err := factorysession.StartRequestFromAPI(decodeExecutionRequest(t, executionRequest))
	if err != nil {
		t.Fatalf("StartRequestFromAPI: %v", err)
	}

	started, err := service.StartAsync(context.Background(), request)
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	mappedStart := factorysession.AsyncStartResponseToAPI(started)
	if mappedStart.SessionId != "dur-sess-petri-success-001" {
		t.Fatalf("sessionId = %q", mappedStart.SessionId)
	}
	if mappedStart.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("status = %q, want SUCCEEDED", mappedStart.Status)
	}

	read, err := service.GetSession(context.Background(), mappedStart.SessionId)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	mappedRead := factorysession.SessionReadResponseToAPI(read)
	if mappedRead.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("read status = %q", mappedRead.Status)
	}

	dispatches, err := service.ListDispatches(context.Background(), mappedStart.SessionId)
	if err != nil {
		t.Fatalf("ListDispatches: %v", err)
	}
	mappedDispatches := factorysession.ListDispatchesResponseToAPI(dispatches)
	if len(mappedDispatches.Dispatches) != 1 || mappedDispatches.Dispatches[0].Id != "disp-petri-success-001" {
		t.Fatalf("dispatches = %#v", mappedDispatches.Dispatches)
	}

	result, err := service.GetResult(context.Background(), mappedStart.SessionId, factorysessionexecution.ResultRequest{
		Mode:             factorysessionexecution.ResultModeFinal,
		IncludeArtifacts: true,
	})
	if err != nil {
		t.Fatalf("GetResult: %v", err)
	}
	mappedResult := factorysession.ResultResponseToAPI(result)
	if mappedResult.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("resultStatus = %q, want FINAL", mappedResult.ResultStatus)
	}
	if mappedResult.ArtifactRefs == nil || len(*mappedResult.ArtifactRefs) != 1 || (*mappedResult.ArtifactRefs)[0].Id != "art-petri-final-001" {
		t.Fatalf("artifactRefs = %#v", mappedResult.ArtifactRefs)
	}

	events, err := service.ReadEvents(context.Background(), mappedStart.SessionId, factorysessionexecution.EventReconnectRequest{})
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	stream := factorysession.FactoryEventStreamFromReadResult(events)
	if len(stream.History) == 0 {
		t.Fatal("terminal events missing")
	}
}

func TestFakeServiceConsumer_ProjectsCanonicalFixtureEventsForRunningApprovalTerminal(t *testing.T) {
	fixturesPath := filepath.Join("..", "..", "api", "testdata", "durable-session-contract-fixtures.json")
	service, err := factorysessionexecution.NewFakeServiceFromContractFixtures(fixturesPath)
	if err != nil {
		t.Fatalf("NewFakeServiceFromContractFixtures: %v", err)
	}

	cases := []struct {
		scenarioID string
		eventCount int
	}{
		{"javascript-running-n-dispatch", 2},
		{"javascript-succeeded-two-dispatch", 3},
		{"javascript-awaiting-approval", 2},
	}
	for _, tc := range cases {
		t.Run(tc.scenarioID, func(t *testing.T) {
			scenario := findScenario(t, loadDurableFixtureCatalog(t), tc.scenarioID)
			executionRequest, ok := scenario["executionRequest"].(map[string]any)
			if !ok {
				t.Fatal("missing executionRequest fixture")
			}
			request, err := factorysession.StartRequestFromAPI(decodeExecutionRequest(t, executionRequest))
			if err != nil {
				t.Fatalf("StartRequestFromAPI: %v", err)
			}
			started, err := service.StartAsync(context.Background(), request)
			if err != nil {
				t.Fatalf("StartAsync: %v", err)
			}
			events, err := service.ReadEvents(context.Background(), started.SessionID, factorysessionexecution.EventReconnectRequest{})
			if err != nil {
				t.Fatalf("ReadEvents: %v", err)
			}
			mapped := factorysession.EventReadResponseToAPI(events)
			if len(mapped) != tc.eventCount {
				t.Fatalf("mapped events = %d, want %d", len(mapped), tc.eventCount)
			}
			for index, event := range mapped {
				if event.SchemaVersion != factoryapi.AgentFactoryEventV1 {
					t.Fatalf("event[%d] schemaVersion = %q, want agent-factory.event.v1", index, event.SchemaVersion)
				}
			}
		})
	}
}
