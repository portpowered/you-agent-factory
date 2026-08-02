package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/platform/metrics"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	contentmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
	"go.uber.org/zap"
)

// These hooks keep transport-focused tests able to inject deterministic
// runners without restoring a production transport construction fallback.
type InvocationRunner interface {
	Run(context.Context) error
	InvokeModel(context.Context, string, factoryapi.ModelInvocationRequest) (modelinference.Result, error)
	GetCurrentFactoryForSession(context.Context, string) (factoryapi.Factory, error)
	CloseFactorySession(context.Context, string) error
}

type testModelRunner = InvocationRunner

func TestPresentationScopeRequestFromInvokePreservesModelDefaults(t *testing.T) {
	t.Parallel()

	request := presentationScopeRequestFromInvoke(InvokeConfig{
		OperatorDefaults: operatorconfig.ResolvedDefaults{
			WorkerModelProvider: "codex",
			WorkerModel:         "gpt-5.6",
		},
	})
	if request.OperatorDefaults.WorkerModelProvider != "codex" {
		t.Fatalf("provider = %q, want codex", request.OperatorDefaults.WorkerModelProvider)
	}
	if request.OperatorDefaults.WorkerModel != "gpt-5.6" {
		t.Fatalf("model = %q, want gpt-5.6", request.OperatorDefaults.WorkerModel)
	}
}

type testModelRuntimeSelections struct {
	Dir                  string
	SystemConfigHomeDir  string
	OperatorDefaults     operatorconfig.ResolvedDefaults
	ExecutionBaseDir     string
	RuntimeMode          interfaces.RuntimeMode
	Logger               *zap.Logger
	Verbose              bool
	RuntimeLogDir        string
	RuntimeLogConfig     logging.RuntimeLogConfig
	RuntimeMetricsDir    string
	RuntimeMetricsConfig platformmetrics.RuntimeMetricsConfig
	Port                 int
	WorkFile             string
}

type testModelRunnerOpener func(context.Context, *testModelRuntimeSelections) (testModelRunner, error)

var openTestModelRunner testModelRunnerOpener

var testModelInvocationBuilder InvocationOperation = testModelInvocationOperation{}

func invokeForTest(t *testing.T, cfg InvokeConfig) error {
	t.Helper()
	return New(testHTTPProtocol(t), testModelInvocationBuilder).Invoke(cfg)
}

type testModelInvocationOperation struct{}

func (testModelInvocationOperation) ResolveModelInvocationFactoryDir(explicit string) (string, error) {
	if root := strings.TrimSpace(explicit); root != "" {
		return root, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(cwd, interfaces.FactoryDir), nil
}

func (testModelInvocationOperation) ExportModelInvocationArtifact(sourcePath, destinationPath string) error {
	input, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.Create(destinationPath)
	if err != nil {
		return err
	}
	defer output.Close()
	_, err = io.Copy(output, input)
	return err
}

func (testModelInvocationOperation) InvokeFactory(
	context.Context,
	factorysessions.InvocationTarget,
	factorysessions.InvocationRequest,
	factorysessions.FactoryEventConsumer,
) (factorysessions.FactoryInvocationOutcome, error) {
	return factorysessions.FactoryInvocationOutcome{}, errors.New("Factory invocation is not supported by the model test operation")
}

func (testModelInvocationOperation) InvokeModel(
	ctx context.Context,
	target factorysessions.InvocationTarget,
	modelName string,
	request modelinference.Request,
) (modelinference.Result, error) {
	executionBaseDir := target.ExecutionBaseDir
	if executionBaseDir == "" {
		executionBaseDir, _ = os.Getwd()
	}
	cfg := &testModelRuntimeSelections{
		Dir:                  target.FactoryDir,
		SystemConfigHomeDir:  target.HomeDir,
		OperatorDefaults:     target.OperatorDefaults,
		ExecutionBaseDir:     executionBaseDir,
		RuntimeMode:          interfaces.RuntimeModeService,
		Logger:               target.Logger,
		Verbose:              target.Verbose,
		RuntimeLogConfig:     logging.DefaultRuntimeLogConfig(),
		RuntimeMetricsConfig: platformmetrics.DefaultRuntimeMetricsConfig(),
	}
	if strings.TrimSpace(target.HomeDir) != "" {
		cfg.RuntimeLogDir = logging.RuntimeLogsRoot(target.HomeDir)
		cfg.RuntimeMetricsDir = metrics.RuntimeMetricsRoot(target.HomeDir)
	}
	normalized := normalizeTestInvocationConfig(cfg)
	if openTestModelRunner == nil {
		return modelinference.Result{}, fmt.Errorf("test model invocation runner is not installed")
	}
	runner, err := openTestModelRunner(ctx, normalized)
	if err != nil {
		return modelinference.Result{}, err
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	runErr := make(chan error, 1)
	go func() { runErr <- runner.Run(runCtx) }()
	for {
		if _, readyErr := runner.GetCurrentFactoryForSession(runCtx, factorysessions.DefaultSessionID); readyErr == nil {
			break
		}
		select {
		case err = <-runErr:
			cancel()
			return modelinference.Result{}, err
		case <-runCtx.Done():
			return modelinference.Result{}, runCtx.Err()
		default:
		}
	}
	generated := factoryapi.ModelInvocationRequest{
		Operation: request.Operation,
		Content:   contentmapping.GeneratedPtrFromParts(request.Content),
	}
	if request.Options != nil {
		mode := factoryapi.ModelInvocationResponseMode(request.Options.ResponseMode)
		generated.Options = &factoryapi.ModelInvocationOptions{ResponseMode: &mode}
	}
	result, err := runner.InvokeModel(runCtx, modelName, generated)
	closeErr := runner.CloseFactorySession(runCtx, factorysessions.DefaultSessionID)
	cancel()
	lifecycleErr := <-runErr
	if err == nil {
		err = closeErr
	}
	if err == nil && lifecycleErr != nil && !errors.Is(lifecycleErr, context.Canceled) {
		err = lifecycleErr
	}
	return result, err
}

func normalizeTestInvocationConfig(cfg *testModelRuntimeSelections) *testModelRuntimeSelections {
	if cfg == nil {
		return nil
	}
	normalized := *cfg
	normalized.Port = 0
	normalized.RuntimeMode = interfaces.RuntimeModeService
	normalized.WorkFile = ""
	return &normalized
}
