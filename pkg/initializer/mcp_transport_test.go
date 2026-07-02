package initializer_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/cmd/factory/compose"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/mcp/factorysession"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil/factoryfixtures"
	"go.uber.org/zap"
)

func TestInitializeMCPTransport_RejectsMissingFactoryConfig(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cfg := &initializer.MCPConfig{
		Factory: &initializer.Config{Dir: t.TempDir()},
		Options: initializer.MCPOptions{
			FixtureCatalogPath: fixtureCatalogPath(t),
		},
	}

	transport, errInit := initializer.InitializeMCPTransport(ctx, cfg)
	_, errService := service.BuildFactoryService(ctx, cfg.Factory)

	if transport != nil {
		t.Fatal("expected InitializeMCPTransport to return nil transport without factory.json")
	}
	if errInit == nil {
		t.Fatal("expected InitializeMCPTransport to fail without factory.json")
	}
	if errService == nil {
		t.Fatal("expected BuildFactoryService to fail without factory.json")
	}
	if errService.Error() != errInit.Error() {
		t.Fatalf("InitializeMCPTransport error = %q, want %q", errInit, errService)
	}
}

func TestInitializeMCPTransport_ComposesSessionExecutionWithoutBuildFactoryService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	transport, err := initializer.InitializeMCPTransport(ctx, &initializer.MCPConfig{
		Options: initializer.MCPOptions{
			FixtureCatalogPath: fixtureCatalogPath(t),
		},
	})
	if err != nil {
		t.Fatalf("InitializeMCPTransport: %v", err)
	}
	if transport.SessionExecution == nil {
		t.Fatal("expected durable session execution service")
	}
	if _, ok := transport.SessionExecution.(*factorysessionexecution.FakeService); !ok {
		t.Fatalf("session execution type = %T, want *factorysessionexecution.FakeService", transport.SessionExecution)
	}
	if transport.SessionClient() == nil {
		t.Fatal("expected MCP session client")
	}
}

func TestInitializeMCPTransport_ComposesModelServiceWhenFactoryConfigured(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())

	ctx := context.Background()
	transport, err := initializer.InitializeMCPTransport(ctx, &initializer.MCPConfig{
		Factory: &initializer.Config{Dir: dir},
		Options: initializer.MCPOptions{
			FixtureCatalogPath: fixtureCatalogPath(t),
		},
	})
	if err != nil {
		t.Fatalf("InitializeMCPTransport: %v", err)
	}
	if transport.Services == nil || transport.Services.Models == nil {
		t.Fatal("expected initializer-produced model service")
	}

	models, err := transport.Services.Models.ListModels(ctx)
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if models.Results == nil {
		t.Fatal("expected model catalog payload")
	}
}

func TestInitializeMCPTransport_SessionOperationsMatchFixtureBehavior(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	transport, err := initializer.InitializeMCPTransport(ctx, &initializer.MCPConfig{
		Options: initializer.MCPOptions{
			FixtureCatalogPath: fixtureCatalogPath(t),
		},
	})
	if err != nil {
		t.Fatalf("InitializeMCPTransport: %v", err)
	}

	client := transport.SessionClient()
	request := factoryapi.FactorySessionExecutionRequest{
		RequestId: "req-js-run-n-001",
		Source: factoryapi.FactorySessionExecutionSource{
			Kind: factoryapi.FactorySessionExecutionSourceKindFactoryId,
			FactoryId: func() *string {
				id := "customer-support-triage"
				return &id
			}(),
		},
	}
	started, err := client.StartAsync(request)
	if err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if started.Error != nil || started.Result == nil {
		t.Fatalf("start = %#v, want success", started)
	}
	if started.Result.SessionId != "dur-sess-js-run-n-001" {
		t.Fatalf("sessionId = %q, want dur-sess-js-run-n-001", started.Result.SessionId)
	}
	if started.Result.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("status = %q, want RUNNING", started.Result.Status)
	}

	read, err := client.GetSession(mcpfactorysession.GetSessionInput{SessionID: started.Result.SessionId})
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if read.Error != nil || read.Result == nil {
		t.Fatalf("get = %#v, want running session read model", read)
	}
	if read.Result.Status != factoryapi.FactorySessionDurableLifecycleStatusRunning {
		t.Fatalf("read status = %q, want RUNNING", read.Result.Status)
	}
}

