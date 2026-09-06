package runtime

import (
	"context"
	"errors"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai/codecs"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
)

// TTSBackend is the private effect used after the TTS codec has normalized one
// generic Models request. Provider protocol values stay behind the LocalAI
// adapter and never cross the Models service boundary.
type TTSBackend func(
	context.Context,
	codecs.TTSRequest,
) (codecs.TTSResponse, error)

type tts struct {
	codec   codecs.TTSCodec
	backend TTSBackend
}

// NewTTS constructs the Models-owned TTS invocation runtime.
func NewTTS(backend TTSBackend) (tts, error) {
	if backend == nil {
		return tts{}, models.ErrInvalidInferenceDependencies
	}
	return tts{codec: codecs.NewTTSCodec(), backend: backend}, nil
}

func (runtime tts) Invoke(
	ctx context.Context,
	request inference.InvocationRuntimeRequest,
) (inference.InvocationRuntimeResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if operation := invocationOperation(request); !strings.EqualFold(operation, models.OperationTTS) {
		return inference.InvocationRuntimeResult{}, &models.InvocationFailure{
			Class:     models.InvocationFailureClassInvalidOperation,
			Operation: models.OperationTTS,
			Message:   "TTS runtime received an unsupported operation",
		}
	}
	if err := ctx.Err(); err != nil {
		return inference.InvocationRuntimeResult{}, err
	}

	backendRequest, err := runtime.codec.EncodeRequest(request.Request)
	if err != nil {
		return inference.InvocationRuntimeResult{}, err
	}
	backendResponse, err := runtime.backend(ctx, backendRequest)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return inference.InvocationRuntimeResult{}, err
		}
		var failure *models.InvocationFailure
		if errors.As(err, &failure) {
			return inference.InvocationRuntimeResult{}, err
		}
		return inference.InvocationRuntimeResult{}, &models.InvocationFailure{
			Class:     models.InvocationFailureClassBackendProtocol,
			Operation: models.OperationTTS,
			Message:   "TTS backend invocation failed",
			Cause:     models.ErrInferenceFailed,
		}
	}
	if err := ctx.Err(); err != nil {
		return inference.InvocationRuntimeResult{}, err
	}

	content, err := runtime.codec.DecodeResponse(backendResponse)
	if err != nil {
		return inference.InvocationRuntimeResult{}, err
	}
	return inference.InvocationRuntimeResult{
		Content: []models.InferenceContent{content},
	}, nil
}

func invocationOperation(request inference.InvocationRuntimeRequest) string {
	operation := strings.TrimSpace(request.Request.Operation)
	if operation == "" {
		operation = strings.TrimSpace(request.Operation.Name)
	}
	return operation
}
