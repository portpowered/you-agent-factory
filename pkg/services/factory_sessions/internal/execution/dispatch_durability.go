package factorysessionexecution

import (
	"encoding/json"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// SetDispatchDurability binds the completed-flush capability and stream
// identity for live Factory Session dispatch reads. Runtime opening performs
// this once before the session can serve customer reads.
func (s *JavaScriptRuntimeService) SetDispatchDurability(
	reader recordings.CompletedFlushWatermarkReader,
	streamGenerationID string,
) {
	if s == nil {
		return
	}
	s.dispatchDurabilityMu.Lock()
	s.dispatchDurability = reader
	s.dispatchStreamGenerationID = strings.TrimSpace(streamGenerationID)
	s.dispatchDurabilityMu.Unlock()
}

// dispatchesForRead detaches the current dispatch projection and applies the
// conservative default used by live durable-session reads. The durable
// session projection is process-local until its recording is published, so a
// zero or unknown state must never be serialized as confirmed.
func dispatchesForRead(dispatches []DispatchSummary, events []json.RawMessage) []DispatchSummary {
	return dispatchesForReadWithDurability(dispatches, events, nil, "")
}

func dispatchesForReadWithDurability(
	dispatches []DispatchSummary,
	events []json.RawMessage,
	reader recordings.CompletedFlushWatermarkReader,
	streamGenerationID string,
) []DispatchSummary {
	result := cloneDispatchSummaries(dispatches)
	annotateDispatchStateCursors(result, events)
	for index := range result {
		result[index].ConfirmationState = ConfirmationStateUnconfirmed
	}
	applyDispatchConfirmation(result, reader, streamGenerationID)
	return result
}

func applyDispatchConfirmation(
	dispatches []DispatchSummary,
	reader recordings.CompletedFlushWatermarkReader,
	streamGenerationID string,
) {
	streamGenerationID = strings.TrimSpace(streamGenerationID)
	if len(dispatches) == 0 || reader == nil || streamGenerationID == "" {
		return
	}
	watermark, ok := reader.CompletedFlushWatermark(streamGenerationID)
	if !ok || strings.TrimSpace(watermark.StreamGenerationID) != streamGenerationID {
		return
	}
	for index := range dispatches {
		dispatch := &dispatches[index]
		if !dispatch.StateSequenceKnown {
			continue
		}
		if dispatch.StreamGenerationID == "" {
			dispatch.StreamGenerationID = streamGenerationID
		}
		if dispatch.StreamGenerationID == streamGenerationID &&
			dispatch.StateSequence <= int64(watermark.Sequence) {
			dispatch.ConfirmationState = ConfirmationStateConfirmed
		}
	}
}

// annotateDispatchStateCursors preserves the latest lifecycle event that can
// have produced each projected dispatch status. Numeric event sequences are
// kept as detached metadata for the durability boundary and are not exposed
// by the public API.
func annotateDispatchStateCursors(dispatches []DispatchSummary, events []json.RawMessage) {
	if len(dispatches) == 0 || len(events) == 0 {
		return
	}
	positions := make(map[string]int64, len(dispatches))
	known := make(map[string]bool, len(dispatches))
	for _, raw := range events {
		var envelope struct {
			Type    string `json:"type"`
			Context struct {
				DispatchID *string `json:"dispatchId"`
				Sequence   int     `json:"sequence"`
			} `json:"context"`
		}
		if json.Unmarshal(raw, &envelope) != nil || !isDispatchLifecycleEvent(envelope.Type) ||
			envelope.Context.DispatchID == nil || strings.TrimSpace(*envelope.Context.DispatchID) == "" ||
			envelope.Context.Sequence <= 0 {
			continue
		}
		dispatchID := strings.TrimSpace(*envelope.Context.DispatchID)
		sequence := int64(envelope.Context.Sequence)
		if !known[dispatchID] || sequence >= positions[dispatchID] {
			positions[dispatchID] = sequence
			known[dispatchID] = true
		}
	}
	for index := range dispatches {
		dispatchID := strings.TrimSpace(dispatches[index].ID)
		if !known[dispatchID] {
			continue
		}
		dispatches[index].StateSequence = positions[dispatchID]
		dispatches[index].StateSequenceKnown = true
	}
}

func isDispatchLifecycleEvent(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "DISPATCH_QUEUED", "DISPATCH_RECONCILED", "DISPATCH_INTERRUPTED":
		return true
	default:
		return false
	}
}
