package factorysessionexecution

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	jsstore "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/store"
	workflowpolicy "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
)

// ResumeInterruptedSession reconstructs one interrupted checkpointed session from
// persisted checkpoint summaries plus durable session state and continues execution.
func (s *JavaScriptRuntimeService) ResumeInterruptedSession(
	ctx context.Context,
	sessionID string,
	req ResumeSessionRequest,
) (AsyncStartResult, error) {
	if err := ctx.Err(); err != nil {
		return AsyncStartResult{}, err
	}
	id, err := NormalizeSessionID(sessionID)
	if err != nil {
		return AsyncStartResult{}, err
	}
	if strings.TrimSpace(req.RequestID) == "" {
		return AsyncStartResult{}, NewValidationError("requestId", "requestId is required")
	}

	state, err := s.snapshotSessionState(id)
	if err != nil {
		return AsyncStartResult{}, err
	}
	if state.session.Status != LifecycleStatusInterrupted {
		return AsyncStartResult{}, &ControlError{
			Operation: LifecycleControlResume,
			Outcome:   LifecycleControlOutcomeInvalidState,
			Status:    state.session.Status,
			Message:   "session is not interrupted and cannot be resumed from checkpoint summaries",
		}
	}
	if state.checkpointSummary == nil {
		return AsyncStartResult{}, NewValidationError("checkpointSummary", "persisted checkpoint summary is required to resume an interrupted session")
	}
	if state.startRequest == nil || strings.TrimSpace(state.sourceContent) == "" {
		return AsyncStartResult{}, NewValidationError("session", "persisted start metadata is required to resume an interrupted session")
	}

	s.mu.Lock()
	if existing, ok := s.sessions[id]; ok && existing.runCancel != nil {
		s.mu.Unlock()
		return AsyncStartResult{}, &ControlError{
			Operation: LifecycleControlResume,
			Outcome:   LifecycleControlOutcomeInvalidState,
			Status:    existing.session.Status,
			Message:   "session is already active in this runtime instance",
		}
	}
	resumingAt := time.Now().UTC()
	state.session.Status = LifecycleStatusResuming
	state.result.SessionStatus = LifecycleStatusResuming
	if state.session.Lifecycle == nil {
		state.session.Lifecycle = &LifecycleTimestamps{}
	}
	state.session.Lifecycle.ResumedAt = &resumingAt
	resumed := cloneRuntimeSessionState(&state)
	resumed.events = rebuildRuntimeSessionCanonicalEvents(&resumed)
	s.sessions[id] = &resumed
	s.mu.Unlock()

	normalized := *state.startRequest
	resolved := state.resolvedSource
	sourceContent := state.sourceContent
	policyResolution := workflowpolicy.Resolution{
		Policy: workflowPolicyFromSessionPolicy(state.session.Policy),
		Hash:   strings.TrimSpace(state.session.Policy.EffectiveHash),
	}

	runCtx, runCancel := workflowRunContext(context.Background(), policyResolution.Policy)
	s.mu.Lock()
	if active, ok := s.sessions[id]; ok {
		active.runCancel = runCancel
	}
	s.mu.Unlock()

	go s.runResumedAsyncSession(
		runCtx,
		id,
		normalized,
		resolved,
		sourceContent,
		policyResolution,
		state.checkpointSummary,
		state.runtimeRecords,
		resumingAt,
	)

	snapshot, err := s.snapshotSessionState(id)
	if err != nil {
		return AsyncStartResult{}, err
	}
	return s.asyncStartFromState(snapshot), nil
}

func (s *JavaScriptRuntimeService) runResumedAsyncSession(
	runCtx context.Context,
	sessionID string,
	normalized StartRequest,
	resolved ResolvedSource,
	sourceContent string,
	policyResolution workflowpolicy.Resolution,
	checkpointSummary *jsstore.CheckpointSummary,
	priorRecords []workflowruntime.RuntimeRecord,
	startedAt time.Time,
) {
	defer func() {
		s.mu.Lock()
		if state, ok := s.sessions[sessionID]; ok {
			state.runCancel = nil
		}
		s.mu.Unlock()
	}()

	resumeContext := workflowruntime.ResumeContextFromCheckpointSummary(
		workflowruntime.CompletedCheckpointSummary{
			CompletedDispatchIDs: checkpointSummary.CompletedDispatchIDs,
			CheckpointState:      checkpointSummary.CheckpointState,
		},
		priorRecords,
	)
	outcome, err := s.invokeWorkflowRuntimeWithResume(
		runCtx,
		normalized,
		resolved,
		sourceContent,
		policyResolution,
		sessionID,
		&resumeContext,
	)

	s.mu.Lock()
	state, ok := s.sessions[sessionID]
	if !ok {
		s.mu.Unlock()
		return
	}
	mergedRecords := mergeRuntimeRecords(priorRecords, outcome.Records)
	outcome.Records = mergedRecords
	state.runtimeRecords = mergedRecords
	if checkpointSummary != nil {
		state.checkpointSummary = cloneCheckpointSummary(checkpointSummary)
	}

	if err != nil {
		failureOutcome := workflowruntime.Outcome{
			OK: false,
			Failure: workflowruntime.Failure{
				Code:    workflowruntime.CodeScriptError,
				Message: err.Error(),
			},
			Records: outcome.Records,
		}
		terminal := projectRuntimeSessionState(sessionID, normalized, resolved, policyResolution, failureOutcome, startedAt)
		s.applyTerminalRuntimeState(state, terminal, failureOutcome, startedAt)
		persistState := cloneRuntimeSessionState(state)
		s.mu.Unlock()
		_ = s.persistSessionSnapshot(persistState)
		return
	}

	terminal := projectRuntimeSessionState(sessionID, normalized, resolved, policyResolution, outcome, startedAt)
	s.applyTerminalRuntimeState(state, terminal, outcome, startedAt)
	if state.checkpointSummary == nil {
		state.checkpointSummary = latestCheckpointSummaryFromRuntime(sessionID, state, state.runtimeRecords)
	}
	persistState := cloneRuntimeSessionState(state)
	s.mu.Unlock()
	_ = s.persistSessionSnapshot(persistState)
}

