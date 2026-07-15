package factorysessions

import (
	"sort"
	"strings"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func projectedCheckpointArtifactStates(
	checkpoints []interfaces.JavaScriptCheckpointRecord,
) []interfaces.FactorySessionArtifactState {
	if len(checkpoints) == 0 {
		return nil
	}
	artifacts := make([]interfaces.FactorySessionArtifactState, 0, len(checkpoints))
	for _, checkpoint := range checkpoints {
		artifactID := strings.TrimSpace(checkpoint.ArtifactID)
		if artifactID == "" {
			artifactID = checkpoint.ID
		}
		artifact := interfaces.FactorySessionArtifactState{
			ID:          artifactID,
			Kind:        interfaces.JavaScriptCheckpointArtifactKind,
			Visibility:  interfaces.JavaScriptCheckpointArtifactVisibility,
			Label:       checkpoint.Label,
			Summary:     checkpoint.Summary,
			AuditMode:   string(factoryapi.FactoryArtifactAuditModeFULL),
			ContentHash: checkpoint.ContentHash,
			SizeBytes:   checkpoint.SizeBytes,
			CapturedAt:  checkpoint.Timestamp,
		}
		if !checkpoint.Timestamp.IsZero() {
			artifact.CaptureMetadata = map[string]string{
				"capturedAt": checkpoint.Timestamp.UTC().Format(time.RFC3339),
			}
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts
}

// ArtifactStatesFromJavaScriptRuntime merges explicit artifact states with
// checkpoint-derived internal artifacts for one JavaScript session.
func ArtifactStatesFromJavaScriptRuntime(
	checkpoints []interfaces.JavaScriptCheckpointRecord,
	states []interfaces.FactorySessionArtifactState,
) []interfaces.FactorySessionArtifactState {
	artifacts := append([]interfaces.FactorySessionArtifactState(nil), states...)
	artifacts = append(artifacts, projectedCheckpointArtifactStates(checkpoints)...)
	if len(artifacts) == 0 {
		return nil
	}
	sort.Slice(artifacts, func(i, j int) bool {
		return artifacts[i].ID < artifacts[j].ID
	})
	return artifacts
}

func projectedArtifacts(states []interfaces.FactorySessionArtifactState) *[]factoryapi.FactoryArtifact {
	if len(states) == 0 {
		return nil
	}
	projected := make([]factoryapi.FactoryArtifact, 0, len(states))
	for _, state := range states {
		projected = append(projected, projectedArtifact(state))
	}
	return &projected
}

func projectedArtifact(state interfaces.FactorySessionArtifactState) factoryapi.FactoryArtifact {
	kind := factoryapi.FactoryArtifactKind(strings.TrimSpace(state.Kind))
	if kind == "" {
		kind = factoryapi.FactoryArtifactKindCHILDRESULT
	}
	visibility := factoryapi.FactoryArtifactVisibility(strings.TrimSpace(state.Visibility))
	if visibility == "" {
		visibility = factoryapi.FactoryArtifactVisibilityPUBLIC
	}
	artifact := factoryapi.FactoryArtifact{
		Id:         strings.TrimSpace(state.ID),
		Kind:       kind,
		Visibility: visibility,
	}
	if label := strings.TrimSpace(state.Label); label != "" {
		artifact.Label = &label
	}
	if summary := strings.TrimSpace(state.Summary); summary != "" {
		artifact.Summary = &summary
	}
	if auditMode := strings.TrimSpace(state.AuditMode); auditMode != "" {
		mode := factoryapi.FactoryArtifactAuditMode(auditMode)
		artifact.AuditMode = &mode
	}
	if hash := strings.TrimSpace(state.ContentHash); hash != "" {
		artifact.ContentHash = &hash
	}
	if state.SizeBytes > 0 {
		size := state.SizeBytes
		artifact.SizeBytes = &size
	}
	if redactions := projectedArtifactRedactionCounts(state.RedactionCounts); redactions != nil {
		artifact.RedactionCounts = redactions
	}
	if metadata := projectedArtifactCaptureMetadata(state); metadata != nil {
		artifact.CaptureMetadata = metadata
	}
	return artifact
}

func projectedArtifactRedactionCounts(
	counts map[string]int,
) *factoryapi.FactoryArtifactRedactionCounts {
	if len(counts) == 0 {
		return nil
	}
	projected := &factoryapi.FactoryArtifactRedactionCounts{}
	hasValue := false
	if value, ok := counts["secrets"]; ok && value > 0 {
		secrets := int32(value)
		projected.Secrets = &secrets
		hasValue = true
	}
	if value, ok := counts["paths"]; ok && value > 0 {
		paths := int32(value)
		projected.Paths = &paths
		hasValue = true
	}
	if value, ok := counts["tokens"]; ok && value > 0 {
		tokens := int32(value)
		projected.Tokens = &tokens
		hasValue = true
	}
	if !hasValue {
		return nil
	}
	return projected
}

func projectedArtifactCaptureMetadata(
	state interfaces.FactorySessionArtifactState,
) *factoryapi.FactoryArtifactCaptureMetadata {
	metadata := state.CaptureMetadata
	if len(metadata) == 0 && state.CapturedAt.IsZero() {
		return nil
	}
	projected := &factoryapi.FactoryArtifactCaptureMetadata{}
	hasValue := false
	if !state.CapturedAt.IsZero() {
		capturedAt := state.CapturedAt.UTC()
		projected.CapturedAt = &capturedAt
		hasValue = true
	}
	if dispatchID := strings.TrimSpace(metadata["sourceDispatchId"]); dispatchID != "" {
		projected.SourceDispatchId = &dispatchID
		hasValue = true
	}
	if mimeType := strings.TrimSpace(metadata["mimeType"]); mimeType != "" {
		projected.MimeType = &mimeType
		hasValue = true
	}
	if !hasValue {
		return nil
	}
	return projected
}

func artifactCaptureMetadata(
	capturedAt time.Time,
	sourceDispatchID string,
	mimeType string,
) map[string]string {
	metadata := make(map[string]string)
	if !capturedAt.IsZero() {
		metadata["capturedAt"] = capturedAt.UTC().Format(time.RFC3339)
	}
	if sourceDispatchID = strings.TrimSpace(sourceDispatchID); sourceDispatchID != "" {
		metadata["sourceDispatchId"] = sourceDispatchID
	}
	if mimeType = strings.TrimSpace(mimeType); mimeType != "" {
		metadata["mimeType"] = mimeType
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}
