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

func TestGetWork_IncludesDispatchOnlyWorkWithProcessingState(t *testing.T) {
	now := time.Now()
	dispatchToken := interfaces.Token{
		ID:      "tok-in-flight",
		PlaceID: "task:review",
		Color: interfaces.TokenColor{
			DataType:   interfaces.DataTypeWork,
			WorkID:     "work-in-flight",
			WorkTypeID: "task",
			Name:       "In flight story",
		},
		CreatedAt: now,
		EnteredAt: now,
	}
	srv := newTestServer(&testutil.MockFactory{
		EngineState: &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
			Marking: petri.MarkingSnapshot{Tokens: map[string]*interfaces.Token{}},
			Dispatches: map[string]*interfaces.DispatchEntry{
				"dispatch-1": {
					DispatchID:     "dispatch-1",
					ConsumedTokens: []interfaces.Token{dispatchToken},
				},
			},
			Topology: listWorkFilterTopology(),
		},
	})

	for _, path := range []string{"/factory-sessions/~default/work/work-in-flight", "/factory-sessions/~default/work/tok-in-flight"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
			}
			work := decodeJSONResponse[factoryapi.Work](t, rec)
			if stringValue(work.WorkId) != "work-in-flight" || work.Name != "In flight story" {
				t.Fatalf("work = %#v, want dispatch-only identity fields", work)
			}
			if work.State == nil || work.State.Name != "review" || work.State.Type != factoryapi.WorkStateTypePROCESSING {
				t.Fatalf("state = %#v, want review/PROCESSING from consumed place", work.State)
			}
		})
	}
}

func TestGetWork_NotFoundWhenAbsentFromMarkingAndDispatches(t *testing.T) {
	now := time.Now()
	srv := newTestServer(&testutil.MockFactory{
		EngineState: &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
			Marking: petri.MarkingSnapshot{Tokens: map[string]*interfaces.Token{
				"tok-mark": listWorkToken("tok-mark", "work-mark", "task:init", "task", now),
			}},
			Dispatches: map[string]*interfaces.DispatchEntry{},
			Topology:   listWorkFilterTopology(),
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/factory-sessions/~default/work/work-missing", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	assertJSONError(t, rec, http.StatusNotFound, "NOT_FOUND", "work not found")
}

func TestListWork_IncludesDispatchOnlyWorkWithProcessingState(t *testing.T) {
	now := time.Now()
	dispatchToken := interfaces.Token{
		ID:      "tok-in-flight",
		PlaceID: "task:review",
		Color: interfaces.TokenColor{
			DataType:   interfaces.DataTypeWork,
			WorkID:     "work-in-flight",
			WorkTypeID: "task",
			Name:       "In flight story",
		},
		CreatedAt: now,
		EnteredAt: now,
	}
	srv := newTestServer(&testutil.MockFactory{
		EngineState: &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
			Marking: petri.MarkingSnapshot{Tokens: map[string]*interfaces.Token{}},
			Dispatches: map[string]*interfaces.DispatchEntry{
				"dispatch-1": {
					DispatchID:     "dispatch-1",
					ConsumedTokens: []interfaces.Token{dispatchToken},
				},
			},
			Topology: listWorkFilterTopology(),
		},
	})

	resp := decodeListWorkPage(t, srv, "/factory-sessions/~default/work")
	if len(resp.Results) != 1 {
		t.Fatalf("results = %d, want 1: %#v", len(resp.Results), resp.Results)
	}
	work := resp.Results[0]
	if stringValue(work.WorkId) != "work-in-flight" || work.Name != "In flight story" {
		t.Fatalf("work = %#v, want dispatch-only identity fields", work)
	}
	if work.State == nil || work.State.Name != "review" || work.State.Type != factoryapi.WorkStateTypePROCESSING {
		t.Fatalf("state = %#v, want review/PROCESSING from consumed place", work.State)
	}
}

func TestListWork_FiltersApplyToDispatchOnlyWork(t *testing.T) {
	now := time.Now()
	dispatches := map[string]*interfaces.DispatchEntry{
		"dispatch-story": {
			DispatchID: "dispatch-story",
			ConsumedTokens: []interfaces.Token{
				*listWorkTokenWithTraces("tok-story", "work-story", "Review PRD", "task:review", "story", "trace-root", "", now),
			},
		},
		"dispatch-bug": {
			DispatchID: "dispatch-bug",
			ConsumedTokens: []interfaces.Token{
				*listWorkTokenWithTraces("tok-bug", "work-bug", "Fix bug", "task:init", "bug", "", "trace-chain-1", now),
			},
		},
	}
	srv := newTestServer(&testutil.MockFactory{
		EngineState: &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
			Marking:    petri.MarkingSnapshot{Tokens: map[string]*interfaces.Token{}},
			Dispatches: dispatches,
			Topology:   listWorkFilterTopology(),
		},
	})
	for _, tc := range []struct {
		name        string
		query       string
		wantWorkIDs []string
	}{
		{name: "work type name", query: "workTypeName=story", wantWorkIDs: []string{"work-story"}},
		{name: "name substring", query: "name=prd", wantWorkIDs: []string{"work-story"}},
		{name: "trace id on current chaining trace", query: "traceId=trace-chain-1", wantWorkIDs: []string{"work-bug"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := decodeListWorkPage(t, srv, "/factory-sessions/~default/work?"+tc.query)
			if len(resp.Results) != len(tc.wantWorkIDs) {
				t.Fatalf("results = %d, want %d: %#v", len(resp.Results), len(tc.wantWorkIDs), resp.Results)
			}
			for i, wantWorkID := range tc.wantWorkIDs {
				if got := stringValue(resp.Results[i].WorkId); got != wantWorkID {
					t.Fatalf("result[%d] workId = %q, want %q", i, got, wantWorkID)
				}
			}
		})
	}
}

