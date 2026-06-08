package apicontract_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

type durableSessionContractFixtureCatalog struct {
	Scenarios        []durableSessionContractScenario       `json:"scenarios"`
	IdempotentReplay durableSessionIdempotentReplayFixture `json:"idempotentReplay"`
	ListResponse     map[string]any                         `json:"listResponse"`
}

type durableSessionContractScenario struct {
	ID               string                     `json:"id"`
	Tags             durableSessionContractTags `json:"tags"`
	ExecutionRequest map[string]any             `json:"executionRequest"`
	AsyncResponse    map[string]any             `json:"asyncResponse,omitempty"`
	SyncResponse     map[string]any             `json:"syncResponse,omitempty"`
	Session          map[string]any             `json:"session"`
	ListSummary      map[string]any             `json:"listSummary"`
	Dispatches       []map[string]any           `json:"dispatches"`
	DispatchDetail   map[string]any             `json:"dispatchDetail,omitempty"`
	Artifacts        []map[string]any           `json:"artifacts"`
	ArtifactDetail   map[string]any             `json:"artifactDetail,omitempty"`
	Result           map[string]any             `json:"result"`
	LifecycleControl map[string]any             `json:"lifecycleControl,omitempty"`
}

type durableSessionContractTags struct {
	Orchestrator  string `json:"orchestrator"`
	Status        string `json:"status"`
	DispatchCount string `json:"dispatchCount"`
	Outcome       string `json:"outcome"`
}

type durableSessionIdempotentReplayFixture struct {
	ExecutionRequest   map[string]any `json:"executionRequest"`
	AsyncResponse      map[string]any `json:"asyncResponse"`
	ReplayAsyncResponse map[string]any `json:"replayAsyncResponse"`
}

func TestOpenAPIContract_DurableSessionFixturesValidateAndRoundTrip(t *testing.T) {
	doc := loadValidatedOpenAPIContract(t)
	catalog := loadDurableSessionContractFixtureCatalog(t)

	seenOrchestrators := map[string]struct{}{}
	seenDispatchCounts := map[string]struct{}{}
	seenOutcomes := map[string]struct{}{}

	for _, scenario := range catalog.Scenarios {
		t.Run(scenario.ID, func(t *testing.T) {
			assertDurableSessionScenarioFixture(t, doc, scenario)
			seenOrchestrators[scenario.Tags.Orchestrator] = struct{}{}
			seenDispatchCounts[scenario.Tags.DispatchCount] = struct{}{}
			seenOutcomes[scenario.Tags.Outcome] = struct{}{}
		})
	}

	assertDurableSessionFixtureCoverage(t, seenOrchestrators, seenDispatchCounts, seenOutcomes)
	assertDurableSessionIdempotentReplayFixture(t, doc, catalog.IdempotentReplay)
	assertDurableSessionListFixture(t, doc, catalog.ListResponse)
}

