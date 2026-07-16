package apisurface

import (
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	workflowresult "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	contentcontract "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
)

// BuildWorkflowSessionLiveResult projects the live terminal session result read shape.
func BuildWorkflowSessionLiveResult(input workflowresult.SessionResultInput) factoryapi.FactorySessionLiveResult {
	return WorkflowSessionLiveResultToAPI(workflowresult.BuildLiveSessionResult(input))
}

// WorkflowSessionLiveResultToAPI maps one JavaScript result-owner projection
// to the generated live Factory Session result contract.
func WorkflowSessionLiveResultToAPI(result workflowresult.LiveSessionResult) factoryapi.FactorySessionLiveResult {
	response := factoryapi.FactorySessionLiveResult{
		SessionId: result.SessionID,
		Status:    factoryapi.FactorySessionStatus(result.Status),
	}
	if refs := WorkflowCheckpointRefsToAPI(result.CheckpointRefs); len(refs) > 0 {
		response.CheckpointRefs = &refs
	}
	response.ResultArtifactRef = WorkflowArtifactRefToAPI(result.ResultArtifactRef)
	return response
}

// WorkflowSessionPartialResultToAPI maps one JavaScript result-owner partial
// projection to the generated live Factory Session partial-result contract.
func WorkflowSessionPartialResultToAPI(result workflowresult.PartialSessionResult) factoryapi.FactorySessionPartialResult {
	response := factoryapi.FactorySessionPartialResult{
		SessionId: result.SessionID,
		Phase:     result.Phase,
	}
	if len(result.CheckpointRefs) > 0 {
		refs := WorkflowCheckpointRefsToAPI(result.CheckpointRefs)
		response.CheckpointRefs = &refs
	}
	response.PartialResultArtifactRef = WorkflowArtifactRefToAPI(result.PartialResultArtifactRef)
	return response
}

// BuildWorkflowSessionResult projects the durable terminal session result read shape.
func BuildWorkflowSessionResult(input workflowresult.SessionResultInput) factoryapi.FactorySessionResult {
	result := workflowresult.BuildSessionResult(input)
	response := factoryapi.FactorySessionResult{
		SessionId:    result.SessionID,
		ResultStatus: factoryapi.FactorySessionResultStatus(result.ResultStatus),
	}
	response.PrimaryResult = contentcontract.GeneratedPtrFromParts(result.PrimaryResult)
	if len(result.ArtifactIDs) > 0 {
		ids := append([]string(nil), result.ArtifactIDs...)
		response.ArtifactIds = &ids
	}
	if len(result.ArtifactRefs) > 0 {
		refs := make([]factoryapi.FactoryArtifactRef, 0, len(result.ArtifactRefs))
		for index := range result.ArtifactRefs {
			refs = append(refs, *WorkflowArtifactRefToAPI(&result.ArtifactRefs[index]))
		}
		response.ArtifactRefs = &refs
	}
	return response
}

// BuildWorkflowSessionResultUpdatedPayload projects the SESSION_RESULT_UPDATED
// event payload from the shared session result contract.
func BuildWorkflowSessionResultUpdatedPayload(input workflowresult.SessionResultInput) factoryapi.SessionResultUpdatedEventPayload {
	result := workflowresult.BuildSessionResultUpdatedPayload(input)
	payload := factoryapi.SessionResultUpdatedEventPayload{
		ResultStatus:  factoryapi.FactoryEventSessionResultStatus(result.ResultStatus),
		ResultSummary: contentcontract.GeneratedPtrFromParts(result.ResultSummary),
	}
	if len(result.ArtifactIDs) > 0 {
		ids := append([]string(nil), result.ArtifactIDs...)
		payload.ArtifactIds = &ids
	}
	return payload
}

// WorkflowCheckpointRefsToAPI maps Factory-owned checkpoint refs at the public boundary.
func WorkflowCheckpointRefsToAPI(refs []interfaces.FactorySessionJavaScriptCheckpointEventRef) []factoryapi.FactorySessionJavaScriptCheckpointRef {
	projected := make([]factoryapi.FactorySessionJavaScriptCheckpointRef, 0, len(refs))
	for _, ref := range refs {
		projected = append(projected, factoryapi.FactorySessionJavaScriptCheckpointRef{
			Id: ref.ID, Label: ref.Label, Summary: ref.Summary, Timestamp: ref.Timestamp,
			ArtifactRef: WorkflowArtifactRefToAPI(ref.ArtifactRef),
		})
	}
	return projected
}

// WorkflowArtifactRefToAPI maps one Factory-owned artifact ref at the public boundary.
func WorkflowArtifactRefToAPI(ref *interfaces.FactoryArtifactRef) *factoryapi.FactoryArtifactRef {
	if ref == nil {
		return nil
	}
	return &factoryapi.FactoryArtifactRef{
		Id: ref.ID, Kind: factoryapi.FactoryArtifactKind(ref.Kind),
		Visibility:  factoryapi.FactoryArtifactVisibility(ref.Visibility),
		ContentHash: ref.ContentHash, SizeBytes: ref.SizeBytes,
	}
}

// WorkflowArtifactsToAPI maps session-owned artifact projections at the public boundary.
func WorkflowArtifactsToAPI(artifacts []interfaces.FactoryArtifact) *[]factoryapi.FactoryArtifact {
	if len(artifacts) == 0 {
		return nil
	}
	projected := make([]factoryapi.FactoryArtifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		projected = append(projected, factoryapi.FactoryArtifact{
			AuditMode:       artifactAuditModeToAPI(artifact.AuditMode),
			CaptureMetadata: artifactCaptureMetadataToAPI(artifact.CaptureMetadata),
			ContentHash:     artifact.ContentHash,
			Id:              artifact.ID,
			Kind:            factoryapi.FactoryArtifactKind(artifact.Kind),
			Label:           artifact.Label,
			RedactionCounts: artifactRedactionCountsToAPI(artifact.RedactionCounts),
			SizeBytes:       artifact.SizeBytes,
			Summary:         artifact.Summary,
			Visibility:      factoryapi.FactoryArtifactVisibility(artifact.Visibility),
		})
	}
	return &projected
}

func artifactAuditModeToAPI(value *string) *factoryapi.FactoryArtifactAuditMode {
	if value == nil {
		return nil
	}
	converted := factoryapi.FactoryArtifactAuditMode(*value)
	return &converted
}

func artifactCaptureMetadataToAPI(
	metadata *interfaces.FactoryArtifactCaptureMetadata,
) *factoryapi.FactoryArtifactCaptureMetadata {
	if metadata == nil {
		return nil
	}
	return &factoryapi.FactoryArtifactCaptureMetadata{
		CapturedAt:       metadata.CapturedAt,
		MimeType:         metadata.MIMEType,
		SourceDispatchId: metadata.SourceDispatchID,
	}
}

func artifactRedactionCountsToAPI(
	counts *interfaces.FactoryArtifactRedactionCounts,
) *factoryapi.FactoryArtifactRedactionCounts {
	if counts == nil {
		return nil
	}
	return &factoryapi.FactoryArtifactRedactionCounts{
		Paths: counts.Paths, Secrets: counts.Secrets, Tokens: counts.Tokens,
	}
}

// NormalizeWorkflowSourceRequest is the shared API, CLI, MCP, and website entry
// point for workflow source lookup and artifact-root validation.
func NormalizeWorkflowSourceRequest(req workflowsource.Request, ctx workflowsource.Context) workflowsource.Resolution {
	return workflowsource.Resolve(req, ctx)
}
