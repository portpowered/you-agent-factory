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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	if got := out.String(); got != "WORK ID\tNAME\tSTATE NAME\tSTATE TYPE\nwork-1\tReview PRD\treview\tPROCESSING\n" {
		t.Fatalf("output = %q", got)
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

	want := "WORK ID\tNAME\tSTATE NAME\tSTATE TYPE\n" +
		"work-1\tReview PRD\treview\tPROCESSING\n"
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

	want := "WORK ID\tNAME\tSTATE NAME\tSTATE TYPE\n" +
		"work-1\tPlan feature\tinit\tINITIAL\n" +
		"work-2\tReview PRD\treview\tPROCESSING\n"
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
	pagination := jsonObject(t, got, "paginationContext")
	if pagination["maxResults"] != float64(1) || pagination["nextToken"] != nextToken {
		t.Fatalf("paginationContext = %#v, want maxResults=1 nextToken=%q", pagination, nextToken)
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
