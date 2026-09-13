package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/platform/metrics"
	platformmetrics "github.com/portpowered/infinite-you/pkg/platform/metrics"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
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
				InvokeConfig{JSON: true, InputMappings: []string{"text=hello"}, OutputMappings: testCase.mappings},
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

func TestInvoke_NonReadyManagedOutcomes_StubBootstrapPreservesReadinessFailureClasses(t *testing.T) {
	tests := []struct {
		name           string
		invokeErr      error
		wantIs         error
		wantContains   []string
		wantNotContain string
	}{
		{
			name: "missing_inference_failure",
			invokeErr: classifiedBootstrapInvokeFailure(
				factoryapi.ManagedRuntimeReadinessStateMISSING,
				factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED,
			),
			wantIs: modelinference.ErrMissing,
			wantContains: []string{
				"pull or install",
				"OMNIVOICE_Q4_K_M",
			},
			wantNotContain: "models endpoint not reachable",
		},
		{
			name: "loading_inference_failure",
			invokeErr: classifiedBootstrapInvokeFailure(
				factoryapi.ManagedRuntimeReadinessStateLOADING,
				factoryapi.ManagedRuntimeLifecycleStateLOADING,
			),
			wantIs: modelinference.ErrLoading,
			wantContains: []string{
				"still loading",
				"OMNIVOICE_Q4_K_M",
			},
			wantNotContain: "models endpoint not reachable",
		},
		{
			name: "failed_inference_failure",
			invokeErr: classifiedBootstrapInvokeFailure(
				factoryapi.ManagedRuntimeReadinessStateFAILED,
				factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED,
			),
			wantIs: modelinference.ErrFailed,
			wantContains: []string{
				"readiness is FAILED",
				"resolve the managed runtime failure",
			},
			wantNotContain: "models endpoint not reachable",
		},
		{
			name: "unsupported_inference_failure",
			invokeErr: classifiedBootstrapInvokeFailure(
				factoryapi.ManagedRuntimeReadinessStateUNSUPPORTED,
				factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED,
			),
			wantIs: modelinference.ErrUnsupported,
			wantContains: []string{
				"readiness is UNSUPPORTED",
				"supported managed runtime",
			},
			wantNotContain: "models endpoint not reachable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			installStubModelBootstrapRunner(t, readyStubModelBootstrapRunner(func(
				_ context.Context,
				_ string,
				_ factoryapi.ModelInvocationRequest,
			) (modelinference.Result, error) {
				return modelinference.Result{}, tt.invokeErr
			}))

			err := invokeForTest(t, InvokeConfig{Context: context.Background(),
				ModelName:  "OMNIVOICE_Q4_K_M",
				Operation:  "TTS",
				Text:       "hello world",
				FactoryDir: t.TempDir(),
				Server:     failureBaselineUnreachableServer,
				OutputPath: filepath.Join(t.TempDir(), "speech.wav"),
				Logger:     zap.NewNop(),
				Output:     io.Discard,
			})
			if err == nil {
				t.Fatal("expected readiness-gated invoke failure")
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Fatalf("error = %v, want errors.Is %v", err, tt.wantIs)
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error = %q, want substring %q", err.Error(), want)
				}
			}
			if tt.wantNotContain != "" && strings.Contains(err.Error(), tt.wantNotContain) {
				t.Fatalf("error = %q, want to avoid transport failure %q", err.Error(), tt.wantNotContain)
			}
		})
	}
}

