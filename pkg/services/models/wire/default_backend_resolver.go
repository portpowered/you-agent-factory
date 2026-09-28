package wire

import (
	"context"
	"errors"
	"fmt"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/artifacts"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
)

// NewDefaultBackendArtifactResolver constructs the production selector for
// the checked-in pinned publication. The manifest is decoded and validated at
// composition time; the returned effect only performs deterministic
// capability selection and returns detached archive facts.
func NewDefaultBackendArtifactResolver() (BackendArtifactResolver, error) {
	manifest, err := artifacts.DefaultManifest()
	if err != nil {
		return nil, fmt.Errorf("decode default backend artifact manifest: %w", err)
	}
	return backendArtifactResolver(manifest), nil
}

func backendArtifactResolver(manifest artifacts.Manifest) BackendArtifactResolver {
	return func(ctx context.Context, request ResolvedHostConfiguration, _ bool) (BackendArtifactSelection, error) {
		if err := ctx.Err(); err != nil {
			return BackendArtifactSelection{}, err
		}
		if request.ProtocolVersion != modelseffects.PinnedHostProtocolVersion {
			return BackendArtifactSelection{}, fmt.Errorf(
				"%w: requested protocol %q",
				artifacts.ErrIncompatibleProtocol,
				request.ProtocolVersion,
			)
		}
		accelerator := defaultBackendAccelerator(request.Platform)
		preferCUDA := request.Platform.Accelerator == "" && accelerator == "cuda"
		selection := artifacts.SelectionRequest{
			Backend:          request.Backend,
			OperatingSystem:  request.Platform.OperatingSystem,
			Architecture:     request.Platform.Architecture,
			ProtocolRevision: manifest.ProtocolRevision(),
			Accelerator:      accelerator,
		}
		descriptor, err := manifest.Select(selection)
		if preferCUDA && errors.Is(err, artifacts.ErrIncompatibleAccelerator) {
			selection.Accelerator = "cpu"
			descriptor, err = manifest.Select(selection)
		}
		if err != nil {
			return BackendArtifactSelection{}, err
		}
		return BackendArtifactSelection{
			Name:        descriptor.Artifact.Name,
			Location:    descriptor.Artifact.Location,
			Bytes:       descriptor.Artifact.SizeBytes,
			SHA256:      descriptor.Artifact.SHA256,
			Accelerator: selection.Accelerator,
		}, nil
	}
}

func defaultBackendAccelerator(platform models.AssetHostPlatform) string {
	if platform.Accelerator != "" {
		return platform.Accelerator
	}
	if platform.CUDAAvailable && platform.Architecture == "amd64" &&
		(platform.OperatingSystem == "linux" || platform.OperatingSystem == "windows") {
		return "cuda"
	}
	if platform.OperatingSystem == "darwin" && platform.Architecture == "arm64" {
		return "metal"
	}
	if (platform.OperatingSystem == "linux" || platform.OperatingSystem == "windows") &&
		platform.Architecture == "amd64" {
		return "cpu"
	}
	return ""
}
