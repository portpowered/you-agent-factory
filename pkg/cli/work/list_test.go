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

	var port int
	if _, err := fmt.Sscanf(srv.URL, "http://127.0.0.1:%d", &port); err != nil {
		t.Fatalf("parse test server port: %v", err)
	}

	var out bytes.Buffer
	err := List(ListConfig{
		Port:      port,
		StateName: "review",
		StateType: "PROCESSING",
		Output:    &out,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if gotQuery == "" {
		t.Fatal("expected request query")
	}
	if got := out.String(); got != "work-1\tReview PRD\treview\tPROCESSING\n" {
		t.Fatalf("output = %q", got)
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

	var port int
	if _, err := fmt.Sscanf(srv.URL, "http://127.0.0.1:%d", &port); err != nil {
		t.Fatalf("parse test server port: %v", err)
	}

	var out bytes.Buffer
	err := List(ListConfig{
		Port:       port,
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

func TestList_InvalidStateType(t *testing.T) {
	err := List(ListConfig{Port: 8080, StateType: "UNKNOWN", Output: &bytes.Buffer{}})
	if err == nil {
		t.Fatal("expected invalid state type error")
	}
	if got := err.Error(); got != "--state-type must be one of INITIAL, PROCESSING, TERMINAL, or FAILED" {
		t.Fatalf("error = %q", got)
	}
}

func stringPtr(value string) *string {
	return &value
}
