package factory

import (
	"encoding/json"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/work"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// ProjectPrimaryResult maps one validated workflow result to WorkContent parts.
func ProjectPrimaryResult(sessionID string, value TypedValue, artifacts []interfaces.FactorySessionArtifactState) ([]work.WorkContentPart, Result) {
	validation := ValidateTypedValue(value)
	if validation.HasIssues() {
		return nil, validation
	}
	if len(value.JSON) == 0 {
		return nil, Result{}
	}
	var decoded any
	if err := json.Unmarshal(value.JSON, &decoded); err != nil {
		return nil, Result{Issues: []Issue{{
			Code:    CodeNonJSONValue,
			Message: "workflow result must be JSON-compatible: " + err.Error(),
			Path:    "$",
		}}}
	}
	parts, issues := projectDecodedValue(sessionID, decoded, artifacts, "$")
	if len(issues) > 0 {
		return nil, Result{Issues: issues}
	}
	if len(parts) == 0 {
		return nil, Result{}
	}
	return parts, Result{}
}

type sessionResultProjectionOperation struct{}

// NewSessionResultProjectionOperation constructs the Factory Runtime-owned
// canonical result projection operation.
func NewSessionResultProjectionOperation() SessionResultProjectionOperation {
	return sessionResultProjectionOperation{}
}

// ProjectSessionResults derives all customer-visible result facts once from a
// canonical input and returns mutually detached projections.
func (sessionResultProjectionOperation) ProjectSessionResults(input SessionResultInput) SessionResultProjection {
	durable := projectSessionResult(input)
	return SessionResultProjection{
		Live:    projectLiveSessionResult(input),
		Durable: durable,
		Updated: projectSessionResultUpdatedPayload(input.Status, durable),
	}
}

func projectLiveSessionResult(input SessionResultInput) LiveSessionResult {
	result := LiveSessionResult{
		SessionID: strings.TrimSpace(input.SessionID),
		Status:    input.Status,
	}
	if len(input.CheckpointRefs) > 0 {
		result.CheckpointRefs = cloneCheckpointRefs(input.CheckpointRefs)
	}
	if input.ResultArtifact != nil {
		copied := cloneArtifactRef(*input.ResultArtifact)
		result.ResultArtifactRef = &copied
	}
	return result
}

func projectSessionResult(input SessionResultInput) SessionResult {
	result := SessionResult{
		SessionID:    strings.TrimSpace(input.SessionID),
		ResultStatus: resultStatusFromSessionStatus(input.Status),
	}
	if parts, validation := ProjectPrimaryResult(input.SessionID, input.PrimaryValue, input.Artifacts); !validation.HasIssues() && len(parts) > 0 {
		result.PrimaryResult = work.CloneWorkContentParts(parts)
	}
	artifactIDs, artifactRefs := projectArtifactProjection(input)
	result.ArtifactIDs = artifactIDs
	result.ArtifactRefs = artifactRefs
	return result
}

func projectSessionResultUpdatedPayload(
	status interfaces.RuntimeStatus,
	sessionResult SessionResult,
) SessionResultUpdatedPayload {
	payload := SessionResultUpdatedPayload{
		ResultStatus: eventResultStatusFromSessionStatus(status),
	}
	payload.ResultSummary = work.CloneWorkContentParts(sessionResult.PrimaryResult)
	payload.ArtifactIDs = append([]string(nil), sessionResult.ArtifactIDs...)
	return payload
}

func cloneCheckpointRefs(
	refs []interfaces.FactorySessionJavaScriptCheckpointEventRef,
) []interfaces.FactorySessionJavaScriptCheckpointEventRef {
	cloned := make([]interfaces.FactorySessionJavaScriptCheckpointEventRef, len(refs))
	for index := range refs {
		cloned[index] = refs[index]
		if refs[index].Label != nil {
			value := *refs[index].Label
			cloned[index].Label = &value
		}
		if refs[index].Summary != nil {
			value := *refs[index].Summary
			cloned[index].Summary = &value
		}
		if refs[index].Timestamp != nil {
			value := *refs[index].Timestamp
			cloned[index].Timestamp = &value
		}
		if refs[index].ArtifactRef != nil {
			value := cloneArtifactRef(*refs[index].ArtifactRef)
			cloned[index].ArtifactRef = &value
		}
	}
	return cloned
}

func cloneArtifactRef(ref interfaces.FactoryArtifactRef) interfaces.FactoryArtifactRef {
	cloned := ref
	if ref.ContentHash != nil {
		value := *ref.ContentHash
		cloned.ContentHash = &value
	}
	if ref.SizeBytes != nil {
		value := *ref.SizeBytes
		cloned.SizeBytes = &value
	}
	return cloned
}

func eventResultStatusFromSessionStatus(status interfaces.RuntimeStatus) interfaces.FactorySessionResultStatus {
	if status == interfaces.RuntimeStatusFinished {
		return interfaces.FactorySessionResultStatusFinal
	}
	return interfaces.FactorySessionResultStatusPartial
}

func resultStatusFromSessionStatus(status interfaces.RuntimeStatus) ResultStatus {
	switch status {
	case interfaces.RuntimeStatusFinished:
		return ResultStatusFinal
	case interfaces.RuntimeStatusActive:
		return ResultStatusPartial
	default:
		return ResultStatusNotReady
	}
}

func projectArtifactProjection(input SessionResultInput) ([]string, []interfaces.FactoryArtifactRef) {
	seen := make(map[string]struct{})
	var artifactIDs []string
	var artifactRefs []interfaces.FactoryArtifactRef
	addArtifact := func(ref interfaces.FactoryArtifactRef) {
		id := strings.TrimSpace(ref.ID)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		artifactIDs = append(artifactIDs, id)
		artifactRefs = append(artifactRefs, cloneArtifactRef(ref))
	}
	if input.ResultArtifact != nil {
		addArtifact(*input.ResultArtifact)
	}
	for _, artifact := range input.Artifacts {
		id := strings.TrimSpace(artifact.ID)
		if id == "" {
			continue
		}
		addArtifact(interfaces.FactoryArtifactRef{
			ID:         id,
			Kind:       strings.ToUpper(strings.TrimSpace(artifact.Kind)),
			Visibility: strings.TrimSpace(artifact.Visibility),
		})
	}
	return artifactIDs, artifactRefs
}

func projectDecodedValue(
	sessionID string,
	value any,
	artifacts []interfaces.FactorySessionArtifactState,
	path string,
) ([]work.WorkContentPart, []Issue) {
	switch typed := value.(type) {
	case nil:
		return []work.WorkContentPart{jsonPart(nil, path)}, nil
	case bool, float64, json.Number:
		return []work.WorkContentPart{jsonPart(typed, path)}, nil
	case string:
		if issues := validateEmbeddedArtifactURI(sessionID, typed, path); len(issues) > 0 {
			return nil, issues
		}
		if artifact, ok := artifactForEmbeddedString(sessionID, typed, artifacts); ok {
			return []work.WorkContentPart{artifactBackedPart(sessionID, artifact, typed)}, nil
		}
		if len(typed) > DefaultMaxEmbeddedBytes {
			if artifact := syntheticLargeTextArtifact(typed, path); artifact.ID != "" {
				return []work.WorkContentPart{artifactBackedPart(sessionID, artifact, typed)}, nil
			}
		}
		return []work.WorkContentPart{jsonPart(typed, path)}, nil
	case []any:
		return []work.WorkContentPart{jsonPart(typed, path)}, nil
	case map[string]any:
		if artifactID, ok := typed["artifactId"].(string); ok {
			if artifact, found := findArtifact(artifacts, artifactID); found {
				return []work.WorkContentPart{artifactBackedPart(sessionID, artifact, "")}, nil
			}
		}
		return []work.WorkContentPart{jsonPart(typed, path)}, nil
	default:
		return []work.WorkContentPart{jsonPart(typed, path)}, nil
	}
}

func validateEmbeddedArtifactURI(sessionID, value, path string) []Issue {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, ArtifactURIScheme+"://") {
		return nil
	}
	issues := ValidateArtifactURIForSession(trimmed, sessionID)
	if len(issues) == 0 {
		return nil
	}
	for index := range issues {
		if path != "" && path != "$" {
			issues[index].Path = path
		}
	}
	return issues
}

