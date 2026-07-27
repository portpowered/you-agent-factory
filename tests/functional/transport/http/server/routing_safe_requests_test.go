package server_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/contractinventory"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	routingReachabilityRequestTimeout = 15 * time.Second
	routingReachabilityModelName      = "OMNIVOICE_Q4_K_M"
	routingReachabilityWorkstation    = "process"
)

const routingLiveJavaScriptWorkflowSource = `phase("plan");
workflow.checkpoint({ label: "routing-reachability-plan", state: { ready: true } });
phase("execute");
return "routing-reachability-live-result";`

const routingArtifactWorkflowSource = `return (async function () {
  const artifactRef = workflow.artifact({
    kind: "log",
    label: "routing-reachability-artifact",
    content: { message: "routing reachability" },
  });
  const child = await agent.run({
    prompt: "routing reachability child",
    label: "routing-reachability-child",
  });
  return { artifactRef, child };
})();`

type routingReachabilityContext struct {
	t                       *testing.T
	server                  *support.FunctionalAPIServer
	baseURL                 string
	factoryDir              string
	liveJavaScriptFactoryDir string
	jsLive                  string
	durable                 routingDurableSessionContext
	opened                  string
	workID                  string
}

type routingDurableSessionContext struct {
	sessionID  string
	dispatchID string
	artifactID string
}

func (ctx *routingReachabilityContext) prepareSessions() {
	ctx.t.Helper()

	ctx.durable = startRoutingArtifactDurableSession(ctx.t, ctx.baseURL)
	ctx.opened = openRoutingFactorySession(ctx.t, ctx.baseURL, ctx.factoryDir)
	ctx.jsLive = openRoutingLiveJavaScriptFactorySession(ctx.t, ctx.baseURL, ctx.liveJavaScriptFactoryDir)
}

func (ctx *routingReachabilityContext) prepareWork() {
	ctx.t.Helper()

	submitted := support.SubmitDefaultSessionWork(ctx.t, ctx.baseURL, factoryapi.SubmitWorkRequest{
		Name:         stringPtr("routing-reachability-work"),
		WorkTypeName: "task",
		Payload:      map[string]string{"title": "routing reachability"},
	})
	workID := ""
	if submitted.WorkId != nil {
		workID = strings.TrimSpace(*submitted.WorkId)
	}
	if workID == "" {
		ctx.t.Fatalf("submit work returned empty workId: %#v", submitted)
	}
	ctx.workID = workID
}

