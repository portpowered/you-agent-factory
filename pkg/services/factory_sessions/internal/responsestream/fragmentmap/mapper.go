// Package fragmentmap projects internal session response-stream events into the
// canonical FactoryResponseEvent vocabulary for session-owned publication.
package fragmentmap

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseevents"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// ErrUnsupportedFragmentKind indicates the mapper does not handle the supplied
// internal response-stream event kind.
var ErrUnsupportedFragmentKind = errors.New("unsupported internal response-stream event kind")

// Context carries correlation fields required on canonical response events but
// absent from internal response-stream events.
type Context struct {
	FactorySessionID string
	RunID            string
}

type fragmentMapper func(Context, responsestream.Event) (responseevents.FactoryResponseEvent, error)

// MapFragment converts one internal response-stream event into canonical
// FactoryResponseEvent values.
func MapFragment(ctx Context, fragment responsestream.Event) ([]responseevents.FactoryResponseEvent, error) {
	mapper, label, ok := fragmentMapperForKind(fragment.Kind)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedFragmentKind, fragment.Kind)
	}
	return mapValidatedFragment(mapper, label, ctx, fragment)
}

func fragmentMapperForKind(kind responsestream.EventKind) (fragmentMapper, string, bool) {
	switch kind {
	case responsestream.EventKindProgressFragment:
		return mapProgressFragment, "progress", true
	case responsestream.EventKindResponseFragment:
		return mapResponseFragment, "response", true
	case responsestream.EventKindStreamCompleted:
		return mapStreamCompletedFragment, "stream-completed", true
	case responsestream.EventKindStreamFailed:
		return mapStreamFailedFragment, "stream-failed", true
	case responsestream.EventKindCompactionSignal:
		return mapCompactionFragment, "compaction", true
	default:
		return nil, "", false
	}
}

func mapValidatedFragment(
	mapper fragmentMapper,
	label string,
	ctx Context,
	fragment responsestream.Event,
) ([]responseevents.FactoryResponseEvent, error) {
	event, err := mapper(ctx, fragment)
	if err != nil {
		return nil, err
	}
	if err := responseevents.ValidateEvent(event); err != nil {
		return nil, fmt.Errorf("mapped %s event invalid: %w", label, err)
	}
	return []responseevents.FactoryResponseEvent{event}, nil
}

func mapResponseFragment(ctx Context, fragment responsestream.Event) (responseevents.FactoryResponseEvent, error) {
	payload, err := json.Marshal(messageDeltaPayloadFromFragment(fragment))
	if err != nil {
		return responseevents.FactoryResponseEvent{}, fmt.Errorf("marshal message delta payload: %w", err)
	}

	return responseevents.FactoryResponseEvent{
		SchemaVersion:    responseevents.SchemaVersionV1,
		EventID:          synthesizedEventID(ctx, fragment),
		Sequence:         fragment.Sequence,
		RecordedAt:       fragment.RecordedAt,
		FactorySessionID: strings.TrimSpace(ctx.FactorySessionID),
		RunID:            strings.TrimSpace(ctx.RunID),
		Kind:             responseevents.KindMessage,
		Phase:            responseevents.PhaseDelta,
		Provenance: responseevents.Provenance{
			Provider:        fragmentProvider(fragment),
			NativeEventType: fragmentNativeEventType(fragment),
			Delivery:        responseevents.DeliverySynthesized,
			Representation:  responseevents.RepresentationDelta,
			Fidelity:        responseFragmentFidelity(fragment),
		},
		Payload:            payload,
		DispatchID:         strings.TrimSpace(fragment.DispatchID),
		ItemID:             synthesizedItemID(ctx, fragment),
		ProviderSessionRef: providerSessionRefString(fragment.ProviderSessionRef),
	}, nil
}

func messageDeltaPayloadFromFragment(fragment responsestream.Event) responseevents.MessageDeltaPayload {
	return responseevents.MessageDeltaPayload{
		ContentBlockIndex: 0,
		ContentBlockKind:  responseevents.ContentBlockText,
		TextDelta:         fragment.Payload,
	}
}