func TestInitializeMCPTransport_RuntimeBackedResolvesProjectRootFromCWD(t *testing.T) {
	projectRoot := t.TempDir()
	t.Chdir(projectRoot)

	transport, err := initializer.InitializeMCPTransport(context.Background(), &initializer.MCPConfig{
		Options: initializer.MCPOptions{
			RuntimeBacked: true,
		},
	})
	if err != nil {
		t.Fatalf("InitializeMCPTransport: %v", err)
	}
	if _, ok := transport.SessionExecution.(*factorysessionexecution.JavaScriptRuntimeService); !ok {
		t.Fatalf("session execution type = %T, want *factorysessionexecution.JavaScriptRuntimeService", transport.SessionExecution)
	}
}

func TestInitializeMCPTransport_RuntimeBackedSelectsJavaScriptRuntimeService(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	transport, err := initializer.InitializeMCPTransport(context.Background(), &initializer.MCPConfig{
		Options: initializer.MCPOptions{
			RuntimeBacked: true,
			ProjectRoot:   projectRoot,
		},
	})
	if err != nil {
		t.Fatalf("InitializeMCPTransport: %v", err)
	}
	if _, ok := transport.SessionExecution.(*factorysessionexecution.JavaScriptRuntimeService); !ok {
		t.Fatalf("session execution type = %T, want *factorysessionexecution.JavaScriptRuntimeService", transport.SessionExecution)
	}
}

func TestInitializeMCPTransport_NilConfigUsesFixtureDefaults(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	t.Chdir(repoRoot)

	transport, err := initializer.InitializeMCPTransport(context.Background(), nil)
	if err != nil {
		t.Fatalf("InitializeMCPTransport(nil): %v", err)
	}
	if transport.SessionExecution == nil {
		t.Fatal("expected durable session execution service")
	}
}

func TestInitializeMCPTransport_RuntimeBackedDiscoversFactoryConfig(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, projectRoot, factoryfixtures.MinimalFactoryConfig())

	transport, err := initializer.InitializeMCPTransport(context.Background(), &initializer.MCPConfig{
		Options: initializer.MCPOptions{
			RuntimeBacked: true,
			ProjectRoot:   projectRoot,
		},
	})
	if err != nil {
		t.Fatalf("InitializeMCPTransport: %v", err)
	}
	if transport.Services == nil || transport.Services.Models == nil {
		t.Fatal("expected initializer to compose model service from discovered factory.json")
	}
}

func TestInitializeMCPTransport_ResolvesFixtureCatalogFromRepoRoot(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	t.Chdir(repoRoot)

	transport, err := initializer.InitializeMCPTransport(context.Background(), &initializer.MCPConfig{})
	if err != nil {
		t.Fatalf("InitializeMCPTransport: %v", err)
	}
	if transport.SessionExecution == nil {
		t.Fatal("expected fixture-backed session execution service")
	}
}

func TestInitializeMCPTransport_ModelCatalogAndReadinessMatchInitializerModelService(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, mcpModelCatalogFactoryConfig(true))

	ctx := context.Background()
	transport, err := initializer.InitializeMCPTransport(ctx, &initializer.MCPConfig{
		Factory: &initializer.Config{
			Dir:               dir,
			MockWorkersConfig: factoryconfig.NewEmptyMockWorkersConfig(),
			Logger:            zap.NewNop(),
		},
		Options: initializer.MCPOptions{
			FixtureCatalogPath: fixtureCatalogPath(t),
		},
	})
	if err != nil {
		t.Fatalf("InitializeMCPTransport: %v", err)
	}
	if transport.Services == nil || transport.Services.Models == nil {
		t.Fatal("expected initializer-produced model service")
	}

	models, err := transport.Services.Models.ListModels(ctx)
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models.Results) != 1 || models.Results[0].Name != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("model catalog = %#v, want one OMNIVOICE_Q4_K_M entry", models.Results)
	}
	if models.Results[0].Status != factoryapi.ModelStatusREADY {
		t.Fatalf("catalog readiness = %s, want READY", models.Results[0].Status)
	}

	detail, err := transport.Services.Models.GetModel(ctx, "OMNIVOICE_Q4_K_M")
	if err != nil {
		t.Fatalf("GetModel: %v", err)
	}
	if detail.ManagedRuntime.ReadinessState != factoryapi.ManagedRuntimeReadinessStateMISSING {
		t.Fatalf("managed readiness = %s, want MISSING", detail.ManagedRuntime.ReadinessState)
	}
	if detail.ManagedRuntime.LifecycleState != factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED {
		t.Fatalf("managed lifecycle = %s, want NOT_INSTALLED", detail.ManagedRuntime.LifecycleState)
	}

	_, err = transport.Services.Models.GetModel(ctx, "missing-model")
	if !errors.Is(err, apisurface.ErrModelNotFound) {
		t.Fatalf("GetModel missing-model error = %v, want ErrModelNotFound", err)
	}
}

