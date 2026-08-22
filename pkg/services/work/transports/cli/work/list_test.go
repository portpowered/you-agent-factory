package work

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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
	err := NewList(testHTTPProtocol(t), testListRequestPreparation{})(ListConfig{Context: context.Background(),
		Server:    serverBase(t, srv),
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
	if got := out.String(); got != "WORK ID\tNAME\tWORK TYPE\tSTATE NAME\tSTATE TYPE\tSTRUCTURED RESULT\tRELATIONS\nwork-1\tReview PRD\tstory\treview\tPROCESSING\t\tnone\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestList_SendsNameAndWorkTypeNameFilters(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		if r.URL.Query().Get("name") != "prd" {
			t.Fatalf("name query = %q, want prd", r.URL.Query().Get("name"))
		}
		if r.URL.Query().Get("workTypeName") != "story" {
			t.Fatalf("workTypeName query = %q, want story", r.URL.Query().Get("workTypeName"))
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListWorkResponse{}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := NewList(testHTTPProtocol(t), testListRequestPreparation{})(ListConfig{Context: context.Background(),
		Server:       serverBase(t, srv),
		Name:         "prd",
		WorkTypeName: "story",
		Output:       &out,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if gotQuery == "" {
		t.Fatal("expected request query")
	}
	if got := out.String(); got != "No work found.\n" {
		t.Fatalf("output = %q, want empty-state output", got)
	}
}

func TestList_SendsTraceIdFilter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("traceId") != "trace-submit-1" {
			t.Fatalf("traceId query = %q, want trace-submit-1", r.URL.Query().Get("traceId"))
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListWorkResponse{}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := NewList(testHTTPProtocol(t), testListRequestPreparation{})(ListConfig{Context: context.Background(),
		Server:  serverBase(t, srv),
		TraceID: "trace-submit-1",
		Output:  &out,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
}

func TestList_VerboseDiagnosticsIncludeActiveFilterKeys(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListWorkResponse{}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	var diagnostics bytes.Buffer
	err := NewList(testHTTPProtocol(t), testListRequestPreparation{FilterSummary: "name,workTypeName,traceId"})(ListConfig{Context: context.Background(),
		Server:       serverBase(t, srv),
		Name:         "alpha",
		WorkTypeName: "story",
		TraceID:      "trace-1",
		Verbose:      true,
		Output:       &out,
		Diagnostics:  &diagnostics,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	diag := diagnostics.String()
	if !strings.Contains(diag, "filters=name,workTypeName,traceId") {
		t.Fatalf("diagnostics missing active filter keys:\n%s", diag)
	}
}

func TestList_SessionScopedRouteUsesFactorySessionPath(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListWorkResponse{}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := NewList(testHTTPProtocol(t), testListRequestPreparation{})(ListConfig{Context: context.Background(),
		Server:    serverBase(t, srv),
		SessionID: "session/beta",
		Output:    &out,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if gotPath != "/factory-sessions/session%2Fbeta/work" {
		t.Fatalf("path = %q, want /factory-sessions/session%%2Fbeta/work", gotPath)
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
	err := NewList(testHTTPProtocol(t), testListRequestPreparation{})(ListConfig{Context: context.Background(),
		Server: serverBase(t, srv),
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
	err := NewList(testHTTPProtocol(t), testListRequestPreparation{})(ListConfig{Context: context.Background(),
		Server: serverBase(t, srv),
		Output: &out,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	want := "WORK ID\tNAME\tWORK TYPE\tSTATE NAME\tSTATE TYPE\tSTRUCTURED RESULT\tRELATIONS\n" +
		"work-1\tReview PRD\tstory\treview\tPROCESSING\t\tnone\n"
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestList_HumanOutputLeavesWorkTypeEmptyWhenAbsent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListWorkResponse{
			Results: []factoryapi.Work{{
				Name:   "Legacy work",
				WorkId: stringPtr("work-legacy"),
				State: &factoryapi.WorkState{
					Name: "init",
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
		Output: &out,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	want := "WORK ID\tNAME\tWORK TYPE\tSTATE NAME\tSTATE TYPE\tSTRUCTURED RESULT\tRELATIONS\n" +
		"work-legacy\tLegacy work\t\tinit\tINITIAL\t\tnone\n"
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
	err := NewList(testHTTPProtocol(t), testListRequestPreparation{})(ListConfig{Context: context.Background(),
		Server: serverBase(t, srv),
		Output: &out,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	want := "WORK ID\tNAME\tWORK TYPE\tSTATE NAME\tSTATE TYPE\tSTRUCTURED RESULT\tRELATIONS\n" +
		"work-1\tPlan feature\tstory\tinit\tINITIAL\t\tnone\n" +
		"work-2\tReview PRD\tstory\treview\tPROCESSING\t\tnone\n"
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
	err := NewList(testHTTPProtocol(t), testListRequestPreparation{})(ListConfig{Context: context.Background(),
		Server: serverBase(t, srv),
		Output: &out,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	got := out.String()
	want := "WORK ID\tNAME\tWORK TYPE\tSTATE NAME\tSTATE TYPE\tSTRUCTURED RESULT\tRELATIONS\n" +
		"work-1\tPlan feature\tstory\tinit\tINITIAL\t\tnone\n" +
		"work-2\tReview PRD\tstory\treview\tPROCESSING\t\tnone\n"
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
	err := NewList(testHTTPProtocol(t), testListRequestPreparation{})(ListConfig{Context: context.Background(),
		Server: serverBase(t, srv),
		Output: &out,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	want := "WORK ID\tNAME\tWORK TYPE\tSTATE NAME\tSTATE TYPE\tSTRUCTURED RESULT\tRELATIONS\n" +
		"work-1\tReview PRD\tstory\treview\tPROCESSING\t\tDEPENDS_ON: Draft PRD [work-draft] (requires complete)\n"
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
	err := NewList(testHTTPProtocol(t), testListRequestPreparation{})(ListConfig{Context: context.Background(),
		Server: serverBase(t, srv),
		Output: &out,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	want := "WORK ID\tNAME\tWORK TYPE\tSTATE NAME\tSTATE TYPE\tSTRUCTURED RESULT\tRELATIONS\n" +
		"work-3\tPublish Release\tstory\tblocked\tFAILED\t\tDEPENDS_ON: Review PRD [work-review] (requires reviewed); PARENT_CHILD: Epic Release [work-epic]; SPAWNED_BY: Release Train [work-parent]\n"
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestList_SendsPaginationControlsAndEmitsJSONResponse(t *testing.T) {
	requestToken := encodeCursor("cursor-1")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("maxResults") != "2" {
			t.Fatalf("maxResults query = %q, want 2", r.URL.Query().Get("maxResults"))
		}
		if r.URL.Query().Get("nextToken") != requestToken {
			t.Fatalf("nextToken query = %q, want %q", r.URL.Query().Get("nextToken"), requestToken)
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
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := NewList(testHTTPProtocol(t), testListRequestPreparation{})(ListConfig{Context: context.Background(),
		Server:     serverBase(t, srv),
		MaxResults: 2,
		NextToken:  requestToken,
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
	if got.PaginationContext == nil || got.PaginationContext.MaxResults != 2 || got.PaginationContext.NextToken != nil {
		t.Fatalf("pagination context = %#v, want maxResults=2 and no continuation", got.PaginationContext)
	}
}

func TestList_JSONVerboseKeepsStdoutParseableAndDiagnosticsSeparate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListWorkResponse{
			Results: []factoryapi.Work{{
				Name:   "Review PRD",
				WorkId: stringPtr("work-1"),
				State: &factoryapi.WorkState{
					Name: "review",
					Type: factoryapi.WorkStateTypePROCESSING,
				},
			}},
			PaginationContext: &factoryapi.PaginationContext{
				MaxResults: 1,
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	var diagnostics bytes.Buffer
	err := NewList(testHTTPProtocol(t), testListRequestPreparation{FilterSummary: "state.name,state.type"})(ListConfig{Context: context.Background(),
		Server:      serverBase(t, srv),
		SessionID:   "session-alpha",
		StateName:   "review",
		StateType:   "PROCESSING",
		MaxResults:  1,
		NextToken:   encodeCursor("cursor-1"),
		JSON:        true,
		Verbose:     true,
		Output:      &out,
		Diagnostics: &diagnostics,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var got factoryapi.ListWorkResponse
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("json output is invalid: %v\n%s", err, out.String())
	}
	diag := diagnostics.String()
	for _, want := range []string{
		"work list request",
		"endpointPath=/factory-sessions/session-alpha/work",
		"session=session-alpha",
		"filters=state.name,state.type",
		"maxResults=1",
		"nextTokenPresent=true",
		"work list response",
		"status=200",
		"resultCount=1",
	} {
		if !strings.Contains(diag, want) {
			t.Fatalf("diagnostics missing %q:\n%s", want, diag)
		}
	}
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

func TestList_JSONOutputOmitsResourcesAndPreservesPaginationAcrossVisibleWorkPages(t *testing.T) {
	secondToken := encodeCursor("cursor-2")
	thirdToken := encodeCursor("cursor-3")
	var requestCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	defer srv.Close()

	var output bytes.Buffer
	err := NewList(testHTTPProtocol(t), testListRequestPreparation{})(ListConfig{Context: context.Background(),
		Server:            serverBase(t, srv),
		SessionID:         "session/alpha",
		StateName:         "review",
		StateType:         "PROCESSING",
		Name:              "Plan",
		WorkTypeName:      "story",
		TraceID:           "trace-1",
		IncludeSuperseded: true,
		SortBy:            "state.type",
		MaxResults:        1,
		Counts:            true,
		JSON:              true,
		Output:            &output,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if bytes.Contains(output.Bytes(), []byte("executor-slot")) {
		t.Fatalf("aggregate JSON included runtime resource text: %q", output.String())
	}
	var response factoryapi.ListWorkResponse
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("aggregate JSON is invalid: %v\n%s", err, output.String())
	}
	if len(response.Results) != 3 || stringValue(response.Results[0].WorkId) != "work-1" || stringValue(response.Results[1].WorkId) != "work-2" || stringValue(response.Results[2].WorkId) != "work-3" {
		t.Fatalf("aggregate results = %#v, want work-1, work-2, work-3 in server order", response.Results)
	}
	if response.PaginationContext == nil || response.PaginationContext.NextToken != nil {
		t.Fatalf("aggregate pagination = %#v, want exhausted continuation", response.PaginationContext)
	}
	if response.Counts == nil || response.Counts.Total != 3 {
		t.Fatalf("aggregate counts = %#v, want total 3", response.Counts)
	}
}

func TestList_HumanOutputAggregatesThreePagesWithOneHeader(t *testing.T) {
	secondToken := encodeCursor("human-page-2")
	thirdToken := encodeCursor("human-page-3")
	requestCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		switch requestCount {
		case 1:
			encodeListPageResponse(t, w, "Plan feature", "work-1", "init", factoryapi.WorkStateTypeINITIAL, &secondToken, "first", 0)
		case 2:
			encodeListPageResponse(t, w, "Review PRD", "work-2", "review", factoryapi.WorkStateTypePROCESSING, &thirdToken, "second", 0)
		case 3:
			encodeListPageResponse(t, w, "Ship Release", "work-3", "done", factoryapi.WorkStateTypeTERMINAL, nil, "third", 0)
		default:
			t.Fatalf("unexpected request count %d", requestCount)
		}
	}))
	defer srv.Close()

	var output bytes.Buffer
	err := NewList(testHTTPProtocol(t), testListRequestPreparation{})(ListConfig{
		Context: context.Background(), Server: serverBase(t, srv), MaxResults: 1, Output: &output,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := "WORK ID\tNAME\tWORK TYPE\tSTATE NAME\tSTATE TYPE\tSTRUCTURED RESULT\tRELATIONS\n" +
		"work-1\tPlan feature\tstory\tinit\tINITIAL\t\tnone\n" +
		"work-2\tReview PRD\tstory\treview\tPROCESSING\t\tnone\n" +
		"work-3\tShip Release\tstory\tdone\tTERMINAL\t\tnone\n"
	if output.String() != want {
		t.Fatalf("human output = %q, want %q", output.String(), want)
	}
	if strings.Count(output.String(), "WORK ID\tNAME\tWORK TYPE") != 1 {
		t.Fatalf("human output header count = %d, want one", strings.Count(output.String(), "WORK ID\tNAME\tWORK TYPE"))
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this JSON output test keeps the generated page shape assertions together so the CLI surface stays reviewer-readable.
func TestList_JSONOutputPreservesGeneratedResponseShape(t *testing.T) {
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
			},
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
	if work["workId"] != "work-1" || work["workTypeName"] != "story" || state["name"] != "review" || state["type"] != "PROCESSING" {
		t.Fatalf("work JSON = %#v, want workId, workTypeName, and structured state fields", work)
	}
	pagination, ok := got["paginationContext"].(map[string]any)
	if !ok {
		t.Fatalf("paginationContext = %#v, want JSON object", got["paginationContext"])
	}
	if pagination["maxResults"] != float64(1) {
		t.Fatalf("paginationContext = %#v, want maxResults=1", pagination)
	}
	if _, hasNextToken := pagination["nextToken"]; hasNextToken {
		t.Fatalf("paginationContext = %#v, want no continuation after exhaustion", pagination)
	}
}

func TestList_JSONOutputSupportsAutomationSelectionWithFiltersAndPagination(t *testing.T) {
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
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := NewList(testHTTPProtocol(t), testListRequestPreparation{})(ListConfig{Context: context.Background(),
		Server:     serverBase(t, srv),
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
	if pagination["maxResults"] != float64(1) {
		t.Fatalf("paginationContext = %#v, want maxResults=1", pagination)
	}
	if _, hasNextToken := pagination["nextToken"]; hasNextToken {
		t.Fatalf("paginationContext = %#v, want no continuation after exhaustion", pagination)
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

func stringPtr(value string) *string {
	return &value
}

func encodeCursor(value string) string {
	return base64.StdEncoding.EncodeToString([]byte(value))
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

func assertListQueryPreserved(
	t *testing.T,
	r *http.Request,
	wantStateName string,
	wantStateType string,
	wantName string,
	wantWorkTypeName string,
	wantTraceID string,
) {
	t.Helper()
	query := r.URL.Query()
	for key, want := range map[string]string{
		"state.name":        wantStateName,
		"state.type":        wantStateType,
		"name":              wantName,
		"workTypeName":      wantWorkTypeName,
		"traceId":           wantTraceID,
		"sortBy":            "state.type",
		"maxResults":        "1",
		"counts":            "true",
		"includeSuperseded": "true",
	} {
		if got := query.Get(key); got != want {
			t.Fatalf("%s query = %q, want %q", key, got, want)
		}
	}
	if got := r.URL.EscapedPath(); got != "/factory-sessions/session%2Falpha/work" {
		t.Fatalf("session path = %q, want escaped session path", got)
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
	count int,
) {
	t.Helper()
	response := factoryapi.ListWorkResponse{
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
	}
	if count > 0 {
		response.Counts = &factoryapi.ListWorkCountSummary{Total: count}
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		t.Fatalf("encode %s response: %v", pageLabel, err)
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

func serverBase(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	return strings.TrimSuffix(srv.URL, "/")
}