func assertDurableSessionScenarioFixture(t *testing.T, doc *openapi3.T, scenario durableSessionContractScenario) {
	t.Helper()

	assertOpenAPIFixtureValidates(t, doc, "FactorySessionExecutionRequest", scenario.ExecutionRequest)
	assertGeneratedFixtureRoundTrip(t, scenario.ExecutionRequest, "FactorySessionExecutionRequest", func(raw []byte) {
		var value factoryapi.FactorySessionExecutionRequest
		decodeRoundTripJSON(t, raw, &value, scenario.ID+" execution request")
	})

	if scenario.AsyncResponse != nil {
		assertOpenAPIFixtureValidates(t, doc, "FactorySessionExecutionResponse", scenario.AsyncResponse)
		assertGeneratedFixtureRoundTrip(t, scenario.AsyncResponse, "FactorySessionExecutionResponse", func(raw []byte) {
			var value factoryapi.FactorySessionExecutionResponse
			decodeRoundTripJSON(t, raw, &value, scenario.ID+" async response")
			if value.SessionId == "" {
				t.Fatalf("%s async response sessionId is empty", scenario.ID)
			}
		})
	}

	if scenario.SyncResponse != nil {
		assertOpenAPIFixtureValidates(t, doc, "FactorySessionSyncExecutionResponse", scenario.SyncResponse)
		assertGeneratedFixtureRoundTrip(t, scenario.SyncResponse, "FactorySessionSyncExecutionResponse", func(raw []byte) {
			var value factoryapi.FactorySessionSyncExecutionResponse
			decodeRoundTripJSON(t, raw, &value, scenario.ID+" sync response")
		})
	}

	assertOpenAPIFixtureValidates(t, doc, "FactorySessionDurableReadModel", scenario.Session)
	assertGeneratedFixtureRoundTrip(t, scenario.Session, "FactorySessionDurableReadModel", func(raw []byte) {
		var value factoryapi.FactorySessionDurableReadModel
		decodeRoundTripJSON(t, raw, &value, scenario.ID+" session")
		assertDurableSessionGetResponseRoundTrip(t, value)
	})

	assertOpenAPIFixtureValidates(t, doc, "FactorySessionDurableSummary", scenario.ListSummary)
	assertGeneratedFixtureRoundTrip(t, scenario.ListSummary, "FactorySessionDurableSummary", func(raw []byte) {
		var value factoryapi.FactorySessionDurableSummary
		decodeRoundTripJSON(t, raw, &value, scenario.ID+" list summary")
	})

	assertDurableSessionScenarioDispatchArtifactFixtures(t, doc, scenario)

	assertOpenAPIFixtureValidates(t, doc, "FactorySessionResult", scenario.Result)
	assertGeneratedFixtureRoundTrip(t, scenario.Result, "FactorySessionResult", func(raw []byte) {
		var value factoryapi.FactorySessionResult
		decodeRoundTripJSON(t, raw, &value, scenario.ID+" result")
		if value.SessionId != scenario.Session["sessionId"] {
			t.Fatalf("%s result sessionId = %q, want %q", scenario.ID, value.SessionId, scenario.Session["sessionId"])
		}
	})

	if scenario.LifecycleControl != nil {
		assertOpenAPIFixtureValidates(t, doc, "FactorySessionLifecycleControlResponse", scenario.LifecycleControl)
		assertGeneratedFixtureRoundTrip(t, scenario.LifecycleControl, "FactorySessionLifecycleControlResponse", func(raw []byte) {
			var value factoryapi.FactorySessionLifecycleControlResponse
			decodeRoundTripJSON(t, raw, &value, scenario.ID+" lifecycle control")
		})
	}

	assertDurableSessionFixtureOmitsHostPaths(t, scenario)
}

func assertDurableSessionScenarioDispatchArtifactFixtures(t *testing.T, doc *openapi3.T, scenario durableSessionContractScenario) {
	t.Helper()

	if len(scenario.Dispatches) > 0 {
		listResponse := map[string]any{
			"sessionId":  scenario.Session["sessionId"],
			"dispatches": scenario.Dispatches,
		}
		assertOpenAPIFixtureValidates(t, doc, "ListFactorySessionDispatchesResponse", listResponse)
		assertGeneratedFixtureRoundTrip(t, listResponse, "ListFactorySessionDispatchesResponse", func(raw []byte) {
			var value factoryapi.ListFactorySessionDispatchesResponse
			decodeRoundTripJSON(t, raw, &value, scenario.ID+" dispatch list")
		})
	}

	if scenario.DispatchDetail != nil {
		assertOpenAPIFixtureValidates(t, doc, "FactoryDispatch", scenario.DispatchDetail)
		assertGeneratedFixtureRoundTrip(t, scenario.DispatchDetail, "FactoryDispatch", func(raw []byte) {
			var value factoryapi.FactoryDispatch
			decodeRoundTripJSON(t, raw, &value, scenario.ID+" dispatch detail")
		})
	}

	if len(scenario.Artifacts) > 0 {
		listResponse := map[string]any{
			"sessionId": scenario.Session["sessionId"],
			"artifacts": scenario.Artifacts,
		}
		assertOpenAPIFixtureValidates(t, doc, "ListFactorySessionArtifactsResponse", listResponse)
		assertGeneratedFixtureRoundTrip(t, listResponse, "ListFactorySessionArtifactsResponse", func(raw []byte) {
			var value factoryapi.ListFactorySessionArtifactsResponse
			decodeRoundTripJSON(t, raw, &value, scenario.ID+" artifact list")
		})
	}

	if scenario.ArtifactDetail != nil {
		assertOpenAPIFixtureValidates(t, doc, "FactorySessionArtifactDetail", scenario.ArtifactDetail)
		assertGeneratedFixtureRoundTrip(t, scenario.ArtifactDetail, "FactorySessionArtifactDetail", func(raw []byte) {
			var value factoryapi.FactorySessionArtifactDetail
			decodeRoundTripJSON(t, raw, &value, scenario.ID+" artifact detail")
			assertArtifactRetrievalRefSafe(t, value.ContentRef)
		})
	}
}

