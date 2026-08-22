package artifacts

import _ "embed"

// defaultManifestData is the checked-in publication snapshot used by the
// composed Models service until a newer generated publication is supplied by
// the artifact release workflow. Keeping the decoder on this path means the
// production selector and deterministic fixtures use the same validation.
//
//go:embed default-manifest.json
var defaultManifestData []byte

// DefaultManifest decodes a detached copy of the checked-in publication
// snapshot. The returned manifest contains no shared mutable slices.
func DefaultManifest() (Manifest, error) {
	return Decode(defaultManifestData)
}
