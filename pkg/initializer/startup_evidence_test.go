package initializer_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/testutil/factoryfixtures"
)

// Consolidated startup evidence for service-break-06-006: all transport composition
// entrypoints succeed for a valid factory configuration without constructing
// root pkg/service. Focused verification command:
//   go test ./cmd/... ./pkg/api/... ./pkg/cli/... ./pkg/mcp/... ./pkg/initializer/... -short
func TestStartupEvidence_ValidFactoryConfigComposesAllTransportBundles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())

	ctx := context.Background()
	cfg := &initializer.Config{
		Dir:                                     dir,
		SkipBuiltInRunnerPrerequisiteValidation: true,
	}

	t.Run("Initialize", func(t *testing.T) {
		t.Parallel()

		services, err := initializer.Initialize(ctx, cfg)
		if err != nil {
			t.Fatalf("Initialize: %v", err)
		}
		assertInitializerServicesReady(t, services)
	})

	t.Run("InitializeAPITransport", func(t *testing.T) {
		t.Parallel()

		transport, err := initializer.InitializeAPITransport(ctx, cfg)
		if err != nil {
			t.Fatalf("InitializeAPITransport: %v", err)
		}
		if transport.Services == nil || transport.Host == nil {
			t.Fatal("expected API transport bundle")
		}
		assertInitializerServicesReady(t, transport.Services)
	})

	t.Run("InitializeCLITransport", func(t *testing.T) {
		t.Parallel()

		transport, err := initializer.InitializeCLITransport(ctx, cfg)
		if err != nil {
			t.Fatalf("InitializeCLITransport: %v", err)
		}
		if transport.Services == nil || transport.Host == nil {
			t.Fatal("expected CLI transport bundle")
		}
		if transport.Runner() == nil {
			t.Fatal("expected CLI runtime runner")
		}
		assertInitializerServicesReady(t, transport.Services)
	})

	t.Run("InitializeMCPTransport", func(t *testing.T) {
		t.Parallel()

		transport, err := initializer.InitializeMCPTransport(ctx, &initializer.MCPConfig{
			Factory: cfg,
			Options: initializer.MCPOptions{
				FixtureCatalogPath: fixtureCatalogPath(t),
			},
		})
		if err != nil {
			t.Fatalf("InitializeMCPTransport: %v", err)
		}
		if transport.SessionExecution == nil || transport.SessionClient() == nil {
			t.Fatal("expected MCP transport session capabilities")
		}
		if transport.Services == nil || transport.Services.Models == nil {
			t.Fatal("expected MCP transport model service when factory configured")
		}
		assertInitializerServicesReady(t, transport.Services)
	})
}

func assertInitializerServicesReady(t *testing.T, services *initializer.Services) {
	t.Helper()

	if services == nil {
		t.Fatal("expected services")
	}
	if services.Sessions == nil || services.FactoryDefinition == nil || services.Models == nil || services.RuntimeHost == nil {
		t.Fatal("expected session, factory-definition, model, and runtime host services")
	}
	if services.Workers.Logger == nil {
		t.Fatal("expected workers service with logger")
	}
}
