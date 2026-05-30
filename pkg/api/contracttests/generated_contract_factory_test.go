package apicontract_test

import (
	"encoding/json"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
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
	submitRequest.Name = "task-1"
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
		{"type":"image","stagedFileRef":"staged://work-item.png","fileName":"work-item.png","mediaType":"image/png"}
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

	if submitRequest.Name == "" || submitRequest.WorkTypeName == "" || submitResponse.TraceId == "" || workRequest.RequestId == "" || upsertResponse.RequestId == "" || namedFactory.Name == "" || namedFactory.Workstations == nil || workstation.Behavior == nil || workstation.Type == nil || cron.Schedule == "" || cron.TriggerAtStart == nil {
		t.Fatal("generated OpenAPI request and response types should be usable")
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
		{"type":"image","file":"fixtures/review.png"}
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

	saveRequest := factoryapi.SaveCurrentFactoryBySessionIdJSONRequestBody{
		Factory: namedFactory,
	}
	current := namedFactory
	badRequest := factoryapi.CreateFactoryBadRequest{
		Code:    factoryapi.INVALIDFACTORYNAME,
		Family:  factoryapi.ErrorFamilyBadRequest,
		Message: "factory name must use lowercase letters, numbers, and hyphens",
	}
	conflict := factoryapi.CreateFactoryConflict{
		Code:    factoryapi.FACTORYALREADYEXISTS,
		Family:  factoryapi.ErrorFamilyConflict,
		Message: "factory already exists",
	}

	if saveRequest.Factory.Name == "" || saveRequest.Factory.WorkTypes == nil || saveRequest.Factory.Workers == nil || saveRequest.Factory.Workstations == nil {
		t.Fatal("generated session factory save request and response types should be usable")
	}
	if current.Name == "" || current.Workstations == nil {
		t.Fatal("generated current named-factory response type should be usable")
	}
	if badRequest.Code != factoryapi.INVALIDFACTORYNAME || badRequest.Family != factoryapi.ErrorFamilyBadRequest {
		t.Fatalf("generated bad-request contract = %#v, want code %q and family %q", badRequest, factoryapi.INVALIDFACTORYNAME, factoryapi.ErrorFamilyBadRequest)
	}
	if conflict.Code != factoryapi.FACTORYALREADYEXISTS || conflict.Family != factoryapi.ErrorFamilyConflict {
		t.Fatalf("generated conflict contract = %#v, want code %q and family %q", conflict, factoryapi.FACTORYALREADYEXISTS, factoryapi.ErrorFamilyConflict)
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
	if strings.Contains(string(encoded), `"factory_name"`) {
		t.Fatalf("generated NamedFactory JSON contains unexpected legacy field: %s", encoded)
	}
}

func assertGeneratedNamedFactoryRoundTripFields(t *testing.T, namedFactory factoryapi.Factory, roundTripped factoryapi.Factory) {
	t.Helper()

	if roundTripped.Name != namedFactory.Name {
		t.Fatalf("round-tripped named factory name = %q, want %q", roundTripped.Name, namedFactory.Name)
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
		Code:    factoryapi.NOTFOUND,
		Family:  factoryapi.ErrorFamilyNotFound,
		Message: "current factory not found",
	}
	if notFound.Code != factoryapi.NOTFOUND || notFound.Family != factoryapi.ErrorFamilyNotFound {
		t.Fatalf("generated not-found contract = %#v, want code %q and family %q", notFound, factoryapi.NOTFOUND, factoryapi.ErrorFamilyNotFound)
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
