package projections

import (
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	sessionprojectionfacts "github.com/portpowered/infinite-you/pkg/services/recordings/internal/sessionprojectionfacts"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// IncrementalSessionProjection applies canonical events in append order and
// retains only the event-derived facts needed by live Factory Session reads.
// The owning event ledger serializes Apply with canonical appends; callers of
// SnapshotSessionProjectionFacts receive detached values.
type IncrementalSessionProjection struct {
	reducer *factoryWorldReducer
}

// NewIncrementalSessionProjection creates an empty append-order projection.
func NewIncrementalSessionProjection() *IncrementalSessionProjection {
	return &IncrementalSessionProjection{reducer: newFactoryWorldReducer(0)}
}

// Apply incorporates one canonical event into the live session projection.
func (projection *IncrementalSessionProjection) Apply(event interfaces.FactoryEvent) error {
	if projection == nil {
		return nil
	}
	if projection.reducer == nil {
		projection.reducer = newFactoryWorldReducer(0)
	}
	return projection.reducer.apply(event)
}

// SnapshotSessionProjectionFacts returns detached event-derived session facts.

func (projection *IncrementalSessionProjection) SnapshotSessionProjectionFacts() sessionprojectionfacts.SessionProjectionFacts {
	if projection == nil || projection.reducer == nil {
		return sessionprojectionfacts.SessionProjectionFacts{}
	}
	state := projection.reducer.stateValue
	facts := sessionprojectionfacts.SessionProjectionFacts{
		PendingHumanApprovals: clonePendingHumanApprovals(state.PendingHumanApprovalsByID),
		JavaScriptRuntime:     cloneJavaScriptRuntimeState(state.JavaScriptRuntime),
		SessionBracket:        cloneSessionBracketState(state.SessionBracket),
	}
	return facts
}

func clonePendingHumanApprovals(
	values map[string]interfaces.FactoryWorldHumanApproval,
) map[string]interfaces.FactoryWorldHumanApproval {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]interfaces.FactoryWorldHumanApproval, len(values))
	for id, value := range values {
		cloned[id] = cloneHumanApproval(value)
	}
	return cloned
}

func cloneHumanApproval(value interfaces.FactoryWorldHumanApproval) interfaces.FactoryWorldHumanApproval {
	cloned := value
	cloned.Decisions = append([]interfaces.HumanApprovalDecision(nil), value.Decisions...)
	cloned.WorkItemIDs = append([]string(nil), value.WorkItemIDs...)
	cloned.TraceIDs = append([]string(nil), value.TraceIDs...)
	if value.WorkstationDescription != nil {
		description := *value.WorkstationDescription
		description.Locales = append([]string(nil), value.WorkstationDescription.Locales...)
		if value.WorkstationDescription.Values != nil {
			description.Values = make(map[string]string, len(value.WorkstationDescription.Values))
			for locale, text := range value.WorkstationDescription.Values {
				description.Values[locale] = text
			}
		}
		cloned.WorkstationDescription = &description
	}
	return cloned
}

func cloneJavaScriptRuntimeState(
	state *interfaces.FactorySessionJavaScriptRuntimeState,
) *interfaces.FactorySessionJavaScriptRuntimeState {
	if state == nil {
		return nil
	}
	cloned := *state
	cloned.Phases = append([]string(nil), state.Phases...)
	cloned.Checkpoints = cloneJavaScriptCheckpoints(state.Checkpoints)
	cloned.Dispatches = cloneSessionDispatches(state.Dispatches)
	cloned.Artifacts = cloneSessionArtifacts(state.Artifacts)
	cloned.PrimaryResult = work.CloneWorkContentParts(state.PrimaryResult)
	return &cloned
}

func cloneJavaScriptCheckpoints(
	checkpoints []interfaces.FactorySessionJavaScriptCheckpointRef,
) []interfaces.FactorySessionJavaScriptCheckpointRef {
	if len(checkpoints) == 0 {
		return nil
	}
	cloned := make([]interfaces.FactorySessionJavaScriptCheckpointRef, len(checkpoints))
	for index, checkpoint := range checkpoints {
		cloned[index] = checkpoint
		cloned[index].Warnings = append([]interfaces.FactorySessionDispatchWarning(nil), checkpoint.Warnings...)
		if checkpoint.ArtifactRef != nil {
			artifactRef := *checkpoint.ArtifactRef
			cloned[index].ArtifactRef = &artifactRef
		}
	}
	return cloned
}

func cloneSessionDispatches(
	dispatches []interfaces.FactorySessionDispatchState,
) []interfaces.FactorySessionDispatchState {
	if len(dispatches) == 0 {
		return nil
	}
	cloned := make([]interfaces.FactorySessionDispatchState, len(dispatches))
	for index, dispatch := range dispatches {
		cloned[index] = dispatch
		cloned[index].RelatedWorkIDs = append([]string(nil), dispatch.RelatedWorkIDs...)
		cloned[index].ArtifactIDs = append([]string(nil), dispatch.ArtifactIDs...)
		cloned[index].Warnings = append([]interfaces.FactorySessionDispatchWarning(nil), dispatch.Warnings...)
		if dispatch.Usage != nil {
			usage := *dispatch.Usage
			cloned[index].Usage = &usage
		}
		if dispatch.FailureDetail != nil {
			failure := *dispatch.FailureDetail
			cloned[index].FailureDetail = &failure
		}
		if dispatch.Petri != nil {
			petri := *dispatch.Petri
			cloned[index].Petri = &petri
		}
		if dispatch.JavaScript != nil {
			javascript := *dispatch.JavaScript
			cloned[index].JavaScript = &javascript
		}
	}
	return cloned
}

func cloneSessionArtifacts(
	artifacts []interfaces.FactorySessionArtifactState,
) []interfaces.FactorySessionArtifactState {
	if len(artifacts) == 0 {
		return nil
	}
	cloned := make([]interfaces.FactorySessionArtifactState, len(artifacts))
	for index, artifact := range artifacts {
		cloned[index] = artifact
		if artifact.RedactionCounts != nil {
			cloned[index].RedactionCounts = make(map[string]int, len(artifact.RedactionCounts))
			for key, count := range artifact.RedactionCounts {
				cloned[index].RedactionCounts[key] = count
			}
		}
		if artifact.CaptureMetadata != nil {
			cloned[index].CaptureMetadata = make(map[string]string, len(artifact.CaptureMetadata))
			for key, value := range artifact.CaptureMetadata {
				cloned[index].CaptureMetadata[key] = value
			}
		}
	}
	return cloned
}

func cloneSessionBracketState(
	bracket *interfaces.FactoryWorldSessionBracketState,
) *interfaces.FactoryWorldSessionBracketState {
	if bracket == nil {
		return nil
	}
	cloned := *bracket
	cloned.ResultSummary = work.CloneWorkContentParts(bracket.ResultSummary)
	cloned.ArtifactIDs = append([]string(nil), bracket.ArtifactIDs...)
	if bracket.DispatchCounts != nil {
		dispatchCounts := *bracket.DispatchCounts
		cloned.DispatchCounts = &dispatchCounts
	}
	cloned.FailureDetail = workerexecution.CloneFailureDetail(bracket.FailureDetail)
	return &cloned
}
