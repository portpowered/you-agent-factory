package inference

import (
	"context"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

// InputEchoInvocationRuntime returns detached content derived from validated
// invocation input. It performs no network, process, or filesystem IO.
type InputEchoInvocationRuntime struct{}

func (InputEchoInvocationRuntime) Invoke(
	_ context.Context,
	request InvocationRuntimeRequest,
) (InvocationRuntimeResult, error) {
	contentType := strings.TrimSpace(request.Request.Input.ContentType)
	if contentType == "" {
		contentType = "text/plain"
	}
	return InvocationRuntimeResult{
		Content: []models.InferenceContent{{
			ContentType: contentType,
			Content:     request.Request.Input.Content,
		}},
	}, nil
}
