package engine

import (
	"context"
	"fmt"
	"sort"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
	workdomain "github.com/portpowered/infinite-you/pkg/services/work"
)

const externalSubmissionHookName = "external-submit"

type queuedSubmissionHook struct {
	batches []workdomain.GeneratedSubmissionBatch
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

func (h *queuedSubmissionHook) enqueue(work []workdomain.SubmitRequest) {
	copied := make([]workdomain.SubmitRequest, len(work))
	copy(copied, work)
	h.batches = append(h.batches, workdomain.GeneratedSubmissionBatch{
		Request:  workdomain.WorkRequestFromSubmitRequests(copied),
		Metadata: workdomain.GeneratedSubmissionBatchMetadata{Source: h.Name()},
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

func consumedTokenIDs(tokens []factorytoken.Token) []string {
	if len(tokens) == 0 {
		return nil
	}
	ids := make([]string, 0, len(tokens))
	for _, token := range tokens {
		ids = append(ids, token.ID)
	}
	return ids
}

func (e *FactoryEngine) processGeneratedSubmissionBatches(
	batches []workdomain.GeneratedSubmissionBatch,
	defaultSource string,
	dedupeSeededReplay bool,
) (int, error) {
	total := 0
	for i := range batches {
		batch := batches[i]
		source := generatedSubmissionSource(batch, defaultSource)
		normalized, requestID, err := e.normalizeGeneratedSubmissionBatch(batch)
		if err != nil {
			return total, err
		}
		if e.skipGeneratedSubmissionRequest(requestID, source) {
			continue
		}
		normalized, replacedSeededWorkIDs := e.dedupeSeededReplaySubmissions(normalized, dedupeSeededReplay)
		tokens, err := e.tokensFromGeneratedSubmissions(normalized)
		if err != nil {
			return total, err
		}
		e.recordGeneratedSubmissionRequest(requestID, source, batch, normalized)
		e.recordGeneratedSubmissionTokens(source, normalized, tokens, replacedSeededWorkIDs)
		if source == externalSubmissionHookName {
			e.pendingProjectionRequestIDs[requestID] = struct{}{}
		}
		total += len(tokens)
	}
	return total, nil
}

// dedupeSeededReplaySubmissions removes only replay-generated Work already
// represented by the restored marking. The replay hook keeps its recorded
// source metadata, so hook identity—not source text—selects this rule.
func (e *FactoryEngine) dedupeSeededReplaySubmissions(
	normalized []workdomain.SubmitRequest,
	dedupeSeededReplay bool,
) ([]workdomain.SubmitRequest, map[string]struct{}) {
	if !dedupeSeededReplay || len(e.seededRestoredWorkIDs) == 0 || len(normalized) == 0 {
		return normalized, nil
	}
	kept := normalized[:0]
	replaced := make(map[string]struct{})
	for _, request := range normalized {
		if _, seeded := e.seededRestoredWorkIDs[request.WorkID]; seeded {
			if _, hasRecordedDispatch := e.replayDispatchWorkIDs[request.WorkID]; hasRecordedDispatch {
				kept = append(kept, request)
				replaced[request.WorkID] = struct{}{}
			}
			continue
		}
		kept = append(kept, request)
	}
	if len(replaced) == 0 {
		replaced = nil
	}
	return kept, replaced
}

func cloneWorkIDSet(source map[string]struct{}) map[string]struct{} {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[string]struct{}, len(source))
	for workID := range source {
		cloned[workID] = struct{}{}
	}
	return cloned
}

func generatedSubmissionSource(batch workdomain.GeneratedSubmissionBatch, defaultSource string) string {
	if batch.Metadata.Source != "" {
		return batch.Metadata.Source
	}
	if defaultSource != "" {
		return defaultSource
	}
	return "generated-batch"
}

func (e *FactoryEngine) normalizeGeneratedSubmissionBatch(batch workdomain.GeneratedSubmissionBatch) ([]workdomain.SubmitRequest, string, error) {
	normalized, err := workdomain.NormalizeGeneratedSubmissionBatch(batch, workdomain.WorkRequestNormalizeOptions{ValidWorkTypes: e.validWorkTypes(), ValidStatesByType: state.ValidStatesByType(e.state.WorkTypes), IDGenerator: e.workRequestIDs})
	if err != nil {
		return nil, "", err
	}
	requestID := ""
	if len(normalized) > 0 {
		requestID = normalized[0].RequestID
	}
	return normalized, requestID, nil
}

func (e *FactoryEngine) skipGeneratedSubmissionRequest(requestID, source string) bool {
	if requestID == "" || source == externalSubmissionHookName {
		return false
	}
	_, exists := e.workRequests[requestID]
	return exists
}

func (e *FactoryEngine) tokensFromGeneratedSubmissions(normalized []workdomain.SubmitRequest) ([]*factorytoken.Token, error) {
	now := e.clock.Now()
	tokens := make([]*factorytoken.Token, 0, len(normalized))
	for _, req := range normalized {
		token, err := e.transformer.InitialTokenFromSubmit(req, now)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	return tokens, nil
}

func (e *FactoryEngine) recordGeneratedSubmissionRequest(requestID, source string, batch workdomain.GeneratedSubmissionBatch, normalized []workdomain.SubmitRequest) {
	e.workRequests[requestID] = workdomain.WorkRequestSubmitResultFromNormalized(requestID, normalized, true)
	if e.recordWorkRequest == nil || len(normalized) == 0 {
		return
	}
	record := workdomain.WorkRequestRecordFromSubmitRequests(requestID, source, normalized)
	record.ParentLineage = append([]string(nil), batch.Metadata.ParentLineage...)
	e.recordWorkRequest(e.runtimeState.TickCount, record)
}

func (e *FactoryEngine) recordGeneratedSubmissionTokens(
	source string,
	normalized []workdomain.SubmitRequest,
	tokens []*factorytoken.Token,
	replacedSeededWorkIDs map[string]struct{},
) {
	parentIDs := make(map[string]struct{})
	removedSeededWorkIDs := make(map[string]struct{}, len(replacedSeededWorkIDs))
	for index, token := range tokens {
		if _, replace := replacedSeededWorkIDs[token.Color.WorkID]; replace {
			if _, removed := removedSeededWorkIDs[token.Color.WorkID]; !removed {
				e.removeSeededReplayWorkToken(token.Color.WorkID)
				removedSeededWorkIDs[token.Color.WorkID] = struct{}{}
			}
		}
		e.runtimeState.Marking.RecordParentChildRegistration(token)
		if token.Color.ParentID != "" {
			parentIDs[token.Color.ParentID] = struct{}{}
		}
		if e.recordSubmission != nil {
			e.recordSubmission(workdomain.FactorySubmissionRecord{SubmissionID: submissionRecordID(e.runtimeState.TickCount, source, index), ObservedTick: e.runtimeState.TickCount, Request: normalized[index], Source: source})
		}
		e.runtimeState.Marking.AddToken(token)
		if e.recordWorkInput != nil {
			e.recordWorkInput(e.runtimeState.TickCount, normalized[index], *token)
		}
	}
	for parentID := range parentIDs {
		e.runtimeState.Marking.CompleteParentChildRegistration(parentID)
	}
}

func (e *FactoryEngine) removeSeededReplayWorkToken(workID string) {
	for tokenID, token := range e.runtimeState.Marking.Tokens {
		if token == nil || token.Color.DataType == factorytoken.DataTypeResource || token.Color.WorkID != workID {
			continue
		}
		e.runtimeState.Marking.RemoveToken(tokenID)
	}
}
