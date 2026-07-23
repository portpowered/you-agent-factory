import type { CanonicalFactoryDefinition } from "../../../../api/current-factory-definition";
import type { DashboardWorkstationNode } from "../../../../api/dashboard/types";
import type { EditableWorkstationInputDraft } from "../workstation-editable-values";
import { normalizeEditableInputGuards } from "../workstation-guards";

type CanonicalWorkstation = NonNullable<
  CanonicalFactoryDefinition["workstations"]
>[number];
type CanonicalWorker = NonNullable<
  CanonicalFactoryDefinition["workers"]
>[number];
type CanonicalWorkstationInput = NonNullable<
  CanonicalWorkstation["inputs"]
>[number];
type CanonicalWorkstationGuard = NonNullable<
  CanonicalWorkstation["guards"]
>[number];

export function resolveCanonicalWorkstation(
  factory: CanonicalFactoryDefinition,
  selectedNode: DashboardWorkstationNode,
): { workstation: CanonicalWorkstation; workstationIndex: number } | null {
  const workstations = factory.workstations ?? [];
  const workstationIndex = workstations.findIndex(
    (workstation) =>
      workstation.id === selectedNode.transition_id ||
      workstation.name === selectedNode.transition_id,
  );
  if (workstationIndex >= 0) {
    return {
      workstation: workstations[workstationIndex],
      workstationIndex,
    };
  }

  const workstationNameIndex = workstations.findIndex(
    (workstation) => workstation.name === selectedNode.workstation_name,
  );
  if (workstationNameIndex >= 0) {
    return {
      workstation: workstations[workstationNameIndex],
      workstationIndex: workstationNameIndex,
    };
  }

  return null;
}

export function resolveWorkerModelProvider(
  factory: CanonicalFactoryDefinition,
  workerName: string,
): string | null {
  const worker = (factory.workers ?? []).find(
    (entry) => entry.name === workerName,
  );
  return worker?.modelProvider ?? null;
}

export function resolveWorkerOptions(
  factory: CanonicalFactoryDefinition,
): string[] {
  return (factory.workers ?? [])
    .map((worker) => worker.name)
    .filter((name) => name.length > 0);
}

export function resolveSharedWorkerWorkstationNames(
  factory: CanonicalFactoryDefinition,
  selectedWorkstation: CanonicalWorkstation,
  selectedWorkstationIndex: number,
): string[] {
  const selectedWorkerName = selectedWorkstation.worker;
  if (!selectedWorkerName) {
    return [];
  }

  return (factory.workstations ?? [])
    .filter(
      (workstation, index) =>
        index !== selectedWorkstationIndex &&
        workstation.worker === selectedWorkerName,
    )
    .map((workstation) => workstation.name)
    .filter((name) => name.length > 0);
}

export function resolveSharedWorkerWorkstationNamesByWorkerName(
  factory: CanonicalFactoryDefinition,
  selectedWorkstation: CanonicalWorkstation,
): Record<string, string[]> {
  const otherWorkstationNamesByWorkerName = new Map<string, string[]>();

  for (const workstation of factory.workstations ?? []) {
    if (
      workstation.name === selectedWorkstation.name &&
      workstation.id === selectedWorkstation.id
    ) {
      continue;
    }

    if (!workstation.worker || workstation.name.length === 0) {
      continue;
    }

    const sharedWorkstations =
      otherWorkstationNamesByWorkerName.get(workstation.worker) ?? [];
    sharedWorkstations.push(workstation.name);
    otherWorkstationNamesByWorkerName.set(
      workstation.worker,
      sharedWorkstations,
    );
  }

  return Object.fromEntries(otherWorkstationNamesByWorkerName);
}

export function resolveWorkerTypeByName(factory: CanonicalFactoryDefinition) {
  return Object.fromEntries(
    (factory.workers ?? []).map((worker) => [worker.name, worker.type]),
  ) as Record<string, CanonicalWorker["type"] | undefined>;
}

export function resolveEditableWorkstationGuards(
  workstation: CanonicalWorkstation,
): CanonicalWorkstationGuard[] {
  return [...(workstation.guards ?? [])];
}

export function resolveEditableWorkstationInputs(
  workstation: CanonicalWorkstation,
): EditableWorkstationInputDraft[] {
  return (workstation.inputs ?? []).map((input) => ({
    guards: normalizeEditableInputGuards([...(input.guards ?? [])]),
    state: input.state,
    workType: input.workType,
  }));
}

export function applyEditableWorkstationInputs(
  inputs: EditableWorkstationInputDraft[],
): CanonicalWorkstationInput[] {
  return inputs.map((input) => {
    const guards = normalizeEditableInputGuards(input.guards);
    return {
      state: input.state,
      workType: input.workType,
      ...(guards.length > 0 ? { guards } : {}),
    };
  });
}
