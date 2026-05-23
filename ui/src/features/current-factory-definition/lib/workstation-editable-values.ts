import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import type { DashboardWorkstationNode } from "../../../api/dashboard/types";
import {
  BUILT_IN_RUNNER_IDS,
  type RunnerID,
} from "../../current-selection/editing/runner-metadata";
import {
  DEFAULT_WORKSTATION_BEHAVIOR,
  resolveEditableWorkstationBehavior,
  resolveEditableWorkstationBehaviorOptions,
  type EditableWorkstationBehavior,
} from "./workstation-behavior";

type CanonicalWorkstation = NonNullable<
  CanonicalFactoryDefinition["workstations"]
>[number];
type CanonicalWorker = NonNullable<CanonicalFactoryDefinition["workers"]>[number];

export interface EditableWorkstationValues {
  behavior: EditableWorkstationBehavior;
  behaviorOptions: EditableWorkstationBehavior[];
  effectiveRunnerName: RunnerID;
  factoryRunnerName: RunnerID | null;
  prompt: string | null;
  runnerName: RunnerID | null;
  runnerOptions: RunnerID[];
  workerTypeByName: Record<string, CanonicalWorker["type"] | undefined>;
  workerName: string;
  workerOptions: string[];
  workstationName: string;
}

export interface EditableWorkstationDraft {
  behavior: EditableWorkstationBehavior;
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
    behavior: resolveEditableWorkstationBehavior(workstation),
    behaviorOptions: resolveEditableWorkstationBehaviorOptions(
      resolveEditableWorkstationBehavior(workstation),
    ),
    effectiveRunnerName: resolveEffectiveRunnerName(factory, workstation),
    factoryRunnerName: factory.runner ?? null,
    prompt: workstation.body ?? null,
    runnerName: workstation.runner ?? null,
    runnerOptions: BUILT_IN_RUNNER_IDS,
    workerTypeByName: resolveWorkerTypeByName(factory),
    workerName: workstation.worker,
    workerOptions: resolveWorkerOptions(factory),
    workstationName: workstation.name,
  };
}

export function editableWorkstationDraftFromValues(
  values: EditableWorkstationValues,
): EditableWorkstationDraft {
  return {
    behavior: values.behavior,
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

  const {
    behavior: existingBehavior,
    runner: _existingRunner,
    ...workstationWithoutRunner
  } =
    workstationResolution.workstation;
  const nextWorkstation = {
    ...workstationWithoutRunner,
    body: draft.prompt,
    worker: draft.workerName,
    ...(draft.runnerName ? { runner: draft.runnerName } : {}),
    ...(draft.behavior === DEFAULT_WORKSTATION_BEHAVIOR &&
    existingBehavior === undefined
      ? {}
      : { behavior: draft.behavior }),
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

function resolveWorkerTypeByName(factory: CanonicalFactoryDefinition) {
  return Object.fromEntries(
    (factory.workers ?? []).map((worker) => [worker.name, worker.type]),
  ) as Record<string, CanonicalWorker["type"] | undefined>;
}
