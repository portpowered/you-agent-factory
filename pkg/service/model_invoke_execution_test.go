package service

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/workers"
)

// backendsizecheck:ignore-function this dual-boundary integration test keeps the full model-invoke execution assertion path in one place.
func TestLoadWorkersFromConfig_ModelInvokeWorkstationExecutesThroughSeparatedMockedRuntimeEdges(t *testing.T) {
	t.Parallel()

	t.Run("local managed runtime boundary", func(t *testing.T) {
		audioPath := filepath.Join(t.TempDir(), "speech.wav")
		provider := &providerCallRecorder{}
		runtime := &fakeLocalModelRuntime{
			response: interfaces.InferenceResponse{
				Content: mustMarshalAudioContentResponse(t, audioPath),
			},
		}

		wsExec := modelInvokeWorkstationExecutorForLocalManagedRuntime(t, runtime, provider)
		result, err := wsExec.Execute(context.Background(), modelInvokeDispatch())
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if result.Outcome != interfaces.OutcomeAccepted {
			t.Fatalf("outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
		}
		assertModelInvokeAcceptedAudioOutput(t, result.Output, audioPath)
		if runtime.invocationCount() != 1 {
			t.Fatalf("managed runtime invocation count = %d, want 1", runtime.invocationCount())
		}
		if calls := provider.Calls(); len(calls) != 0 {
			t.Fatalf("provider calls = %#v, want local managed runtime to bypass provider path", calls)
		}
		assertManagedRuntimeModelInvokeInvocation(t, runtime.invocationRequests(), interfaces.ModelLocalityLocal)
	})

	t.Run("cloud provider edge", func(t *testing.T) {
		provider, wsExec := modelInvokeExecutionFixture(t, "gpt-4o-mini-tts", interfaces.ModelLocalityCloud)
		result, err := wsExec.Execute(context.Background(), modelInvokeDispatch())
		if err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if result.Outcome != interfaces.OutcomeAccepted {
			t.Fatalf("outcome = %s, want %s", result.Outcome, interfaces.OutcomeAccepted)
		}
		if result.Output != "audio-ready" {
			t.Fatalf("output = %q, want accepted provider response content", result.Output)
		}
		assertModelInvokeProviderCall(t, provider.Calls(), "gpt-4o-mini-tts", interfaces.ModelLocalityCloud)
	})
}

func modelInvokeWorkstationExecutorForLocalManagedRuntime(
	t *testing.T,
	runtime *fakeLocalModelRuntime,
	provider *providerCallRecorder,
) *workers.WorkstationExecutor {
	t.Helper()

	factoryCfg := localModelFactoryConfig()
	cache := localModelTestCacheLayout(t)
	runtimeCfg := newLoadedFactoryConfigForServiceTest(t, "", factoryCfg, map[string]*interfaces.WorkerConfig{
		"tts-worker": modelInvokeLocalManagedRuntimeWorker(),
	}, map[string]*interfaces.FactoryWorkstationConfig{
		"speak": modelInvokeWorkstationConfig(),
	})

	opts, err := loadWorkersFromConfig(
		"",
		factoryCfg,
		"",
		runtimeCfg,
		nil,
		logging.NoopLogger{},
		true,
		provider,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		newLocalModelResourceLimiter(),
		newManagedLocalModelManager(staticModelAssetPuller{cache: cache}, runtime),
	)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}
	exec, ok := fc.WorkerExecutors["tts-worker"]
	if !ok {
		t.Fatal("expected tts-worker executor to be registered")
	}
	wsExec, ok := exec.(*workers.WorkstationExecutor)
	if !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}
	return wsExec
}

func modelInvokeLocalManagedRuntimeWorker() *interfaces.WorkerConfig {
	worker := modelInvokeRuntimeWorker("OMNIVOICE_Q4_K_M", interfaces.ModelLocalityLocal)
	worker.Resources = []interfaces.ResourceConfig{{
		Name:     "omnivoice-cache",
		Capacity: 1,
	}}
	return worker
}

func (r *fakeLocalModelRuntime) invocationRequests() []localModelInvocationRequest {
	r.mu.Lock()
	defer r.mu.Unlock()

	requests := make([]localModelInvocationRequest, len(r.invocations))
	copy(requests, r.invocations)
	return requests
}

func assertManagedRuntimeModelInvokeInvocation(
	t *testing.T,
	invocations []localModelInvocationRequest,
	wantLocality string,
) {
	t.Helper()

	if len(invocations) != 1 {
		t.Fatalf("managed runtime invocations = %d, want 1", len(invocations))
	}
	request := invocations[0].Request
	if request.ModelOperation != "TTS" {
		t.Fatalf("managed runtime model operation = %q, want TTS", request.ModelOperation)
	}
	if request.ModelLocality != wantLocality {
		t.Fatalf("managed runtime model locality = %q, want %q", request.ModelLocality, wantLocality)
	}
	if request.Model != "OMNIVOICE_Q4_K_M" {
		t.Fatalf("managed runtime model = %q, want OMNIVOICE_Q4_K_M", request.Model)
	}
	if len(request.ModelBindings) != 2 {
		t.Fatalf("managed runtime bindings = %#v, want 2 entries", request.ModelBindings)
	}
	assertModelInvokeTextBinding(t, request.ModelBindings[0])
	assertModelInvokeVoiceBinding(t, request.ModelBindings[1])
}

func assertModelInvokeAcceptedAudioOutput(t *testing.T, output string, audioPath string) {
	t.Helper()

	var content []interfaces.WorkContentPart
	if err := json.Unmarshal([]byte(output), &content); err != nil {
		t.Fatalf("decode accepted model invoke output: %v", err)
	}
	if len(content) != 1 || content[0].Type != interfaces.WorkContentPartTypeAudio || content[0].File != audioPath {
		t.Fatalf("accepted output = %#v, want one audio part at %q", content, audioPath)
	}
}
