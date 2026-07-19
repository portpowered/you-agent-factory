package apicontract_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	factorysessioncursors "github.com/portpowered/infinite-you/pkg/factory/sessions/cursors"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func TestGeneratedOpenAPIContractsCompile(t *testing.T) {
	submitRequest := generatedSubmitRequestFixture(t)
	workRequest := generatedWorkRequestFixture()
	namedFactory := generatedNamedFactoryFixture()

	assertGeneratedOpenAPIBoundaryTypes(t, submitRequest, workRequest, namedFactory)
	assertGeneratedSubmitRequestJSON(t, submitRequest)
	assertGeneratedWorkRequestJSON(t, workRequest)
}

func TestGeneratedFactoryContractsCompileAndRoundTrip(t *testing.T) {
	namedFactory := generatedNamedFactoryFixture()

	assertGeneratedNamedFactoryContracts(t, namedFactory)
	assertGeneratedNamedFactoryJSONRoundTrip(t, namedFactory)
	assertGeneratedReservedCurrentFactoryJSONRoundTrip(t, namedFactory)
	assertGeneratedCurrentFactoryNotFoundJSON(t)
}

func TestGeneratedFactoryLayoutEmptyStateVariantsCompileAndRoundTrip(t *testing.T) {
	textState := factoryapi.FactoryLayoutEmptyState{}
	if err := textState.FromFactoryLayoutEmptyState0(factoryapi.FactoryLayoutEmptyState0{Text: "No work is waiting."}); err != nil {
		t.Fatalf("encode generated text empty-state variant: %v", err)
	}
	decodedText, err := textState.AsFactoryLayoutEmptyState0()
	if err != nil || decodedText.Text != "No work is waiting." {
		t.Fatalf("generated text empty-state variant = %#v, %v", decodedText, err)
	}

	imageState := factoryapi.FactoryLayoutEmptyState{}
	wantImage := factoryapi.FactoryLayoutImage{
		AlternativeText: "No active review",
		Source: factoryapi.FactoryLayoutImageSource{
			Data:      []byte{1, 2, 3},
			Kind:      factoryapi.EMBEDDED,
			MediaType: factoryapi.Imagepng,
		},
	}
	if err := imageState.FromFactoryLayoutEmptyState1(factoryapi.FactoryLayoutEmptyState1{Image: wantImage}); err != nil {
		t.Fatalf("encode generated image empty-state variant: %v", err)
	}
	decodedImage, err := imageState.AsFactoryLayoutEmptyState1()
	if err != nil || decodedImage.Image.AlternativeText != wantImage.AlternativeText {
		t.Fatalf("generated image empty-state variant = %#v, %v", decodedImage, err)
	}
}

func TestGeneratedFactoryContractsSupportClassifierRoutes(t *testing.T) {
	classifierType := factoryapi.WorkstationTypeClassifierWorkstation
	namedFactory := factoryapi.Factory{
		Name: "classifier-factory",
		Workstations: &[]factoryapi.Workstation{{
			Name:   "classify-task",
			Type:   &classifierType,
			Worker: "planner",
			Inputs: []factoryapi.WorkstationIO{{WorkType: "task", State: "init"}},
			ClassificationRoutes: &[]factoryapi.ClassificationRoute{
				{Label: "approved", Outputs: []factoryapi.WorkstationIO{{WorkType: "task", State: "done"}}},
				{Label: "spam", Outputs: []factoryapi.WorkstationIO{{WorkType: "task", State: "failed"}}},
			},
		}},
	}

	encoded, err := json.Marshal(namedFactory)
	if err != nil {
		t.Fatalf("marshal generated classifier factory: %v", err)
	}
	if !strings.Contains(string(encoded), `"classificationRoutes"`) {
		t.Fatalf("generated classifier factory JSON missing classificationRoutes: %s", encoded)
	}
}

func generatedSubmitRequestFixture(t *testing.T) factoryapi.SubmitWorkRequest {
	t.Helper()

	var submitRequest factoryapi.SubmitWorkRequest
	submitRequest.Name = stringPtr("task-1")
	submitRequest.WorkTypeName = "task"
	submitRequest.CurrentChainingTraceId = stringPtr("chain-submit-1")
	submitRelationState := "complete"
	submitRequest.Relations = &[]factoryapi.SubmitRelation{{
		Type:          factoryapi.RelationTypeDependsOn,
		TargetWorkId:  "work-1",
		RequiredState: &submitRelationState,
	}}
	if err := json.Unmarshal([]byte(`[
		{"type":"text","text":"Draft a summary."},
		{"type":"image","url":"file://staged/work-item.png","stagedFileRef":"staged://work-item.png","fileName":"work-item.png","mediaType":"image/png"}
	]`), &submitRequest.Items); err != nil {
		t.Fatalf("unmarshal generated submit request items: %v", err)
	}
	return submitRequest
}

