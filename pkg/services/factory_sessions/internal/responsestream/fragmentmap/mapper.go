// Package fragmentmap projects internal session response-stream events into the
// canonical FactoryResponseEvent vocabulary for session-owned publication.
package fragmentmap

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseevents"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
	"github.com/portpowered/infinite-you/pkg/services/providers"
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
	if fragment.Kind == responsestream.EventKindProgressFragment {
		if event, ok, err := mapNativeProgressFragment(ctx, fragment); ok || err != nil {
			if err != nil {
				return nil, err
			}
			if err := responseevents.ValidateEvent(event); err != nil {
				return nil, fmt.Errorf("mapped native progress event invalid: %w", err)
			}
			return []responseevents.FactoryResponseEvent{event}, nil
		}
		// A dotted value is a provider-native phase. Unsupported native phases
		// (for example session.started or diagnostics) are intentionally omitted
		// instead of being mislabeled as generic customer progress.
		nativeType := strings.TrimSpace(fragment.ExternalEventType)
		if strings.Contains(nativeType, ".") && !strings.EqualFold(nativeType, "response.progress") {
			return nil, nil
		}
	}
	mapper, label, ok := fragmentMapperForKind(fragment.Kind)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedFragmentKind, fragment.Kind)
	}
	return mapValidatedFragment(mapper, label, ctx, fragment)
}

func mapNativeProgressFragment(
	ctx Context,
	fragment responsestream.Event,
) (responseevents.FactoryResponseEvent, bool, error) {
	native := strings.ToLower(strings.TrimSpace(fragment.ExternalEventType))
	parts := strings.Split(native, ".")
	if len(parts) != 2 {
		return responseevents.FactoryResponseEvent{}, false, nil
	}
	phase, ok := nativeResponsePhase(parts[1])
	if !ok {
		return responseevents.FactoryResponseEvent{}, false, nil
	}

	var (
		kind           responseevents.Kind
		payload        []byte
		itemID         string
		nativeEvent    = fragment.ExternalEventType
		representation = responseevents.RepresentationNotification
		err            error
	)
	switch parts[0] {
	case "run", "turn":
		kind = responseevents.KindRun
		nativeEvent = "providers_progress"
		payload, err = json.Marshal(responseevents.RunPayload{Status: parts[1]})
	case "message":
		kind = responseevents.KindMessage
		if phase == responseevents.PhaseCompleted {
			provider := fragmentProvider(fragment)
			if provider == "unknown" || strings.TrimSpace(provider) == "" {
				provider = "provider"
			}
			itemID = provider + "-message"
		} else {
			itemID = nativeItemID(fragment, "message")
		}
		if phase == responseevents.PhaseDelta {
			representation = responseevents.RepresentationDelta
			payload, err = json.Marshal(responseevents.MessageDeltaPayload{
				ContentBlockIndex: 0,
				ContentBlockKind:  responseevents.ContentBlockText,
				TextDelta:         fragment.Payload,
			})
		} else {
			representation = responseevents.RepresentationSnapshot
			payload, err = json.Marshal(responseevents.MessagePayload{
				Role: "assistant",
				ContentBlocks: []responseevents.ContentBlock{{
					Kind: responseevents.ContentBlockText,
					Text: fragment.Payload,
				}},
			})
		}
	case "tool":
		kind = responseevents.KindTool
		itemID = nativeItemID(fragment, "tool")
		if phase == responseevents.PhaseDelta {
			representation = responseevents.RepresentationDelta
			payload, err = json.Marshal(responseevents.ToolDeltaPayload{
				ToolCallID:  itemID,
				OutputDelta: fragment.Payload,
			})
			break
		}
		var result json.RawMessage
		if strings.TrimSpace(fragment.Payload) != "" {
			result, _ = json.Marshal(map[string]string{"detail": fragment.Payload})
		}
		payload, err = json.Marshal(responseevents.ToolPayload{
			ToolCallID:    itemID,
			ToolName:      strings.TrimSpace(fragment.Metadata["tool_name"]),
			Status:        parts[1],
			ResultSummary: result,
		})
	default:
		return responseevents.FactoryResponseEvent{}, false, nil
	}
	if err != nil {
		return responseevents.FactoryResponseEvent{}, true, err
	}
	return nativeProgressEvent(ctx, fragment, kind, phase, nativeEvent, representation, payload, itemID), true, nil
}

