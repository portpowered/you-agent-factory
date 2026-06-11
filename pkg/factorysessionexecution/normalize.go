package factorysessionexecution

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

// NormalizeStartRequest validates and normalizes one durable execution start request.
func NormalizeStartRequest(req StartRequest) (StartRequest, error) {
	requestID := strings.TrimSpace(req.RequestID)
	if requestID == "" {
		return StartRequest{}, NewValidationError("requestId", "requestId is required")
	}

	source, err := normalizeSource(req.Source)
	if err != nil {
		return StartRequest{}, err
	}

	normalized := StartRequest{
		RequestID:       requestID,
		Source:          source,
		Args:            cloneArgs(req.Args),
		RequestedPolicy: cloneArgs(req.RequestedPolicy),
	}
	if req.Orchestrator != nil {
		override := *req.Orchestrator
		override.Kind = strings.TrimSpace(override.Kind)
		if len(override.Raw) > 0 {
			canonical, err := canonicalizeRawJSON(override.Raw)
			if err != nil {
				return StartRequest{}, NewValidationError("orchestrator", "orchestrator must be a JSON object")
			}
			encoded, err := json.Marshal(canonical)
			if err != nil {
				return StartRequest{}, fmt.Errorf("marshal orchestrator: %w", err)
			}
			override.Raw = encoded
		}
		normalized.Orchestrator = &override
	}
	if req.Wait != nil {
		wait := *req.Wait
		normalized.Wait = &wait
	}
	return normalized, nil
}

func normalizeSource(source Source) (Source, error) {
	switch source.Kind {
	case workflowsource.KindFactoryID:
		factoryID := strings.TrimSpace(source.FactoryID)
		if factoryID == "" {
			return Source{}, NewValidationError("source.factoryId", "factoryId is required when source.kind is FACTORY_ID")
		}
		return Source{Kind: source.Kind, FactoryID: factoryID}, nil
	case workflowsource.KindFactoryInline:
		if len(source.FactoryInline) == 0 {
			return Source{}, NewValidationError("source.factoryInline", "factoryInline is required when source.kind is FACTORY_INLINE")
		}
		canonical, err := canonicalizeRawJSON(source.FactoryInline)
		if err != nil {
			return Source{}, NewValidationError("source.factoryInline", "factoryInline must be a JSON object")
		}
		encoded, err := json.Marshal(canonical)
		if err != nil {
			return Source{}, fmt.Errorf("marshal factoryInline: %w", err)
		}
		return Source{Kind: source.Kind, FactoryInline: encoded}, nil
	case workflowsource.KindWorkflowFile:
		workflowFile := strings.TrimSpace(source.WorkflowFile)
		if workflowFile == "" {
			return Source{}, NewValidationError("source.workflowFile", "workflowFile is required when source.kind is WORKFLOW_FILE")
		}
		return Source{Kind: source.Kind, WorkflowFile: workflowFile}, nil
	case workflowsource.KindWorkflowName:
		workflowName := strings.TrimSpace(source.WorkflowName)
		if workflowName == "" {
			return Source{}, NewValidationError("source.workflowName", "workflowName is required when source.kind is WORKFLOW_NAME")
		}
		return Source{Kind: source.Kind, WorkflowName: workflowName}, nil
	case workflowsource.KindInlineWorkflow:
		if source.InlineWorkflow == nil {
			return Source{}, NewValidationError("source.inlineWorkflow", "inlineWorkflow is required when source.kind is INLINE_WORKFLOW")
		}
		inlineSource := strings.TrimSpace(source.InlineWorkflow.InlineSource)
		if inlineSource == "" {
			return Source{}, NewValidationError("source.inlineWorkflow.inlineSource", "inlineSource is required when source.kind is INLINE_WORKFLOW")
		}
		return Source{
			Kind: source.Kind,
			InlineWorkflow: &InlineWorkflowSource{
				Dialect:      strings.TrimSpace(source.InlineWorkflow.Dialect),
				InlineSource: inlineSource,
				Entrypoint:   strings.TrimSpace(source.InlineWorkflow.Entrypoint),
				Metadata:     cloneStringMap(source.InlineWorkflow.Metadata),
			},
		}, nil
	default:
		return Source{}, NewValidationError("source.kind", "source.kind must be one of FACTORY_ID, FACTORY_INLINE, WORKFLOW_FILE, WORKFLOW_NAME, or INLINE_WORKFLOW")
	}
}