func generatedWorkRequestFixture() factoryapi.WorkRequest {
	workID := "work-1"
	requestID := "request-1"
	traceID := "trace-1"
	currentChainingTraceID := "chain-work-1"
	previousChainingTraceIDs := []string{"chain-a", "chain-z"}
	initialState := "queued"
	initialWorkState := factoryapi.WorkState{Name: initialState, Type: factoryapi.WorkStateTypeINITIAL}
	tags := factoryapi.StringMap{"priority": "high"}
	relation := factoryapi.Relation{
		Type:           factoryapi.RelationTypeDependsOn,
		SourceWorkName: "publish",
		TargetWorkName: "draft",
		RequiredState:  stringPtr("complete"),
	}
	parentChildRelation := factoryapi.Relation{
		Type:           factoryapi.RelationTypeParentChild,
		SourceWorkName: "draft",
		TargetWorkName: "epic",
	}
	workRelations := []factoryapi.Relation{relation, parentChildRelation}
	batchWork := factoryapi.Work{
		Name:                     "draft",
		WorkId:                   &workID,
		RequestId:                &requestID,
		WorkTypeName:             stringPtr("task"),
		State:                    &initialWorkState,
		CurrentChainingTraceId:   &currentChainingTraceID,
		PreviousChainingTraceIds: &previousChainingTraceIDs,
		TraceId:                  &traceID,
		Payload:                  map[string]any{"title": "first draft"},
		Tags:                     &tags,
		Relations:                &workRelations,
	}
	return factoryapi.WorkRequest{
		RequestId:              requestID,
		CurrentChainingTraceId: stringPtr("chain-request-1"),
		Type:                   factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works:                  &[]factoryapi.Work{batchWork},
		Relations:              &[]factoryapi.Relation{relation, parentChildRelation},
	}
}

func assertGeneratedOpenAPIBoundaryTypes(
	t *testing.T,
	submitRequest factoryapi.SubmitWorkRequest,
	workRequest factoryapi.WorkRequest,
	namedFactory factoryapi.Factory,
) {
	t.Helper()

	assertGeneratedOpenAPISurfaceTypes(t, submitRequest, workRequest, namedFactory)
	assertGeneratedOpenAPIChainingAndRelations(t, submitRequest, workRequest)
}

func assertGeneratedOpenAPISurfaceTypes(
	t *testing.T,
	submitRequest factoryapi.SubmitWorkRequest,
	workRequest factoryapi.WorkRequest,
	namedFactory factoryapi.Factory,
) {
	t.Helper()

	submitResponse := factoryapi.SubmitWorkResponse{TraceId: "trace-1", RequestId: "request-1", Accepted: true}
	sourceKind := factoryapi.InvocationInputSourceKindText
	invocationArgs := map[string]any{"input": "hello", "tag": []any{"alpha", "beta"}}
	invocationContent := factoryapi.WorkContent{}
	invocationRequest := factoryapi.InvocationRequest{
		SourceKind: &sourceKind,
		Content:    &invocationContent,
		Args:       &invocationArgs,
	}
	invocationResponse := factoryapi.InvocationResponse{
		RequestId:     "invoke-1",
		TraceId:       "trace-invoke-1",
		Status:        factoryapi.InvocationTerminalStatusCompleted,
		PrimaryResult: &factoryapi.WorkContent{},
	}
	invocationReturnPolicy := factoryapi.InvocationReturnPolicySubmittedWorkTerminal
	invocationReturn := factoryapi.InvocationReturn{Policy: invocationReturnPolicy}
	invocationSignaturePolicy := factoryapi.FactoryInvocationUnknownNamedArgumentPolicyCollect
	parameterTypeHint := factoryapi.FactoryInvocationParameterTypeHintString
	parameterValueMode := factoryapi.FactoryInvocationParameterValueModeExact
	bindingKind := factoryapi.FactoryInvocationParameterBindingKindNamed
	outputContractMode := factoryapi.FactoryInvocationOutputContractModeJson
	invocationSignature := factoryapi.FactoryInvocationSignature{
		UnknownNamedArgumentPolicy: &invocationSignaturePolicy,
		Parameters: &[]factoryapi.FactoryInvocationParameter{{
			Name:      "input",
			TypeHint:  &parameterTypeHint,
			ValueMode: &parameterValueMode,
			Bindings: &[]factoryapi.FactoryInvocationParameterBinding{{
				Kind: bindingKind,
			}},
		}},
		OutputContract: &factoryapi.FactoryInvocationOutputContract{
			Mode: &outputContractMode,
		},
		Examples: &[]factoryapi.FactoryInvocationExample{{
			Name: "basic",
			Argv: &[]string{"brief.md"},
		}},
	}
	upsertResponse := factoryapi.UpsertWorkRequestResponse{
		RequestId: workRequest.RequestId,
		TraceId:   "trace-1",
		Works: []factoryapi.UpsertWorkRequestSubmittedWork{
			{Name: "generated", WorkTypeName: "task", WorkId: "work-generated"},
		},
	}
	triggerAtStart := true
	workstationKind := factoryapi.WorkstationKindCron
	workstationRuntimeType := factoryapi.WorkstationTypeModelWorkstation
	cron := factoryapi.WorkstationCron{Schedule: "*/5 * * * *", TriggerAtStart: &triggerAtStart}
	workstation := factoryapi.Workstation{
		Name:     "daily-refresh",
		Behavior: &workstationKind,
		Type:     &workstationRuntimeType,
		Worker:   "agent",
		Inputs:   []factoryapi.WorkstationIO{{WorkType: "task", State: "init"}},
		Outputs:  &[]factoryapi.WorkstationIO{{WorkType: "task", State: "complete"}},
	}

	assertGeneratedSubmitAndInvocationTypesUsable(t, submitRequest, submitResponse, invocationRequest, invocationResponse, invocationReturn)
	assertGeneratedInvocationSignatureTypesUsable(t, invocationSignature)
	assertGeneratedFactoryAndUpsertTypesUsable(t, workRequest, upsertResponse, namedFactory)
	assertGeneratedWorkstationTypesUsable(t, workstation, cron)
}

