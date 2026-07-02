package initializer_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil/factoryfixtures"
	"go.uber.org/zap"
)

func TestStartupCompatibility_RejectsInvalidManagedRuntimeDependency(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "factory.json"), []byte(`{
  "name": "bad-runtime",
  "workTypes": [{
    "name": "task",
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "complete", "type": "TERMINAL"},
      {"name": "failed", "type": "FAILED"}
    ]
  }],
  "resources": [{
    "name": "omnivoice-cache",
    "type": "MODEL",
    "capacity": 1,
    "model": "OMNIVOICE_Q4_K_M",
    "backend": "GGUF",
    "loadPolicy": "ON_DEMAND"
  }],
  "workers": [],
  "workstations": []
}`), 0o600); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}

	ctx := context.Background()
	cfg := &initializer.Config{Dir: dir, Logger: zap.NewNop()}
	assertInitializerStartupParity(t, ctx, cfg, func(ctx context.Context, cfg *initializer.Config) error {
		_, err := initializer.Initialize(ctx, cfg)
		return err
	})
}

func TestStartupCompatibility_RejectsInvalidOperatorDefaultsDuringRuntimeBuild(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())

	ctx := context.Background()
	cfg := &initializer.Config{
		Dir:    dir,
		Logger: zap.NewNop(),
		OperatorDefaults: operatorconfig.ResolvedDefaults{
			WorkerModelProvider: "NOT_A_REAL_PROVIDER",
		},
	}
	assertInitializerStartupParity(t, ctx, cfg, func(ctx context.Context, cfg *initializer.Config) error {
		_, err := initializer.Initialize(ctx, cfg)
		return err
	})
}

func TestStartupCompatibility_RejectsRuntimeHostConstructionFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	writeStartupCompatibilityWorkerAgentsMD(t, dir, "worker-a", `---
type: MODEL_WORKER
model: gpt-5.4
modelProvider: claude
openCodeAgent: reviewer
---
You are a helpful assistant.
`)
	writeStartupCompatibilityWorkstationAgentsMD(t, dir, "process")

	ctx := context.Background()
	cfg := &initializer.Config{
		Dir:                                     dir,
		MockWorkersConfig:                       config.NewEmptyMockWorkersConfig(),
		Logger:                                  zap.NewNop(),
		SkipBuiltInRunnerPrerequisiteValidation: true,
	}
	errInit := func() error {
		services, err := initializer.Initialize(ctx, cfg)
		if services != nil {
			t.Fatal("expected Initialize to return nil services for openCodeAgent runtime host construction failure")
		}
		return err
	}()
	errService := func() error {
		_, err := service.BuildFactoryService(ctx, cfg)
		return err
	}()
	if errInit == nil {
		t.Fatal("expected Initialize to fail for openCodeAgent runtime host construction")
	}
	if errService == nil {
		t.Fatal("expected BuildFactoryService to fail for openCodeAgent runtime host construction")
	}
	if errService.Error() != errInit.Error() {
		t.Fatalf("Initialize error = %q, want %q", errInit, errService)
	}
	if !strings.Contains(errInit.Error(), "openCodeAgent") {
		t.Fatalf("Initialize error = %q, want openCodeAgent runtime host construction context", errInit)
	}
}

func TestStartupCompatibility_AllTransportPathsRejectRecordAndReplayTogether(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cfg := &initializer.Config{
		Dir:        t.TempDir(),
		RecordPath: "recording.json",
		ReplayPath: "recording.json",
		Logger:     zap.NewNop(),
	}

	cases := []struct {
		name string
		run  func(context.Context, *initializer.Config) error
	}{
		{
			name: "Initialize",
			run: func(ctx context.Context, cfg *initializer.Config) error {
				_, err := initializer.Initialize(ctx, cfg)
				return err
			},
		},
		{
			name: "InitializeAPITransport",
			run: func(ctx context.Context, cfg *initializer.Config) error {
				_, err := initializer.InitializeAPITransport(ctx, cfg)
				return err
			},
		},
		{
			name: "InitializeCLITransport",
			run: func(ctx context.Context, cfg *initializer.Config) error {
				_, err := initializer.InitializeCLITransport(ctx, cfg)
				return err
			},
		},
		{
			name: "InitializeMCPTransport",
			run: func(ctx context.Context, cfg *initializer.Config) error {
				_, err := initializer.InitializeMCPTransport(ctx, &initializer.MCPConfig{Factory: cfg})
				return err
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertInitializerStartupParity(t, ctx, cfg, tc.run)
		})
	}
}

func assertInitializerStartupParity(
	t *testing.T,
	ctx context.Context,
	cfg *initializer.Config,
	init func(context.Context, *initializer.Config) error,
) {
	t.Helper()

	errInit := init(ctx, cfg)
	errService := func() error {
		_, err := service.BuildFactoryService(ctx, cfg)
		return err
	}()
	if errInit == nil {
		t.Fatal("expected initializer startup path to fail")
	}
	if errService == nil {
		t.Fatal("expected BuildFactoryService to fail")
	}
	if errService.Error() != errInit.Error() {
		t.Fatalf("initializer startup error = %q, want %q", errInit, errService)
	}
}

func writeStartupCompatibilityWorkerAgentsMD(t *testing.T, factoryDir, workerName, content string) {
	t.Helper()
	workerDir := filepath.Join(factoryDir, "workers", workerName)
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		t.Fatalf("create worker dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workerDir, "AGENTS.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write worker AGENTS.md: %v", err)
	}
}

func writeStartupCompatibilityWorkstationAgentsMD(t *testing.T, factoryDir, workstationName string) {
	t.Helper()
	workstationDir := filepath.Join(factoryDir, "workstations", workstationName)
	if err := os.MkdirAll(workstationDir, 0o755); err != nil {
		t.Fatalf("create workstation dir: %v", err)
	}
	content := "---\ntype: MODEL_WORKSTATION\n---\nDo the work.\n"
	if err := os.WriteFile(filepath.Join(workstationDir, "AGENTS.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}
}

// Consolidated startup evidence for service-break-06-006: all transport composition
// entrypoints succeed for a valid factory configuration without constructing
// root pkg/service. Focused verification command:
//
//	go test ./cmd/... ./pkg/api/... ./pkg/cli/... ./pkg/mcp/... ./pkg/initializer/... -short
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
