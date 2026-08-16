package executor

import (
	"strings"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	completionEvidenceTaskComplete  = "task_complete"
	completionEvidenceTurnCompleted = "turn_completed"
)

func completionEvidenceValues(resp workerexecution.InferenceResponse) []string {
	values := make([]string, 0, 2)
	if resp.Diagnostics != nil {
		values = append(values, resp.Diagnostics.Metadata[workerexecution.ProviderResponseMetadataCompletionEvidence])
		if resp.Diagnostics.Provider != nil {
			values = append(values, resp.Diagnostics.Provider.ResponseMetadata[workerexecution.ProviderResponseMetadataCompletionEvidence])
		}
	}
	return values
}

// completionValidationFailure keeps the detached normalizer's provider
// completion policy available to the detached agent-run normalizer.
func completionValidationFailure(resp workerexecution.InferenceResponse) (string, string) {
	for _, value := range completionEvidenceValues(resp) {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case completionEvidenceTaskComplete, completionEvidenceTurnCompleted:
			return "provider completion evidence was contradictory", "contradictory_completion"
		}
	}
	if strings.TrimSpace(resp.Content) != "" {
		return "provider completion evidence was incomplete", "missing_completion_evidence"
	}
	return "provider completion evidence was missing", "missing_completion_evidence"
}
