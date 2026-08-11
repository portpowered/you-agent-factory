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
	if kind == responsestream.EventKindProgressFragment && !preservesProgressPhase(fragment) {
		// MESSAGE progress is coalesced into one terminal customer-facing
		// snapshot by the runner. Other generic progress remains bounded too.
		// REASONING and SESSION metadata need their declared phases below so
		// they can reach their canonical customer-facing projections.
		eventType = responsestream.EventTypeProgress
	}
	return responsestream.Event{
		Kind:               kind,
		Type:               eventType,
		DispatchID:         strings.TrimSpace(fragment.DispatchID),
		Provider:           strings.TrimSpace(fragment.Provider),
		ProviderSessionRef: workerexecution.CloneProviderSessionMetadata(fragment.ProviderSessionRef),
		Payload:            fragment.Payload,
		ExternalEventType:  firstNonEmpty(fragment.ExternalEventType, nativeType),
		Metadata:           cloneStringMap(fragment.Metadata),
	}
}

func preservesProgressPhase(fragment factorysessions.ProgressFragment) bool {
	switch strings.ToLower(strings.TrimSpace(fragment.Metadata["kind"])) {
	case "reasoning":
		switch strings.ToLower(strings.TrimSpace(fragment.Type)) {
		case "started", "start", "delta", "completed", "complete":
			return true
		}
	case "session":
		return strings.EqualFold(strings.TrimSpace(fragment.Type), "updated")
	default:
	}
	return false
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
