package session

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	fse "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysession"
)

var canonicalListRequestPreparation RequestPreparation = listRequestPreparation{}

type listRequestPreparation struct{}

func (listRequestPreparation) PrepareListSessions(request fse.ListSessionsRequest) (fse.ListSessionsRequest, error) {
	if request.Scope == "" {
		request.Scope = fse.SessionListScopeLive
	}
	switch request.Scope {
	case fse.SessionListScopeLive, fse.SessionListScopePersisted, fse.SessionListScopeAll:
		return request, nil
	default:
		return fse.ListSessionsRequest{}, &fse.ExecutionValidationError{Field: "scope", Message: fmt.Sprintf("scope must be live, persisted, or all (got %q)", request.Scope)}
	}
}
func (listRequestPreparation) PrepareStart(fse.StartRequest) (fse.StartRequest, error) {
	panic("unexpected PrepareStart")
}
func (listRequestPreparation) PrepareControl(fse.ControlRequest) (fse.ControlRequest, error) {
	panic("unexpected PrepareControl")
}
func (listRequestPreparation) PrepareApprove(fse.ApproveRequest) (fse.ApproveRequest, error) {
	panic("unexpected PrepareApprove")
}
func (listRequestPreparation) PrepareRetryDispatch(fse.RetryDispatchRequest) (fse.RetryDispatchRequest, error) {
	panic("unexpected PrepareRetryDispatch")
}
func (listRequestPreparation) PrepareInterruptDispatch(fse.InterruptDispatchRequest) (fse.InterruptDispatchRequest, error) {
	panic("unexpected PrepareInterruptDispatch")
}
func (listRequestPreparation) PrepareResult(fse.ResultRequest) (fse.ResultRequest, error) {
	panic("unexpected PrepareResult")
}
func (listRequestPreparation) PrepareEventReconnect(fse.EventReconnectRequest) (fse.EventReconnectRequest, error) {
	panic("unexpected PrepareEventReconnect")
}

func newContractDurableLister(t *testing.T) durableSessionLister {
	t.Helper()
	return func(_ context.Context, request fse.ListSessionsRequest) (fse.ListSessionsResult, error) {
		if request.Scope != fse.SessionListScopeAll {
			t.Fatalf("durable list scope = %q, want all", request.Scope)
		}
		return contractDurableListResult(), nil
	}
}

func contractDurableListResult() fse.ListSessionsResult {
	return fse.ListSessionsResult{
		Scope: fse.SessionListScopeAll,
		DurableSessions: []fse.DurableSessionListSummary{
			{
				SessionID:        "dur-sess-js-success-002",
				Status:           fse.LifecycleStatusSucceeded,
				OrchestratorKind: "JAVASCRIPT",
				ResolvedSource: fse.ResolvedSource{
					Kind:      "WORKFLOW_FILE",
					SourceRef: "workflow/.claude/workflows/docs-refresh.yaml",
				},
				Policy:        fse.PolicyProjection{EffectiveHash: "eff-policy-docs-refresh"},
				Phase:         "complete",
				ResultSummary: &fse.ResultSummary{ResultStatus: "FINAL"},
				Actions:       fse.SessionActionAvailability{CanTerminate: true},
			},
			{
				SessionID:        "dur-sess-petri-success-001",
				Status:           fse.LifecycleStatusSucceeded,
				OrchestratorKind: "PETRI_NET",
				ResolvedSource:   fse.ResolvedSource{Kind: "FACTORY_ID", SourceRef: "factory/customer-support-triage"},
				ResultSummary:    &fse.ResultSummary{ResultStatus: "FINAL"},
			},
		},
	}
}

