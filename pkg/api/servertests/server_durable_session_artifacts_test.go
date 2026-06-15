package apiserver_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestListFactorySessionArtifacts_RuntimeBackedReturnsTypedEmptyList(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
	started, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-api-runtime-artifact-list-001",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "simple-final",
		},
		Args: map[string]any{
			"subject": "api",
			"count":   2,
			"prefix":  "you",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	waitForRuntimeSessionTerminal(t, service, started.SessionID)

	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/factory-sessions/" + started.SessionID + "/artifacts")
	if err != nil {
		t.Fatalf("GET /factory-sessions/{session_id}/artifacts: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, readBody(t, resp))
	}

	var response factoryapi.ListFactorySessionArtifactsResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode artifact list response: %v", err)
	}
	if response.SessionId != started.SessionID {
		t.Fatalf("sessionId = %q, want %q", response.SessionId, started.SessionID)
	}
	if response.Artifacts == nil {
		t.Fatal("artifacts = nil, want empty typed list")
	}
	if len(response.Artifacts) != 0 {
		t.Fatalf("artifacts = %#v, want empty list for simple-final runtime session", response.Artifacts)
	}
}

func TestListFactorySessionArtifacts_RuntimeBackedReturnsTypedArtifactList(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "progress-primitives.workflow.js", "progress-primitives")
	service := factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
	completed, err := service.StartSync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-api-runtime-artifact-list-002",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "progress-primitives",
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/factory-sessions/" + completed.SessionID + "/artifacts")
	if err != nil {
		t.Fatalf("GET /factory-sessions/{session_id}/artifacts: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, readBody(t, resp))
	}

	var response factoryapi.ListFactorySessionArtifactsResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode artifact list response: %v", err)
	}
	if response.SessionId != completed.SessionID {
		t.Fatalf("sessionId = %q, want %q", response.SessionId, completed.SessionID)
	}
	if len(response.Artifacts) != 1 {
		t.Fatalf("artifacts = %#v, want one artifact", response.Artifacts)
	}
	artifact := response.Artifacts[0]
	if artifact.Id != "artifact-1" || string(artifact.Kind) != "log" {
		t.Fatalf("artifact = %#v, want artifact-1 log", artifact)
	}
	wantHref := "/factory-sessions/" + completed.SessionID + "/artifacts/artifact-1"
	if artifact.RetrievalRef == nil || artifact.RetrievalRef.Href != wantHref {
		t.Fatalf("retrievalRef = %#v, want %q", artifact.RetrievalRef, wantHref)
	}
}

func TestListFactorySessionArtifacts_RuntimeBackedMissingSessionReturnsNotFound(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/factory-sessions/dur-sess-missing-artifact-001/artifacts")
	if err != nil {
		t.Fatalf("GET /factory-sessions/{session_id}/artifacts: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", resp.StatusCode, readBody(t, resp))
	}
	var errResp factoryapi.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Code != factoryapi.NOTFOUND {
		t.Fatalf("code = %q, want NOT_FOUND", errResp.Code)
	}
}

func TestListFactorySessionArtifacts_LivePetriSessionReturnsNotFound(t *testing.T) {
	srv := newAPITestServer(&testutil.MockFactory{
		FactorySession: factoryapi.FactorySession{
			Id:         "session-beta",
			FactoryDir: "/workspace/root/beta",
			FolderPath: "/workspace/root",
			Project:    "beta",
		},
	})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/factory-sessions/session-beta/artifacts")
	if err != nil {
		t.Fatalf("GET /factory-sessions/session-beta/artifacts: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", resp.StatusCode, readBody(t, resp))
	}
}

