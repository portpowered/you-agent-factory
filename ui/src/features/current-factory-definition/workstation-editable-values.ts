import type { DashboardWorkstationNode } from "../../api/dashboard/types";
import type { CanonicalFactoryDefinition } from "../../api/current-factory-definition";

type CanonicalWorkstation = NonNullable<CanonicalFactoryDefinition["workstations"]>[number];
type CanonicalWorker = NonNullable<CanonicalFactoryDefinition["workers"]>[number];

export interface EditableWorkstationValues {
  model: string | null;
  prompt: string | null;
  promptFile: string | null;
  workerName: string;
  workstationName: string;
}

export function resolveEditableWorkstationValues(
  factory: CanonicalFactoryDefinition,
  selectedNode: DashboardWorkstationNode,
): EditableWorkstationValues | null {
  const workstation = resolveCanonicalWorkstation(factory, selectedNode);
  if (!workstation) {
    return null;
  }

  const worker = resolveCanonicalWorker(factory, workstation.worker);
  if (!worker) {
    return null;
  }

  return {
    model: worker.model ?? null,
    prompt: workstation.body ?? null,
    promptFile: workstation.promptFile ?? null,
    workerName: worker.name,
    workstationName: workstation.name,
  };
}

function resolveCanonicalWorkstation(
  factory: CanonicalFactoryDefinition,
  selectedNode: DashboardWorkstationNode,
): CanonicalWorkstation | null {
  const workstations = factory.workstations ?? [];
  return (
    workstations.find(
      (workstation) =>
        workstation.id === selectedNode.transition_id ||
        workstation.name === selectedNode.transition_id,
    ) ??
    workstations.find(
      (workstation) => workstation.name === selectedNode.workstation_name,
    ) ??
    null
  );
}

function resolveCanonicalWorker(
  factory: CanonicalFactoryDefinition,
  workerName: string,
): CanonicalWorker | null {
  return (
    factory.workers?.find((worker) => worker.name === workerName) ?? null
  );
}
