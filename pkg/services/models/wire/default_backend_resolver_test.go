package wire

import (
	"context"
	"errors"
	"testing"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/artifacts"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
)

func TestNewDefaultBackendArtifactResolverSelectsPinnedMatrix(t *testing.T) {
	t.Parallel()

	resolver, err := NewDefaultBackendArtifactResolver()
	if err != nil {
		t.Fatalf("NewDefaultBackendArtifactResolver: %v", err)
	}
	for _, test := range []struct {
		name     string
		backend  string
		platform models.AssetHostPlatform
	}{
		{name: "llamacpp linux", backend: "localai-llamacpp", platform: models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"}},
		{name: "llamacpp mac", backend: "localai-llamacpp", platform: models.AssetHostPlatform{OperatingSystem: "darwin", Architecture: "arm64"}},
		{name: "llamacpp windows", backend: "localai-llamacpp", platform: models.AssetHostPlatform{OperatingSystem: "windows", Architecture: "amd64"}},
		{name: "whisper linux", backend: "localai-whisper", platform: models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"}},
		{name: "vibevoice linux", backend: "localai-vibevoice", platform: models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			selection, err := resolver(context.Background(), BackendArtifactSelectionRequest{
				Backend: test.backend, Platform: test.platform,
				ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
			})
			if err != nil {
				t.Fatalf("resolve %s: %v", test.backend, err)
			}
			if selection.Name == "" || selection.Location == "" || selection.Bytes <= 0 || len(selection.SHA256) != 64 {
				t.Fatalf("selection = %#v, want detached pinned archive facts", selection)
			}
		})
	}
}

func TestNewDefaultBackendArtifactResolverRejectsIncompatibleRequests(t *testing.T) {
	t.Parallel()

	resolver, err := NewDefaultBackendArtifactResolver()
	if err != nil {
		t.Fatalf("NewDefaultBackendArtifactResolver: %v", err)
	}
	base := BackendArtifactSelectionRequest{
		Backend: "localai-vibevoice", Platform: models.AssetHostPlatform{
			OperatingSystem: "linux", Architecture: "amd64",
		}, ProtocolVersion: modelseffects.PinnedHostProtocolVersion,
	}
	tests := []struct {
		name string
		edit func(*BackendArtifactSelectionRequest)
		want error
	}{
		{name: "protocol", edit: func(request *BackendArtifactSelectionRequest) { request.ProtocolVersion = "localai-backend-v0" }, want: artifacts.ErrIncompatibleProtocol},
		{name: "platform", edit: func(request *BackendArtifactSelectionRequest) { request.Platform.OperatingSystem = "freebsd" }, want: artifacts.ErrUnsupportedPlatform},
		{name: "backend", edit: func(request *BackendArtifactSelectionRequest) { request.Backend = "localai-unknown" }, want: artifacts.ErrUnknownBackend},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := base
			test.edit(&request)
			_, err := resolver(context.Background(), request)
			if !errors.Is(err, test.want) {
				t.Fatalf("resolve error = %v, want %v", err, test.want)
			}
		})
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := resolver(cancelled, base); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled resolve error = %v, want context.Canceled", err)
	}
}

func TestNewDefaultHostCompatibilityCheckerUsesPinnedArtifactMatrix(t *testing.T) {
	t.Parallel()

	checker, err := NewDefaultHostCompatibilityChecker()
	if err != nil {
		t.Fatalf("NewDefaultHostCompatibilityChecker: %v", err)
	}
	if err := checker.Check(context.Background(), HostCompatibilityRequest{
		Backend:   "localai-llamacpp",
		ModelName: "llm",
		Platform:  models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
	}); err != nil {
		t.Fatalf("supported pinned host: %v", err)
	}
	if err := checker.Check(context.Background(), HostCompatibilityRequest{
		Backend:   "localai-llamacpp",
		ModelName: "llm",
		Platform:  models.AssetHostPlatform{OperatingSystem: "freebsd", Architecture: "amd64"},
	}); err == nil {
		t.Fatal("unsupported pinned host unexpectedly passed compatibility")
	}
}