func jsonPart(value any, path string) work.WorkContentPart {
	raw, err := json.Marshal(value)
	if err != nil {
		raw = []byte("null")
	}
	part := work.WorkContentPart{
		Type: work.WorkContentPartTypeJSON,
		JSON: raw,
	}
	if path != "" && path != "$" {
		part.Label = strings.TrimPrefix(path, "$.")
	}
	return part
}

func artifactBackedPart(sessionID string, artifact interfaces.FactorySessionArtifactState, text string) work.WorkContentPart {
	artifactID := strings.TrimSpace(artifact.ID)
	uri := FormatArtifactURI(sessionID, artifactID)
	kind := strings.ToUpper(strings.TrimSpace(artifact.Kind))
	switch kind {
	case "IMAGE":
		return work.WorkContentPart{
			Type:        work.WorkContentPartTypeImage,
			URL:         uri,
			ArtifactID:  artifactID,
			Label:       artifact.Label,
			ContentType: artifactContentType(artifact),
		}
	case "AUDIO":
		return work.WorkContentPart{
			Type:        work.WorkContentPartTypeAudio,
			URL:         uri,
			ArtifactID:  artifactID,
			Label:       artifact.Label,
			ContentType: artifactContentType(artifact),
		}
	case "BINARY", "PATCH", "DATASET":
		return work.WorkContentPart{
			Type:        work.WorkContentPartTypeBinary,
			URL:         uri,
			ArtifactID:  artifactID,
			Label:       artifact.Label,
			ContentType: artifactContentType(artifact),
		}
	case "LOG", "FINDING", "CHECKPOINT", "CHILD_RESULT", "WORKTREE_SUMMARY":
		if strings.TrimSpace(text) != "" && len(text) <= DefaultMaxEmbeddedBytes {
			return work.WorkContentPart{
				Type:       work.WorkContentPartTypeText,
				Text:       text,
				ArtifactID: artifactID,
				Label:      artifact.Label,
			}
		}
		return work.WorkContentPart{
			Type:       work.WorkContentPartTypeText,
			Text:       uri,
			ArtifactID: artifactID,
			Label:      artifact.Label,
		}
	default:
		if strings.TrimSpace(text) != "" && len(text) <= DefaultMaxEmbeddedBytes {
			return work.WorkContentPart{
				Type:       work.WorkContentPartTypeText,
				Text:       text,
				ArtifactID: artifactID,
				Label:      firstNonEmpty(artifact.Label, "result"),
			}
		}
		return work.WorkContentPart{
			Type:       work.WorkContentPartTypeText,
			Text:       uri,
			ArtifactID: artifactID,
			Label:      firstNonEmpty(artifact.Label, "result"),
		}
	}
}