func TestGetFactorySessionArtifact_RuntimeBackedReturnsTypedDetail(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "progress-primitives.workflow.js", "progress-primitives")
	service := factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
	completed, err := service.StartSync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-api-runtime-artifact-detail-001",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "progress-primitives",
		},
	})
	if err != nil {
		t.Fatalf("StartSync: %v", err)
	}

	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/factory-sessions/" + completed.SessionID + "/artifacts/artifact-1")
	if err != nil {
		t.Fatalf("GET /factory-sessions/{session_id}/artifacts/{artifact_id}: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.StatusCode, readBody(t, resp))
	}

	var response factoryapi.FactorySessionArtifactDetail
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode artifact detail response: %v", err)
	}
	if response.Id != "artifact-1" || string(response.Kind) != "log" {
		t.Fatalf("artifact = %#v, want artifact-1 log", response)
	}
	if response.SessionId != completed.SessionID {
		t.Fatalf("sessionId = %q, want %q", response.SessionId, completed.SessionID)
	}
	if response.Label == nil || *response.Label != "step-output" {
		t.Fatalf("label = %#v, want step-output", response.Label)
	}
	wantHref := "/factory-sessions/" + completed.SessionID + "/artifacts/artifact-1"
	if response.ContentRef == nil || response.ContentRef.Href != wantHref {
		t.Fatalf("contentRef = %#v, want %q", response.ContentRef, wantHref)
	}
}

func TestGetFactorySessionArtifact_RuntimeBackedUnknownArtifactReturnsNotFound(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
	started, err := service.StartAsync(context.Background(), factorysessionexecution.StartRequest{
		RequestID: "req-api-runtime-artifact-detail-missing-001",
		Source: factorysessionexecution.Source{
			Kind:         workflowsource.KindWorkflowName,
			WorkflowName: "simple-final",
		},
		Args: map[string]any{
			"subject": "api",
			"count":   2,
			"prefix":  "you",
		},
	})
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	waitForRuntimeSessionTerminal(t, service, started.SessionID)

	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/factory-sessions/" + started.SessionID + "/artifacts/artifact-missing-001")
	if err != nil {
		t.Fatalf("GET /factory-sessions/{session_id}/artifacts/{artifact_id}: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", resp.StatusCode, readBody(t, resp))
	}
	var errResp factoryapi.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Code != factoryapi.NOTFOUND {
		t.Fatalf("code = %q, want NOT_FOUND", errResp.Code)
	}
}

func TestGetFactorySessionArtifact_RuntimeBackedMissingSessionReturnsNotFound(t *testing.T) {
	projectRoot := setupAPIRuntimeWorkflowFixture(t, "simple-final.workflow.js", "simple-final")
	service := factorysessionexecution.NewJavaScriptRuntimeService(factorysessionexecution.JavaScriptRuntimeServiceConfig{
		ProjectRoot: projectRoot,
	})
	srv := newAPITestServer(&testutil.MockFactory{DurableExecutionService: service})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/factory-sessions/dur-sess-missing-artifact-detail-001/artifacts/artifact-1")
	if err != nil {
		t.Fatalf("GET /factory-sessions/{session_id}/artifacts/{artifact_id}: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", resp.StatusCode, readBody(t, resp))
	}
	var errResp factoryapi.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if errResp.Code != factoryapi.NOTFOUND {
		t.Fatalf("code = %q, want NOT_FOUND", errResp.Code)
	}
}

func TestGetFactorySessionArtifact_LivePetriSessionReturnsNotFound(t *testing.T) {
	srv := newAPITestServer(&testutil.MockFactory{
		FactorySession: factoryapi.FactorySession{
			Id:         "session-beta",
			FactoryDir: "/workspace/root/beta",
			FolderPath: "/workspace/root",
			Project:    "beta",
		},
	})
	server := httptest.NewServer(srv.Handler())
	defer server.Close()

	resp, err := http.Get(server.URL + "/factory-sessions/session-beta/artifacts/artifact-1")
	if err != nil {
		t.Fatalf("GET /factory-sessions/session-beta/artifacts/artifact-1: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", resp.StatusCode, readBody(t, resp))
	}
}