func TestInvoke_NonReadyManagedOutcomes_StubBootstrapPreservesManagedRuntimeVocabulary(t *testing.T) {
	tests := []struct {
		name          string
		readiness     factoryapi.ManagedRuntimeReadinessState
		lifecycle     factoryapi.ManagedRuntimeLifecycleState
		wantIs        error
		wantReadiness modelinference.ReadinessState
		wantLifecycle modelinference.LifecycleState
	}{
		{
			name:          "missing",
			readiness:     factoryapi.ManagedRuntimeReadinessStateMISSING,
			lifecycle:     factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED,
			wantIs:        modelinference.ErrMissing,
			wantReadiness: modelinference.ReadinessStateMissing,
			wantLifecycle: modelinference.LifecycleStateNotInstalled,
		},
		{
			name:          "loading",
			readiness:     factoryapi.ManagedRuntimeReadinessStateLOADING,
			lifecycle:     factoryapi.ManagedRuntimeLifecycleStateLOADING,
			wantIs:        modelinference.ErrLoading,
			wantReadiness: modelinference.ReadinessStateLoading,
			wantLifecycle: modelinference.LifecycleStateLoading,
		},
		{
			name:          "failed",
			readiness:     factoryapi.ManagedRuntimeReadinessStateFAILED,
			lifecycle:     factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED,
			wantIs:        modelinference.ErrFailed,
			wantReadiness: modelinference.ReadinessStateFailed,
			wantLifecycle: modelinference.LifecycleStateNotInstalled,
		},
		{
			name:          "unsupported",
			readiness:     factoryapi.ManagedRuntimeReadinessStateUNSUPPORTED,
			lifecycle:     factoryapi.ManagedRuntimeLifecycleStateNOTINSTALLED,
			wantIs:        modelinference.ErrUnsupported,
			wantReadiness: modelinference.ReadinessStateUnsupported,
			wantLifecycle: modelinference.LifecycleStateNotInstalled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			invokeErr := (modelinference.Runtime{
				Identity:       "OMNIVOICE_Q4_K_M",
				ReadinessState: modelinference.ReadinessState(tt.readiness),
				LifecycleState: modelinference.LifecycleState(tt.lifecycle),
			}).InvocationError()
			installStubModelBootstrapRunner(t, readyStubModelBootstrapRunner(func(
				_ context.Context,
				_ string,
				_ factoryapi.ModelInvocationRequest,
			) (modelinference.Result, error) {
				return modelinference.Result{}, invokeErr
			}))

			err := invokeForTest(t, InvokeConfig{Context: context.Background(),
				ModelName:  "OMNIVOICE_Q4_K_M",
				Operation:  "TTS",
				Text:       "hello world",
				FactoryDir: t.TempDir(),
				Server:     failureBaselineUnreachableServer,
				OutputPath: filepath.Join(t.TempDir(), "speech.wav"),
				Logger:     zap.NewNop(),
				Output:     io.Discard,
			})
			if err == nil {
				t.Fatal("expected managed runtime readiness failure")
			}
			if !errors.Is(err, tt.wantIs) {
				t.Fatalf("error = %v, want errors.Is %v", err, tt.wantIs)
			}
			var readinessErr *modelinference.InvocationError
			if !errors.As(err, &readinessErr) {
				t.Fatalf("error = %T, want *ManagedRuntimeInvocationError", err)
			}
			if readinessErr.ReadinessState != tt.wantReadiness {
				t.Fatalf("readiness = %s, want %s", readinessErr.ReadinessState, tt.wantReadiness)
			}
			if readinessErr.LifecycleState != tt.wantLifecycle {
				t.Fatalf("lifecycle = %s, want %s", readinessErr.LifecycleState, tt.wantLifecycle)
			}
			if !strings.Contains(err.Error(), string(tt.wantReadiness)) {
				t.Fatalf("error = %q, want readiness token %q", err.Error(), tt.wantReadiness)
			}
			if strings.Contains(err.Error(), "models endpoint not reachable") {
				t.Fatalf("error = %q, want bootstrap readiness failure instead of transport failure", err.Error())
			}
		})
	}
}

