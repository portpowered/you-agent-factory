package work

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestList_SendsNonTerminalFilterAndCountsInJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("nonTerminal") != "true" || query.Get("terminal") != "" || query.Get("counts") != "true" {
			t.Fatalf("terminality/count query = %v, want nonTerminal=true and counts=true", query)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListWorkResponse{
			Results:           []factoryapi.Work{},
			PaginationContext: &factoryapi.PaginationContext{MaxResults: 1},
			Counts:            &factoryapi.ListWorkCountSummary{Total: 0},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	var output bytes.Buffer
	err := NewList(testHTTPProtocol(t), testListRequestPreparation{})(ListConfig{
		Context: context.Background(), Server: serverBase(t, server),
		NonTerminal: true, Counts: true, MaxResults: 1, JSON: true,
		Output: &output,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var response factoryapi.ListWorkResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("JSON output = %q: %v", output.String(), err)
	}
	if response.Counts == nil || response.Counts.Total != 0 || len(response.Results) != 0 {
		t.Fatalf("JSON response = %#v, want empty page with counts.total=0", response)
	}
	if strings.Contains(output.String(), "Total:") || strings.Contains(output.String(), "No work found.") {
		t.Fatalf("JSON output contains human text: %q", output.String())
	}
}
