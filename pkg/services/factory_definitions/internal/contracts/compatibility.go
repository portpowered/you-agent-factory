package factorycontracts

import (
	"fmt"
	"strings"
)

type Workstation struct {
	Name           string
	Type           string
	Kind           WorkstationKind
	WorkerTypeName string
}

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
func ExemptFromWorkerWorkstationCompatibility(workstation Workstation) bool {
	switch strings.TrimSpace(workstation.Type) {
	case WorkstationTypeLogical, WorkstationTypeClassify:
		return true
	default:
		return false
	}
}

// EffectiveWorkstationTypeForCompatibility returns the workstation type used for
// behavior pairing when the authored type is omitted.
func EffectiveWorkstationTypeForCompatibility(workstation Workstation) string {
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
	switch PublicWorkerTypeFromInternal(workerType) {
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
func ExpectedWorkerBehaviorClassForWorkstation(workstation Workstation, workerType string) (WorkerWorkstationBehaviorClass, bool) {
	if ExemptFromWorkerWorkstationCompatibility(workstation) {
		return "", false
	}

	workstationType := EffectiveWorkstationTypeForCompatibility(workstation)
	if IsPollerRunPublicWorkstationType(workstationType, WorkstationKind(workstation.Kind)) {
		return WorkerWorkstationBehaviorPoller, true
	}

	switch PublicWorkstationTypeFromInternalRuntime(workstationType, workerType, WorkstationKind(workstation.Kind)) {
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
func WorkerMatchesWorkstationBehavior(workerType string, workstation Workstation) bool {
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
func PublicWorkerTypeForFactoryUsage(worker Config, workstations []Workstation) string {
	if strings.TrimSpace(worker.Type) != WorkerTypeModel {
		return PublicWorkerTypeFromInternal(worker.Type)
	}

	usageClasses := modelWorkerUsageBehaviorClasses(worker.Name, workstations)
	if len(usageClasses) == 0 {
		return PublicWorkerTypeFromInternal(worker.Type)
	}

	publicTypes := make(map[string]struct{}, len(usageClasses))
	for _, class := range usageClasses {
		publicTypes[publicWorkerTypeForBehaviorClass(class)] = struct{}{}
	}
	if len(publicTypes) > 1 {
		// Legacy MODEL_WORKER factories may share one worker across agent and
		// inference workstations; preserve the alias instead of projecting to a
		// single taxonomy name that would invalidate the other pairing.
		return WorkerTypeModel
	}
	for publicType := range publicTypes {
		return publicType
	}
	return PublicWorkerTypeFromInternal(worker.Type)
}

func publicWorkerTypeForBehaviorClass(class WorkerWorkstationBehaviorClass) string {
	switch class {
	case WorkerWorkstationBehaviorInference:
		return WorkerTypeInference
	case WorkerWorkstationBehaviorPoller:
		return WorkerTypePoller
	case WorkerWorkstationBehaviorScript:
		return WorkerTypeScript
	case WorkerWorkstationBehaviorAgent:
		return WorkerTypeAgent
	default:
		return WorkerTypeModel
	}
}

func modelWorkerUsageBehaviorClasses(workerName string, workstations []Workstation) []WorkerWorkstationBehaviorClass {
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

// EffectiveWorkstationBehaviorClass resolves the runtime behavior class for a
// workstation, including legacy defaulting for standard agent workstations that
// omit an explicit type while binding a worker.
func EffectiveWorkstationBehaviorClass(workstationType string, kind WorkstationKind, hasWorker bool) string {
	if class := ProjectWorkstationBehaviorClass(workstationType, WorkstationKind(kind)); class != "" {
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
	workerTrimmed := strings.TrimSpace(workerType)
	wsTrimmed := strings.TrimSpace(workstationType)
	if workerTrimmed == WorkerTypeModel && wsTrimmed == WorkstationTypeModel {
		return true
	}
	if wsTrimmed == WorkstationTypeModel && workerTrimmed == WorkerTypeScript {
		return true
	}
	if wsTrimmed == "" && (kind == "" || kind == WorkstationKindStandard) {
		if IsInferenceWorkerType(workerType) || workerTrimmed == WorkerTypeScript {
			return true
		}
	}
	if !IsPollerRunWorkstationType(workstationType, WorkstationKind(kind)) {
		return false
	}
	switch StrictWorkerType(workerType) {
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
