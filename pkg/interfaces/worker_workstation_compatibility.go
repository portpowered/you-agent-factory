package interfaces

import (
	"fmt"
	"strings"
)

// EffectiveWorkstationBehaviorClass resolves the runtime behavior class for a
// workstation, including legacy defaulting for standard agent workstations that
// omit an explicit type while binding a worker.
func EffectiveWorkstationBehaviorClass(workstationType string, kind WorkstationKind, hasWorker bool) string {
	if class := ProjectWorkstationBehaviorClass(workstationType, kind); class != "" {
		return class
	}
	if !hasWorker {
		return ""
	}
	if kind == WorkstationKindPoller {
		return WorkstationTypePoller
	}
	if strings.TrimSpace(workstationType) == "" && (kind == "" || kind == WorkstationKindStandard) {
		return WorkstationTypeAgent
	}
	return ""
}

// IsLegacyGrandfatheredWorkerWorkstationPair reports pairings that remain valid
// during the migration window even when behavior classes differ.
func IsLegacyGrandfatheredWorkerWorkstationPair(workerType, workstationType string, kind WorkstationKind) bool {
	worker := StrictPublicFactoryWorkerType(workerType)
	wsType := StrictPublicFactoryWorkstationType(workstationType)
	if worker == WorkerTypeModel && wsType == WorkstationTypeModel {
		return true
	}
	if wsType == WorkstationTypeModel && worker == WorkerTypeScript {
		return true
	}
	if wsType == "" && (kind == "" || kind == WorkstationKindStandard) && IsInferenceWorkerType(worker) {
		return true
	}
	if !IsPollerRunWorkstationType(workstationType, kind) {
		return false
	}
	switch worker {
	case WorkerTypeScript, WorkerTypeHosted, WorkerTypePoller:
		return true
	default:
		return false
	}
}

// RequiresWorkerWorkstationBehaviorCompatibility reports whether the workstation
// should be checked against its bound worker behavior class.
func RequiresWorkerWorkstationBehaviorCompatibility(workstationType string, kind WorkstationKind, workerTypeName string) bool {
	if strings.TrimSpace(workerTypeName) == "" {
		return false
	}
	switch EffectiveWorkstationBehaviorClass(workstationType, kind, true) {
	case WorkstationTypeLogical, WorkstationTypeClassify, "":
		return false
	default:
		return true
	}
}

// CompatibleWorkerWorkstationBehavior reports whether worker and workstation
// taxonomy values describe the same runtime behavior class, including supported
// legacy alias pairings.
func CompatibleWorkerWorkstationBehavior(workerType, workstationType string, kind WorkstationKind) bool {
	if strings.TrimSpace(workerType) == "" {
		return true
	}
	if IsLegacyGrandfatheredWorkerWorkstationPair(workerType, workstationType, kind) {
		return true
	}
	workerClass := ProjectWorkerBehaviorClass(workerType)
	wsClass := EffectiveWorkstationBehaviorClass(workstationType, kind, true)
	if workerClass == "" || wsClass == "" {
		return true
	}
	return RuntimeBehaviorClassLabel(workerClass) == RuntimeBehaviorClassLabel(wsClass)
}

// RuntimeBehaviorClassLabel returns customer-facing behavior terminology for
// validation findings.
func RuntimeBehaviorClassLabel(behaviorClass string) string {
	switch behaviorClass {
	case WorkerTypeInference, WorkstationTypeInference:
		return "inference"
	case WorkerTypeAgent, WorkstationTypeAgent:
		return "agent"
	case WorkerTypeScript, WorkstationTypeScript:
		return "script"
	case WorkerTypePoller, WorkstationTypePoller:
		return "poller"
	default:
		return strings.ToLower(strings.TrimSpace(behaviorClass))
	}
}

// WorkerWorkstationBehaviorMismatchMessage formats an actionable compatibility
// finding that names the provided worker and workstation taxonomy values.
func WorkerWorkstationBehaviorMismatchMessage(
	workstationName string,
	workstationType string,
	kind WorkstationKind,
	workerName string,
	workerType string,
) string {
	wsClass := EffectiveWorkstationBehaviorClass(workstationType, kind, true)
	wsLabel := displayWorkstationTaxonomyValue(workstationType, kind)
	workerLabel := displayWorkerTaxonomyValue(workerType)
	return fmt.Sprintf(
		"workstation %q (%s) is a %s-run workstation but worker %q (%s) is a %s worker; bind a compatible %s worker or change the workstation type",
		workstationName,
		wsLabel,
		RuntimeBehaviorClassLabel(wsClass),
		workerName,
		workerLabel,
		RuntimeBehaviorClassLabel(ProjectWorkerBehaviorClass(workerType)),
		RuntimeBehaviorClassLabel(wsClass),
	)
}

func displayWorkstationTaxonomyValue(workstationType string, kind WorkstationKind) string {
	trimmed := strings.TrimSpace(workstationType)
	if trimmed != "" {
		return trimmed
	}
	if kind == WorkstationKindPoller {
		return "legacy poller kind"
	}
	return "legacy agent-run default"
}

func displayWorkerTaxonomyValue(workerType string) string {
	trimmed := strings.TrimSpace(workerType)
	if trimmed != "" {
		return trimmed
	}
	return "unspecified worker type"
}
