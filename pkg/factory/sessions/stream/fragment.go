package stream

import (
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/sessions/responsestream"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
)

func mapInferenceProgressFragment(fragment workerprovider.InferenceProgressFragment) responsestream.Event {
	kind := responsestream.EventKindProgressFragment
	switch fragment.Kind {
	case workerprovider.ResponseFragmentKind:
		kind = responsestream.EventKindResponseFragment
	case workerprovider.CompletedFragmentKind:
		kind = responsestream.EventKindStreamCompleted
	case workerprovider.FailedFragmentKind:
		kind = responsestream.EventKindStreamFailed
	}
	return responsestream.Event{
		Kind:               kind,
		Type:               responsestream.EventType(strings.TrimSpace(fragment.Type)),
		DispatchID:         strings.TrimSpace(fragment.DispatchID),
		ProviderSessionRef: interfaces.CloneProviderSessionMetadata(fragment.ProviderSessionRef),
		Payload:            fragment.Payload,
		ExternalEventType:  strings.TrimSpace(fragment.ExternalEventType),
		Metadata:           cloneStringMap(fragment.Metadata),
	}
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}