func assertDurableSessionIdempotentReplayFixture(t *testing.T, doc *openapi3.T, fixture durableSessionIdempotentReplayFixture) {
	t.Helper()

	assertOpenAPIFixtureValidates(t, doc, "FactorySessionExecutionRequest", fixture.ExecutionRequest)
	assertOpenAPIFixtureValidates(t, doc, "FactorySessionExecutionResponse", fixture.AsyncResponse)
	assertOpenAPIFixtureValidates(t, doc, "FactorySessionExecutionResponse", fixture.ReplayAsyncResponse)

	var initial factoryapi.FactorySessionExecutionResponse
	var replay factoryapi.FactorySessionExecutionResponse
	assertGeneratedFixtureRoundTrip(t, fixture.AsyncResponse, "FactorySessionExecutionResponse", func(raw []byte) {
		decodeRoundTripJSON(t, raw, &initial, "idempotent async response")
	})
	assertGeneratedFixtureRoundTrip(t, fixture.ReplayAsyncResponse, "FactorySessionExecutionResponse", func(raw []byte) {
		decodeRoundTripJSON(t, raw, &replay, "idempotent replay async response")
	})

	if initial.SessionId != replay.SessionId {
		t.Fatalf("idempotent replay sessionId = %q, want %q", replay.SessionId, initial.SessionId)
	}
	if initial.EffectivePolicyHash == nil || replay.EffectivePolicyHash == nil || *initial.EffectivePolicyHash != *replay.EffectivePolicyHash {
		t.Fatalf("idempotent replay effectivePolicyHash = %#v, want %#v", replay.EffectivePolicyHash, initial.EffectivePolicyHash)
	}
}

func assertDurableSessionListFixture(t *testing.T, doc *openapi3.T, listResponse map[string]any) {
	t.Helper()

	assertOpenAPIFixtureValidates(t, doc, "ListFactorySessionsResponse", listResponse)
	assertGeneratedFixtureRoundTrip(t, listResponse, "ListFactorySessionsResponse", func(raw []byte) {
		var value factoryapi.ListFactorySessionsResponse
		decodeRoundTripJSON(t, raw, &value, "durable session list response")
		if len(value.Sessions) != 0 {
			t.Fatalf("durable list fixture live sessions = %#v, want empty slice", value.Sessions)
		}
		if value.DurableSessions == nil || len(*value.DurableSessions) == 0 {
			t.Fatal("durable list fixture durableSessions is empty")
		}
	})
}

func assertDurableSessionFixtureCoverage(
	t *testing.T,
	seenOrchestrators map[string]struct{},
	seenDispatchCounts map[string]struct{},
	seenOutcomes map[string]struct{},
) {
	t.Helper()

	for _, orchestrator := range []string{"PETRI", "JAVASCRIPT"} {
		if _, ok := seenOrchestrators[orchestrator]; !ok {
			t.Fatalf("durable session fixtures missing orchestrator %q", orchestrator)
		}
	}
	for _, dispatchCount := range []string{"ONE", "TWO", "N"} {
		if _, ok := seenDispatchCounts[dispatchCount]; !ok {
			t.Fatalf("durable session fixtures missing dispatchCount %q", dispatchCount)
		}
	}
	for _, outcome := range []string{
		"running",
		"paused",
		"failed-with-partial",
		"timed-out",
		"canceled",
		"succeeded",
		"unsupported-runner",
		"missing-source",
	} {
		if _, ok := seenOutcomes[outcome]; !ok {
			t.Fatalf("durable session fixtures missing outcome %q", outcome)
		}
	}
}

