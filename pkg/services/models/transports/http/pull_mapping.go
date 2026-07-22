package http

import (
	"net/http"

	"github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func managedRuntimePullHTTPStatus(result models.PullResult) int {
	if factoryapi.ManagedRuntimePullOutcome(result.ManagedPullOutcome) == factoryapi.ManagedRuntimePullOutcomeTIMEDOUT {
		return http.StatusGatewayTimeout
	}
	return http.StatusUnprocessableEntity
}

func modelPullResponseFromService(result models.PullResult) factoryapi.ModelPullResponse {
	files := make([]factoryapi.ModelPullDownloadedFile, 0, len(result.DownloadedFiles))
	for _, file := range result.DownloadedFiles {
		current := factoryapi.ModelPullDownloadedFile{Path: file.Path, Bytes: file.Bytes}
		if sha := file.SHA256; sha != "" {
			current.Sha256 = &sha
		}
		files = append(files, current)
	}
	response := factoryapi.ModelPullResponse{
		ModelName: result.ModelName, ProviderLocality: factoryapi.WorkerModelLocality(result.ProviderLocality),
		Outcome: factoryapi.ModelPullOutcome(result.Outcome), CachePath: result.CachePath,
		Revision: result.Revision, DownloadedFiles: files,
	}
	response.ManagedRuntimePull = managedRuntimePullResultToGenerated(result, files)
	return response
}
