package apisurface

import (
	"strings"

	managedruntime "github.com/portpowered/infinite-you/pkg/models/managedruntime"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
)

// ManagedRuntimeToAPI maps the model-owned runtime projection to the public contract.
func ManagedRuntimeToAPI(runtime managedruntime.Runtime) factoryapi.ManagedRuntime {
	diagnostics := factoryapi.StringMap{}
	for key, value := range runtime.Diagnostics {
		diagnostics[key] = value
	}
	return factoryapi.ManagedRuntime{
		Identity:            runtime.Identity,
		ReadinessState:      factoryapi.ManagedRuntimeReadinessState(runtime.ReadinessState),
		LifecycleState:      factoryapi.ManagedRuntimeLifecycleState(runtime.LifecycleState),
		Locality:            factoryapi.WorkerModelLocality(runtime.Locality),
		SupportedOperations: operationsToAPI(runtime.SupportedOperations),
		Diagnostics:         &diagnostics,
	}
}

func operationsToAPI(operations []managedruntime.Operation) []factoryapi.ModelOperation {
	converted := make([]factoryapi.ModelOperation, 0, len(operations))
	for _, operation := range operations {
		item := factoryapi.ModelOperation{Name: operation.Name}
		if operation.Inputs != nil {
			inputs := operationSlotsToAPI(operation.Inputs)
			item.Inputs = &inputs
		}
		if operation.Outputs != nil {
			outputs := operationSlotsToAPI(operation.Outputs)
			item.Outputs = &outputs
		}
		converted = append(converted, item)
	}
	return converted
}

func operationSlotsToAPI(slots []managedruntime.OperationSlot) []factoryapi.ModelOperationSlot {
	converted := make([]factoryapi.ModelOperationSlot, 0, len(slots))
	for _, slot := range slots {
		contentTypes := make([]factoryapi.ModelOperationContentType, 0, len(slot.ContentTypes))
		for _, contentType := range slot.ContentTypes {
			contentTypes = append(contentTypes, factoryapi.ModelOperationContentType(contentType))
		}
		converted = append(converted, factoryapi.ModelOperationSlot{
			Name: slot.Name, ContentTypes: contentTypes, Required: slot.Required,
		})
	}
	return converted
}

// ManagedRuntimePullResultFromService maps a service-owned pull result into the
// public managed-runtime pull contract while preserving legacy outcome fields.
func ManagedRuntimePullResultFromService(result ModelPullResult, files []factoryapi.ModelPullDownloadedFile) factoryapi.ManagedRuntimePullResult {
	pull := factoryapi.ManagedRuntimePullResult{
		Identity:       result.ModelName,
		PullOutcome:    managedRuntimePullOutcomeFromService(result),
		ReadinessState: managedRuntimeReadinessFromService(result),
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
	if strings.TrimSpace(result.ProviderLocality) == workerconfig.ModelLocalityCloud {
		pull.ReadinessState = factoryapi.ManagedRuntimeReadinessStateREADY
		pull.PullOutcome = factoryapi.ManagedRuntimePullOutcomeALREADYREADY
	}
	return pull
}

func managedRuntimePullOutcomeFromService(result ModelPullResult) factoryapi.ManagedRuntimePullOutcome {
	if outcome := strings.TrimSpace(result.ManagedPullOutcome); outcome != "" {
		return factoryapi.ManagedRuntimePullOutcome(outcome)
	}
	return managedRuntimePullOutcomeFromLegacy(factoryapi.ModelPullOutcome(strings.TrimSpace(strings.ToUpper(result.Outcome))))
}

func managedRuntimeReadinessFromService(result ModelPullResult) factoryapi.ManagedRuntimeReadinessState {
	if readiness := strings.TrimSpace(result.ReadinessState); readiness != "" {
		return factoryapi.ManagedRuntimeReadinessState(readiness)
	}
	return factoryapi.ManagedRuntimeReadinessStateREADY
}

func managedRuntimePullOutcomeFromLegacy(outcome factoryapi.ModelPullOutcome) factoryapi.ManagedRuntimePullOutcome {
	switch outcome {
	case factoryapi.ModelPullOutcomePULLED:
		return factoryapi.ManagedRuntimePullOutcomeINSTALLEDSUCCESSFULLY
	case factoryapi.ModelPullOutcomeALREADYPRESENT:
		return factoryapi.ManagedRuntimePullOutcomeALREADYPRESENT
	default:
		return factoryapi.ManagedRuntimePullOutcomeUNSUPPORTEDRUNTIME
	}
}

func managedRuntimePullSourceDiagnostics(result ModelPullResult) *factoryapi.ManagedRuntimeSourceDiagnostics {
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
