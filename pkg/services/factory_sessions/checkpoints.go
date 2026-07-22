package factorysessions

import (
	"encoding/json"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workflowresult "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"strings"
	"time"
)

// ProjectCheckpointArtifactRef maps one internal checkpoint record to detached
// domain artifact metadata without raw VM state or host paths.
func ProjectCheckpointArtifactRef(record interfaces.JavaScriptCheckpointRecord) interfaces.FactoryArtifactRef {
	artifactID := strings.TrimSpace(record.ArtifactID)
	if artifactID == "" {
		artifactID = record.ID
	}
	ref := interfaces.FactoryArtifactRef{
		ID:         artifactID,
		Kind:       "CHECKPOINT",
		Visibility: "INTERNAL_CHECKPOINT",
	}
	if hash := strings.TrimSpace(record.ContentHash); hash != "" {
		ref.ContentHash = &hash
	}
	if record.SizeBytes > 0 {
		size := record.SizeBytes
		ref.SizeBytes = &size
	}
	return ref
}

// ProjectCheckpointRef maps one internal checkpoint record to the Factory-owned
// session/event checkpoint ref shape.
func ProjectCheckpointRef(record interfaces.JavaScriptCheckpointRecord) interfaces.FactorySessionJavaScriptCheckpointEventRef {
	ref := interfaces.FactorySessionJavaScriptCheckpointEventRef{
		ID:          record.ID,
		ArtifactRef: artifactRefPointer(ProjectCheckpointArtifactRef(record)),
	}
	if label := strings.TrimSpace(record.Label); label != "" {
		ref.Label = &label
	}
	if summary := strings.TrimSpace(record.Summary); summary != "" {
		ref.Summary = &summary
	}
	if !record.Timestamp.IsZero() {
		timestamp := record.Timestamp.UTC()
		ref.Timestamp = &timestamp
	}
	return ref
}

// ProjectCheckpointRefs maps internal checkpoint records to public refs.
func ProjectCheckpointRefs(records []interfaces.JavaScriptCheckpointRecord) []interfaces.FactorySessionJavaScriptCheckpointEventRef {
	if len(records) == 0 {
		return nil
	}
	projected := make([]interfaces.FactorySessionJavaScriptCheckpointEventRef, 0, len(records))
	for _, record := range records {
		projected = append(projected, ProjectCheckpointRef(record))
	}
	return projected
}

// JavaScriptRuntimeStateFromCheckpoints builds the JavaScript runtime projection
// input from checkpoint store records and optional runtime overrides.
func JavaScriptRuntimeStateFromCheckpoints(
	store workflowresult.JavaScriptCheckpointStore,
	override *interfaces.FactorySessionJavaScriptRuntimeState,
) *interfaces.FactorySessionJavaScriptRuntimeState {
	state := override
	if state == nil {
		state = &interfaces.FactorySessionJavaScriptRuntimeState{}
	}
	if store == nil {
		return state
	}
	records := store.List()
	if len(records) == 0 {
		return state
	}
	state.Checkpoints = make([]interfaces.FactorySessionJavaScriptCheckpointRef, 0, len(records))
	for _, record := range records {
		projected := ProjectCheckpointRef(record)
		state.Checkpoints = append(state.Checkpoints, interfaces.FactorySessionJavaScriptCheckpointRef{
			ID:          projected.ID,
			Label:       stringValue(projected.Label),
			Summary:     stringValue(projected.Summary),
			Timestamp:   timeValue(projected.Timestamp),
			ArtifactRef: projectRuntimeArtifactRef(projected.ArtifactRef),
		})
	}
	return state
}

func projectRuntimeArtifactRef(ref *interfaces.FactoryArtifactRef) *interfaces.JavaScriptCheckpointArtifactRef {
	if ref == nil {
		return nil
	}
	artifact := &interfaces.JavaScriptCheckpointArtifactRef{
		ID:         ref.ID,
		Kind:       ref.Kind,
		Visibility: ref.Visibility,
	}
	if ref.ContentHash != nil {
		artifact.ContentHash = *ref.ContentHash
	}
	if ref.SizeBytes != nil {
		artifact.SizeBytes = *ref.SizeBytes
	}
	return artifact
}

