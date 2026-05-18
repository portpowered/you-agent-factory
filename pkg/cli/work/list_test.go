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
