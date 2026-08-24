package inference

import (
	"context"
	"errors"
	"reflect"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai/codecs"
)

// ASRBackend is the narrow protocol effect required by the ASR runtime. A
// backend may be a LocalAI adapter or a deterministic fixture, but it cannot
// publish provider-specific values through the Models service.
type ASRBackend interface {
	Transcribe(context.Context, codecs.ASRRequest) (codecs.ASRResponse, error)
}

// ASRInvocationRuntime maps one generic ASR invocation through the private
// codec and an injected protocol backend.
type ASRInvocationRuntime struct {
	codec   codecs.ASRCodec
	backend ASRBackend
}

// NewASRInvocationRuntime constructs an ASR runtime with an explicit backend
// dependency.
func NewASRInvocationRuntime(backend ASRBackend) (ASRInvocationRuntime, error) {
	if backend == nil || isNilASRBackend(backend) {
		return ASRInvocationRuntime{}, models.ErrInvalidInferenceDependencies
	}
	return ASRInvocationRuntime{codec: codecs.NewASRCodec(), backend: backend}, nil
}

// Invoke implements the Models inference runtime seam.
func (runtime ASRInvocationRuntime) Invoke(
	ctx context.Context,
	request InvocationRuntimeRequest,
) (InvocationRuntimeResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if operation := strings.TrimSpace(request.Request.Operation); !strings.EqualFold(operation, models.OperationASR) {
		return InvocationRuntimeResult{}, &models.InvocationFailure{
			Class: models.InvocationFailureClassInvalidOperation, Operation: models.OperationASR,
			Message: "ASR runtime received an unsupported operation",
		}
	}
	if err := ctx.Err(); err != nil {
		return InvocationRuntimeResult{}, err
	}

	backendRequest, err := runtime.codec.EncodeRequest(request.Request)
	if err != nil {
		return InvocationRuntimeResult{}, err
	}
	backendResponse, err := runtime.backend.Transcribe(ctx, backendRequest)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return InvocationRuntimeResult{}, err
		}
		return InvocationRuntimeResult{}, &models.InvocationFailure{
			Class: models.InvocationFailureClassBackendProtocol, Operation: models.OperationASR,
			Message: "ASR backend invocation failed", Cause: models.ErrInferenceFailed,
		}
	}
	if err := ctx.Err(); err != nil {
		return InvocationRuntimeResult{}, err
	}
	content, err := runtime.codec.DecodeResponseValue(backendResponse)
	if err != nil {
		return InvocationRuntimeResult{}, err
	}
	return InvocationRuntimeResult{Content: content}, nil
}

func isNilASRBackend(backend ASRBackend) bool {
	value := reflect.ValueOf(backend)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
