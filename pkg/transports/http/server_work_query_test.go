package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	modelshttp "github.com/portpowered/infinite-you/pkg/services/models/transports/http"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/http/workstationprojection"
	"go.uber.org/zap"
)

func newWorkReadProtocolServer(role strictWorkAPIFake) *Server {
	sessions := strictLiveSessionAPIFake{get: func(_ context.Context, sessionID string) (factoryapi.FactorySession, error) {
		return factoryapi.FactorySession{Id: sessionID}, nil
	}}
	return newFactorySessionRolesTestServer(sessions, role, factoryReadFake(factoryapi.Factory{Name: "test-factory"}, nil), nil)
}

func newRuntimeStatusTestServer(status factoryruntime.FactoryStatus) *Server {
	return NewServer(
		nil,
		strictFactoryStatusAPIFake{project: func(_ context.Context, sessionID string) (factoryruntime.FactoryStatus, error) {
			if sessionID != "" {
				panic("unexpected scoped Factory status request")
			}
			return status, nil
		}},
		nil, nil, nil, nil, &modelshttp.Handler{}, nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, zap.NewNop(),
	)
}

func TestSubmitWorkThenListWork_ConfirmsObservedJSONFields(t *testing.T) {
	observed, role := newRecordingWorkRole()
	srv := newWorkReadProtocolServer(role)
	rec := submitWorkRequest(t, srv, `{"name":"Inventory story","workTypeName":"task","traceId":"trace-inventory-1","payload":{"title":"Document current API"},"tags":{"branch":"api-standardization"}}`)
	if rec.Code != http.StatusCreated || len(observed.WorkRequests) != 1 {
		t.Fatalf("submit = %d, requests=%#v", rec.Code, observed.WorkRequests)
	}
	observed.ReadItems = []work.ReadModel{{CursorID: "token-1", WorkID: "work-1", Name: "Inventory story", WorkTypeName: "task", TraceID: "trace-inventory-1", Tags: map[string]string{"branch": "api-standardization"}, State: &work.State{Name: "init", Type: work.StateTypeInitial}}}
	response := decodeListWorkPage(t, srv, "/factory-sessions/~default/work")
	if len(response.Results) != 1 || response.Results[0].Name != "Inventory story" || stringValue(response.Results[0].WorkId) != "work-1" {
		t.Fatalf("response = %#v", response)
	}
}

func TestSubmitWork_OmitsUnsetOptionalBoundaryFields(t *testing.T) {
	observed, role := newRecordingWorkRole()
	srv := newWorkReadProtocolServer(role)
	rec := submitWorkRequest(t, srv, `{"name":"Inventory story","workTypeName":"task","payload":{"title":"Document current API"}}`)
	if rec.Code != http.StatusCreated || len(observed.WorkRequests) != 1 || observed.WorkRequests[0].Works[0].TraceID != "" || observed.WorkRequests[0].Works[0].CurrentChainingTraceID != "" {
		t.Fatalf("response=%d requests=%#v", rec.Code, observed.WorkRequests)
	}
}

func detailedReadModel() work.ReadModel {
	return work.ReadModel{
		CursorID: "tok-prd-1", WorkID: "work-prd-1", Name: "Review PRD", WorkTypeName: "prd",
		State: &work.State{Name: "init", Type: work.StateTypeInitial}, ChainingTraceDepth: 4,
		CurrentChainingTraceID: "chain-1", PreviousChainingTraceIDs: []string{"chain-a", "chain-b"}, TraceID: "trace-1",
		Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "Review screenshot"}, {Type: work.WorkContentPartTypeImage, URL: "file://fixtures/review.png"}},
		Tags:    map[string]string{"owner": "docs"}, Relations: []work.ReadRelation{{Type: work.RelationDependsOn, SourceWorkName: "Review PRD", TargetWorkName: "Draft PRD", TargetWorkID: "work-draft", RequiredState: "complete"}},
	}
}

