package workflowresult

import (
	"encoding/json"
	"strings"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workcontent"
)

// ProjectPrimaryResult maps one validated workflow result to WorkContent parts.
func ProjectPrimaryResult(sessionID string, value TypedValue, artifacts []interfaces.FactorySessionArtifactState) ([]interfaces.WorkContentPart, Result) {
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

// BuildLiveSessionResult projects the live terminal session result read shape.
func BuildLiveSessionResult(input SessionResultInput) factoryapi.FactorySessionLiveResult {
	result := factoryapi.FactorySessionLiveResult{
		SessionId: strings.TrimSpace(input.SessionID),
		Status:    input.Status,
	}
	if len(input.CheckpointRefs) > 0 {
		checkpoints := append([]factoryapi.FactorySessionJavaScriptCheckpointRef(nil), input.CheckpointRefs...)
		result.CheckpointRefs = &checkpoints
	}
	if input.ResultArtifact != nil {
		copied := *input.ResultArtifact
		result.ResultArtifactRef = &copied
	}
	return result
}

// BuildSessionResult projects the durable terminal session result read shape.
func BuildSessionResult(input SessionResultInput) factoryapi.FactorySessionResult {
	result := factoryapi.FactorySessionResult{
		SessionId:    strings.TrimSpace(input.SessionID),
		ResultStatus: resultStatusFromSessionStatus(input.Status),
	}
	if parts, validation := ProjectPrimaryResult(input.SessionID, input.PrimaryValue, input.Artifacts); !validation.HasIssues() && len(parts) > 0 {
		if generated := workcontent.GeneratedPtrFromParts(parts); generated != nil {
			result.PrimaryResult = generated
		}
	}
	artifactIDs, artifactRefs := projectArtifactProjection(input)
	if len(artifactIDs) > 0 {
		result.ArtifactIds = &artifactIDs
	}
	if len(artifactRefs) > 0 {
		result.ArtifactRefs = &artifactRefs
	}
	return result
}

// BuildSessionResultUpdatedPayload projects the SESSION_RESULT_UPDATED event
// payload using the same result and artifact ids as BuildSessionResult.
func BuildSessionResultUpdatedPayload(input SessionResultInput) factoryapi.SessionResultUpdatedEventPayload {
	sessionResult := BuildSessionResult(input)
	payload := factoryapi.SessionResultUpdatedEventPayload{
		SessionId: sessionResult.SessionId,
		Status:    input.Status,
	}
	if sessionResult.PrimaryResult != nil {
		payload.PrimaryResult = sessionResult.PrimaryResult
	}
	if sessionResult.ArtifactIds != nil && len(*sessionResult.ArtifactIds) > 0 {
		artifactID := (*sessionResult.ArtifactIds)[0]
		if ref := artifactRefForID(artifactID, input); ref != nil {
			payload.ResultArtifactRef = ref
		}
	}
	if len(input.CheckpointRefs) > 0 {
		checkpoints := append([]factoryapi.FactorySessionJavaScriptCheckpointRef(nil), input.CheckpointRefs...)
		payload.CheckpointRefs = &checkpoints
	}
	return payload
}

func resultStatusFromSessionStatus(status factoryapi.FactorySessionStatus) factoryapi.FactorySessionResultStatus {
	switch status {
	case factoryapi.FactorySessionStatusFINISHED:
		return factoryapi.FactorySessionResultStatusFinal
	case factoryapi.FactorySessionStatusACTIVE:
		return factoryapi.FactorySessionResultStatusPartial
	default:
		return factoryapi.FactorySessionResultStatusNotReady
	}
}

func projectArtifactProjection(input SessionResultInput) ([]string, []factoryapi.FactoryArtifactRef) {
	seen := make(map[string]struct{})
	var artifactIDs []string
	var artifactRefs []factoryapi.FactoryArtifactRef
	addArtifact := func(ref factoryapi.FactoryArtifactRef) {
		id := strings.TrimSpace(ref.Id)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		artifactIDs = append(artifactIDs, id)
		artifactRefs = append(artifactRefs, ref)
	}
	if input.ResultArtifact != nil {
		addArtifact(*input.ResultArtifact)
	}
	for _, artifact := range input.Artifacts {
		id := strings.TrimSpace(artifact.ID)
		if id == "" {
			continue
		}
		addArtifact(factoryapi.FactoryArtifactRef{
			Id:         id,
			Kind:       factoryapi.FactoryArtifactKind(strings.ToUpper(strings.TrimSpace(artifact.Kind))),
			Visibility: factoryapi.FactoryArtifactVisibility(strings.TrimSpace(artifact.Visibility)),
		})
	}
	return artifactIDs, artifactRefs
}

func artifactRefForID(artifactID string, input SessionResultInput) *factoryapi.FactoryArtifactRef {
	if input.ResultArtifact != nil && strings.TrimSpace(input.ResultArtifact.Id) == strings.TrimSpace(artifactID) {
		copied := *input.ResultArtifact
		return &copied
	}
	for _, artifact := range input.Artifacts {
		if strings.TrimSpace(artifact.ID) != strings.TrimSpace(artifactID) {
			continue
		}
		ref := factoryapi.FactoryArtifactRef{
			Id:         strings.TrimSpace(artifact.ID),
			Kind:       factoryapi.FactoryArtifactKind(strings.ToUpper(strings.TrimSpace(artifact.Kind))),
			Visibility: factoryapi.FactoryArtifactVisibility(strings.TrimSpace(artifact.Visibility)),
		}
		if ref.Visibility == "" {
			ref.Visibility = factoryapi.FactoryArtifactVisibilityPUBLIC
		}
		if hash := strings.TrimSpace(artifact.ContentHash); hash != "" {
			ref.ContentHash = &hash
		}
		if artifact.SizeBytes > 0 {
			size := artifact.SizeBytes
			ref.SizeBytes = &size
		}
		return &ref
	}
	return nil
}

func projectDecodedValue(
	sessionID string,
	value any,
	artifacts []interfaces.FactorySessionArtifactState,
	path string,
) ([]interfaces.WorkContentPart, []Issue) {
	switch typed := value.(type) {
	case nil:
		return []interfaces.WorkContentPart{jsonPart(nil, path)}, nil
	case bool, float64, json.Number:
		return []interfaces.WorkContentPart{jsonPart(typed, path)}, nil
	case string:
		if issues := validateEmbeddedArtifactURI(sessionID, typed, path); len(issues) > 0 {
			return nil, issues
		}
		if artifact, ok := artifactForEmbeddedString(sessionID, typed, artifacts); ok {
			return []interfaces.WorkContentPart{artifactBackedPart(sessionID, artifact, typed)}, nil
		}
		if len(typed) > DefaultMaxEmbeddedBytes {
			if artifact := syntheticLargeTextArtifact(typed, path); artifact.ID != "" {
				return []interfaces.WorkContentPart{artifactBackedPart(sessionID, artifact, typed)}, nil
			}
		}
		return []interfaces.WorkContentPart{jsonPart(typed, path)}, nil
	case []any:
		return []interfaces.WorkContentPart{jsonPart(typed, path)}, nil
	case map[string]any:
		if artifactID, ok := typed["artifactId"].(string); ok {
			if artifact, found := findArtifact(artifacts, artifactID); found {
				return []interfaces.WorkContentPart{artifactBackedPart(sessionID, artifact, "")}, nil
			}
		}
		return []interfaces.WorkContentPart{jsonPart(typed, path)}, nil
	default:
		return []interfaces.WorkContentPart{jsonPart(typed, path)}, nil
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

func jsonPart(value any, path string) interfaces.WorkContentPart {
	raw, err := json.Marshal(value)
	if err != nil {
		raw = []byte("null")
	}
	part := interfaces.WorkContentPart{
		Type: interfaces.WorkContentPartTypeJSON,
		JSON: raw,
	}
	if path != "" && path != "$" {
		part.Label = strings.TrimPrefix(path, "$.")
	}
	return part
}

func artifactBackedPart(sessionID string, artifact interfaces.FactorySessionArtifactState, text string) interfaces.WorkContentPart {
	artifactID := strings.TrimSpace(artifact.ID)
	uri := FormatArtifactURI(sessionID, artifactID)
	kind := strings.ToUpper(strings.TrimSpace(artifact.Kind))
	switch kind {
	case "IMAGE":
		return interfaces.WorkContentPart{
			Type:        interfaces.WorkContentPartTypeImage,
			URL:         uri,
			ArtifactID:  artifactID,
			Label:       artifact.Label,
			ContentType: artifactContentType(artifact),
		}
	case "AUDIO":
		return interfaces.WorkContentPart{
			Type:        interfaces.WorkContentPartTypeAudio,
			URL:         uri,
			ArtifactID:  artifactID,
			Label:       artifact.Label,
			ContentType: artifactContentType(artifact),
		}
	case "BINARY", "PATCH", "DATASET":
		return interfaces.WorkContentPart{
			Type:        interfaces.WorkContentPartTypeBinary,
			URL:         uri,
			ArtifactID:  artifactID,
			Label:       artifact.Label,
			ContentType: artifactContentType(artifact),
		}
	case "LOG", "FINDING", "CHECKPOINT", "CHILD_RESULT", "WORKTREE_SUMMARY":
		if strings.TrimSpace(text) != "" && len(text) <= DefaultMaxEmbeddedBytes {
			return interfaces.WorkContentPart{
				Type:       interfaces.WorkContentPartTypeText,
				Text:       text,
				ArtifactID: artifactID,
				Label:      artifact.Label,
			}
		}
		return interfaces.WorkContentPart{
			Type:       interfaces.WorkContentPartTypeText,
			Text:       uri,
			ArtifactID: artifactID,
			Label:      artifact.Label,
		}
	default:
		if strings.TrimSpace(text) != "" && len(text) <= DefaultMaxEmbeddedBytes {
			return interfaces.WorkContentPart{
				Type:       interfaces.WorkContentPartTypeText,
				Text:       text,
				ArtifactID: artifactID,
				Label:      firstNonEmpty(artifact.Label, "result"),
			}
		}
		return interfaces.WorkContentPart{
			Type:       interfaces.WorkContentPartTypeText,
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
