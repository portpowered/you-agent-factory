package recording

import (
	"encoding/json"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
)

// ApplyJavaScriptProjectionFacts retains the privacy-safe facts already owned by
// the canonical Factory Session JavaScript projection.
func ApplyJavaScriptProjectionFacts(
	facts *CanonicalFacts,
	javascript *interfaces.FactorySessionJavaScriptRuntimeState,
) {
	if facts == nil || javascript == nil {
		return
	}
	facts.ArgumentsDigest = strings.TrimSpace(javascript.ArgsDigest)
	if count := len(javascript.Checkpoints); count > 0 {
		checkpoint := javascript.Checkpoints[count-1]
		facts.Checkpoint = &CanonicalCheckpoint{
			ID: checkpoint.ID, Label: checkpoint.Label, Summary: checkpoint.Summary, Timestamp: checkpoint.Timestamp,
		}
		if checkpoint.ArtifactRef != nil {
			facts.Checkpoint.ArtifactID = checkpoint.ArtifactRef.ID
		}
	}
	if result := resultFromJavaScriptProjection(javascript); result != nil {
		facts.Result = result
	}
}

func resultFromJavaScriptProjection(javascript *interfaces.FactorySessionJavaScriptRuntimeState) *CanonicalResult {
	status := strings.TrimSpace(javascript.ResultStatus)
	if status == "" && len(javascript.PrimaryResult) == 0 {
		return nil
	}
	result := &CanonicalResult{Status: status, Mode: "final"}
	if status == "PARTIAL" || status == "FAILED_WITH_PARTIAL" {
		result.Mode = "partial"
	}
	if len(javascript.PrimaryResult) == 0 {
		return result
	}
	if raw, err := json.Marshal(javascript.PrimaryResult); err == nil {
		result.PrimaryResult = raw
	}
	for _, part := range javascript.PrimaryResult {
		if artifactID := strings.TrimSpace(part.ArtifactID); artifactID != "" {
			result.ArtifactIDs = append(result.ArtifactIDs, artifactID)
		}
	}
	return result
}
