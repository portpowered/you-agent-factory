package work

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestList_SendsTerminalFilterAndCountsBeforePagination(t *testing.T) {
	requestToken := encodeCursor("terminal-page")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("terminal") != "true" || query.Get("nonTerminal") != "" || query.Get("includeSuperseded") != "true" {
			t.Fatalf("terminality/supersession query = %v, want terminal=true and includeSuperseded=true only", query)
		}
		if query.Get("counts") != "true" || query.Get("maxResults") != "1" || query.Get("nextToken") != requestToken {
			t.Fatalf("count/pagination query = %v", query)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListWorkResponse{
			Results: []factoryapi.Work{{
				Name:         "Failed work",
				WorkId:       stringPtr("work-failed"),
				WorkTypeName: stringPtr("story"),
				SupersededBy: stringPtr("work-replacement"),
				State: &factoryapi.WorkState{
					Name: "failed",
					Type: factoryapi.WorkStateTypeFAILED,
				},
			}},
			PaginationContext: &factoryapi.PaginationContext{MaxResults: 1},
			Counts:            &factoryapi.ListWorkCountSummary{Total: 3},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	var output bytes.Buffer
	err := NewList(testHTTPProtocol(t), testListRequestPreparation{})(ListConfig{
		Context:           context.Background(),
		Server:            serverBase(t, server),
		Terminal:          true,
		IncludeSuperseded: true,
		Counts:            true,
		MaxResults:        1,
		NextToken:         requestToken,
		Output:            &output,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := "Total: 3\nWORK ID\tNAME\tWORK TYPE\tSTATE NAME\tSTATE TYPE\tCONFIRMATION STATE\tSTRUCTURED RESULT\tRELATIONS\n" +
		"work-failed\tFailed work\tstory\tfailed\tFAILED\tUNCONFIRMED\t\tnone\n  Superseded by: work-replacement\n"
	if got := output.String(); got != want {
		t.Fatalf("human output = %q, want %q", got, want)
	}
}
