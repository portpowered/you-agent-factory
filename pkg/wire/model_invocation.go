package wire

import (
	"context"
	"os"
	"strings"

	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	"github.com/portpowered/infinite-you/pkg/service"
	modelscli "github.com/portpowered/infinite-you/pkg/transports/cli/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

// BuildModelInvocation constructs the model invocation collaborator selected
// by normalized CLI inputs. The transport owns parsing and rendering only.
func BuildModelInvocation(ctx context.Context, request modelscli.InvocationRequest) (modelscli.InvocationRunner, error) {
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
		RuntimeMetricsConfig: platformmetrics.DefaultRuntimeMetricsConfig(),
	}
	if strings.TrimSpace(request.HomeDir) != "" {
		cfg.RuntimeLogDir = defaultpaths.RuntimeLogsRoot(request.HomeDir)
		cfg.RuntimeMetricsDir = defaultpaths.RuntimeMetricsRoot(request.HomeDir)
	}
	bootstrap, err := service.BuildInvocationBootstrap(ctx, service.NormalizeInvocationBootstrapConfig(cfg))
	if err != nil {
		return nil, err
	}
	return &modelInvocationRunner{bootstrap: bootstrap}, nil
}

type modelInvocationRunner struct {
	bootstrap *service.InvocationBootstrap
}

func (r *modelInvocationRunner) Run(ctx context.Context) error { return r.bootstrap.Run(ctx) }

func (r *modelInvocationRunner) InvokeModel(ctx context.Context, modelName string, request factoryapi.ModelInvocationRequest) (apisurface.ModelInvocationResult, error) {
	return r.bootstrap.Service.InvokeModel(ctx, modelName, request)
}

func (r *modelInvocationRunner) GetCurrentFactoryForSession(ctx context.Context, sessionID string) (factoryapi.Factory, error) {
	return r.bootstrap.GetCurrentFactoryForSession(ctx, sessionID)
}

func (r *modelInvocationRunner) CloseFactorySession(ctx context.Context, sessionID string) error {
	return r.bootstrap.CloseFactorySession(ctx, sessionID)
}