func TestListWork_PaginationCursorUsesDispatchTokenID(t *testing.T) {
	now := time.Now()
	markingTokens := map[string]*interfaces.Token{
		"tok-mark-1": listWorkToken("tok-mark-1", "work-mark-1", "task:init", "task", now),
		"tok-mark-2": listWorkToken("tok-mark-2", "work-mark-2", "task:init", "task", now),
	}
	dispatchToken := *listWorkToken("tok-dispatch-1", "work-dispatch-1", "task:review", "task", now)
	srv := newTestServer(&testutil.MockFactory{
		EngineState: &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
			Marking: petri.MarkingSnapshot{Tokens: markingTokens},
			Dispatches: map[string]*interfaces.DispatchEntry{
				"dispatch-1": {DispatchID: "dispatch-1", ConsumedTokens: []interfaces.Token{dispatchToken}},
			},
			Topology: listWorkFilterTopology(),
		},
	})

	firstResp := decodeListWorkPage(t, srv, "/factory-sessions/~default/work?maxResults=2")
	if len(firstResp.Results) != 2 || firstResp.PaginationContext == nil || stringValue(firstResp.PaginationContext.NextToken) == "" {
		t.Fatalf("first page = %#v, want paginated response", firstResp)
	}
	secondResp := decodeListWorkPage(t, srv, "/factory-sessions/~default/work?maxResults=2&nextToken="+stringValue(firstResp.PaginationContext.NextToken))
	if len(secondResp.Results) != 1 {
		t.Fatalf("second page = %#v, want one remaining work item", secondResp)
	}
	if got := stringValue(secondResp.Results[0].WorkId); got != "work-dispatch-1" {
		t.Fatalf("second page workId = %q, want work-dispatch-1", got)
	}
	if secondResp.Results[0].State == nil || secondResp.Results[0].State.Type != factoryapi.WorkStateTypePROCESSING {
		t.Fatalf("second page state = %#v, want PROCESSING for dispatch-only work", secondResp.Results[0].State)
	}
	trailingResp := decodeListWorkPage(t, srv, "/factory-sessions/~default/work?maxResults=2&nextToken="+encodeNextToken("tok-dispatch-1"))
	if len(trailingResp.Results) != 0 {
		t.Fatalf("trailing page = %#v, want empty page after dispatch token cursor", trailingResp)
	}
}
