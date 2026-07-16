package apisurface

import (
	"net/http"

	modelassets "github.com/portpowered/infinite-you/pkg/models/assets"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// ManagedRuntimePullError reports a classified managed-runtime pull failure while
// preserving the transport-visible pull result contract.
type ManagedRuntimePullError = modelassets.PullError

// IsManagedRuntimePullError reports whether err carries a classified managed-runtime
// pull failure result for API and CLI transport mapping.
func IsManagedRuntimePullError(err error) bool {
	_, ok := modelassets.AsPullError(err)
	return ok
}

// AsManagedRuntimePullError returns the classified pull failure when present.
func AsManagedRuntimePullError(err error) (*ManagedRuntimePullError, bool) {
	return modelassets.AsPullError(err)
}

// ManagedRuntimePullHTTPStatus maps one classified pull failure to an HTTP status.
func ManagedRuntimePullHTTPStatus(result ModelPullResult) int {
	switch factoryapi.ManagedRuntimePullOutcome(result.ManagedPullOutcome) {
	case factoryapi.ManagedRuntimePullOutcomeTIMEDOUT:
		return http.StatusGatewayTimeout
	case factoryapi.ManagedRuntimePullOutcomeSOURCEFETCHFAILED,
		factoryapi.ManagedRuntimePullOutcomeSTILLLOADING:
		return http.StatusUnprocessableEntity
	default:
		return http.StatusUnprocessableEntity
	}
}

// ModelPullResponseFromService maps a service-owned pull result into the public
// pull response contract.
func ModelPullResponseFromService(result ModelPullResult) factoryapi.ModelPullResponse {
	files := make([]factoryapi.ModelPullDownloadedFile, 0, len(result.DownloadedFiles))
	for _, file := range result.DownloadedFiles {
		current := factoryapi.ModelPullDownloadedFile{
			Path:  file.Path,
			Bytes: file.Bytes,
		}
		if sha := file.SHA256; sha != "" {
			current.Sha256 = &sha
		}
		files = append(files, current)
	}
	response := factoryapi.ModelPullResponse{
		ModelName:        result.ModelName,
		ProviderLocality: factoryapi.WorkerModelLocality(result.ProviderLocality),
		Outcome:          factoryapi.ModelPullOutcome(result.Outcome),
		CachePath:        result.CachePath,
		Revision:         result.Revision,
		DownloadedFiles:  files,
	}
	response.ManagedRuntimePull = ManagedRuntimePullResultFromService(result, files)
	return response
}
