package guards_batch

import (
	"fmt"
)

// executorReviewMismatchCause records which subsystem owns an observed executor or
// review queue divergence. Story 001 uses these labels without introducing new
// customer-facing vocabulary.
type executorReviewMismatchCause string

const (
	executorReviewCauseActiveRuntimeBehavior   executorReviewMismatchCause = "active_runtime_behavior"
	executorReviewCauseProjectionDrift         executorReviewMismatchCause = "projection_drift"
	executorReviewCauseDuplicateReviewCreation executorReviewMismatchCause = "duplicate_review_creation"
	executorReviewCauseFailedPostProcessing    executorReviewMismatchCause = "failed_post_processing"
	executorReviewCauseHistoricalResidualState executorReviewMismatchCause = "historical_residual_queue_state"
)

// executorReviewPlannerDisposition is the follow-up planner classification for a
// residual recovery lane.
type executorReviewPlannerDisposition string

const (
	executorReviewDispositionComplete              executorReviewPlannerDisposition = "complete"
	executorReviewDispositionSafeManualRepair      executorReviewPlannerDisposition = "safe_manual_repair"
	executorReviewDispositionSupersededQueueNoise  executorReviewPlannerDisposition = "superseded_queue_noise"
	executorReviewDispositionNeedsRuntimeReconcile executorReviewPlannerDisposition = "needs_runtime_reconcile"
)

// queueWorkSnapshot is one durable queue row from `you work list --json`.
type queueWorkSnapshot struct {
	WorkID                 string
	Name                   string
	WorkTypeName           string
	StateName              string
	StateType              string
	TraceID                string
	CurrentChainingTraceID string
	ChainingTraceDepth     int
}

// executorReviewLaneClassification is reviewer-verifiable output for one named
// recovery lane.
type executorReviewLaneClassification struct {
	LaneName           string
	RecoveryTraceID    string
	SpawnedTraceID     string
	TaskWorkID         string
	ReviewWorkIDs      []string
	IdeaWorkID         string
	MismatchCause      executorReviewMismatchCause
	PlannerDisposition executorReviewPlannerDisposition
	WorktreeComplete   bool
}

func workItemsByType(items []queueWorkSnapshot, workType string) []queueWorkSnapshot {
	out := make([]queueWorkSnapshot, 0, len(items))
	for _, item := range items {
		if item.WorkTypeName == workType {
			out = append(out, item)
		}
	}
	return out
}

func workItemsByTrace(items []queueWorkSnapshot, traceID string) []queueWorkSnapshot {
	if traceID == "" {
		return nil
	}
	out := make([]queueWorkSnapshot, 0, len(items))
	for _, item := range items {
		if item.TraceID == traceID || item.CurrentChainingTraceID == traceID {
			out = append(out, item)
		}
	}
	return out
}

func distinctTraceIDs(items []queueWorkSnapshot) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		traceID := item.CurrentChainingTraceID
		if traceID == "" {
			traceID = item.TraceID
		}
		if traceID == "" {
			continue
		}
		if _, ok := seen[traceID]; ok {
			continue
		}
		seen[traceID] = struct{}{}
		out = append(out, traceID)
	}
	return out
}

func activeReviewInitCount(items []queueWorkSnapshot) int {
	count := 0
	for _, item := range workItemsByType(items, "review") {
		if item.StateName == "init" && item.StateType != "TERMINAL" && item.StateType != "FAILED" {
			count++
		}
	}
	return count
}

func classifyExecutorReviewLaneFromQueueEvidence(
	laneName string,
	items []queueWorkSnapshot,
	worktreeComplete bool,
) (executorReviewLaneClassification, error) {
	if len(items) == 0 {
		return executorReviewLaneClassification{}, fmt.Errorf("no queue evidence for %q", laneName)
	}

	traces := distinctTraceIDs(items)
	recoveryTrace := ""
	for _, item := range items {
		if item.WorkTypeName == "idea" && item.StateName == "to-complete" {
			recoveryTrace = item.CurrentChainingTraceID
			if recoveryTrace == "" {
				recoveryTrace = item.TraceID
			}
			break
		}
	}
	if recoveryTrace == "" && len(traces) > 0 {
		recoveryTrace = traces[0]
	}

	spawnedTrace := ""
	for _, traceID := range traces {
		if traceID != recoveryTrace {
			spawnedTrace = traceID
			break
		}
	}

	result := executorReviewLaneClassification{
		LaneName:         laneName,
		RecoveryTraceID:  recoveryTrace,
		SpawnedTraceID:   spawnedTrace,
		WorktreeComplete: worktreeComplete,
	}
	populateLaneWorkIDs(&result, items)

	return classifyLaneFromWorkShape(result, items), nil
}

func populateLaneWorkIDs(result *executorReviewLaneClassification, items []queueWorkSnapshot) {
	for _, item := range workItemsByType(items, "idea") {
		if item.StateName == "to-complete" {
			result.IdeaWorkID = item.WorkID
			break
		}
	}
	for _, item := range workItemsByType(items, "task") {
		if item.StateName == "failed" || item.StateName == "in-review" || item.StateName == "init" {
			result.TaskWorkID = item.WorkID
			break
		}
	}
	for _, item := range workItemsByType(items, "review") {
		if item.StateName == "init" {
			result.ReviewWorkIDs = append(result.ReviewWorkIDs, item.WorkID)
		}
	}
}

