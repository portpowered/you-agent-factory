package inference_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/pkg/root"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Session invocation is the public boundary here: direct Models commands bind
// ~default and cannot express concurrent Factory Session isolation.
func TestModelsFixedLeavesKeepExplicitSessionResultsAndRecoveryIsolated(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	host := story004HostServer(t)
	body := []byte("fixed-leaf-embedding-backend")
	selection := story004EmbedBackendSelection(body)
	writeGenericBuiltinModelCache(t, home, story004EmbedSource)
	writeGenericBackendCache(t, home, "localai-llamacpp", selection, body)
	routes := newFixedLeafRoutes()
	network := &rejectingModelAssetHTTP{}
	launcher := &recordingModelHostLauncher{endpoint: host.URL}
	edges := story004EmbedEdges(home, network, host.Client(),
		launcher, &joinedProtocolNegotiator{},
		&joinedCompatibilityChecker{}, selection, nil)
	edges.ModelEmbeddingBackend = nil
	commands := support.NewRecordingCommandRunner("unexpected model command")
	edges.ModelRuntimeCommandRunner = commands
	var invocations atomic.Int64
	edges.ModelInvocationBackend = func(ctx context.Context, request models.InvokeModelRequest) ([]models.InferenceContent, []models.InferenceArtifact, error) {
		invocations.Add(1)
		return routes.invoke(ctx, request)
	}
	server := functionalStartAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: functionalScaffoldFactory(t, builtInOnlyModelFactoryConfig()),
		Env:        functionalHomeEnvironment(home), Edges: edges,
		BeforeStart: func(t testing.TB, process support.Process, input root.Input) {
			// BuildProcess has returned, but no customer operation has activated it.
			if launcher.Calls() != 0 || network.Calls() != 0 || commands.CallCount() != 0 || invocations.Load() != 0 {
				t.Fatalf("construction activated model effects: hosts=%d assets=%d commands=%d invocations=%d",
					launcher.Calls(), network.Calls(), commands.CallCount(), invocations.Load())
			}
			bootstrapFixedLeafProfile(t, process, input)
		},
	})
	for _, name := range []string{"selected", "peer"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			runFixedLeafSessionRecovery(t, server.URL(), routes, name)
		})
	}
	t.Run("cancel while peer is accepted", func(t *testing.T) {
		t.Parallel()
		runFixedLeafSessionCancellation(t, server.URL(), routes)
	})
	t.Run("fault and retry while peer is accepted", func(t *testing.T) {
		t.Parallel()
		runFixedLeafSessionFaultWithHeldPeer(t, server.URL(), routes)
	})
	t.Run("capacity refusal and recovery while peer is accepted", func(t *testing.T) {
		t.Parallel()
		runFixedLeafSessionCapacityWithHeldPeer(t, server.URL(), routes, launcher)
	})
}

// Complete mutable profile initialization through this same public process
// before the transport's readiness budget and parallel child sessions begin.
func bootstrapFixedLeafProfile(t testing.TB, process support.Process, input root.Input) {
	t.Helper()
	bootstrap := support.FakeInputs(t.Context(), []string{"you", "--json", "models", "list"})
	bootstrap.Input.Env = input.Env
	bootstrap.Input.WorkingDirectory = input.WorkingDirectory
	if err := process.Execute(bootstrap.Input); err != nil {
		t.Fatalf("initialize fixed-leaf profile: %v", err)
	}
}

func fixedLeafFactoryConfig() map[string]any {
	config := builtInOnlyModelFactoryConfig()
	config["name"] = "fixed-leaf-embedding"
	config["workTypes"].([]map[string]any)[0]["handlingBehavior"] = []string{"DEFAULT"}
	config["resources"] = []map[string]any{{"name": "embed-cache", "type": "MODEL",
		"capacity": 1, "model": "embed", "backend": "localai-llamacpp", "loadPolicy": "ON_DEMAND"}}
	config["invocationSignature"] = map[string]any{
		"parameters": []map[string]any{{"name": "text", "required": true,
			"bindings": []map[string]any{{"kind": "POSITIONAL", "position": 1}, {"kind": "STDIN"}}}},
	}
	config["workers"] = []map[string]any{{
		"name": "embed-worker", "type": "MODEL_WORKER", "model": "embed",
		"modelLocality": "LOCAL", "modelProvider": "CODEX",
		"resources": []map[string]any{{"name": "embed-cache", "capacity": 1}},
		"operations": []map[string]any{{"name": "EMBED",
			"inputs":  []map[string]any{{"name": "text", "required": true, "contentTypes": []string{"TEXT"}}},
			"outputs": []map[string]any{{"name": "embedding", "contentTypes": []string{"JSON"}}},
		}},
	}}
	config["workstations"] = []map[string]any{{
		"name": "embed", "type": "MODEL_INVOKE", "worker": "embed-worker", "operation": "EMBED",
		"operationBindings": []map[string]any{{"slot": "text", "selector": map[string]any{"type": "TEXT"}}},
		"inputs":            []map[string]string{{"workType": "task", "state": "init"}},
		"outputs":           []map[string]string{{"workType": "task", "state": "complete"}},
		"onFailure":         []map[string]string{{"workType": "task", "state": "failed"}},
	}}
	return config
}

