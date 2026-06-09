package factorysession_test

import (
	"context"
	"path/filepath"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
)

// pkgmaintcheck:ignore-cyclomatic-complexity this consumer test keeps fake-service projection assertions together across apisurface mappers.
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

	result, err := service.GetResult(context.Background(), mappedStart.SessionId, factorysessionexecution.ResultRequest{})
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
