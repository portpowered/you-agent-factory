package cli

import (
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func pullResultToGenerated(result models.PullResult) factoryapi.ModelPullResponse {
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

func managedRuntimePullResultToGenerated(result models.PullResult, files []factoryapi.ModelPullDownloadedFile) factoryapi.ManagedRuntimePullResult {
	pull := factoryapi.ManagedRuntimePullResult{
		Identity:       result.ModelName,
		PullOutcome:    managedRuntimePullOutcome(result),
		ReadinessState: managedRuntimePullReadiness(result),
	}
	if cachePath := strings.TrimSpace(result.CachePath); cachePath != "" {
		pull.CachePath = &cachePath
	}
	if revision := strings.TrimSpace(result.Revision); revision != "" {
		pull.Revision = &revision
	}
	if len(files) > 0 {
		copied := append([]factoryapi.ModelPullDownloadedFile(nil), files...)
		pull.DownloadedFiles = &copied
	}
	if diagnostics := managedRuntimePullSourceDiagnostics(result); diagnostics != nil {
		pull.SourceDiagnostics = diagnostics
	}
	return pull
}

func managedRuntimePullOutcome(result models.PullResult) factoryapi.ManagedRuntimePullOutcome {
	if outcome := strings.TrimSpace(result.ManagedPullOutcome); outcome != "" {
		return factoryapi.ManagedRuntimePullOutcome(outcome)
	}
	switch factoryapi.ModelPullOutcome(strings.TrimSpace(strings.ToUpper(result.Outcome))) {
	case factoryapi.ModelPullOutcomePULLED:
		return factoryapi.ManagedRuntimePullOutcomeINSTALLEDSUCCESSFULLY
	case factoryapi.ModelPullOutcomeALREADYPRESENT:
		return factoryapi.ManagedRuntimePullOutcomeALREADYPRESENT
	default:
		return factoryapi.ManagedRuntimePullOutcomeUNSUPPORTEDRUNTIME
	}
}

func managedRuntimePullReadiness(result models.PullResult) factoryapi.ManagedRuntimeReadinessState {
	if readiness := strings.TrimSpace(result.ReadinessState); readiness != "" {
		return factoryapi.ManagedRuntimeReadinessState(readiness)
	}
	return factoryapi.ManagedRuntimeReadinessStateREADY
}

func managedRuntimePullSourceDiagnostics(result models.PullResult) *factoryapi.ManagedRuntimeSourceDiagnostics {
	sourceKind := strings.TrimSpace(result.SourceKind)
	sourceID := strings.TrimSpace(result.SourceID)
	resolverNotes := strings.TrimSpace(result.ResolverNotes)
	if sourceKind == "" && sourceID == "" && resolverNotes == "" {
		return nil
	}
	diagnostics := factoryapi.ManagedRuntimeSourceDiagnostics{}
	if sourceKind != "" {
		diagnostics.SourceKind = &sourceKind
	}
	if sourceID != "" {
		diagnostics.SourceId = &sourceID
	}
	if resolverNotes != "" {
		diagnostics.ResolverNotes = &resolverNotes
	}
	return &diagnostics
}
