package wire

import (
	models "github.com/portpowered/infinite-you/pkg/services/models"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
)

func invocationArtifactSources(artifacts []models.InferenceArtifact) []inference.InvocationArtifactSource {
	if len(artifacts) == 0 {
		return nil
	}
	sources := make([]inference.InvocationArtifactSource, 0, len(artifacts))
	for _, artifact := range artifacts {
		sources = append(sources, inference.InvocationArtifactSource{
			RefValue:   artifact.Artifact.String(),
			Name:       artifact.Name,
			MediaType:  artifact.MediaType,
			SizeBytes:  artifact.SizeBytes,
			Properties: artifact.Properties,
		})
	}
	return sources
}
