package work

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestList_ExplicitMaxResultsRendersOneBoundedServerPage(t *testing.T) {
	const pageSize = 5
	nextToken := encodeCursor("work-5")
	srv, requestCount := newExplicitMaxResultsServer(t, pageSize, nextToken)
	defer srv.Close()

	var output bytes.Buffer
	err := NewList(testHTTPProtocol(t), testListRequestPreparation{})(ListConfig{
		Context:    context.Background(),
		Server:     serverBase(t, srv),
		MaxResults: pageSize,
		JSON:       true,
		Output:     &output,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	var response factoryapi.ListWorkResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("JSON output is invalid: %v\n%s", err, output.String())
	}
	if *requestCount != 1 {
		t.Fatalf("HTTP request count = %d, want one server page", *requestCount)
	}
	if len(response.Results) != pageSize {
		t.Fatalf("results = %d, want %d rows from one bounded page", len(response.Results), pageSize)
	}
	if response.PaginationContext == nil || response.PaginationContext.MaxResults != pageSize || response.PaginationContext.NextToken == nil || *response.PaginationContext.NextToken != nextToken {
		t.Fatalf("paginationContext = %#v, want maxResults=%d and usable nextToken", response.PaginationContext, pageSize)
	}
}

func newExplicitMaxResultsServer(t *testing.T, pageSize int, nextToken string) (*httptest.Server, *int) {
	t.Helper()
	requestCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if got := r.URL.Query().Get("maxResults"); got != "5" {
			t.Fatalf("maxResults query = %q, want 5", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch requestCount {
		case 1:
			results := make([]factoryapi.Work, 0, pageSize)
			for index := 1; index <= pageSize; index++ {
				workID := fmt.Sprintf("work-%d", index)
				results = append(results, factoryapi.Work{Name: workID, WorkId: stringPtr(workID)})
			}
			if err := json.NewEncoder(w).Encode(factoryapi.ListWorkResponse{
				Results: results,
				PaginationContext: &factoryapi.PaginationContext{
					MaxResults: pageSize,
					NextToken:  &nextToken,
				},
			}); err != nil {
				t.Fatalf("encode first page: %v", err)
			}
		case 2:
			if got := r.URL.Query().Get("nextToken"); got != nextToken {
				t.Fatalf("unexpected automatic continuation token %q, want no second request", got)
			}
			if err := json.NewEncoder(w).Encode(factoryapi.ListWorkResponse{
				Results:           []factoryapi.Work{{Name: "work-6", WorkId: stringPtr("work-6")}},
				PaginationContext: &factoryapi.PaginationContext{MaxResults: pageSize},
			}); err != nil {
				t.Fatalf("encode second page: %v", err)
			}
		default:
			t.Fatalf("unexpected request count %d", requestCount)
		}
	}))
	return srv, &requestCount
}

func newVisibleWorkPaginationServer(t *testing.T) *httptest.Server {
	t.Helper()
	secondToken := encodeCursor("cursor-2")
	requestCount := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		requestCount++
		if requestCount != 1 {
			t.Fatalf("unexpected request count %d", requestCount)
		}
		assertListPageRequest(t, r, "1", "")
		assertListQueryPreserved(t, r, "review", "PROCESSING", "Plan", "story", "trace-1")
		encodeListPageResponse(t, w, "Plan feature", "work-1", "init", factoryapi.WorkStateTypeINITIAL, &secondToken, "first", 3)
	}))
}

func TestList_VerboseLogsFailureStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		if err := json.NewEncoder(w).Encode(factoryapi.ErrorResponse{Message: "service unavailable", Code: "INTERNAL_ERROR"}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	var diagnostics bytes.Buffer
	err := NewList(testHTTPProtocol(t), testListRequestPreparation{})(ListConfig{Context: context.Background(),
		Server:      serverBase(t, srv),
		Verbose:     true,
		Output:      &out,
		Diagnostics: &diagnostics,
	})
	if err == nil {
		t.Fatal("expected list failure")
	}
	diag := diagnostics.String()
	if !strings.Contains(diag, "work list response") || !strings.Contains(diag, "status=500") {
		t.Fatalf("diagnostics missing failure status:\n%s", diag)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout should stay empty on failure, got %q", out.String())
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
	err := NewList(testHTTPProtocol(t), testListRequestPreparation{})(ListConfig{Context: context.Background(),
		Server: serverBase(t, srv),
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

func TestList_ContinuationTokenIsReturnedForExplicitFollowUp(t *testing.T) {
	token := encodeCursor("cursor-2")
	requestCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if got := r.URL.Query().Get("nextToken"); got != "" {
			t.Fatalf("first request nextToken = %q, want omitted", got)
		}
		w.Header().Set("Content-Type", "application/json")
		nextToken := token
		encodeListPageResponse(t, w, "First page", "work-first", "init", factoryapi.WorkStateTypeINITIAL, &nextToken, "first", 2)
	}))
	defer srv.Close()

	var output bytes.Buffer
	err := NewList(testHTTPProtocol(t), testListRequestPreparation{})(ListConfig{
		Context: context.Background(), Server: serverBase(t, srv), MaxResults: 1, JSON: true, Output: &output,
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	var response factoryapi.ListWorkResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("JSON output is invalid: %v\n%s", err, output.String())
	}
	if requestCount != 1 || len(response.Results) != 1 {
		t.Fatalf("request count/results = %d/%d, want one bounded page", requestCount, len(response.Results))
	}
	if response.PaginationContext == nil || response.PaginationContext.NextToken == nil || *response.PaginationContext.NextToken != token {
		t.Fatalf("paginationContext = %#v, want returned continuation token", response.PaginationContext)
	}
}

func TestList_CanceledContextDoesNotWriteOutput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var output bytes.Buffer
	err := NewList(testHTTPProtocol(t), testListRequestPreparation{})(ListConfig{
		Context: ctx, Server: "http://127.0.0.1:1", Output: &output,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("List() error = %v, want context.Canceled", err)
	}
	if output.Len() != 0 {
		t.Fatalf("stdout = %q, want empty after cancellation", output.String())
	}
}
