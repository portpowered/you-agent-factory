package factorysessionexecution

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	workflowresult "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
	"github.com/portpowered/infinite-you/pkg/workcontent"
)

func projectRuntimeSuccessResult(sessionID string, value workflowresult.TypedValue, artifacts []ArtifactSummary) (ResultReadResult, *ResultSummary, error) {
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
