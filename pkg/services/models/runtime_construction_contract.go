package models

type RuntimeAssetEndpoints struct{ BaseURL, APIBaseURL string }

// AssetHostPlatform carries the host's CUDA capability separately from the
// accelerator selected for a particular backend archive.
type AssetHostPlatform struct {
	OperatingSystem, Architecture, Accelerator string
	CUDAAvailable                              bool
}
