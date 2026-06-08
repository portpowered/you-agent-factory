package projections

import (
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func (r *factoryWorldReducer) applyOrchestratorLifecycleEvent(event factoryapi.FactoryEvent) (bool, error) {
	switch event.Type {
	case factoryapi.FactoryEventTypeJavaScriptCheckpointRef:
		return true, r.applyJavaScriptCheckpointRefEvent(event)
	case factoryapi.FactoryEventTypeJavaScriptPhaseChange:
		return true, r.applyJavaScriptPhaseChangeEvent(event)
	case factoryapi.FactoryEventTypeArtifactCreated:
		return true, r.applyArtifactCreatedEvent(event)
	default:
		return false, nil
	}
}

func (r *factoryWorldReducer) applyJavaScriptCheckpointRefEvent(event factoryapi.FactoryEvent) error {
	payload, err := event.Payload.AsJavaScriptCheckpointRefEventPayload()
	if err != nil {
		return err
	}
	checkpoint := interfaces.FactorySessionJavaScriptCheckpointRef{
		ID: payload.CheckpointId,
		ArtifactRef: &interfaces.JavaScriptCheckpointArtifactRef{
			ID:         payload.ArtifactRef.Id,
			Kind:       string(payload.ArtifactRef.Kind),
			Visibility: string(payload.ArtifactRef.Visibility),
		},
	}
	if payload.ArtifactRef.ContentHash != nil {
		checkpoint.ArtifactRef.ContentHash = *payload.ArtifactRef.ContentHash
	}
	if payload.ArtifactRef.SizeBytes != nil {
		checkpoint.ArtifactRef.SizeBytes = *payload.ArtifactRef.SizeBytes
	}
	if payload.Label != nil {
		checkpoint.Label = *payload.Label
	}
	if payload.Summary != nil {
		checkpoint.Summary = *payload.Summary
	}
	if payload.Timestamp != nil {
		checkpoint.Timestamp = payload.Timestamp.UTC()
	}
	r.stateValue.JavaScriptCheckpoints = append(r.stateValue.JavaScriptCheckpoints, checkpoint)
	if runtime := r.ensureJavaScriptRuntime(); runtime != nil {
		runtime.Checkpoints = append(runtime.Checkpoints, checkpoint)
	}
	return nil
}

func (r *factoryWorldReducer) applyJavaScriptPhaseChangeEvent(event factoryapi.FactoryEvent) error {
	payload, err := event.Payload.AsJavaScriptPhaseChangeEventPayload()
	if err != nil {
		return err
	}
	runtime := r.ensureJavaScriptRuntime()
	runtime.Phase = payload.Phase
	runtime.Phases = append([]string(nil), payload.Phases...)
	if payload.ArgsDigest != nil {
		runtime.ArgsDigest = *payload.ArgsDigest
	}
	runtime.ScriptStatus = string(payload.ScriptStatus)
	runtime.QueuedDispatches = payload.ChildDispatchCounts.Queued
	runtime.RunningDispatches = payload.ChildDispatchCounts.Running
	runtime.CompletedDispatches = payload.ChildDispatchCounts.Completed
	return nil
}

func (r *factoryWorldReducer) applyArtifactCreatedEvent(event factoryapi.FactoryEvent) error {
	payload, err := event.Payload.AsArtifactCreatedEventPayload()
	if err != nil {
		return err
	}
	artifact := projectArtifactCreatedPayload(payload)
	r.stateValue.Artifacts = append(r.stateValue.Artifacts, artifact)
	if runtime := r.ensureJavaScriptRuntime(); runtime != nil {
		runtime.Artifacts = append(runtime.Artifacts, artifact)
	}
	return nil
}

func (r *factoryWorldReducer) ensureJavaScriptRuntime() *interfaces.FactorySessionJavaScriptRuntimeState {
	if r.stateValue.JavaScriptRuntime == nil {
		r.stateValue.JavaScriptRuntime = &interfaces.FactorySessionJavaScriptRuntimeState{}
	}
	return r.stateValue.JavaScriptRuntime
}

func projectArtifactCreatedPayload(payload factoryapi.ArtifactCreatedEventPayload) interfaces.FactorySessionArtifactState {
	artifact := payload.Artifact
	state := interfaces.FactorySessionArtifactState{
		ID:         artifact.Id,
		Kind:       string(artifact.Kind),
		Visibility: string(artifact.Visibility),
	}
	if artifact.Label != nil {
		state.Label = *artifact.Label
	}
	if artifact.Summary != nil {
		state.Summary = *artifact.Summary
	}
	if artifact.AuditMode != nil {
		state.AuditMode = string(*artifact.AuditMode)
	}
	if artifact.ContentHash != nil {
		state.ContentHash = *artifact.ContentHash
	}
	if artifact.SizeBytes != nil {
		state.SizeBytes = *artifact.SizeBytes
	}
	if counts := artifactRedactionCountsFromAPI(artifact.RedactionCounts); len(counts) > 0 {
		state.RedactionCounts = counts
	}
	if metadata := artifactCaptureMetadataFromAPI(artifact.CaptureMetadata); len(metadata) > 0 {
		state.CaptureMetadata = metadata
	}
	if payload.CapturedAt != nil {
		state.CapturedAt = payload.CapturedAt.UTC()
	}
	return state
}

func artifactRedactionCountsFromAPI(counts *factoryapi.FactoryArtifactRedactionCounts) map[string]int {
	if counts == nil {
		return nil
	}
	redactions := make(map[string]int)
	if counts.Secrets != nil {
		redactions["secrets"] = int(*counts.Secrets)
	}
	if counts.Paths != nil {
		redactions["paths"] = int(*counts.Paths)
	}
	if counts.Tokens != nil {
		redactions["tokens"] = int(*counts.Tokens)
	}
	return redactions
}

func artifactCaptureMetadataFromAPI(metadata *factoryapi.FactoryArtifactCaptureMetadata) map[string]string {
	if metadata == nil {
		return nil
	}
	capture := make(map[string]string)
	if metadata.CapturedAt != nil {
		capture["capturedAt"] = metadata.CapturedAt.UTC().Format(time.RFC3339)
	}
	if metadata.SourceDispatchId != nil {
		capture["sourceDispatchId"] = *metadata.SourceDispatchId
	}
	if metadata.MimeType != nil {
		capture["mimeType"] = *metadata.MimeType
	}
	return capture
}
