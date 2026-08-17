package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/platform/metrics"
	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"go.uber.org/zap"
)

type stubModelBootstrapRunner struct {
	invokeModel  func(context.Context, string, factoryapi.ModelInvocationRequest) (modelinference.Result, error)
	run          func(context.Context) error
	sessionReady bool
}

func (s *stubModelBootstrapRunner) Run(ctx context.Context) error {
	if s.run != nil {
		return s.run(ctx)
	}
	<-ctx.Done()
	return ctx.Err()
}

func (s *stubModelBootstrapRunner) InvokeModel(
	ctx context.Context,
	modelName string,
	request factoryapi.ModelInvocationRequest,
) (modelinference.Result, error) {
	if s.invokeModel != nil {
		return s.invokeModel(ctx, modelName, request)
	}
	return modelinference.Result{}, errors.New("unexpected InvokeModel call")
}

func (s *stubModelBootstrapRunner) GetCurrentFactoryForSession(context.Context, string) (factoryapi.Factory, error) {
	if s.sessionReady {
		return factoryapi.Factory{Name: "factory"}, nil
	}
	return factoryapi.Factory{}, apisurface.ErrFactorySessionNotFound
}

func (s *stubModelBootstrapRunner) CloseFactorySession(context.Context, string) error {
	return nil
}

func installStubModelBootstrapRunner(t *testing.T, runner *stubModelBootstrapRunner) {
	t.Helper()
	originalBuilder := openTestModelRunner
	t.Cleanup(func() {
		openTestModelRunner = originalBuilder
	})
	openTestModelRunner = func(_ context.Context, _ *testModelRuntimeSelections) (testModelRunner, error) {
		if runner == nil {
			t.Fatal("stub bootstrap runner is required")
		}
		return runner, nil
	}
}

func readyStubModelBootstrapRunner(invokeModel func(context.Context, string, factoryapi.ModelInvocationRequest) (modelinference.Result, error)) *stubModelBootstrapRunner {
	return &stubModelBootstrapRunner{
		sessionReady: true,
		invokeModel:  invokeModel,
	}
}

