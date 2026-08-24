package inference

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestRunnerInvokesModelsRootGenericOperationExactlyOnce(t *testing.T) {
	modelsEdge := &captureModelsService{
		result: models.InvokeModelResult{
			Status: models.ModelInvocationStatusCompleted,
			Outputs: []models.InferenceOutput{{
				Name:        "audio",
				Modality:    models.ModalityAudio,
				ContentType: "audio/wav",
				MediaType:   "audio/wav",
				Content:     "fixture-audio-bytes",
			}},
		},
	}
	runner := newTestRunner(t, modelsEdge, nil)
	request := validRequest()
	request.ModelOperation = models.OperationTTS
	request.ModelBindings = []workers.ResolvedModelOperationBinding{
		{
			Slot:   "text",
			Source: workers.ModelOperationBindingSourceInput,
			Content: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeText,
				Text: "Read this in order.",
			}},
		},
		{
			Slot:   "voice",
			Source: workers.ModelOperationBindingSourceInput,
			Content: []work.WorkContentPart{{
				Type:        work.WorkContentPartTypeAudio,
				URL:         "data:audio/wav;base64,dm9pY2U=",
				ContentType: "audio/wav",
			}},
		},
		{
			Slot:   "parameters",
			Source: workers.ModelOperationBindingSourceConfig,
			Content: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeJSON,
				JSON: []byte(`{"speed":1,"pitch":0.2}`),
			}},
		},
	}

	result, err := runner.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := modelsEdge.Calls(); got != 1 {
		t.Fatalf("Models InvokeModel calls = %d, want exactly one", got)
	}
	if got := modelsEdge.LegacyCalls(); got != 0 {
		t.Fatalf("Models InvokeLocal calls = %d, want zero", got)
	}
	captured := modelsEdge.Request()
	if captured.Model.NameOrURI != "WHISPER" || captured.Operation != models.OperationTTS {
		t.Fatalf("generic model selection = %#v, want WHISPER/TTS", captured)
	}
	if captured.Holder != "dispatch-1" {
		t.Fatalf("generic invocation holder = %q, want dispatch-1", captured.Holder)
	}
	if captured.Scope.String() != "factory-session:tts" {
		t.Fatalf("generic invocation scope = %q, want factory-session:tts", captured.Scope.String())
	}
	if captured.OutputMode != models.OutputModeAuto {
		t.Fatalf("generic invocation output mode = %q, want AUTO", captured.OutputMode)
	}
	if len(captured.Inputs) != 2 || captured.Inputs[0].Name != "text" ||
		captured.Inputs[0].Content != "Read this in order." || captured.Inputs[1].Name != "voice" {
		t.Fatalf("ordered generic inputs = %#v", captured.Inputs)
	}
	if len(captured.Parameters) != 2 || captured.Parameters[0].Name != "speed" ||
		captured.Parameters[1].Name != "pitch" {
		t.Fatalf("ordered generic parameters = %#v", captured.Parameters)
	}
	if result.Outcome != workers.OutcomeAccepted || result.ProposedOutput == nil ||
		len(result.ProposedOutput.Primary) != 1 {
		t.Fatalf("runner result = %#v, want accepted proposed audio", result)
	}
	audio := result.ProposedOutput.Primary[0]
	if audio.Type != work.WorkContentPartTypeAudio || audio.ContentType != "audio/wav" ||
		!strings.HasPrefix(audio.URL, "data:audio/wav;base64,") {
		t.Fatalf("audio Work proposal = %#v", audio)
	}
	decoded, decodeErr := base64.StdEncoding.DecodeString(strings.TrimPrefix(audio.URL, "data:audio/wav;base64,"))
	if decodeErr != nil || string(decoded) != "fixture-audio-bytes" {
		t.Fatalf("audio data URL payload = %q, decode error = %v", decoded, decodeErr)
	}
}

