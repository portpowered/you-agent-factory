package factorysessionexecution

import (
	"fmt"
	"sort"
	"strings"
	"time"

	workflowresult "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
	workflowruntime "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/runtime"
)

// RuntimeExecutionProjection carries durable dispatch, artifact, phase, and progress
// state projected from ordered workflow runtime records.
type RuntimeExecutionProjection struct {
	Phase                     string
	PhaseCount                int
	Dispatches                []DispatchSummary
	DispatchJavaScript        map[string]DispatchJavaScriptProjection
	DispatchStatusTransitions map[string][]DispatchStatus
	Artifacts                 []ArtifactSummary
	Progress                  ProgressCounts
}

// ProjectRuntimeExecutionRecords maps ordered runtime host-effect records into
// durable session dispatch, artifact, phase, and progress projections.
func ProjectRuntimeExecutionRecords(
	sessionID string,
	records []workflowruntime.RuntimeRecord,
	observedAt time.Time,
) RuntimeExecutionProjection {
	projection := RuntimeExecutionProjection{}
	if len(records) == 0 {
		return projection
	}

	currentPhase := ""
	dispatchOrder := make([]string, 0)
	dispatchByID := make(map[string]DispatchSummary)
	dispatchJavaScript := make(map[string]DispatchJavaScriptProjection)
	dispatchStatusTransitions := make(map[string][]DispatchStatus)
	artifactByID := make(map[string]ArtifactSummary)

	for _, record := range records {
		switch record.Kind {
		case workflowruntime.RecordKindPhase:
			if record.Phase == nil {
				continue
			}
			name := strings.TrimSpace(record.Phase.Name)
			if name == "" {
				continue
			}
			currentPhase = name
			projection.PhaseCount++
			projection.Phase = name
		case workflowruntime.RecordKindArtifact:
			if record.Artifact == nil {
				continue
			}
			summary := artifactSummaryFromRuntimeRecord(sessionID, *record.Artifact, observedAt)
			artifactByID[summary.ID] = summary
		case workflowruntime.RecordKindChildDispatch:
			if record.ChildDispatch == nil {
				continue
			}
			summary := dispatchSummaryFromChildRecord(currentPhase, *record.ChildDispatch)
			dispatchByID[summary.ID] = summary
			dispatchJavaScript[summary.ID] = dispatchJavaScriptFromChildRecord(*record.ChildDispatch)
			appendDispatchStatusTransition(dispatchStatusTransitions, summary.ID, summary.Status)
			if _, seen := indexOfString(dispatchOrder, summary.ID); !seen {
				dispatchOrder = append(dispatchOrder, summary.ID)
			}
			if artifact, ok := childArtifactFromDispatch(sessionID, *record.ChildDispatch, observedAt); ok {
				artifactByID[artifact.ID] = artifact
			}
		}
	}

	projection.Dispatches = make([]DispatchSummary, 0, len(dispatchOrder))
	for _, dispatchID := range dispatchOrder {
		projection.Dispatches = append(projection.Dispatches, dispatchByID[dispatchID])
	}
	projection.DispatchJavaScript = dispatchJavaScript
	projection.DispatchStatusTransitions = dispatchStatusTransitions
	projection.Artifacts = orderedArtifactSummaries(artifactByID)
	projection.Progress = progressCountsFromDispatches(projection.Dispatches, projection.PhaseCount)
	return projection
}

func appendDispatchStatusTransition(
	transitions map[string][]DispatchStatus,
	dispatchID string,
	status DispatchStatus,
) {
	if dispatchID == "" || status == "" {
		return
	}
	existing := transitions[dispatchID]
	if len(existing) > 0 && existing[len(existing)-1] == status {
		return
	}
	transitions[dispatchID] = append(existing, status)
}

func artifactSummaryFromRuntimeRecord(
	sessionID string,
	record workflowruntime.ArtifactRecord,
	observedAt time.Time,
) ArtifactSummary {
	return ArtifactSummary{
		ID:           record.ID,
		Kind:         record.Kind,
		Visibility:   record.Visibility,
		Label:        record.Label,
		ContentHash:  record.ContentHash,
		SizeBytes:    record.SizeBytes,
		CreatedAt:    timePtr(observedAt.UTC()),
		RetrievalRef: artifactRetrievalRefForSession(sessionID, record.ID),
	}
}

func childArtifactFromDispatch(
	sessionID string,
	child workflowruntime.ChildDispatchRecord,
	observedAt time.Time,
) (ArtifactSummary, bool) {
	if strings.TrimSpace(child.Status) != workflowruntime.ChildDispatchStatusCompleted {
		return ArtifactSummary{}, false
	}
	parsed, issues := workflowresult.ParseArtifactURI(strings.TrimSpace(child.ArtifactRef))
	if len(issues) > 0 || parsed.ArtifactID == "" {
		return ArtifactSummary{}, false
	}
	return ArtifactSummary{
		ID:           parsed.ArtifactID,
		Kind:         "CHILD_OUTPUT",
		Visibility:   "WORKFLOW_RUNTIME",
		Label:        child.Label,
		DispatchID:   child.DispatchID,
		CreatedAt:    timePtr(observedAt.UTC()),
		RetrievalRef: artifactRetrievalRefForSession(sessionID, parsed.ArtifactID),
	}, true
}