func (s *JavaScriptRuntimeService) invokeWorkflowRuntimeWithResume(
	ctx context.Context,
	normalized StartRequest,
	resolved ResolvedSource,
	sourceContent string,
	policyResolution workflowpolicy.Resolution,
	sessionID string,
	resume *workflowruntime.ResumeContext,
) (workflowruntime.Outcome, error) {
	argsJSON, err := marshalStartArgs(normalized.Args)
	if err != nil {
		return workflowruntime.Outcome{}, err
	}
	return workflowruntime.Run(ctx, workflowruntime.Request{
		Source:    sourceContent,
		SourceRef: resolved.SourceRef,
		SessionID: sessionID,
		Args:      argsJSON,
		Metadata:  workflowMetadataFromResolved(resolved, normalized),
		Policy:    policyResolution.Policy,
		Resume:    resume,
	}, s.childExecutorHooks(resolveChildExecutorMode(s.childExecutorMode, normalized)))
}

func mergeRuntimeRecords(existing, resumed []workflowruntime.RuntimeRecord) []workflowruntime.RuntimeRecord {
	if len(existing) == 0 {
		return cloneRuntimeRecords(resumed)
	}
	if len(resumed) == 0 {
		return cloneRuntimeRecords(existing)
	}
	merged := make([]workflowruntime.RuntimeRecord, 0, len(existing)+len(resumed))
	merged = append(merged, cloneRuntimeRecords(existing)...)
	merged = append(merged, cloneRuntimeRecords(resumed)...)
	return merged
}

func latestCheckpointSummaryFromRuntime(
	sessionID string,
	state *runtimeSessionState,
	records []workflowruntime.RuntimeRecord,
) *jsstore.CheckpointSummary {
	if state == nil {
		return nil
	}
	return jsstore.LatestCheckpointSummaryFromRecords(jsstore.CheckpointSummaryInput{
		SessionID:  sessionID,
		Phase:      state.session.Phase,
		SourceHash: state.session.SourceHash,
		PolicyHash: state.session.Policy.EffectiveHash,
		Records:    records,
	})
}

func cloneCheckpointSummary(summary *jsstore.CheckpointSummary) *jsstore.CheckpointSummary {
	if summary == nil {
		return nil
	}
	cloned := *summary
	if len(summary.CompletedDispatchIDs) > 0 {
		cloned.CompletedDispatchIDs = append([]string(nil), summary.CompletedDispatchIDs...)
	}
	if len(summary.PendingDispatchIDs) > 0 {
		cloned.PendingDispatchIDs = append([]string(nil), summary.PendingDispatchIDs...)
	}
	if len(summary.ArtifactIDs) > 0 {
		cloned.ArtifactIDs = append([]string(nil), summary.ArtifactIDs...)
	}
	if len(summary.CheckpointState) > 0 {
		cloned.CheckpointState = make(map[string]any, len(summary.CheckpointState))
		for key, value := range summary.CheckpointState {
			cloned.CheckpointState[key] = value
		}
	}
	return &cloned
}

func workflowPolicyFromSessionPolicy(policy PolicyProjection) workflowpolicy.EffectivePolicy {
	if len(policy.Effective) == 0 {
		return workflowpolicy.DefaultEffectivePolicy()
	}
	encoded, err := json.Marshal(policy.Effective)
	if err != nil {
		return workflowpolicy.DefaultEffectivePolicy()
	}
	var effective workflowpolicy.EffectivePolicy
	if err := json.Unmarshal(encoded, &effective); err != nil {
		return workflowpolicy.DefaultEffectivePolicy()
	}
	return effective
}
