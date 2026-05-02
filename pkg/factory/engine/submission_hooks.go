package engine

import (
	"context"
	"fmt"
	"sort"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/internal/submission"
	"github.com/portpowered/infinite-you/pkg/petri"
)

const externalSubmissionHookName = "external-submit"

type queuedSubmissionHook struct {
	batches []interfaces.GeneratedSubmissionBatch
}

func newQueuedSubmissionHook() *queuedSubmissionHook {
	return &queuedSubmissionHook{}
}

func (h *queuedSubmissionHook) Name() string {
	return externalSubmissionHookName
}

func (h *queuedSubmissionHook) Priority() int {
	return 0
}

func (h *queuedSubmissionHook) enqueue(work []interfaces.SubmitRequest) {
	copied := make([]interfaces.SubmitRequest, len(work))
	copy(copied, work)
	h.batches = append(h.batches, interfaces.GeneratedSubmissionBatch{
		Request:  submission.WorkRequestFromSubmitRequests(copied),
		Metadata: interfaces.GeneratedSubmissionBatchMetadata{Source: h.Name()},
	})
}

func (h *queuedSubmissionHook) OnTick(_ context.Context, _ interfaces.SubmissionHookContext[interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]]) (interfaces.SubmissionHookResult, error) {
	if len(h.batches) == 0 {
		return interfaces.SubmissionHookResult{}, nil
	}

	var result interfaces.SubmissionHookResult
	for len(h.batches) > 0 {
		batch := h.batches[0]
		h.batches = h.batches[1:]
		result.GeneratedBatches = append(result.GeneratedBatches, batch)
	}
	return result, nil
}

func sortedSubmissionHooks(hooks []factory.SubmissionHook) []factory.SubmissionHook {
	sorted := make([]factory.SubmissionHook, len(hooks))
	copy(sorted, hooks)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Priority() == sorted[j].Priority() {
			return sorted[i].Name() < sorted[j].Name()
		}
		return sorted[i].Priority() < sorted[j].Priority()
	})
	return sorted
}

func copyHookState(state map[string]string) map[string]string {
	if len(state) == 0 {
		return nil
	}
	copied := make(map[string]string, len(state))
	for k, v := range state {
		copied[k] = v
	}
	return copied
}

func submissionRecordID(tick int, hookName string, index int) string {
	return fmt.Sprintf("tick-%d:%s:%d", tick, hookName, index)
}

func completionRecordID(tick int, dispatchID string, index int) string {
	return fmt.Sprintf("tick-%d:%s:%d", tick, dispatchID, index)
}

func consumedTokenIDs(tokens []interfaces.Token) []string {
	if len(tokens) == 0 {
		return nil
	}
	ids := make([]string, 0, len(tokens))
	for _, token := range tokens {
		ids = append(ids, token.ID)
	}
	return ids
}
