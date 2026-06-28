package factorysessionexecution

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	jsstore "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/store"
	workflowresult "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
	"github.com/portpowered/infinite-you/pkg/workcontent"
)

func applyRuntimeSessionFields(target *runtimeSessionState, source runtimeSessionState) {
	preservedResume := preserveRuntimeResumeState(*target)
	target.session = source.session
	target.result = source.result
	target.dispatches = cloneDispatchSummaries(source.dispatches)
	target.dispatchJavaScript = cloneDispatchJavaScriptProjections(source.dispatchJavaScript)
	target.dispatchStatusTransitions = cloneDispatchStatusTransitions(source.dispatchStatusTransitions)
	target.artifacts = cloneArtifactSummaries(source.artifacts)
	target.events = source.events
	restoreRuntimeResumeState(target, preservedResume)
}

type preservedRuntimeResumeState struct {
	runtimeRecords    []workflowruntime.RuntimeRecord
	checkpointSummary *jsstore.CheckpointSummary
	startRequest      *StartRequest
	resolvedSource    ResolvedSource
	sourceContent     string
	lifecycle         *LifecycleTimestamps
}

func preserveRuntimeResumeState(state runtimeSessionState) preservedRuntimeResumeState {
	return preservedRuntimeResumeState{
		runtimeRecords:    cloneRuntimeRecords(state.runtimeRecords),
		checkpointSummary: cloneCheckpointSummary(state.checkpointSummary),
		startRequest:      cloneStartRequestPtr(state.startRequest),
		resolvedSource:    state.resolvedSource,
		sourceContent:     state.sourceContent,
		lifecycle:         cloneLifecycleTimestamps(state.session.Lifecycle),
	}
}

func restoreRuntimeResumeState(state *runtimeSessionState, preserved preservedRuntimeResumeState) {
	if state == nil {
		return
	}
	state.runtimeRecords = mergeRuntimeRecords(preserved.runtimeRecords, state.runtimeRecords)
	if preserved.checkpointSummary != nil {
		state.checkpointSummary = cloneCheckpointSummary(preserved.checkpointSummary)
	}
	if preserved.startRequest != nil {
		state.startRequest = cloneStartRequestPtr(preserved.startRequest)
	}
	if preserved.resolvedSource.SourceRef != "" {
		state.resolvedSource = preserved.resolvedSource
	}
	if preserved.sourceContent != "" {
		state.sourceContent = preserved.sourceContent
	}
	mergeResumeLifecycleLineage(state, preserved.lifecycle)
}

func cloneLifecycleTimestamps(lifecycle *LifecycleTimestamps) *LifecycleTimestamps {
	if lifecycle == nil {
		return nil
	}
	cloned := &LifecycleTimestamps{}
	if lifecycle.QueuedAt != nil {
		cloned.QueuedAt = timePtr(lifecycle.QueuedAt.UTC())
	}
	if lifecycle.AwaitingApprovalAt != nil {
		cloned.AwaitingApprovalAt = timePtr(lifecycle.AwaitingApprovalAt.UTC())
	}
	if lifecycle.StartedAt != nil {
		cloned.StartedAt = timePtr(lifecycle.StartedAt.UTC())
	}
	if lifecycle.PausedAt != nil {
		cloned.PausedAt = timePtr(lifecycle.PausedAt.UTC())
	}
	if lifecycle.ResumedAt != nil {
		cloned.ResumedAt = timePtr(lifecycle.ResumedAt.UTC())
	}
	if lifecycle.FinishedAt != nil {
		cloned.FinishedAt = timePtr(lifecycle.FinishedAt.UTC())
	}
	if lifecycle.InterruptedAt != nil {
		cloned.InterruptedAt = timePtr(lifecycle.InterruptedAt.UTC())
	}
	if lifecycle.TerminatedAt != nil {
		cloned.TerminatedAt = timePtr(lifecycle.TerminatedAt.UTC())
	}
	if lifecycle.UpdatedAt != nil {
		cloned.UpdatedAt = timePtr(lifecycle.UpdatedAt.UTC())
	}
	return cloned
}

func mergeResumeLifecycleLineage(state *runtimeSessionState, preserved *LifecycleTimestamps) {
	if state == nil || preserved == nil {
		return
	}
	if state.session.Lifecycle == nil {
		state.session.Lifecycle = &LifecycleTimestamps{}
	}
	if preserved.StartedAt != nil {
		state.session.Lifecycle.StartedAt = timePtr(preserved.StartedAt.UTC())
	}
	if preserved.InterruptedAt != nil {
		state.session.Lifecycle.InterruptedAt = timePtr(preserved.InterruptedAt.UTC())
	}
	if preserved.ResumedAt != nil {
		state.session.Lifecycle.ResumedAt = timePtr(preserved.ResumedAt.UTC())
	}
	if preserved.PausedAt != nil {
		state.session.Lifecycle.PausedAt = timePtr(preserved.PausedAt.UTC())
	}
}