func dispatchSummaryFromChildRecord(currentPhase string, child workflowruntime.ChildDispatchRecord) DispatchSummary {
	summary := DispatchSummary{
		ID:           child.DispatchID,
		Status:       DispatchStatus(strings.TrimSpace(child.Status)),
		DispatchKind: "JAVASCRIPT_AGENT",
		Phase:        currentPhase,
		Label:        child.Label,
		Attempt:      1,
		RunnerID:     strings.TrimSpace(child.RunnerID),
		Model:        strings.TrimSpace(child.Model),
	}
	if ref := strings.TrimSpace(child.ProviderSessionRef); ref != "" {
		provider := strings.TrimSpace(child.Provider)
		if provider == "" && strings.TrimSpace(child.ExecutionMode) == workflowruntime.ChildExecutionModeFake {
			provider = "fake"
		}
		summary.Provider = provider
		summary.ProviderSessionRefs = []ProviderSessionRef{{
			Provider: provider,
			Kind:     "AGENT",
			ID:       ref,
		}}
	}
	if artifactID := artifactIDFromRef(child.ArtifactRef); artifactID != "" &&
		summary.Status == DispatchStatusCompleted {
		summary.OutputArtifactIDs = []string{artifactID}
	}
	if summary.Status == DispatchStatusFailed {
		summary.FailureDetail = dispatchFailureDetailFromChildRecord(child)
	}
	return summary
}

func dispatchFailureDetailFromChildRecord(child workflowruntime.ChildDispatchRecord) *DispatchFailureDetail {
	message := strings.TrimSpace(child.FailureMessage)
	if message == "" {
		return nil
	}
	detail := &DispatchFailureDetail{Message: message}
	if reason := strings.TrimSpace(child.FailureReason); reason != "" {
		detail.Reason = reason
	} else {
		detail.Reason = workflowruntime.ChildExecutionFailureReason
	}
	if errorClass := strings.TrimSpace(child.FailureErrorClass); errorClass != "" {
		detail.ErrorClass = errorClass
	}
	return detail
}

func dispatchJavaScriptFromChildRecord(child workflowruntime.ChildDispatchRecord) DispatchJavaScriptProjection {
	return DispatchJavaScriptProjection{
		TaskKind:      "AGENT",
		TaskLabel:     child.Label,
		ExecutionMode: strings.TrimSpace(child.ExecutionMode),
	}
}

func cloneDispatchJavaScriptProjections(
	source map[string]DispatchJavaScriptProjection,
) map[string]DispatchJavaScriptProjection {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[string]DispatchJavaScriptProjection, len(source))
	for id, projection := range source {
		cloned[id] = projection
	}
	return cloned
}

func cloneRuntimeRecords(records []workflowruntime.RuntimeRecord) []workflowruntime.RuntimeRecord {
	if len(records) == 0 {
		return nil
	}
	cloned := make([]workflowruntime.RuntimeRecord, len(records))
	for i, record := range records {
		cloned[i] = cloneRuntimeRecord(record)
	}
	return cloned
}

func cloneRuntimeRecord(record workflowruntime.RuntimeRecord) workflowruntime.RuntimeRecord {
	cloned := record
	if record.Phase != nil {
		phase := *record.Phase
		cloned.Phase = &phase
	}
	if record.Artifact != nil {
		artifact := *record.Artifact
		cloned.Artifact = &artifact
	}
	if record.ChildDispatch != nil {
		child := *record.ChildDispatch
		cloned.ChildDispatch = &child
	}
	return cloned
}

func cloneDispatchStatusTransitions(
	source map[string][]DispatchStatus,
) map[string][]DispatchStatus {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[string][]DispatchStatus, len(source))
	for id, transitions := range source {
		cloned[id] = append([]DispatchStatus(nil), transitions...)
	}
	return cloned
}

func artifactIDFromRef(raw string) string {
	parsed, issues := workflowresult.ParseArtifactURI(strings.TrimSpace(raw))
	if len(issues) > 0 {
		return ""
	}
	return parsed.ArtifactID
}

func artifactRetrievalRefForSession(sessionID, artifactID string) *ArtifactRetrievalRef {
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(artifactID) == "" {
		return nil
	}
	return &ArtifactRetrievalRef{
		Href:   fmt.Sprintf("/factory-sessions/%s/artifacts/%s", sessionID, artifactID),
		Method: "GET",
	}
}

func orderedArtifactSummaries(artifactByID map[string]ArtifactSummary) []ArtifactSummary {
	if len(artifactByID) == 0 {
		return nil
	}
	ids := make([]string, 0, len(artifactByID))
	for id := range artifactByID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	artifacts := make([]ArtifactSummary, 0, len(ids))
	for _, id := range ids {
		artifacts = append(artifacts, artifactByID[id])
	}
	return artifacts
}

func progressCountsFromDispatches(dispatches []DispatchSummary, phaseCount int) ProgressCounts {
	progress := ProgressCounts{PhaseCount: phaseCount}
	for _, dispatch := range dispatches {
		progress.TotalDispatches++
		switch dispatch.Status {
		case DispatchStatusCompleted:
			progress.CompletedDispatches++
		case DispatchStatusFailed:
			progress.FailedDispatches++
		case DispatchStatusQueued, DispatchStatusRunning:
			progress.InFlightDispatches++
		}
	}
	return progress
}

func indexOfString(values []string, target string) (int, bool) {
	for index, value := range values {
		if value == target {
			return index, true
		}
	}
	return -1, false
}