func nativeProgressEvent(
	ctx Context,
	fragment responsestream.Event,
	kind responseevents.Kind,
	phase responseevents.Phase,
	nativeEvent string,
	representation responseevents.Representation,
	payload []byte,
	itemID string,
) responseevents.FactoryResponseEvent {
	return responseevents.FactoryResponseEvent{
		SchemaVersion:    responseevents.SchemaVersionV1,
		EventID:          synthesizedEventID(ctx, fragment),
		Sequence:         fragment.Sequence,
		RecordedAt:       fragment.RecordedAt,
		FactorySessionID: strings.TrimSpace(ctx.FactorySessionID),
		RunID:            firstNonEmptyString(ctx.RunID, fragment.DispatchID),
		Kind:             kind,
		Phase:            phase,
		Provenance: responseevents.Provenance{
			Provider:        fragmentProvider(fragment),
			NativeEventType: nativeEvent,
			Delivery:        responseevents.DeliveryNativeStream,
			Representation:  representation,
			Fidelity:        responseevents.FidelityNormalized,
		},
		Payload:            payload,
		DispatchID:         strings.TrimSpace(fragment.DispatchID),
		ItemID:             itemID,
		ProviderSessionRef: providerSessionRefString(fragment.ProviderSessionRef),
	}
}

func nativeResponsePhase(value string) (responseevents.Phase, bool) {
	switch value {
	case "started":
		return responseevents.PhaseStarted, true
	case "delta", "updated":
		return responseevents.PhaseDelta, true
	case "completed":
		return responseevents.PhaseCompleted, true
	case "failed":
		return responseevents.PhaseFailed, true
	case "canceled":
		return responseevents.PhaseCanceled, true
	default:
		return "", false
	}
}

func nativeItemID(fragment responsestream.Event, fallback string) string {
	for _, key := range []string{"correlation_id", "item_id", "message_id"} {
		if value := strings.TrimSpace(fragment.Metadata[key]); value != "" {
			return value
		}
	}
	return fallback + "-" + strings.TrimSpace(fragment.DispatchID)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
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
		RunID:            firstNonEmptyString(ctx.RunID, fragment.DispatchID),
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
		RunID:            firstNonEmptyString(ctx.RunID, fragment.DispatchID),
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
		RunID:            firstNonEmptyString(ctx.RunID, fragment.DispatchID),
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
		Code:      code,
		Message:   message,
		Retryable: strings.EqualFold(fragment.Metadata["retryable"], "true"),
	}
}

func streamFailedErrorCode(fragment responsestream.Event) string {
	if strings.EqualFold(fragment.Metadata["work_failure_type"], string(workerexecution.WorkFailureTypeTimeout)) {
		return "timeout"
	}
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
	kind, phase, payloadValue := semanticProgress(fragment)
	payload, err := json.Marshal(payloadValue)
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
		Kind:             kind,
		Phase:            phase,
		Provenance: responseevents.Provenance{
			Provider:        fragmentProvider(fragment),
			NativeEventType: fragmentNativeEventType(fragment),
			Delivery:        responseevents.DeliverySynthesized,
			Representation:  semanticProgressRepresentation(kind, phase),
			Fidelity:        progressFragmentFidelity(fragment),
		},
		Payload:            payload,
		DispatchID:         strings.TrimSpace(fragment.DispatchID),
		ItemID:             strings.TrimSpace(fragment.Metadata["item_id"]),
		ProviderSessionRef: providerSessionRefString(fragment.ProviderSessionRef),
	}, nil
}

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func semanticProgress(fragment responsestream.Event) (responseevents.Kind, responseevents.Phase, any) {
	metadata := fragment.Metadata
	kind := strings.ToLower(strings.TrimSpace(metadata["kind"]))
	phase := semanticPhase(fragment.Type)
	switch kind {
	case "run":
		if phase == responseevents.PhaseUpdated {
			return responseevents.KindProgress, phase, progressPayloadFromFragment(fragment)
		}
		return responseevents.KindRun, phase, responseevents.RunPayload{Status: strings.ToLower(string(phase))}
	case "session":
		payload := responseevents.SessionPayload{Status: strings.ToLower(string(phase))}
		if phase == responseevents.PhaseUpdated && strings.EqualFold(metadata["title_present"], "true") {
			title := fragment.Payload
			payload.Title = &title
		}
		return responseevents.KindSession, phase, payload
	case "message":
		phase = contentProgressPhase(phase)
		if phase == responseevents.PhaseDelta {
			return responseevents.KindMessage, phase, responseevents.MessageDeltaPayload{ContentBlockIndex: 0, ContentBlockKind: responseevents.ContentBlockText, TextDelta: fragment.Payload}
		}
		return responseevents.KindMessage, phase, responseevents.MessagePayload{
			Role:          "assistant",
			ContentBlocks: []responseevents.ContentBlock{{Kind: responseevents.ContentBlockText, Text: fragment.Payload}},
			Partial:       strings.EqualFold(strings.TrimSpace(metadata["partial"]), "true"),
		}
	case "reasoning":
		phase = contentProgressPhase(phase)
		payload := responseevents.ReasoningPayload{Summary: fragment.Payload}
		if phase == responseevents.PhaseDelta {
			payload, payload.SummaryDelta = responseevents.ReasoningPayload{}, fragment.Payload
		}
		return responseevents.KindReasoning, phase, payload
	case "tool":
		phase = contentProgressPhase(phase)
		toolID := strings.TrimSpace(metadata["item_id"])
		name := strings.TrimSpace(fragment.Payload)
		if name == "" {
			name = "ACP tool"
		}
		if phase == responseevents.PhaseDelta {
			return responseevents.KindTool, phase, responseevents.ToolDeltaPayload{
				ToolCallID:  toolID,
				OutputDelta: fragment.Payload,
			}
		}
		payload := responseevents.ToolPayload{ToolCallID: toolID, ToolName: name, Status: metadata["status"]}
		if raw := json.RawMessage(metadata["raw_input"]); json.Valid(raw) {
			payload.ArgumentsSummary = raw
		}
		if raw := json.RawMessage(metadata["raw_output"]); json.Valid(raw) {
			payload.ResultSummary = raw
		}
		return responseevents.KindTool, phase, payload
	case "file_change":
		return responseevents.KindFileChange, responseevents.PhaseUpdated, responseevents.FileChangePayload{Path: metadata["path"], Operation: metadata["operation"], Summary: fragment.Payload}
	case "plan":
		return responseevents.KindPlan, responseevents.PhaseUpdated, planPayloadFromFragment(fragment, metadata)
	case "usage":
		return responseevents.KindUsage, responseevents.PhaseUpdated, responseevents.UsagePayload{TotalTokens: parseProgressInt64(metadata["used_tokens"])}
	case "error":
		return responseevents.KindError, responseevents.PhaseFailed, responseevents.ErrorPayload{Code: firstNonEmptyProgress(metadata["error_code"], "provider_failure"), Message: firstNonEmptyProgress(fragment.Payload, "provider execution failed")}
	default:
		return responseevents.KindProgress, responseevents.PhaseUpdated, progressPayloadFromFragment(fragment)
	}
}