func responseFragmentFidelity(fragment responsestream.Event) responseevents.Fidelity {
	if fragmentPayloadTruncated(fragment.Metadata) {
		return responseevents.FidelityLossy
	}
	return responseevents.FidelityNormalized
}

func synthesizedItemID(ctx Context, fragment responsestream.Event) string {
	material := fmt.Sprintf(
		"%s|%s|%s|%s",
		strings.TrimSpace(ctx.FactorySessionID),
		strings.TrimSpace(ctx.RunID),
		strings.TrimSpace(fragment.DispatchID),
		providerSessionRefString(fragment.ProviderSessionRef),
	)
	sum := sha256.Sum256([]byte(material))
	return "item-legacy-" + hex.EncodeToString(sum[:8])
}

func mapStreamCompletedFragment(ctx Context, fragment responsestream.Event) (responseevents.FactoryResponseEvent, error) {
	payload, err := json.Marshal(responseevents.RunPayload{Status: "completed"})
	if err != nil {
		return responseevents.FactoryResponseEvent{}, fmt.Errorf("marshal run payload: %w", err)
	}

	return responseevents.FactoryResponseEvent{
		SchemaVersion:    responseevents.SchemaVersionV1,
		EventID:          synthesizedEventID(ctx, fragment),
		Sequence:         fragment.Sequence,
		RecordedAt:       fragment.RecordedAt,
		FactorySessionID: strings.TrimSpace(ctx.FactorySessionID),
		RunID:            strings.TrimSpace(ctx.RunID),
		Kind:             responseevents.KindRun,
		Phase:            responseevents.PhaseCompleted,
		Provenance: responseevents.Provenance{
			Provider:        fragmentProvider(fragment),
			NativeEventType: fragmentNativeEventType(fragment),
			Delivery:        responseevents.DeliverySynthesized,
			Representation:  responseevents.RepresentationSnapshot,
			Fidelity:        terminalFragmentFidelity(fragment),
		},
		Payload:            payload,
		DispatchID:         strings.TrimSpace(fragment.DispatchID),
		ProviderSessionRef: providerSessionRefString(fragment.ProviderSessionRef),
	}, nil
}

func mapStreamFailedFragment(ctx Context, fragment responsestream.Event) (responseevents.FactoryResponseEvent, error) {
	payload, err := json.Marshal(errorPayloadFromFragment(fragment))
	if err != nil {
		return responseevents.FactoryResponseEvent{}, fmt.Errorf("marshal error payload: %w", err)
	}

	return responseevents.FactoryResponseEvent{
		SchemaVersion:    responseevents.SchemaVersionV1,
		EventID:          synthesizedEventID(ctx, fragment),
		Sequence:         fragment.Sequence,
		RecordedAt:       fragment.RecordedAt,
		FactorySessionID: strings.TrimSpace(ctx.FactorySessionID),
		RunID:            strings.TrimSpace(ctx.RunID),
		Kind:             responseevents.KindError,
		Phase:            responseevents.PhaseFailed,
		Provenance: responseevents.Provenance{
			Provider:        fragmentProvider(fragment),
			NativeEventType: fragmentNativeEventType(fragment),
			Delivery:        responseevents.DeliverySynthesized,
			Representation:  responseevents.RepresentationNotification,
			Fidelity:        terminalFragmentFidelity(fragment),
		},
		Payload:            payload,
		DispatchID:         strings.TrimSpace(fragment.DispatchID),
		ProviderSessionRef: providerSessionRefString(fragment.ProviderSessionRef),
	}, nil
}

func errorPayloadFromFragment(fragment responsestream.Event) responseevents.ErrorPayload {
	code := streamFailedErrorCode(fragment)
	message := strings.TrimSpace(fragment.Payload)
	if message == "" {
		message = "dispatch stream failed"
	}
	return responseevents.ErrorPayload{
		Code:    code,
		Message: message,
	}
}