func cloneArgs(args map[string]any) map[string]any {
	if len(args) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(args))
	for key, value := range args {
		cloned[key] = value
	}
	return cloned
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func cloneFakeSessionState(state *fakeSessionState) *fakeSessionState {
	if state == nil {
		return nil
	}
	cloned := &fakeSessionState{
		scenarioID:      state.scenarioID,
		session:         cloneSessionRead(state.session),
		dispatches:      cloneDispatchSummaries(state.dispatches),
		dispatchDetails: cloneDispatchDetails(state.dispatchDetails),
		artifacts:       cloneArtifactSummaries(state.artifacts),
		artifactDetails: cloneArtifactDetails(state.artifactDetails),
		result:          cloneResultRead(state.result),
		events:          make([]json.RawMessage, len(state.events)),
	}
	for index, event := range state.events {
		cloned.events[index] = append(json.RawMessage(nil), event...)
	}
	return cloned
}

func cloneSessionRead(session SessionReadResult) SessionReadResult {
	cloned := session
	cloned.ResolvedSource = cloneResolvedSource(session.ResolvedSource)
	cloned.Policy = clonePolicyProjection(session.Policy)
	if session.Progress != nil {
		progress := *session.Progress
		cloned.Progress = &progress
	}
	if session.ResultSummary != nil {
		summary := *session.ResultSummary
		cloned.ResultSummary = &summary
	}
	if session.Failure != nil {
		failure := *session.Failure
		cloned.Failure = &failure
	}
	if session.Lifecycle != nil {
		cloned.Lifecycle = cloneLifecycleTimestamps(session.Lifecycle)
	}
	if session.Budgets != nil {
		budgets := *session.Budgets
		cloned.Budgets = &budgets
	}
	cloned.Usage = cloneSessionUsage(session.Usage)
	cloned.PhaseSummaries = append([]PhaseSummary(nil), session.PhaseSummaries...)
	cloned.ArtifactRefs = append([]ArtifactRefSummary(nil), session.ArtifactRefs...)
	cloned.Links = session.Links
	return cloned
}

func cloneResolvedSource(source ResolvedSource) ResolvedSource {
	cloned := source
	cloned.ResolutionOrder = append([]string(nil), source.ResolutionOrder...)
	if len(source.Metadata) > 0 {
		cloned.Metadata = make(map[string]string, len(source.Metadata))
		for key, value := range source.Metadata {
			cloned.Metadata[key] = value
		}
	}
	return cloned
}

