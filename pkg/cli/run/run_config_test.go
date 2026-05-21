package run

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestRun_BootstrapErrorSkipsServiceStart(t *testing.T) {
	originalBuilder := buildFactoryService
	originalBootstrap := bootstrapFactory
	defer func() {
		buildFactoryService = originalBuilder
		bootstrapFactory = originalBootstrap
	}()

	bootstrapFactory = func(_ string) error {
		return errors.New("bootstrap failed")
	}

	builderCalled := false
	buildFactoryService = func(_ context.Context, _ *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		builderCalled = true
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	err := Run(context.Background(), RunConfig{Bootstrap: true})
	if err == nil {
		t.Fatal("expected bootstrap failure")
	}
	if !strings.Contains(err.Error(), "bootstrap failed") {
		t.Fatalf("error = %q, want bootstrap failure", err.Error())
	}
	if builderCalled {
		t.Fatal("factory service builder should not run when bootstrap fails")
	}
}

func TestRun_VerbosePassedToServiceConfig(t *testing.T) {
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	var capturedVerbose bool
	buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		capturedVerbose = cfg.Verbose
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	if err := Run(context.Background(), RunConfig{Verbose: true}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !capturedVerbose {
		t.Fatal("verbose = false, want true")
	}
}

func TestRun_DefaultsExecutionBaseDirToCurrentWorkingDirectory(t *testing.T) {
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	workingDirectory := t.TempDir()
	originalWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(workingDirectory); err != nil {
		t.Fatalf("Chdir(%q): %v", workingDirectory, err)
	}
	defer func() {
		if chdirErr := os.Chdir(originalWorkingDirectory); chdirErr != nil {
			t.Fatalf("restore working directory: %v", chdirErr)
		}
	}()

	var capturedBaseDir string
	buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		capturedBaseDir = cfg.ExecutionBaseDir
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	if err := Run(context.Background(), RunConfig{Dir: "factory"}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if testutil.CanonicalPath(capturedBaseDir) != testutil.CanonicalPath(workingDirectory) {
		t.Fatalf("execution base dir = %q, want %q", capturedBaseDir, workingDirectory)
	}
}

func TestRun_ExplicitExecutionBaseDirOverridesCurrentWorkingDirectory(t *testing.T) {
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	workingDirectory := t.TempDir()
	overrideDir := t.TempDir()
	originalWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(workingDirectory); err != nil {
		t.Fatalf("Chdir(%q): %v", workingDirectory, err)
	}
	defer func() {
		if chdirErr := os.Chdir(originalWorkingDirectory); chdirErr != nil {
			t.Fatalf("restore working directory: %v", chdirErr)
		}
	}()

	var capturedBaseDir string
	buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		capturedBaseDir = cfg.ExecutionBaseDir
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	if err := Run(context.Background(), RunConfig{Dir: "factory", ExecutionBaseDir: overrideDir}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if testutil.CanonicalPath(capturedBaseDir) != testutil.CanonicalPath(overrideDir) {
		t.Fatalf("execution base dir = %q, want %q", capturedBaseDir, overrideDir)
	}
}

func TestRun_RuntimeLogConfigPassedToServiceConfig(t *testing.T) {
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	var capturedConfig *service.FactoryServiceConfig
	buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		capturedConfig = cfg
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	runtimeLogConfig := logging.RuntimeLogConfig{
		MaxSize:    12,
		MaxBackups: 6,
		MaxAge:     21,
		Compress:   true,
	}
	err := Run(context.Background(), RunConfig{
		RuntimeLogDir:    "runtime-logs",
		RuntimeLogConfig: runtimeLogConfig,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if capturedConfig == nil {
		t.Fatal("expected factory service to be built")
	}
	if capturedConfig.RuntimeLogDir != "runtime-logs" {
		t.Fatalf("runtime log dir = %q, want runtime-logs", capturedConfig.RuntimeLogDir)
	}
	if capturedConfig.RuntimeLogConfig != runtimeLogConfig {
		t.Fatalf("runtime log config = %#v, want %#v", capturedConfig.RuntimeLogConfig, runtimeLogConfig)
	}
}

func TestRun_WithMockWorkersWithoutPathPassesDefaultConfigToService(t *testing.T) {
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	var capturedConfig *service.FactoryServiceConfig
	buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		capturedConfig = cfg
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	err := Run(context.Background(), RunConfig{MockWorkersEnabled: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if capturedConfig == nil || capturedConfig.MockWorkersConfig == nil {
		t.Fatal("expected default mock workers config to be passed to service")
	}
	if len(capturedConfig.MockWorkersConfig.MockWorkers) != 0 {
		t.Fatalf("mock worker count = %d, want empty default accept config", len(capturedConfig.MockWorkersConfig.MockWorkers))
	}
}

func TestRun_WithMockWorkersConfigPathLoadsConfigBeforeServiceStart(t *testing.T) {
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	dir := t.TempDir()
	mockWorkersPath := filepath.Join(dir, "mock-workers.json")
	writeFile(t, mockWorkersPath, `{
  "mockWorkers": [
    {
      "id": "reviewer-rejects",
      "workerName": "reviewer",
      "runType": "reject",
      "rejectConfig": {
        "stderr": "needs changes",
        "exitCode": 42
      }
    }
  ]
}
`)

	var capturedConfig *service.FactoryServiceConfig
	buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		capturedConfig = cfg
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	err := Run(context.Background(), RunConfig{
		MockWorkersEnabled:    true,
		MockWorkersConfigPath: mockWorkersPath,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if capturedConfig == nil || capturedConfig.MockWorkersConfig == nil {
		t.Fatal("expected loaded mock workers config to be passed to service")
	}
	got := capturedConfig.MockWorkersConfig.MockWorkers
	if len(got) != 1 {
		t.Fatalf("mock worker count = %d, want 1", len(got))
	}
	if got[0].ID != "reviewer-rejects" || got[0].WorkerName != "reviewer" {
		t.Fatalf("loaded mock worker = %#v, want reviewer target", got[0])
	}
	if got[0].RejectConfig == nil || got[0].RejectConfig.ExitCode == nil || *got[0].RejectConfig.ExitCode != 42 {
		t.Fatalf("reject config = %#v, want exit code 42", got[0].RejectConfig)
	}
}

func TestRun_WithMockWorkersInvalidPathFailsBeforeServiceStart(t *testing.T) {
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	builderCalled := false
	buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		builderCalled = true
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	err := Run(context.Background(), RunConfig{
		MockWorkersEnabled:    true,
		MockWorkersConfigPath: filepath.Join(t.TempDir(), "missing.json"),
	})
	if err == nil {
		t.Fatal("expected missing mock workers config path to fail")
	}
	if !strings.Contains(err.Error(), "read mock workers config") {
		t.Fatalf("error = %q, want read mock workers config context", err.Error())
	}
	if builderCalled {
		t.Fatal("factory service builder should not be called when mock config loading fails")
	}
}

func TestRun_WithMockWorkersInvalidJSONFailsBeforeServiceStart(t *testing.T) {
	originalBuilder := buildFactoryService
	defer func() {
		buildFactoryService = originalBuilder
	}()

	dir := t.TempDir()
	mockWorkersPath := filepath.Join(dir, "mock-workers.json")
	writeFile(t, mockWorkersPath, `{"mockWorkers":[{"runType":"bogus"}]}`)

	builderCalled := false
	buildFactoryService = func(_ context.Context, cfg *service.FactoryServiceConfig) (factoryServiceRunner, error) {
		builderCalled = true
		return stubFactoryService{
			run: func(context.Context) error {
				return nil
			},
		}, nil
	}

	err := Run(context.Background(), RunConfig{
		MockWorkersEnabled:    true,
		MockWorkersConfigPath: mockWorkersPath,
	})
	if err == nil {
		t.Fatal("expected invalid mock workers config to fail")
	}
	if !strings.Contains(err.Error(), "runType must be one of") {
		t.Fatalf("error = %q, want runType validation context", err.Error())
	}
	if builderCalled {
		t.Fatal("factory service builder should not be called when mock config validation fails")
	}
}
