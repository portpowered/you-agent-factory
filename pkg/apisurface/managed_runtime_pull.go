package apisurface

import (
	"errors"
	"fmt"
	"net/http"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

// ManagedRuntimePullError reports a classified managed-runtime pull failure while
// preserving the transport-visible pull result contract.
type ManagedRuntimePullError struct {
	Result ModelPullResult
	Cause  error
}

func (e *ManagedRuntimePullError) Error() string {
	if e == nil {
		return ""
	}
	outcome := factoryapi.ManagedRuntimePullOutcome(e.Result.ManagedPullOutcome)
	readiness := factoryapi.ManagedRuntimeReadinessState(e.Result.ReadinessState)
	if outcome == "" {
		return fmt.Sprintf("managed runtime pull failed for %q", e.Result.ModelName)
	}
	return fmt.Sprintf(
		"managed runtime pull for %q failed with outcome %s (readiness %s)",
		e.Result.ModelName,
		outcome,
		readiness,
	)
}

func (e *ManagedRuntimePullError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// IsManagedRuntimePullError reports whether err carries a classified managed-runtime
// pull failure result for API and CLI transport mapping.
func IsManagedRuntimePullError(err error) bool {
	var pullErr *ManagedRuntimePullError
	return errors.As(err, &pullErr) && pullErr != nil
}

// AsManagedRuntimePullError returns the classified pull failure when present.
func AsManagedRuntimePullError(err error) (*ManagedRuntimePullError, bool) {
	var pullErr *ManagedRuntimePullError
	if !errors.As(err, &pullErr) || pullErr == nil {
		return nil, false
	}
	return pullErr, true
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
