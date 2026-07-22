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
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
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

func TestMapBootstrapModelInvokeError_PreservesInferenceFailureCauseChain(t *testing.T) {
	readinessErr := (modelinference.Runtime{
		Identity:       "OMNIVOICE_Q4_K_M",
		ReadinessState: modelinference.ReadinessStateLoading,
		LifecycleState: modelinference.LifecycleStateLoading,
	}).InvocationError()
	failure := &workers.InferenceFailure{
		Class: workers.InferenceFailureClassLoadingModel, Message: "managed runtime is loading",
		ModelName: "OMNIVOICE_Q4_K_M", Operation: "TTS", Cause: readinessErr,
	}

	mapped := mapBootstrapModelInvokeError(failure)
	if !errors.Is(mapped, modelinference.ErrLoading) {
		t.Fatalf("mapped error = %v, want ErrManagedRuntimeLoading in chain", mapped)
	}
	if failure, ok := workers.AsInferenceFailure(mapped); !ok || failure.Class != workers.InferenceFailureClassLoadingModel {
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
