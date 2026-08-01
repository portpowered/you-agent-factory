package stream

import (
	"strings"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// MapProgressFragment converts the shared Worker progress contract into the
// bounded internal response-stream vocabulary used by live and durable sessions.
func MapProgressFragment(fragment factorysessions.ProgressFragment) responsestream.Event {
	nativeType := strings.TrimSpace(fragment.Type)
	kind := responsestream.EventKindProgressFragment
	switch fragment.Kind {
	case factorysessions.ResponseFragmentKind:
		kind = responsestream.EventKindResponseFragment
	case factorysessions.CompletedFragmentKind:
		kind = responsestream.EventKindStreamCompleted
	case factorysessions.FailedFragmentKind:
		kind = responsestream.EventKindStreamFailed
	}
	eventType := responsestream.EventType(nativeType)
	if kind == responsestream.EventKindProgressFragment {
		// Providers reports native phase names (for example message.completed)
		// while the bounded response stream accepts only its small semantic enum.
		// Preserve the native value separately for canonical projection.
		eventType = responsestream.EventTypeProgress
	}
	return responsestream.Event{
		Kind:               kind,
		Type:               eventType,
		DispatchID:         strings.TrimSpace(fragment.DispatchID),
		ProviderSessionRef: workerexecution.CloneProviderSessionMetadata(fragment.ProviderSessionRef),
		Payload:            fragment.Payload,
		ExternalEventType:  firstNonEmpty(fragment.ExternalEventType, nativeType),
		Metadata:           cloneStringMap(fragment.Metadata),
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
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
