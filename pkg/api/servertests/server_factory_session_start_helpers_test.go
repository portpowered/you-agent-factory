package apiserver_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	api "github.com/portpowered/infinite-you/pkg/api"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface/factorysession"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"go.uber.org/zap"
)

type durableStartFixtureCatalog struct {
	Scenarios        []durableStartFixtureScenario       `json:"scenarios"`
	IdempotentReplay durableStartIdempotentReplayFixture `json:"idempotentReplay"`
}

type durableStartFixtureScenario struct {
	ID               string         `json:"id"`
	ExecutionRequest map[string]any `json:"executionRequest"`
	AsyncResponse    map[string]any `json:"asyncResponse"`
	SyncResponse     map[string]any `json:"syncResponse"`
}

type durableStartIdempotentReplayFixture struct {
	ExecutionRequest    map[string]any `json:"executionRequest"`
	AsyncResponse       map[string]any `json:"asyncResponse"`
	ReplayAsyncResponse map[string]any `json:"replayAsyncResponse"`
}

func newDurableSessionAPITestServer(t *testing.T) *api.Server {
	t.Helper()
	fixturesPath := filepath.Join("..", "testdata", "durable-session-contract-fixtures.json")
	service, err := factorysessionexecution.NewFakeServiceFromContractFixtures(fixturesPath)
	if err != nil {
		t.Fatalf("NewFakeServiceFromContractFixtures: %v", err)
	}
	logger, _ := zap.NewDevelopment()
	return api.NewServerWithOptions(&testutil.MockFactory{}, 8080, logger, api.ServerOptions{
		DurableSessionExecution: factorysession.NewExecutionAPI(service),
	})
}

func loadDurableStartFixtureCatalog(t *testing.T) durableStartFixtureCatalog {
	t.Helper()
	fixturesPath := filepath.Join("..", "testdata", "durable-session-contract-fixtures.json")
	raw, err := os.ReadFile(fixturesPath)
	if err != nil {
		t.Fatalf("read fixtures: %v", err)
	}
	var catalog durableStartFixtureCatalog
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatalf("decode fixtures: %v", err)
	}
	return catalog
}

func durableStartScenarioByID(t *testing.T, scenarioID string) durableStartFixtureScenario {
	t.Helper()
	catalog := loadDurableStartFixtureCatalog(t)
	for _, scenario := range catalog.Scenarios {
		if scenario.ID == scenarioID {
			return scenario
		}
	}
	t.Fatalf("scenario %q not found", scenarioID)
	return durableStartFixtureScenario{}
}

func decodeDurableExecutionRequest(t *testing.T, fixture map[string]any) factoryapi.FactorySessionExecutionRequest {
	t.Helper()
	encoded, err := json.Marshal(fixture)
	if err != nil {
		t.Fatalf("marshal execution request: %v", err)
	}
	var request factoryapi.FactorySessionExecutionRequest
	if err := json.Unmarshal(encoded, &request); err != nil {
		t.Fatalf("decode execution request: %v", err)
	}
	return request
}

func loadOpenAPIContractForServerTests(t *testing.T) *openapi3.T {
	t.Helper()
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile("../../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("load openapi contract: %v", err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("validate openapi contract: %v", err)
	}
	return doc
}

func assertResponseValidatesOpenAPISchema(t *testing.T, doc *openapi3.T, schemaName string, payload any) {
	t.Helper()
	schemaRef, ok := doc.Components.Schemas[schemaName]
	if !ok || schemaRef.Value == nil {
		t.Fatalf("openapi schema %s is missing", schemaName)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal %s payload: %v", schemaName, err)
	}
	var decoded any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decode %s payload: %v", schemaName, err)
	}
	if err := schemaRef.Value.VisitJSON(decoded); err != nil {
		t.Fatalf("%s response should validate against OpenAPI: %v", schemaName, err)
	}
}