func applyTerminalRuntimeProjection(
	state *runtimeSessionState,
	terminal runtimeSessionState,
	outcome workflowruntime.Outcome,
) {
	priorSession := cloneSessionRead(state.session)
	priorResult := cloneResultRead(state.result)
	priorStatus := state.session.Status
	preserved := snapshotInterruptedDispatches(state)
	preservedEvents := extractDispatchInterruptedEvents(state.events)
	applyRuntimeSessionFields(state, terminal)
	if len(preserved) == 0 {
		return
	}
	if outcome.OK && priorStatus == LifecycleStatusResuming {
		state.session.StaleLease = false
		state.events = mergePreservedDispatchInterruptedEvents(
			rebuildRuntimeSessionCanonicalEvents(state),
			preservedEvents,
		)
		return
	}
	restoreInterruptedDispatchResultSuppression(state, preserved)
	finalizeInterruptedTerminalSession(state, priorSession, priorResult)
	state.events = mergePreservedDispatchInterruptedEvents(
		BuildCanonicalRuntimeSessionEvents(
			state.session,
			state.result,
			runtimeDispatchEventInputFromState(state),
		),
		preservedEvents,
	)
}

func applyRuntimeExecutionRecordProjection(
	state *runtimeSessionState,
	sessionID string,
	records []workflowruntime.RuntimeRecord,
	finishedAt time.Time,
) {
	recordProjection := ProjectRuntimeExecutionRecords(sessionID, records, finishedAt)
	if recordProjection.Phase != "" {
		state.session.Phase = recordProjection.Phase
	}
	state.dispatches = cloneDispatchSummaries(recordProjection.Dispatches)
	state.dispatchJavaScript = cloneDispatchJavaScriptProjections(recordProjection.DispatchJavaScript)
	state.dispatchStatusTransitions = cloneDispatchStatusTransitions(recordProjection.DispatchStatusTransitions)
	state.artifacts = cloneArtifactSummaries(recordProjection.Artifacts)
	state.session.Progress = &recordProjection.Progress
	state.session.ArtifactRefs = artifactRefsFromSummaries(state.artifacts)
	state.session.ArtifactCount = len(state.session.ArtifactRefs)
}

func applyRuntimeSuccessProjection(
	state *runtimeSessionState,
	sessionID string,
	outcome workflowruntime.Outcome,
	finishedAt time.Time,
) {
	applyRuntimeExecutionRecordProjection(state, sessionID, outcome.Records, finishedAt)

	projected, resultSummary, err := projectRuntimeSuccessResult(sessionID, outcome.Value, state.artifacts)
	if err != nil {
		state.session.Status = LifecycleStatusFailed
		state.session.Failure = &FailureSummary{
			Reason:  "WORKFLOW_RUNTIME_INVALID_RESULT",
			Message: err.Error(),
		}
		state.session.ResultSummary = &ResultSummary{
			ResultStatus: string(ResultStatusUnavailable),
		}
		state.result = ResultReadResult{
			SessionID:     sessionID,
			ResultStatus:  ResultStatusUnavailable,
			SessionStatus: LifecycleStatusFailed,
			Failure:       cloneFailureSummary(state.session.Failure),
			Availability:  defaultUnavailableAvailability(),
		}
		return
	}
	state.session.Status = LifecycleStatusSucceeded
	state.session.ResultSummary = resultSummary
	state.result = projected
}

func projectRuntimeSuccessResult(
	sessionID string,
	value workflowresult.TypedValue,
	artifacts []ArtifactSummary,
) (ResultReadResult, *ResultSummary, error) {
	parts, validation := workflowresult.ProjectPrimaryResult(sessionID, value, artifactStatesFromSummaries(artifacts))
	if validation.HasIssues() {
		return ResultReadResult{}, nil, fmt.Errorf("project primary result: %v", validation.Issues)
	}

	primaryJSON := workContentJSONFromParts(parts)
	result := ResultReadResult{
		SessionID:     sessionID,
		ResultStatus:  ResultStatusFinal,
		SessionStatus: LifecycleStatusSucceeded,
		PrimaryResult: primaryJSON,
		ArtifactIDs:   artifactIDsFromSummaries(artifacts),
	}
	summary := &ResultSummary{
		ResultStatus: string(ResultStatusFinal),
		Summary:      resultSummaryTextFromParts(parts),
	}
	return result, summary, nil
}

