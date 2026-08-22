package work

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func newVisibleWorkPaginationServer(t *testing.T) *httptest.Server {
	t.Helper()
	secondToken := encodeCursor("cursor-2")
	thirdToken := encodeCursor("cursor-3")
	requestCount := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		requestCount++
		switch requestCount {
		case 1:
			assertListPageRequest(t, r, "1", "")
			assertListQueryPreserved(t, r, "review", "PROCESSING", "Plan", "story", "trace-1")
			encodeListPageResponse(t, w, "Plan feature", "work-1", "init", factoryapi.WorkStateTypeINITIAL, &secondToken, "first", 3)
		case 2:
			assertListPageRequest(t, r, "1", secondToken)
			assertListQueryPreserved(t, r, "review", "PROCESSING", "Plan", "story", "trace-1")
			encodeListPageResponse(t, w, "Review PRD", "work-2", "review", factoryapi.WorkStateTypePROCESSING, &thirdToken, "second", 3)
		case 3:
			assertListPageRequest(t, r, "1", thirdToken)
			assertListQueryPreserved(t, r, "review", "PROCESSING", "Plan", "story", "trace-1")
			encodeListPageResponse(t, w, "Ship Release", "work-3", "done", factoryapi.WorkStateTypeTERMINAL, nil, "third", 3)
		default:
			t.Fatalf("unexpected request count %d", requestCount)
		}
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

func TestList_RepeatedContinuationFailsWithoutPartialOutputOrOpaqueDiagnostics(t *testing.T) {
	const token = "opaque-continuation"
	requestCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		repeatedToken := token
		encodeListPageResponse(t, w, "Repeated page", "work-repeated", "init", factoryapi.WorkStateTypeINITIAL, &repeatedToken, "repeated", 0)
	}))
	defer srv.Close()

	var output bytes.Buffer
	var diagnostics bytes.Buffer
	err := NewList(testHTTPProtocol(t), testListRequestPreparation{})(ListConfig{
		Context: context.Background(), Server: serverBase(t, srv), MaxResults: 1,
		Verbose: true, Output: &output, Diagnostics: &diagnostics,
	})
	if err == nil || !strings.Contains(err.Error(), "work list pagination did not advance after page 2") {
		t.Fatalf("List() error = %v, want repeated continuation error", err)
	}
	if strings.Contains(err.Error(), token) || strings.Contains(diagnostics.String(), token) {
		t.Fatalf("opaque continuation token leaked in failure: error=%q diagnostics=%q", err, diagnostics.String())
	}
	if requestCount != 2 {
		t.Fatalf("HTTP request count = %d, want two before stopping", requestCount)
	}
	if output.Len() != 0 {
		t.Fatalf("stdout = %q, want empty after incomplete traversal", output.String())
	}
	for _, want := range []string{"work list response", "page=2", "nextTokenPresent=true"} {
		if !strings.Contains(diagnostics.String(), want) {
			t.Fatalf("diagnostics missing %q:\n%s", want, diagnostics.String())
		}
	}
}

func TestList_LaterPageTransportFailureDoesNotRenderAccumulatedOutput(t *testing.T) {
	const token = "opaque-transport-continuation"
	requestCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		if requestCount == 1 {
			w.Header().Set("Content-Type", "application/json")
			nextToken := token
			encodeListPageResponse(t, w, "First page", "work-first", "init", factoryapi.WorkStateTypeINITIAL, &nextToken, "first", 0)
			return
		}
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("test server does not support connection hijacking")
		}
		connection, _, err := hijacker.Hijack()
		if err != nil {
			t.Fatalf("hijack page two connection: %v", err)
		}
		_ = connection.Close()
	}))
	defer srv.Close()

	var output bytes.Buffer
	var diagnostics bytes.Buffer
	err := NewList(testHTTPProtocol(t), testListRequestPreparation{})(ListConfig{
		Context: context.Background(), Server: serverBase(t, srv), MaxResults: 1,
		Verbose: true, Output: &output, Diagnostics: &diagnostics,
	})
	if err == nil || !strings.Contains(err.Error(), "work list page 2") {
		t.Fatalf("List() error = %v, want page-two transport error", err)
	}
	if strings.Contains(err.Error(), token) || strings.Contains(diagnostics.String(), token) {
		t.Fatalf("opaque continuation token leaked in failure: error=%q diagnostics=%q", err, diagnostics.String())
	}
	if requestCount < 2 {
		t.Fatalf("HTTP request count = %d, want the failing second page to be attempted", requestCount)
	}
	if output.Len() != 0 {
		t.Fatalf("stdout = %q, want empty after incomplete traversal", output.String())
	}
	for _, want := range []string{"work list response", "page=2", "error=unreachable"} {
		if !strings.Contains(diagnostics.String(), want) {
			t.Fatalf("diagnostics missing %q:\n%s", want, diagnostics.String())
		}
	}
}