func artifactRefPointer(ref interfaces.FactoryArtifactRef) *interfaces.FactoryArtifactRef {
	copied := ref
	return &copied
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func timeValue(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return value.UTC()
}

// ProjectSessionResult maps one JavaScript session runtime to the terminal result
// read shape without raw checkpoint bodies or host paths.
func ProjectSessionResult(
	sessionID string,
	ctx ProjectionContext,
	store workflowresult.JavaScriptCheckpointStore,
	projection workflowresult.SessionResultProjectionOperation,
) workflowresult.LiveSessionResult {
	if projection == nil {
		return workflowresult.LiveSessionResult{}
	}
	runtime := ProjectRuntimeContract(ctx)
	input := workflowresult.SessionResultInput{
		SessionID: sessionID,
		Status:    interfaces.RuntimeStatus(runtime.Status),
	}
	if ctx.JavaScript != nil {
		input.Artifacts = append(input.Artifacts, ctx.JavaScript.Artifacts...)
		if primaryJSON := primaryResultJSON(ctx.JavaScript.PrimaryResult); len(primaryJSON) > 0 {
			input.PrimaryValue = workflowresult.TypedValue{JSON: primaryJSON}
		}
	}
	if checkpointRefs := ProjectCheckpointRefs(store.List()); len(checkpointRefs) > 0 {
		input.CheckpointRefs = checkpointRefs
		if latest := checkpointRefs[len(checkpointRefs)-1]; latest.ArtifactRef != nil {
			copied := *latest.ArtifactRef
			input.ResultArtifact = &copied
		}
	}
	if input.ResultArtifact == nil {
		input.ResultArtifact = finalResultArtifactRef(input.Artifacts)
	}
	return projection.ProjectSessionResults(input).Live
}

func finalResultArtifactRef(artifacts []interfaces.FactorySessionArtifactState) *interfaces.FactoryArtifactRef {
	for _, artifact := range artifacts {
		if strings.EqualFold(strings.TrimSpace(artifact.Kind), "FINAL_RESULT") {
			ref := interfaces.FactoryArtifactRef{
				ID:         strings.TrimSpace(artifact.ID),
				Kind:       "FINAL_RESULT",
				Visibility: strings.TrimSpace(artifact.Visibility),
			}
			if ref.Visibility == "" {
				ref.Visibility = "PUBLIC"
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
	}
	return nil
}

func primaryResultJSON(parts []work.WorkContentPart) json.RawMessage {
	if len(parts) == 0 {
		return nil
	}
	for _, part := range parts {
		if part.Type.Normalized() == work.WorkContentPartTypeJSON && len(part.JSON) > 0 {
			return part.JSON
		}
	}
	payload := make([]map[string]any, 0, len(parts))
	for _, part := range parts {
		entry := map[string]any{"type": string(part.Type.Normalized())}
		if part.Text != "" {
			entry["text"] = part.Text
		}
		if part.ArtifactID != "" {
			entry["artifactId"] = part.ArtifactID
		}
		if part.URL != "" {
			entry["url"] = part.URL
		}
		payload = append(payload, entry)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	return raw
}

// ProjectSessionPartialResult maps one JavaScript session runtime to the partial
// result read shape without raw checkpoint bodies or host paths.
func ProjectSessionPartialResult(
	sessionID string,
	ctx ProjectionContext,
	store workflowresult.JavaScriptCheckpointStore,
) workflowresult.PartialSessionResult {
	runtime := ProjectRuntimeContract(ctx)
	phase := ""
	if runtime.JavaScript != nil && runtime.JavaScript.Phase != nil {
		phase = strings.TrimSpace(*runtime.JavaScript.Phase)
	}
	result := workflowresult.PartialSessionResult{
		SessionID: sessionID,
		Phase:     phase,
	}
	if checkpointRefs := ProjectCheckpointRefs(store.List()); len(checkpointRefs) > 0 {
		result.CheckpointRefs = checkpointRefs
		if latest := checkpointRefs[len(checkpointRefs)-1]; latest.ArtifactRef != nil {
			copied := *latest.ArtifactRef
			result.PartialResultArtifactRef = &copied
		}
	}
	return result
}
