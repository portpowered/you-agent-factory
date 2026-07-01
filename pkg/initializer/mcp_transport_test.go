package initializer_test

import (
	"context"
	"path/filepath"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/cmd/factory/compose"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/initializer"
	mcpfactorysession "github.com/portpowered/infinite-you/pkg/mcp/factorysession"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil/factoryfixtures"
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

	_, errInit := initializer.InitializeMCPTransport(ctx, cfg)
	_, errService := service.BuildFactoryService(ctx, cfg.Factory)

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
