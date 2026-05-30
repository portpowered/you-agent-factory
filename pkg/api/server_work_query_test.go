package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/factory"
	factoryrequests "github.com/portpowered/infinite-you/pkg/factory/requests"
	factoryruntime "github.com/portpowered/infinite-you/pkg/factory/runtime"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"go.uber.org/zap"
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

func TestSubmitWork_OmitsUnsetOptionalBoundaryFields(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := submitWorkRequest(t, srv, `{"name":"Inventory story","workTypeName":"task","payload":{"title":"Document current API"}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /work status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	if len(mf.Submitted) != 1 {
		t.Fatalf("submitted requests = %d, want 1", len(mf.Submitted))
	}

	submitted := mf.Submitted[0]
	if submitted.Name != "Inventory story" || submitted.WorkTypeID != "task" {
		t.Fatalf("submitted request = %#v, want canonical required fields", submitted)
	}
	if submitted.TraceID == "" || submitted.CurrentChainingTraceID == "" {
		t.Fatalf("submitted chaining identifiers = %#v, want server-owned defaults when optionals are omitted", submitted)
	}
	if submitted.Relations != nil {
		t.Fatalf("submitted relations = %#v, want nil when omitted", submitted.Relations)
	}
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

	resp := decodeJSONResponse[factoryapi.Work](t, rec)
	if stringValue(resp.WorkId) != "work-prd-1" || stringValue(resp.WorkTypeName) != "prd" {
		t.Fatalf("work response = %#v, want work-prd-1 prd type", resp)
	}
	if resp.ChainingTraceDepth == nil || *resp.ChainingTraceDepth != 4 || resp.CurrentChainingTraceId == nil || *resp.CurrentChainingTraceId != "chain-1" || resp.PreviousChainingTraceIds == nil || len(*resp.PreviousChainingTraceIds) != 2 {
		t.Fatalf("chaining trace fields = %#v, want preserved trace lineage", resp)
	}
	assertGeneratedWorkContentParts(t, resp.Content, []interfaces.WorkContentPart{
		{Type: interfaces.WorkContentPartTypeText, Text: "Review screenshot"},
		{Type: interfaces.WorkContentPartTypeImage, File: "fixtures/review.png"},
	})
}

func TestGetWork_ByWorkID(t *testing.T) {
	now := time.Now()
	srv := newTestServer(&testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{Tokens: map[string]*interfaces.Token{
			"tok-prd-1": {
				ID:      "tok-prd-1",
				PlaceID: "prd:init",
				Color: interfaces.TokenColor{
					WorkID:     "work-prd-1",
					WorkTypeID: "prd",
					TraceID:    "trace-1",
				},
				CreatedAt: now,
				EnteredAt: now,
			},
		}},
	})

	req := httptest.NewRequest("GET", "/work/work-prd-1", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSONResponse[factoryapi.Work](t, rec)
	if stringValue(resp.WorkId) != "work-prd-1" {
		t.Fatalf("work response = %#v, want work-prd-1", resp)
	}
}

func TestGetWork_OmitsEmptyOptionalCollections(t *testing.T) {
	now := time.Now()
	srv := newTestServer(&testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{Tokens: map[string]*interfaces.Token{
			"tok-prd-2": {
				ID:      "tok-prd-2",
				PlaceID: "prd:init",
				Color: interfaces.TokenColor{
					WorkID:     "work-prd-2",
					WorkTypeID: "prd",
					TraceID:    "trace-2",
				},
				CreatedAt: now,
				EnteredAt: now,
			},
		}},
	})

	req := httptest.NewRequest("GET", "/work/tok-prd-2", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSONResponse[factoryapi.Work](t, rec)
	if resp.CurrentChainingTraceId == nil || *resp.CurrentChainingTraceId != "trace-2" {
		t.Fatalf("current chaining trace ID = %#v, want trace fallback", resp.CurrentChainingTraceId)
	}
	if resp.PreviousChainingTraceIds != nil || resp.Tags != nil {
		t.Fatalf("optional collections = %#v, want omitted empty fields", resp)
	}
}

func TestTokenToResponse_CopiesOptionalTagMap(t *testing.T) {
	token := &interfaces.Token{
		ID:      "tok-prd-copy",
		PlaceID: "prd:init",
		Color: interfaces.TokenColor{
			WorkID:     "work-prd-copy",
			WorkTypeID: "prd",
			TraceID:    "trace-copy",
			Tags: map[string]string{
				"branch": "stable",
			},
		},
	}

	resp := tokenToResponse(token, false)
	token.Color.Tags["branch"] = "mutated"
	token.Color.Tags["new"] = "late-tag"

	if resp.Tags == nil || (*resp.Tags)["branch"] != "stable" {
		t.Fatalf("response tags = %#v, want copied pre-mutation values", resp.Tags)
	}
	if _, ok := (*resp.Tags)["new"]; ok {
		t.Fatalf("response tags = %#v, want copied map to omit post-shaping additions", resp.Tags)
	}
}

func TestTokenToResponse_CopiesOptionalPreviousChainingTraceIDs(t *testing.T) {
	token := &interfaces.Token{
		ID:      "tok-prd-copy-slice",
		PlaceID: "prd:init",
		Color: interfaces.TokenColor{
			WorkID:                   "work-prd-copy-slice",
			WorkTypeID:               "prd",
			TraceID:                  "trace-copy-slice",
			CurrentChainingTraceID:   "chain-current",
			PreviousChainingTraceIDs: []string{"chain-parent"},
		},
	}

	resp := tokenToResponse(token, false)
	token.Color.PreviousChainingTraceIDs[0] = "chain-mutated"
	token.Color.PreviousChainingTraceIDs = append(token.Color.PreviousChainingTraceIDs, "chain-late")

	if resp.PreviousChainingTraceIds == nil || len(*resp.PreviousChainingTraceIds) != 1 || (*resp.PreviousChainingTraceIds)[0] != "chain-parent" {
		t.Fatalf("response previous chaining trace IDs = %#v, want copied pre-mutation values", resp.PreviousChainingTraceIds)
	}
}

func TestGetWorkNotFound(t *testing.T) {
	srv := newTestServer(&testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}})
	req := httptest.NewRequest("GET", "/work/nonexistent", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "work not found")
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
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "work not found")
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
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "work not found")
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

func TestListWork_FiltersByWorkTypeNameNameSubstringAndTraceId(t *testing.T) {
	now := time.Now()
	tokens := map[string]*interfaces.Token{
		"tok-1": listWorkTokenWithTraces("tok-1", "work-story", "Review PRD", "task:review", "story", "trace-root", "", now),
		"tok-2": listWorkTokenWithTraces("tok-2", "work-bug", "Fix bug", "task:init", "bug", "", "trace-chain-1", now),
		"tok-3": listWorkTokenWithTraces("tok-3", "work-plan", "Plan feature", "task:init", "story", "trace-plan", "", now),
	}
	srv := newTestServer(&testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: tokens}, Net: listWorkFilterTopology()})
	for _, tc := range []struct {
		name        string
		query       string
		wantWorkIDs []string
	}{
		{name: "work type name", query: "workTypeName=story", wantWorkIDs: []string{"work-plan", "work-story"}},
		{name: "name substring", query: "name=prd", wantWorkIDs: []string{"work-story"}},
		{name: "trace id on current chaining trace", query: "traceId=trace-chain-1", wantWorkIDs: []string{"work-bug"}},
		{name: "trace id on trace id", query: "traceId=trace-plan", wantWorkIDs: []string{"work-plan"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := decodeListWorkPage(t, srv, "/work?"+tc.query)
			if len(resp.Results) != len(tc.wantWorkIDs) {
				t.Fatalf("results = %d, want %d: %#v", len(resp.Results), len(tc.wantWorkIDs), resp.Results)
			}
			gotIDs := make([]string, len(resp.Results))
			for i, work := range resp.Results {
				gotIDs[i] = stringValue(work.WorkId)
			}
			for i, wantWorkID := range tc.wantWorkIDs {
				if gotIDs[i] != wantWorkID {
					t.Fatalf("result[%d] workId = %q, want %q (all=%v)", i, gotIDs[i], wantWorkID, gotIDs)
				}
			}
		})
	}
}

func TestListWork_FiltersByNameBeforePagination(t *testing.T) {
	now := time.Now()
	tokens := map[string]*interfaces.Token{
		"tok-1": listWorkTokenWithTraces("tok-1", "work-alpha", "Alpha one", "task:init", "task", "", "", now),
		"tok-2": listWorkTokenWithTraces("tok-2", "work-beta", "Other item", "task:init", "task", "", "", now),
		"tok-3": listWorkTokenWithTraces("tok-3", "work-gamma", "Alpha two", "task:init", "task", "", "", now),
	}
	srv := newTestServer(&testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: tokens}, Net: listWorkFilterTopology()})
	resp := decodeListWorkPage(t, srv, "/work?name=alpha&maxResults=2")
	assertListedWorkIDs(t, resp.Results, []string{"work-alpha", "work-gamma"})
	if resp.PaginationContext == nil || stringValue(resp.PaginationContext.NextToken) != "" {
		t.Fatalf("pagination = %#v, want terminal page after name filter", resp.PaginationContext)
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

func TestUpsertWorkRequest_NormalizesLegacyStringPayloadIntoCanonicalContent(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := upsertWorkRequest(t, srv, "/work-requests/request-1", `{"requestId":"request-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"prd","payload":"legacy text"}]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(mf.Submitted) != 1 || len(mf.Submitted[0].Content) != 1 || mf.Submitted[0].Content[0].Text != "legacy text" {
		t.Fatalf("submitted content = %#v, want canonical text content", mf.Submitted)
	}
}

func TestUpsertWorkRequest_RejectsInvalidContentPartShape(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)
	rec := upsertWorkRequest(t, srv, "/work-requests/request-1", `{"requestId":"request-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"prd","content":[{"type":"text","file":"wrong"}]}]}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "works[0].content[0].file is not supported")
	if len(mf.Submitted) != 0 || len(mf.WorkRequests) != 0 {
		t.Fatalf("submissions = workRequests:%d submitted:%d, want 0/0", len(mf.WorkRequests), len(mf.Submitted))
	}
}

func TestUpsertWorkRequest_AcceptsCanonicalContent(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := upsertWorkRequest(t, srv, "/work-requests/request-canonical", `{
		"requestId":"request-canonical",
		"type":"FACTORY_REQUEST_BATCH",
		"works":[{"name":"draft","workTypeName":"prd","content":[{"type":"text","text":"Review this UI."},{"type":"image","file":"fixtures/ui.png"}]}]
	}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(mf.Submitted) != 1 {
		t.Fatalf("submitted count = %d, want 1", len(mf.Submitted))
	}
	if len(mf.Submitted[0].Content) != 2 {
		t.Fatalf("content count = %d, want 2", len(mf.Submitted[0].Content))
	}
	if mf.Submitted[0].Content[0].Type != interfaces.WorkContentPartTypeText || mf.Submitted[0].Content[0].Text != "Review this UI." {
		t.Fatalf("submitted content[0] = %#v, want canonical text content", mf.Submitted[0].Content[0])
	}
	if mf.Submitted[0].Content[1].Type != interfaces.WorkContentPartTypeImage || mf.Submitted[0].Content[1].File != "fixtures/ui.png" {
		t.Fatalf("submitted content[1] = %#v, want canonical image content", mf.Submitted[0].Content[1])
	}
}

func TestUpsertWorkRequest_AcceptsUppercaseAndExtendedContent(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := upsertWorkRequest(t, srv, "/work-requests/request-model-content", `{
		"requestId":"request-model-content",
		"type":"FACTORY_REQUEST_BATCH",
		"works":[{
			"name":"draft",
			"workTypeName":"prd",
			"content":[
				{"type":"IMAGE","file":"fixtures/ui.png","label":"reference"},
				{"type":"BINARY","file":"artifacts/raw.bin","contentType":"application/octet-stream"},
				{"type":"JSON","json":{"mode":"preview"}}
			]
		}]
	}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(mf.Submitted) != 1 || len(mf.Submitted[0].Content) != 3 {
		t.Fatalf("submitted content = %#v, want 3 canonical parts", mf.Submitted)
	}
	if mf.Submitted[0].Content[0].Type != interfaces.WorkContentPartTypeImage || mf.Submitted[0].Content[0].Label != "reference" {
		t.Fatalf("submitted content[0] = %#v, want normalized image part", mf.Submitted[0].Content[0])
	}
	if mf.Submitted[0].Content[1].Type != interfaces.WorkContentPartTypeBinary || mf.Submitted[0].Content[1].ContentType != "application/octet-stream" {
		t.Fatalf("submitted content[1] = %#v, want canonical binary part", mf.Submitted[0].Content[1])
	}
	if mf.Submitted[0].Content[2].Type != interfaces.WorkContentPartTypeJSON {
		t.Fatalf("submitted content[2] = %#v, want canonical json part", mf.Submitted[0].Content[2])
	}
	jsonValue := map[string]any{}
	if err := json.Unmarshal(mf.Submitted[0].Content[2].JSON, &jsonValue); err != nil {
		t.Fatalf("decode json content: %v", err)
	}
	if jsonValue["mode"] != "preview" {
		t.Fatalf("submitted content[2].json = %s, want preview json", mf.Submitted[0].Content[2].JSON)
	}
}

func TestUpsertWorkRequest_FirstSubmitAndRepeatedRequestID(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	var firstTraceID string
	for i, body := range []string{
		`{"requestId":"request-api-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"task","traceId":"trace-original","payload":{"title":"Draft"}}]}`,
		`{"requestId":"request-api-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"changed-draft","workTypeName":"task","traceId":"trace-retry","payload":{"title":"Changed retry"}}]}`,
	} {
		rec := upsertWorkRequest(t, srv, "/work-requests/request-api-1", body)
		if rec.Code != http.StatusCreated {
			t.Fatalf("PUT /work-requests status = %d, want 201: %s", rec.Code, rec.Body.String())
		}
		resp := decodeJSONResponse[factoryapi.UpsertWorkRequestResponse](t, rec)
		if resp.RequestId != "request-api-1" || resp.TraceId == "" {
			t.Fatalf("upsert response = %#v, want request and trace", resp)
		}
		if i == 0 {
			firstTraceID = resp.TraceId
		} else if resp.TraceId != firstTraceID {
			t.Fatalf("repeated trace_id = %q, want original %q", resp.TraceId, firstTraceID)
		}
	}

	if len(mf.WorkRequests) != 1 || len(mf.Submitted) != 1 {
		t.Fatalf("submissions = workRequests:%d submitted:%d, want 1/1", len(mf.WorkRequests), len(mf.Submitted))
	}
	if mf.Submitted[0].RequestID != "request-api-1" || mf.Submitted[0].TraceID != "trace-original" || mf.Submitted[0].Name != "draft" {
		t.Fatalf("submitted request = %#v, want original request metadata", mf.Submitted[0])
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this upsert boundary test keeps the full relation and runtime mapping contract inline for reviewer-readable coverage.
func TestUpsertWorkRequest_MapsWorkTypeNameAndRelationsToRuntime(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := upsertWorkRequest(t, srv, "/work-requests/request-api-batch", `{
		"requestId":"request-api-batch",
		"currentChainingTraceId":"chain-request-batch",
		"type":"FACTORY_REQUEST_BATCH",
		"works":[
			{"name":"draft","workTypeName":"task","state":"queued","currentChainingTraceId":"chain-draft","traceId":"chain-draft","payload":{"title":"Draft"}},
			{"name":"review","workTypeName":"review","payload":"review draft"}
		],
		"relations":[{"type":"DEPENDS_ON","sourceWorkName":"review","targetWorkName":"draft","requiredState":"complete"}]
	}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("PUT /work-requests status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	submittedRequest := mf.WorkRequests[0]
	if len(mf.WorkRequests) != 1 || len(submittedRequest.Works) != 2 {
		t.Fatalf("work request submissions = %#v, want one request with two works", mf.WorkRequests)
	}
	if submittedRequest.CurrentChainingTraceID != "chain-request-batch" || submittedRequest.Works[0].CurrentChainingTraceID != "chain-draft" || submittedRequest.Works[1].CurrentChainingTraceID != "chain-request-batch" {
		t.Fatalf("work request chaining traces = %#v", submittedRequest)
	}
	if submittedRequest.Works[0].WorkTypeID != "task" || submittedRequest.Works[1].WorkTypeID != "review" || submittedRequest.Works[0].State != "queued" {
		t.Fatalf("domain works = %#v, want task/review and queued draft", submittedRequest.Works)
	}
	if len(submittedRequest.Relations) != 1 || submittedRequest.Relations[0].SourceWorkName != "review" || submittedRequest.Relations[0].TargetWorkName != "draft" {
		t.Fatalf("domain relation = %#v, want review depends on draft", submittedRequest.Relations)
	}
	if len(mf.Submitted) != 2 {
		t.Fatalf("normalized submissions = %d, want 2", len(mf.Submitted))
	}
	relation := mf.Submitted[1].Relations[0]
	if relation.TargetWorkID != "batch-request-api-batch-draft" || relation.RequiredState != "complete" {
		t.Fatalf("normalized relation = %#v, want dependency on draft completion", relation)
	}
}

func TestUpsertWorkRequest_ReturnsPerWorkIdentifiers(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := upsertWorkRequest(t, srv, "/work-requests/request-api-batch", `{
		"requestId":"request-api-batch",
		"type":"FACTORY_REQUEST_BATCH",
		"works":[
			{"name":"draft","workTypeName":"task","payload":{"title":"Draft"}},
			{"name":"review","workTypeName":"review","payload":"review draft"}
		]
	}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("PUT /work-requests status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSONResponse[factoryapi.UpsertWorkRequestResponse](t, rec)
	if resp.RequestId != "request-api-batch" || resp.TraceId == "" {
		t.Fatalf("upsert response = %#v, want request and trace", resp)
	}
	if len(resp.Works) != 2 {
		t.Fatalf("upsert works = %#v, want 2 items", resp.Works)
	}
	want := []factoryapi.UpsertWorkRequestSubmittedWork{
		{Name: "draft", WorkTypeName: "task", WorkId: "batch-request-api-batch-draft"},
		{Name: "review", WorkTypeName: "review", WorkId: "batch-request-api-batch-review"},
	}
	for i, work := range resp.Works {
		if work != want[i] {
			t.Fatalf("upsert works[%d] = %#v, want %#v", i, work, want[i])
		}
	}
}

func TestMoveWork_SucceedsAndReturnsUpdatedWork(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	mf := moveWorkMockFactory(now, "work-move-1", "task", "init")
	srv := newTestServer(mf)

	rec := postMoveWork(t, srv, "work-move-1", `{"stateName":"complete"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /work/work-move-1/move status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	work := decodeJSONResponse[factoryapi.Work](t, rec)
	if stringValue(work.WorkId) != "work-move-1" || work.State == nil || work.State.Name != "complete" {
		t.Fatalf("work = %#v, want work-move-1 at complete", work)
	}
}

func TestMoveWork_AcceptsWhileFactoryPaused(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	mf := moveWorkMockFactory(now, "work-move-paused", "task", "init")
	mf.State = interfaces.FactoryStatePaused
	srv := newTestServer(mf)

	rec := postMoveWork(t, srv, "work-move-paused", `{"stateName":"complete"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /work/work-move-paused/move status = %d, want 200 while paused: %s", rec.Code, rec.Body.String())
	}
}

func TestMoveWork_Returns404ForMissingWork(t *testing.T) {
	mf := &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)},
		Net:     moveWorkTestNet(),
	}
	srv := newTestServer(mf)

	rec := postMoveWork(t, srv, "missing-work", `{"stateName":"complete"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

func TestMoveWork_Returns400ForInvalidState(t *testing.T) {
	now := time.Now().UTC()
	mf := moveWorkMockFactory(now, "work-move-invalid", "task", "init")
	srv := newTestServer(mf)

	rec := postMoveWork(t, srv, "work-move-invalid", `{"stateName":"nowhere"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
}

func TestMoveWork_Returns409ForDuplicateRequestId(t *testing.T) {
	now := time.Now().UTC()
	mf := moveWorkMockFactory(now, "work-move-dup", "task", "init")
	srv := newTestServer(mf)

	body := `{"stateName":"complete","requestId":"move-req-1"}`
	rec := postMoveWork(t, srv, "work-move-dup", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("first move status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	rec = postMoveWork(t, srv, "work-move-dup", body)
	if rec.Code != http.StatusConflict {
		t.Fatalf("second move status = %d, want 409: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSONResponse[factoryapi.ErrorResponse](t, rec)
	if resp.Code != factoryapi.MOVEWORKREQUESTALREADYAPPLIED {
		t.Fatalf("error code = %q, want %q", resp.Code, factoryapi.MOVEWORKREQUESTALREADYAPPLIED)
	}
}

func TestMoveWorkBySessionId_Returns404ForMissingSession(t *testing.T) {
	mf := &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)},
		Net:     moveWorkTestNet(),
		SessionFactories: map[string]*testutil.MockFactory{
			"~default": {
				Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)},
				Net:     moveWorkTestNet(),
			},
		},
	}
	srv := newTestServer(mf)

	req := httptest.NewRequest(http.MethodPost, "/factory-sessions/missing-session/work/work-1/move", bytes.NewBufferString(`{"stateName":"complete"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

func TestMoveWorkBySessionId_SucceedsForScopedSession(t *testing.T) {
	now := time.Now().UTC()
	beta := moveWorkMockFactory(now, "work-beta-move", "task", "init")
	mf := &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)},
		Net:     moveWorkTestNet(),
		SessionFactories: map[string]*testutil.MockFactory{
			"~default": {
				Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)},
				Net:     moveWorkTestNet(),
			},
			"beta": beta,
		},
	}
	srv := newTestServer(mf)

	req := httptest.NewRequest(http.MethodPost, "/factory-sessions/beta/work/work-beta-move/move", bytes.NewBufferString(`{"stateName":"complete"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	work := decodeJSONResponse[factoryapi.Work](t, rec)
	if work.State == nil || work.State.Name != "complete" {
		t.Fatalf("work = %#v, want complete state", work)
	}
}

func TestMoveWork_IntegrationWithRuntimeFactoryWhilePaused(t *testing.T) {
	f, err := factoryruntime.New(
		factory.WithNet(moveWorkTestNet()),
		factory.WithInlineDispatch(),
		factory.WithLogger(logging.NoopLogger{}),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	if _, err := f.SubmitWorkRequest(ctx, factoryrequests.WorkRequestFromSubmitRequests([]interfaces.SubmitRequest{{
		WorkID:     "work-runtime-paused",
		WorkTypeID: "task",
		TraceID:    "trace-runtime-paused",
	}})); err != nil {
		t.Fatalf("SubmitWorkRequest: %v", err)
	}
	tickable, ok := f.(factoryruntime.TickableFactory)
	if !ok {
		t.Fatal("expected tickable factory")
	}
	if err := tickable.Tick(ctx); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	if err := f.Pause(ctx); err != nil {
		t.Fatalf("Pause: %v", err)
	}

	logger, _ := zap.NewDevelopment()
	srv := NewServer(newRuntimeMoveAPISurface(f), 8080, logger)
	rec := postMoveWork(t, srv, "work-runtime-paused", `{"stateName":"complete"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
}

type runtimeMoveAPISurface struct {
	*testutil.MockFactory
	runtime factory.Factory
}

func newRuntimeMoveAPISurface(f factory.Factory) apisurface.APISurface {
	return &runtimeMoveAPISurface{
		MockFactory: &testutil.MockFactory{},
		runtime:     f,
	}
}

func (r *runtimeMoveAPISurface) MoveWork(ctx context.Context, workID, stateName string, source interfaces.WorkStateChangeSource, requestID string) (interfaces.OperatorMoveResult, error) {
	return r.runtime.MoveWork(ctx, workID, stateName, source, requestID)
}

func (r *runtimeMoveAPISurface) GetEngineStateSnapshot(ctx context.Context) (*interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], error) {
	return r.runtime.GetEngineStateSnapshot(ctx)
}

func postMoveWork(t *testing.T, srv *Server, workID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/work/"+workID+"/move", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func moveWorkMockFactory(now time.Time, workID, workTypeID, stateName string) *testutil.MockFactory {
	net := moveWorkTestNet()
	placeID := state.PlaceID(workTypeID, stateName)
	return &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{
			Tokens: map[string]*interfaces.Token{
				"tok-1": {
					ID:      "tok-1",
					PlaceID: placeID,
					Color: interfaces.TokenColor{
						WorkID:     workID,
						WorkTypeID: workTypeID,
					},
					CreatedAt: now,
					EnteredAt: now,
				},
			},
		},
		Net: net,
	}
}

func moveWorkTestNet() *state.Net {
	wt := &state.WorkType{
		ID:   "task",
		Name: "Task",
		States: []state.StateDefinition{
			{Value: "init", Category: state.StateCategoryInitial},
			{Value: "complete", Category: state.StateCategoryTerminal},
			{Value: "failed", Category: state.StateCategoryFailed},
		},
	}
	places := make(map[string]*petri.Place)
	for _, place := range wt.GeneratePlaces() {
		places[place.ID] = place
	}
	return &state.Net{
		Places:    places,
		WorkTypes: map[string]*state.WorkType{"task": wt},
	}
}

func TestUpsertWorkRequest_AcceptsParentChildRelationsByWorkName(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := upsertWorkRequest(t, srv, "/work-requests/request-api-parent-child", `{
		"requestId":"request-api-parent-child",
		"type":"FACTORY_REQUEST_BATCH",
		"works":[
			{"name":"parent","workTypeName":"task","traceId":"trace-parent-child","payload":{"title":"Parent"}},
			{"name":"prerequisite","workTypeName":"task","payload":{"title":"Prerequisite"}},
			{"name":"child","workTypeName":"task","payload":{"title":"Child"}}
		],
		"relations":[
			{"type":"PARENT_CHILD","sourceWorkName":"child","targetWorkName":"parent"},
			{"type":"DEPENDS_ON","sourceWorkName":"child","targetWorkName":"prerequisite"}
		]
	}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("PUT /work-requests status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	if len(mf.WorkRequests) != 1 || len(mf.WorkRequests[0].Relations) != 2 || mf.WorkRequests[0].Relations[0].Type != interfaces.WorkRelationParentChild {
		t.Fatalf("work request relations = %#v, want parent-child plus dependency", mf.WorkRequests)
	}
	child := submittedRequestNamed(t, mf.Submitted, "child")
	if child.TraceID != "trace-parent-child" || len(child.Relations) != 2 {
		t.Fatalf("normalized child = %#v, want inherited trace and relations", child)
	}
	assertSubmittedChildRelations(t, child.Relations)
}

func TestUpsertWorkRequest_CopiesWorkTagMapBeforeRuntimeSubmission(t *testing.T) {
	workTags := factoryapi.StringMap{"priority": "high"}
	req := factoryapi.WorkRequest{
		RequestId: "request-tag-copy",
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works: &[]factoryapi.Work{
			{
				Name:         "draft",
				WorkTypeName: stringPointerForAPITest("task"),
				Payload:      map[string]any{"title": "Draft"},
				Tags:         &workTags,
			},
		},
	}
	domain, err := generatedWorkRequestToDomain(req)
	if err != nil {
		t.Fatalf("generatedWorkRequestToDomain error = %v", err)
	}
	if len(domain.Works) != 1 {
		t.Fatalf("domain works = %#v, want one work", domain.Works)
	}

	workTags["priority"] = "mutated"
	workTags["post"] = "added"

	if domain.Works[0].Tags["priority"] != "high" {
		t.Fatalf("domain work tags = %#v, want pre-mutation values", domain.Works[0].Tags)
	}
	if _, ok := domain.Works[0].Tags["post"]; ok {
		t.Fatalf("domain work tags = %#v, want copied map to omit post-decode additions", domain.Works[0].Tags)
	}
}

func TestUpsertWorkRequest_WorkTypeIDReturnsBadRequest(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := upsertWorkRequest(t, srv, "/work-requests/request-api-legacy", `{"requestId":"request-api-legacy","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","work_type_id":"legacy-task","payload":{"title":"Draft"}}]}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "works[0].work_type_id is not supported; use workTypeName")
}

func TestUpsertWorkRequest_TargetStateReturnsBadRequest(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := upsertWorkRequest(t, srv, "/work-requests/request-api-state-alias", `{"requestId":"request-api-state-alias","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"task","target_state":"queued","payload":{"title":"Draft"}}]}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "works[0].target_state is not supported; use state")
}

func TestUpsertWorkRequest_ConflictingCurrentChainingTraceIDReturnsBadRequest(t *testing.T) {
	mf := &testutil.MockFactory{Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)}}
	srv := newTestServer(mf)

	rec := upsertWorkRequest(t, srv, "/work-requests/request-api-chaining-conflict", `{"requestId":"request-api-chaining-conflict","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"task","currentChainingTraceId":"chain-a","traceId":"trace-b","payload":{"title":"Draft"}}]}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "works[0].currentChainingTraceId and traceId must match when both are provided")
}

func TestUpsertWorkRequest_InvalidExplicitStateReturnsBadRequest(t *testing.T) {
	mf := &testutil.MockFactory{
		Marking: &petri.MarkingSnapshot{Tokens: make(map[string]*interfaces.Token)},
		Net: &state.Net{WorkTypes: map[string]*state.WorkType{
			"task": {ID: "task", States: []state.StateDefinition{{Value: "init", Category: state.StateCategoryInitial}, {Value: "complete", Category: state.StateCategoryTerminal}}},
		}},
	}
	srv := newTestServer(mf)

	rec := upsertWorkRequest(t, srv, "/work-requests/request-api-invalid-state", `{"requestId":"request-api-invalid-state","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"task","state":"queued","payload":{"title":"Draft"}}]}`)
	assertJSONError(t, rec, http.StatusBadRequest, "BAD_REQUEST", `work_request: works[0] ("draft") references unknown state "queued" for work type name "task"`)
}

func TestUpsertWorkRequestValidationFailures(t *testing.T) {
	runUpsertValidationFailureCases(t, []upsertValidationFailureCase{
		{name: "invalid_json", path: "/work-requests/request-api-1", body: `{"requestId":`, wantMsg: "invalid request payload"},
		{name: "missing_required_request_id", path: "/work-requests/request-api-1", body: `{"type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"task"}]}`, wantMsg: "requestId is required"},
		{name: "path_body_mismatch", path: "/work-requests/request-api-1", body: `{"requestId":"request-api-2","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"task"}]}`, wantMsg: "request_id path and requestId body must match"},
		{name: "cycle_error", path: "/work-requests/request-api-1", body: `{"requestId":"request-api-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"a","workTypeName":"task"},{"name":"b","workTypeName":"task"}],"relations":[{"type":"DEPENDS_ON","sourceWorkName":"a","targetWorkName":"b"},{"type":"DEPENDS_ON","sourceWorkName":"b","targetWorkName":"a"}]}`, wantMsg: `work_request: dependency cycle detected involving "a"`},
		{name: "malformed_relation", path: "/work-requests/request-api-1", body: `{"requestId":"request-api-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"a","workTypeName":"task"}],"relations":[{"type":"DEPENDS_ON","sourceWorkName":"a","targetWorkName":"missing"}]}`, wantMsg: `work_request: relations[0] references unknown targetWorkName "missing"`},
		{name: "self_parenting_relation", path: "/work-requests/request-api-1", body: `{"requestId":"request-api-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"a","workTypeName":"task"}],"relations":[{"type":"PARENT_CHILD","sourceWorkName":"a","targetWorkName":"a"}]}`, wantMsg: `work_request: relations[0] has self-parenting on "a"`},
	})

	runUpsertValidationFailureCases(t, []upsertValidationFailureCase{
		{name: "duplicate_parent_child_relation", path: "/work-requests/request-api-1", body: `{"requestId":"request-api-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"parent","workTypeName":"task"},{"name":"child","workTypeName":"task"}],"relations":[{"type":"PARENT_CHILD","sourceWorkName":"child","targetWorkName":"parent"},{"type":"PARENT_CHILD","sourceWorkName":"child","targetWorkName":"parent"}]}`, wantMsg: `work_request: relations[1] duplicates relations[0] ("PARENT_CHILD" "child" -> "parent")`},
		{name: "missing_work_type_name", path: "/work-requests/request-api-1", body: `{"requestId":"request-api-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft"}]}`, wantMsg: `work_request: works[0] ("draft") is missing workTypeName`},
		{name: "work_type_id_not_supported", path: "/work-requests/request-api-1", body: `{"requestId":"request-api-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"task","work_type_id":"legacy-task"}]}`, wantMsg: `works[0].work_type_id is not supported; use workTypeName`},
		{name: "unknown_work_type", path: "/work-requests/request-api-1", body: `{"requestId":"request-api-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"unknown"}]}`, factory: &testutil.MockFactory{SubmitWorkRequestErr: errors.New(`work_request: works[0] ("draft") references unknown work type "unknown"`)}, wantMsg: `work_request: works[0] ("draft") references unknown work type name "unknown"`},
		{
			name: "invalid_dependency_required_state",
			path: "/work-requests/request-api-1",
			body: `{"requestId":"request-api-1","type":"FACTORY_REQUEST_BATCH","works":[{"name":"draft","workTypeName":"task"},{"name":"review","workTypeName":"task"}],"relations":[{"type":"DEPENDS_ON","sourceWorkName":"review","targetWorkName":"draft","requiredState":"queued"}]}`,
			factory: &testutil.MockFactory{
				Net: &state.Net{
					WorkTypes: map[string]*state.WorkType{
						"task": {
							ID: "task",
							States: []state.StateDefinition{
								{Value: "init", Category: state.StateCategoryInitial},
								{Value: "complete", Category: state.StateCategoryTerminal},
							},
						},
					},
				},
			},
			wantMsg: `work_request: relations[0] references unknown requiredState "queued" for target work type name "task"`,
		},
	})
}
