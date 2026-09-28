package wire

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"testing"
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