func TestInvoke_RoutesThroughSharedBootstrapWithoutHTTPEndpoint(t *testing.T) {
	originalBuilder := openTestModelRunner
	defer func() {
		openTestModelRunner = originalBuilder
	}()

	homeDir := t.TempDir()
	var capturedModel string
	var capturedRequest factoryapi.ModelInvocationRequest
	openTestModelRunner = func(_ context.Context, cfg *testModelRuntimeSelections) (testModelRunner, error) {
		if cfg == nil || strings.TrimSpace(cfg.Dir) == "" {
			t.Fatal("expected bootstrap service config with factory dir")
		}
		if cfg.Port != 0 {
			t.Fatalf("bootstrap port = %d, want 0 for no-server invoke", cfg.Port)
		}
		if cfg.SystemConfigHomeDir != homeDir || cfg.RuntimeLogDir != logging.RuntimeLogsRoot(homeDir) || cfg.RuntimeMetricsDir != metrics.RuntimeMetricsRoot(homeDir) {
			t.Fatalf("bootstrap home paths = home %q logs %q metrics %q; want roots below %q", cfg.SystemConfigHomeDir, cfg.RuntimeLogDir, cfg.RuntimeMetricsDir, homeDir)
		}
		return &stubModelBootstrapRunner{
			sessionReady: true,
			invokeModel: func(_ context.Context, modelName string, request factoryapi.ModelInvocationRequest) (modelinference.Result, error) {
				capturedModel = modelName
				capturedRequest = request
				return modelinference.Result{
					ModelName:        modelName,
					Worker:           "tts-worker",
					Operation:        request.Operation,
					ProviderLocality: string(factoryapi.WorkerModelLocalityLocal),
					Content: []work.WorkContentPart{{
						Type: work.WorkContentPartTypeText,
						Text: "hello",
					}},
				}, nil
			},
		}, nil
	}

	if err := invokeForTest(t, InvokeConfig{Context: context.Background(),
		ModelName:  "OMNIVOICE_Q4_K_M",
		Operation:  "TTS",
		Text:       "hello world",
		FactoryDir: t.TempDir(),
		HomeDir:    homeDir,
		Server:     failureBaselineUnreachableServer,
		JSON:       true,
		Logger:     zap.NewNop(),
		Output:     io.Discard,
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if capturedModel != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("model = %q, want OMNIVOICE_Q4_K_M", capturedModel)
	}
	if capturedRequest.Operation != "TTS" {
		t.Fatalf("operation = %q, want TTS", capturedRequest.Operation)
	}
}

func TestInvoke_UnreachableServerDoesNotFailWithTransportUnreachableMessage(t *testing.T) {
	originalBuilder := openTestModelRunner
	defer func() {
		openTestModelRunner = originalBuilder
	}()

	openTestModelRunner = func(_ context.Context, _ *testModelRuntimeSelections) (testModelRunner, error) {
		return &stubModelBootstrapRunner{
			sessionReady: true,
			invokeModel: func(_ context.Context, modelName string, request factoryapi.ModelInvocationRequest) (modelinference.Result, error) {
				return modelinference.Result{
					ModelName: modelName,
					Worker:    "tts-worker",
					Operation: request.Operation,
				}, nil
			},
		}, nil
	}

	if err := invokeForTest(t, InvokeConfig{Context: context.Background(),
		ModelName:  "OMNIVOICE_Q4_K_M",
		Operation:  "TTS",
		Text:       "hello world",
		FactoryDir: t.TempDir(),
		Server:     failureBaselineUnreachableServer,
		JSON:       true,
		Output:     io.Discard,
		Logger:     zap.NewNop(),
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
}

// capturingModelInvocationOperation records exactly what the CLI hands to the
// composition-root invocation operation, so the target field mapping can be
// asserted directly instead of being inferred from downstream runtime effects.
type capturingModelInvocationOperation struct {
	factoryDir       string
	homeDir          string
	operatorDefaults operatorconfig.ResolvedDefaults
	verbose          bool
	modelName        string
	request          modelinference.Request
	result           modelinference.Result
	err              error
}

func (c *capturingModelInvocationOperation) ResolveModelInvocationFactoryDir(explicit string) (string, error) {
	return explicit, nil
}

func (c *capturingModelInvocationOperation) ExportModelInvocationArtifact(string, string) error {
	return nil
}

func (c *capturingModelInvocationOperation) InvokeModel(
	_ context.Context,
	target InvocationTarget,
	modelName string,
	request modelinference.Request,
) (modelinference.Result, error) {
	c.factoryDir = target.FactoryDir
	c.homeDir = target.HomeDir
	c.operatorDefaults = target.OperatorDefaults
	c.verbose = target.Verbose
	c.modelName = modelName
	c.request = request
	return c.result, c.err
}

// TestRunBootstrapModelInvocation_CarriesResolvedInvocationInputs pins the four
// invocation inputs the bootstrap CLI path actually populates on its way to the
// composition-root operation, plus the model name and mapped request. This is
// the observable contract of the boundary: a later change to how the target is
// carried must keep every one of these values arriving unchanged.
func TestRunBootstrapModelInvocation_CarriesResolvedInvocationInputs(t *testing.T) {
	t.Parallel()

	operation := &capturingModelInvocationOperation{
		result: modelinference.Result{ModelName: "voice", Operation: "TTS", Worker: "narrator"},
	}
	defaults := operatorconfig.ResolvedDefaults{
		WorkerModelProvider: "codex",
		WorkerModel:         "gpt-5.6",
	}
	cfg := InvocationRequest{
		FactoryDir:       filepath.Join("tmp", "factory"),
		HomeDir:          filepath.Join("tmp", "home"),
		OperatorDefaults: defaults,
		Logger:           zap.NewNop(),
		Verbose:          true,
	}

	result, err := runBootstrapModelInvocation(
		context.Background(),
		operation,
		cfg,
		"voice",
		factoryapi.ModelInvocationRequest{
			Operation: "TTS",
			Content:   &factoryapi.WorkContent{mustGeneratedTextContentPart("speak this")},
		},
	)
	if err != nil {
		t.Fatalf("runBootstrapModelInvocation() error = %v, want success", err)
	}
	if result.ModelName != "voice" || result.Operation != "TTS" || result.Worker != "narrator" {
		t.Fatalf("result = %#v, want the operation result returned unchanged", result)
	}
	if operation.factoryDir != cfg.FactoryDir || operation.homeDir != cfg.HomeDir {
		t.Fatalf("target dirs = factory %q home %q, want %q and %q",
			operation.factoryDir, operation.homeDir, cfg.FactoryDir, cfg.HomeDir)
	}
	if operation.operatorDefaults != defaults {
		t.Fatalf("target operator defaults = %#v, want %#v", operation.operatorDefaults, defaults)
	}
	if !operation.verbose {
		t.Fatal("target verbose = false, want the resolved verbose flag carried through")
	}
	if operation.modelName != "voice" {
		t.Fatalf("model name = %q, want voice", operation.modelName)
	}
	if operation.request.Operation != "TTS" || len(operation.request.Content) != 1 {
		t.Fatalf("mapped request = %#v, want the TTS request with one content part", operation.request)
	}
}

// TestRunBootstrapModelInvocation_RejectsMissingInputs pins the two guard
// branches that produce a CLI error before any invocation is attempted.
func TestRunBootstrapModelInvocation_RejectsMissingInputs(t *testing.T) {
	t.Parallel()

	// The guard under test is specifically the nil-context branch, so an
	// explicitly nil Context value is the input being pinned here.
	var absentContext context.Context
	if _, err := runBootstrapModelInvocation(
		absentContext, &capturingModelInvocationOperation{}, InvocationRequest{}, "voice", factoryapi.ModelInvocationRequest{},
	); err == nil || !strings.Contains(err.Error(), "context is required") {
		t.Fatalf("runBootstrapModelInvocation(nil ctx) error = %v, want a context requirement", err)
	}
	if _, err := runBootstrapModelInvocation(
		context.Background(), nil, InvocationRequest{}, "voice", factoryapi.ModelInvocationRequest{},
	); err == nil || !strings.Contains(err.Error(), "models invoke operation is required") {
		t.Fatalf("runBootstrapModelInvocation(nil operation) error = %v, want an operation requirement", err)
	}
}

func TestMapBootstrapModelInvokeError_PreservesInferenceFailureCauseChain(t *testing.T) {
	readinessErr := (modelinference.Runtime{
		Identity:       "OMNIVOICE_Q4_K_M",
		ReadinessState: modelinference.ReadinessStateLoading,
		LifecycleState: modelinference.LifecycleStateLoading,
	}).InvocationError()
	failure := &modelinference.InferenceFailure{
		Class: modelinference.InferenceFailureClassLoadingModel, Message: "managed runtime is loading",
		ModelName: "OMNIVOICE_Q4_K_M", Operation: "TTS", Cause: readinessErr,
	}

	mapped := mapBootstrapModelInvokeError(failure)
	if !errors.Is(mapped, modelinference.ErrLoading) {
		t.Fatalf("mapped error = %v, want ErrManagedRuntimeLoading in chain", mapped)
	}
	var classified *modelinference.InferenceFailure
	if !errors.As(mapped, &classified) || classified.Class != modelinference.InferenceFailureClassLoadingModel {
		t.Fatalf("mapped error = %T, want loading_model InferenceFailure", mapped)
	}
}

func TestInvoke_AudioBootstrapCopiesStreamFile(t *testing.T) {
	originalBuilder := openTestModelRunner
	defer func() {
		openTestModelRunner = originalBuilder
	}()

	streamFile := filepath.Join(t.TempDir(), "stream.wav")
	if err := os.WriteFile(streamFile, []byte("RIFF....WAVE"), 0o644); err != nil {
		t.Fatalf("write stream file: %v", err)
	}

	openTestModelRunner = func(_ context.Context, _ *testModelRuntimeSelections) (testModelRunner, error) {
		return &stubModelBootstrapRunner{
			sessionReady: true,
			invokeModel: func(_ context.Context, modelName string, request factoryapi.ModelInvocationRequest) (modelinference.Result, error) {
				return modelinference.Result{
					ModelName:  modelName,
					Operation:  request.Operation,
					StreamFile: streamFile,
				}, nil
			},
		}, nil
	}

	outputPath := filepath.Join(t.TempDir(), "speech.wav")
	if err := invokeForTest(t, InvokeConfig{Context: context.Background(),
		ModelName:  "OMNIVOICE_Q4_K_M",
		Operation:  "TTS",
		Text:       "hello world",
		OutputPath: outputPath,
		FactoryDir: t.TempDir(),
		Logger:     zap.NewNop(),
		Output:     io.Discard,
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(got) != "RIFF....WAVE" {
		t.Fatalf("output = %q, want streamed audio bytes", string(got))
	}
}
