package factorysessions

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	workflowresult "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
	jsstore "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/store"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping"
)

// ProjectCheckpointArtifactRef maps one internal checkpoint record to public
// artifact metadata without raw VM state or host paths.
func ProjectCheckpointArtifactRef(record interfaces.JavaScriptCheckpointRecord) factoryapi.FactoryArtifactRef {
	artifactID := strings.TrimSpace(record.ArtifactID)
	if artifactID == "" {
		artifactID = record.ID
	}
	ref := factoryapi.FactoryArtifactRef{
		Id:         artifactID,
		Kind:       factoryapi.FactoryArtifactKindCHECKPOINT,
		Visibility: factoryapi.FactoryArtifactVisibilityINTERNALCHECKPOINT,
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

// ProjectCheckpointRef maps one internal checkpoint record to the public
// session/event checkpoint ref shape.
func ProjectCheckpointRef(record interfaces.JavaScriptCheckpointRecord) factoryapi.FactorySessionJavaScriptCheckpointRef {
	ref := factoryapi.FactorySessionJavaScriptCheckpointRef{
		Id:          record.ID,
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
func ProjectCheckpointRefs(records []interfaces.JavaScriptCheckpointRecord) []factoryapi.FactorySessionJavaScriptCheckpointRef {
	if len(records) == 0 {
		return nil
	}
	projected := make([]factoryapi.FactorySessionJavaScriptCheckpointRef, 0, len(records))
	for _, record := range records {
		projected = append(projected, ProjectCheckpointRef(record))
	}
	return projected
}

// JavaScriptRuntimeStateFromCheckpoints builds the JavaScript runtime projection
// input from checkpoint store records and optional runtime overrides.
func JavaScriptRuntimeStateFromCheckpoints(
	store *JavaScriptCheckpointStore,
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
			ID:          projected.Id,
			Label:       stringValue(projected.Label),
			Summary:     stringValue(projected.Summary),
			Timestamp:   timeValue(projected.Timestamp),
			ArtifactRef: projectInterfaceArtifactRef(projected.ArtifactRef),
		})
	}
	return state
}

func projectInterfaceArtifactRef(ref *factoryapi.FactoryArtifactRef) *interfaces.JavaScriptCheckpointArtifactRef {
	if ref == nil {
		return nil
	}
	artifact := &interfaces.JavaScriptCheckpointArtifactRef{
		ID:         ref.Id,
		Kind:       string(ref.Kind),
		Visibility: string(ref.Visibility),
	}
	if ref.ContentHash != nil {
		artifact.ContentHash = *ref.ContentHash
	}
	if ref.SizeBytes != nil {
		artifact.SizeBytes = *ref.SizeBytes
	}
	return artifact
}

func artifactRefPointer(ref factoryapi.FactoryArtifactRef) *factoryapi.FactoryArtifactRef {
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
	store *JavaScriptCheckpointStore,
) factoryapi.FactorySessionLiveResult {
	runtime := ProjectRuntime(ctx)
	input := workflowresult.SessionResultInput{
		SessionID: sessionID,
		Status:    runtime.Status,
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
	return apisurface.BuildWorkflowSessionLiveResult(input)
}

func finalResultArtifactRef(artifacts []interfaces.FactorySessionArtifactState) *factoryapi.FactoryArtifactRef {
	for _, artifact := range artifacts {
		if strings.EqualFold(strings.TrimSpace(artifact.Kind), "FINAL_RESULT") {
			ref := factoryapi.FactoryArtifactRef{
				Id:         strings.TrimSpace(artifact.ID),
				Kind:       factoryapi.FactoryArtifactKindFINALRESULT,
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
	}
	return nil
}

func primaryResultJSON(parts []interfaces.WorkContentPart) json.RawMessage {
	if len(parts) == 0 {
		return nil
	}
	for _, part := range parts {
		if part.Type.Normalized() == interfaces.WorkContentPartTypeJSON && len(part.JSON) > 0 {
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
	store *JavaScriptCheckpointStore,
) factoryapi.FactorySessionPartialResult {
	runtime := ProjectRuntime(ctx)
	phase := ""
	if runtime.Javascript != nil && runtime.Javascript.Phase != nil {
		phase = strings.TrimSpace(*runtime.Javascript.Phase)
	}
	result := factoryapi.FactorySessionPartialResult{
		SessionId: sessionID,
		Phase:     phase,
	}
	if checkpointRefs := ProjectCheckpointRefs(store.List()); len(checkpointRefs) > 0 {
		result.CheckpointRefs = &checkpointRefs
		if latest := checkpointRefs[len(checkpointRefs)-1]; latest.ArtifactRef != nil {
			artifactRef := *latest.ArtifactRef
			result.PartialResultArtifactRef = &artifactRef
		}
	}
	return result
}

// JavaScriptCheckpointStore keeps orchestrator-owned checkpoint bundles for one
// live JavaScript workflow session.
//
// Deprecated: use pkg/orchestrators/javascript/store.CheckpointStore directly.
type JavaScriptCheckpointStore = jsstore.CheckpointStore

// NewJavaScriptCheckpointStore allocates an empty checkpoint store.
func NewJavaScriptCheckpointStore() *JavaScriptCheckpointStore {
	return jsstore.NewCheckpointStore()
}
