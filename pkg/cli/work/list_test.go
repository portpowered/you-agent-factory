package work

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestList_SendsStateFilters(t *testing.T) {
	var gotQuery string
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		if r.URL.Query().Get("state.name") != "review" {
			t.Fatalf("state.name query = %q, want review", r.URL.Query().Get("state.name"))
		}
		if r.URL.Query().Get("state.type") != "PROCESSING" {
			t.Fatalf("state.type query = %q, want PROCESSING", r.URL.Query().Get("state.type"))
		}
		if r.URL.Query().Get("sortBy") != "state.type" {
			t.Fatalf("sortBy query = %q, want state.type", r.URL.Query().Get("sortBy"))
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListWorkResponse{
			Results: []factoryapi.Work{{
				Name:         "Review PRD",
				WorkId:       stringPtr("work-1"),
				WorkTypeName: stringPtr("story"),
				State: &factoryapi.WorkState{
					Name: "review",
					Type: factoryapi.WorkStateTypePROCESSING,
				},
			}},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := List(ListConfig{
		Port:      serverPort(t, srv),
		StateName: "review",
		StateType: "PROCESSING",
		SortBy:    "state.type",
		Output:    &out,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if gotQuery == "" {
		t.Fatal("expected request query")
	}
	if gotPath != "/factory-sessions/~default/work" {
		t.Fatalf("path = %q, want /factory-sessions/~default/work", gotPath)
	}
	if got := out.String(); got != "WORK ID\tNAME\tSTATE NAME\tSTATE TYPE\tRELATIONS\nwork-1\tReview PRD\treview\tPROCESSING\tnone\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestList_SessionScopedRouteUsesFactorySessionPath(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListWorkResponse{}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := List(ListConfig{
		Port:      serverPort(t, srv),
		SessionID: "session-beta",
		Output:    &out,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if gotPath != "/factory-sessions/session-beta/work" {
		t.Fatalf("path = %q, want /factory-sessions/session-beta/work", gotPath)
	}
	if got := out.String(); got != "No work found.\n" {
		t.Fatalf("output = %q, want empty-state output", got)
	}
}

func TestList_HumanOutputShowsEmptyState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListWorkResponse{
			Results: []factoryapi.Work{},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := List(ListConfig{
		Port:   serverPort(t, srv),
		Output: &out,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if got := out.String(); got != "No work found.\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestList_HumanOutputShowsOneWorkItemIdentityAndState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListWorkResponse{
			Results: []factoryapi.Work{{
				Name:         "Review PRD",
				WorkId:       stringPtr("work-1"),
				WorkTypeName: stringPtr("story"),
				State: &factoryapi.WorkState{
					Name: "review",
					Type: factoryapi.WorkStateTypePROCESSING,
				},
			}},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := List(ListConfig{
		Port:   serverPort(t, srv),
		Output: &out,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	want := "WORK ID\tNAME\tSTATE NAME\tSTATE TYPE\tRELATIONS\n" +
		"work-1\tReview PRD\treview\tPROCESSING\tnone\n"
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestList_HumanOutputShowsManyWorkItems(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListWorkResponse{
			Results: []factoryapi.Work{
				{
					Name:         "Plan feature",
					WorkId:       stringPtr("work-1"),
					WorkTypeName: stringPtr("story"),
					State: &factoryapi.WorkState{
						Name: "init",
						Type: factoryapi.WorkStateTypeINITIAL,
					},
				},
				{
					Name:         "Review PRD",
					WorkId:       stringPtr("work-2"),
					WorkTypeName: stringPtr("story"),
					State: &factoryapi.WorkState{
						Name: "review",
						Type: factoryapi.WorkStateTypePROCESSING,
					},
				},
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := List(ListConfig{
		Port:   serverPort(t, srv),
		Output: &out,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	want := "WORK ID\tNAME\tSTATE NAME\tSTATE TYPE\tRELATIONS\n" +
		"work-1\tPlan feature\tinit\tINITIAL\tnone\n" +
		"work-2\tReview PRD\treview\tPROCESSING\tnone\n"
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestList_HumanOutputOmitsRuntimeResourcesWhenMixedResponseContainsOnlyVisibleWork(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListWorkResponse{
			Results: []factoryapi.Work{
				{
					Name:         "Plan feature",
					WorkId:       stringPtr("work-1"),
					WorkTypeName: stringPtr("story"),
					State: &factoryapi.WorkState{
						Name: "init",
						Type: factoryapi.WorkStateTypeINITIAL,
					},
				},
				{
					Name:         "Review PRD",
					WorkId:       stringPtr("work-2"),
					WorkTypeName: stringPtr("story"),
					State: &factoryapi.WorkState{
						Name: "review",
						Type: factoryapi.WorkStateTypePROCESSING,
					},
				},
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := List(ListConfig{
		Port:   serverPort(t, srv),
		Output: &out,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	got := out.String()
	want := "WORK ID\tNAME\tSTATE NAME\tSTATE TYPE\tRELATIONS\n" +
		"work-1\tPlan feature\tinit\tINITIAL\tnone\n" +
		"work-2\tReview PRD\treview\tPROCESSING\tnone\n"
	if got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if bytes.Contains(out.Bytes(), []byte("executor-slot")) {
		t.Fatalf("output included runtime resource text: %q", got)
	}
}

func TestList_HumanOutputShowsRelationSummaryForOneRelation(t *testing.T) {
	requiredState := "complete"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListWorkResponse{
			Results: []factoryapi.Work{{
				Name:         "Review PRD",
				WorkId:       stringPtr("work-1"),
				WorkTypeName: stringPtr("story"),
				State: &factoryapi.WorkState{
					Name: "review",
					Type: factoryapi.WorkStateTypePROCESSING,
				},
				Relations: &[]factoryapi.Relation{{
					Type:           factoryapi.RelationTypeDependsOn,
					SourceWorkName: "Review PRD",
					TargetWorkName: "Draft PRD",
					TargetWorkId:   stringPtr("work-draft"),
					RequiredState:  &requiredState,
				}},
			}},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := List(ListConfig{
		Port:   serverPort(t, srv),
		Output: &out,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	want := "WORK ID\tNAME\tSTATE NAME\tSTATE TYPE\tRELATIONS\n" +
		"work-1\tReview PRD\treview\tPROCESSING\tDEPENDS_ON: Draft PRD [work-draft] (requires complete)\n"
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestList_HumanOutputShowsDeterministicSummaryForMultipleRelations(t *testing.T) {
	requiredState := "reviewed"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListWorkResponse{
			Results: []factoryapi.Work{{
				Name:         "Publish Release",
				WorkId:       stringPtr("work-3"),
				WorkTypeName: stringPtr("story"),
				State: &factoryapi.WorkState{
					Name: "blocked",
					Type: factoryapi.WorkStateTypeFAILED,
				},
				Relations: &[]factoryapi.Relation{
					{
						Type:           factoryapi.RelationTypeSpawnedBy,
						SourceWorkName: "Publish Release",
						TargetWorkName: "Release Train",
						TargetWorkId:   stringPtr("work-parent"),
					},
					{
						Type:           factoryapi.RelationTypeDependsOn,
						SourceWorkName: "Publish Release",
						TargetWorkName: "Review PRD",
						TargetWorkId:   stringPtr("work-review"),
						RequiredState:  &requiredState,
					},
					{
						Type:           factoryapi.RelationTypeParentChild,
						SourceWorkName: "Publish Release",
						TargetWorkName: "Epic Release",
						TargetWorkId:   stringPtr("work-epic"),
					},
				},
			}},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := List(ListConfig{
		Port:   serverPort(t, srv),
		Output: &out,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	want := "WORK ID\tNAME\tSTATE NAME\tSTATE TYPE\tRELATIONS\n" +
		"work-3\tPublish Release\tblocked\tFAILED\tDEPENDS_ON: Review PRD [work-review] (requires reviewed); PARENT_CHILD: Epic Release [work-epic]; SPAWNED_BY: Release Train [work-parent]\n"
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestList_SendsPaginationControlsAndEmitsJSONResponse(t *testing.T) {
	nextToken := "cursor-2"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("maxResults") != "2" {
			t.Fatalf("maxResults query = %q, want 2", r.URL.Query().Get("maxResults"))
		}
		if r.URL.Query().Get("nextToken") != "cursor-1" {
			t.Fatalf("nextToken query = %q, want cursor-1", r.URL.Query().Get("nextToken"))
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListWorkResponse{
			Results: []factoryapi.Work{{
				Name:         "Second page work",
				WorkId:       stringPtr("work-2"),
				WorkTypeName: stringPtr("story"),
				State: &factoryapi.WorkState{
					Name: "review",
					Type: factoryapi.WorkStateTypePROCESSING,
				},
			}},
			PaginationContext: &factoryapi.PaginationContext{
				MaxResults: 2,
				NextToken:  &nextToken,
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := List(ListConfig{
		Port:       serverPort(t, srv),
		MaxResults: 2,
		NextToken:  "cursor-1",
		JSON:       true,
		Output:     &out,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	var got factoryapi.ListWorkResponse
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("json output is not valid ListWorkResponse JSON: %v\n%s", err, out.String())
	}
	if len(got.Results) != 1 || stringValue(got.Results[0].WorkId) != "work-2" {
		t.Fatalf("json results = %#v, want work-2", got.Results)
	}
	if got.PaginationContext == nil || got.PaginationContext.MaxResults != 2 || stringValue(got.PaginationContext.NextToken) != nextToken {
		t.Fatalf("pagination context = %#v, want maxResults=2 nextToken=%q", got.PaginationContext, nextToken)
	}
}

func TestList_JSONOutputOmitsResourcesAndPreservesPaginationAcrossVisibleWorkPages(t *testing.T) {
	secondToken := "cursor-2"
	var requestCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		requestCount++
		switch requestCount {
		case 1:
			assertListPageRequest(t, r, "1", "")
			encodeListPageResponse(t, w, "Plan feature", "work-1", "init", factoryapi.WorkStateTypeINITIAL, &secondToken, "first")
		case 2:
			assertListPageRequest(t, r, "1", secondToken)
			encodeListPageResponse(t, w, "Review PRD", "work-2", "review", factoryapi.WorkStateTypePROCESSING, nil, "second")
		default:
			t.Fatalf("unexpected request count %d", requestCount)
		}
	}))
	defer srv.Close()

	var firstOut bytes.Buffer
	err := List(ListConfig{
		Port:       serverPort(t, srv),
		MaxResults: 1,
		JSON:       true,
		Output:     &firstOut,
	})
	if err != nil {
		t.Fatalf("List first page: %v", err)
	}
	if bytes.Contains(firstOut.Bytes(), []byte("executor-slot")) {
		t.Fatalf("first page JSON included runtime resource text: %q", firstOut.String())
	}
	assertListJSONPage(t, firstOut.Bytes(), "work-1", &secondToken, "first")

	var secondOut bytes.Buffer
	err = List(ListConfig{
		Port:       serverPort(t, srv),
		MaxResults: 1,
		NextToken:  secondToken,
		JSON:       true,
		Output:     &secondOut,
	})
	if err != nil {
		t.Fatalf("List second page: %v", err)
	}
	if bytes.Contains(secondOut.Bytes(), []byte("executor-slot")) {
		t.Fatalf("second page JSON included runtime resource text: %q", secondOut.String())
	}
	assertListJSONPage(t, secondOut.Bytes(), "work-2", nil, "second")
}

// pkgmaintcheck:ignore-cyclomatic-complexity this JSON output test keeps the generated page shape assertions together so the CLI surface stays reviewer-readable.
func TestList_JSONOutputPreservesGeneratedResponseShape(t *testing.T) {
	nextToken := "cursor-2"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListWorkResponse{
			Results: []factoryapi.Work{{
				Name:         "Review PRD",
				WorkId:       stringPtr("work-1"),
				WorkTypeName: stringPtr("story"),
				State: &factoryapi.WorkState{
					Name: "review",
					Type: factoryapi.WorkStateTypePROCESSING,
				},
			}},
			PaginationContext: &factoryapi.PaginationContext{
				MaxResults: 1,
				NextToken:  &nextToken,
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := List(ListConfig{
		Port:   serverPort(t, srv),
		JSON:   true,
		Output: &out,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if bytes.Contains(out.Bytes(), []byte("WORK ID")) || bytes.Contains(out.Bytes(), []byte("No work found.")) {
		t.Fatalf("json output included human-readable text: %q", out.String())
	}

	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("json output is invalid: %v\n%s", err, out.String())
	}
	results, ok := got["results"].([]any)
	if !ok || len(results) != 1 {
		t.Fatalf("results = %#v, want one JSON array item", got["results"])
	}
	work, ok := results[0].(map[string]any)
	if !ok {
		t.Fatalf("results[0] = %#v, want JSON object", results[0])
	}
	state, ok := work["state"].(map[string]any)
	if !ok {
		t.Fatalf("state = %#v, want JSON object", work["state"])
	}
	if work["workId"] != "work-1" || state["name"] != "review" || state["type"] != "PROCESSING" {
		t.Fatalf("work JSON = %#v, want workId and structured state fields", work)
	}
	pagination, ok := got["paginationContext"].(map[string]any)
	if !ok {
		t.Fatalf("paginationContext = %#v, want JSON object", got["paginationContext"])
	}
	if pagination["maxResults"] != float64(1) || pagination["nextToken"] != nextToken {
		t.Fatalf("paginationContext = %#v, want maxResults=1 nextToken=%q", pagination, nextToken)
	}
}

func TestList_JSONOutputSupportsAutomationSelectionWithFiltersAndPagination(t *testing.T) {
	nextToken := "cursor-review-2"
	requiredState := "approved"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state.name") != "review" {
			t.Fatalf("state.name query = %q, want review", r.URL.Query().Get("state.name"))
		}
		if r.URL.Query().Get("state.type") != "PROCESSING" {
			t.Fatalf("state.type query = %q, want PROCESSING", r.URL.Query().Get("state.type"))
		}
		if r.URL.Query().Get("maxResults") != "1" {
			t.Fatalf("maxResults query = %q, want 1", r.URL.Query().Get("maxResults"))
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListWorkResponse{
			Results: []factoryapi.Work{{
				Name:         "Review PRD",
				WorkId:       stringPtr("work-review"),
				WorkTypeName: stringPtr("story"),
				State: &factoryapi.WorkState{
					Name: "review",
					Type: factoryapi.WorkStateTypePROCESSING,
				},
				Relations: &[]factoryapi.Relation{{
					Type:           factoryapi.RelationTypeDependsOn,
					SourceWorkName: "Review PRD",
					TargetWorkName: "Approve Scope",
					TargetWorkId:   stringPtr("work-approve"),
					RequiredState:  &requiredState,
				}},
			}},
			PaginationContext: &factoryapi.PaginationContext{
				MaxResults: 1,
				NextToken:  &nextToken,
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := List(ListConfig{
		Port:       serverPort(t, srv),
		StateName:  "review",
		StateType:  "PROCESSING",
		MaxResults: 1,
		JSON:       true,
		Output:     &out,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if bytes.Contains(out.Bytes(), []byte("WORK ID")) || bytes.Contains(out.Bytes(), []byte("No work found.")) {
		t.Fatalf("json output included human-readable text: %q", out.String())
	}

	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("json output is invalid: %v\n%s", err, out.String())
	}
	selected := selectJSONWorkByState(t, got, "review", "PROCESSING")
	if selected["workId"] != "work-review" {
		t.Fatalf("selected workId = %#v, want work-review", selected["workId"])
	}
	dependency := selectJSONRelationByType(t, selected, "DEPENDS_ON")
	if dependency["targetWorkName"] != "Approve Scope" || dependency["targetWorkId"] != "work-approve" || dependency["requiredState"] != requiredState {
		t.Fatalf("dependency relation = %#v, want target work and required state preserved", dependency)
	}
	pagination := jsonObject(t, got, "paginationContext")
	if pagination["maxResults"] != float64(1) || pagination["nextToken"] != nextToken {
		t.Fatalf("paginationContext = %#v, want maxResults=1 nextToken=%q", pagination, nextToken)
	}
}

func TestList_JSONOutputLeavesRelationsOmittedWhenAPIResponseDoesNotIncludeThem(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListWorkResponse{
			Results: []factoryapi.Work{{
				Name:         "Plan Release",
				WorkId:       stringPtr("work-plan"),
				WorkTypeName: stringPtr("story"),
				State: &factoryapi.WorkState{
					Name: "planned",
					Type: factoryapi.WorkStateTypeINITIAL,
				},
			}},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := List(ListConfig{
		Port:   serverPort(t, srv),
		JSON:   true,
		Output: &out,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("json output is invalid: %v\n%s", err, out.String())
	}
	results, ok := got["results"].([]any)
	if !ok || len(results) != 1 {
		t.Fatalf("results = %#v, want one JSON array item", got["results"])
	}
	work, ok := results[0].(map[string]any)
	if !ok {
		t.Fatalf("results[0] = %#v, want JSON object", results[0])
	}
	if _, hasRelations := work["relations"]; hasRelations {
		t.Fatalf("relations = %#v, want omitted when API response omits relations", work["relations"])
	}
}

func TestList_InvalidStateType(t *testing.T) {
	err := List(ListConfig{Port: 8080, StateType: "UNKNOWN", Output: &bytes.Buffer{}})
	if err == nil {
		t.Fatal("expected invalid state type error")
	}
	if got := err.Error(); got != "--state-type must be one of INITIAL, PROCESSING, TERMINAL, or FAILED" {
		t.Fatalf("error = %q", got)
	}
}

func TestList_InvalidSortBy(t *testing.T) {
	err := List(ListConfig{Port: 8080, SortBy: "name", Output: &bytes.Buffer{}})
	if err == nil {
		t.Fatal("expected invalid sort-by error")
	}
	if got := err.Error(); got != "--sort-by must be state.type" {
		t.Fatalf("error = %q", got)
	}
}

func stringPtr(value string) *string {
	return &value
}

func assertListPageRequest(t *testing.T, r *http.Request, wantMaxResults string, wantNextToken string) {
	t.Helper()
	if got := r.URL.Query().Get("maxResults"); got != wantMaxResults {
		t.Fatalf("maxResults query = %q, want %q", got, wantMaxResults)
	}
	if got := r.URL.Query().Get("nextToken"); got != wantNextToken {
		t.Fatalf("nextToken query = %q, want %q", got, wantNextToken)
	}
}

func encodeListPageResponse(
	t *testing.T,
	w http.ResponseWriter,
	workName string,
	workID string,
	stateName string,
	stateType factoryapi.WorkStateType,
	nextToken *string,
	pageLabel string,
) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(factoryapi.ListWorkResponse{
		Results: []factoryapi.Work{{
			Name:         workName,
			WorkId:       stringPtr(workID),
			WorkTypeName: stringPtr("story"),
			State: &factoryapi.WorkState{
				Name: stateName,
				Type: stateType,
			},
		}},
		PaginationContext: &factoryapi.PaginationContext{
			MaxResults: 1,
			NextToken:  nextToken,
		},
	}); err != nil {
		t.Fatalf("encode %s response: %v", pageLabel, err)
	}
}

func assertListJSONPage(t *testing.T, payload []byte, wantWorkID string, wantNextToken *string, pageLabel string) {
	t.Helper()
	var response factoryapi.ListWorkResponse
	if err := json.Unmarshal(payload, &response); err != nil {
		t.Fatalf("%s page JSON is invalid: %v\n%s", pageLabel, err, string(payload))
	}
	if len(response.Results) != 1 || stringValue(response.Results[0].WorkId) != wantWorkID {
		t.Fatalf("%s page results = %#v, want only %s", pageLabel, response.Results, wantWorkID)
	}
	if response.PaginationContext == nil || stringValue(response.PaginationContext.NextToken) != stringValue(wantNextToken) {
		t.Fatalf("%s page pagination context = %#v, want nextToken=%q", pageLabel, response.PaginationContext, stringValue(wantNextToken))
	}
}

func selectJSONRelationByType(t *testing.T, work map[string]any, relationType string) map[string]any {
	t.Helper()

	relations, ok := work["relations"].([]any)
	if !ok {
		t.Fatalf("relations = %#v, want JSON array", work["relations"])
	}
	for _, item := range relations {
		relation, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("relation item = %#v, want JSON object", item)
		}
		if relation["type"] == relationType {
			return relation
		}
	}
	t.Fatalf("no relation selected by type=%q from %#v", relationType, relations)
	return nil
}

func selectJSONWorkByState(t *testing.T, response map[string]any, stateName string, stateType string) map[string]any {
	t.Helper()

	results, ok := response["results"].([]any)
	if !ok {
		t.Fatalf("results = %#v, want JSON array", response["results"])
	}
	for _, item := range results {
		work, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("result item = %#v, want JSON object", item)
		}
		state := jsonObject(t, work, "state")
		if state["name"] == stateName && state["type"] == stateType {
			return work
		}
	}
	t.Fatalf("no work selected by state.name=%q and state.type=%q from %#v", stateName, stateType, results)
	return nil
}

func jsonObject(t *testing.T, object map[string]any, key string) map[string]any {
	t.Helper()

	value, ok := object[key].(map[string]any)
	if !ok {
		t.Fatalf("%s = %#v, want JSON object", key, object[key])
	}
	return value
}

func serverPort(t *testing.T, srv *httptest.Server) int {
	t.Helper()

	var port int
	if _, err := fmt.Sscanf(srv.URL, "http://127.0.0.1:%d", &port); err != nil {
		t.Fatalf("parse test server port: %v", err)
	}
	return port
}
