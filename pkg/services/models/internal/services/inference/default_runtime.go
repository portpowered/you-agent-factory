package inference

import (
	"context"
	"strings"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

// InputEchoInvocationRuntime returns detached content derived from validated
// invocation input. It performs no network, process, or filesystem IO.
type InputEchoInvocationRuntime struct{}

var _ InvocationRuntime = InputEchoInvocationRuntime{}

func (InputEchoInvocationRuntime) Invoke(
	_ context.Context,
	request models.InvokeModelRequest,
) ([]models.InferenceContent, error) {
	contentType := strings.TrimSpace(request.Input.ContentType)
	if contentType == "" {
		contentType = "text/plain"
	}
	return []models.InferenceContent{{
		ContentType: contentType,
		Content:     request.Input.Content,
	}}, nil
}
