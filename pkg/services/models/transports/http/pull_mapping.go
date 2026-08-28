package http

import (
	"github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func modelPullResponseFromService(result models.PullResult) factoryapi.ModelPullResponse {
	files := make([]factoryapi.ModelPullDownloadedFile, 0, len(result.DownloadedFiles))
	for _, file := range result.DownloadedFiles {
		current := factoryapi.ModelPullDownloadedFile{Path: file.Path, Bytes: file.Bytes}
		if sha := file.SHA256; sha != "" {
			current.Sha256 = &sha
		}
		files = append(files, current)
	}
	managedPull := managedRuntimePullResultToGenerated(result, files)
	response := factoryapi.ModelPullResponse{
		ModelName: result.ModelName, ProviderLocality: factoryapi.WorkerModelLocality(result.ProviderLocality),
		Outcome: modelPullOutcomeFromManagedRuntime(managedPull.PullOutcome), CachePath: result.CachePath,
		Revision: result.Revision, DownloadedFiles: files,
		ManagedRuntimePull: managedPull,
	}
	return response
}

func modelPullOutcomeFromManagedRuntime(outcome factoryapi.ManagedRuntimePullOutcome) factoryapi.ModelPullOutcome {
	switch outcome {
	case factoryapi.ManagedRuntimePullOutcomeINSTALLEDSUCCESSFULLY:
		return factoryapi.ModelPullOutcomePULLED
	case factoryapi.ManagedRuntimePullOutcomeALREADYPRESENT,
		factoryapi.ManagedRuntimePullOutcomeALREADYREADY:
		return factoryapi.ModelPullOutcomeALREADYPRESENT
	case factoryapi.ManagedRuntimePullOutcomeSTILLLOADING,
		factoryapi.ManagedRuntimePullOutcomeTIMEDOUT,
		factoryapi.ManagedRuntimePullOutcomeSOURCEFETCHFAILED,
		factoryapi.ManagedRuntimePullOutcomeUNSUPPORTEDRUNTIME:
		return factoryapi.ModelPullOutcomeFAILED
	default:
		return factoryapi.ModelPullOutcomeFAILED
	}
}

func modelRemoveResponseFromService(result models.RemoveModelAssetsResult) factoryapi.ModelRemoveResponse {
	return factoryapi.ModelRemoveResponse{
		ModelName:    result.ModelName,
		Revision:     result.Revision,
		CachePath:    result.CachePath,
		Outcome:      factoryapi.ModelRemoveOutcome(result.Outcome),
		BytesRemoved: result.BytesRemoved,
	}
}
