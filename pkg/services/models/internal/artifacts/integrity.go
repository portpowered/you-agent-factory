package artifacts

import (
	"crypto/sha256"
	"encoding/hex"
)

// VerifyBytes proves that supplied archive bytes match the detached size and
// checksum facts. It does not read files, download data, or mutate state.
func (descriptor ArtifactDescriptor) VerifyBytes(data []byte) error {
	if descriptor.Artifact.SizeBytes <= 0 {
		return failure(FailureInvalidSize, "artifact.sizeBytes", "", "must be a positive integer")
	}
	if !digestPattern.MatchString(descriptor.Artifact.SHA256) {
		return failure(FailureInvalidDigest, "artifact.sha256", descriptor.Artifact.SHA256, "must be a lowercase 64-character SHA-256")
	}
	if int64(len(data)) != descriptor.Artifact.SizeBytes {
		return failuref(FailureIntegrityMismatch, "artifact.sizeBytes", descriptor.Artifact.Name, "published size %d does not match supplied bytes %d", descriptor.Artifact.SizeBytes, len(data))
	}
	sum := sha256.Sum256(data)
	actual := hex.EncodeToString(sum[:])
	if actual != descriptor.Artifact.SHA256 {
		return failuref(FailureIntegrityMismatch, "artifact.sha256", descriptor.Artifact.Name, "published digest %s does not match supplied bytes %s", descriptor.Artifact.SHA256, actual)
	}
	return nil
}

// VerifyBytes is the package-level form used by pure Models tests.
func VerifyBytes(descriptor ArtifactDescriptor, data []byte) error {
	return descriptor.VerifyBytes(data)
}
