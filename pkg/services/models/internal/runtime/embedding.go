package runtime

import (
	"context"
	"errors"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai/codecs"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
)

// EmbeddingBackend is the private effect used after the EMBED codec has
// normalized one generic Models request. Protocol values never cross the
// Models service boundary.
type EmbeddingBackend func(
	context.Context,
	codecs.EmbeddingRequest,
) (codecs.EmbeddingResponse, error)

type embedding struct {
	codec   codecs.EmbedCodec
	backend EmbeddingBackend
}

// NewEmbedding constructs the Models-owned EMBED invocation runtime.
func NewEmbedding(backend EmbeddingBackend) (embedding, error) {
	if backend == nil {
		return embedding{}, models.ErrInvalidInferenceDependencies
	}
	return embedding{codec: codecs.NewEmbedCodec(), backend: backend}, nil
}

func (runtime embedding) Invoke(
	ctx context.Context,
	request inference.InvocationRuntimeRequest,
) (inference.InvocationRuntimeResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if operation := strings.TrimSpace(request.Request.Operation); !strings.EqualFold(operation, models.OperationEMBED) {
		return inference.InvocationRuntimeResult{}, &models.InvocationFailure{
			Class:     models.InvocationFailureClassInvalidOperation,
			Operation: models.OperationEMBED,
			Message:   "EMBED runtime received an unsupported operation",
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
		return inference.InvocationRuntimeResult{}, &models.InvocationFailure{
			Class:     models.InvocationFailureClassBackendProtocol,
			Operation: models.OperationEMBED,
			Message:   "EMBED backend invocation failed",
			Cause:     models.ErrInferenceFailed,
		}
	}
	if err := ctx.Err(); err != nil {
		return inference.InvocationRuntimeResult{}, err
	}

	content, err := runtime.codec.DecodeResponseValue(backendResponse)
	if err != nil {
		return inference.InvocationRuntimeResult{}, err
	}
	return inference.InvocationRuntimeResult{Content: []models.InferenceContent{content}}, nil
}
