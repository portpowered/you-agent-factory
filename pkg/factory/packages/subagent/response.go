package subagent

import (
	"fmt"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/work"
	workercompatibility "github.com/portpowered/infinite-you/pkg/workers/compatibility"
	workertaxonomy "github.com/portpowered/infinite-you/pkg/workers/taxonomy"
)

// ShouldFormatInvocationResponse reports whether workstation output should be
// shaped into packaged subagent response work content for terminal
// primary-result selection.
func ShouldFormatInvocationResponse(workstation *interfaces.FactoryWorkstationConfig) bool {
	if workstation == nil {
		return false
	}
	if strings.TrimSpace(workstation.Name) != PackagedRunWorkstationName {
		return false
	}
	switch workercompatibility.EffectiveWorkstationTypeForCompatibility(workercompatibility.Workstation{
		Name: workstation.Name, Type: workstation.Type, Kind: workertaxonomy.WorkstationKind(workstation.Kind), WorkerTypeName: workstation.WorkerTypeName,
	}) {
	case interfaces.WorkstationTypeModel, interfaces.WorkstationTypeAgent:
		return true
	default:
		return false
	}
}

// ResponseContentFromWorkerOutput converts subagent worker output into canonical
// text work content for invocation primary-result selection.
func ResponseContentFromWorkerOutput(output, stopToken string) ([]work.WorkContentPart, error) {
	response := normalizeSubagentResponseText(output, stopToken)
	if response == "" {
		return nil, fmt.Errorf("subagent worker output is empty after normalization")
	}
	return []work.WorkContentPart{{
		Type: work.WorkContentPartTypeText,
		Text: response,
	}}, nil
}

func normalizeSubagentResponseText(output, stopToken string) string {
	trimmed := strings.TrimSpace(output)
	stopToken = strings.TrimSpace(stopToken)
	if stopToken == "" {
		return trimmed
	}
	if strings.HasSuffix(trimmed, stopToken) {
		return strings.TrimSpace(strings.TrimSuffix(trimmed, stopToken))
	}
	if idx := strings.LastIndex(trimmed, "\n"+stopToken); idx >= 0 {
		return strings.TrimSpace(trimmed[:idx])
	}
	return trimmed
}
