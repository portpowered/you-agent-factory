package subsystems

import (
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/packagedfactories/goal"
	"github.com/portpowered/infinite-you/pkg/packagedfactories/tts"
)

func tokenColorsFromTokens(tokens []interfaces.Token) []interfaces.TokenColor {
	colors := make([]interfaces.TokenColor, len(tokens))
	for i, token := range tokens {
		colors[i] = token.Color
	}
	return colors
}

func firstNonResourceInput(inputs []interfaces.TokenColor) *interfaces.TokenColor {
	for i := range inputs {
		if inputs[i].DataType != interfaces.DataTypeResource && inputs[i].WorkTypeID != interfaces.SystemTimeWorkTypeID {
			return &inputs[i]
		}
	}
	for i := range inputs {
		if inputs[i].DataType != interfaces.DataTypeResource {
			return &inputs[i]
		}
	}
	return nil
}

func applyPackagedTTSInvocationMetadata(
	token *interfaces.Token,
	workstation *interfaces.FactoryWorkstationConfig,
	workerOutput string,
	inputColors []interfaces.TokenColor,
	runtimeConfig interfaces.RuntimeWorkstationLookup,
) error {
	if token == nil || !tts.ShouldFormatInvocationMetadata(workstation) {
		return nil
	}

	traceID := ""
	if source := firstNonResourceInput(inputColors); source != nil {
		traceID = strings.TrimSpace(source.TraceID)
	}

	backendLabel := ""
	if workstation != nil && runtimeConfig != nil {
		if lookup, ok := runtimeConfig.(interfaces.RuntimeDefinitionLookup); ok {
			if worker, ok := lookup.Worker(strings.TrimSpace(workstation.WorkerTypeName)); ok && worker != nil {
				backendLabel = tts.BackendLabelFromWorker(worker)
			}
		}
	}

	metadataContent, err := tts.MetadataContentFromWorkerOutput(workerOutput, traceID, "", backendLabel)
	if err != nil {
		return fmt.Errorf("shape packaged tts invocation metadata: %w", err)
	}

	token.Color.Content = metadataContent
	token.Color.Payload = nil
	return nil
}

func applyPackagedGoalInvocationSummary(
	token *interfaces.Token,
	workstation *interfaces.FactoryWorkstationConfig,
	workerOutput string,
	runtimeConfig interfaces.RuntimeWorkstationLookup,
) error {
	if token == nil || !goal.ShouldFormatInvocationSummary(workstation) {
		return nil
	}
	if strings.TrimSpace(workerOutput) == "" {
		return nil
	}

	stopToken := ""
	if workstation != nil && runtimeConfig != nil {
		if lookup, ok := runtimeConfig.(interfaces.RuntimeDefinitionLookup); ok {
			if worker, ok := lookup.Worker(strings.TrimSpace(workstation.WorkerTypeName)); ok && worker != nil {
				stopToken = strings.TrimSpace(worker.StopToken)
			}
		}
	}

	summaryContent, err := goal.SummaryContentFromWorkerOutput(workerOutput, stopToken)
	if err != nil {
		return fmt.Errorf("shape packaged goal invocation summary: %w", err)
	}

	token.Color.Content = summaryContent
	token.Color.Payload = nil
	return nil
}
