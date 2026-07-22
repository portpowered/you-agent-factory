package stream

import (
	"strings"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
)

func mapInferenceProgressFragment(fragment factorysessions.ProgressFragment) responsestream.Event {
	kind := responsestream.EventKindProgressFragment
	switch fragment.Kind {
	case factorysessions.ResponseFragmentKind:
		kind = responsestream.EventKindResponseFragment
	case factorysessions.CompletedFragmentKind:
		kind = responsestream.EventKindStreamCompleted
	case factorysessions.FailedFragmentKind:
		kind = responsestream.EventKindStreamFailed
	}
	return responsestream.Event{
		Kind:               kind,
		Type:               responsestream.EventType(strings.TrimSpace(fragment.Type)),
		DispatchID:         strings.TrimSpace(fragment.DispatchID),
		ProviderSessionRef: providersessions.CloneMetadata(fragment.ProviderSessionRef),
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
