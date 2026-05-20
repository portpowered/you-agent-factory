import type { CanonicalFactoryDefinition } from "../../api/current-factory-definition";
import type { DashboardWorkstationNode } from "../../api/dashboard/types";
import {
  BUILT_IN_RUNNER_IDS,
  type RunnerID,
} from "../current-selection/runner-metadata";

type CanonicalWorkstation = NonNullable<
  CanonicalFactoryDefinition["workstations"]
>[number];
export interface EditableWorkstationValues {
  effectiveRunnerName: RunnerID;
  factoryRunnerName: RunnerID | null;
  prompt: string | null;
  runnerName: RunnerID | null;
  runnerOptions: RunnerID[];
  workerName: string;
  workerOptions: string[];
  workstationName: string;
}

export interface EditableWorkstationDraft {
  prompt: string;
  runnerName: RunnerID | null;
  workerName: string;
}

export function resolveEditableWorkstationValues(
  factory: CanonicalFactoryDefinition,
  selectedNode: DashboardWorkstationNode,
): EditableWorkstationValues | null {
  const workstationResolution = resolveCanonicalWorkstation(factory, selectedNode);
  if (!workstationResolution) {
    return null;
  }

  const { workstation } = workstationResolution;

  return {
    effectiveRunnerName: resolveEffectiveRunnerName(factory, workstation),
    factoryRunnerName: factory.runner ?? null,
    prompt: workstation.body ?? null,
    runnerName: workstation.runner ?? null,
    runnerOptions: BUILT_IN_RUNNER_IDS,
    workerName: workstation.worker,
    workerOptions: resolveWorkerOptions(factory),
    workstationName: workstation.name,
  };
}

export function editableWorkstationDraftFromValues(
  values: EditableWorkstationValues,
): EditableWorkstationDraft {
  return {
    prompt: values.prompt ?? "",
    runnerName: values.runnerName,
    workerName: values.workerName,
  };
}

export function applyEditableWorkstationDraft(
  factory: CanonicalFactoryDefinition,
  selectedNode: DashboardWorkstationNode,
  draft: EditableWorkstationDraft,
): CanonicalFactoryDefinition | null {
  const workstationResolution = resolveCanonicalWorkstation(factory, selectedNode);
  if (!workstationResolution || !factory.workers || !factory.workstations) {
    return null;
  }

  if (!factory.workers.some((worker) => worker.name === draft.workerName)) {
    return null;
  }

  const { runner: _existingRunner, ...workstationWithoutRunner } =
    workstationResolution.workstation;
  const nextWorkstation = draft.runnerName
    ? {
        ...workstationWithoutRunner,
        body: draft.prompt,
        runner: draft.runnerName,
        worker: draft.workerName,
      }
    : {
        ...workstationWithoutRunner,
        body: draft.prompt,
        worker: draft.workerName,
      };

  return {
    ...factory,
    workers: factory.workers,
    workstations: factory.workstations.map((workstation, index) =>
      index === workstationResolution.workstationIndex ? nextWorkstation : workstation,
    ),
  };
}

function resolveCanonicalWorkstation(
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

function resolveEffectiveRunnerName(
  factory: CanonicalFactoryDefinition,
  workstation: CanonicalWorkstation,
): RunnerID {
  return (
    workstation.runner ??
    factory.runner ??
    "codex"
  );
}

function resolveWorkerOptions(factory: CanonicalFactoryDefinition): string[] {
  return (factory.workers ?? [])
    .map((worker) => worker.name)
    .filter((name) => name.length > 0);
}
