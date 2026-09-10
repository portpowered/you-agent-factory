package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
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
) (factorysessions.FactoryInvocationOutcome, error) {
	return factorysessions.FactoryInvocationOutcome{}, errors.New("Factory invocation is not supported by the model test operation")
}

func (testModelInvocationOperation) InvokeModel(
	ctx context.Context,
	target InvocationTarget,
	modelName string,
	request modelinference.Request,
) (modelinference.Result, error) {
	executionBaseDir, _ := os.Getwd()
	cfg := &testModelRuntimeSelections{
		Dir:                  target.FactoryDir,
		SystemConfigHomeDir:  target.HomeDir,
		OperatorDefaults:     target.OperatorDefaults,
		ExecutionBaseDir:     executionBaseDir,
		RuntimeMode:          interfaces.RuntimeModeService,
		Logger:               zap.NewNop(),
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

func TestPullEmitsMissingAssetEstimateBeforePull(t *testing.T) {
	t.Parallel()

	scope, err := (modelinference.RuntimeScopeRef{}).Parse("asset-estimate:pull")
	if err != nil {
		t.Fatalf("parse runtime scope: %v", err)
	}
	events := []string{}
	root := &pullCLIModelsService{
		catalog: modelinference.Detail{Summary: modelinference.Summary{Name: "model"}},
		preflightResult: modelinference.PreflightModelAssetsResult{
			ModelName: "model", BackendBytes: 25, ModelBytes: 206, TotalBytes: 231,
			BackendDownloadRequired: true, ModelDownloadRequired: true,
		},
		pullResult: modelinference.PullResult{ModelName: "model", Outcome: "PULLED"},
		events:     &events,
	}
	service := NewService(Config{
		Models: root,
		OpenCatalogScope: func(context.Context) (InvokeRuntimeScope, error) {
			return InvokeRuntimeScope{Scope: scope}, nil
		},
	})
	var diagnostics bytes.Buffer
	var output bytes.Buffer
	if err := service.Pull(PullConfig{
		Context: context.Background(), ModelName: "model", JSON: true,
		Output: &output, Diagnostics: &diagnostics,
	}); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if got, want := diagnostics.String(), "models asset estimate modelName=\"model\" backendBytes=25 modelBytes=206 totalBytes=231\n"; got != want {
		t.Fatalf("asset estimate = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(events, []string{"preflight", "pull"}) {
		t.Fatalf("Models effects = %#v, want preflight before pull", events)
	}
	var response map[string]any
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("pull JSON = %q: %v", output.String(), err)
	}
}

type pullCLIModelsService struct {
	modelinference.Service
	catalog         modelinference.Detail
	preflightResult modelinference.PreflightModelAssetsResult
	preflightErr    error
	pullResult      modelinference.PullResult
	pullErr         error
	events          *[]string
}

func (service *pullCLIModelsService) GetCatalogModel(
	context.Context,
	modelinference.GetModelRequest,
) (modelinference.GetModelResult, error) {
	return modelinference.GetModelResult{Model: service.catalog.Clone()}, nil
}

func (service *pullCLIModelsService) PreflightModelAssets(
	context.Context,
	modelinference.PrepareModelAssetsRequest,
) (modelinference.PreflightModelAssetsResult, error) {
	if service.events != nil {
		*service.events = append(*service.events, "preflight")
	}
	return service.preflightResult, service.preflightErr
}

func (service *pullCLIModelsService) PullModelForScope(
	context.Context,
	modelinference.PullModelRequest,
) (modelinference.PullResult, error) {
	if service.events != nil {
		*service.events = append(*service.events, "pull")
	}
	return service.pullResult, service.pullErr
}

func TestRootInvokeMapsUnsupportedOperationToBadRequest(t *testing.T) {
	t.Parallel()

	root := &rootService{models: &preflightModelsRoot{
		getCatalogErr: modelinference.ErrUnsupportedOperation,
	}}
	_, err := root.catalogForInvoke(
		InvokeConfig{Context: context.Background(), JSON: true},
		preflightTestScope(t), "llm", "NOPE",
	)
	assertCLIContractBadRequest(t, err, "unknown operation \"NOPE\"")
}

func TestRootInvokePreflightsGenericContractBeforeAssetEstimate(t *testing.T) {
	t.Parallel()

	operation, ok := (modelinference.GenericOperationCatalog{}).GenericOperationContract(modelinference.OperationOMNI)
	if !ok {
		t.Fatal("generic OMNI operation is not published")
	}
	cases := []struct {
		name       string
		config     InvokeConfig
		wantClass  modelinference.InvocationFailureClass
		wantPhrase string
	}{
		{
			name: "unknown slot", config: InvokeConfig{
				Context: context.Background(), Output: io.Discard,
				InputMappings: []string{"missing=value"}, JSON: true,
			}, wantClass: modelinference.InvocationFailureClassInvalidSlot, wantPhrase: "unknown input slot",
		},
		{
			name: "missing required slot", config: InvokeConfig{
				Context: context.Background(), Output: io.Discard,
				InputMappings: []string{`parameters=json:{"temperature":0.2}`}, JSON: true,
			}, wantClass: modelinference.InvocationFailureClassInvalidSlot, wantPhrase: "required input slot is missing",
		},
		{
			name: "duplicate slot", config: InvokeConfig{
				Context: context.Background(), Output: io.Discard,
				InputSpecs: []string{
					`{"name":"prompt","modality":"TEXT","content":"one"}`,
					`{"name":"prompt","modality":"TEXT","content":"two"}`,
				}, JSON: true,
			}, wantClass: modelinference.InvocationFailureClassSlotArity, wantPhrase: "accepts at most one value",
		},
		{
			name: "malformed parameter", config: InvokeConfig{
				Context: context.Background(), Output: io.Discard,
				ParameterSpecs: []string{`{"name":"temperature","value":}`}, JSON: true,
			}, wantClass: modelinference.InvocationFailureClassInvalidParameter, wantPhrase: "parse --parameter 1",
		},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			scope := preflightTestScope(t)
			events := []string{}
			modelsRoot := &preflightModelsRoot{
				catalog: modelinference.Detail{Summary: modelinference.Summary{
					Name: "llm", Operations: []modelinference.Operation{operation},
				}}, events: &events,
			}
			root := &rootService{models: modelsRoot}
			cfg := testCase.config
			_, err := root.invokeGenericInScopeWithOffline(cfg, scope, "llm", modelinference.OperationOMNI, "", modelsRoot.catalog, false)
			if err == nil {
				t.Fatal("invokeGenericInScopeWithOffline returned nil, want typed preflight failure")
			}
			var failure *modelinference.InvocationFailure
			if !errors.As(err, &failure) || failure == nil || failure.Class != testCase.wantClass {
				t.Fatalf("invokeGenericInScopeWithOffline error = %v, want invocation class %q", err, testCase.wantClass)
			}
			assertCLIContractBadRequest(t, err, testCase.wantPhrase)
			if len(events) != 0 {
				t.Fatalf("preflight effects = %#v, want no asset estimate or invocation", events)
			}
		})
	}
}

