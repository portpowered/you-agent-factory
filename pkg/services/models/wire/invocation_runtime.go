package wire

import (
	"context"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/models"
	localai "github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai"
	modelcodecs "github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai/codecs"
	modelsruntime "github.com/portpowered/infinite-you/pkg/services/models/internal/runtime"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
)

type backendInvocationRuntime struct {
	backend InvocationBackend
}

func (runtime backendInvocationRuntime) Invoke(
	ctx context.Context,
	request inference.InvocationRuntimeRequest,
) (inference.InvocationRuntimeResult, error) {
	content, artifacts, err := runtime.backend(ctx, request.Request)
	if err != nil {
		return inference.InvocationRuntimeResult{}, err
	}
	return inference.InvocationRuntimeResult{
		Content:   content,
		Artifacts: invocationArtifactSources(artifacts),
	}, nil
}

type operationInvocationRuntime struct {
	generic   invocationRuntime
	omni      invocationRuntime
	asr       invocationRuntime
	embedding invocationRuntime
	tts       invocationRuntime
}

func (runtime operationInvocationRuntime) Invoke(
	ctx context.Context,
	request inference.InvocationRuntimeRequest,
) (inference.InvocationRuntimeResult, error) {
	if runtime.asr != nil && isASROperation(request) {
		return runtime.asr.Invoke(
			localai.WithInvocationEndpoint(ctx, request.HostSlot.Endpoint), request,
		)
	}
	if runtime.omni != nil && isOMNIOperation(request) {
		return runtime.omni.Invoke(ctx, request)
	}
	if runtime.embedding != nil && isEmbeddingOperation(request) {
		return runtime.embedding.Invoke(
			localai.WithInvocationEndpoint(ctx, request.HostSlot.Endpoint), request,
		)
	}
	if isTTSOperation(request) {
		if runtime.tts == nil {
			return failClosedInvocationRuntime{}.Invoke(ctx, request)
		}
		return runtime.tts.Invoke(
			localai.WithInvocationEndpoint(ctx, request.HostSlot.Endpoint), request,
		)
	}
	return runtime.generic.Invoke(ctx, request)
}

func inferenceRuntime(options invocationRuntimeOptions) (invocationRuntime, error) {
	runtime := operationInvocationRuntime{
		generic: genericInvocationRuntime(options.Backend),
		omni:    newInvocationRuntime(options.Client, options.Dialer),
	}
	if err := configureASRRuntime(&runtime, options); err != nil {
		return nil, err
	}
	if err := configureEmbeddingRuntime(&runtime, options); err != nil {
		return nil, err
	}
	if err := configureTTSRuntime(&runtime, options); err != nil {
		return nil, err
	}
	return runtime, nil
}

func configureASRRuntime(runtime *operationInvocationRuntime, options invocationRuntimeOptions) error {
	backend := options.ASR
	// An explicitly injected generic backend is a complete controlled
	// operation effect. Keep it authoritative when a typed edge is absent;
	// pinned typed defaults are production fallbacks, not fixture overrides.
	if backend == nil && options.Backend == nil {
		backend = localai.NewPinnedASRBackend(
			options.Dialer, options.ASRTempDirectory, options.ASRCreateTemp,
			options.ASRWriteFile, options.ASRRemoveFile,
		)
	}
	if backend == nil {
		return nil
	}
	asr, err := newASRInvocationRuntime(backend)
	if err != nil {
		return err
	}
	runtime.asr = asr
	return nil
}

func configureEmbeddingRuntime(runtime *operationInvocationRuntime, options invocationRuntimeOptions) error {
	backend := options.Embedding
	if backend == nil && options.Backend == nil {
		backend = localai.NewPinnedEmbeddingBackend(options.Dialer)
	}
	if backend == nil {
		return nil
	}
	embedding, err := newEmbeddingInvocationRuntime(backend)
	if err != nil {
		return err
	}
	runtime.embedding = embedding
	return nil
}

func configureTTSRuntime(runtime *operationInvocationRuntime, options invocationRuntimeOptions) error {
	backend := localai.NewPinnedTTSBackend(
		options.Dialer,
		options.TTSTempDirectory,
		options.TTSCreateTemp,
		options.TTSInspectFile,
		options.TTSReadFile,
		options.TTSRemoveFile,
	)
	if backend == nil {
		return nil
	}
	tts, err := newTTSInvocationRuntime(backend)
	if err != nil {
		return err
	}
	runtime.tts = tts
	return nil
}

func genericInvocationRuntime(backend InvocationBackend) invocationRuntime {
	if backend == nil {
		return failClosedInvocationRuntime{}
	}
	return backendInvocationRuntime{backend: backend}
}

func newASRInvocationRuntime(backend ASRBackend) (invocationRuntime, error) {
	return modelsruntime.New(func(
		ctx context.Context,
		request modelcodecs.ASRRequest,
	) (modelcodecs.ASRResponse, []models.InferenceArtifact, error) {
		response, err := backend(ctx, models.ASRBackendRequest{
			Audio: append([]byte(nil), request.Audio...), MediaType: request.MediaType,
			Prompt: request.Prompt, Parameters: cloneInvocationParameters(request.Parameters),
		})
		if err != nil {
			return modelcodecs.ASRResponse{}, nil, err
		}
		segments := make([]modelcodecs.ASRSegment, len(response.Segments))
		for index, segment := range response.Segments {
			segments[index] = modelcodecs.ASRSegment{
				ID: segment.ID, Start: segment.Start, End: segment.End, Text: segment.Text,
			}
		}
		return modelcodecs.ASRResponse{Text: response.Text, Segments: segments}, response.Artifacts, nil
	})
}