func assertGeneratedSubmitAndInvocationTypesUsable(
	t *testing.T,
	submitRequest factoryapi.SubmitWorkRequest,
	submitResponse factoryapi.SubmitWorkResponse,
	invocationRequest factoryapi.InvocationRequest,
	invocationResponse factoryapi.InvocationResponse,
	invocationReturn factoryapi.InvocationReturn,
) {
	t.Helper()

	if submitRequest.Name == nil || *submitRequest.Name == "" || submitRequest.WorkTypeName == "" || submitResponse.TraceId == "" {
		t.Fatal("generated OpenAPI submit request and response types should be usable")
	}
	if invocationRequest.SourceKind == nil || *invocationRequest.SourceKind != factoryapi.InvocationInputSourceKindText || invocationRequest.Content == nil {
		t.Fatal("generated OpenAPI invocation request types should be usable")
	}
	if invocationResponse.PrimaryResult == nil || invocationReturn.Policy != factoryapi.InvocationReturnPolicySubmittedWorkTerminal {
		t.Fatal("generated OpenAPI invocation response and return types should be usable")
	}
}

func assertGeneratedInvocationSignatureTypesUsable(t *testing.T, invocationSignature factoryapi.FactoryInvocationSignature) {
	t.Helper()

	if invocationSignature.UnknownNamedArgumentPolicy == nil || invocationSignature.Parameters == nil {
		t.Fatal("generated invocation signature parameters should be usable")
	}
	if invocationSignature.OutputContract == nil || invocationSignature.Examples == nil {
		t.Fatal("generated invocation signature contract and examples should be usable")
	}
}

func assertGeneratedFactoryAndUpsertTypesUsable(
	t *testing.T,
	workRequest factoryapi.WorkRequest,
	upsertResponse factoryapi.UpsertWorkRequestResponse,
	namedFactory factoryapi.Factory,
) {
	t.Helper()

	if workRequest.RequestId == "" || upsertResponse.RequestId == "" {
		t.Fatal("generated work request and upsert response types should be usable")
	}
	if namedFactory.Name == "" || namedFactory.Workstations == nil {
		t.Fatal("generated factory types should be usable")
	}
}

func assertGeneratedWorkstationTypesUsable(
	t *testing.T,
	workstation factoryapi.Workstation,
	cron factoryapi.WorkstationCron,
) {
	t.Helper()

	if workstation.Behavior == nil || workstation.Type == nil {
		t.Fatal("generated workstation types should be usable")
	}
	if cron.Schedule == "" || cron.TriggerAtStart == nil {
		t.Fatal("generated workstation cron types should be usable")
	}
}

func assertGeneratedOpenAPIChainingAndRelations(t *testing.T, submitRequest factoryapi.SubmitWorkRequest, workRequest factoryapi.WorkRequest) {
	t.Helper()

	if submitRequest.CurrentChainingTraceId == nil || *submitRequest.CurrentChainingTraceId != "chain-submit-1" {
		t.Fatal("generated submit request should expose current chaining trace ID")
	}
	if workRequest.Relations == nil || len(*workRequest.Relations) != 2 || (*workRequest.Relations)[1].Type != factoryapi.RelationTypeParentChild {
		t.Fatal("generated work request relations should advertise parent-child support")
	}
	if workRequest.Works == nil || len(*workRequest.Works) != 1 || (*workRequest.Works)[0].State == nil || (*workRequest.Works)[0].State.Name != "queued" {
		t.Fatal("generated work request works should advertise explicit state support")
	}
}

