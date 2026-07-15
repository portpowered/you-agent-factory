package interfaces

import (
	workercompatibility "github.com/portpowered/infinite-you/pkg/workers/compatibility"
	workertaxonomy "github.com/portpowered/infinite-you/pkg/workers/taxonomy"
)

type WorkerWorkstationBehaviorClass = workercompatibility.WorkerWorkstationBehaviorClass

const (
	WorkerWorkstationBehaviorInference = workercompatibility.WorkerWorkstationBehaviorInference
	WorkerWorkstationBehaviorAgent     = workercompatibility.WorkerWorkstationBehaviorAgent
	WorkerWorkstationBehaviorScript    = workercompatibility.WorkerWorkstationBehaviorScript
	WorkerWorkstationBehaviorPoller    = workercompatibility.WorkerWorkstationBehaviorPoller
)

func compatibilityWorkstation(value FactoryWorkstationConfig) workercompatibility.Workstation {
	return workercompatibility.Workstation{Name: value.Name, Type: value.Type, Kind: workertaxonomy.WorkstationKind(value.Kind), WorkerTypeName: value.WorkerTypeName}
}

func compatibilityWorkstations(values []FactoryWorkstationConfig) []workercompatibility.Workstation {
	if len(values) == 0 {
		return nil
	}
	out := make([]workercompatibility.Workstation, len(values))
	for i, value := range values {
		out[i] = compatibilityWorkstation(value)
	}
	return out
}

func ExemptFromWorkerWorkstationCompatibility(workstation FactoryWorkstationConfig) bool {
	return workercompatibility.ExemptFromWorkerWorkstationCompatibility(compatibilityWorkstation(workstation))
}
func EffectiveWorkstationTypeForCompatibility(workstation FactoryWorkstationConfig) string {
	return workercompatibility.EffectiveWorkstationTypeForCompatibility(compatibilityWorkstation(workstation))
}
func WorkerBehaviorClass(workerType string) (WorkerWorkstationBehaviorClass, bool) {
	return workercompatibility.WorkerBehaviorClass(workerType)
}
func ExpectedWorkerBehaviorClassForWorkstation(workstation FactoryWorkstationConfig, workerType string) (WorkerWorkstationBehaviorClass, bool) {
	return workercompatibility.ExpectedWorkerBehaviorClassForWorkstation(compatibilityWorkstation(workstation), workerType)
}
func WorkerMatchesWorkstationBehavior(workerType string, workstation FactoryWorkstationConfig) bool {
	return workercompatibility.WorkerMatchesWorkstationBehavior(workerType, compatibilityWorkstation(workstation))
}
func PublicWorkerTypeForFactoryUsage(worker WorkerConfig, workstations []FactoryWorkstationConfig) string {
	return workercompatibility.PublicWorkerTypeForFactoryUsage(worker, compatibilityWorkstations(workstations))
}
func EffectiveWorkstationBehaviorClass(workstationType string, kind WorkstationKind, hasWorker bool) string {
	return workercompatibility.EffectiveWorkstationBehaviorClass(workstationType, workertaxonomy.WorkstationKind(kind), hasWorker)
}
func IsLegacyGrandfatheredWorkerWorkstationPair(workerType, workstationType string, kind WorkstationKind) bool {
	return workercompatibility.IsLegacyGrandfatheredWorkerWorkstationPair(workerType, workstationType, workertaxonomy.WorkstationKind(kind))
}
func RequiresWorkerWorkstationBehaviorCompatibility(workstationType string, kind WorkstationKind, workerTypeName string) bool {
	return workercompatibility.RequiresWorkerWorkstationBehaviorCompatibility(workstationType, workertaxonomy.WorkstationKind(kind), workerTypeName)
}
func CompatibleWorkerWorkstationBehavior(workerType, workstationType string, kind WorkstationKind) bool {
	return workercompatibility.CompatibleWorkerWorkstationBehavior(workerType, workstationType, workertaxonomy.WorkstationKind(kind))
}
func RuntimeBehaviorClassLabel(behaviorClass string) string {
	return workercompatibility.RuntimeBehaviorClassLabel(behaviorClass)
}
func WorkerWorkstationBehaviorMismatchMessage(workstationName, workstationType string, kind WorkstationKind, workerName, workerType string) string {
	return workercompatibility.WorkerWorkstationBehaviorMismatchMessage(workstationName, workstationType, workertaxonomy.WorkstationKind(kind), workerName, workerType)
}
