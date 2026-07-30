package support

import (
	"fmt"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// AsInferenceResponseObservation exposes the provider-boundary fields shared by
// legacy INFERENCE_RESPONSE and current MODEL_RESPONSE events. Functional
// golden tests care about the public outcome, text, diagnostics, failure, and
// Provider Session rather than which internal worker generation produced it.
func AsInferenceResponseObservation(event factoryapi.FactoryEvent) (factoryapi.InferenceResponseEventPayload, error) {
	if event.Type == factoryapi.FactoryEventTypeInferenceResponse {
		return event.Payload.AsInferenceResponseEventPayload()
	}
	if event.Type != factoryapi.FactoryEventTypeModelResponse {
		return factoryapi.InferenceResponseEventPayload{}, fmt.Errorf("event type %q is not a provider response", event.Type)
	}

	modelResponse, err := event.Payload.AsModelResponseEventPayload()
	if err != nil {
		return factoryapi.InferenceResponseEventPayload{}, err
	}
	response := modelResponse.OutputPreview
	if response == nil && modelResponse.OutputContent != nil {
		var textParts []string
		for _, part := range *modelResponse.OutputContent {
			textPart, textErr := part.AsWorkTextContentPart()
			if textErr == nil {
				textParts = append(textParts, textPart.Text)
			}
		}
		if len(textParts) > 0 {
			joined := strings.Join(textParts, "")
			response = &joined
		}
	}
	return factoryapi.InferenceResponseEventPayload{
		Attempt:            modelResponse.Attempt,
		Diagnostics:        modelResponse.Diagnostics,
		DurationMillis:     modelResponse.DurationMillis,
		FailureDetail:      modelResponse.FailureDetail,
		InferenceRequestId: modelResponse.ModelRequestId,
		Outcome:            modelResponse.Outcome,
		ProviderSession:    modelResponse.ProviderSession,
		Response:           response,
	}, nil
}
