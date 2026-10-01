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

//go:embed qwen-windows-cuda-manifest.json
var qwenWindowsCUDAManifestData []byte

// QwenWindowsCUDAManifest records the immutable manual Windows CUDA test
// publication separately so the established CPU and Metal pins stay unchanged.
func QwenWindowsCUDAManifest() (Manifest, error) {
	return Decode(qwenWindowsCUDAManifestData)
}
