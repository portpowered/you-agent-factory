package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestSubmitWorkThenListWork_ConfirmsObservedJSONFields(t *testing.T) {
	now := time.Date(2026, 4, 12, 16, 30, 0, 0, time.UTC)
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	submitted := assertSubmitThenListWorkRequest(t, srv, mf)
	assertSubmitThenListWorkListing(t, srv, mf, submitted, now)
}

func assertSubmitThenListWorkRequest(t *testing.T, srv *Server, mf *testutil.MockFactory) interfaces.SubmitRequest {
	t.Helper()

	rec := submitWorkRequest(t, srv, `{"name":"Inventory story","workTypeName":"task","traceId":"trace-inventory-1","payload":{"title":"Document current API"},"tags":{"branch":"api-standardization"}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /work status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSONResponse[factoryapi.SubmitWorkResponse](t, rec)
	if resp.TraceId != "trace-inventory-1" || len(mf.Submitted) != 1 {
		t.Fatalf("submit response = %#v submitted=%#v", resp, mf.Submitted)
	}
	submitted := mf.Submitted[0]
	if submitted.Name != "Inventory story" || submitted.WorkTypeID != "task" || submitted.TraceID != "trace-inventory-1" {
		t.Fatalf("submitted request = %#v, want name/work type/trace from JSON body", submitted)
	}
	return submitted
}

func assertSubmitThenListWorkListing(t *testing.T, srv *Server, mf *testutil.MockFactory, submitted interfaces.SubmitRequest, now time.Time) {
	t.Helper()

	mf.Marking.Tokens["tok-inventory-1"] = &interfaces.Token{
		ID:      "tok-inventory-1",
		PlaceID: "task:init",
		Color: interfaces.TokenColor{
			Name:       submitted.Name,
			WorkID:     "work-inventory-1",
			WorkTypeID: submitted.WorkTypeID,
			TraceID:    submitted.TraceID,
			Content: []interfaces.WorkContentPart{
				{Type: interfaces.WorkContentPartTypeText, Text: "Inspect this"},
				{Type: interfaces.WorkContentPartTypeImage, File: "fixtures/inventory.png"},
			},
			Tags: submitted.Tags,
		},
		CreatedAt: now,
		EnteredAt: now,
	}
	mf.Net = &state.Net{
		Places:    map[string]*petri.Place{"task:init": {ID: "task:init", TypeID: "task", State: "init"}},
		WorkTypes: map[string]*state.WorkType{"task": {ID: "task", States: []state.StateDefinition{{Value: "init", Category: state.StateCategoryInitial}}}},
	}

	req := httptest.NewRequest(http.MethodGet, "/work", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /work status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	listResp := decodeJSONResponse[factoryapi.ListWorkResponse](t, rec)
	if len(listResp.Results) != 1 {
		t.Fatalf("work result count = %d, want 1", len(listResp.Results))
	}
	work := listResp.Results[0]
	if work.Name != "Inventory story" || stringValue(work.WorkId) != "work-inventory-1" || stringValue(work.WorkTypeName) != "task" || stringValue(work.TraceId) != "trace-inventory-1" {
		t.Fatalf("work = %#v, want canonical identity fields", work)
	}
	if work.State == nil || work.State.Name != "init" || work.State.Type != factoryapi.WorkStateTypeINITIAL {
		t.Fatalf("state = %#v, want init/INITIAL", work.State)
	}
	assertGeneratedWorkContentParts(t, work.Content, []interfaces.WorkContentPart{
		{Type: interfaces.WorkContentPartTypeText, Text: "Inspect this"},
		{Type: interfaces.WorkContentPartTypeImage, File: "fixtures/inventory.png"},
	})
	if work.Tags == nil || (*work.Tags)["branch"] != "api-standardization" {
		t.Fatalf("tags = %#v, want branch api-standardization", work.Tags)
	}
}

func TestGetWork(t *testing.T) {
	now := time.Now()
	srv := newTestServer(&testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{Tokens: map[string]*interfaces.Token{
			"tok-prd-1": {
				ID:      "tok-prd-1",
				PlaceID: "prd:init",
				Color: interfaces.TokenColor{
					WorkID:                   "work-prd-1",
					WorkTypeID:               "prd",
					ChainingTraceDepth:       4,
					CurrentChainingTraceID:   "chain-1",
					PreviousChainingTraceIDs: []string{"chain-a", "chain-b"},
					TraceID:                  "trace-1",
					Content:                  []interfaces.WorkContentPart{{Type: interfaces.WorkContentPartTypeText, Text: "Review screenshot"}, {Type: interfaces.WorkContentPartTypeImage, File: "fixtures/review.png"}},
				},
				CreatedAt: now,
				EnteredAt: now,
				History: interfaces.TokenHistory{
					TotalVisits:         map[string]int{"execute": 1},
					ConsecutiveFailures: make(map[string]int),
					PlaceVisits:         map[string]int{"prd:init": 1},
				},
			},
		}},
	})

	req := httptest.NewRequest("GET", "/work/tok-prd-1", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSONResponse[factoryapi.TokenResponse](t, rec)
	if resp.Id != "tok-prd-1" || resp.PlaceId != "prd:init" {
		t.Fatalf("token response = %#v, want tok-prd-1 at prd:init", resp)
	}
	if resp.ChainingTraceDepth == nil || *resp.ChainingTraceDepth != 4 || resp.CurrentChainingTraceId == nil || *resp.CurrentChainingTraceId != "chain-1" || resp.PreviousChainingTraceIds == nil || len(*resp.PreviousChainingTraceIds) != 2 {
		t.Fatalf("chaining trace fields = %#v, want preserved trace lineage", resp)
	}
	assertGeneratedWorkContentParts(t, resp.Content, []interfaces.WorkContentPart{
		{Type: interfaces.WorkContentPartTypeText, Text: "Review screenshot"},
		{Type: interfaces.WorkContentPartTypeImage, File: "fixtures/review.png"},
	})
	if resp.History == nil || resp.History.TotalVisits == nil || (*resp.History.TotalVisits)["execute"] != 1 {
		t.Error("expected history in single token response")
	}
}

func TestGetWorkNotFound(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}})
	req := httptest.NewRequest("GET", "/work/nonexistent", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "token not found")
}

func TestGetStatus_ReturnsAggregateSnapshotStatus(t *testing.T) {
	now := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	snapshot := &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
		RuntimeStatus: interfaces.RuntimeStatusActive,
		FactoryState:  string(interfaces.FactoryStateRunning),
		Topology: &state.Net{
			Places: map[string]*petri.Place{
				"task:init":            {ID: "task:init", TypeID: "task", State: "init"},
				"task:review":          {ID: "task:review", TypeID: "task", State: "review"},
				"task:complete":        {ID: "task:complete", TypeID: "task", State: "complete"},
				"task:failed":          {ID: "task:failed", TypeID: "task", State: "failed"},
				"agent-slot:available": {ID: "agent-slot:available", TypeID: "agent-slot", State: "available"},
			},
			WorkTypes: map[string]*state.WorkType{"task": {ID: "task", States: []state.StateDefinition{{Value: "init", Category: state.StateCategoryInitial}, {Value: "review", Category: state.StateCategoryProcessing}, {Value: "complete", Category: state.StateCategoryTerminal}, {Value: "failed", Category: state.StateCategoryFailed}}}},
			Resources: map[string]*state.ResourceDef{"agent-slot": {ID: "agent-slot", Name: "agent-slot", Capacity: 2}},
		},
		Marking: petri.MarkingSnapshot{Tokens: map[string]*interfaces.Token{
			"tok-init":              {ID: "tok-init", PlaceID: "task:init", Color: interfaces.TokenColor{WorkID: "work-init", WorkTypeID: "task"}, CreatedAt: now, EnteredAt: now},
			"tok-review":            {ID: "tok-review", PlaceID: "task:review", Color: interfaces.TokenColor{WorkID: "work-review", WorkTypeID: "task"}, CreatedAt: now, EnteredAt: now},
			"tok-complete":          {ID: "tok-complete", PlaceID: "task:complete", Color: interfaces.TokenColor{WorkID: "work-complete", WorkTypeID: "task"}, CreatedAt: now, EnteredAt: now},
			"tok-failed":            {ID: "tok-failed", PlaceID: "task:failed", Color: interfaces.TokenColor{WorkID: "work-failed", WorkTypeID: "task"}, CreatedAt: now, EnteredAt: now},
			"agent-slot:resource:0": {ID: "agent-slot:resource:0", PlaceID: "agent-slot:available", Color: interfaces.TokenColor{DataType: interfaces.DataTypeResource}, CreatedAt: now, EnteredAt: now},
			"tok-time":              {ID: "tok-time", PlaceID: interfaces.SystemTimePendingPlaceID, Color: interfaces.TokenColor{WorkID: "time-daily-refresh", WorkTypeID: interfaces.SystemTimeWorkTypeID, TraceID: "trace-time"}, CreatedAt: now, EnteredAt: now},
		}},
	}
	srv := newTestServer(&testutil.MockFactory{EngineState: snapshot})

	req := httptest.NewRequest(http.MethodGet, "/status", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /status status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSONResponse[factoryapi.StatusResponse](t, rec)
	if resp.FactoryState != "RUNNING" || resp.RuntimeStatus != "ACTIVE" || resp.TotalTokens != 5 {
		t.Fatalf("status response = %#v, want RUNNING/ACTIVE with 5 tokens", resp)
	}
	if resp.Categories.Initial != 1 || resp.Categories.Processing != 1 || resp.Categories.Terminal != 1 || resp.Categories.Failed != 1 {
		t.Fatalf("categories = %#v, want one token in each category", resp.Categories)
	}
	if resp.Resources == nil || len(*resp.Resources) != 1 {
		t.Fatalf("resources = %#v, want one resource summary", resp.Resources)
	}
	resource := (*resp.Resources)[0]
	if resource.Name != "agent-slot" || resource.Available != 1 || resource.Total != 2 {
		t.Fatalf("resource = %#v, want agent-slot 1/2", resource)
	}
}

func TestListWork_HidesInternalTimeWorkTokens(t *testing.T) {
	now := time.Date(2026, 4, 18, 9, 0, 0, 0, time.UTC)
	srv := newTestServer(&testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: map[string]*interfaces.Token{
		"tok-story": {ID: "tok-story", PlaceID: "story:init", Color: interfaces.TokenColor{WorkID: "work-story", WorkTypeID: "story", TraceID: "trace-story"}, CreatedAt: now, EnteredAt: now},
		"tok-time":  {ID: "tok-time", PlaceID: interfaces.SystemTimePendingPlaceID, Color: interfaces.TokenColor{WorkID: "time-daily-refresh", WorkTypeID: interfaces.SystemTimeWorkTypeID, TraceID: "trace-time", Tags: map[string]string{interfaces.TimeWorkTagKeyCronWorkstation: "daily-refresh"}}, CreatedAt: now, EnteredAt: now},
	}}})

	req := httptest.NewRequest(http.MethodGet, "/work", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /work status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSONResponse[factoryapi.ListWorkResponse](t, rec)
	if len(resp.Results) != 1 || stringValue(resp.Results[0].WorkId) != "work-story" {
		t.Fatalf("listed work = %#v, want only customer work", resp.Results)
	}
	if resp.PaginationContext == nil || stringValue(resp.PaginationContext.NextToken) != "" {
		t.Fatalf("pagination context = %#v, want metadata without next token after internal token filtering", resp.PaginationContext)
	}
}

func TestListWork_FiltersInternalTokensBeforePagination(t *testing.T) {
	now := time.Date(2026, 4, 18, 9, 0, 0, 0, time.UTC)
	srv := newTestServer(&testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: map[string]*interfaces.Token{
		"tok-filter-1": {ID: "tok-filter-1", PlaceID: "story:init", Color: interfaces.TokenColor{WorkID: "work-filter-1", WorkTypeID: "story", TraceID: "trace-filter-1"}, CreatedAt: now, EnteredAt: now},
		"tok-filter-2": {ID: "tok-filter-2", PlaceID: interfaces.SystemTimePendingPlaceID, Color: interfaces.TokenColor{WorkID: "time-daily-refresh", WorkTypeID: interfaces.SystemTimeWorkTypeID, TraceID: "trace-filter-2"}, CreatedAt: now, EnteredAt: now},
		"tok-filter-3": {ID: "tok-filter-3", PlaceID: "story:init", Color: interfaces.TokenColor{WorkID: "work-filter-3", WorkTypeID: "story", TraceID: "trace-filter-3"}, CreatedAt: now, EnteredAt: now},
		"tok-filter-4": {ID: "tok-filter-4", PlaceID: "story:init", Color: interfaces.TokenColor{WorkID: "work-filter-4", WorkTypeID: "story", TraceID: "trace-filter-4"}, CreatedAt: now, EnteredAt: now},
	}}})

	firstResp := decodeListWorkPage(t, srv, "/work?maxResults=2")
	if len(firstResp.Results) != 2 || stringValue(firstResp.Results[0].WorkId) != "work-filter-1" || stringValue(firstResp.Results[1].WorkId) != "work-filter-3" || firstResp.PaginationContext == nil {
		t.Fatalf("first page = %#v, want public work before pagination", firstResp)
	}
	nextToken := stringValue(firstResp.PaginationContext.NextToken)
	if nextToken == "" {
		t.Fatal("expected first page nextToken")
	}

	secondResp := decodeListWorkPage(t, srv, "/work?maxResults=2&nextToken="+nextToken)
	if len(secondResp.Results) != 1 || stringValue(secondResp.Results[0].WorkId) != "work-filter-4" {
		t.Fatalf("second page listed work = %#v, want remaining public work", secondResp.Results)
	}
	if secondResp.PaginationContext == nil || stringValue(secondResp.PaginationContext.NextToken) != "" {
		t.Fatalf("second page pagination context = %#v, want metadata without next token on final page", secondResp.PaginationContext)
	}
}

func TestGetWork_HidesInternalTimeWorkToken(t *testing.T) {
	now := time.Date(2026, 4, 18, 9, 0, 0, 0, time.UTC)
	srv := newTestServer(&testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: map[string]*interfaces.Token{
		"tok-time": {ID: "tok-time", PlaceID: interfaces.SystemTimePendingPlaceID, Color: interfaces.TokenColor{WorkID: "time-daily-refresh", WorkTypeID: interfaces.SystemTimeWorkTypeID, TraceID: "trace-time"}, CreatedAt: now, EnteredAt: now},
	}}})
	req := httptest.NewRequest(http.MethodGet, "/work/tok-time", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "token not found")
}

func TestListWork_HidesResourceTokens(t *testing.T) {
	now := time.Date(2026, 4, 18, 9, 0, 0, 0, time.UTC)
	srv := newTestServer(&testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: map[string]*interfaces.Token{
		"tok-story":           {ID: "tok-story", PlaceID: "story:init", Color: interfaces.TokenColor{WorkID: "work-story", WorkTypeID: "story", TraceID: "trace-story"}, CreatedAt: now, EnteredAt: now},
		"agent-slot:resource": {ID: "agent-slot:resource", PlaceID: "agent-slot:available", Color: interfaces.TokenColor{DataType: interfaces.DataTypeResource, WorkID: "resource-work", WorkTypeID: "agent-slot"}, CreatedAt: now, EnteredAt: now},
	}}})

	req := httptest.NewRequest(http.MethodGet, "/work", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /work status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSONResponse[factoryapi.ListWorkResponse](t, rec)
	if len(resp.Results) != 1 || stringValue(resp.Results[0].WorkId) != "work-story" {
		t.Fatalf("listed work = %#v, want only non-resource work", resp.Results)
	}
}

func TestGetWork_HidesResourceToken(t *testing.T) {
	now := time.Date(2026, 4, 18, 9, 0, 0, 0, time.UTC)
	srv := newTestServer(&testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: map[string]*interfaces.Token{
		"agent-slot:resource": {ID: "agent-slot:resource", PlaceID: "agent-slot:available", Color: interfaces.TokenColor{DataType: interfaces.DataTypeResource, WorkID: "resource-work", WorkTypeID: "agent-slot"}, CreatedAt: now, EnteredAt: now},
	}}})
	req := httptest.NewRequest(http.MethodGet, "/work/agent-slot:resource", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "token not found")
}

func TestListWork(t *testing.T) {
	tokens := makeListWorkTokens("prd", 3, time.Now())
	srv := newTestServer(&testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: tokens}})
	resp := decodeListWorkPage(t, srv, "/work?maxResults=2")
	if len(resp.Results) != 2 || resp.PaginationContext == nil || stringValue(resp.PaginationContext.NextToken) == "" {
		t.Fatalf("list work response = %#v, want paginated first page", resp)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this list-work contract test keeps the relation, pagination, and status assertions together to preserve route-level intent.
func TestListWork_ReturnsRuntimeRelationsWithSourceToTargetDirection(t *testing.T) {
	now := time.Now()
	tokens := map[string]*interfaces.Token{
		"tok-1": listWorkToken("tok-1", "work-review", "task:init", "task", now),
		"tok-2": listWorkToken("tok-2", "work-draft", "task:init", "task", now),
		"tok-3": listWorkToken("tok-3", "work-parent", "task:init", "task", now),
		"tok-4": listWorkToken("tok-4", "work-standalone", "task:init", "task", now),
		"tok-5": listWorkToken("tok-5", "work-origin", "task:init", "task", now),
	}
	tokens["tok-1"].Color.Name = "review"
	tokens["tok-1"].Color.Relations = []interfaces.Relation{{Type: interfaces.RelationDependsOn, TargetWorkID: "work-draft", RequiredState: "complete"}, {Type: interfaces.RelationParentChild, TargetWorkID: "work-parent"}, {Type: interfaces.RelationSpawnedBy, TargetWorkID: "work-origin"}}
	tokens["tok-2"].Color.Name = "draft"
	tokens["tok-3"].Color.Name = "parent"
	tokens["tok-4"].Color.Name = "standalone"
	tokens["tok-5"].Color.Name = "origin"

	srv := newTestServer(&testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: tokens}, Net: listWorkFilterTopology()})
	resp := decodeListWorkPage(t, srv, "/work?state.name=init")
	review := listedWorkByID(t, resp.Results, "work-review")
	if review.Relations == nil || len(*review.Relations) != 3 {
		t.Fatalf("review relations = %#v, want runtime relations", review.Relations)
	}
	relations := *review.Relations
	if got := relations[0]; got.Type != factoryapi.RelationTypeDependsOn || got.SourceWorkName != "review" || got.TargetWorkName != "draft" || stringValue(got.TargetWorkId) != "work-draft" || stringValue(got.RequiredState) != "complete" {
		t.Fatalf("depends_on relation = %#v, want review -> draft complete", got)
	}
	if got := relations[1]; got.Type != factoryapi.RelationTypeParentChild || got.SourceWorkName != "review" || got.TargetWorkName != "parent" || stringValue(got.TargetWorkId) != "work-parent" || got.RequiredState != nil {
		t.Fatalf("parent_child relation = %#v, want review -> parent without required state", got)
	}
	if got := relations[2]; got.Type != factoryapi.RelationTypeSpawnedBy || got.SourceWorkName != "review" || got.TargetWorkName != "origin" || stringValue(got.TargetWorkId) != "work-origin" || got.RequiredState != nil {
		t.Fatalf("spawned_by relation = %#v, want review -> origin without required state", got)
	}
	if standalone := listedWorkByID(t, resp.Results, "work-standalone"); standalone.Relations != nil {
		t.Fatalf("standalone relations = %#v, want omitted relations", *standalone.Relations)
	}
}

func TestListWork_FiltersByStateNameAndType(t *testing.T) {
	now := time.Now()
	tokens := map[string]*interfaces.Token{
		"tok-1": listWorkToken("tok-1", "work-init", "task:init", "task", now),
		"tok-2": listWorkToken("tok-2", "work-review", "task:review", "task", now),
		"tok-3": listWorkToken("tok-3", "work-failed", "task:failed", "task", now),
	}
	srv := newTestServer(&testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: tokens}, Net: listWorkFilterTopology()})
	for _, tc := range []struct {
		name        string
		query       string
		wantWorkIDs []string
	}{
		{name: "state name", query: "state.name=review", wantWorkIDs: []string{"work-review"}},
		{name: "state type", query: "state.type=PROCESSING", wantWorkIDs: []string{"work-review"}},
		{name: "combined", query: "state.name=review&state.type=PROCESSING", wantWorkIDs: []string{"work-review"}},
		{name: "combined mismatch", query: "state.name=review&state.type=FAILED", wantWorkIDs: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := decodeListWorkPage(t, srv, "/work?"+tc.query)
			if len(resp.Results) != len(tc.wantWorkIDs) {
				t.Fatalf("results = %d, want %d: %#v", len(resp.Results), len(tc.wantWorkIDs), resp.Results)
			}
			for i, wantWorkID := range tc.wantWorkIDs {
				if got := stringValue(resp.Results[i].WorkId); got != wantWorkID || resp.Results[i].State == nil {
					t.Fatalf("result[%d] = %#v, want work %q with state", i, resp.Results[i], wantWorkID)
				}
			}
		})
	}
}

func TestListWork_DefaultOrderingSurfacesActiveWorkBeforeTerminalWork(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: map[string]*interfaces.Token{
		"tok-1": listWorkToken("tok-1", "work-complete", "task:complete", "task", time.Now()),
		"tok-2": listWorkToken("tok-2", "work-failed", "task:failed", "task", time.Now()),
		"tok-3": listWorkToken("tok-3", "work-review", "task:review", "task", time.Now()),
		"tok-4": listWorkToken("tok-4", "work-init", "task:init", "task", time.Now()),
	}}, Net: listWorkFilterTopology()})
	resp := decodeListWorkPage(t, srv, "/work")
	assertListedWorkIDs(t, resp.Results, []string{"work-init", "work-review", "work-failed", "work-complete"})
}

func TestListWork_SortsByStateType(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: map[string]*interfaces.Token{
		"tok-1": listWorkToken("tok-1", "work-complete", "task:complete", "task", time.Now()),
		"tok-2": listWorkToken("tok-2", "work-failed", "task:failed", "task", time.Now()),
		"tok-3": listWorkToken("tok-3", "work-review", "task:review", "task", time.Now()),
		"tok-4": listWorkToken("tok-4", "work-init", "task:init", "task", time.Now()),
	}}, Net: listWorkFilterTopology()})
	resp := decodeListWorkPage(t, srv, "/work?sortBy=state.type")
	assertListedWorkIDs(t, resp.Results, []string{"work-failed", "work-init", "work-review", "work-complete"})
}

func TestListWork_InvalidStateTypeReturnsBadRequest(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{})
	req := httptest.NewRequest(http.MethodGet, "/work?state.type=UNKNOWN", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "state.type must be one of INITIAL, PROCESSING, TERMINAL, or FAILED")
}

func TestListWork_InvalidSortByReturnsBadRequest(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{})
	req := httptest.NewRequest(http.MethodGet, "/work?sortBy=name", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "sortBy must be state.type")
}

func TestListWork_InvalidMaxResultsUsesGeneratedBadRequest(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{})
	for _, tc := range []struct{ name, path string }{{"empty", "/work?maxResults="}, {"invalid", "/work?maxResults=abc"}} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)
			assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "invalid request parameter")
		})
	}
}

func TestListWork_NonPositiveMaxResultsDefaultsToCurrentBehavior(t *testing.T) {
	tokens := makeListWorkTokens("legacy", 3, time.Now())
	srv := newTestServer(&testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: tokens}})
	for _, tc := range []struct{ name, path string }{{"absent", "/work"}, {"non_positive", "/work?maxResults=0"}} {
		t.Run(tc.name, func(t *testing.T) {
			resp := decodeListWorkPage(t, srv, tc.path)
			if len(resp.Results) != len(tokens) {
				t.Fatalf("expected defaulted response with %d results, got %d", len(tokens), len(resp.Results))
			}
			if resp.PaginationContext == nil || resp.PaginationContext.MaxResults != defaultMaxResults || stringValue(resp.PaginationContext.NextToken) != "" {
				t.Fatalf("expected pagination context with maxResults %d and no next token, got %#v", defaultMaxResults, resp.PaginationContext)
			}
		})
	}
}

func TestListWork_NextTokenContinuesPublicRoutePagination(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: makeListWorkTokens("cursor", 3, time.Now())}})
	firstResp := decodeListWorkPage(t, srv, "/work?maxResults=2")
	if len(firstResp.Results) != 2 || firstResp.PaginationContext == nil {
		t.Fatalf("first page = %#v, want paginated response", firstResp)
	}
	nextToken := stringValue(firstResp.PaginationContext.NextToken)
	if nextToken == "" {
		t.Fatal("expected first page nextToken")
	}
	secondResp := decodeListWorkPage(t, srv, "/work?maxResults=2&nextToken="+nextToken)
	if len(secondResp.Results) != 1 || secondResp.PaginationContext == nil || secondResp.PaginationContext.MaxResults != 2 || stringValue(secondResp.PaginationContext.NextToken) != "" {
		t.Fatalf("second page = %#v, want one result and terminal pagination context", secondResp)
	}
	if stringValue(firstResp.Results[0].WorkId) != "work-cursor-1" || stringValue(firstResp.Results[1].WorkId) != "work-cursor-2" || stringValue(secondResp.Results[0].WorkId) != "work-cursor-3" {
		t.Fatalf("unexpected continued page results: first=%#v second=%#v", firstResp.Results, secondResp.Results)
	}
	trailingResp := decodeListWorkPage(t, srv, "/work?maxResults=2&nextToken="+encodeNextToken("tok-cursor-3"))
	if len(trailingResp.Results) != 0 || trailingResp.PaginationContext == nil || trailingResp.PaginationContext.MaxResults != 2 || stringValue(trailingResp.PaginationContext.NextToken) != "" {
		t.Fatalf("trailing page = %#v, want empty final page", trailingResp)
	}
}

func decodeListWorkPage(t *testing.T, srv *Server, path string) factoryapi.ListWorkResponse {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s status = %d, want 200: %s", path, rec.Code, rec.Body.String())
	}
	return decodeJSONResponse[factoryapi.ListWorkResponse](t, rec)
}

func assertListedWorkIDs(t *testing.T, works []factoryapi.Work, want []string) {
	t.Helper()
	if len(works) != len(want) {
		t.Fatalf("results = %d, want %d: %#v", len(works), len(want), works)
	}
	for i, wantWorkID := range want {
		if got := stringValue(works[i].WorkId); got != wantWorkID {
			t.Fatalf("result[%d].workId = %q, want %q: %#v", i, got, wantWorkID, works)
		}
	}
}