func TestRunnerPreservesModelsOwnedOutputURLsAndArtifacts(t *testing.T) {
	artifact, err := (models.InferenceArtifactRef{}).Parse("models-inference:artifact:audio")
	if err != nil {
		t.Fatalf("Parse artifact = %v", err)
	}
	modelsEdge := &captureModelsService{result: models.InvokeModelResult{
		Status: models.ModelInvocationStatusCompleted,
		Outputs: []models.InferenceOutput{{
			Name:     "audio",
			Modality: models.ModalityAudio,
			Content:  "file:///runtime/audio.wav",
			Artifact: &models.InferenceArtifact{Artifact: artifact, Name: "speech.wav"},
		}},
	}}
	runner := newTestRunner(t, modelsEdge, nil)
	result, err := runner.Execute(context.Background(), validRequest())
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	part := result.ProposedOutput.Primary[0]
	if part.URL != "file:///runtime/audio.wav" || part.ArtifactID != artifact.String() {
		t.Fatalf("preserved audio output = %#v", part)
	}
	if len(result.ProposedOutput.ArtifactRefs) != 1 ||
		result.ProposedOutput.ArtifactRefs[0].ArtifactID != artifact.String() {
		t.Fatalf("artifact refs = %#v", result.ProposedOutput.ArtifactRefs)
	}
}

func TestRunnerNormalizesModelsFailureWithoutSuccessfulOutput(t *testing.T) {
	wantErr := errors.New("runtime failed")
	modelsEdge := &captureModelsService{err: wantErr}
	runner := newTestRunner(t, modelsEdge, nil)
	result, err := runner.Execute(context.Background(), validRequest())
	if !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want Models failure %v", err, wantErr)
	}
	if result.ProposedOutput != nil || result.Content != "" {
		t.Fatalf("failed result = %#v, want no successful output", result)
	}
	if modelsEdge.Calls() != 1 || modelsEdge.LegacyCalls() != 0 {
		t.Fatalf("Models call counts = generic %d legacy %d", modelsEdge.Calls(), modelsEdge.LegacyCalls())
	}
}

func TestRunnerDelegatesNonManagedInferenceWithoutModelsInvocation(t *testing.T) {
	modelsEdge := &captureModelsService{}
	delegate := &captureDelegateRunner{result: workers.RunnerExecutionResult{Content: "provider output"}}
	runner := newTestRunner(t, modelsEdge, delegate)
	request := validRequest()
	request.ModelLocality = models.RuntimeModelLocalityCloud
	request.ModelProvider = workers.RunnerIDCodex
	result, err := runner.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Content != "provider output" || delegate.Calls() != 1 {
		t.Fatalf("delegate result = %#v, calls = %d", result, delegate.Calls())
	}
	if modelsEdge.Calls() != 0 || modelsEdge.LegacyCalls() != 0 {
		t.Fatalf("Models calls = generic %d legacy %d, want zero", modelsEdge.Calls(), modelsEdge.LegacyCalls())
	}
	if delegate.Request().RunnerID != workers.RunnerIDCodex {
		t.Fatalf("delegate runner id = %q, want codex", delegate.Request().RunnerID)
	}
}

func TestRunnerRejectsInvalidRequestsBeforeModelsInvocation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*workers.RunnerExecutionRequest)
	}{
		{name: "wrong runner", mutate: func(request *workers.RunnerExecutionRequest) { request.RunnerID = "script" }},
		{name: "missing operation", mutate: func(request *workers.RunnerExecutionRequest) { request.ModelOperation = "" }},
		{name: "invalid binding", mutate: func(request *workers.RunnerExecutionRequest) {
			request.ModelBindings[0].Slot = ""
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			modelsEdge := &captureModelsService{}
			runner := newTestRunner(t, modelsEdge, nil)
			request := validRequest()
			test.mutate(&request)
			_, err := runner.Execute(context.Background(), request)
			assertFailureType(t, err, workers.WorkFailureTypePermanentBadRequest)
			if modelsEdge.Calls() != 0 {
				t.Fatalf("Models calls = %d, want zero", modelsEdge.Calls())
			}
		})
	}
}

func TestRunnerPreservesPreCanceledContextWithoutCallingModels(t *testing.T) {
	modelsEdge := &captureModelsService{}
	runner := newTestRunner(t, modelsEdge, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := runner.Execute(ctx, validRequest())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() error = %v, want context.Canceled", err)
	}
	if modelsEdge.Calls() != 0 {
		t.Fatalf("Models calls = %d, want zero", modelsEdge.Calls())
	}
}

func TestNewRejectsMissingModelsAndManagedModel(t *testing.T) {
	if _, err := New(validConfig(), Dependencies{}); err == nil {
		t.Fatal("New() with nil Models returned nil error")
	}
	if _, err := New(Config{Worker: models.LocalWorker{
		Name: "worker", Type: interfaces.WorkerTypeInference,
		ModelLocality: models.RuntimeModelLocalityLocal,
	}}, Dependencies{Models: &captureModelsService{}}); err == nil {
		t.Fatal("New() with empty managed model returned nil error")
	}
}

