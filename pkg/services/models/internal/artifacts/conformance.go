//go:build backendconformance || functionallong

package artifacts

// Artifacts returns detached descriptors for every validated publication
// entry in stable manifest order. Callers receive the same validated facts
// used by Select without access to the manifest's private wire representation.
func (manifest Manifest) Artifacts() []ArtifactDescriptor {
	artifacts := make([]ArtifactDescriptor, len(manifest.entries))
	for index, entry := range manifest.entries {
		artifacts[index] = descriptorFromEntry(entry, manifest.publication)
	}
	return artifacts
}