func clonePolicyProjection(policy PolicyProjection) PolicyProjection {
	cloned := PolicyProjection{
		EffectiveHash: policy.EffectiveHash,
	}
	if len(policy.Requested) > 0 {
		cloned.Requested = cloneArgs(policy.Requested)
	}
	if len(policy.Effective) > 0 {
		cloned.Effective = cloneArgs(policy.Effective)
	}
	return cloned
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneLifecycleTimestamps(lifecycle *LifecycleTimestamps) *LifecycleTimestamps {
	if lifecycle == nil {
		return nil
	}
	return &LifecycleTimestamps{
		QueuedAt:           cloneTimePtr(lifecycle.QueuedAt),
		AwaitingApprovalAt: cloneTimePtr(lifecycle.AwaitingApprovalAt),
		StartedAt:          cloneTimePtr(lifecycle.StartedAt),
		PausedAt:           cloneTimePtr(lifecycle.PausedAt),
		ResumedAt:          cloneTimePtr(lifecycle.ResumedAt),
		FinishedAt:         cloneTimePtr(lifecycle.FinishedAt),
		InterruptedAt:      cloneTimePtr(lifecycle.InterruptedAt),
		TerminatedAt:       cloneTimePtr(lifecycle.TerminatedAt),
		UpdatedAt:          cloneTimePtr(lifecycle.UpdatedAt),
	}
}

func cloneDispatchUsage(usage *DispatchUsage) *DispatchUsage {
	if usage == nil {
		return nil
	}
	cloned := *usage
	return &cloned
}

func cloneDispatchFailureDetail(detail *DispatchFailureDetail) *DispatchFailureDetail {
	if detail == nil {
		return nil
	}
	cloned := *detail
	return &cloned
}

func cloneDispatchSummary(summary DispatchSummary) DispatchSummary {
	cloned := summary
	cloned.ProviderSessionRefs = append([]ProviderSessionRef(nil), summary.ProviderSessionRefs...)
	cloned.OutputArtifactIDs = append([]string(nil), summary.OutputArtifactIDs...)
	cloned.Usage = cloneDispatchUsage(summary.Usage)
	cloned.Warnings = append([]DispatchWarning(nil), summary.Warnings...)
	cloned.FailureDetail = cloneDispatchFailureDetail(summary.FailureDetail)
	return cloned
}

func cloneDispatchSummaries(dispatches []DispatchSummary) []DispatchSummary {
	if len(dispatches) == 0 {
		return nil
	}
	cloned := make([]DispatchSummary, len(dispatches))
	for index, dispatch := range dispatches {
		cloned[index] = cloneDispatchSummary(dispatch)
	}
	return cloned
}

func cloneDispatchPetriProjection(petri *DispatchPetriProjection) *DispatchPetriProjection {
	if petri == nil {
		return nil
	}
	cloned := *petri
	return &cloned
}

func cloneDispatchJavaScriptProjection(js *DispatchJavaScriptProjection) *DispatchJavaScriptProjection {
	if js == nil {
		return nil
	}
	cloned := *js
	return &cloned
}

func cloneDispatchDetail(detail DispatchDetail) DispatchDetail {
	cloned := detail
	cloned.DispatchSummary = cloneDispatchSummary(detail.DispatchSummary)
	cloned.ArtifactIDs = append([]string(nil), detail.ArtifactIDs...)
	cloned.Petri = cloneDispatchPetriProjection(detail.Petri)
	cloned.JavaScript = cloneDispatchJavaScriptProjection(detail.JavaScript)
	return cloned
}

func cloneDispatchDetails(details map[string]DispatchDetail) map[string]DispatchDetail {
	if len(details) == 0 {
		return nil
	}
	cloned := make(map[string]DispatchDetail, len(details))
	for key, value := range details {
		cloned[key] = cloneDispatchDetail(value)
	}
	return cloned
}

func cloneArtifactRedactionCounts(counts *ArtifactRedactionCounts) *ArtifactRedactionCounts {
	if counts == nil {
		return nil
	}
	cloned := *counts
	return &cloned
}

func cloneArtifactRetrievalRef(ref *ArtifactRetrievalRef) *ArtifactRetrievalRef {
	if ref == nil {
		return nil
	}
	cloned := *ref
	return &cloned
}

func cloneArtifactSummary(summary ArtifactSummary) ArtifactSummary {
	cloned := summary
	cloned.CreatedAt = cloneTimePtr(summary.CreatedAt)
	cloned.RedactionCounts = cloneArtifactRedactionCounts(summary.RedactionCounts)
	cloned.RetrievalRef = cloneArtifactRetrievalRef(summary.RetrievalRef)
	return cloned
}

func cloneArtifactSummaries(artifacts []ArtifactSummary) []ArtifactSummary {
	if len(artifacts) == 0 {
		return nil
	}
	cloned := make([]ArtifactSummary, len(artifacts))
	for index, artifact := range artifacts {
		cloned[index] = cloneArtifactSummary(artifact)
	}
	return cloned
}

func cloneArtifactDetail(detail ArtifactDetail) ArtifactDetail {
	cloned := detail
	cloned.ArtifactSummary = cloneArtifactSummary(detail.ArtifactSummary)
	cloned.CaptureMetadata = cloneArgs(detail.CaptureMetadata)
	if len(detail.Content) > 0 {
		cloned.Content = append(json.RawMessage(nil), detail.Content...)
	}
	cloned.ContentRef = cloneArtifactRetrievalRef(detail.ContentRef)
	return cloned
}

func cloneArtifactDetails(details map[string]ArtifactDetail) map[string]ArtifactDetail {
	if len(details) == 0 {
		return nil
	}
	cloned := make(map[string]ArtifactDetail, len(details))
	for key, value := range details {
		cloned[key] = cloneArtifactDetail(value)
	}
	return cloned
}

func cloneResultRead(result ResultReadResult) ResultReadResult {
	cloned := result
	if len(result.PrimaryResult) > 0 {
		cloned.PrimaryResult = append(json.RawMessage(nil), result.PrimaryResult...)
	}
	cloned.ArtifactIDs = append([]string(nil), result.ArtifactIDs...)
	cloned.ArtifactRefs = append([]ArtifactRefSummary(nil), result.ArtifactRefs...)
	if result.Failure != nil {
		failure := *result.Failure
		cloned.Failure = &failure
	}
	if result.Availability != nil {
		availability := *result.Availability
		cloned.Availability = &availability
	}
	return cloned
}