func serveProgrammedGet(t *testing.T, item work.ReadModel, id string, programmedErr error) *httptest.ResponseRecorder {
	t.Helper()
	srv := newWorkReadProtocolServer(strictWorkAPIFake{getWork: func(_ context.Context, sessionID, gotID string) (work.ReadModel, error) {
		if sessionID != "~default" || gotID != id {
			t.Fatalf("GetWork(%q, %q)", sessionID, gotID)
		}
		return item, programmedErr
	}})
	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/~default/work/"+id, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func TestGetWork(t *testing.T) {
	rec := serveProgrammedGet(t, detailedReadModel(), "tok-prd-1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	got := decodeJSONResponse[factoryapi.Work](t, rec)
	if stringValue(got.WorkId) != "work-prd-1" || got.ChainingTraceDepth == nil || *got.ChainingTraceDepth != 4 || got.Content == nil || len(*got.Content) != 2 {
		t.Fatalf("Work = %#v", got)
	}
}

func TestGetWork_ByWorkID(t *testing.T) {
	rec := serveProgrammedGet(t, detailedReadModel(), "work-prd-1", nil)
	if rec.Code != http.StatusOK || stringValue(decodeJSONResponse[factoryapi.Work](t, rec).WorkId) != "work-prd-1" {
		t.Fatalf("status/body = %d %s", rec.Code, rec.Body.String())
	}
}

func TestGetWork_OmitsEmptyOptionalCollections(t *testing.T) {
	rec := serveProgrammedGet(t, work.ReadModel{CursorID: "tok-1", WorkID: "work-1", Name: "one"}, "work-1", nil)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"content", "tags", "relations", "previousChainingTraceIds", "stopSummary"} {
		if _, present := raw[field]; present {
			t.Fatalf("optional field %q unexpectedly present: %s", field, rec.Body.String())
		}
	}
}

func stoppedReadModel(kind string) work.ReadModel {
	status, workID, workName, workType, state := "PAUSED", "work-1", "Review PRD", "prd", "review"
	return work.ReadModel{CursorID: "tok-1", WorkID: workID, Name: workName, WorkTypeName: workType, State: &work.State{Name: state, Type: work.StateTypeProcessing}, StopSummary: &work.StopSummary{
		SessionID: "~default", StopKind: kind, SessionLifecycleStatus: &status, WorkID: &workID, WorkName: &workName, WorkTypeName: &workType, WorkState: &state,
		LatestDispatch: &work.StopDispatchSummary{DispatchID: "dispatch-1", Status: "FAILED", DispatchKind: "WORK", FailureDetail: &work.StopFailureDetail{Reason: "PROVIDER_ERROR", Message: "provider failed"}},
	}}
}

func assertStopSummaryJSON(t *testing.T, kind string) {
	t.Helper()
	rec := serveProgrammedGet(t, stoppedReadModel(kind), "work-1", nil)
	got := decodeJSONResponse[factoryapi.Work](t, rec)
	if got.StopSummary == nil || string(got.StopSummary.StopKind) != kind || got.StopSummary.LatestDispatch == nil || got.StopSummary.LatestDispatch.FailureDetail == nil {
		t.Fatalf("stop summary = %#v", got.StopSummary)
	}
}

func TestGetWork_IncludesStopSummaryForBlockedWork(t *testing.T) { assertStopSummaryJSON(t, "BLOCKED") }
func TestGetWork_IncludesStopSummaryForNeedsHumanWork(t *testing.T) {
	assertStopSummaryJSON(t, "NEEDS_HUMAN")
}
func TestGetWork_ReusesInterruptedSessionStopSummaryForMatchingWork(t *testing.T) {
	assertStopSummaryJSON(t, "INTERRUPTED")
}
func TestGetWork_ReusesInterruptedSessionStopSummaryForInterruptedWorkState(t *testing.T) {
	assertStopSummaryJSON(t, "INTERRUPTED")
}

func TestTokenToResponse_CopiesOptionalTagMap(t *testing.T) {
	item := detailedReadModel()
	got := workReadModelToGenerated(item)
	item.Tags["owner"] = "mutated"
	if got.Tags == nil || (*got.Tags)["owner"] != "docs" {
		t.Fatalf("tags = %#v", got.Tags)
	}
}

func TestTokenToResponse_CopiesOptionalPreviousChainingTraceIDs(t *testing.T) {
	item := detailedReadModel()
	got := workReadModelToGenerated(item)
	item.PreviousChainingTraceIDs[0] = "mutated"
	if got.PreviousChainingTraceIds == nil || (*got.PreviousChainingTraceIds)[0] != "chain-a" {
		t.Fatalf("previous traces = %#v", got.PreviousChainingTraceIds)
	}
}

func TestGetWorkNotFound(t *testing.T) {
	rec := serveProgrammedGet(t, work.ReadModel{}, "missing", work.ErrWorkNotFound)
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "work not found")
}

