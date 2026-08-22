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
	joined := strings.Join(values, "\n")
	if request.Request.UsesGenericInvocationShape() && len(request.Operation.Outputs) > 0 {
		content := make([]models.InferenceContent, 0, len(request.Operation.Outputs))
		for _, output := range request.Operation.Outputs {
			outputType := genericOutputContentType(output)
			if strings.EqualFold(string(output.Modality), string(models.ModalityAudio)) && outputType == "" {
				outputType = "audio/wav"
			}
			content = append(content, models.InferenceContent{
				Name: output.Name, Modality: output.Modality,
				ContentType: outputType, MediaType: outputType, Content: joined,
			})
		}
		return InvocationRuntimeResult{Content: content}, nil
	}
	return InvocationRuntimeResult{
		Content: []models.InferenceContent{{
			ContentType: contentType,
			Content:     joined,
		}},
	}, nil
}

func genericOutputContentType(output models.OperationSlot) string {
	if len(output.MediaTypes) > 0 && strings.TrimSpace(output.MediaTypes[0]) != "" {
		return strings.TrimSpace(output.MediaTypes[0])
	}
	if len(output.ContentTypes) > 0 {
		if mediaType := genericContentTypeMediaType(output.ContentTypes[0]); mediaType != "" {
			return mediaType
		}
	}
	switch output.Modality {
	case models.ModalityJSON:
		return "application/json"
	case models.ModalityText:
		return "text/plain"
	case models.ModalityAudio:
		return "audio/wav"
	case models.ModalityImage:
		return "image/*"
	case models.ModalityVideo:
		return "video/*"
	default:
		return "application/octet-stream"
	}
}

func genericContentTypeMediaType(contentType string) string {
	switch strings.ToUpper(strings.TrimSpace(contentType)) {
	case "TEXT":
		return "text/plain"
	case "JSON":
		return "application/json"
	case "AUDIO":
		return "audio/wav"
	case "IMAGE":
		return "image/*"
	case "VIDEO":
		return "video/*"
	case "BINARY":
		return "application/octet-stream"
	default:
		if strings.Contains(strings.TrimSpace(contentType), "/") {
			return strings.TrimSpace(contentType)
		}
		return ""
	}
}
