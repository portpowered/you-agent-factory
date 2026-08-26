package apicontract_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type durableSessionContractFixtureCatalog struct {
	Scenarios        []durableSessionContractScenario      `json:"scenarios"`
	IdempotentReplay durableSessionIdempotentReplayFixture `json:"idempotentReplay"`
	ListResponse     map[string]any                        `json:"listResponse"`
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
	Events           []map[string]any           `json:"events,omitempty"`
	LifecycleControl map[string]any             `json:"lifecycleControl,omitempty"`
}

type durableSessionContractTags struct {
	Orchestrator  string `json:"orchestrator"`
	Status        string `json:"status"`
	DispatchCount string `json:"dispatchCount"`
	Outcome       string `json:"outcome"`
}

type durableSessionIdempotentReplayFixture struct {
	ExecutionRequest    map[string]any `json:"executionRequest"`
	AsyncResponse       map[string]any `json:"asyncResponse"`
	ReplayAsyncResponse map[string]any `json:"replayAsyncResponse"`
}

func TestGeneratedDurableSessionDispatchArtifactContracts_NotFoundJSON(t *testing.T) {
	notFound := factoryapi.ErrorResponse{
		Code:    factoryapi.ErrorResponseCodeNOTFOUND,
		Family:  factoryapi.ErrorFamilyNotFound,
		Message: "factory session not found",
	}
	if notFound.Code != factoryapi.ErrorResponseCodeNOTFOUND || notFound.Family != factoryapi.ErrorFamilyNotFound {
		t.Fatalf("generated not-found contract = %#v, want code %q and family %q", notFound, factoryapi.ErrorResponseCodeNOTFOUND, factoryapi.ErrorFamilyNotFound)
	}

	encoded, err := json.Marshal(notFound)
	if err != nil {
		t.Fatalf("marshal generated ErrorResponse: %v", err)
	}
	if !strings.Contains(string(encoded), `"code":"NOT_FOUND"`) {
		t.Fatalf("generated ErrorResponse JSON missing NOT_FOUND code: %s", encoded)
	}
	if !strings.Contains(string(encoded), `"family":"NOT_FOUND"`) {
		t.Fatalf("generated ErrorResponse JSON missing NOT_FOUND family: %s", encoded)
	}
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

	assertDurableSessionScenarioEventFixtures(t, doc, scenario)
	assertDurableSessionFixtureInspectionEventLinksAreSessionScoped(t, scenario)

	assertDurableSessionFixtureOmitsHostPaths(t, scenario)
}

