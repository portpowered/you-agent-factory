package internal

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func (service *combinedService) BuildPortableArtifact(
	request recordings.BuildPortableArtifactRequest,
) (recordings.BuildPortableArtifactResult, error) {
	return service.artifactsExport.BuildPortableArtifact(request)
}

func (service *combinedService) ValidatePortableArtifact(
	request recordings.ValidatePortableArtifactRequest,
) (recordings.ValidatePortableArtifactResult, error) {
	return service.artifactsExport.ValidatePortableArtifact(request)
}

func (service *combinedService) EncodePortableArtifact(
	request recordings.EncodePortableArtifactRequest,
) (recordings.EncodePortableArtifactResult, error) {
	return service.artifactsExport.EncodePortableArtifact(request)
}

func (service *combinedService) DecodePortableArtifact(
	request recordings.DecodePortableArtifactRequest,
) (recordings.DecodePortableArtifactResult, error) {
	return service.artifactsExport.DecodePortableArtifact(request)
}

func (service *combinedService) SummarizePortableArtifact(
	request recordings.SummarizePortableArtifactRequest,
) (recordings.SummarizePortableArtifactResult, error) {
	return service.artifactsExport.SummarizePortableArtifact(request)
}

func (service *combinedService) ExportPortableArtifact(
	ctx context.Context,
	request recordings.ExportPortableArtifactRequest,
) (recordings.ExportPortableArtifactResult, error) {
	return service.artifactsExport.ExportPortableArtifact(ctx, request)
}

func (service *combinedService) ReadPortableArtifact(
	ctx context.Context,
	request recordings.ReadPortableArtifactRequest,
) (recordings.ReadPortableArtifactResult, error) {
	return service.artifactsExport.ReadPortableArtifact(ctx, request)
}