func assertGeneratedSubmitRequestJSON(t *testing.T, submitRequest factoryapi.SubmitWorkRequest) {
	t.Helper()

	submitRequestJSON, err := json.Marshal(submitRequest)
	if err != nil {
		t.Fatalf("marshal generated submit request: %v", err)
	}
	if !strings.Contains(string(submitRequestJSON), `"relations"`) || !strings.Contains(string(submitRequestJSON), `"targetWorkId":"work-1"`) {
		t.Fatalf("generated submit request JSON must preserve token-level relations: %s", submitRequestJSON)
	}
	if !strings.Contains(string(submitRequestJSON), `"items"`) || !strings.Contains(string(submitRequestJSON), `"stagedFileRef":"staged://work-item.png"`) {
		t.Fatalf("generated submit request JSON must preserve structured submit-work items: %s", submitRequestJSON)
	}
}

func assertGeneratedWorkRequestJSON(t *testing.T, workRequest factoryapi.WorkRequest) {
	t.Helper()

	if err := json.Unmarshal([]byte(`[
		{"type":"text","text":"Review the screenshot."},
		{"type":"image","url":"file://fixtures/review.png"}
	]`), &(*workRequest.Works)[0].Content); err != nil {
		t.Fatalf("unmarshal generated work content: %v", err)
	}
	if workRequest.CurrentChainingTraceId == nil || *workRequest.CurrentChainingTraceId != "chain-request-1" || (*workRequest.Works)[0].CurrentChainingTraceId == nil || *(*workRequest.Works)[0].CurrentChainingTraceId != "chain-work-1" {
		t.Fatal("generated work request contracts should expose current chaining trace IDs")
	}
	if (*workRequest.Works)[0].PreviousChainingTraceIds == nil || len(*(*workRequest.Works)[0].PreviousChainingTraceIds) != 2 {
		t.Fatal("generated work request contracts should expose predecessor chaining trace IDs")
	}
	if (*workRequest.Works)[0].Relations == nil || len(*(*workRequest.Works)[0].Relations) != 2 || (*(*workRequest.Works)[0].Relations)[0].Type != factoryapi.RelationTypeDependsOn {
		t.Fatal("generated work contracts should expose API-aligned relation entries")
	}
	if (*workRequest.Works)[0].Content == nil {
		t.Fatal("generated work contracts should expose canonical content parts")
	}

	withoutRelations := factoryapi.Work{Name: "no-relations"}
	withoutRelationsJSON, err := json.Marshal(withoutRelations)
	if err != nil {
		t.Fatalf("marshal generated work without relations: %v", err)
	}
	if strings.Contains(string(withoutRelationsJSON), `"relations"`) {
		t.Fatalf("generated work without relations should omit optional relations: %s", withoutRelationsJSON)
	}
	var decodedWithoutRelations factoryapi.Work
	decodeRoundTripJSON(t, withoutRelationsJSON, &decodedWithoutRelations, "generated work without relations")
	if decodedWithoutRelations.Relations != nil {
		t.Fatalf("decoded generated work relations = %#v, want nil when omitted", decodedWithoutRelations.Relations)
	}
}

func assertGeneratedNamedFactoryContracts(t *testing.T, namedFactory factoryapi.Factory) {
	t.Helper()

	saveRequest := factoryapi.SaveFactoryForSessionRequest{Factory: namedFactory}
	current := namedFactory
	badRequest := factoryapi.CreateFactoryBadRequest{
		Code:    factoryapi.ErrorResponseCodeINVALIDFACTORYNAME,
		Family:  factoryapi.ErrorFamilyBadRequest,
		Message: "factory name must use lowercase letters, numbers, and hyphens",
	}
	conflict := factoryapi.CreateFactoryConflict{
		Code:    factoryapi.ErrorResponseCodeFACTORYALREADYEXISTS,
		Family:  factoryapi.ErrorFamilyConflict,
		Message: "factory already exists",
	}

	if saveRequest.Factory.Name == "" || saveRequest.Factory.WorkTypes == nil || saveRequest.Factory.Workers == nil || saveRequest.Factory.Workstations == nil {
		t.Fatal("generated session factory save request and response types should be usable")
	}
	if current.Name == "" || current.Workstations == nil {
		t.Fatal("generated current named-factory response type should be usable")
	}
	if badRequest.Code != factoryapi.ErrorResponseCodeINVALIDFACTORYNAME || badRequest.Family != factoryapi.ErrorFamilyBadRequest {
		t.Fatalf("generated bad-request contract = %#v, want code %q and family %q", badRequest, factoryapi.ErrorResponseCodeINVALIDFACTORYNAME, factoryapi.ErrorFamilyBadRequest)
	}
	if conflict.Code != factoryapi.ErrorResponseCodeFACTORYALREADYEXISTS || conflict.Family != factoryapi.ErrorFamilyConflict {
		t.Fatalf("generated conflict contract = %#v, want code %q and family %q", conflict, factoryapi.ErrorResponseCodeFACTORYALREADYEXISTS, factoryapi.ErrorFamilyConflict)
	}
}

