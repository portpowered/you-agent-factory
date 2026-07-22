package local

import (
	"context"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models/internal/assets"
)

func mustNewAssetPuller(t *testing.T, cacheDir string) AssetPuller {
	t.Helper()
	puller, err := newAssetPullerWithPlatform(
		cacheDir,
		HostPlatform{OperatingSystem: runtime.GOOS, Architecture: runtime.GOARCH},
	)
	if err != nil {
		t.Fatalf("NewAssetPuller: %v", err)
	}
	return puller
}

func newAssetPullerWithPlatform(cacheDir string, platform HostPlatform) (AssetPuller, error) {
	return NewAssetPuller(
		cacheDir,
		platform,
		http.DefaultClient,
		assets.DefaultEndpoints(),
		os.MkdirAll,
		os.Stat,
		os.UserHomeDir,
		os.WriteFile,
		os.Rename,
		os.Remove,
		os.ReadFile,
		os.ReadDir,
		func(path string) (io.WriteCloser, error) { return os.Create(path) },
		func(path string) (io.ReadCloser, error) { return os.Open(path) },
	)
}

func TestNewAssetPullerUsesInjectedHostPlatform(t *testing.T) {
	t.Parallel()

	puller, err := newAssetPullerWithPlatform(t.TempDir(), HostPlatform{
		OperatingSystem: "customer-os",
		Architecture:    "customer-arch",
	})
	if err != nil {
		t.Fatalf("NewAssetPuller: %v", err)
	}
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	_, err = puller.PullModel(context.Background(), loaded, "OMNIVOICE_Q4_K_M")
	if err == nil || !strings.Contains(err.Error(), "customer-os/customer-arch") {
		t.Fatalf("PullModel error = %v, want injected customer-os/customer-arch compatibility decision", err)
	}
}

func TestNewAssetPullerRejectsMissingHostPlatform(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name     string
		platform HostPlatform
		want     string
	}{
		{name: "operating system", platform: HostPlatform{Architecture: "amd64"}, want: "host operating system is required"},
		{name: "architecture", platform: HostPlatform{OperatingSystem: "linux"}, want: "host architecture is required"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := newAssetPullerWithPlatform(t.TempDir(), test.platform)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewAssetPuller error = %v, want %q", err, test.want)
			}
		})
	}
}
