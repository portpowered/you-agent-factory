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
	inputs := request.Request.Inputs
	if len(inputs) == 0 {
		inputs = []models.InferenceInput{request.Request.Input}
	}
	contentType := strings.TrimSpace(inputs[0].ContentType)
	if contentType == "" {
		contentType = "text/plain"
	}
	if request.Request.UsesGenericInvocationShape() {
		switch strings.ToUpper(strings.TrimSpace(request.Request.Operation)) {
		case models.OperationTTS:
			contentType = "audio/wav"
		case models.OperationEMBED:
			contentType = "application/json"
		case models.OperationOMNI:
			contentType = "text/plain"
		}
	}
	values := make([]string, 0, len(inputs))
	for _, input := range inputs {
		values = append(values, input.Content)
	}
	return InvocationRuntimeResult{
		Content: []models.InferenceContent{{
			ContentType: contentType,
			Content:     strings.Join(values, "\n"),
		}},
	}, nil
}