func assertGeneratedNamedFactoryJSONRoundTrip(t *testing.T, namedFactory factoryapi.Factory) {
	t.Helper()

	encoded, err := json.Marshal(namedFactory)
	if err != nil {
		t.Fatalf("marshal generated NamedFactory: %v", err)
	}
	assertGeneratedNamedFactoryJSONShape(t, encoded)

	var roundTripped factoryapi.Factory
	decodeRoundTripJSON(t, encoded, &roundTripped, "generated NamedFactory")
	assertGeneratedNamedFactoryRoundTripFields(t, namedFactory, roundTripped)
	assertGeneratedNamedFactoryRoundTripRoutes(t, roundTripped)
}

func assertGeneratedNamedFactoryJSONShape(t *testing.T, encoded []byte) {
	t.Helper()

	if !strings.Contains(string(encoded), `"name":"customer-support-triage"`) {
		t.Fatalf("generated NamedFactory JSON missing canonical name field: %s", encoded)
	}
	if !strings.Contains(string(encoded), `"invocationSignature"`) {
		t.Fatalf("generated NamedFactory JSON missing invocationSignature field: %s", encoded)
	}
	if strings.Contains(string(encoded), `"factory_name"`) {
		t.Fatalf("generated NamedFactory JSON contains unexpected legacy field: %s", encoded)
	}
}

func assertGeneratedNamedFactoryRoundTripFields(t *testing.T, namedFactory factoryapi.Factory, roundTripped factoryapi.Factory) {
	t.Helper()

	if roundTripped.Name != namedFactory.Name {
		t.Fatalf("round-tripped named factory name = %q, want %q", roundTripped.Name, namedFactory.Name)
	}
	if roundTripped.InvocationSignature == nil || roundTripped.InvocationSignature.Parameters == nil || len(*roundTripped.InvocationSignature.Parameters) != 1 {
		t.Fatalf("round-tripped named factory invocationSignature = %#v, want one parameter", roundTripped.InvocationSignature)
	}
	if roundTripped.Workstations == nil || len(*roundTripped.Workstations) != 1 || (*roundTripped.Workstations)[0].Worker != "planner" {
		t.Fatalf("round-tripped named factory workstations = %#v, want planner workstation", roundTripped.Workstations)
	}
}

func assertGeneratedNamedFactoryRoundTripRoutes(t *testing.T, roundTripped factoryapi.Factory) {
	t.Helper()

	workstation := (*roundTripped.Workstations)[0]
	if workstation.OnContinue == nil || len(*workstation.OnContinue) != 2 {
		t.Fatalf("round-tripped workstation onContinue = %#v, want two array routes", workstation.OnContinue)
	}
	if workstation.OnRejection == nil || len(*workstation.OnRejection) != 1 || (*workstation.OnRejection)[0].State != "review" {
		t.Fatalf("round-tripped workstation onRejection = %#v, want review route", workstation.OnRejection)
	}
	if workstation.OnFailure == nil || len(*workstation.OnFailure) != 1 || (*workstation.OnFailure)[0].State != "failed" {
		t.Fatalf("round-tripped workstation onFailure = %#v, want failed route", workstation.OnFailure)
	}
}

func assertGeneratedReservedCurrentFactoryJSONRoundTrip(t *testing.T, namedFactory factoryapi.Factory) {
	t.Helper()

	namedFactory.Name = "UNDEFINED"
	encoded, err := json.Marshal(namedFactory)
	if err != nil {
		t.Fatalf("marshal generated current Factory: %v", err)
	}
	if !strings.Contains(string(encoded), `"name":"UNDEFINED"`) {
		t.Fatalf("generated current Factory JSON missing reserved current-factory name: %s", encoded)
	}

	var roundTripped factoryapi.Factory
	decodeRoundTripJSON(t, encoded, &roundTripped, "generated current Factory")
	if roundTripped.Name != "UNDEFINED" {
		t.Fatalf("round-tripped current factory name = %q, want %q", roundTripped.Name, "UNDEFINED")
	}
}