func artifactForEmbeddedString(sessionID, value string, artifacts []interfaces.FactorySessionArtifactState) (interfaces.FactorySessionArtifactState, bool) {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, ArtifactURIScheme+"://") {
		return interfaces.FactorySessionArtifactState{}, false
	}
	if issues := ValidateArtifactURIForSession(trimmed, sessionID); len(issues) > 0 {
		return interfaces.FactorySessionArtifactState{}, false
	}
	parsed, issues := ParseArtifactURI(trimmed)
	if len(issues) > 0 {
		return interfaces.FactorySessionArtifactState{}, false
	}
	artifact, ok := findArtifact(artifacts, parsed.ArtifactID)
	if !ok {
		return interfaces.FactorySessionArtifactState{ID: parsed.ArtifactID}, true
	}
	return artifact, true
}

func syntheticLargeTextArtifact(text, path string) interfaces.FactorySessionArtifactState {
	if len(text) <= DefaultMaxEmbeddedBytes {
		return interfaces.FactorySessionArtifactState{}
	}
	return interfaces.FactorySessionArtifactState{
		ID:         hashLabel(path, text),
		Kind:       "LOG",
		Visibility: "PUBLIC",
		Label:      strings.TrimPrefix(path, "$."),
		Summary:    "Large workflow text output projected as an artifact ref",
		SizeBytes:  int64(len(text)),
	}
}

func findArtifact(artifacts []interfaces.FactorySessionArtifactState, artifactID string) (interfaces.FactorySessionArtifactState, bool) {
	trimmed := strings.TrimSpace(artifactID)
	for _, artifact := range artifacts {
		if strings.TrimSpace(artifact.ID) == trimmed {
			return artifact, true
		}
	}
	return interfaces.FactorySessionArtifactState{}, false
}

func artifactContentType(artifact interfaces.FactorySessionArtifactState) string {
	if contentType := strings.TrimSpace(artifact.CaptureMetadata["contentType"]); contentType != "" {
		return contentType
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
