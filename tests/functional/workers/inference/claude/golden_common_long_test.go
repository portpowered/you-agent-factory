//go:build functionallong

package claude

import (
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type claudeInvocationResultGoldenView struct {
	OK      bool   `json:"ok"`
	Content string `json:"content,omitempty"`
}

func claudeInvocationResultGolden(
	inferenceResponse factoryapi.InferenceResponseEventPayload,
) claudeInvocationResultGoldenView {
	content := ""
	if inferenceResponse.Response != nil {
		content = strings.TrimSpace(*inferenceResponse.Response)
	}
	return claudeInvocationResultGoldenView{
		OK:      inferenceResponse.Outcome == factoryapi.InferenceOutcomeSucceeded,
		Content: content,
	}
}

func successfulInferenceResponsePayload(
	t *testing.T,
	events []factoryapi.FactoryEvent,
) (factoryapi.InferenceResponseEventPayload, bool) {
	t.Helper()

	var payload factoryapi.InferenceResponseEventPayload
	found := false
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeModelResponse {
			continue
		}
		response, err := support.AsInferenceResponseObservation(event)
		if err != nil {
			t.Fatalf("decode INFERENCE_RESPONSE %q: %v", event.Id, err)
		}
		if response.Outcome != factoryapi.InferenceOutcomeSucceeded {
			continue
		}
		payload = response
		found = true
	}
	return payload, found
}