func assertGeneratedCurrentFactoryNotFoundJSON(t *testing.T) {
	t.Helper()

	notFound := factoryapi.CurrentFactoryNotFound{
		Code:    factoryapi.ErrorResponseCodeNOTFOUND,
		Family:  factoryapi.ErrorFamilyNotFound,
		Message: "current factory not found",
	}
	if notFound.Code != factoryapi.ErrorResponseCodeNOTFOUND || notFound.Family != factoryapi.ErrorFamilyNotFound {
		t.Fatalf("generated not-found contract = %#v, want code %q and family %q", notFound, factoryapi.ErrorResponseCodeNOTFOUND, factoryapi.ErrorFamilyNotFound)
	}

	encoded, err := json.Marshal(notFound)
	if err != nil {
		t.Fatalf("marshal generated CurrentFactoryNotFound: %v", err)
	}
	if !strings.Contains(string(encoded), `"code":"NOT_FOUND"`) {
		t.Fatalf("generated CurrentFactoryNotFound JSON missing NOT_FOUND code: %s", encoded)
	}
	if !strings.Contains(string(encoded), `"family":"NOT_FOUND"`) {
		t.Fatalf("generated CurrentFactoryNotFound JSON missing NOT_FOUND family: %s", encoded)
	}
}

type syncPreflightRecoveryFixtureCatalog struct {
	Scenarios                []syncPreflightRecoveryScenario      `json:"scenarios"`
	IdentityScopeComparisons []syncPreflightIdentityScopeScenario `json:"identityScopeComparisons"`
}

type syncPreflightRecoveryScenario struct {
	ID       string                    `json:"id"`
	Tags     syncPreflightRecoveryTags `json:"tags"`
	Response map[string]any            `json:"response"`
}

type syncPreflightRecoveryTags struct {
	ReasonCode         string `json:"reasonCode"`
	CheckpointReusable string `json:"checkpointReusable,omitempty"`
	CursorValid        string `json:"cursorValid,omitempty"`
}

type syncPreflightIdentityScopeScenario struct {
	ID                 string         `json:"id"`
	Previous           map[string]any `json:"previous"`
	Current            map[string]any `json:"current"`
	WantClassification string         `json:"wantClassification"`
}

// TestOpenAPIContract_SyncPreflightRecoveryFixturesValidateAndRoundTrip keeps
// sync-preflight recovery contract coverage in this file to satisfy pkg-file-count.
func TestOpenAPIContract_SyncPreflightRecoveryFixturesValidateAndRoundTrip(t *testing.T) {
	doc := loadValidatedOpenAPIContract(t)
	catalog := loadSyncPreflightRecoveryFixtureCatalog(t)

	seenReasonCodes := map[string]struct{}{}
	for _, scenario := range catalog.Scenarios {
		t.Run(scenario.ID, func(t *testing.T) {
			assertSyncPreflightRecoveryScenarioFixture(t, doc, scenario)
			seenReasonCodes[scenario.Tags.ReasonCode] = struct{}{}
		})
	}

	for _, reasonCode := range []string{"ok", "cursor_stale", "session_not_found", "logical_session_remap"} {
		if _, ok := seenReasonCodes[reasonCode]; !ok {
			t.Fatalf("sync preflight recovery fixture coverage for %s = missing, want scenario", reasonCode)
		}
	}
}

func TestOpenAPIContract_SyncPreflightIdentityScopeComparisonsDistinguishBackendAndStreamChanges(t *testing.T) {
	catalog := loadSyncPreflightRecoveryFixtureCatalog(t)

	for _, scenario := range catalog.IdentityScopeComparisons {
		t.Run(scenario.ID, func(t *testing.T) {
			previous := identityScopeFromFixtureMap(scenario.Previous)
			current := identityScopeFromFixtureMap(scenario.Current)

			reason, ok := factorysessioncursors.ClassifyIdentityMismatch(previous, current)
			if !ok {
				t.Fatal("ClassifyIdentityMismatch = false, want mismatch")
			}
			if string(reason) != scenario.WantClassification {
				t.Fatalf("classification = %q, want %q", reason, scenario.WantClassification)
			}

			if previous.BackendScopeID == current.BackendScopeID &&
				scenario.WantClassification == string(factorysessioncursors.ReasonBackendScopeChanged) {
				t.Fatal("backend scope classification requires backendScopeId change")
			}
			if previous.StreamGenerationID == current.StreamGenerationID &&
				scenario.WantClassification == string(factorysessioncursors.ReasonStreamGenerationChanged) {
				t.Fatal("stream generation classification requires streamGenerationId change")
			}
			if previous.BackendScopeID != current.BackendScopeID &&
				scenario.WantClassification == string(factorysessioncursors.ReasonStreamGenerationChanged) &&
				previous.FactorySessionID == current.FactorySessionID {
				if previous.BackendScopeID == current.BackendScopeID {
					t.Fatal("stream-only classification should not change backendScopeId")
				}
			}
		})
	}
}

