package interfaces

import "strings"

// WorkerWorkstationBehaviorClass names the customer-facing dispatch behavior
// shared by compatible worker and workstation types.
type WorkerWorkstationBehaviorClass string

const (
	WorkerWorkstationBehaviorInference WorkerWorkstationBehaviorClass = "inference"
	WorkerWorkstationBehaviorAgent     WorkerWorkstationBehaviorClass = "agent"
	WorkerWorkstationBehaviorScript    WorkerWorkstationBehaviorClass = "script"
	WorkerWorkstationBehaviorPoller    WorkerWorkstationBehaviorClass = "poller"
)

// ExemptFromWorkerWorkstationCompatibility reports whether a workstation should
// skip worker/workstation behavior pairing checks.
func ExemptFromWorkerWorkstationCompatibility(workstation FactoryWorkstationConfig) bool {
	switch strings.TrimSpace(workstation.Type) {
	case WorkstationTypeLogical, WorkstationTypeClassify:
		return true
	default:
		return false
	}
}

// EffectiveWorkstationTypeForCompatibility returns the workstation type used for
// behavior pairing when the authored type is omitted.
func EffectiveWorkstationTypeForCompatibility(workstation FactoryWorkstationConfig) string {
	if trimmed := strings.TrimSpace(workstation.Type); trimmed != "" {
		return trimmed
	}
	if workstation.Kind == WorkstationKindPoller {
		return ""
	}
	return WorkstationTypeModel
}

// WorkerBehaviorClass maps a worker type to its customer-facing behavior class.
func WorkerBehaviorClass(workerType string) (WorkerWorkstationBehaviorClass, bool) {
	switch PublicWorkerTypeFromInternalRuntime(workerType) {
	case WorkerTypeInference:
		return WorkerWorkstationBehaviorInference, true
	case WorkerTypeAgent:
		return WorkerWorkstationBehaviorAgent, true
	case WorkerTypeScript:
		return WorkerWorkstationBehaviorScript, true
	case WorkerTypePoller:
		return WorkerWorkstationBehaviorPoller, true
	default:
		return "", false
	}
}

// ExpectedWorkerBehaviorClassForWorkstation maps a workstation to the worker
// behavior class it requires using the linked worker type for legacy script vs
// agent workstation projection.
func ExpectedWorkerBehaviorClassForWorkstation(workstation FactoryWorkstationConfig, workerType string) (WorkerWorkstationBehaviorClass, bool) {
	if ExemptFromWorkerWorkstationCompatibility(workstation) {
		return "", false
	}

	workstationType := EffectiveWorkstationTypeForCompatibility(workstation)
	if IsPollerRunPublicWorkstationType(workstationType, workstation.Kind) {
		return WorkerWorkstationBehaviorPoller, true
	}

	switch PublicWorkstationTypeFromInternalRuntime(workstationType, workerType, workstation.Kind) {
	case WorkstationTypeInference:
		return WorkerWorkstationBehaviorInference, true
	case WorkstationTypeAgent:
		return WorkerWorkstationBehaviorAgent, true
	case WorkstationTypeScript:
		return WorkerWorkstationBehaviorScript, true
	case WorkstationTypePoller:
		return WorkerWorkstationBehaviorPoller, true
	default:
		return "", false
	}
}

// WorkerMatchesWorkstationBehavior reports whether a worker type is compatible
// with a workstation's authored type and scheduling kind.
func WorkerMatchesWorkstationBehavior(workerType string, workstation FactoryWorkstationConfig) bool {
	if ExemptFromWorkerWorkstationCompatibility(workstation) {
		return true
	}

	workerType = strings.TrimSpace(workerType)
	if workerType == "" {
		return true
	}

	workstationType := EffectiveWorkstationTypeForCompatibility(workstation)
	if workstationType == WorkstationTypeModel && workerType == WorkerTypeModel {
		return true
	}

	expected, ok := ExpectedWorkerBehaviorClassForWorkstation(workstation, workerType)
	if !ok {
		return true
	}
	actual, ok := WorkerBehaviorClass(workerType)
	if !ok {
		return false
	}
	return actual == expected
}

// PublicWorkerTypeForFactoryUsage maps an internal worker type to the preferred
// public taxonomy name using workstation references to preserve legacy agent and
// script pairings for MODEL_WORKER during the migration window.
func PublicWorkerTypeForFactoryUsage(worker WorkerConfig, workstations []FactoryWorkstationConfig) string {
	if strings.TrimSpace(worker.Type) != WorkerTypeModel {
		return PublicWorkerTypeFromInternalRuntime(worker.Type)
	}

	usageClasses := modelWorkerUsageBehaviorClasses(worker.Name, workstations)
	if len(usageClasses) == 0 {
		return PublicWorkerTypeFromInternalRuntime(worker.Type)
	}
	if containsWorkerWorkstationBehaviorClass(usageClasses, WorkerWorkstationBehaviorInference) {
		return WorkerTypeInference
	}
	if containsWorkerWorkstationBehaviorClass(usageClasses, WorkerWorkstationBehaviorPoller) {
		return WorkerTypePoller
	}
	if containsWorkerWorkstationBehaviorClass(usageClasses, WorkerWorkstationBehaviorScript) &&
		!containsWorkerWorkstationBehaviorClass(usageClasses, WorkerWorkstationBehaviorAgent) {
		return WorkerTypeScript
	}
	if containsWorkerWorkstationBehaviorClass(usageClasses, WorkerWorkstationBehaviorAgent) {
		return WorkerTypeAgent
	}
	return PublicWorkerTypeFromInternalRuntime(worker.Type)
}

func modelWorkerUsageBehaviorClasses(workerName string, workstations []FactoryWorkstationConfig) []WorkerWorkstationBehaviorClass {
	workerName = strings.TrimSpace(workerName)
	if workerName == "" {
		return nil
	}

	seen := make(map[WorkerWorkstationBehaviorClass]struct{})
	var classes []WorkerWorkstationBehaviorClass
	for _, workstation := range workstations {
		if strings.TrimSpace(workstation.WorkerTypeName) != workerName {
			continue
		}
		if ExemptFromWorkerWorkstationCompatibility(workstation) {
			continue
		}
		class, ok := ExpectedWorkerBehaviorClassForWorkstation(workstation, WorkerTypeModel)
		if !ok {
			continue
		}
		if _, exists := seen[class]; exists {
			continue
		}
		seen[class] = struct{}{}
		classes = append(classes, class)
	}
	return classes
}

func containsWorkerWorkstationBehaviorClass(classes []WorkerWorkstationBehaviorClass, target WorkerWorkstationBehaviorClass) bool {
	for _, class := range classes {
		if class == target {
			return true
		}
	}
	return false
}

// DisplayWorkstationTypeForCompatibility returns the workstation type label to
// use in validation messages, preserving legacy authored values.
func DisplayWorkstationTypeForCompatibility(workstation FactoryWorkstationConfig) string {
	if trimmed := strings.TrimSpace(workstation.Type); trimmed != "" {
		return trimmed
	}
	if workstation.Kind == WorkstationKindPoller {
		return WorkstationTypePoller
	}
	return WorkstationTypeModel
}