func TestInitializeMCPTransport_SessionNotFoundReturnsTypedEnvelope(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	transport, err := initializer.InitializeMCPTransport(ctx, &initializer.MCPConfig{
		Options: initializer.MCPOptions{
			FixtureCatalogPath: fixtureCatalogPath(t),
		},
	})
	if err != nil {
		t.Fatalf("InitializeMCPTransport: %v", err)
	}

	response, err := transport.SessionClient().GetSession(mcpfactorysession.GetSessionInput{
		SessionID: "dur-sess-missing-999",
	})
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if response.Result != nil {
		t.Fatalf("result = %#v, want not-found error envelope", response.Result)
	}
	if response.Error == nil {
		t.Fatal("error = nil, want not-found envelope")
	}
	if response.Error.Code != "factory_session.session.not_found" {
		t.Fatalf("error code = %q, want factory_session.session.not_found", response.Error.Code)
	}
	if response.Error.SessionID != "dur-sess-missing-999" {
		t.Fatalf("sessionId = %q, want dur-sess-missing-999", response.Error.SessionID)
	}
}

func TestInjectMCPTransport_MatchesInitializeMCPTransport(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cfg := &initializer.MCPConfig{
		Options: initializer.MCPOptions{
			FixtureCatalogPath: fixtureCatalogPath(t),
		},
	}

	direct, err := initializer.InitializeMCPTransport(ctx, cfg)
	if err != nil {
		t.Fatalf("InitializeMCPTransport: %v", err)
	}
	wired, err := compose.InjectMCPTransport(ctx, cfg)
	if err != nil {
		t.Fatalf("InjectMCPTransport: %v", err)
	}
	if direct.SessionExecution == nil || wired.SessionExecution == nil {
		t.Fatal("expected session execution services from both composition paths")
	}
	if _, ok := direct.SessionExecution.(*factorysessionexecution.FakeService); !ok {
		t.Fatalf("direct session execution type = %T, want *factorysessionexecution.FakeService", direct.SessionExecution)
	}
	if _, ok := wired.SessionExecution.(*factorysessionexecution.FakeService); !ok {
		t.Fatalf("wired session execution type = %T, want *factorysessionexecution.FakeService", wired.SessionExecution)
	}
}

func fixtureCatalogPath(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "api", "testdata", "durable-session-contract-fixtures.json")
}

func mcpModelCatalogFactoryConfig(includeResource bool) map[string]any {
	worker := map[string]any{
		"name":          "voice-local",
		"type":          interfaces.WorkerTypeModel,
		"modelProvider": "CODEX",
		"model":         "OMNIVOICE_Q4_K_M",
		"modelLocality": interfaces.ModelLocalityLocal,
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
	}
	cfg := map[string]any{
		"name":    "factory",
		"workers": []map[string]any{worker},
	}
	if includeResource {
		worker["resources"] = []map[string]any{{"name": "omnivoice-cache", "capacity": 1}}
		cfg["resources"] = []map[string]any{{
			"name":       "omnivoice-cache",
			"type":       interfaces.ResourceTypeModel,
			"capacity":   1,
			"model":      "OMNIVOICE_Q4_K_M",
			"backend":    "LLAMACPP",
			"loadPolicy": "ON_DEMAND",
		}}
	}
	return cfg
}
