package modelmcp

import (
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

// AcquireLeaseResult is the MCP JSON projection for you.model.acquire_lease.
type AcquireLeaseResult struct {
	Lease ModelLeaseResult `json:"Lease"`
}

// ModelLeaseResult carries detached lease capability facts with serialized
// opaque references for MCP hosts.
type ModelLeaseResult struct {
	Lease         string                  `json:"Lease"`
	Scope         string                  `json:"Scope"`
	ModelName     string                  `json:"ModelName"`
	Holder        string                  `json:"Holder"`
	ExpiresAt     time.Time               `json:"ExpiresAt"`
	Status        models.ModelLeaseStatus `json:"Status"`
	HostReadiness models.ReadinessState   `json:"HostReadiness"`
}

// InvokeWithLeaseResult is the MCP JSON projection for you.model.invoke_with_lease.
type InvokeWithLeaseResult struct {
	Invocation       string                              `json:"Invocation"`
	Scope            string                              `json:"Scope"`
	Lease            string                              `json:"Lease"`
	ModelName        string                              `json:"ModelName"`
	Operation        string                              `json:"Operation"`
	Status           models.ModelInvocationStatus        `json:"Status"`
	Content          []models.InferenceContent           `json:"Content"`
	Artifacts        []InferenceArtifactResult           `json:"Artifacts"`
	LeaseDisposition models.InvocationLeaseDisposition `json:"LeaseDisposition"`
}

// InferenceArtifactResult carries detached artifact metadata with a serialized
// opaque artifact reference for MCP hosts.
type InferenceArtifactResult struct {
	Artifact  string `json:"Artifact"`
	Name      string `json:"Name"`
	MediaType string `json:"MediaType"`
	SizeBytes int64  `json:"SizeBytes"`
}

func projectAcquireLeaseResult(result models.AcquireModelLeaseResult) AcquireLeaseResult {
	return AcquireLeaseResult{Lease: projectModelLease(result.Lease)}
}

func projectModelLease(lease models.ModelLease) ModelLeaseResult {
	return ModelLeaseResult{
		Lease:         lease.Lease.String(),
		Scope:         lease.Scope.String(),
		ModelName:     lease.ModelName,
		Holder:        lease.Holder,
		ExpiresAt:     lease.ExpiresAt,
		Status:        lease.Status,
		HostReadiness: lease.HostReadiness,
	}
}

func projectInvokeWithLeaseResult(result models.InvokeModelResult) InvokeWithLeaseResult {
	artifacts := make([]InferenceArtifactResult, len(result.Artifacts))
	for i, artifact := range result.Artifacts {
		artifacts[i] = InferenceArtifactResult{
			Artifact:  artifact.Artifact.String(),
			Name:      artifact.Name,
			MediaType: artifact.MediaType,
			SizeBytes: artifact.SizeBytes,
		}
	}
	return InvokeWithLeaseResult{
		Invocation:       result.Invocation.String(),
		Scope:            result.Scope.String(),
		Lease:            result.Lease.String(),
		ModelName:        result.ModelName,
		Operation:        result.Operation,
		Status:           result.Status,
		Content:          append([]models.InferenceContent(nil), result.Content...),
		Artifacts:        artifacts,
		LeaseDisposition: result.LeaseDisposition,
	}
}
