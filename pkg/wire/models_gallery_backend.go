package wire

import (
	"context"

	modelswire "github.com/portpowered/infinite-you/pkg/services/models/wire"
)

func newLinuxBackendArtifactResolver(
	installer modelswire.GalleryBackendInstaller,
	client modelswire.AssetHTTPDoer,
) (modelswire.BackendArtifactResolver, error) {
	gallery, err := modelswire.NewGalleryBackendArtifactResolver(installer)
	if err != nil {
		return nil, err
	}
	published, err := modelswire.NewPublishedBackendArtifactResolver(client)
	if err != nil {
		return nil, err
	}
	return preferPublishedLinuxCUDA(published, gallery), nil
}

func preferPublishedLinuxCUDA(published, gallery modelswire.BackendArtifactResolver) modelswire.BackendArtifactResolver {
	return func(ctx context.Context, request modelswire.ResolvedHostConfiguration, offline bool) (modelswire.BackendArtifactSelection, error) {
		platform := request.Platform
		if !offline && (platform.Accelerator == "cuda" || (platform.Accelerator == "" && platform.CUDAAvailable)) {
			cudaRequest := request
			cudaRequest.Platform.Accelerator = "cuda"
			selection, err := published(ctx, cudaRequest, false)
			if err == nil && selection.Accelerator == "cuda" && selection.Name != "" {
				return selection, nil
			}
			if ctx.Err() != nil {
				return modelswire.BackendArtifactSelection{}, ctx.Err()
			}
		}
		return gallery(ctx, request, offline)
	}
}