func TestList_LaterPageStatusFailureDoesNotRenderAccumulatedOutput(t *testing.T) {
	const token = "opaque-status-continuation"
	requestCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		if requestCount == 1 {
			nextToken := token
			encodeListPageResponse(t, w, "First page", "work-first", "init", factoryapi.WorkStateTypeINITIAL, &nextToken, "first", 0)
			return
		}
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(factoryapi.ErrorResponse{Message: "temporary backend failure"})
	}))
	defer srv.Close()

	var output bytes.Buffer
	var diagnostics bytes.Buffer
	err := NewList(testHTTPProtocol(t), testListRequestPreparation{})(ListConfig{
		Context: context.Background(), Server: serverBase(t, srv), MaxResults: 1,
		Verbose: true, Output: &output, Diagnostics: &diagnostics,
	})
	if err == nil || !strings.Contains(err.Error(), "work list page 2 failed (502): temporary backend failure") {
		t.Fatalf("List() error = %v, want page-two status error", err)
	}
	if strings.Contains(err.Error(), token) || strings.Contains(diagnostics.String(), token) {
		t.Fatalf("opaque continuation token leaked in failure: error=%q diagnostics=%q", err, diagnostics.String())
	}
	if output.Len() != 0 {
		t.Fatalf("stdout = %q, want empty after incomplete traversal", output.String())
	}
	for _, want := range []string{"work list response", "page=2", "status=502"} {
		if !strings.Contains(diagnostics.String(), want) {
			t.Fatalf("diagnostics missing %q:\n%s", want, diagnostics.String())
		}
	}
}

func TestList_LaterPageDecodeFailureDoesNotRenderAccumulatedOutput(t *testing.T) {
	const token = "opaque-decode-continuation"
	requestCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		if requestCount == 1 {
			nextToken := token
			encodeListPageResponse(t, w, "First page", "work-first", "init", factoryapi.WorkStateTypeINITIAL, &nextToken, "first", 0)
			return
		}
		_, _ = w.Write([]byte(`{"results":[`))
	}))
	defer srv.Close()

	var output bytes.Buffer
	var diagnostics bytes.Buffer
	err := NewList(testHTTPProtocol(t), testListRequestPreparation{})(ListConfig{
		Context: context.Background(), Server: serverBase(t, srv), MaxResults: 1,
		Verbose: true, Output: &output, Diagnostics: &diagnostics,
	})
	if err == nil || !strings.Contains(err.Error(), "work list page 2 response decode") {
		t.Fatalf("List() error = %v, want page-two decode error", err)
	}
	if strings.Contains(err.Error(), "factory not reachable") {
		t.Fatalf("decode failure was misclassified as unreachable: %v", err)
	}
	if strings.Contains(err.Error(), token) || strings.Contains(diagnostics.String(), token) {
		t.Fatalf("opaque continuation token leaked in failure: error=%q diagnostics=%q", err, diagnostics.String())
	}
	if output.Len() != 0 {
		t.Fatalf("stdout = %q, want empty after incomplete traversal", output.String())
	}
	for _, want := range []string{"work list response", "page=2", "error=decode"} {
		if !strings.Contains(diagnostics.String(), want) {
			t.Fatalf("diagnostics missing %q:\n%s", want, diagnostics.String())
		}
	}
}

func TestList_CancellationAfterPageStopsFurtherRequestsAndOutput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	requestCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		nextToken := "cancel-continuation"
		encodeListPageResponse(t, w, "First page", "work-first", "init", factoryapi.WorkStateTypeINITIAL, &nextToken, "first", 0)
		cancel()
	}))
	defer srv.Close()

	var output bytes.Buffer
	err := NewList(testHTTPProtocol(t), testListRequestPreparation{})(ListConfig{
		Context: ctx, Server: serverBase(t, srv), MaxResults: 1, Output: &output,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("List() error = %v, want context.Canceled", err)
	}
	if requestCount != 1 {
		t.Fatalf("HTTP request count = %d, want no request after cancellation", requestCount)
	}
	if output.Len() != 0 {
		t.Fatalf("stdout = %q, want empty after cancellation", output.String())
	}
}