func (ctx *routingReachabilityContext) safeRequest(operation contractinventory.Operation) (*http.Request, error) {
	path, err := ctx.resolveOperationPath(operation)
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimSuffix(ctx.baseURL, "/") + path

	switch operation.OperationID {
	case "previewFactory":
		return newJSONRequest(http.MethodPost, endpoint, map[string]any{})
	case "validateFactory":
		return newJSONRequest(http.MethodPost, endpoint, map[string]any{})
	case "openFactorySession":
		return newJSONRequest(http.MethodPost, endpoint, factoryapi.OpenFactorySessionRequest{
			FolderPath: ctx.factoryDir,
		})
	case "startDurableFactorySessionAsync", "startDurableFactorySessionSync":
		return newJSONRequest(http.MethodPost, endpoint, map[string]any{})
	case "closeFactorySession":
		return http.NewRequest(http.MethodDelete, sessionEndpoint(ctx.baseURL, ctx.opened), nil)
	case "submitWorkBySessionId":
		return newJSONRequest(http.MethodPost, sessionEndpoint(ctx.baseURL, factorysessions.DefaultSessionID)+"/work", factoryapi.SubmitWorkRequest{
			Name:         stringPtr("routing-reachability-submit"),
			WorkTypeName: "task",
			Payload:      map[string]string{"title": "routing reachability submit"},
		})
	case "upsertWorkRequestBySessionId":
		return newJSONRequest(
			http.MethodPut,
			sessionEndpoint(ctx.baseURL, factorysessions.DefaultSessionID)+"/work-requests/routing-reachability",
			map[string]any{},
		)
	case "stageSubmitWorkFileBySessionId":
		return newJSONRequest(http.MethodPost, sessionEndpoint(ctx.baseURL, factorysessions.DefaultSessionID)+"/work/staged-files", map[string]any{})
	case "moveWorkBySessionId":
		return newJSONRequest(
			http.MethodPost,
			sessionEndpoint(ctx.baseURL, factorysessions.DefaultSessionID)+"/work/"+url.PathEscape(ctx.workID)+"/move",
			map[string]any{},
		)
	case "saveCurrentFactoryBySessionId":
		return newJSONRequest(http.MethodPut, sessionEndpoint(ctx.baseURL, factorysessions.DefaultSessionID)+"/factory", map[string]any{})
	case "invokeFactorySessionBySessionId":
		return newJSONRequest(http.MethodPost, sessionEndpoint(ctx.baseURL, factorysessions.DefaultSessionID)+"/invocations", map[string]any{})
	case "pauseFactorySession", "resumeFactorySession", "terminateFactorySession", "cancelFactorySession", "approveFactorySession",
		"interruptFactorySessionDispatch", "retryFactorySessionDispatch":
		endpoint, err := sessionScopedEndpoint(ctx, operation)
		if err != nil {
			return nil, err
		}
		return newJSONRequest(http.MethodPost, endpoint, map[string]any{})
	case "validateCurrentFactoryWorkstationPromptTemplateBySessionId":
		return newJSONRequest(
			http.MethodPost,
			sessionEndpoint(ctx.baseURL, factorysessions.DefaultSessionID)+"/factory/workstations/"+routingReachabilityWorkstation+"/prompt-template-validation",
			map[string]any{},
		)
	case "invokeModel":
		return newJSONRequest(
			http.MethodPost,
			strings.TrimSuffix(ctx.baseURL, "/")+"/models/"+routingReachabilityModelName+"/invocations",
			factoryapi.ModelInvocationRequest{Operation: "EMBED"},
		)
	case "pullModel":
		return newJSONRequest(
			http.MethodPost,
			strings.TrimSuffix(ctx.baseURL, "/")+"/models/"+routingReachabilityModelName+"/pull",
			nil,
		)
	case "getProviderSessionDetails":
		return http.NewRequest(
			http.MethodGet,
			strings.TrimSuffix(ctx.baseURL, "/")+"/provider-sessions/detail?provider=openai&kind=session_id&id=routing-reachability",
			nil,
		)
	case "getEventsBySessionId", "getFactoryResponseEventsBySessionId":
		request, err := http.NewRequest(http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		request.Header.Set("Accept", "text/event-stream")
		return request, nil
	default:
		request, err := http.NewRequest(operation.Method, endpoint, nil)
		if err != nil {
			return nil, err
		}
		if len(operation.RequestMediaTypes) > 0 {
			request.Header.Set("Content-Type", operation.RequestMediaTypes[0])
		}
		return request, nil
	}
}

func (ctx *routingReachabilityContext) resolveOperationPath(operation contractinventory.Operation) (string, error) {
	replacements := map[string]string{
		"{session_id}":       ctx.sessionIDFor(operation),
		"{model_name}":       routingReachabilityModelName,
		"{workstation_name}": routingReachabilityWorkstation,
		"{request_id}":       "routing-reachability",
		"{id}":               ctx.workID,
		"{dispatch_id}":      ctx.durable.dispatchID,
		"{artifact_id}":      ctx.durable.artifactID,
	}
	path := operation.Path
	for placeholder, value := range replacements {
		if !strings.Contains(path, placeholder) {
			continue
		}
		if strings.TrimSpace(value) == "" {
			return "", fmt.Errorf("missing path value for %s in %s", placeholder, operation.OperationID)
		}
		path = strings.ReplaceAll(path, placeholder, value)
	}
	if strings.Contains(path, "{") {
		return "", fmt.Errorf("unresolved path placeholders remain in %q", path)
	}
	return path, nil
}

func (ctx *routingReachabilityContext) sessionIDFor(operation contractinventory.Operation) string {
	switch operation.OperationID {
	case "closeFactorySession", "terminateFactorySession", "cancelFactorySession":
		if ctx.opened != "" {
			return ctx.opened
		}
	case "getFactorySessionResult", "getFactorySessionPartialResult":
		if ctx.jsLive != "" {
			return ctx.jsLive
		}
	case "getFactorySessionResults":
		if ctx.durable.sessionID != "" {
			return ctx.durable.sessionID
		}
	case "listFactorySessionArtifacts", "getFactorySessionArtifact",
		"listFactorySessionDispatches", "getFactorySessionDispatch",
		"approveFactorySession", "interruptFactorySessionDispatch", "retryFactorySessionDispatch":
		if ctx.durable.sessionID != "" {
			return ctx.durable.sessionID
		}
	}
	return factorysessions.DefaultSessionID
}

func sessionScopedEndpoint(ctx *routingReachabilityContext, operation contractinventory.Operation) (string, error) {
	path, err := ctx.resolveOperationPath(operation)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(ctx.baseURL, "/") + path, nil
}

func sessionEndpoint(baseURL, sessionID string) string {
	return strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
}

func newJSONRequest(method, endpoint string, body any) (*http.Request, error) {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		reader = bytes.NewReader(payload)
	}
	request, err := http.NewRequest(method, endpoint, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	return request, nil
}

func startRoutingArtifactDurableSession(t *testing.T, baseURL string) routingDurableSessionContext {
	t.Helper()

	dialect := "you-workflow-v1"
	response := postRoutingJSON[factoryapi.FactorySessionSyncExecutionResponse](
		t,
		baseURL+"/factory-sessions/sync",
		factoryapi.FactorySessionExecutionRequest{
			RequestId: "routing-reachability-durable",
			Source: factoryapi.FactorySessionExecutionSource{
				Kind: factoryapi.FactorySessionExecutionSourceKindInlineWorkflow,
				InlineWorkflow: &factoryapi.FactorySessionExecutionInlineWorkflow{
					Dialect: &dialect,
					InlineSource: factoryapi.FactoryOrchestratorJavaScriptInlineSource{
						Encoding: factoryapi.FactoryOrchestratorJavaScriptInlineSourceEncodingUtf8,
						Inline:   routingArtifactWorkflowSource,
					},
				},
			},
		},
		"start routing durable session",
	)
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Fatalf("durable session status = %q, want SUCCEEDED", response.Status)
	}
	if strings.TrimSpace(response.SessionId) == "" {
		t.Fatal("durable session id is empty")
	}

	dispatches := support.GetJSON[factoryapi.ListFactorySessionDispatchesResponse](
		t,
		sessionEndpoint(baseURL, response.SessionId)+"/dispatches",
	)
	if len(dispatches.Dispatches) == 0 {
		t.Fatalf("durable session dispatches = %#v, want at least one dispatch", dispatches)
	}

	artifacts := support.GetJSON[factoryapi.ListFactorySessionArtifactsResponse](
		t,
		sessionEndpoint(baseURL, response.SessionId)+"/artifacts",
	)
	if len(artifacts.Artifacts) == 0 {
		t.Fatalf("durable session artifacts = %#v, want at least one artifact", artifacts)
	}

	return routingDurableSessionContext{
		sessionID:  response.SessionId,
		dispatchID: dispatches.Dispatches[0].Id,
		artifactID: artifacts.Artifacts[0].Id,
	}
}

