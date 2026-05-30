import type { CanonicalFactoryDefinition } from "../../../../api/current-factory-definition";
import type { FactoryWorker } from "../../../../api/events/types";

export function findWorkerInFactoryDefinition(
  factory: CanonicalFactoryDefinition,
  workerName: string,
): FactoryWorker | undefined {
  return (factory.workers ?? []).find((worker) => worker.name === workerName);
}

export function workstationNamesReferencingWorkerInFactoryDefinition(
  factory: CanonicalFactoryDefinition,
  workerName: string,
): string[] {
  return (factory.workstations ?? [])
    .filter((workstation) => workstation.worker === workerName)
    .map((workstation) => workstation.name)
    .filter((name) => name.length > 0);
}
