package artifacts

import backendregistry "github.com/portpowered/infinite-you/pkg/services/models/internal/backendregistry"

// Select returns the one artifact compatible with the exact request.
func (manifest Manifest) Select(request SelectionRequest) (ArtifactDescriptor, error) {
	if err := validateSelectionRequest(request); err != nil {
		return ArtifactDescriptor{}, err
	}

	platformMatches := make([]manifestEntry, 0, len(manifest.entries))
	for _, entry := range manifest.entries {
		if entry.backend.ID == request.Backend &&
			entry.target.OperatingSystem == request.OperatingSystem &&
			entry.target.Architecture == request.Architecture {
			platformMatches = append(platformMatches, entry)
		}
	}
	if len(platformMatches) == 0 {
		return ArtifactDescriptor{}, failure(FailureMissingMatch, "selection", request.Backend, "no artifact matches the requested backend and platform")
	}

	protocolMatches := filterProtocol(platformMatches, request.ProtocolRevision)
	if len(protocolMatches) == 0 {
		return ArtifactDescriptor{}, failure(FailureIncompatibleProtocol, "protocolRevision", request.ProtocolRevision, "the matching backend and platform use another revision")
	}
	compatible := filterAccelerator(protocolMatches, request.Accelerator)
	if len(compatible) == 0 {
		return ArtifactDescriptor{}, failure(FailureIncompatibleAccelerator, "accelerator", request.Accelerator, "the matching artifact does not declare this accelerator")
	}
	if len(compatible) > 1 {
		return ArtifactDescriptor{}, failure(FailureDuplicateMatch, "selection", request.Backend, "the manifest contains multiple compatible entries")
	}
	return descriptorFromEntry(compatible[0], manifest.publication), nil
}

func validateSelectionRequest(request SelectionRequest) error {
	if !validToken(request.Backend) {
		if _, known := backendregistry.LookupArtifact(request.Backend); !known {
			return failure(FailureUnknownBackend, "backend", request.Backend, "backend is outside the supported LocalAI set")
		}
		return failure(FailureInvalidSelection, "backend", request.Backend, "must be a safe backend identifier")
	}
	if _, known := backendregistry.LookupArtifact(request.Backend); !known {
		return failure(FailureUnknownBackend, "backend", request.Backend, "backend is outside the supported LocalAI set")
	}
	targetID := request.OperatingSystem + "-" + request.Architecture
	target, knownTarget := supportedTargets[targetID]
	if !knownTarget {
		return failure(FailureUnsupportedPlatform, "platform", targetID, "target is outside the supported closed set")
	}
	if !validToken(request.ProtocolRevision) {
		return failure(FailureInvalidSelection, "protocolRevision", request.ProtocolRevision, "must be a non-empty safe revision identifier")
	}
	if !contains(target.accelerators, request.Accelerator) {
		return failure(FailureIncompatibleAccelerator, "accelerator", request.Accelerator, "the requested accelerator is not compatible with "+targetID)
	}
	return nil
}

func filterProtocol(entries []manifestEntry, revision string) []manifestEntry {
	filtered := make([]manifestEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.protocol.Revision == revision {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func filterAccelerator(entries []manifestEntry, accelerator string) []manifestEntry {
	filtered := make([]manifestEntry, 0, len(entries))
	for _, entry := range entries {
		if contains(entry.target.Accelerators, accelerator) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func descriptorFromEntry(entry manifestEntry, publication publication) ArtifactDescriptor {
	return ArtifactDescriptor{
		ID: entry.id, Publication: publication.identity, Backend: entry.backend,
		Source: entry.source, Protocol: entry.protocol,
		Target: TargetCompatibility{
			ID: entry.target.ID, OperatingSystem: entry.target.OperatingSystem,
			Architecture: entry.target.Architecture,
			Accelerators: append([]string(nil), entry.target.Accelerators...),
		},
		Artifact: entry.artifact,
	}
}

// ArtifactCount reports the number of validated entries in the snapshot.
func (manifest Manifest) ArtifactCount() int {
	return len(manifest.entries)
}

// ProtocolRevision returns the immutable protocol revision shared by every
// artifact in the validated publication.
func (manifest Manifest) ProtocolRevision() string {
	return manifest.publication.protocol.Revision
}