func TestRootInvokeMapsInvalidOutputMappingsToBadRequest(t *testing.T) {
	t.Parallel()

	operation, ok := (modelinference.GenericOperationCatalog{}).GenericOperationContract(modelinference.OperationOMNI)
	if !ok {
		t.Fatal("generic OMNI operation is not published")
	}
	cases := []struct {
		name       string
		mappings   []string
		wantPhrase string
	}{
		{name: "unknown output slot", mappings: []string{"text=result.txt", "other=other.txt"}, wantPhrase: "unknown slot"},
		{name: "incomplete output mapping", mappings: []string{"text=result.txt"}, wantPhrase: "cover every output slot"},
		{name: "malformed output mapping", mappings: []string{"text"}, wantPhrase: "expected slot=path"},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := validateCLIOutputShape(
				InvokeConfig{JSON: true, OutputMappings: testCase.mappings},
				modelinference.Detail{Summary: modelinference.Summary{Operations: []modelinference.Operation{operation}}},
				modelinference.OperationOMNI,
			)
			assertCLIContractBadRequest(t, mapModelsClientError(err), testCase.wantPhrase)
		})
	}
}

func assertCLIContractBadRequest(t *testing.T, err error, wantMessage string) {
	t.Helper()
	if err == nil {
		t.Fatal("error = nil, want BAD_REQUEST")
	}
	var coded interface {
		CLIErrorCode() string
		CLIErrorFamily() factoryapi.ErrorFamily
		CLIErrorMessage() string
	}
	if !errors.As(err, &coded) {
		t.Fatalf("error = %v, want coded CLI diagnostic", err)
	}
	if coded.CLIErrorCode() != modelsRootBadRequestCode || coded.CLIErrorFamily() != factoryapi.ErrorFamilyBadRequest {
		t.Fatalf("CLI diagnostic = (%q, %q), want BAD_REQUEST/BAD_REQUEST", coded.CLIErrorCode(), coded.CLIErrorFamily())
	}
	if !strings.Contains(coded.CLIErrorMessage(), wantMessage) {
		t.Fatalf("CLI diagnostic message = %q, want %q", coded.CLIErrorMessage(), wantMessage)
	}
	if strings.Contains(coded.CLIErrorMessage(), "CLI_COMMAND_FAILED") || strings.Contains(coded.CLIErrorMessage(), "INTERNAL_SERVER_ERROR") {
		t.Fatalf("CLI diagnostic fell back to generic failure: %q", coded.CLIErrorMessage())
	}
}

type preflightModelsRoot struct {
	modelinference.Service
	catalog       modelinference.Detail
	getCatalogErr error
	events        *[]string
}

func (root *preflightModelsRoot) GetCatalogModel(
	context.Context, modelinference.GetModelRequest,
) (modelinference.GetModelResult, error) {
	if root.getCatalogErr != nil {
		return modelinference.GetModelResult{}, root.getCatalogErr
	}
	return modelinference.GetModelResult{Model: root.catalog}, nil
}

func (root *preflightModelsRoot) GetModelReadiness(
	context.Context, modelinference.GetModelReadinessRequest,
) (modelinference.GetModelReadinessResult, error) {
	return modelinference.GetModelReadinessResult{}, modelinference.ErrUnsupportedOperation
}

func (root *preflightModelsRoot) PreflightModelAssets(
	context.Context, modelinference.PrepareModelAssetsRequest,
) (modelinference.PreflightModelAssetsResult, error) {
	if root.events != nil {
		*root.events = append(*root.events, "preflight")
	}
	return modelinference.PreflightModelAssetsResult{}, nil
}

func (root *preflightModelsRoot) InvokeModel(
	context.Context, modelinference.InvokeModelRequest,
) (modelinference.InvokeModelResult, error) {
	if root.events != nil {
		*root.events = append(*root.events, "invoke")
	}
	return modelinference.InvokeModelResult{}, nil
}

func preflightTestScope(t *testing.T) modelinference.RuntimeScopeRef {
	t.Helper()
	scope, err := (modelinference.RuntimeScopeRef{}).Parse("cli-preflight:test-scope")
	if err != nil {
		t.Fatalf("parse preflight scope: %v", err)
	}
	return scope
}
