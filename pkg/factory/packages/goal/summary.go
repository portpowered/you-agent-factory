package goal

import (
	"fmt"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/work"
	workercompatibility "github.com/portpowered/infinite-you/pkg/workers/compatibility"
	workertaxonomy "github.com/portpowered/infinite-you/pkg/workers/taxonomy"
)

// ShouldFormatInvocationSummary reports whether workstation output should be
// shaped into packaged goal summary work content for terminal primary-result
// selection. The minimal factory's execute workstation is the only worker
// boundary that can produce terminal goal content.
func ShouldFormatInvocationSummary(workstation *interfaces.FactoryWorkstationConfig) bool {
	if workstation == nil {
		return false
	}
	if strings.TrimSpace(workstation.Name) != PackagedExecuteWorkstationName {
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

// SummaryContentFromWorkerOutput converts goal worker output into canonical text
// work content for invocation primary-result selection.
func SummaryContentFromWorkerOutput(output, stopToken string) ([]work.WorkContentPart, error) {
	summary := normalizeGoalSummaryText(output, stopToken)
	if summary == "" {
		return nil, fmt.Errorf("goal worker output is empty after normalization")
	}
	return []work.WorkContentPart{{
		Type: work.WorkContentPartTypeText,
		Text: summary,
	}}, nil
}

func normalizeGoalSummaryText(output, stopToken string) string {
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