func streamFailedErrorCode(fragment responsestream.Event) string {
	switch fragment.Type {
	case responsestream.EventTypeCanceled:
		return "stream_canceled"
	case responsestream.EventTypeFailed:
		return "stream_failed"
	default:
		return "stream_failed"
	}
}

func terminalFragmentFidelity(fragment responsestream.Event) responseevents.Fidelity {
	if fragmentPayloadTruncated(fragment.Metadata) {
		return responseevents.FidelityLossy
	}
	return responseevents.FidelityNormalized
}

func mapCompactionFragment(ctx Context, fragment responsestream.Event) (responseevents.FactoryResponseEvent, error) {
	payload, err := json.Marshal(streamGapPayloadFromCompaction(fragment.Compaction))
	if err != nil {
		return responseevents.FactoryResponseEvent{}, fmt.Errorf("marshal stream gap payload: %w", err)
	}

	return responseevents.FactoryResponseEvent{
		SchemaVersion:    responseevents.SchemaVersionV1,
		EventID:          synthesizedEventID(ctx, fragment),
		Sequence:         fragment.Sequence,
		RecordedAt:       fragment.RecordedAt,
		FactorySessionID: strings.TrimSpace(ctx.FactorySessionID),
		RunID:            strings.TrimSpace(ctx.RunID),
		Kind:             responseevents.KindStreamGap,
		Phase:            responseevents.PhaseUpdated,
		Provenance: responseevents.Provenance{
			Provider:        fragmentProvider(fragment),
			NativeEventType: fragmentNativeEventType(fragment),
			Delivery:        responseevents.DeliverySynthesized,
			Representation:  responseevents.RepresentationNotification,
			Fidelity:        compactionFragmentFidelity(),
		},
		Payload:            payload,
		DispatchID:         strings.TrimSpace(fragment.DispatchID),
		ProviderSessionRef: providerSessionRefString(fragment.ProviderSessionRef),
	}, nil
}

func streamGapPayloadFromCompaction(summary *responsestream.CompactionSummary) responseevents.StreamGapPayload {
	if summary == nil {
		return responseevents.StreamGapPayload{FirstAvailableSequence: 1, Reason: "compaction"}
	}

	fromSequence := int64(0)
	toSequence := int64(0)
	if summary.LastDroppedSequence > 0 && summary.DroppedSequenceCount > 0 {
		fromSequence = summary.LastDroppedSequence - int64(summary.DroppedSequenceCount) + 1
		if fromSequence < 0 {
			fromSequence = 0
		}
	}
	if summary.LastDroppedSequence > 0 {
		toSequence = summary.LastDroppedSequence
	}
	firstAvailableSequence := summary.FirstRetainedSequence
	if firstAvailableSequence <= 0 {
		firstAvailableSequence = summary.LastDroppedSequence + 1
	}
	if firstAvailableSequence <= 0 {
		firstAvailableSequence = 1
	}

	return responseevents.StreamGapPayload{
		FromSequence:           fromSequence,
		ToSequence:             toSequence,
		FirstAvailableSequence: firstAvailableSequence,
		Reason:                 compactionReasonString(summary.Reason),
	}
}

func compactionReasonString(reason responsestream.CompactionReason) string {
	switch reason {
	case responsestream.CompactionReasonTruncated:
		return "truncated"
	case responsestream.CompactionReasonCoalesced:
		return "coalesced"
	case responsestream.CompactionReasonAgeEvicted:
		return "age_evicted"
	default:
		if trimmed := strings.TrimSpace(string(reason)); trimmed != "" {
			return strings.ToLower(trimmed)
		}
		return "compaction"
	}
}

func compactionFragmentFidelity() responseevents.Fidelity {
	return responseevents.FidelityLossy
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
		if provider := workerexecution.CanonicalProviderSessionProvider(fragment.ProviderSessionRef.Provider); provider != "" {
			return provider
		}
	}
	if fragment.Metadata != nil {
		if runner := strings.TrimSpace(fragment.Metadata["runner_id"]); runner != "" {
			return runner
		}
	}
	return "internal-fragment"
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

func providerSessionRefString(session *workerexecution.ProviderSessionMetadata) string {
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