func assertDurableSessionScenarioDispatchArtifactFixtures(t *testing.T, doc *openapi3.T, scenario durableSessionContractScenario) {
	t.Helper()

	dispatchListResponse := map[string]any{
		"sessionId":  scenario.Session["sessionId"],
		"dispatches": scenario.Dispatches,
	}
	assertOpenAPIFixtureValidates(t, doc, "ListFactorySessionDispatchesResponse", dispatchListResponse)
	assertGeneratedFixtureRoundTrip(t, dispatchListResponse, "ListFactorySessionDispatchesResponse", func(raw []byte) {
		var value factoryapi.ListFactorySessionDispatchesResponse
		decodeRoundTripJSON(t, raw, &value, scenario.ID+" dispatch list")
	})

	if scenario.DispatchDetail != nil {
		assertOpenAPIFixtureValidates(t, doc, "FactoryDispatch", scenario.DispatchDetail)
		assertGeneratedFixtureRoundTrip(t, scenario.DispatchDetail, "FactoryDispatch", func(raw []byte) {
			var value factoryapi.FactoryDispatch
			decodeRoundTripJSON(t, raw, &value, scenario.ID+" dispatch detail")
		})
	}

	artifactListResponse := map[string]any{
		"sessionId": scenario.Session["sessionId"],
		"artifacts": scenario.Artifacts,
	}
	assertOpenAPIFixtureValidates(t, doc, "ListFactorySessionArtifactsResponse", artifactListResponse)
	assertGeneratedFixtureRoundTrip(t, artifactListResponse, "ListFactorySessionArtifactsResponse", func(raw []byte) {
		var value factoryapi.ListFactorySessionArtifactsResponse
		decodeRoundTripJSON(t, raw, &value, scenario.ID+" artifact list")
	})

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
		"awaiting-approval",
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
func assertDurableExecutionSurfaceSchemas(t *testing.T, schemas map[string]any, paths map[string]any) {
	t.Helper()

	asyncOperation := pathOperation(t, paths, "/factory-sessions/async", "post")
	if got, _ := asyncOperation["operationId"].(string); got != "startDurableFactorySessionAsync" {
		t.Fatalf("paths./factory-sessions/async.post.operationId = %q, want startDurableFactorySessionAsync", got)
	}
	assertRequestSchemaRef(t, asyncOperation, "#/components/schemas/FactorySessionExecutionRequest")
	assertResponseSchemaRef(t, asyncOperation, "202", "#/components/schemas/FactorySessionExecutionResponse")
	assertResponseRef(t, asyncOperation, "400", "#/components/responses/BadRequest")
	assertResponseRef(t, asyncOperation, "409", "#/components/responses/ExecutionRequestIdConflict")

	syncOperation := pathOperation(t, paths, "/factory-sessions/sync", "post")
	if got, _ := syncOperation["operationId"].(string); got != "startDurableFactorySessionSync" {
		t.Fatalf("paths./factory-sessions/sync.post.operationId = %q, want startDurableFactorySessionSync", got)
	}
	assertRequestSchemaRef(t, syncOperation, "#/components/schemas/FactorySessionExecutionRequest")
	assertResponseSchemaRef(t, syncOperation, "200", "#/components/schemas/FactorySessionSyncExecutionResponse")
	assertResponseRef(t, syncOperation, "400", "#/components/responses/BadRequest")
	assertResponseRef(t, syncOperation, "409", "#/components/responses/ExecutionRequestIdConflict")

	requestSchema := schemaObject(t, schemas, "FactorySessionExecutionRequest")
	assertRequiredFields(t, requestSchema, "requestId", "source")
	requestProperties := schemaProperties(t, requestSchema, "FactorySessionExecutionRequest")
	assertPropertyRef(t, requestProperties, "source", "#/components/schemas/FactorySessionExecutionSource")
	assertPropertyRef(t, requestProperties, "orchestrator", "#/components/schemas/FactoryOrchestrator")
	assertPropertyRef(t, requestProperties, "requestedPolicy", "#/components/schemas/FactorySessionRequestedPolicy")
	assertPropertyRef(t, requestProperties, "wait", "#/components/schemas/FactorySessionExecutionWaitOptions")
	assertSchemaPropertiesPresent(t, requestProperties, "FactorySessionExecutionRequest", "requestId", "source", "args", "orchestrator", "requestedPolicy", "wait")

	sourceSchema := schemaObject(t, schemas, "FactorySessionExecutionSource")
	assertRequiredFields(t, sourceSchema, "kind")
	sourceProperties := schemaProperties(t, sourceSchema, "FactorySessionExecutionSource")
	assertPropertyRef(t, sourceProperties, "kind", "#/components/schemas/FactorySessionExecutionSourceKind")
	assertSchemaPropertiesPresent(t, sourceProperties, "FactorySessionExecutionSource", "factoryId", "factoryInline", "workflowFile", "workflowName", "inlineWorkflow")
	assertEnumValues(t, schemaObject(t, schemas, "FactorySessionExecutionSourceKind"), "FactorySessionExecutionSourceKind", []string{
		"FACTORY_ID", "FACTORY_INLINE", "WORKFLOW_FILE", "WORKFLOW_NAME", "INLINE_WORKFLOW",
	})

	asyncResponseSchema := schemaObject(t, schemas, "FactorySessionExecutionResponse")
	assertRequiredFields(t, asyncResponseSchema, "sessionId", "status", "orchestratorKind", "resolvedSource")
	asyncResponseProperties := schemaProperties(t, asyncResponseSchema, "FactorySessionExecutionResponse")
	assertPropertyRef(t, asyncResponseProperties, "status", "#/components/schemas/FactorySessionDurableLifecycleStatus")
	assertPropertyRef(t, asyncResponseProperties, "orchestratorKind", "#/components/schemas/FactoryOrchestratorKind")
	assertPropertyRef(t, asyncResponseProperties, "resolvedSource", "#/components/schemas/FactorySessionResolvedSourceIdentity")
	assertPropertyRef(t, asyncResponseProperties, "links", "#/components/schemas/FactorySessionExecutionLinks")
	assertSchemaPropertiesPresent(t, asyncResponseProperties, "FactorySessionExecutionResponse", "sessionId", "status", "orchestratorKind", "dialect", "resolvedSource", "sourceHash", "requestedPolicy", "effectivePolicy", "effectivePolicyHash", "links")

	syncResponseSchema := schemaObject(t, schemas, "FactorySessionSyncExecutionResponse")
	assertRequiredFields(t, syncResponseSchema, "sessionId", "status", "orchestratorKind", "resolvedSource", "syncOutcome")
	syncResponseProperties := schemaProperties(t, syncResponseSchema, "FactorySessionSyncExecutionResponse")
	assertPropertyRef(t, syncResponseProperties, "syncOutcome", "#/components/schemas/FactorySessionSyncExecutionOutcome")
	assertPropertyRef(t, syncResponseProperties, "result", "#/components/schemas/FactorySessionResult")
	assertSchemaPropertiesPresent(t, syncResponseProperties, "FactorySessionSyncExecutionResponse", "requestedPolicy", "effectivePolicy", "effectivePolicyHash", "syncOutcome", "result", "timedOut", "sessionCanceledByTimeout")
	assertEnumValues(t, schemaObject(t, schemas, "FactorySessionSyncExecutionOutcome"), "FactorySessionSyncExecutionOutcome", []string{"COMPLETED", "TIMED_OUT", "STILL_RUNNING"})

	waitOptions := schemaObject(t, schemas, "FactorySessionExecutionWaitOptions")
	waitProperties := schemaProperties(t, waitOptions, "FactorySessionExecutionWaitOptions")
	assertSchemaPropertiesPresent(t, waitProperties, "FactorySessionExecutionWaitOptions", "timeoutMillis", "cancelOnTimeout")

	openFactorySession := pathOperation(t, paths, "/factory-sessions", "post")
	openDescription, _ := openFactorySession["description"].(string)
	if !strings.Contains(strings.ToLower(openDescription), "not the primary durable") {
		t.Fatalf("paths./factory-sessions.post.description must document live-session compatibility, got %q", openDescription)
	}
	invokeOperation := pathOperation(t, paths, "/factory-sessions/{session_id}/invocations", "post")
	invokeDescription, _ := invokeOperation["description"].(string)
	if !strings.Contains(strings.ToLower(invokeDescription), "not the primary durable") {
		t.Fatalf("paths./factory-sessions/{session_id}/invocations.post.description must document live-session compatibility, got %q", invokeDescription)
	}
}

func assertDurableSourceResolutionAndIdempotencySurfaceSchemas(t *testing.T, schemas map[string]any, paths map[string]any) {
	t.Helper()

	asyncOperation := pathOperation(t, paths, "/factory-sessions/async", "post")
	asyncDescription, _ := asyncOperation["description"].(string)
	for _, fragment := range []string{
		".claude/workflows",
		"~/.you-agent-factory/workflows",
		"package-relative workflow directories",
		"built-in/global JavaScript factories",
		"explicit factory lookup",
		"EXECUTION_REQUEST_ID_CONFLICT",
	} {
		if !strings.Contains(asyncDescription, fragment) {
			t.Fatalf("paths./factory-sessions/async.post.description must document %q, got %q", fragment, asyncDescription)
		}
	}

	syncOperation := pathOperation(t, paths, "/factory-sessions/sync", "post")
	syncDescription, _ := syncOperation["description"].(string)
	if !strings.Contains(strings.ToLower(syncDescription), "idempotency") {
		t.Fatalf("paths./factory-sessions/sync.post.description must document requestId idempotency, got %q", syncDescription)
	}

	sourceSchema := schemaObject(t, schemas, "FactorySessionExecutionSource")
	sourceDescription, _ := sourceSchema["description"].(string)
	if !strings.Contains(sourceDescription, "FactorySessionWorkflowSourceResolutionOrder") {
		t.Fatalf("components.schemas.FactorySessionExecutionSource.description must reference FactorySessionWorkflowSourceResolutionOrder")
	}

	resolvedSourceSchema := schemaObject(t, schemas, "FactorySessionResolvedSourceIdentity")
	resolvedSourceProperties := schemaProperties(t, resolvedSourceSchema, "FactorySessionResolvedSourceIdentity")
	assertPropertyRef(t, resolvedSourceProperties, "kind", "#/components/schemas/FactorySessionExecutionSourceKind")
	assertArrayItemRef(t, resolvedSourceProperties, "resolutionOrder", "#/components/schemas/FactorySessionWorkflowSourceResolutionOrder")
	assertSchemaPropertiesPresent(t, resolvedSourceProperties, "FactorySessionResolvedSourceIdentity",
		"kind", "sourceRef", "sourceHash", "dialect", "metadata", "resolutionOrder")

	assertEnumValues(t, schemaObject(t, schemas, "FactorySessionWorkflowSourceResolutionOrder"), "FactorySessionWorkflowSourceResolutionOrder", []string{
		"PROJECT_CLAUDE_WORKFLOWS",
		"USER_YOU_AGENT_FACTORY_WORKFLOWS",
		"PACKAGE_RELATIVE_WORKFLOW_DIRECTORIES",
		"BUILTIN_GLOBAL_JAVASCRIPT_FACTORIES",
		"EXPLICIT_FACTORY_LOOKUP",
	})

	requestedPolicySchema := schemaObject(t, schemas, "FactorySessionRequestedPolicy")
	requestedPolicyProperties := schemaProperties(t, requestedPolicySchema, "FactorySessionRequestedPolicy")
	assertSchemaPropertiesPresent(t, requestedPolicyProperties, "FactorySessionRequestedPolicy", "policyHash")

	effectivePolicySchema := schemaObject(t, schemas, "FactorySessionEffectivePolicy")
	effectivePolicyProperties := schemaProperties(t, effectivePolicySchema, "FactorySessionEffectivePolicy")
	assertSchemaPropertiesPresent(t, effectivePolicyProperties, "FactorySessionEffectivePolicy", "policyHash")

	requestSchema := schemaObject(t, schemas, "FactorySessionExecutionRequest")
	requestDescription, _ := requestSchema["description"].(string)
	if !strings.Contains(strings.ToLower(requestDescription), "idempotency") {
		t.Fatalf("components.schemas.FactorySessionExecutionRequest.description must document idempotency normalization")
	}
	requestProperties := schemaProperties(t, requestSchema, "FactorySessionExecutionRequest")
	assertPropertyRef(t, requestProperties, "requestedPolicy", "#/components/schemas/FactorySessionRequestedPolicy")

	asyncResponseProperties := schemaProperties(t, schemaObject(t, schemas, "FactorySessionExecutionResponse"), "FactorySessionExecutionResponse")
	assertPropertyRef(t, asyncResponseProperties, "requestedPolicy", "#/components/schemas/FactorySessionRequestedPolicy")
	assertPropertyRef(t, asyncResponseProperties, "effectivePolicy", "#/components/schemas/FactorySessionEffectivePolicy")

	errorSchema := schemaObject(t, schemas, "ErrorResponse")
	codeProperty, ok := schemaProperties(t, errorSchema, "ErrorResponse")["code"].(map[string]any)
	if !ok {
		t.Fatalf("components.schemas.ErrorResponse.properties.code is missing")
	}
	codeEnum, ok := codeProperty["enum"].([]any)
	if !ok || !containsString(codeEnum, "EXECUTION_REQUEST_ID_CONFLICT") {
		t.Fatalf("components.schemas.ErrorResponse.properties.code.enum is missing EXECUTION_REQUEST_ID_CONFLICT")
	}
}

func assertDurableSessionReadSurfaceSchemas(t *testing.T, schemas map[string]any, paths map[string]any) {
	t.Helper()

	listOperation := pathOperation(t, paths, "/factory-sessions", "get")
	if got, _ := listOperation["operationId"].(string); got != "listFactorySessions" {
		t.Fatalf("paths./factory-sessions.get.operationId = %q, want listFactorySessions", got)
	}
	assertResponseSchemaRef(t, listOperation, "200", "#/components/schemas/ListFactorySessionsResponse")
	assertResponseRef(t, listOperation, "400", "#/components/responses/BadRequest")
	parameters, ok := listOperation["parameters"].([]any)
	if !ok {
		t.Fatalf("paths./factory-sessions.get.parameters is missing")
	}
	assertParameterRef(t, parameters, "#/components/parameters/FactorySessionListScope")

	listResponseSchema := schemaObject(t, schemas, "ListFactorySessionsResponse")
	assertRequiredFields(t, listResponseSchema, "sessions")
	listResponseProperties := schemaProperties(t, listResponseSchema, "ListFactorySessionsResponse")
	assertPropertyRef(t, listResponseProperties, "scope", "#/components/schemas/FactorySessionListScope")
	assertArrayItemRef(t, listResponseProperties, "sessions", "#/components/schemas/FactorySessionSummary")
	assertArrayItemRef(t, listResponseProperties, "durableSessions", "#/components/schemas/FactorySessionDurableSummary")
	assertArrayItemRef(t, listResponseProperties, "recordedSessions", "#/components/schemas/FactorySessionRecordedSummary")
	assertSchemaPropertiesPresent(t, listResponseProperties, "ListFactorySessionsResponse", "scope", "sessions", "durableSessions", "recordedSessions")
	assertEnumValues(t, schemaObject(t, schemas, "FactorySessionListScope"), "FactorySessionListScope", []string{"live", "persisted", "history", "all"})

	getOperation := pathOperation(t, paths, "/factory-sessions/{session_id}", "get")
	if got, _ := getOperation["operationId"].(string); got != "getFactorySession" {
		t.Fatalf("paths./factory-sessions/{session_id}.get.operationId = %q, want getFactorySession", got)
	}
	assertResponseSchemaRef(t, getOperation, "200", "#/components/schemas/FactorySessionGetResponse")
	assertResponseRef(t, getOperation, "404", "#/components/responses/NotFound")

	getResponseSchema := schemaObject(t, schemas, "FactorySessionGetResponse")
	assertSchemaOneOfRefs(t, getResponseSchema, "FactorySessionGetResponse", []string{
		"#/components/schemas/FactorySession",
		"#/components/schemas/FactorySessionDurableReadModel",
	})

	durableReadModel := schemaObject(t, schemas, "FactorySessionDurableReadModel")
	assertRequiredFields(t, durableReadModel, "sessionId", "status", "orchestratorKind", "resolvedSource")
	durableReadModelProperties := schemaProperties(t, durableReadModel, "FactorySessionDurableReadModel")
	assertPropertyRef(t, durableReadModelProperties, "status", "#/components/schemas/FactorySessionDurableLifecycleStatus")
	assertPropertyRef(t, durableReadModelProperties, "orchestratorKind", "#/components/schemas/FactoryOrchestratorKind")
	assertPropertyRef(t, durableReadModelProperties, "resolvedSource", "#/components/schemas/FactorySessionResolvedSourceIdentity")
	assertArrayItemRef(t, durableReadModelProperties, "phaseSummaries", "#/components/schemas/FactorySessionDurablePhaseSummary")
	assertPropertyRef(t, durableReadModelProperties, "latestCheckpoint", "#/components/schemas/FactorySessionCheckpointRef")
	assertPropertyRef(t, durableReadModelProperties, "progress", "#/components/schemas/FactorySessionDurableProgressCounts")
	assertPropertyRef(t, durableReadModelProperties, "budgets", "#/components/schemas/FactorySessionBudgets")
	assertPropertyRef(t, durableReadModelProperties, "usage", "#/components/schemas/FactorySessionUsage")
	assertArrayItemRef(t, durableReadModelProperties, "artifactRefs", "#/components/schemas/FactoryArtifactRef")
	assertPropertyRef(t, durableReadModelProperties, "resultSummary", "#/components/schemas/FactorySessionDurableResultSummary")
	assertPropertyRef(t, durableReadModelProperties, "failureDetail", "#/components/schemas/FailureDetail")
	assertPropertyRef(t, durableReadModelProperties, "lifecycle", "#/components/schemas/FactorySessionDurableLifecycleTimestamps")
	assertPropertyRef(t, durableReadModelProperties, "links", "#/components/schemas/FactorySessionExecutionLinks")
	assertPropertyRef(t, durableReadModelProperties, "requestedPolicy", "#/components/schemas/FactorySessionRequestedPolicy")
	assertPropertyRef(t, durableReadModelProperties, "effectivePolicy", "#/components/schemas/FactorySessionEffectivePolicy")
	assertSchemaPropertiesPresent(t, durableReadModelProperties, "FactorySessionDurableReadModel",
		"sessionId", "status", "orchestratorKind", "dialect", "resolvedSource", "sourceHash",
		"requestedPolicy", "effectivePolicy", "effectivePolicyHash", "phase", "phaseSummaries", "latestCheckpoint", "progress", "budgets", "usage",
		"artifactRefs", "resultSummary", "failureDetail", "partialResultAvailable", "lifecycle", "staleLease", "links")

	durableSummary := schemaObject(t, schemas, "FactorySessionDurableSummary")
	assertRequiredFields(t, durableSummary, "sessionId", "status", "orchestratorKind", "resolvedSource")
	durableSummaryProperties := schemaProperties(t, durableSummary, "FactorySessionDurableSummary")
	assertPropertyRef(t, durableSummaryProperties, "status", "#/components/schemas/FactorySessionDurableLifecycleStatus")
	assertPropertyRef(t, durableSummaryProperties, "resolvedSource", "#/components/schemas/FactorySessionResolvedSourceIdentity")
	assertPropertyRef(t, durableSummaryProperties, "requestedPolicy", "#/components/schemas/FactorySessionRequestedPolicy")
	assertPropertyRef(t, durableSummaryProperties, "effectivePolicy", "#/components/schemas/FactorySessionEffectivePolicy")
	assertSchemaPropertiesPresent(t, durableSummaryProperties, "FactorySessionDurableSummary",
		"sessionId", "status", "orchestratorKind", "dialect", "resolvedSource", "sourceHash",
		"requestedPolicy", "effectivePolicy", "effectivePolicyHash", "phase", "progress",
		"resultSummary", "artifactCount", "recoverable", "actions", "staleLease", "lifecycle", "links")

	assertEnumValues(t, schemaObject(t, schemas, "FactorySessionDurableLifecycleStatus"), "FactorySessionDurableLifecycleStatus", []string{
		"QUEUED", "AWAITING_APPROVAL", "RUNNING", "PAUSED", "RESUMING", "SUCCEEDED", "FAILED",
		"CANCELING", "CANCELED", "TIMED_OUT", "INTERRUPTED", "TERMINATED",
	})
	assertEnumValues(t, schemaObject(t, schemas, "FactorySessionResultStatus"), "FactorySessionResultStatus", []string{
		"NOT_READY", "PARTIAL", "FINAL", "FAILED_WITH_PARTIAL", "UNAVAILABLE",
	})

	resultSummary := schemaObject(t, schemas, "FactorySessionDurableResultSummary")
	assertRequiredFields(t, resultSummary, "resultStatus")
	assertPropertyRef(t, schemaProperties(t, resultSummary, "FactorySessionDurableResultSummary"), "resultStatus", "#/components/schemas/FactorySessionResultStatus")
}

func assertDurableSessionResultSurfaceSchemas(t *testing.T, schemas map[string]any, paths map[string]any) {
	t.Helper()

	liveResultOperation := pathOperation(t, paths, "/factory-sessions/{session_id}/result", "get")
	assertResponseSchemaRef(t, liveResultOperation, "200", "#/components/schemas/FactorySessionLiveResult")

	resultsOperation := pathOperation(t, paths, "/factory-sessions/{session_id}/results", "get")
	if got, _ := resultsOperation["operationId"].(string); got != "getFactorySessionResults" {
		t.Fatalf("paths./factory-sessions/{session_id}/results.get.operationId = %q, want getFactorySessionResults", got)
	}
	assertResponseSchemaRef(t, resultsOperation, "200", "#/components/schemas/FactorySessionResult")
	assertResponseRef(t, resultsOperation, "404", "#/components/responses/NotFound")
	parameters, ok := resultsOperation["parameters"].([]any)
	if !ok {
		t.Fatalf("paths./factory-sessions/{session_id}/results.get.parameters is missing")
	}
	assertParameterRef(t, parameters, "#/components/parameters/FactorySessionResultMode")
	assertParameterRef(t, parameters, "#/components/parameters/FactorySessionResultIncludeArtifacts")

	resultSchema := schemaObject(t, schemas, "FactorySessionResult")
	assertRequiredFields(t, resultSchema, "sessionId", "resultStatus")
	resultProperties := schemaProperties(t, resultSchema, "FactorySessionResult")
	assertPropertyRef(t, resultProperties, "resultStatus", "#/components/schemas/FactorySessionResultStatus")
	assertPropertyRef(t, resultProperties, "sessionStatus", "#/components/schemas/FactorySessionDurableLifecycleStatus")
	assertPropertyRef(t, resultProperties, "mode", "#/components/schemas/FactorySessionResultMode")
	assertPropertyRef(t, resultProperties, "primaryResult", "#/components/schemas/WorkContent")
	assertArrayItemRef(t, resultProperties, "artifactRefs", "#/components/schemas/FactoryArtifactRef")
	assertPropertyRef(t, resultProperties, "failureDetail", "#/components/schemas/FailureDetail")
	assertPropertyRef(t, resultProperties, "availability", "#/components/schemas/FactorySessionResultAvailabilityDetail")
	assertSchemaPropertiesPresent(t, resultProperties, "FactorySessionResult",
		"sessionId", "resultStatus", "sessionStatus", "mode", "includeArtifacts",
		"primaryResult", "artifactIds", "artifactRefs", "failureDetail", "partialResultAvailable", "availability")
	assertEnumValues(t, schemaObject(t, schemas, "FactorySessionResultMode"), "FactorySessionResultMode", []string{"final", "partial"})

	liveResultSchema := schemaObject(t, schemas, "FactorySessionLiveResult")
	assertRequiredFields(t, liveResultSchema, "sessionId", "status")
	liveResultProperties := schemaProperties(t, liveResultSchema, "FactorySessionLiveResult")
	assertPropertyRef(t, liveResultProperties, "status", "#/components/schemas/FactorySessionStatus")
	assertPropertyRef(t, liveResultProperties, "resultArtifactRef", "#/components/schemas/FactoryArtifactRef")
}

func assertDurableSessionDispatchArtifactSurfaceSchemas(t *testing.T, schemas map[string]any, paths map[string]any) {
	t.Helper()

	dispatchesOperation := pathOperation(t, paths, "/factory-sessions/{session_id}/dispatches", "get")
	if got, _ := dispatchesOperation["operationId"].(string); got != "listFactorySessionDispatches" {
		t.Fatalf("paths./factory-sessions/{session_id}/dispatches.get.operationId = %q, want listFactorySessionDispatches", got)
	}
	assertResponseSchemaRef(t, dispatchesOperation, "200", "#/components/schemas/ListFactorySessionDispatchesResponse")
	assertResponseRef(t, dispatchesOperation, "404", "#/components/responses/NotFound")

	dispatchOperation := pathOperation(t, paths, "/factory-sessions/{session_id}/dispatches/{dispatch_id}", "get")
	if got, _ := dispatchOperation["operationId"].(string); got != "getFactorySessionDispatch" {
		t.Fatalf("paths./factory-sessions/{session_id}/dispatches/{dispatch_id}.get.operationId = %q, want getFactorySessionDispatch", got)
	}
	assertResponseSchemaRef(t, dispatchOperation, "200", "#/components/schemas/FactoryDispatch")
	assertResponseRef(t, dispatchOperation, "404", "#/components/responses/NotFound")
	dispatchParameters, ok := dispatchOperation["parameters"].([]any)
	if !ok {
		t.Fatalf("paths./factory-sessions/{session_id}/dispatches/{dispatch_id}.get.parameters is missing")
	}
	assertParameterRef(t, dispatchParameters, "#/components/parameters/DispatchID")

	artifactsOperation := pathOperation(t, paths, "/factory-sessions/{session_id}/artifacts", "get")
	if got, _ := artifactsOperation["operationId"].(string); got != "listFactorySessionArtifacts" {
		t.Fatalf("paths./factory-sessions/{session_id}/artifacts.get.operationId = %q, want listFactorySessionArtifacts", got)
	}
	assertResponseSchemaRef(t, artifactsOperation, "200", "#/components/schemas/ListFactorySessionArtifactsResponse")
	assertResponseRef(t, artifactsOperation, "404", "#/components/responses/NotFound")

	artifactOperation := pathOperation(t, paths, "/factory-sessions/{session_id}/artifacts/{artifact_id}", "get")
	if got, _ := artifactOperation["operationId"].(string); got != "getFactorySessionArtifact" {
		t.Fatalf("paths./factory-sessions/{session_id}/artifacts/{artifact_id}.get.operationId = %q, want getFactorySessionArtifact", got)
	}
	assertResponseSchemaRef(t, artifactOperation, "200", "#/components/schemas/FactorySessionArtifactDetail")
	assertResponseRef(t, artifactOperation, "404", "#/components/responses/NotFound")
	artifactParameters, ok := artifactOperation["parameters"].([]any)
	if !ok {
		t.Fatalf("paths./factory-sessions/{session_id}/artifacts/{artifact_id}.get.parameters is missing")
	}
	assertParameterRef(t, artifactParameters, "#/components/parameters/ArtifactID")

	listDispatchesSchema := schemaObject(t, schemas, "ListFactorySessionDispatchesResponse")
	assertRequiredFields(t, listDispatchesSchema, "sessionId", "dispatches")
	listDispatchesProperties := schemaProperties(t, listDispatchesSchema, "ListFactorySessionDispatchesResponse")
	assertArrayItemRef(t, listDispatchesProperties, "dispatches", "#/components/schemas/FactorySessionDispatchSummary")

	dispatchSummarySchema := schemaObject(t, schemas, "FactorySessionDispatchSummary")
	assertRequiredFields(t, dispatchSummarySchema, "id", "status", "dispatchKind", "confirmationState")
	dispatchSummaryProperties := schemaProperties(t, dispatchSummarySchema, "FactorySessionDispatchSummary")
	assertPropertyRef(t, dispatchSummaryProperties, "status", "#/components/schemas/FactoryDispatchStatus")
	assertPropertyRef(t, dispatchSummaryProperties, "dispatchKind", "#/components/schemas/FactoryDispatchKind")
	assertPropertyRef(t, dispatchSummaryProperties, "confirmationState", "#/components/schemas/ConfirmationState")
	assertPropertyRef(t, dispatchSummaryProperties, "usage", "#/components/schemas/FactoryDispatchUsage")
	assertPropertyRef(t, dispatchSummaryProperties, "failureDetail", "#/components/schemas/FailureDetail")
	assertPropertyRef(t, dispatchSummaryProperties, "javascript", "#/components/schemas/FactoryDispatchJavaScriptProjection")
	assertArrayItemRef(t, dispatchSummaryProperties, "providerSessionRefs", "#/components/schemas/LoadableProviderSessionRef")
	assertArrayItemRef(t, dispatchSummaryProperties, "warnings", "#/components/schemas/FactoryDispatchWarning")
	assertSchemaPropertiesPresent(t, dispatchSummaryProperties, "FactorySessionDispatchSummary",
		"id", "status", "dispatchKind", "phase", "label", "attempt", "runnerId", "model", "provider",
		"providerSessionRefs", "usage", "warnings", "outputArtifactIds", "failureDetail", "javascript")

	dispatchDetailSchema := schemaObject(t, schemas, "FactoryDispatch")
	assertRequiredFields(t, dispatchDetailSchema, "id", "sessionId", "orchestratorKind", "dispatchKind", "status", "confirmationState")
	dispatchDetailProperties := schemaProperties(t, dispatchDetailSchema, "FactoryDispatch")
	assertPropertyRef(t, dispatchDetailProperties, "confirmationState", "#/components/schemas/ConfirmationState")
	assertPropertyRef(t, dispatchDetailProperties, "petri", "#/components/schemas/FactoryDispatchPetriProjection")
	assertPropertyRef(t, dispatchDetailProperties, "javascript", "#/components/schemas/FactoryDispatchJavaScriptProjection")
	assertArrayItemRef(t, dispatchDetailProperties, "providerSessionRefs", "#/components/schemas/LoadableProviderSessionRef")
	assertSchemaPropertiesPresent(t, dispatchDetailProperties, "FactoryDispatch", "attempt", "providerSessionRefs")

	listArtifactsSchema := schemaObject(t, schemas, "ListFactorySessionArtifactsResponse")
	assertRequiredFields(t, listArtifactsSchema, "sessionId", "artifacts")
	listArtifactsProperties := schemaProperties(t, listArtifactsSchema, "ListFactorySessionArtifactsResponse")
	assertArrayItemRef(t, listArtifactsProperties, "artifacts", "#/components/schemas/FactorySessionArtifactSummary")

	artifactSummarySchema := schemaObject(t, schemas, "FactorySessionArtifactSummary")
	assertRequiredFields(t, artifactSummarySchema, "id", "kind", "visibility")
	artifactSummaryProperties := schemaProperties(t, artifactSummarySchema, "FactorySessionArtifactSummary")
	assertPropertyRef(t, artifactSummaryProperties, "kind", "#/components/schemas/FactoryArtifactKind")
	assertPropertyRef(t, artifactSummaryProperties, "visibility", "#/components/schemas/FactoryArtifactVisibility")
	assertPropertyRef(t, artifactSummaryProperties, "auditMode", "#/components/schemas/FactoryArtifactAuditMode")
	assertPropertyRef(t, artifactSummaryProperties, "redactionCounts", "#/components/schemas/FactoryArtifactRedactionCounts")
	assertPropertyRef(t, artifactSummaryProperties, "retrievalRef", "#/components/schemas/FactorySessionArtifactRetrievalRef")
	assertSchemaPropertiesPresent(t, artifactSummaryProperties, "FactorySessionArtifactSummary",
		"id", "kind", "visibility", "contentHash", "sizeBytes", "createdAt", "dispatchId",
		"auditMode", "redactionCounts", "retrievalRef")

	artifactDetailSchema := schemaObject(t, schemas, "FactorySessionArtifactDetail")
	assertRequiredFields(t, artifactDetailSchema, "sessionId", "id", "kind", "visibility")
	artifactDetailProperties := schemaProperties(t, artifactDetailSchema, "FactorySessionArtifactDetail")
	assertPropertyRef(t, artifactDetailProperties, "content", "#/components/schemas/WorkContent")
	assertPropertyRef(t, artifactDetailProperties, "contentRef", "#/components/schemas/FactorySessionArtifactRetrievalRef")
	assertSchemaPropertiesPresent(t, artifactDetailProperties, "FactorySessionArtifactDetail",
		"sessionId", "id", "kind", "visibility", "contentHash", "sizeBytes", "createdAt", "dispatchId",
		"auditMode", "redactionCounts", "captureMetadata", "content", "contentRef")
}

func assertDurableSessionLifecycleControlSurfaceSchemas(t *testing.T, schemas map[string]any, paths map[string]any) {
	t.Helper()

	lifecycleRoutes := []struct {
		path          string
		operationID   string
		requestSchema string
		requiredBody  bool
	}{
		{"/factory-sessions/{session_id}/approve", "approveFactorySession", "#/components/schemas/FactorySessionApproveRequest", false},
		{"/factory-sessions/{session_id}/pause", "pauseFactorySession", "#/components/schemas/FactorySessionLifecycleControlRequest", false},
		{"/factory-sessions/{session_id}/resume", "resumeFactorySession", "#/components/schemas/FactorySessionLifecycleControlRequest", false},
		{"/factory-sessions/{session_id}/cancel", "cancelFactorySession", "#/components/schemas/FactorySessionLifecycleControlRequest", false},
		{"/factory-sessions/{session_id}/terminate", "terminateFactorySession", "#/components/schemas/FactorySessionLifecycleControlRequest", false},
		{"/factory-sessions/{session_id}/retry-dispatch", "retryFactorySessionDispatch", "#/components/schemas/FactorySessionRetryDispatchRequest", true},
		{"/factory-sessions/{session_id}/interrupt-dispatch", "interruptFactorySessionDispatch", "#/components/schemas/FactorySessionInterruptDispatchRequest", true},
	}
	for _, route := range lifecycleRoutes {
		operation := pathOperation(t, paths, route.path, "post")
		if got, _ := operation["operationId"].(string); got != route.operationID {
			t.Fatalf("paths.%s.post.operationId = %q, want %s", route.path, got, route.operationID)
		}
		if route.requiredBody {
			assertRequestSchemaRef(t, operation, route.requestSchema)
		}
		assertResponseSchemaRef(t, operation, "200", "#/components/schemas/FactorySessionLifecycleControlResponse")
		assertResponseSchemaRef(t, operation, "202", "#/components/schemas/FactorySessionLifecycleControlResponse")
		assertResponseRef(t, operation, "404", "#/components/responses/NotFound")
		assertResponseRef(t, operation, "409", "#/components/responses/FactorySessionLifecycleControlConflict")
	}

	retryOperation := pathOperation(t, paths, "/factory-sessions/{session_id}/retry-dispatch", "post")
	assertRequestSchemaRef(t, retryOperation, "#/components/schemas/FactorySessionRetryDispatchRequest")
	retryRequestSchema := schemaObject(t, schemas, "FactorySessionRetryDispatchRequest")
	assertRequiredFields(t, retryRequestSchema, "dispatchId")

	interruptOperation := pathOperation(t, paths, "/factory-sessions/{session_id}/interrupt-dispatch", "post")
	assertRequestSchemaRef(t, interruptOperation, "#/components/schemas/FactorySessionInterruptDispatchRequest")
	interruptRequestSchema := schemaObject(t, schemas, "FactorySessionInterruptDispatchRequest")
	assertRequiredFields(t, interruptRequestSchema, "dispatchId")

	controlResponseSchema := schemaObject(t, schemas, "FactorySessionLifecycleControlResponse")
	assertRequiredFields(t, controlResponseSchema, "sessionId", "operation", "outcome", "status")
	controlResponseProperties := schemaProperties(t, controlResponseSchema, "FactorySessionLifecycleControlResponse")
	assertPropertyRef(t, controlResponseProperties, "operation", "#/components/schemas/FactorySessionLifecycleControlKind")
	assertPropertyRef(t, controlResponseProperties, "outcome", "#/components/schemas/FactorySessionLifecycleControlOutcome")
	assertPropertyRef(t, controlResponseProperties, "status", "#/components/schemas/FactorySessionDurableLifecycleStatus")
	assertPropertyRef(t, controlResponseProperties, "session", "#/components/schemas/FactorySessionDurableReadModel")
	assertPropertyRef(t, controlResponseProperties, "links", "#/components/schemas/FactorySessionLifecycleControlLinks")
	assertSchemaPropertiesPresent(t, controlResponseProperties, "FactorySessionLifecycleControlResponse",
		"sessionId", "operation", "outcome", "status", "session", "effectivePolicyHash",
		"approvalPreviewId", "dispatchId", "retryDispatchId", "detail", "links")

	approveRequestSchema := schemaObject(t, schemas, "FactorySessionApproveRequest")
	approveRequestProperties := schemaProperties(t, approveRequestSchema, "FactorySessionApproveRequest")
	assertSchemaPropertiesPresent(t, approveRequestProperties, "FactorySessionApproveRequest",
		"requestId", "reason", "approvalPreviewId", "approvedPolicy")

	controlLinksSchema := schemaObject(t, schemas, "FactorySessionLifecycleControlLinks")
	controlLinksProperties := schemaProperties(t, controlLinksSchema, "FactorySessionLifecycleControlLinks")
	assertSchemaPropertiesPresent(t, controlLinksProperties, "FactorySessionLifecycleControlLinks",
		"session", "results", "dispatches", "artifacts", "events", "status")

	assertEnumValues(t, schemaObject(t, schemas, "FactorySessionLifecycleControlKind"), "FactorySessionLifecycleControlKind", []string{
		"APPROVE", "PAUSE", "RESUME", "CANCEL", "TERMINATE", "RETRY_DISPATCH", "INTERRUPT_DISPATCH",
	})
	assertEnumValues(t, schemaObject(t, schemas, "FactorySessionLifecycleControlOutcome"), "FactorySessionLifecycleControlOutcome", []string{
		"ACCEPTED", "NO_OP", "INVALID_STATE", "TERMINAL_SESSION", "CONFLICT",
	})
}
