package models

import (
	"context"
	"os"
	"strings"

	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	modelsservice "github.com/portpowered/infinite-you/pkg/models/service"
	"github.com/portpowered/infinite-you/pkg/service"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping"
)

// These hooks keep transport-focused tests able to inject deterministic
// runners without restoring a production transport construction fallback.
type modelBootstrapRunner = InvocationRunner
type modelInvocationBootstrapBuilder func(context.Context, *service.FactoryServiceConfig) (modelBootstrapRunner, error)

var buildModelInvocationBootstrap modelInvocationBootstrapBuilder = buildRealTestModelInvocationBootstrap
var augmentModelsInvokeBootstrapServiceConfig func(*service.FactoryServiceConfig)

func testModelInvocationBuilder(ctx context.Context, request InvocationRequest) (InvocationRunner, error) {
	executionBaseDir, _ := os.Getwd()
	cfg := &service.FactoryServiceConfig{
		Dir:                  request.FactoryDir,
		SystemConfigHomeDir:  request.HomeDir,
		OperatorDefaults:     request.OperatorDefaults,
		ExecutionBaseDir:     executionBaseDir,
		RuntimeMode:          interfaces.RuntimeModeService,
		Logger:               request.Logger,
		Verbose:              request.Verbose,
		RuntimeLogConfig:     logging.DefaultRuntimeLogConfig(),
		RuntimeMetricsConfig: logging.DefaultRuntimeMetricsConfig(),
	}
	if strings.TrimSpace(request.HomeDir) != "" {
		cfg.RuntimeLogDir = defaultpaths.RuntimeLogsRoot(request.HomeDir)
		cfg.RuntimeMetricsDir = defaultpaths.RuntimeMetricsRoot(request.HomeDir)
	}
	normalized := service.NormalizeInvocationBootstrapConfig(cfg)
	if augmentModelsInvokeBootstrapServiceConfig != nil {
		augmentModelsInvokeBootstrapServiceConfig(normalized)
	}
	return buildModelInvocationBootstrap(ctx, normalized)
}

func buildRealTestModelInvocationBootstrap(ctx context.Context, cfg *service.FactoryServiceConfig) (InvocationRunner, error) {
	svc, err := service.BuildFactoryService(ctx, service.NormalizeInvocationBootstrapConfig(cfg))
	if err != nil {
		return nil, err
	}
	bootstrap, err := service.NewInvocationBootstrap(svc)
	if err != nil {
		return nil, err
	}
	shell := service.FactoryServiceShell{Service: bootstrap.Service}
	deps, err := service.ModelServiceDependencies(shell)
	if err != nil {
		return nil, err
	}
	models, err := modelsservice.NewService(deps)
	if err != nil {
		return nil, err
	}
	bootstrap.Service = service.AttachModelServiceCollaborator(shell, models)
	return &testBootstrapModelRunner{bootstrap: bootstrap}, nil
}

type testBootstrapModelRunner struct{ bootstrap *service.InvocationBootstrap }

func (r *testBootstrapModelRunner) Run(ctx context.Context) error { return r.bootstrap.Run(ctx) }

func (r *testBootstrapModelRunner) InvokeModel(ctx context.Context, modelName string, request factoryapi.ModelInvocationRequest) (apisurface.ModelInvocationResult, error) {
	return r.bootstrap.Service.InvokeModel(ctx, modelName, request)
}

func (r *testBootstrapModelRunner) GetCurrentFactoryForSession(ctx context.Context, sessionID string) (factoryapi.Factory, error) {
	return r.bootstrap.GetCurrentFactoryForSession(ctx, sessionID)
}

func (r *testBootstrapModelRunner) CloseFactorySession(ctx context.Context, sessionID string) error {
	return r.bootstrap.CloseFactorySession(ctx, sessionID)
}
