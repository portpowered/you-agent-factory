package wire

import (
	"context"
	"errors"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	models "github.com/portpowered/infinite-you/pkg/services/models"
)

func TestWindowsCUDAAvailableProbeDetectsDevice(t *testing.T) {
	t.Parallel()
	output := func(context.Context) ([]byte, error) {
		return []byte("GPU 0: NVIDIA GeForce RTX 4090"), nil
	}
	if !windowsCUDAAvailable(output) {
		t.Fatal("Windows nvidia-smi probe with device was not recognized")
	}
}

func TestWindowsCUDAAvailableProbeRejectsFailedProbe(t *testing.T) {
	t.Parallel()
	output := func(context.Context) ([]byte, error) {
		return nil, errors.New("nvidia-smi failed")
	}
	if windowsCUDAAvailable(output) {
		t.Fatal("failed Windows NVIDIA probe was accepted")
	}
}

func TestWindowsCUDAAvailableProbeRejectsNoDevice(t *testing.T) {
	t.Parallel()
	output := func(context.Context) ([]byte, error) {
		return nil, errors.New("no NVIDIA devices")
	}
	if windowsCUDAAvailable(output) {
		t.Fatal("Windows nvidia-smi probe without device was accepted")
	}
}

func TestModelAssetHostPlatformWindowsAcceleratorDetection(t *testing.T) {
	t.Parallel()
	platform := models.AssetHostPlatform{
		OperatingSystem: "windows",
		Architecture:    "amd64",
	}
	result := provideModelAssetHostPlatform(serviceedges.Edges{ModelAssetHostPlatform: platform})
	if result.Accelerator != "" {
		t.Fatalf("expected automatic Windows accelerator selection, got %q", result.Accelerator)
	}
}

func TestModelAssetHostPlatformWindowsNoAcceleratorWhenNoProbe(t *testing.T) {
	t.Parallel()
	// When windowsCUDAAvailable probe returns an error, Accelerator should remain empty
	if windowsCUDAAvailable(func(context.Context) ([]byte, error) {
		return nil, errors.New("nvidia-smi failed")
	}) {
		t.Fatal("windowsCUDAAvailable with failed probe was accepted")
	}
}

func TestModelAssetHostPlatformWindowsArm64NoAccelerator(t *testing.T) {
	t.Parallel()
	platform := models.AssetHostPlatform{
		OperatingSystem: "windows",
		Architecture:    "arm64",
	}
	result := provideModelAssetHostPlatform(serviceedges.Edges{ModelAssetHostPlatform: platform})
	if result.Accelerator != "" {
		t.Fatalf("expected empty Accelerator on arm64, got %q", result.Accelerator)
	}
}

func TestModelAssetHostPlatformLinuxNoAcceleratorWhenNoDevice(t *testing.T) {
	t.Parallel()
	platform := models.AssetHostPlatform{
		OperatingSystem: "linux",
		Architecture:    "amd64",
	}
	result := provideModelAssetHostPlatform(serviceedges.Edges{ModelAssetHostPlatform: platform})
	if result.Accelerator != "" {
		t.Fatalf("expected empty Accelerator on Linux without device, got %q", result.Accelerator)
	}
}
