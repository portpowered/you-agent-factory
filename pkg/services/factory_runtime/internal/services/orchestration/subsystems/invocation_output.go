package subsystems

import (
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
)

func applyPackagedTTSInvocationMetadata(
	outputShaping factorydefinitions.InvocationOutputShapingService,
	token *factorytoken.Token,
	workstation *factorydefinitions.FactoryWorkstationConfig,
	workerOutput string,
	inputColors []factorytoken.Color,
	runtimeConfig factorydefinitions.RuntimeWorkstationLookup,
) error {
	if outputShaping == nil || token == nil || !outputShaping.ShouldFormatTTSInvocationMetadata(workstation) {
		return nil
	}
	if strings.TrimSpace(workerOutput) == "" {
		return nil
	}

	traceID := ""
	if source := firstNonResourceInput(inputColors); source != nil {
		traceID = strings.TrimSpace(source.TraceID)
	}

	backendLabel := ""
	if workstation != nil && runtimeConfig != nil {
		if lookup, ok := runtimeConfig.(factorydefinitions.RuntimeDefinitionLookup); ok {
			if worker, ok := lookup.Worker(strings.TrimSpace(workstation.WorkerTypeName)); ok && worker != nil {
				backendLabel = outputShaping.TTSBackendLabelFromWorker(worker)
			}
		}
	}

	metadataContent, err := outputShaping.TTSMetadataContentFromWorkerOutput(
		workerOutput,
		traceID,
		"",
		backendLabel,
	)
	if err != nil {
		// Packaged TTS metadata is only shaped from successful audio output.
		return nil
	}

	token.Color.Content = metadataContent
	token.Color.Payload = nil
	return nil
}

func applyPackagedGoalInvocationSummary(
	outputShaping factorydefinitions.InvocationOutputShapingService,
	token *factorytoken.Token,
	workstation *factorydefinitions.FactoryWorkstationConfig,
	workerOutput string,
	runtimeConfig factorydefinitions.RuntimeWorkstationLookup,
) error {
	if outputShaping == nil || token == nil || !outputShaping.ShouldFormatInvocationSummary(workstation) {
		return nil
	}
	if strings.TrimSpace(workerOutput) == "" {
		return nil
	}

	stopToken := workstationStopToken(workstation, runtimeConfig)
	summaryContent, err := outputShaping.SummaryContentFromWorkerOutput(workerOutput, stopToken)
	if err != nil {
		return fmt.Errorf("shape packaged goal invocation summary: %w", err)
	}

	token.Color.Content = summaryContent
	token.Color.Payload = nil
	return nil
}

func applyPackagedSubagentInvocationResponse(
	outputShaping factorydefinitions.InvocationOutputShapingService,
	token *factorytoken.Token,
	workstation *factorydefinitions.FactoryWorkstationConfig,
	workerOutput string,
	runtimeConfig factorydefinitions.RuntimeWorkstationLookup,
) error {
	if outputShaping == nil || token == nil || !outputShaping.ShouldFormatInvocationResponse(workstation) {
		return nil
	}
	if strings.TrimSpace(workerOutput) == "" {
		return nil
	}

	stopToken := workstationStopToken(workstation, runtimeConfig)
	responseContent, err := outputShaping.ResponseContentFromWorkerOutput(workerOutput, stopToken)
	if err != nil {
		return fmt.Errorf("shape packaged subagent invocation response: %w", err)
	}

	token.Color.Content = responseContent
	token.Color.Payload = nil
	return nil
}

func workstationStopToken(
	workstation *factorydefinitions.FactoryWorkstationConfig,
	runtimeConfig factorydefinitions.RuntimeWorkstationLookup,
) string {
	if workstation == nil || runtimeConfig == nil {
		return ""
	}
	lookup, ok := runtimeConfig.(factorydefinitions.RuntimeDefinitionLookup)
	if !ok {
		return ""
	}
	worker, ok := lookup.Worker(strings.TrimSpace(workstation.WorkerTypeName))
	if !ok || worker == nil {
		return ""
	}
	return strings.TrimSpace(worker.StopToken)
}