func newTestRunner(t *testing.T, modelsEdge *captureModelsService, delegate workers.Runner) workers.Runner {
	t.Helper()
	runner, err := New(validConfig(), Dependencies{Models: modelsEdge, Delegate: delegate})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return runner
}

func validConfig() Config {
	scope, err := (models.RuntimeScopeRef{}).Parse("factory-session:tts")
	if err != nil {
		panic(err)
	}
	return Config{Worker: models.LocalWorker{
		Name:          "tts-worker",
		Type:          interfaces.WorkerTypeInference,
		Model:         "WHISPER",
		ModelLocality: models.RuntimeModelLocalityLocal,
	}, Scope: scope}
}

func validRequest() workers.RunnerExecutionRequest {
	return workers.RunnerExecutionRequest{
		RunnerID:       Identity,
		WorkerType:     interfaces.WorkerTypeInference,
		ModelOperation: models.OperationTTS,
		ModelBindings: []workers.ResolvedModelOperationBinding{{
			Slot:   "text",
			Source: workers.ModelOperationBindingSourceInput,
			Content: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeText,
				Text: "hello",
			}},
		}},
		Dispatch: work.WorkDispatch{
			DispatchID:      "dispatch-1",
			WorkstationName: "tts-workstation",
		},
	}
}

type captureModelsService struct {
	mu         sync.Mutex
	request    models.InvokeModelRequest
	result     models.InvokeModelResult
	err        error
	calls      int
	legacyCall atomic.Int32
}

func (service *captureModelsService) InvokeModel(
	_ context.Context,
	request models.InvokeModelRequest,
) (models.InvokeModelResult, error) {
	service.mu.Lock()
	service.calls++
	service.request = cloneInvokeModelRequest(request)
	result := service.result.Clone()
	err := service.err
	service.mu.Unlock()
	return result, err
}

// InvokeLocal is intentionally present only as a tripwire: the V4 runner must
// never use the retired specialized Models edge.
func (service *captureModelsService) InvokeLocal(
	context.Context,
	models.LocalInvocationRequest,
) (models.LocalInvocationResult, error) {
	service.legacyCall.Add(1)
	return models.LocalInvocationResult{}, errors.New("retired InvokeLocal edge was called")
}

func (service *captureModelsService) Request() models.InvokeModelRequest {
	service.mu.Lock()
	defer service.mu.Unlock()
	return cloneInvokeModelRequest(service.request)
}

func (service *captureModelsService) Calls() int {
	service.mu.Lock()
	defer service.mu.Unlock()
	return service.calls
}

func (service *captureModelsService) LegacyCalls() int {
	return int(service.legacyCall.Load())
}

func cloneInvokeModelRequest(request models.InvokeModelRequest) models.InvokeModelRequest {
	clone := request
	if request.Inputs != nil {
		clone.Inputs = make([]models.InferenceInput, len(request.Inputs))
		for index, input := range request.Inputs {
			clone.Inputs[index] = input.Clone()
		}
	}
	if request.Parameters != nil {
		clone.Parameters = make([]models.OperationParameter, len(request.Parameters))
		for index, parameter := range request.Parameters {
			clone.Parameters[index] = parameter.Clone()
		}
	}
	return clone
}

type captureDelegateRunner struct {
	mu      sync.Mutex
	request workers.RunnerExecutionRequest
	result  workers.RunnerExecutionResult
	calls   int
}

func (runner *captureDelegateRunner) Execute(
	_ context.Context,
	request workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	runner.calls++
	runner.request = workers.CloneProviderInferenceRequest(request)
	return runner.result, nil
}

func (runner *captureDelegateRunner) Calls() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.calls
}

func (runner *captureDelegateRunner) Request() workers.RunnerExecutionRequest {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return workers.CloneProviderInferenceRequest(runner.request)
}

func assertFailureType(t *testing.T, err error, want workers.WorkFailureType) {
	t.Helper()
	var failure *workers.ProviderError
	if !errors.As(err, &failure) || failure.Type != want {
		t.Fatalf("error = %#v, want ProviderError type %q", err, want)
	}
}
