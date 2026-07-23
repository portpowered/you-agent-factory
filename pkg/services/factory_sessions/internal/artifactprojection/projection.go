// Package artifactprojection owns Factory Session artifact projection policy.
package artifactprojection

import (
	"sort"
	"strings"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

const checkpointArtifactAuditModeFull = "FULL"

// StatesFromJavaScriptRuntime merges explicit artifact states with
// checkpoint-derived internal artifacts for one JavaScript session.
func StatesFromJavaScriptRuntime(
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
			AuditMode:   checkpointArtifactAuditModeFull,
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