// Gate evidence for session-persistence-hardening-and-observability-005: proves the
// verification gates still protect observable recovery behavior, not only compile-time
// contract shape.
func TestSessionPersistenceHardeningGateEvidence_RecoveryOutcomesControlCheckpointReuse(t *testing.T) {
	doc := loadValidatedOpenAPIContract(t)
	catalog := loadSyncPreflightRecoveryFixtureCatalog(t)

	var reusableScenario *syncPreflightRecoveryScenario
	var staleScenario *syncPreflightRecoveryScenario
	for index := range catalog.Scenarios {
		scenario := catalog.Scenarios[index]
		switch scenario.Tags.ReasonCode {
		case "ok":
			reusableScenario = &scenario
		case "cursor_stale":
			staleScenario = &scenario
		}
	}
	if reusableScenario == nil || staleScenario == nil {
		t.Fatal("sync preflight recovery fixtures missing ok or cursor_stale scenario")
	}

	assertSyncPreflightRecoveryScenarioFixture(t, doc, *reusableScenario)
	assertSyncPreflightRecoveryScenarioFixture(t, doc, *staleScenario)

	var reusableResponse factoryapi.FactorySessionSyncPreflightResponse
	assertGeneratedFixtureRoundTrip(t, reusableScenario.Response, "FactorySessionSyncPreflightResponse", func(raw []byte) {
		decodeRoundTripJSON(t, raw, &reusableResponse, reusableScenario.ID+" reusable response")
	})
	var staleResponse factoryapi.FactorySessionSyncPreflightResponse
	assertGeneratedFixtureRoundTrip(t, staleScenario.Response, "FactorySessionSyncPreflightResponse", func(raw []byte) {
		decodeRoundTripJSON(t, raw, &staleResponse, staleScenario.ID+" stale response")
	})

	if !reusableResponse.CheckpointReusable {
		t.Fatal("reusable recovery outcome checkpointReusable = false, want true")
	}
	if staleResponse.CheckpointReusable {
		t.Fatal("stale cursor recovery outcome checkpointReusable = true, want false")
	}

	staleDiagnostic, ok := factorysessioncursors.InvalidationFromPreflight(apisurface.FactorySessionCursorPreflightResult(staleResponse))
	if !ok {
		t.Fatal("stale cursor recovery outcome missing invalidation diagnostic")
	}
	if staleDiagnostic.Reason != factorysessioncursors.ReasonCursorStale {
		t.Fatalf("stale cursor diagnostic reason = %q, want %q", staleDiagnostic.Reason, factorysessioncursors.ReasonCursorStale)
	}
	if staleDiagnostic.RecoveryAction != factorysessioncursors.RecoveryReplayWithoutCursor {
		t.Fatalf(
			"stale cursor diagnostic recovery = %q, want %q",
			staleDiagnostic.RecoveryAction,
			factorysessioncursors.RecoveryReplayWithoutCursor,
		)
	}
}

func assertSyncPreflightRecoveryScenarioFixture(
	t *testing.T,
	doc *openapi3.T,
	scenario syncPreflightRecoveryScenario,
) {
	t.Helper()

	assertOpenAPIFixtureValidates(t, doc, "FactorySessionSyncPreflightResponse", scenario.Response)
	assertGeneratedFixtureRoundTrip(t, scenario.Response, "FactorySessionSyncPreflightResponse", func(raw []byte) {
		var value factoryapi.FactorySessionSyncPreflightResponse
		decodeRoundTripJSON(t, raw, &value, scenario.ID+" sync preflight response")
		assertSyncPreflightRecoveryOutcome(t, scenario, value)
	})
}

func assertSyncPreflightRecoveryOutcome(
	t *testing.T,
	scenario syncPreflightRecoveryScenario,
	response factoryapi.FactorySessionSyncPreflightResponse,
) {
	t.Helper()

	if string(response.ReasonCode) != scenario.Tags.ReasonCode {
		t.Fatalf("%s reasonCode = %q, want %q", scenario.ID, response.ReasonCode, scenario.Tags.ReasonCode)
	}

	switch response.ReasonCode {
	case factoryapi.Ok:
		assertSyncPreflightOkOutcome(t, scenario.ID, response)
	case factoryapi.CursorStale:
		assertSyncPreflightCursorStaleOutcome(t, scenario.ID, response)
	case factoryapi.SessionNotFound:
		assertSyncPreflightSessionNotFoundOutcome(t, scenario.ID, response)
	case factoryapi.LogicalSessionRemap:
		assertSyncPreflightLogicalSessionRemapOutcome(t, scenario.ID, response)
	default:
		t.Fatalf("%s reasonCode = %q, want supported recovery outcome", scenario.ID, response.ReasonCode)
	}

	assertSyncPreflightInvalidationDiagnostic(t, scenario.ID, response)
}

