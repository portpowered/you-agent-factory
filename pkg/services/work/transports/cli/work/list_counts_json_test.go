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

func TestList_HumanOutputRendersCompactStructuredResultTypes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListWorkResponse{
			Results: []factoryapi.Work{
				{Name: "object", WorkId: stringPtr("work-object"), StructuredResult: map[string]any{"z": float64(2), "a": "first"}},
				{Name: "array", WorkId: stringPtr("work-array"), StructuredResult: []any{"first", true, float64(3)}},
				{Name: "scalar", WorkId: stringPtr("work-scalar"), StructuredResult: "ready"},
				{Name: "boolean", WorkId: stringPtr("work-bool"), StructuredResult: true},
				{Name: "null", WorkId: stringPtr("work-null"), StructuredResult: json.RawMessage("null")},
				{Name: "omitted", WorkId: stringPtr("work-omitted")},
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := NewList(testHTTPProtocol(t), testListRequestPreparation{})(ListConfig{
		Context: context.Background(), Server: serverBase(t, srv), Output: &out,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	want := "WORK ID\tNAME\tWORK TYPE\tSTATE NAME\tSTATE TYPE\tSTRUCTURED RESULT\tRELATIONS\n" +
		"work-object\tobject\t\t\t\t{\"a\":\"first\",\"z\":2}\tnone\n" +
		"work-array\tarray\t\t\t\t[\"first\",true,3]\tnone\n" +
		"work-scalar\tscalar\t\t\t\t\"ready\"\tnone\n" +
		"work-bool\tboolean\t\t\t\ttrue\tnone\n" +
		"work-null\tnull\t\t\t\tnull\tnone\n" +
		"work-omitted\tomitted\t\t\t\t\tnone\n"
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestList_JSONOutputPreservesNativeStructuredResultAndExplicitNull(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListWorkResponse{
			Results: []factoryapi.Work{
				{WorkId: stringPtr("work-object"), StructuredResult: map[string]any{"count": float64(2)}},
				{WorkId: stringPtr("work-null"), StructuredResult: json.RawMessage("null")},
				{WorkId: stringPtr("work-omitted")},
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := NewList(testHTTPProtocol(t), testListRequestPreparation{})(ListConfig{
		Context: context.Background(), Server: serverBase(t, srv), JSON: true, Output: &out,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	var response struct {
		Results []map[string]json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatalf("decode JSON output: %v\n%s", err, out.String())
	}
	if len(response.Results) != 3 {
		t.Fatalf("results = %#v, want three Work items", response.Results)
	}
	var object map[string]any
	if err := json.Unmarshal(response.Results[0]["structuredResult"], &object); err != nil {
		t.Fatalf("object structuredResult = %s: %v", response.Results[0]["structuredResult"], err)
	}
	if object["count"] != float64(2) {
		t.Fatalf("object structuredResult = %#v, want native object", object)
	}
	null, ok := response.Results[1]["structuredResult"]
	if !ok || !bytes.Equal(bytes.TrimSpace(null), []byte("null")) {
		t.Fatalf("null structuredResult = %q (present=%t), want explicit null", null, ok)
	}
	if _, ok := response.Results[2]["structuredResult"]; ok {
		t.Fatalf("omitted structuredResult = %s, want field omitted", response.Results[2]["structuredResult"])
	}
}