func contentProgressPhase(phase responseevents.Phase) responseevents.Phase {
	if phase == responseevents.PhaseUpdated {
		return responseevents.PhaseDelta
	}
	return phase
}

func semanticProgressRepresentation(kind responseevents.Kind, phase responseevents.Phase) responseevents.Representation {
	if kind == responseevents.KindMessage && phase == responseevents.PhaseCompleted {
		return responseevents.RepresentationSnapshot
	}
	return responseevents.RepresentationNotification
}

func semanticPhase(value responsestream.EventType) responseevents.Phase {
	normalized := responsestream.EventType(strings.ToUpper(strings.TrimSpace(string(value))))
	switch normalized {
	case responsestream.EventTypeStarted, "START":
		return responseevents.PhaseStarted
	case responsestream.EventTypeTextDelta, "DELTA":
		return responseevents.PhaseDelta
	case responsestream.EventTypeFinalText, "COMPLETED", "COMPLETE":
		return responseevents.PhaseCompleted
	case responsestream.EventTypeFailed:
		return responseevents.PhaseFailed
	case responsestream.EventTypeCanceled, "CANCELLED":
		return responseevents.PhaseCanceled
	case responsestream.EventTypeProgress, responsestream.EventTypeUnknown, "UPDATED":
		return responseevents.PhaseUpdated
	default:
		return responseevents.PhaseUpdated
	}
}

func parseProgressInt64(value string) int64 {
	var parsed int64
	_, _ = fmt.Sscan(strings.TrimSpace(value), &parsed)
	return parsed
}

func firstNonEmptyProgress(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
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
	if provider := strings.TrimSpace(fragment.Provider); provider != "" {
		return provider
	}
	if fragment.ProviderSessionRef != nil {
		if provider := providers.ID(fragment.ProviderSessionRef.Provider).CanonicalSessionProvider(); provider != "" {
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
	if native := strings.TrimSpace(fragment.Metadata["native_type"]); native != "" {
		return native
	}
	if typed := strings.TrimSpace(string(fragment.Type)); typed != "" {
		return typed
	}
	return string(fragment.Kind)
}

func providerSessionRefString(session *providers.SessionMetadata) string {
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

// planPayloadFromFragment recovers a plan's individual steps from the
// provider's own reported entries.
//
// The ACP client already captures the entry list; keeping only a summary
// string discarded it, so nothing downstream could render an actual plan no
// matter what the provider reported. A provider that reports no usable entries
// still yields the summary, which is the prior behavior.
func planPayloadFromFragment(
	fragment responsestream.Event,
	metadata map[string]string,
) responseevents.PlanPayload {
	payload := responseevents.PlanPayload{
		Summary: firstNonEmptyProgress(fragment.Payload, "ACP plan updated"),
	}
	raw := strings.TrimSpace(metadata["entries"])
	if raw == "" {
		return payload
	}
	var entries []struct {
		Content string `json:"content"`
		Title   string `json:"title"`
		Status  string `json:"status"`
	}
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return payload
	}
	for index, entry := range entries {
		description := strings.TrimSpace(entry.Content)
		if description == "" {
			description = strings.TrimSpace(entry.Title)
		}
		if description == "" {
			continue
		}
		payload.Steps = append(payload.Steps, responseevents.PlanStep{
			ID:          strconv.Itoa(index + 1),
			Description: description,
			Status:      entry.Status,
		})
	}
	return payload
}
