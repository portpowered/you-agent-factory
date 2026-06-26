package subsystems

import (
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/packagedfactories/goal"
)

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
