package projections

import (
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

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
	if artifact.RedactionCounts != nil {
		counts := make(map[string]int)
		if artifact.RedactionCounts.Secrets != nil {
			counts["secrets"] = int(*artifact.RedactionCounts.Secrets)
		}
		if artifact.RedactionCounts.Paths != nil {
			counts["paths"] = int(*artifact.RedactionCounts.Paths)
		}
		if artifact.RedactionCounts.Tokens != nil {
			counts["tokens"] = int(*artifact.RedactionCounts.Tokens)
		}
		if len(counts) > 0 {
			state.RedactionCounts = counts
		}
	}
	if artifact.CaptureMetadata != nil {
		metadata := make(map[string]string)
		if artifact.CaptureMetadata.CapturedAt != nil {
			metadata["capturedAt"] = artifact.CaptureMetadata.CapturedAt.UTC().Format(time.RFC3339)
		}
		if artifact.CaptureMetadata.SourceDispatchId != nil {
			metadata["sourceDispatchId"] = *artifact.CaptureMetadata.SourceDispatchId
		}
		if artifact.CaptureMetadata.MimeType != nil {
			metadata["mimeType"] = *artifact.CaptureMetadata.MimeType
		}
		if len(metadata) > 0 {
			state.CaptureMetadata = metadata
		}
	}
	if payload.CapturedAt != nil {
		state.CapturedAt = payload.CapturedAt.UTC()
	}
	return state
}
