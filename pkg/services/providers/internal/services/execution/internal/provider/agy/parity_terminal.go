package agy

import (
	"fmt"
	"strings"
	"unicode/utf8"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/provider/adapter"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// ParityTerminal builds adapter-neutral terminal outcomes from sanitized Agy
// final-only transcripts for cross-provider parity proofs.
func ParityTerminal(runID, dispatchID string, transcript []byte) (
	workerexecution.InferenceResponse,
	adapter.Capabilities,
	[]factorysessions.ResponseEventDraft,
	error,
) {
	if !utf8.Valid(transcript) {
		return workerexecution.InferenceResponse{}, adapter.Capabilities{}, nil, fmt.Errorf("agy parse final: invalid utf-8 transcript")
	}
	content := strings.TrimSpace(string(transcript))
	if content == "" {
		return workerexecution.InferenceResponse{}, adapter.Capabilities{}, nil, fmt.Errorf("agy parse final: empty transcript")
	}
	events, err := finalOnlyProgressEvents(runID, content)
	if err != nil {
		return workerexecution.InferenceResponse{}, adapter.Capabilities{}, nil, fmt.Errorf("agy parity drafts: %w", err)
	}
	drafts := make([]factorysessions.ResponseEventDraft, 0, len(events))
	for _, event := range events {
		draft := event.Draft()
		draft.DispatchID = dispatchID
		drafts = append(drafts, draft)
	}
	return workerexecution.InferenceResponse{Content: content}, adapter.Capabilities{
		MessageSnapshots: true,
		FinalOnly:        true,
	}, drafts, nil
}