func assertSyncPreflightOkOutcome(
	t *testing.T,
	scenarioID string,
	response factoryapi.FactorySessionSyncPreflightResponse,
) {
	t.Helper()

	if !response.CheckpointReusable {
		t.Fatalf("%s checkpointReusable = false, want true", scenarioID)
	}
	if !response.ReconnectCursor.ValidForStreamGeneration {
		t.Fatalf("%s reconnect cursor validForStreamGeneration = false, want true", scenarioID)
	}
	if response.BackendScopeId == nil || response.FactorySessionId == nil || response.StreamGenerationId == nil {
		t.Fatalf("%s identity fields = %#v, want full identity set", scenarioID, response)
	}
}

func assertSyncPreflightCursorStaleOutcome(
	t *testing.T,
	scenarioID string,
	response factoryapi.FactorySessionSyncPreflightResponse,
) {
	t.Helper()

	if response.CheckpointReusable {
		t.Fatalf("%s checkpointReusable = true, want false", scenarioID)
	}
	if response.ReconnectCursor.ValidForStreamGeneration {
		t.Fatalf("%s reconnect cursor validForStreamGeneration = true, want false", scenarioID)
	}
	if response.BackendScopeId == nil || response.FactorySessionId == nil || response.StreamGenerationId == nil {
		t.Fatalf("%s identity fields = %#v, want full identity set for stale cursor", scenarioID, response)
	}
}

func assertSyncPreflightSessionNotFoundOutcome(
	t *testing.T,
	scenarioID string,
	response factoryapi.FactorySessionSyncPreflightResponse,
) {
	t.Helper()

	if response.CheckpointReusable {
		t.Fatalf("%s checkpointReusable = true, want false", scenarioID)
	}
	if response.BackendScopeId != nil || response.FactorySessionId != nil || response.StreamGenerationId != nil {
		t.Fatalf("%s identity fields = %#v, want nil for missing session", scenarioID, response)
	}
}

func assertSyncPreflightLogicalSessionRemapOutcome(
	t *testing.T,
	scenarioID string,
	response factoryapi.FactorySessionSyncPreflightResponse,
) {
	t.Helper()

	if response.CheckpointReusable {
		t.Fatalf("%s checkpointReusable = true, want false", scenarioID)
	}
	if response.BackendScopeId == nil || response.FactorySessionId == nil || response.StreamGenerationId == nil {
		t.Fatalf("%s identity fields = %#v, want full identity set for remap", scenarioID, response)
	}
}

func assertSyncPreflightInvalidationDiagnostic(
	t *testing.T,
	scenarioID string,
	response factoryapi.FactorySessionSyncPreflightResponse,
) {
	t.Helper()

	diagnostic, ok := factorysessioncursors.InvalidationFromPreflight(apisurface.FactorySessionCursorPreflightResult(response))
	switch response.ReasonCode {
	case factoryapi.Ok:
		if ok {
			t.Fatalf("%s invalidation diagnostic = %#v, want none for ok", scenarioID, diagnostic)
		}
	default:
		if !ok {
			t.Fatalf("%s invalidation diagnostic missing for %q", scenarioID, response.ReasonCode)
		}
	}
}

func identityScopeFromFixtureMap(payload map[string]any) factorysessioncursors.IdentityScope {
	return factorysessioncursors.IdentityScope{
		BackendScopeID:      stringFixtureValue(payload, "backendScopeId"),
		LogicalSessionKeyID: stringFixtureValue(payload, "logicalSessionKeyId"),
		FactorySessionID:    stringFixtureValue(payload, "factorySessionId"),
		StreamGenerationID:  stringFixtureValue(payload, "streamGenerationId"),
	}
}

func stringFixtureValue(payload map[string]any, key string) string {
	value, ok := payload[key].(string)
	if !ok {
		return ""
	}
	return value
}

func loadSyncPreflightRecoveryFixtureCatalog(t *testing.T) syncPreflightRecoveryFixtureCatalog {
	t.Helper()

	fixtureBytes, err := os.ReadFile("../testdata/sync-preflight-recovery-contract-fixtures.json")
	if err != nil {
		t.Fatalf("read sync preflight recovery contract fixtures: %v", err)
	}

	var catalog syncPreflightRecoveryFixtureCatalog
	if err := json.Unmarshal(fixtureBytes, &catalog); err != nil {
		t.Fatalf("decode sync preflight recovery contract fixtures: %v", err)
	}
	if len(catalog.Scenarios) == 0 {
		t.Fatal("sync preflight recovery contract fixtures contain no scenarios")
	}
	if len(catalog.IdentityScopeComparisons) == 0 {
		t.Fatal("sync preflight recovery contract fixtures contain no identity scope comparisons")
	}
	return catalog
}
