package models

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
	"github.com/portpowered/infinite-you/pkg/service"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/work"
	"go.uber.org/zap"
)

type stubModelBootstrapRunner struct {
	invokeModel  func(context.Context, string, factoryapi.ModelInvocationRequest) (apisurface.ModelInvocationResult, error)
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
) (apisurface.ModelInvocationResult, error) {
	if s.invokeModel != nil {
		return s.invokeModel(ctx, modelName, request)
	}
	return apisurface.ModelInvocationResult{}, errors.New("unexpected InvokeModel call")
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
	originalBuilder := buildModelInvocationBootstrap
	t.Cleanup(func() {
		buildModelInvocationBootstrap = originalBuilder
	})
	buildModelInvocationBootstrap = func(_ context.Context, _ *service.FactoryServiceConfig) (modelBootstrapRunner, error) {
		if runner == nil {
			t.Fatal("stub bootstrap runner is required")
		}
		return runner, nil
	}
}

func readyStubModelBootstrapRunner(invokeModel func(context.Context, string, factoryapi.ModelInvocationRequest) (apisurface.ModelInvocationResult, error)) *stubModelBootstrapRunner {
	return &stubModelBootstrapRunner{
		sessionReady: true,
		invokeModel:  invokeModel,
	}
}

func TestInvoke_RoutesThroughSharedBootstrapWithoutHTTPEndpoint(t *testing.T) {
	originalBuilder := buildModelInvocationBootstrap
	defer func() {
		buildModelInvocationBootstrap = originalBuilder
	}()

	homeDir := t.TempDir()
	var capturedModel string
	var capturedRequest factoryapi.ModelInvocationRequest
	buildModelInvocationBootstrap = func(_ context.Context, cfg *service.FactoryServiceConfig) (modelBootstrapRunner, error) {
		if cfg == nil || strings.TrimSpace(cfg.Dir) == "" {
			t.Fatal("expected bootstrap service config with factory dir")
		}
		if cfg.Port != 0 {
			t.Fatalf("bootstrap port = %d, want 0 for no-server invoke", cfg.Port)
		}
		if cfg.APIServerStarter != nil {
			t.Fatal("expected bootstrap config to skip API server starter")
		}
		if cfg.SystemConfigHomeDir != homeDir || cfg.RuntimeLogDir != defaultpaths.RuntimeLogsRoot(homeDir) || cfg.RuntimeMetricsDir != defaultpaths.RuntimeMetricsRoot(homeDir) {
			t.Fatalf("bootstrap home paths = home %q logs %q metrics %q; want roots below %q", cfg.SystemConfigHomeDir, cfg.RuntimeLogDir, cfg.RuntimeMetricsDir, homeDir)
		}
		return &stubModelBootstrapRunner{
			sessionReady: true,
			invokeModel: func(_ context.Context, modelName string, request factoryapi.ModelInvocationRequest) (apisurface.ModelInvocationResult, error) {
				capturedModel = modelName
				capturedRequest = request
				return apisurface.ModelInvocationResult{
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

	if err := Invoke(InvokeConfig{
		BuildInvocation: testModelInvocationBuilder,
		ModelName:       "OMNIVOICE_Q4_K_M",
		Operation:       "TTS",
		Text:            "hello world",
		FactoryDir:      t.TempDir(),
		HomeDir:         homeDir,
		Server:          failureBaselineUnreachableServer,
		JSON:            true,
		Logger:          zap.NewNop(),
		Output:          io.Discard,
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
	originalBuilder := buildModelInvocationBootstrap
	defer func() {
		buildModelInvocationBootstrap = originalBuilder
	}()

	buildModelInvocationBootstrap = func(_ context.Context, _ *service.FactoryServiceConfig) (modelBootstrapRunner, error) {
		return &stubModelBootstrapRunner{
			sessionReady: true,
			invokeModel: func(_ context.Context, modelName string, request factoryapi.ModelInvocationRequest) (apisurface.ModelInvocationResult, error) {
				return apisurface.ModelInvocationResult{
					ModelName: modelName,
					Worker:    "tts-worker",
					Operation: request.Operation,
				}, nil
			},
		}, nil
	}

	if err := Invoke(InvokeConfig{
		BuildInvocation: testModelInvocationBuilder,
		ModelName:       "OMNIVOICE_Q4_K_M",
		Operation:       "TTS",
		Text:            "hello world",
		FactoryDir:      t.TempDir(),
		Server:          failureBaselineUnreachableServer,
		JSON:            true,
		Output:          io.Discard,
		Logger:          zap.NewNop(),
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
}

func TestMapBootstrapModelInvokeError_PreservesInferenceFailureCauseChain(t *testing.T) {
	readinessErr := apisurface.InvocationErrorFromManagedRuntime(factoryapi.ManagedRuntime{
		Identity:       "OMNIVOICE_Q4_K_M",
		ReadinessState: factoryapi.ManagedRuntimeReadinessStateLOADING,
		LifecycleState: factoryapi.ManagedRuntimeLifecycleStateLOADING,
	})
	failure, ok := apisurface.ClassifyInferenceFailure(readinessErr, apisurface.InferenceFailureContext{
		ModelName: "OMNIVOICE_Q4_K_M",
		Operation: "TTS",
	})
	if !ok || failure == nil {
		t.Fatal("expected classified inference failure")
	}

	mapped := mapBootstrapModelInvokeError(failure)
	if !errors.Is(mapped, apisurface.ErrManagedRuntimeLoading) {
		t.Fatalf("mapped error = %v, want ErrManagedRuntimeLoading in chain", mapped)
	}
	if failure, ok := apisurface.AsInferenceFailure(mapped); !ok || failure.Class != apisurface.InferenceFailureClassLoadingModel {
		t.Fatalf("mapped error = %T, want loading_model InferenceFailure", mapped)
	}
}

func TestInvoke_AudioBootstrapCopiesStreamFile(t *testing.T) {
	originalBuilder := buildModelInvocationBootstrap
	defer func() {
		buildModelInvocationBootstrap = originalBuilder
	}()

	streamFile := filepath.Join(t.TempDir(), "stream.wav")
	if err := os.WriteFile(streamFile, []byte("RIFF....WAVE"), 0o644); err != nil {
		t.Fatalf("write stream file: %v", err)
	}

	buildModelInvocationBootstrap = func(_ context.Context, _ *service.FactoryServiceConfig) (modelBootstrapRunner, error) {
		return &stubModelBootstrapRunner{
			sessionReady: true,
			invokeModel: func(_ context.Context, modelName string, request factoryapi.ModelInvocationRequest) (apisurface.ModelInvocationResult, error) {
				return apisurface.ModelInvocationResult{
					ModelName:  modelName,
					Operation:  request.Operation,
					StreamFile: streamFile,
				}, nil
			},
		}, nil
	}

	outputPath := filepath.Join(t.TempDir(), "speech.wav")
	if err := Invoke(InvokeConfig{
		BuildInvocation: testModelInvocationBuilder,
		ModelName:       "OMNIVOICE_Q4_K_M",
		Operation:       "TTS",
		Text:            "hello world",
		OutputPath:      outputPath,
		FactoryDir:      t.TempDir(),
		Logger:          zap.NewNop(),
		Output:          io.Discard,
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