func openRoutingLiveJavaScriptFactorySession(t *testing.T, baseURL, factoryDir string) string {
	t.Helper()

	response := postRoutingJSON[factoryapi.OpenFactorySessionResponse](
		t,
		baseURL+"/factory-sessions",
		factoryapi.OpenFactorySessionRequest{FolderPath: factoryDir},
		"open routing live JavaScript factory session",
	)
	if response.Session == nil || strings.TrimSpace(response.Session.Id) == "" {
		t.Fatalf("open live JavaScript factory session response = %#v, want session id", response)
	}
	return response.Session.Id
}

func openRoutingFactorySession(t *testing.T, baseURL, factoryDir string) string {
	t.Helper()

	response := postRoutingJSON[factoryapi.OpenFactorySessionResponse](
		t,
		baseURL+"/factory-sessions",
		factoryapi.OpenFactorySessionRequest{FolderPath: factoryDir},
		"open routing factory session",
	)
	if response.Session == nil || strings.TrimSpace(response.Session.Id) == "" {
		t.Fatalf("open factory session response = %#v, want session id", response)
	}
	return response.Session.Id
}

func postRoutingJSON[T any](t *testing.T, endpoint string, body any, label string) T {
	t.Helper()

	request, err := newJSONRequest(http.MethodPost, endpoint, body)
	if err != nil {
		t.Fatalf("%s: build request: %v", label, err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("%s: %v", label, err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("%s: read body: %v", label, err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		t.Fatalf("%s status = %d, want success: %s", label, response.StatusCode, strings.TrimSpace(string(payload)))
	}
	var decoded T
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("%s: decode response: %v\n%s", label, err, string(payload))
	}
	return decoded
}

func scaffoldRoutingLiveJavaScriptFactory(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "routing-reachability-live-javascript",
		"orchestrator": map[string]any{
			"kind": "JAVASCRIPT",
			"javascript": map[string]any{
				"sourceRef": "workflow.js",
			},
		},
	})
	if err := os.WriteFile(
		filepath.Join(dir, "workflow.js"),
		[]byte(routingLiveJavaScriptWorkflowSource),
		0o600,
	); err != nil {
		t.Fatalf("write live JavaScript routing workflow: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(dir, "mock-workers.json"),
		[]byte(`{"mockWorkers":[]}`),
		0o600,
	); err != nil {
		t.Fatalf("write live JavaScript mock-workers config: %v", err)
	}
	return dir
}

func scaffoldRoutingReachabilityFactory(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{
		"name": "routing-reachability",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":      routingReachabilityWorkstation,
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	})
	if err := os.WriteFile(
		filepath.Join(dir, "workflow.js"),
		[]byte(`return { ok: true };`),
		0o600,
	); err != nil {
		t.Fatalf("write routing javascript workflow: %v", err)
	}
	installRoutingReachabilityModelWorker(t, dir)
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	return dir
}