func finalizeInterruptedTerminalSession(
	state *runtimeSessionState,
	priorSession SessionReadResult,
	priorResult ResultReadResult,
) {
	if state == nil {
		return
	}
	interruptedAt := interruptedTerminalTimestamp(state.session, priorSession)
	if state.session.Lifecycle == nil {
		state.session.Lifecycle = &LifecycleTimestamps{}
	}
	if interruptedAt != nil {
		state.session.Lifecycle.InterruptedAt = timePtr(*interruptedAt)
		if state.session.Lifecycle.FinishedAt == nil {
			state.session.Lifecycle.FinishedAt = timePtr(*interruptedAt)
		}
	}
	state.session.Status = LifecycleStatusInterrupted
	state.session.Failure = nil

	canonicalStatus := canonicalResultStatus(priorResult, priorSession)
	switch canonicalStatus {
	case ResultStatusPartial, ResultStatusFailedWithPartial:
		if priorSession.ResultSummary != nil {
			summary := *priorSession.ResultSummary
			state.session.ResultSummary = &summary
		} else {
			state.session.ResultSummary = nil
		}
		if state.session.ResultSummary == nil {
			state.session.ResultSummary = &ResultSummary{ResultStatus: string(ResultStatusPartial)}
		} else {
			state.session.ResultSummary.ResultStatus = string(ResultStatusPartial)
		}
		state.result = cloneResultRead(priorResult)
		state.result.ResultStatus = ResultStatusPartial
		state.result.SessionStatus = LifecycleStatusInterrupted
		state.result.Mode = ResultModeFinal
		state.result.Availability = nil
	case ResultStatusFinal:
		fallthrough
	case ResultStatusNotReady, ResultStatusUnavailable:
		fallthrough
	default:
		state.session.ResultSummary = &ResultSummary{ResultStatus: string(ResultStatusUnavailable)}
		state.result = ResultReadResult{
			SessionID:     state.session.SessionID,
			ResultStatus:  ResultStatusUnavailable,
			SessionStatus: LifecycleStatusInterrupted,
			Mode:          ResultModeFinal,
			Availability: &ResultAvailabilityDetail{
				Reason:    "SESSION_INTERRUPTED",
				Message:   "Session was interrupted before a final result was available.",
				Retryable: false,
			},
		}
	}
}

func interruptedTerminalTimestamp(session, prior SessionReadResult) *time.Time {
	if session.Lifecycle != nil {
		if session.Lifecycle.InterruptedAt != nil {
			return timePtr(session.Lifecycle.InterruptedAt.UTC())
		}
		if session.Lifecycle.FinishedAt != nil {
			return timePtr(session.Lifecycle.FinishedAt.UTC())
		}
		if session.Lifecycle.UpdatedAt != nil {
			return timePtr(session.Lifecycle.UpdatedAt.UTC())
		}
	}
	if prior.Lifecycle != nil {
		if prior.Lifecycle.InterruptedAt != nil {
			return timePtr(prior.Lifecycle.InterruptedAt.UTC())
		}
		if prior.Lifecycle.UpdatedAt != nil {
			return timePtr(prior.Lifecycle.UpdatedAt.UTC())
		}
		if prior.Lifecycle.StartedAt != nil {
			return timePtr(prior.Lifecycle.StartedAt.UTC())
		}
	}
	return nil
}

func workContentJSONFromParts(parts []interfaces.WorkContentPart) json.RawMessage {
	content := workcontent.GeneratedPtrFromParts(parts)
	if content == nil {
		return nil
	}
	encoded, err := json.Marshal(content)
	if err != nil {
		return nil
	}
	return encoded
}

func resultSummaryTextFromParts(parts []interfaces.WorkContentPart) string {
	for _, part := range parts {
		if part.Type.Normalized() == interfaces.WorkContentPartTypeText {
			if text := strings.TrimSpace(part.Text); text != "" {
				return text
			}
		}
	}
	return ""
}

func artifactStatesFromSummaries(artifacts []ArtifactSummary) []interfaces.FactorySessionArtifactState {
	if len(artifacts) == 0 {
		return nil
	}
	states := make([]interfaces.FactorySessionArtifactState, 0, len(artifacts))
	for _, artifact := range artifacts {
		states = append(states, interfaces.FactorySessionArtifactState{
			ID:          artifact.ID,
			Kind:        artifact.Kind,
			Visibility:  artifact.Visibility,
			Label:       artifact.Label,
			ContentHash: artifact.ContentHash,
			SizeBytes:   artifact.SizeBytes,
			AuditMode:   artifact.AuditMode,
		})
	}
	return states
}