func classifyLaneFromWorkShape(
	result executorReviewLaneClassification,
	items []queueWorkSnapshot,
) executorReviewLaneClassification {
	reviewInitCount := activeReviewInitCount(items)
	failedTasks, inReviewTasks := taskStateCounts(items)

	switch {
	case reviewInitCount > 1:
		result.MismatchCause = executorReviewCauseDuplicateReviewCreation
		result.PlannerDisposition = executorReviewDispositionNeedsRuntimeReconcile
	case result.SpawnedTraceID != "":
		return classifySplitTraceLane(result, items, failedTasks)
	case failedTasks > 0 && result.WorktreeComplete:
		result.MismatchCause = executorReviewCauseFailedPostProcessing
		result.PlannerDisposition = executorReviewDispositionSafeManualRepair
	case inReviewTasks > 0 && reviewInitCount == 1 && result.WorktreeComplete:
		result.MismatchCause = executorReviewCauseFailedPostProcessing
		result.PlannerDisposition = executorReviewDispositionSafeManualRepair
	default:
		result.MismatchCause = executorReviewCauseHistoricalResidualState
		result.PlannerDisposition = executorReviewDispositionSupersededQueueNoise
	}
	return result
}

func taskStateCounts(items []queueWorkSnapshot) (failedTasks int, inReviewTasks int) {
	for _, item := range workItemsByType(items, "task") {
		switch item.StateName {
		case "failed":
			failedTasks++
		case "in-review":
			inReviewTasks++
		}
	}
	return failedTasks, inReviewTasks
}

func classifySplitTraceLane(
	result executorReviewLaneClassification,
	items []queueWorkSnapshot,
	failedTasks int,
) executorReviewLaneClassification {
	spawnedItems := workItemsByTrace(items, result.SpawnedTraceID)
	recoveryItems := workItemsByTrace(items, result.RecoveryTraceID)
	if len(recoveryItems) > 0 && failedPlanOnTrace(recoveryItems) && completedPlanOnTrace(spawnedItems) {
		if failedTasks > 0 && result.WorktreeComplete {
			result.MismatchCause = executorReviewCauseFailedPostProcessing
			result.PlannerDisposition = executorReviewDispositionSafeManualRepair
			return result
		}
	}
	result.MismatchCause = executorReviewCauseHistoricalResidualState
	result.PlannerDisposition = executorReviewDispositionSupersededQueueNoise
	return result
}

func failedPlanOnTrace(items []queueWorkSnapshot) bool {
	for _, item := range workItemsByType(items, "plan") {
		if item.StateName == "failed" {
			return true
		}
	}
	return false
}

func completedPlanOnTrace(items []queueWorkSnapshot) bool {
	for _, item := range workItemsByType(items, "plan") {
		if item.StateName == "complete" {
			return true
		}
	}
	return false
}

// executorReviewManualRepairPreconditions records observable evidence required
// before a bounded `you work move` repair is allowed for executor/review lanes
// with failed-post-processing residue. This is not a generic completion shortcut:
// it applies only when worktree delivery is already complete, the authoritative
// trace still shows exactly one task:failed, and duplicate review:init defects
// are absent so runtime reconcile—not manual token deletion—owns review cleanup.
type executorReviewManualRepairPreconditions struct {
	SafeManualRepairDisposition bool
	FailedPostProcessingCause   bool
	WorktreeComplete            bool
	AuthoritativeFailedTaskOnly bool
	AuthoritativePlanComplete   bool
	NoDuplicateReviewInit       bool
}

func (p executorReviewManualRepairPreconditions) AllowsBoundedExecutorReviewManualRepair() bool {
	return p.SafeManualRepairDisposition &&
		p.FailedPostProcessingCause &&
		p.WorktreeComplete &&
		p.AuthoritativeFailedTaskOnly &&
		p.AuthoritativePlanComplete &&
		p.NoDuplicateReviewInit
}

func evaluateExecutorReviewManualRepairPreconditions(
	classification executorReviewLaneClassification,
	items []queueWorkSnapshot,
) executorReviewManualRepairPreconditions {
	authoritativeTrace := classification.SpawnedTraceID
	if authoritativeTrace == "" {
		authoritativeTrace = classification.RecoveryTraceID
	}
	traceItems := workItemsByTrace(items, authoritativeTrace)

	return executorReviewManualRepairPreconditions{
		SafeManualRepairDisposition: classification.PlannerDisposition == executorReviewDispositionSafeManualRepair,
		FailedPostProcessingCause:   classification.MismatchCause == executorReviewCauseFailedPostProcessing,
		WorktreeComplete:            classification.WorktreeComplete,
		AuthoritativeFailedTaskOnly: failedTaskCountOnTrace(traceItems) == 1 && classification.TaskWorkID != "",
		AuthoritativePlanComplete:   completedPlanOnTrace(traceItems),
		NoDuplicateReviewInit:       activeReviewInitCount(traceItems) == 0,
	}
}

func failedTaskCountOnTrace(items []queueWorkSnapshot) int {
	count := 0
	for _, item := range workItemsByType(items, "task") {
		if item.StateName == "failed" {
			count++
		}
	}
	return count
}