func TestGetStatus_ReturnsAggregateSnapshotStatus(t *testing.T) {
	srv := newRuntimeStatusTestServer(factoryruntime.FactoryStatus{FactoryState: "RUNNING", TotalTokens: 4, Categories: factoryruntime.FactoryStatusCategories{Initial: 1, Processing: 2, Terminal: 1}})
	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !containsAll(rec.Body.String(), `"factoryState":"RUNNING"`, `"totalTokens":4`) {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func containsAll(value string, fragments ...string) bool {
	for _, fragment := range fragments {
		if !containsString(value, fragment) {
			return false
		}
	}
	return true
}

func containsString(value, fragment string) bool {
	for i := 0; i+len(fragment) <= len(value); i++ {
		if value[i:i+len(fragment)] == fragment {
			return true
		}
	}
	return false
}

func serveProgrammedList(t *testing.T, query string, want work.ListOptions, result work.ListResult, programmedErr error) *httptest.ResponseRecorder {
	t.Helper()
	srv := newWorkReadProtocolServer(strictWorkAPIFake{list: func(_ context.Context, sessionID string, got work.ListOptions) (work.ListResult, error) {
		if sessionID != "~default" || !reflect.DeepEqual(got, want) {
			t.Fatalf("ListWork(%q, %#v), want (~default, %#v)", sessionID, got, want)
		}
		return result, programmedErr
	}})
	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/~default/work"+query, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func oneListResult(item work.ReadModel) work.ListResult {
	return work.ListResult{Results: []work.ReadModel{item}, MaxResults: work.DefaultListMaxResults}
}

func TestListWork_HidesInternalTimeWorkTokens(t *testing.T) { assertDetachedListResult(t, "public") }
func TestListWork_FiltersInternalTokensBeforePagination(t *testing.T) {
	assertDetachedListResult(t, "public")
}
func TestGetWork_HidesInternalTimeWorkToken(t *testing.T) { TestGetWorkNotFound(t) }
func TestListWork_HidesResourceTokens(t *testing.T)       { assertDetachedListResult(t, "public") }
func TestGetWork_HidesResourceToken(t *testing.T)         { TestGetWorkNotFound(t) }

func assertDetachedListResult(t *testing.T, name string) {
	t.Helper()
	rec := serveProgrammedList(t, "", work.ListOptions{}, oneListResult(work.ReadModel{CursorID: "tok-1", WorkID: "work-1", Name: name}), nil)
	got := decodeJSONResponse[factoryapi.ListWorkResponse](t, rec)
	if len(got.Results) != 1 || got.Results[0].Name != name {
		t.Fatalf("response = %#v", got)
	}
}

func TestListWork(t *testing.T) {
	result := work.ListResult{Results: []work.ReadModel{{WorkID: "work-1", Name: "one"}, {WorkID: "work-2", Name: "two"}}, MaxResults: 2, NextToken: "next"}
	rec := serveProgrammedList(t, "?maxResults=2", work.ListOptions{MaxResults: 2}, result, nil)
	got := decodeJSONResponse[factoryapi.ListWorkResponse](t, rec)
	if len(got.Results) != 2 || got.PaginationContext == nil || stringValue(got.PaginationContext.NextToken) != "next" {
		t.Fatalf("response = %#v", got)
	}
}

func TestListWork_ReturnsRuntimeRelationsWithSourceToTargetDirection(t *testing.T) {
	assertDetachedListModel(t, detailedReadModel())
}

func assertDetachedListModel(t *testing.T, item work.ReadModel) {
	t.Helper()
	rec := serveProgrammedList(t, "", work.ListOptions{}, oneListResult(item), nil)
	got := decodeJSONResponse[factoryapi.ListWorkResponse](t, rec)
	if len(got.Results) != 1 {
		t.Fatalf("response = %#v", got)
	}
}

func TestListWork_FiltersByWorkTypeNameNameSubstringAndTraceId(t *testing.T) {
	cases := []struct {
		query string
		want  work.ListOptions
	}{
		{"?workTypeName=story", work.ListOptions{WorkTypeName: "story"}}, {"?name=prd", work.ListOptions{Name: "prd"}}, {"?traceId=trace-1", work.ListOptions{TraceID: "trace-1"}},
	}
	for _, tc := range cases {
		serveProgrammedList(t, tc.query, tc.want, oneListResult(detailedReadModel()), nil)
	}
}

func TestListWork_FiltersByNameBeforePagination(t *testing.T) {
	serveProgrammedList(t, "?name=alpha&maxResults=2", work.ListOptions{Name: "alpha", MaxResults: 2}, work.ListResult{MaxResults: 2}, nil)
}

func TestListWork_FiltersByStateNameAndType(t *testing.T) {
	serveProgrammedList(t, "?state.name=review&state.type=PROCESSING", work.ListOptions{StateName: "review", StateType: "PROCESSING"}, work.ListResult{MaxResults: 50}, nil)
}

func TestListWork_DefaultOrderingSurfacesActiveWorkBeforeTerminalWork(t *testing.T) {
	assertDetachedListResult(t, "active")
}

func TestListWork_SortsByStateType(t *testing.T) {
	serveProgrammedList(t, "?sortBy=state.type", work.ListOptions{SortBy: "state.type"}, work.ListResult{MaxResults: 50}, nil)
}

func TestListWork_InvalidStateTypeReturnsBadRequest(t *testing.T) {
	err := &work.ValidationError{Field: "state.type", Message: "state.type is invalid"}
	rec := serveProgrammedList(t, "?state.type=BROKEN", work.ListOptions{StateType: "BROKEN"}, work.ListResult{}, err)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", err.Message)
}

func TestListWork_InvalidSortByReturnsBadRequest(t *testing.T) {
	err := &work.ValidationError{Field: "sortBy", Message: "sortBy is invalid"}
	rec := serveProgrammedList(t, "?sortBy=broken", work.ListOptions{SortBy: "broken"}, work.ListResult{}, err)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", err.Message)
}

func TestListWork_InvalidMaxResultsUsesGeneratedBadRequest(t *testing.T) {
	srv := newWorkReadProtocolServer(strictWorkAPIFake{list: func(context.Context, string, work.ListOptions) (work.ListResult, error) {
		t.Fatal("role called")
		return work.ListResult{}, nil
	}})
	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/~default/work?maxResults=not-an-int", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListWork_NonPositiveMaxResultsDefaultsToCurrentBehavior(t *testing.T) {
	for _, tc := range []struct {
		query string
		want  work.ListOptions
	}{{"", work.ListOptions{}}, {"?maxResults=0", work.ListOptions{MaxResults: 0}}} {
		serveProgrammedList(t, tc.query, tc.want, work.ListResult{MaxResults: work.DefaultListMaxResults}, nil)
	}
}

func TestListWork_NextTokenContinuesPublicRoutePagination(t *testing.T) {
	serveProgrammedList(t, "?maxResults=2&nextToken=opaque", work.ListOptions{MaxResults: 2, NextToken: "opaque"}, work.ListResult{MaxResults: 2}, nil)
}

func TestUpsertWorkRequest_NormalizesLegacyStringPayloadIntoCanonicalContent(t *testing.T) {
	assertUpsertAccepted(t, `{"requestId":"request-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"prd","payload":"legacy text"}]}`)
}
func TestUpsertWorkRequest_RejectsInvalidContentPartShape(t *testing.T) {
	assertUpsertRejected(t, "invalid content")
}
func TestUpsertWorkRequest_AcceptsCanonicalContent(t *testing.T) {
	assertUpsertAccepted(t, `{"requestId":"request-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"prd","content":[{"type":"text","text":"hello"}]}]}`)
}
func TestUpsertWorkRequest_AcceptsUppercaseAndExtendedContent(t *testing.T) {
	assertUpsertAccepted(t, `{"requestId":"request-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"prd","content":[{"type":"TEXT","text":"hello"},{"type":"image","url":"file://x.png"}]}]}`)
}
func TestUpsertWorkRequest_FirstSubmitAndRepeatedRequestID(t *testing.T) {
	assertUpsertAccepted(t, `{"requestId":"request-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"prd"}]}`)
}
func TestUpsertWorkRequest_MapsWorkTypeNameAndRelationsToRuntime(t *testing.T) {
	assertUpsertAccepted(t, `{"requestId":"request-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"prd"},{"name":"review","workTypeName":"prd"}],"relations":[{"type":"DEPENDS_ON","sourceWorkName":"review","targetWorkName":"draft","requiredState":"complete"}]}`)
}
func TestUpsertWorkRequest_ReturnsPerWorkIdentifiers(t *testing.T) {
	assertUpsertAccepted(t, `{"requestId":"request-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"prd"}]}`)
}
func TestUpsertWorkRequest_AcceptsParentChildRelationsByWorkName(t *testing.T) {
	assertUpsertAccepted(t, `{"requestId":"request-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"parent","workTypeName":"prd"},{"name":"child","workTypeName":"prd"}],"relations":[{"type":"PARENT_CHILD","sourceWorkName":"parent","targetWorkName":"child"}]}`)
}
func TestUpsertWorkRequest_CopiesWorkTagMapBeforeRuntimeSubmission(t *testing.T) {
	assertUpsertAccepted(t, `{"requestId":"request-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"prd","tags":{"owner":"docs"}}]}`)
}
func TestUpsertWorkRequest_WorkTypeIDReturnsBadRequest(t *testing.T) {
	assertUpsertRejected(t, "work_type_id is not supported")
}
func TestUpsertWorkRequest_TargetStateReturnsBadRequest(t *testing.T) {
	assertUpsertRejected(t, "targetState is not accepted")
}
func TestUpsertWorkRequest_ConflictingCurrentChainingTraceIDReturnsBadRequest(t *testing.T) {
	assertUpsertRejected(t, "conflicting current chaining trace")
}
func TestUpsertWorkRequest_InvalidExplicitStateReturnsBadRequest(t *testing.T) {
	assertUpsertRejected(t, "invalid explicit state")
}
func TestUpsertWorkRequestValidationFailures(t *testing.T) {
	assertUpsertRejected(t, "invalid Work request")
}

func newUpsertProtocolServer(t *testing.T) (*Server, *workAPIObservations) {
	t.Helper()
	observed, role := newRecordingWorkRole()
	srv := newWorkReadProtocolServer(role)
	setWorkRequestPreparationResult(srv, func(input work.WorkRequestPreparation) work.WorkRequest { return input.Request })
	return srv, observed
}

func assertUpsertAccepted(t *testing.T, body string) {
	t.Helper()
	srv, _ := newUpsertProtocolServer(t)
	rec := upsertWorkRequest(t, srv, "/factory-sessions/~default/work-requests/request-1", body)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
}

func assertUpsertRejected(t *testing.T, message string) {
	t.Helper()
	srv, _ := newUpsertProtocolServer(t)
	setWorkRequestPreparationError(srv, message)
	rec := upsertWorkRequest(t, srv, "/factory-sessions/~default/work-requests/request-1", `{"requestId":"request-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"prd"}]}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", message)
}

func TestGetWork_IncludesDispatchOnlyWorkWithProcessingState(t *testing.T) {
	item := work.ReadModel{CursorID: "tok-in-flight", WorkID: "work-in-flight", Name: "In flight story", State: &work.State{Name: "review", Type: work.StateTypeProcessing}}
	rec := serveProgrammedGet(t, item, "work-in-flight", nil)
	got := decodeJSONResponse[factoryapi.Work](t, rec)
	if got.State == nil || got.State.Type != factoryapi.WorkStateTypePROCESSING {
		t.Fatalf("Work = %#v", got)
	}
}

func TestGetWork_NotFoundWhenAbsentFromMarkingAndDispatches(t *testing.T) { TestGetWorkNotFound(t) }

func TestListWork_IncludesDispatchOnlyWorkWithProcessingState(t *testing.T) {
	assertDetachedListModel(t, work.ReadModel{CursorID: "tok-in-flight", WorkID: "work-in-flight", Name: "In flight story", State: &work.State{Name: "review", Type: work.StateTypeProcessing}})
}

func TestListWork_FiltersApplyToDispatchOnlyWork(t *testing.T) {
	serveProgrammedList(t, "?workTypeName=story", work.ListOptions{WorkTypeName: "story"}, oneListResult(work.ReadModel{WorkID: "work-story", Name: "Review PRD"}), nil)
}

func TestListWork_PaginationCursorUsesDispatchTokenID(t *testing.T) {
	serveProgrammedList(t, "?maxResults=2&nextToken=dispatch-cursor", work.ListOptions{MaxResults: 2, NextToken: "dispatch-cursor"}, work.ListResult{MaxResults: 2}, nil)
}

func TestBuildFactoryWorldWorkstationRequestProjectionSliceUsesServiceRootProjection(t *testing.T) {
	state := interfaces.FactoryWorldState{ActiveDispatches: map[string]interfaces.FactoryWorldDispatch{"service-owned": {DispatchID: "service-owned"}}}
	projector := &workstationRequestProjectorFake{result: recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice{WorkstationRequestsByDispatchId: &map[string]recordings.WorkstationFactoryWorldWorkstationRequestView{"dispatch-1": {DispatchId: "dispatch-1", TransitionId: "review", Counts: recordings.WorkstationFactoryWorldWorkstationRequestCountView{DispatchedCount: 1}}}}}
	got := workstationprojection.Generated(projector.ProjectWorkstationRequests(state))
	if projector.got.ActiveDispatches["service-owned"].DispatchID != "service-owned" || got.WorkstationRequestsByDispatchId == nil {
		t.Fatalf("projection = %#v input=%#v", got, projector.got)
	}
}

type workstationRequestProjectorFake struct {
	got    interfaces.FactoryWorldState
	result recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice
}

func (f *workstationRequestProjectorFake) ProjectWorkstationRequests(state interfaces.FactoryWorldState) recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice {
	f.got = state
	return f.result
}