func classifiedBootstrapInvokeFailure(
	readiness factoryapi.ManagedRuntimeReadinessState,
	lifecycle factoryapi.ManagedRuntimeLifecycleState,
) error {
	readinessErr := (modelinference.Runtime{
		Identity:       "OMNIVOICE_Q4_K_M",
		ReadinessState: modelinference.ReadinessState(readiness),
		LifecycleState: modelinference.LifecycleState(lifecycle),
	}).InvocationError()
	message := readinessErr.Error()
	switch readiness {
	case factoryapi.ManagedRuntimeReadinessStateMISSING:
		message = "model \"OMNIVOICE_Q4_K_M\" is not available: pull or install the managed runtime before invoking"
	case factoryapi.ManagedRuntimeReadinessStateLOADING:
		message = "model \"OMNIVOICE_Q4_K_M\" is still loading: wait for the managed runtime to finish loading and retry the invocation"
	}
	return &modelinference.InferenceFailure{
		Class: readinessFailureClass(readiness), Message: message,
		ModelName: "OMNIVOICE_Q4_K_M", WorkerName: "voice-local", Operation: "TTS", Cause: readinessErr,
	}
}

func readinessFailureClass(readiness factoryapi.ManagedRuntimeReadinessState) modelinference.InferenceFailureClass {
	switch readiness {
	case factoryapi.ManagedRuntimeReadinessStateMISSING:
		return modelinference.InferenceFailureClassMissingModel
	case factoryapi.ManagedRuntimeReadinessStateLOADING:
		return modelinference.InferenceFailureClassLoadingModel
	default:
		return modelinference.InferenceFailureClassRuntimeFailure
	}
}

func TestInvokeGenericMapsBackendPreflightFailure(t *testing.T) {
	t.Parallel()

	scope, err := (modelinference.RuntimeScopeRef{}).Parse("asset-estimate:failure")
	if err != nil {
		t.Fatalf("parse runtime scope: %v", err)
	}
	catalog := modelinference.Detail{Summary: modelinference.Summary{
		Operations: []modelinference.Operation{{
			Name:    modelinference.OperationOMNI,
			Outputs: []modelinference.OperationSlot{{Name: "text", Modality: modelinference.ModalityText}},
		}},
	}}
	root := &genericCLIModelsService{
		catalog:      catalog,
		preflightErr: fmt.Errorf("controlled HEAD failed: %w", modelinference.ErrAssetBackendNotReady),
	}
	service := &rootService{models: root}
	handled, err := service.invokeGenericInScopeWithOffline(
		InvokeConfig{Context: context.Background(), Output: io.Discard, JSON: true},
		scope, "model", modelinference.OperationOMNI, "hello", catalog, false,
	)
	if !handled || err == nil {
		t.Fatalf("invokeGenericInScope = handled:%v error:%v, want typed failure", handled, err)
	}
	var coded interface {
		CLIErrorCode() string
		CLIErrorMessage() string
	}
	if !errors.As(err, &coded) || coded.CLIErrorCode() != "MODEL_BACKEND_NOT_READY" || coded.CLIErrorMessage() != "managed model backend is unavailable" {
		t.Fatalf("mapped preflight error = %v, want backend readiness diagnostic", err)
	}
}

func TestModelsRemoteGenericInvokeCatalogFailuresAvoidPost(t *testing.T) {
	t.Parallel()
	t.Run("outage", testRemoteGenericCatalogOutage)
	t.Run("cancellation", testRemoteGenericCatalogCancellation)
}

func testRemoteGenericCatalogOutage(t *testing.T) {
	var catalogCalls atomic.Int32
	var postCalls atomic.Int32
	server := httptest.NewServer(remoteCatalogOutageHandler(&catalogCalls, &postCalls))
	defer server.Close()

	var output bytes.Buffer
	err := remoteHTTPService(t, remoteStaticInputReader([]byte("PNG"))).Invoke(remoteInvokeConfig(
		context.Background(), server.URL, []string{"prompt=hello", "image=@fixture.png"}, &output,
	))
	var apiErr *clihttp.APIError
	if err == nil || !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("catalog outage = %v, want typed service-unavailable error", err)
	}
	if catalogCalls.Load() != 1 || postCalls.Load() != 0 || output.Len() != 0 {
		t.Fatalf("catalog outage effects = GET:%d POST:%d output:%q, want 1/0/empty", catalogCalls.Load(), postCalls.Load(), output.String())
	}
}

