package wire

import (
	"context"
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
	return func(ctx context.Context, request BackendArtifactSelectionRequest) (BackendArtifactSelection, error) {
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
		descriptor, err := manifest.Select(artifacts.SelectionRequest{
			Backend:          request.Backend,
			OperatingSystem:  request.Platform.OperatingSystem,
			Architecture:     request.Platform.Architecture,
			ProtocolRevision: manifest.ProtocolRevision(),
			Accelerator:      defaultBackendAccelerator(request.Platform),
		})
		if err != nil {
			return BackendArtifactSelection{}, err
		}
		return BackendArtifactSelection{
			Name:     descriptor.Artifact.Name,
			Location: descriptor.Artifact.Location,
			Bytes:    descriptor.Artifact.SizeBytes,
			SHA256:   descriptor.Artifact.SHA256,
		}, nil
	}, nil
}

func defaultBackendAccelerator(platform models.AssetHostPlatform) string {
	if platform.OperatingSystem == "darwin" && platform.Architecture == "arm64" {
		return "metal"
	}
	if (platform.OperatingSystem == "linux" || platform.OperatingSystem == "windows") &&
		platform.Architecture == "amd64" {
		return "cpu"
	}
	return ""
}