func runFixedLeafSessionRecovery(t *testing.T, baseURL string, routes *fixedLeafRoutes, name string) {
	t.Helper()
	session := openFixedLeafSession(t, baseURL)
	for index, failure := range []bool{false, true, false} {
		text := fmt.Sprintf("%s-%d", name, index)
		route := routes.register(text, failure)
		requestID := "fixed-leaf-" + text
		response := invokeFixedLeafSession(t, baseURL, session, requestID, text)
		encodedResponse, _ := json.Marshal(response)
		t.Logf("session=%s invocation=%s", session, encodedResponse)
		select {
		case request := <-route.observed:
			if request.Scope.IsZero() || request.Operation != "EMBED" || request.ModelName != "embed" {
				t.Fatalf("selected model effect = %#v", request)
			}
			routes.recordScope(t, name, request.Scope.String())
		default:
			for _, event := range support.GetFactoryEventsForSessionAt(t, baseURL, session) {
				if event.Type == "MODEL_RESPONSE" || event.Type == "DISPATCH_RESPONSE" {
					encoded, _ := json.Marshal(event)
					t.Logf("terminal event=%s", encoded)
				}
			}
			t.Fatalf("session invocation did not reach the selected model effect: %#v", response)
		}
		if response.RequestId != requestID || (response.SessionId != nil && *response.SessionId != session) {
			t.Fatalf("session response identity = %#v, want %s/%s", response, session, requestID)
		}
		if failure {
			if response.Status != factoryapi.InvocationTerminalStatusFailed || response.PrimaryResult != nil {
				t.Fatalf("failed model response = %#v, want FAILED without success result", response)
			}
		} else {
			assertFixedLeafSuccess(t, response, route.output)
		}
		assertFixedLeafModelEvent(t, baseURL, session, response.RequestId, failure)
	}
}

func invokeFixedLeafSession(t *testing.T, baseURL, session, requestID, text string) factoryapi.InvocationResponse {
	t.Helper()
	source := factoryapi.InvocationInputSourceKindText
	var part factoryapi.WorkContentPart
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{Type: factoryapi.WorkContentPartTypeText, Text: text}); err != nil {
		t.Fatal(err)
	}
	content := []factoryapi.WorkContentPart{part}
	return postFunctionalJSON[factoryapi.InvocationResponse](t,
		baseURL+"/factory-sessions/"+url.PathEscape(session)+"/invocations",
		factoryapi.InvocationRequest{RequestId: &requestID, SourceKind: &source, Content: &content},
		"explicit-session model invocation")
}

type fixedLeafRoute struct {
	failure  bool
	output   string
	observed chan models.InvokeModelRequest
	release  chan struct{}
	canceled chan struct{}
}

type fixedLeafRoutes struct {
	mu     sync.Mutex
	routes map[string]*fixedLeafRoute
	scopes map[string]string
}

func newFixedLeafRoutes() *fixedLeafRoutes {
	return &fixedLeafRoutes{routes: map[string]*fixedLeafRoute{}, scopes: map[string]string{}}
}

func (routes *fixedLeafRoutes) register(text string, failure bool) *fixedLeafRoute {
	routes.mu.Lock()
	defer routes.mu.Unlock()
	route := &fixedLeafRoute{failure: failure, output: fmt.Sprintf("%d", len(routes.routes)+101), observed: make(chan models.InvokeModelRequest, 1)}
	routes.routes[text] = route
	return route
}

func (routes *fixedLeafRoutes) invoke(ctx context.Context, request models.InvokeModelRequest) ([]models.InferenceContent, []models.InferenceArtifact, error) {
	if len(request.Inputs) != 1 {
		return nil, nil, errors.New("expected one selected model input")
	}
	routes.mu.Lock()
	route := routes.routes[request.Inputs[0].Content]
	routes.mu.Unlock()
	if route == nil {
		return nil, nil, errors.New("unexpected model route")
	}
	request.Inputs = append([]models.InferenceInput(nil), request.Inputs...)
	select {
	case route.observed <- request:
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}
	if route.release != nil {
		select {
		case <-route.release:
		case <-ctx.Done():
			close(route.canceled)
			return nil, nil, ctx.Err()
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if route.failure {
		return nil, nil, models.ErrInferenceTimeout
	}
	return []models.InferenceContent{{Name: "embedding", Modality: models.ModalityJSON,
		ContentType: "application/json", MediaType: "application/json", Content: "[" + route.output + "]"}}, nil, nil
}

func (routes *fixedLeafRoutes) recordScope(t *testing.T, name, scope string) {
	t.Helper()
	routes.mu.Lock()
	defer routes.mu.Unlock()
	if previous := routes.scopes[name]; previous != "" && previous != scope {
		t.Fatalf("session scope changed across repeat: %s -> %s", previous, scope)
	}
	for peer, peerScope := range routes.scopes {
		if peer != name && peerScope == scope {
			t.Fatalf("peer sessions shared model scope %s", scope)
		}
	}
	routes.scopes[name] = scope
}
