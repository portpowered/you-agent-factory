package workflowpreview

import "github.com/portpowered/infinite-you/pkg/workflowresult"

// DefaultResultConstraints returns the shared structured-result contract metadata.
func DefaultResultConstraints() ResultConstraints {
	return ResultConstraints{
		RequiresStructuredCloneableJSON: true,
		ArtifactURIScheme:               workflowresult.ArtifactURIScheme,
		MaxEmbeddedBytes:                workflowresult.DefaultMaxEmbeddedBytes,
		RejectedValueKinds: []string{
			"function",
			"unresolved-promise",
			"cyclic-value",
			"host-handle",
			"unsupported-binary",
		},
	}
}