func newEmbeddingInvocationRuntime(backend EmbeddingBackend) (invocationRuntime, error) {
	return modelsruntime.NewEmbedding(func(
		ctx context.Context,
		request modelcodecs.EmbeddingRequest,
	) (modelcodecs.EmbeddingResponse, error) {
		response, err := backend(ctx, models.EmbeddingBackendRequest{
			Text:       request.Prompt,
			Parameters: cloneInvocationParameters(request.Parameters),
		})
		if err != nil {
			return modelcodecs.EmbeddingResponse{}, err
		}
		return modelcodecs.EmbeddingResponse{
			Embeddings: append([]float64(nil), response.Embeddings...),
		}, nil
	})
}

func newTTSInvocationRuntime(
	backend func(context.Context, modelcodecs.TTSRequest) (modelcodecs.TTSResponse, error),
) (invocationRuntime, error) {
	return modelsruntime.NewTTS(modelsruntime.TTSBackend(backend))
}

type omniInvocationRuntime struct {
	codec    *localai.OmniCodec
	fallback invocationRuntime
}

// newInvocationRuntime keeps OMNI on the pinned protocol path. A missing
// client fails closed for OMNI, while non-OMNI operations also fail closed
// unless an explicit operation backend is composed.
func newInvocationRuntime(
	client InvocationProtocolClient,
	dialer InvocationProtocolDialer,
) invocationRuntime {
	fallback := failClosedInvocationRuntime{}
	if isNilDependency(client) {
		client = nil
	}
	var protocolClient localai.ProtocolClient
	if client != nil {
		protocolClient = invocationProtocolAdapter{client: client}
	} else if dialer != nil {
		protocolClient = localai.NewPinnedGRPCProtocolClient(dialer)
	}
	return omniInvocationRuntime{
		codec:    localai.NewPinnedOmniCodec(protocolClient),
		fallback: fallback,
	}
}

func (runtime omniInvocationRuntime) Invoke(
	ctx context.Context,
	request inference.InvocationRuntimeRequest,
) (inference.InvocationRuntimeResult, error) {
	if !isOMNIOperation(request) {
		return runtime.fallback.Invoke(ctx, request)
	}
	if runtime.codec == nil {
		return inference.InvocationRuntimeResult{}, models.ErrUnavailable
	}
	ctx = localai.WithInvocationEndpoint(ctx, request.HostSlot.Endpoint)
	omniResult, err := runtime.codec.Invoke(ctx, request.Request, request.Operation)
	if err != nil {
		return inference.InvocationRuntimeResult{}, err
	}
	return inference.InvocationRuntimeResult{
		Content:   omniResult.Content,
		Artifacts: invocationArtifactSources(omniResult.Artifacts),
	}, nil
}

type invocationProtocolAdapter struct {
	client InvocationProtocolClient
}

func (adapter invocationProtocolAdapter) Predict(
	ctx context.Context,
	request localai.PredictRequest,
) (localai.PredictResponse, error) {
	inputs := make([]models.InvocationProtocolInput, len(request.Inputs))
	for index, input := range request.Inputs {
		inputs[index] = models.InvocationProtocolInput{
			Slot: input.Slot, Modality: input.Modality, MediaType: input.MediaType,
			Content: input.Content, Reference: input.Reference,
		}
	}
	response, err := adapter.client.Predict(ctx, models.InvocationProtocolRequest{
		Operation: models.OperationOMNI,
		Prompt:    request.Prompt, Inputs: inputs, Parameters: request.Parameters,
	})
	if err != nil {
		return localai.PredictResponse{}, err
	}
	return localai.PredictResponse{Text: response.Text, Usage: response.Usage}, nil
}

func isOMNIOperation(request inference.InvocationRuntimeRequest) bool {
	return invocationOperationName(request) == models.OperationOMNI
}

func isASROperation(request inference.InvocationRuntimeRequest) bool {
	return invocationOperationName(request) == models.OperationASR
}

func isEmbeddingOperation(request inference.InvocationRuntimeRequest) bool {
	return invocationOperationName(request) == models.OperationEMBED
}

func isTTSOperation(request inference.InvocationRuntimeRequest) bool {
	return invocationOperationName(request) == models.OperationTTS
}

func invocationOperationName(request inference.InvocationRuntimeRequest) string {
	operation := request.Operation.Name
	if operation == "" {
		operation = request.Request.Operation
	}
	return strings.ToUpper(strings.TrimSpace(operation))
}

func cloneInvocationParameters(parameters map[string]any) map[string]any {
	if parameters == nil {
		return nil
	}
	cloned := make(map[string]any, len(parameters))
	for name, value := range parameters {
		cloned[name] = cloneInvocationParameterValue(value)
	}
	return cloned
}

func cloneInvocationParameterValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneInvocationParameters(typed)
	case []any:
		cloned := make([]any, len(typed))
		for index, item := range typed {
			cloned[index] = cloneInvocationParameterValue(item)
		}
		return cloned
	default:
		return value
	}
}
