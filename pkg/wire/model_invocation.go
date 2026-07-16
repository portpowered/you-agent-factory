package wire

import (
	"context"
	"fmt"
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
	runner, err := buildInvocationRunner(ctx, service.NormalizeInvocationBootstrapConfig(cfg))
	if err != nil {
		return nil, err
	}
	modelRunner, ok := runner.(modelscli.InvocationRunner)
	if !ok {
		return nil, fmt.Errorf("build model invocation: shared invocation runner does not implement model invocation")
	}
	return &modelInvocationRunner{runner: modelRunner}, nil
}

type modelInvocationRunner struct {
	runner modelscli.InvocationRunner
}

func (r *modelInvocationRunner) Run(ctx context.Context) error { return r.runner.Run(ctx) }

func (r *modelInvocationRunner) InvokeModel(ctx context.Context, modelName string, request factoryapi.ModelInvocationRequest) (apisurface.ModelInvocationResult, error) {
	return r.runner.InvokeModel(ctx, modelName, request)
}

func (r *modelInvocationRunner) GetCurrentFactoryForSession(ctx context.Context, sessionID string) (factoryapi.Factory, error) {
	return r.runner.GetCurrentFactoryForSession(ctx, sessionID)
}

func (r *modelInvocationRunner) CloseFactorySession(ctx context.Context, sessionID string) error {
	return r.runner.CloseFactorySession(ctx, sessionID)
}
