package backendconformance

// MinimumPinnedArtifactSizeBytes is the lower bound for a published backend
// archive. Placeholder-sized manifest values must not satisfy conformance.
const MinimumPinnedArtifactSizeBytes int64 = 1 << 20
