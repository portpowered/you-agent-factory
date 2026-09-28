package wire

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"runtime"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
)

func TestLinuxCUDAAvailableRequiresDeviceAndWorkingNvidiaProbe(t *testing.T) {
	t.Parallel()
	device := func(path string) (os.FileInfo, error) {
		if path == "/dev/dxg" {
			return nil, nil
		}
		return nil, fs.ErrNotExist
	}
	if !linuxCUDAAvailable(device, func(context.Context) ([]byte, error) {
		return []byte("GPU 0: NVIDIA GeForce RTX 4090"), nil
	}) {
		t.Fatal("WSL NVIDIA device and probe were not recognized")
	}
	if linuxCUDAAvailable(device, func(context.Context) ([]byte, error) {
		return nil, errors.New("nvidia-smi failed")
	}) {
		t.Fatal("failed NVIDIA probe was accepted")
	}
	if linuxCUDAAvailable(func(string) (os.FileInfo, error) { return nil, fs.ErrNotExist },
		func(context.Context) ([]byte, error) { t.Fatal("probe called without a GPU device"); return nil, nil }) {
		t.Fatal("missing NVIDIA device was accepted")
	}
}

func TestWindowsCUDAAvailableRequiresWorkingNvidiaProbe(t *testing.T) {
	t.Parallel()
	if !windowsCUDAAvailable(func(context.Context) ([]byte, error) {
		return []byte("GPU 0: NVIDIA GeForce RTX 4090"), nil
	}) {
		t.Fatal("Windows NVIDIA device was not recognized")
	}
	if windowsCUDAAvailable(func(context.Context) ([]byte, error) {
		return nil, errors.New("nvidia-smi failed")
	}) {
		t.Fatal("failed NVIDIA probe was accepted")
	}
	if windowsCUDAAvailable(func(context.Context) ([]byte, error) {
		return []byte("  \n"), nil
	}) {
		t.Fatal("empty NVIDIA device list was accepted")
	}
	if windowsCUDAAvailable(func(context.Context) ([]byte, error) {
		return []byte("No devices were found"), nil
	}) {
		t.Fatal("non-GPU output was accepted")
	}
}

func TestWindowsHostRecordsCUDAWithoutSelectingUnpublishedArchive(t *testing.T) {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		t.Skip("native Windows host probe")
	}
	expected := windowsCUDAAvailable(func(ctx context.Context) ([]byte, error) {
		return exec.CommandContext(ctx, "nvidia-smi", "-L").Output()
	})
	platform := provideModelAssetHostPlatform(serviceedges.Edges{})
	if platform.CUDAAvailable != expected || platform.Accelerator != "" {
		t.Fatalf("Windows host platform = %#v, want CUDA capability %t and no selected archive", platform, expected)
	}
}