func installRoutingReachabilityModelWorker(t *testing.T, dir string) {
	t.Helper()

	factoryPath := filepath.Join(dir, "factory.json")
	raw, err := os.ReadFile(factoryPath)
	if err != nil {
		t.Fatalf("read factory.json: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("decode factory.json: %v", err)
	}
	cfg["workers"] = []map[string]any{
		{"name": "worker-a"},
		{
			"name":          "tts-worker",
			"type":          interfaces.WorkerTypeModel,
			"model":         routingReachabilityModelName,
			"modelProvider": "CODEX",
			"modelLocality": interfaces.ModelLocalityCloud,
			"operations": []map[string]any{{
				"name": "TTS",
				"inputs": []map[string]any{{
					"name":         "text",
					"contentTypes": []string{interfaces.ModelOperationContentTypeText},
					"required":     true,
				}},
				"outputs": []map[string]any{{
					"name":         "audio",
					"contentTypes": []string{interfaces.ModelOperationContentTypeAudio},
				}},
			}},
		},
	}
	encoded, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("encode factory.json: %v", err)
	}
	if err := os.WriteFile(factoryPath, encoded, 0o600); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
	support.WriteAgentConfig(t, dir, "tts-worker", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, routingReachabilityModelName))
}

func stringPtr(value string) *string {
	return &value
}

func routingPackageDirectory() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	return filepath.Dir(filename)
}

func loadRESTOperationInventoryFromFile(path string) (*contractinventory.Inventory, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read openapi file: %w", err)
	}
	return contractinventory.ExtractFromOpenAPIYAML(data)
}
