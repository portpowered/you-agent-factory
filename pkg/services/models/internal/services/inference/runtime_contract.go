package inference

import (
	models "github.com/portpowered/infinite-you/pkg/services/models"
)

// HostHandleSlot carries parent-private host-slot facts for one scoped invoke.
// It does not expose supervisor handles, endpoints, processes, or filesystem
// paths to peers.
type HostHandleSlot struct {
	Reused bool
}

// InvocationArtifactSource is runtime-owned artifact materialization input kept
// inside Inference. SourcePath is for internal export only and never crosses
// the peer boundary.
type InvocationArtifactSource struct {
	RefValue   string
	SourcePath string
	Name       string
	MediaType  string
	SizeBytes  int64
	Properties map[string]string
}

// InvocationRuntimeRequest is the parent-private invoke input for one accepted
// lease-backed operation.
type InvocationRuntimeRequest struct {
	Request  models.InvokeModelRequest
	HostSlot HostHandleSlot
}

// InvocationRuntimeResult contains detached runtime output facts without
// exposing private handles, endpoints, processes, or filesystem paths.
type InvocationRuntimeResult struct {
	Content   []models.InferenceContent
	Artifacts []InvocationArtifactSource
}
