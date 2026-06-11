package preview

import "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"

// DefaultResultConstraints returns the shared structured-result contract metadata.
func DefaultResultConstraints() ResultConstraints {
	return ResultConstraints{
		RequiresStructuredCloneableJSON: true,
		ArtifactURIScheme:               result.ArtifactURIScheme,
		MaxEmbeddedBytes:                result.DefaultMaxEmbeddedBytes,
		RejectedValueKinds: []string{
			"function",
			"unresolved-promise",
			"cyclic-value",
			"host-handle",
			"unsupported-binary",
		},
	}
}