func assertDurableSessionGetResponseRoundTrip(t *testing.T, session factoryapi.FactorySessionDurableReadModel) {
	t.Helper()

	var response factoryapi.FactorySessionGetResponse
	if err := response.FromFactorySessionDurableReadModel(session); err != nil {
		t.Fatalf("encode FactorySessionGetResponse durable union: %v", err)
	}

	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal FactorySessionGetResponse durable union: %v", err)
	}

	var roundTripped factoryapi.FactorySessionGetResponse
	decodeRoundTripJSON(t, encoded, &roundTripped, "FactorySessionGetResponse durable union")

	durable, err := roundTripped.AsFactorySessionDurableReadModel()
	if err != nil {
		t.Fatalf("decode FactorySessionGetResponse durable union: %v", err)
	}
	if durable.SessionId != session.SessionId || durable.Status != session.Status {
		t.Fatalf("round-tripped durable session = %#v, want sessionId=%q status=%q", durable, session.SessionId, session.Status)
	}
}

func assertArtifactRetrievalRefSafe(t *testing.T, contentRef *factoryapi.FactorySessionArtifactRetrievalRef) {
	t.Helper()
	if contentRef == nil {
		return
	}
	if strings.Contains(contentRef.Href, "://") && !strings.HasPrefix(contentRef.Href, "/") {
		t.Fatalf("artifact contentRef href must be API-relative, got %q", contentRef.Href)
	}
}

func assertDurableSessionFixtureOmitsHostPaths(t *testing.T, scenario durableSessionContractScenario) {
	t.Helper()

	encoded, err := json.Marshal(scenario)
	if err != nil {
		t.Fatalf("marshal scenario %s for host-path scan: %v", scenario.ID, err)
	}
	text := string(encoded)
	for _, forbidden := range []string{"/Users/", "file://", "C:\\"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("scenario %s fixture contains forbidden host path fragment %q", scenario.ID, forbidden)
		}
	}
}

func assertOpenAPIFixtureValidates(t *testing.T, doc *openapi3.T, schemaName string, payload map[string]any) {
	t.Helper()

	schemaRef, ok := doc.Components.Schemas[schemaName]
	if !ok || schemaRef.Value == nil {
		t.Fatalf("openapi schema %s is missing", schemaName)
	}
	if err := schemaRef.Value.VisitJSON(payload); err != nil {
		t.Fatalf("%s fixture should validate: %v", schemaName, err)
	}
}

func assertGeneratedFixtureRoundTrip(t *testing.T, payload map[string]any, label string, assertDecoded func(raw []byte)) {
	t.Helper()

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal %s fixture map: %v", label, err)
	}
	assertDecoded(raw)

	var roundTripMap map[string]any
	decodeRoundTripJSON(t, raw, &roundTripMap, label+" map")
	rawAgain, err := json.Marshal(roundTripMap)
	if err != nil {
		t.Fatalf("marshal round-tripped %s map: %v", label, err)
	}
	assertDecoded(rawAgain)
}

func loadDurableSessionContractFixtureCatalog(t *testing.T) durableSessionContractFixtureCatalog {
	t.Helper()

	fixtureBytes, err := os.ReadFile("../testdata/durable-session-contract-fixtures.json")
	if err != nil {
		t.Fatalf("read durable session contract fixtures: %v", err)
	}

	var catalog durableSessionContractFixtureCatalog
	if err := json.Unmarshal(fixtureBytes, &catalog); err != nil {
		t.Fatalf("parse durable session contract fixtures: %v", err)
	}
	if len(catalog.Scenarios) == 0 {
		t.Fatal("durable session contract fixtures must include at least one scenario")
	}
	return catalog
}
