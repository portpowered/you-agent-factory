package compat

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/factorysessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/factorysessions/responsestream"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

// ErrUnsupportedFragmentKind indicates the mapper does not yet handle the
// supplied legacy response-stream fragment kind.
var ErrUnsupportedFragmentKind = errors.New("unsupported legacy fragment kind")

// Context carries correlation fields required on canonical response events but
// absent from legacy response-stream fragments.
type Context struct {
	FactorySessionID string
	RunID            string
}

// MapFragment converts one legacy response-stream event into canonical
// FactoryResponseEvent values. Progress fragments are supported in this lane;
// other kinds return ErrUnsupportedFragmentKind until later stories land.
func MapFragment(ctx Context, fragment responsestream.Event) ([]responseevents.FactoryResponseEvent, error) {
	switch fragment.Kind {
	case responsestream.EventKindProgressFragment:
		event, err := mapProgressFragment(ctx, fragment)
		if err != nil {
			return nil, err
		}
		if err := responseevents.ValidateEvent(event); err != nil {
			return nil, fmt.Errorf("mapped progress event invalid: %w", err)
		}
		return []responseevents.FactoryResponseEvent{event}, nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedFragmentKind, fragment.Kind)
	}
}

func mapProgressFragment(ctx Context, fragment responsestream.Event) (responseevents.FactoryResponseEvent, error) {
	payload, err := json.Marshal(progressPayloadFromFragment(fragment))
	if err != nil {
		return responseevents.FactoryResponseEvent{}, fmt.Errorf("marshal progress payload: %w", err)
	}

	return responseevents.FactoryResponseEvent{
		SchemaVersion:    responseevents.SchemaVersionV1,
		EventID:          synthesizedEventID(ctx, fragment),
		Sequence:         fragment.Sequence,
		RecordedAt:       fragment.RecordedAt,
		FactorySessionID: strings.TrimSpace(ctx.FactorySessionID),
		RunID:            strings.TrimSpace(ctx.RunID),
		Kind:             responseevents.KindProgress,
		Phase:            responseevents.PhaseUpdated,
		Provenance: responseevents.Provenance{
			Provider:        fragmentProvider(fragment),
			NativeEventType: fragmentNativeEventType(fragment),
			Delivery:        responseevents.DeliverySynthesized,
			Representation:  responseevents.RepresentationNotification,
			Fidelity:        progressFragmentFidelity(fragment),
		},
		Payload:            payload,
		DispatchID:         strings.TrimSpace(fragment.DispatchID),
		ProviderSessionRef: providerSessionRefString(fragment.ProviderSessionRef),
	}, nil
}

func progressPayloadFromFragment(fragment responsestream.Event) responseevents.ProgressPayload {
	label := progressLabel(fragment)
	message := strings.TrimSpace(fragment.Payload)
	if message == "" {
		return responseevents.ProgressPayload{Label: label}
	}
	return responseevents.ProgressPayload{
		Label:   label,
		Message: message,
	}
}

func progressLabel(fragment responsestream.Event) string {
	if typed := strings.TrimSpace(string(fragment.Type)); typed != "" {
		return typed
	}
	return "PROGRESS"
}

func progressFragmentFidelity(fragment responsestream.Event) responseevents.Fidelity {
	if fragmentPayloadTruncated(fragment.Metadata) {
		return responseevents.FidelityLossy
	}
	return responseevents.FidelityNormalized
}

func fragmentPayloadTruncated(metadata map[string]string) bool {
	if len(metadata) == 0 {
		return false
	}
	value, ok := metadata["payload_truncated"]
	return ok && strings.EqualFold(strings.TrimSpace(value), "true")
}

func fragmentProvider(fragment responsestream.Event) string {
	if fragment.ProviderSessionRef != nil {
		if provider := interfaces.CanonicalProviderSessionProvider(fragment.ProviderSessionRef.Provider); provider != "" {
			return provider
		}
	}
	if fragment.Metadata != nil {
		if runner := strings.TrimSpace(fragment.Metadata["runner_id"]); runner != "" {
			return runner
		}
	}
	return "legacy-fragment"
}

func fragmentNativeEventType(fragment responsestream.Event) string {
	if external := strings.TrimSpace(fragment.ExternalEventType); external != "" {
		return external
	}
	if typed := strings.TrimSpace(string(fragment.Type)); typed != "" {
		return typed
	}
	return string(fragment.Kind)
}

func providerSessionRefString(session *interfaces.ProviderSessionMetadata) string {
	if session == nil {
		return ""
	}
	return strings.TrimSpace(session.ID)
}

func synthesizedEventID(ctx Context, fragment responsestream.Event) string {
	material := fmt.Sprintf(
		"%s|%s|%d|%s|%s",
		strings.TrimSpace(ctx.FactorySessionID),
		strings.TrimSpace(ctx.RunID),
		fragment.Sequence,
		fragment.Kind,
		strings.TrimSpace(fragment.DispatchID),
	)
	sum := sha256.Sum256([]byte(material))
	return "evt-legacy-" + hex.EncodeToString(sum[:8])
}