func remoteCatalogOutageHandler(catalogCalls, postCalls *atomic.Int32) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodGet {
			catalogCalls.Add(1)
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(writer).Encode(factoryapi.ErrorResponse{
				Code:   factoryapi.ErrorResponseCode("MODEL_BACKEND_NOT_READY"),
				Family: factoryapi.ErrorFamilyInternalServerError, Message: "catalog unavailable",
			})
			return
		}
		if request.Method == http.MethodPost {
			postCalls.Add(1)
		}
		http.NotFound(writer, request)
	}
}

func testRemoteGenericCatalogCancellation(t *testing.T) {
	started := make(chan struct{})
	done := make(chan struct{})
	server := httptest.NewServer(remoteCatalogCancellationHandler(started, done))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var output bytes.Buffer
	result := make(chan error, 1)
	go func() {
		result <- remoteHTTPService(t, remoteStaticInputReader([]byte("PNG"))).Invoke(remoteInvokeConfig(
			ctx, server.URL, []string{"prompt=hello", "image=@fixture.png"}, &output,
		))
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("catalog request did not start")
	}
	cancel()
	select {
	case err := <-result:
		if err == nil || !errors.Is(err, context.Canceled) {
			t.Fatalf("catalog cancellation = %v, want context cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("catalog cancellation did not return")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("catalog handler did not observe cancellation")
	}
	if output.Len() != 0 {
		t.Fatalf("catalog cancellation output = %q, want empty", output.String())
	}
}

func remoteCatalogCancellationHandler(started, done chan struct{}) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			http.NotFound(writer, request)
			return
		}
		close(started)
		<-request.Context().Done()
		close(done)
	}
}

func remoteASRModelDetail() factoryapi.ModelDetail {
	required := true
	return factoryapi.ModelDetail{
		Name: "asr",
		Operations: []factoryapi.ModelInvocationOperation{{
			Name: modelinference.OperationASR,
			Inputs: remotePointerSlice([]factoryapi.ModelInvocationSlot{{
				Name: "audio", Modality: remotePointer(factoryapi.ModelInvocationContentTypeAudio), Required: &required,
			}}),
			Outputs: remotePointerSlice([]factoryapi.ModelInvocationSlot{
				{Name: "transcript", Modality: remotePointer(factoryapi.ModelInvocationContentTypeText)},
				{Name: "segments", Modality: remotePointer(factoryapi.ModelInvocationContentTypeJSON)},
			}),
		}},
	}
}

func remoteParameterOutputModelDetail() factoryapi.ModelDetail {
	required := true
	return factoryapi.ModelDetail{
		Name: "llm",
		Operations: []factoryapi.ModelInvocationOperation{{
			Name: modelinference.OperationOMNI,
			Inputs: remotePointerSlice([]factoryapi.ModelInvocationSlot{
				{Name: "prompt", Modality: remotePointer(factoryapi.ModelInvocationContentTypeText), Required: &required, MediaTypes: remotePointerSlice([]string{"text/plain"})},
				{Name: "image", Modality: remotePointer(factoryapi.ModelInvocationContentTypeImage), MediaTypes: remotePointerSlice([]string{"image/*"})},
				{Name: "parameters", Modality: remotePointer(factoryapi.ModelInvocationContentTypeJSON), MediaTypes: remotePointerSlice([]string{"application/json"})},
			}),
			Outputs: remotePointerSlice([]factoryapi.ModelInvocationSlot{
				{Name: "text", Modality: remotePointer(factoryapi.ModelInvocationContentTypeText)},
				{Name: "usage", Modality: remotePointer(factoryapi.ModelInvocationContentTypeJSON)},
			}),
		}},
	}
}
