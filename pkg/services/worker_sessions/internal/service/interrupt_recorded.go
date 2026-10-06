package service

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const interruptContextMaxBytes = 65536

// Freeze the recipe and bounded captured prefix before the durable intent or
// source stop. Admission uses these same values even if the source emits more.
func (r *registry) freezeInterruptInput(ctx context.Context, plan interruptPlan) (interruptPlan, error) {
	plan.request = plan.request.Normalize()
	target, err := r.freezeControlTarget(plan.request.SourceWorkerSessionID)
	if err != nil {
		return plan, err
	}
	plan, err = r.capturedInterruptPlan(ctx, plan, target.capture)
	if err != nil {
		return plan, err
	}
	if plan.request.ResumeMode == "recorded" {
		_, metadata, exists := r.loadObservationState(plan.request.SourceWorkerSessionID)
		if r.logs == nil || !exists || !metadata.direct {
			return plan, workersessions.ErrInterruptExecutionUnavailable
		}
		plan.context, plan.truncated, err = r.readInterruptContext(ctx, plan, target.capture)
		if err != nil {
			return plan, err
		}
	}
	// Validate size and privacy before cancellation, including component paths
	// that intentionally have no recording identity.
	if _, err := encodeInterruptInput(plan); err != nil {
		return plan, err
	}
	_, metadata, direct := r.loadObservationState(plan.request.SourceWorkerSessionID)
	if direct && metadata.direct && r.logs != nil {
		if err := r.restart.ValidateWorkerRestartRecipe(ctx, plan.request.SuccessorWorkerSessionID, interruptSuccessorExecution(plan)); err != nil {
			return plan, err
		}
	}
	return plan, nil
}

func interruptSuccessorExecution(plan interruptPlan) workers.WorkstationDispatchRequest {
	execution := continuationExecution(plan.execution, continuationDispatchID(plan.dispatchID, plan.request.SuccessorWorkerSessionID), plan.request.ReplacementMessage, plan.reference)
	if plan.request.ResumeMode == "recorded" {
		execution.Execution.Continuation = nil
		execution.Execution.UserMessage = "Captured context (truncated=" + strconv.FormatBool(plan.truncated) + "):\n" + plan.context + "\nReplacement message:\n" + plan.request.ReplacementMessage
	}
	return execution
}

func (r *registry) readInterruptContext(ctx context.Context, plan interruptPlan, target recordings.WorkerControlTarget) (string, bool, error) {
	request := recordings.WorkerCapturedActivityRequest{WorkerSessionID: plan.request.SourceWorkerSessionID, Limit: 100}
	var contextText strings.Builder
	truncated := false
	var head uint64
	for {
		page, err := r.logs.reader.ReadWorkerCapturedActivity(ctx, request)
		if err != nil || page.Catalog.RecordingID != target.RecordingID || page.Catalog.WorkerSessionID != target.WorkerSessionID ||
			page.Catalog.FactorySessionID != target.FactorySessionID || page.Catalog.RecordingGenerationID != target.RecordingGenerationID ||
			page.Catalog.OwnerEpoch != target.OwnerEpoch || page.OwnerLost || page.Health == recordings.WorkerRecordingStatusDegraded {
			return "", false, workersessions.ErrInterruptExecutionUnavailable
		}
		if head == 0 {
			head = uint64(page.Catalog.CommittedPosition)
		}
		if len(page.Records) == 0 {
			return "", false, workersessions.ErrInterruptExecutionUnavailable
		}
		for _, captured := range page.Records {
			if uint64(captured.Record.ID.Position) > head {
				return contextText.String(), truncated, nil
			}
			var draft workers.Draft
			if json.Unmarshal(captured.Record.Payload, &draft) != nil {
				return "", false, workersessions.ErrInterruptExecutionUnavailable
			}
			if draft.Kind != workers.KindMessage && draft.Kind != workers.KindTool {
				continue
			}
			// Captured summaries are data for a fresh prompt, never tool replay.
			fragment := string(draft.Payload) + "\n"
			remaining := interruptContextMaxBytes - contextText.Len()
			bounded, cut := boundedInterruptContext(fragment, remaining)
			contextText.WriteString(bounded)
			truncated = truncated || cut || captured.Truncated
			if cut || contextText.Len() == interruptContextMaxBytes {
				return contextText.String(), true, nil
			}
		}
		if uint64(page.Records[len(page.Records)-1].Record.ID.Position) >= head || page.NextToken == "" {
			return contextText.String(), truncated, nil
		}
		request.NextToken = page.NextToken
	}
}

func boundedInterruptContext(text string, limit int) (string, bool) {
	if len(text) <= limit {
		return text, false
	}
	text = text[:limit]
	for !utf8.ValidString(text) {
		text = text[:len(text)-1]
	}
	return text, true
}

func interruptContinuationAssociation(req workersessions.ContinueRequest, execution workers.WorkstationDispatchRequest, snapshot continuationSourceSnapshot) *workersessions.ProviderSessionAssociation {
	if execution.Execution.Continuation == nil || snapshot.session.ProviderSessionAssociation == nil {
		return nil
	}
	return continuationAssociation(req, execution, snapshot.turnID, snapshot.session.ProviderSessionAssociation.Reference)
}

// The interrupt owner already synced input and joined the exact source. Reuse
// continuation admission/lineage without re-reading mutable source input.
func (r *registry) admitInterruptSuccessor(plan interruptPlan) (workersessions.ContinueResult, error) {
	req := workersessions.ContinueRequest{RequestID: interruptContinuationRequestID(plan.request.RequestID),
		SourceWorkerSessionID: plan.request.SourceWorkerSessionID, SuccessorWorkerSessionID: plan.request.SuccessorWorkerSessionID,
		FollowUpInput: plan.request.ReplacementMessage}
	r.mu.Lock()
	if r.stopping {
		r.mu.Unlock()
		return workersessions.ContinueResult{}, workersessions.ErrInterruptServerStopping
	}
	source := r.sessions[req.SourceWorkerSessionID]
	if source.State != workersessions.StateCanceled || source.SuccessorWorkerSessionID != "" || r.continuationSources[source.ID] != "" {
		r.mu.Unlock()
		return workersessions.ContinueResult{}, workersessions.ErrInterruptSourceConflict
	}
	execution := interruptSuccessorExecution(plan)
	if _, exists := r.sessions[req.SuccessorWorkerSessionID]; exists {
		r.mu.Unlock()
		return workersessions.ContinueResult{}, workersessions.ErrInterruptSourceConflict
	}
	if _, exists := r.dispatchOwners[execution.Execution.Dispatch.DispatchID]; exists {
		r.mu.Unlock()
		return workersessions.ContinueResult{}, workersessions.ErrInterruptSourceConflict
	}
	snapshot := continuationSourceSnapshot{session: source, execution: plan.execution, dispatchID: plan.dispatchID,
		executor: plan.supervision.executor, clock: plan.supervision.clock, scheduler: plan.supervision.scheduler, turnID: plan.supervision.turnID}
	if metadata := r.observations[source.ID]; metadata != nil {
		snapshot.direct = metadata.direct
	}
	if r.continueReplays == nil {
		r.continueReplays = make(map[string]*continueReplay)
	}
	replay := r.storeContinuationReservationLocked(req, continueTuple{sourceID: source.ID, successorID: req.SuccessorWorkerSessionID, input: req.FollowUpInput}, snapshot, execution)
	replay.plan.interrupt = true
	r.mu.Unlock()
	defer r.finishStart()
	result, err := r.continueReserved(replay.plan)
	r.finishContinueReplay(replay, result, err)
	return result, err
}