func TestList_PerformsGETFactorySessions(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListFactorySessionsResponse{
			Sessions: []factoryapi.FactorySessionSummary{sampleSessionSummary()},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := NewList(testHTTPProtocol(t), canonicalListRequestPreparation)(ListConfig{Context: context.Background(),
		Server: srv.URL,
		Output: &out,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if gotPath != "/factory-sessions" {
		t.Fatalf("path = %q, want /factory-sessions", gotPath)
	}
}

func TestList_HumanOutputShowsEmptyState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListFactorySessionsResponse{
			Sessions: []factoryapi.FactorySessionSummary{},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := NewList(testHTTPProtocol(t), canonicalListRequestPreparation)(ListConfig{Context: context.Background(),
		Port:   serverPort(t, srv),
		Output: &out,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got := out.String(); got != "No live factory sessions were found.\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestList_HumanOutputRendersSessionTable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListFactorySessionsResponse{
			Sessions: []factoryapi.FactorySessionSummary{
				{
					FactoryDir: "/workspace/root",
					FolderPath: "/workspace/root",
					Id:         "~default",
					IsDefault:  true,
					Project:    "root",
					Target: factoryapi.FactorySessionTargetRef{
						Kind: factoryapi.FactorySessionTargetRefKindDefault,
					},
				},
				{
					FactoryDir: "/workspace/root/beta",
					FolderPath: "/workspace/root",
					Id:         "session-beta",
					IsDefault:  false,
					Project:    "beta",
					Target: factoryapi.FactorySessionTargetRef{
						Kind: factoryapi.FactorySessionTargetRefKindNamed,
						Name: stringPtr("beta"),
					},
				},
			},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := NewList(testHTTPProtocol(t), canonicalListRequestPreparation)(ListConfig{Context: context.Background(),
		Port:   serverPort(t, srv),
		Output: &out,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	want := "SESSION ID\tPROJECT\tFOLDER PATH\tFACTORY DIR\tDEFAULT\tORCHESTRATOR KIND\tTARGET KIND\tTARGET NAME\n" +
		"~default\troot\t/workspace/root\t/workspace/root\tyes\t\tdefault\t\n" +
		"session-beta\tbeta\t/workspace/root\t/workspace/root/beta\tno\t\tnamed\tbeta\n"
	if got := out.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestList_JSONModeEmitsListFactorySessionsResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListFactorySessionsResponse{
			Sessions: []factoryapi.FactorySessionSummary{sampleSessionSummary()},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := NewList(testHTTPProtocol(t), canonicalListRequestPreparation)(ListConfig{Context: context.Background(),
		Port:   serverPort(t, srv),
		JSON:   true,
		Output: &out,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	var got factoryapi.ListFactorySessionsResponse
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, out.String())
	}
	if len(got.Sessions) != 1 || got.Sessions[0].Id != "session-alpha" {
		t.Fatalf("sessions = %#v, want one session-alpha summary", got.Sessions)
	}
	if strings.Contains(out.String(), "SESSION ID") {
		t.Fatalf("JSON output included human table header: %q", out.String())
	}
}

func TestList_UnreachableServiceNamesEndpoint(t *testing.T) {
	var out bytes.Buffer
	err := NewList(testHTTPProtocol(t), canonicalListRequestPreparation)(ListConfig{Context: context.Background(),
		Port:   1,
		JSON:   true,
		Output: &out,
	})
	if err == nil {
		t.Fatal("expected unreachable error")
	}
	if !strings.Contains(err.Error(), "factory sessions endpoint not reachable at http://localhost:1/factory-sessions") {
		t.Fatalf("error = %q", err.Error())
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty output when --json is set", out.String())
	}
}

func TestList_PersistedScopeRendersDurableJavaScriptFactorySessions(t *testing.T) {
	var out bytes.Buffer
	err := NewList(testHTTPProtocol(t), canonicalListRequestPreparation)(ListConfig{Context: context.Background(),
		Scope:         "persisted",
		Output:        &out,
		DurableLister: newContractDurableLister(t),
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Factory Sessions (durable):") {
		t.Fatalf("output missing durable section: %q", output)
	}
	for _, want := range []string{
		"dur-sess-js-success-002",
		"SUCCEEDED",
		"JAVASCRIPT",
		"WORKFLOW_FILE",
		"workflow/.claude/workflows/docs-refresh.yaml",
		"FINAL",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("output missing %q:\n%s", want, output)
		}
	}
}

func TestList_PersistedScopeEmptyStateWithoutMatchingRows(t *testing.T) {
	emptyLister := func(context.Context, fse.ListSessionsRequest) (fse.ListSessionsResult, error) {
		return fse.ListSessionsResult{Scope: fse.SessionListScopePersisted}, nil
	}

	var out bytes.Buffer
	err := NewList(testHTTPProtocol(t), canonicalListRequestPreparation)(ListConfig{Context: context.Background(),
		Scope:         "persisted",
		Output:        &out,
		DurableLister: emptyLister,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got := out.String(); got != "No persisted Factory Sessions were found.\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestList_AllScopeCombinesLiveHTTPAndDurableProviderRows(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListFactorySessionsResponse{
			Sessions: []factoryapi.FactorySessionSummary{sampleSessionSummary()},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := NewList(testHTTPProtocol(t), canonicalListRequestPreparation)(ListConfig{Context: context.Background(),
		Scope:         "all",
		Port:          serverPort(t, srv),
		Output:        &out,
		DurableLister: newContractDurableLister(t),
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "SESSION ID\tPROJECT\tFOLDER PATH") {
		t.Fatalf("output missing live table header: %q", output)
	}
	if !strings.Contains(output, "session-alpha") {
		t.Fatalf("output missing live session row: %q", output)
	}
	if !strings.Contains(output, "Factory Sessions (durable):") {
		t.Fatalf("output missing durable section: %q", output)
	}
	if !strings.Contains(output, "dur-sess-petri-success-001") {
		t.Fatalf("output missing persisted durable row: %q", output)
	}
}

func TestList_UnsupportedScopeReturnsCommandError(t *testing.T) {
	var out bytes.Buffer
	err := NewList(testHTTPProtocol(t), canonicalListRequestPreparation)(ListConfig{Context: context.Background(),
		Scope:  "workspace",
		Output: &out,
	})
	if err == nil {
		t.Fatal("expected unsupported scope to fail")
	}
	if !strings.Contains(err.Error(), "scope must be live, persisted, or all") {
		t.Fatalf("error = %q", err.Error())
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty output on scope validation failure", out.String())
	}
}

func TestList_UnsupportedScopeJSONKeepsStdoutEmpty(t *testing.T) {
	var out bytes.Buffer
	err := NewList(testHTTPProtocol(t), canonicalListRequestPreparation)(ListConfig{Context: context.Background(),
		Scope:  "workspace",
		JSON:   true,
		Output: &out,
	})
	if err == nil {
		t.Fatal("expected unsupported scope to fail")
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty JSON output on scope validation failure", out.String())
	}
}

func TestList_PersistedScopeProviderFailureReturnsErrorWithoutPartialOutput(t *testing.T) {
	failingLister := func(context.Context, fse.ListSessionsRequest) (fse.ListSessionsResult, error) {
		return fse.ListSessionsResult{}, fmt.Errorf("provider unavailable")
	}

	var out bytes.Buffer
	err := NewList(testHTTPProtocol(t), canonicalListRequestPreparation)(ListConfig{Context: context.Background(),
		Scope:         "persisted",
		Output:        &out,
		DurableLister: failingLister,
	})
	if err == nil {
		t.Fatal("expected provider failure")
	}
	if !strings.Contains(err.Error(), "list durable factory sessions failed") {
		t.Fatalf("error = %q", err.Error())
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty output on provider failure", out.String())
	}
}

func TestList_PersistedScopeProviderFailureJSONKeepsStdoutEmpty(t *testing.T) {
	failingLister := func(context.Context, fse.ListSessionsRequest) (fse.ListSessionsResult, error) {
		return fse.ListSessionsResult{}, fmt.Errorf("provider unavailable")
	}

	var out bytes.Buffer
	err := NewList(testHTTPProtocol(t), canonicalListRequestPreparation)(ListConfig{Context: context.Background(),
		Scope:         "persisted",
		JSON:          true,
		Output:        &out,
		DurableLister: failingLister,
	})
	if err == nil {
		t.Fatal("expected provider failure")
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty JSON output on provider failure", out.String())
	}
}

func TestList_AllScopeProviderFailureReturnsErrorWithoutPartialOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListFactorySessionsResponse{
			Sessions: []factoryapi.FactorySessionSummary{sampleSessionSummary()},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	failingLister := func(context.Context, fse.ListSessionsRequest) (fse.ListSessionsResult, error) {
		return fse.ListSessionsResult{}, fmt.Errorf("provider unavailable")
	}

	var out bytes.Buffer
	err := NewList(testHTTPProtocol(t), canonicalListRequestPreparation)(ListConfig{Context: context.Background(),
		Scope:         "all",
		Port:          serverPort(t, srv),
		Output:        &out,
		DurableLister: failingLister,
	})
	if err == nil {
		t.Fatal("expected provider failure")
	}
	if !strings.Contains(err.Error(), "list durable factory sessions failed") {
		t.Fatalf("error = %q", err.Error())
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty output on provider failure", out.String())
	}
}

func TestList_PersistedScopeJSONMatchesCanonicalListResponse(t *testing.T) {
	lister := newContractDurableLister(t)
	want := canonicalListResponse(t, lister, fse.SessionListScopePersisted, nil)

	var out bytes.Buffer
	err := NewList(testHTTPProtocol(t), canonicalListRequestPreparation)(ListConfig{Context: context.Background(),
		Scope:         "persisted",
		JSON:          true,
		Output:        &out,
		DurableLister: lister,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	assertListJSONMatchesCanonical(t, out.Bytes(), want)
}

func TestList_PersistedScopeJSONIncludesDurableJavaScriptSessionFields(t *testing.T) {
	var out bytes.Buffer
	err := NewList(testHTTPProtocol(t), canonicalListRequestPreparation)(ListConfig{Context: context.Background(),
		Scope:         "persisted",
		JSON:          true,
		Output:        &out,
		DurableLister: newContractDurableLister(t),
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	got := decodeListResponse(t, out.Bytes())
	if got.Scope == nil || *got.Scope != factoryapi.FactorySessionListScopePersisted {
		t.Fatalf("scope = %#v, want PERSISTED", got.Scope)
	}
	if len(got.Sessions) != 0 {
		t.Fatalf("sessions = %#v, want none for persisted scope", got.Sessions)
	}
	if got.DurableSessions == nil || len(*got.DurableSessions) == 0 {
		t.Fatal("durableSessions missing persisted rows")
	}

	row := durableRowBySessionID(t, got, "dur-sess-js-success-002")
	if row.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("status = %q, want SUCCEEDED", row.Status)
	}
	if row.OrchestratorKind != factoryapi.JAVASCRIPT {
		t.Fatalf("orchestratorKind = %q, want JAVASCRIPT", row.OrchestratorKind)
	}
	if row.ResolvedSource.SourceRef == nil || *row.ResolvedSource.SourceRef != "workflow/.claude/workflows/docs-refresh.yaml" {
		t.Fatalf("resolvedSource = %#v", row.ResolvedSource)
	}
	if row.EffectivePolicyHash == nil || *row.EffectivePolicyHash != "eff-policy-docs-refresh" {
		t.Fatalf("effectivePolicyHash = %#v", row.EffectivePolicyHash)
	}
	if row.Actions == nil {
		t.Fatal("actions missing from durable JavaScript list row")
	}
}

func TestList_AllScopeJSONMatchesCanonicalListResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListFactorySessionsResponse{
			Sessions: []factoryapi.FactorySessionSummary{sampleSessionSummary()},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	lister := newContractDurableLister(t)
	liveSessions := []fse.LiveSessionSummary{{
		ID:         "session-alpha",
		FactoryDir: "/workspace/fleet/alpha",
		FolderPath: "/workspace/fleet",
		Project:    "alpha",
	}}
	want := canonicalListResponse(t, lister, fse.SessionListScopeAll, liveSessions)

	var out bytes.Buffer
	err := NewList(testHTTPProtocol(t), canonicalListRequestPreparation)(ListConfig{Context: context.Background(),
		Scope:         "all",
		Port:          serverPort(t, srv),
		JSON:          true,
		Output:        &out,
		DurableLister: lister,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	assertListJSONMatchesCanonical(t, out.Bytes(), want)
	if !strings.Contains(out.String(), "dur-sess-js-success-002") {
		t.Fatalf("JSON missing durable JavaScript row: %s", out.String())
	}
	if !strings.Contains(out.String(), "session-alpha") {
		t.Fatalf("JSON missing live session row: %s", out.String())
	}
}

func TestList_PersistedScopeJSONOmitsSensitiveProviderInternals(t *testing.T) {
	var out bytes.Buffer
	err := NewList(testHTTPProtocol(t), canonicalListRequestPreparation)(ListConfig{Context: context.Background(),
		Scope:         "persisted",
		JSON:          true,
		Output:        &out,
		DurableLister: newContractDurableLister(t),
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	payload := out.String()
	for _, forbidden := range []string{
		"/Users/",
		"primaryResult",
		"artifactDetail",
		"Documentation refresh complete.",
		"approvalPreviewId",
		"\"javascript\"",
		"meta({",
		"workflowFile",
	} {
		if strings.Contains(payload, forbidden) {
			t.Fatalf("JSON leaked sensitive provider field %q:\n%s", forbidden, payload)
		}
	}
}

func TestList_PersistedScopeJSONEmptyState(t *testing.T) {
	emptyLister := func(context.Context, fse.ListSessionsRequest) (fse.ListSessionsResult, error) {
		return fse.ListSessionsResult{Scope: fse.SessionListScopePersisted}, nil
	}

	var out bytes.Buffer
	err := NewList(testHTTPProtocol(t), canonicalListRequestPreparation)(ListConfig{Context: context.Background(),
		Scope:         "persisted",
		JSON:          true,
		Output:        &out,
		DurableLister: emptyLister,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	got := decodeListResponse(t, out.Bytes())
	if got.Scope == nil || *got.Scope != factoryapi.FactorySessionListScopePersisted {
		t.Fatalf("scope = %#v, want PERSISTED", got.Scope)
	}
	if len(got.Sessions) != 0 {
		t.Fatalf("sessions = %#v, want empty array", got.Sessions)
	}
	if got.DurableSessions != nil && len(*got.DurableSessions) != 0 {
		t.Fatalf("durableSessions = %#v, want omitted or empty", got.DurableSessions)
	}
	if strings.Contains(out.String(), "Factory Sessions (durable):") {
		t.Fatalf("JSON included human table header: %q", out.String())
	}
}

func TestList_PersistedScopeJSONPreservesDeterministicOrdering(t *testing.T) {
	var out bytes.Buffer
	err := NewList(testHTTPProtocol(t), canonicalListRequestPreparation)(ListConfig{Context: context.Background(),
		Scope:         "persisted",
		JSON:          true,
		Output:        &out,
		DurableLister: newContractDurableLister(t),
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	got := decodeListResponse(t, out.Bytes())
	if got.DurableSessions == nil || len(*got.DurableSessions) < 2 {
		t.Fatalf("durableSessions = %#v, want at least two rows for ordering check", got.DurableSessions)
	}
	ids := make([]string, 0, len(*got.DurableSessions))
	for _, row := range *got.DurableSessions {
		ids = append(ids, row.SessionId)
	}
	for i := 1; i < len(ids); i++ {
		if ids[i-1] > ids[i] {
			t.Fatalf("durable session ids not sorted: %v", ids)
		}
	}
}

func TestList_JSONVerboseKeepsStdoutParseableAndDiagnosticsSeparate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(factoryapi.ListFactorySessionsResponse{
			Sessions: []factoryapi.FactorySessionSummary{sampleSessionSummary()},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	var out bytes.Buffer
	var diagnostics bytes.Buffer
	err := NewList(testHTTPProtocol(t), canonicalListRequestPreparation)(ListConfig{Context: context.Background(),
		Port:        serverPort(t, srv),
		JSON:        true,
		Verbose:     true,
		Output:      &out,
		Diagnostics: &diagnostics,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	var got factoryapi.ListFactorySessionsResponse
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("stdout is not parseable JSON: %v\n%s", err, out.String())
	}
	diag := diagnostics.String()
	for _, want := range []string{
		"session list request",
		"endpointPath=/factory-sessions",
		"session list response",
	} {
		if !strings.Contains(diag, want) {
			t.Fatalf("diagnostics missing %q:\n%s", want, diag)
		}
	}
}

func canonicalListResponse(
	t *testing.T,
	lister durableSessionLister,
	scope fse.SessionListScope,
	liveSessions []fse.LiveSessionSummary,
) factoryapi.ListFactorySessionsResponse {
	t.Helper()

	normalized := fse.ListSessionsRequest{Scope: scope}
	durableResult, err := lister(context.Background(), fse.ListSessionsRequest{Scope: fse.SessionListScopeAll})
	if err != nil {
		t.Fatalf("list durable sessions: %v", err)
	}
	detachedResult := fse.ListSessionsResult{
		Scope:           normalized.Scope,
		LiveSessions:    liveSessions,
		DurableSessions: durableResult.DurableSessions,
	}
	return factorysession.ListSessionsResponseToAPI(detachedResult)
}

func assertListJSONMatchesCanonical(t *testing.T, gotJSON []byte, want factoryapi.ListFactorySessionsResponse) {
	t.Helper()

	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal canonical list response: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(gotJSON), wantJSON) {
		var got factoryapi.ListFactorySessionsResponse
		if err := json.Unmarshal(gotJSON, &got); err != nil {
			t.Fatalf("decode list JSON: %v\n%s", err, string(gotJSON))
		}
		t.Fatalf("list JSON = %#v, want %#v", got, want)
	}
}

func decodeListResponse(t *testing.T, payload []byte) factoryapi.ListFactorySessionsResponse {
	t.Helper()

	var got factoryapi.ListFactorySessionsResponse
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("decode list JSON: %v\n%s", err, string(payload))
	}
	return got
}

func durableRowBySessionID(
	t *testing.T,
	response factoryapi.ListFactorySessionsResponse,
	sessionID string,
) factoryapi.FactorySessionDurableSummary {
	t.Helper()

	if response.DurableSessions == nil {
		t.Fatalf("durableSessions missing while searching for %q", sessionID)
	}
	for _, row := range *response.DurableSessions {
		if row.SessionId == sessionID {
			return row
		}
	}
	t.Fatalf("durable row %q not found in %#v", sessionID, response.DurableSessions)
	return factoryapi.FactorySessionDurableSummary{}
}

func sampleSessionSummary() factoryapi.FactorySessionSummary {
	return factoryapi.FactorySessionSummary{
		FactoryDir: "/workspace/fleet/alpha",
		FolderPath: "/workspace/fleet",
		Id:         "session-alpha",
		IsDefault:  false,
		Project:    "alpha",
		Target: factoryapi.FactorySessionTargetRef{
			Kind: factoryapi.FactorySessionTargetRefKindNamed,
			Name: stringPtr("alpha"),
		},
	}
}

func stringPtr(value string) *string {
	return &value
}

func serverPort(t *testing.T, srv *httptest.Server) int {
	t.Helper()

	var port int
	if _, err := fmt.Sscanf(srv.URL, "http://127.0.0.1:%d", &port); err != nil {
		t.Fatalf("parse test server port: %v", err)
	}
	return port
}
