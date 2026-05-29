import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import type { DashboardWorkstationNode } from "../../../api/dashboard/types";
import {
  BUILT_IN_RUNNER_IDS,
  type RunnerID,
} from "../../current-selection/workstation-selection/public";
import {
  resolveRunnerSelection,
  type ResolvedRunnerSelection,
  type RunnerSelectionSource,
} from "./runner-selection";
import {
  DEFAULT_WORKSTATION_BEHAVIOR,
  resolveEditableWorkstationBehavior,
  resolveEditableWorkstationBehaviorOptions,
  type EditableWorkstationBehavior,
} from "./workstation-behavior";
import {
  resolveEditableWorkstationType,
  type EditableWorkstationType,
} from "./workstation-type";

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
  resolvedRunnerSelection: ResolvedRunnerSelection;
  runnerName: RunnerID | null;
  runnerOptions: RunnerID[];
  runnerSelectionSource: RunnerSelectionSource;
  sharedWorkerWorkstationNamesByWorkerName: Record<string, string[]>;
  sharedWorkerWorkstationNames: string[];
  workerTypeByName: Record<string, CanonicalWorker["type"] | undefined>;
  workerName: string;
  workerOptions: string[];
  workerModelProvider: string | null;
  workstationName: string;
  workstationType: EditableWorkstationType;
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
  const workerModelProvider = resolveWorkerModelProvider(
    factory,
    workstation.worker,
  );
  const resolvedRunnerSelection = resolveRunnerSelection(
    workstation.runner,
    factory.runner,
    workerModelProvider,
  );

  return {
    behavior: resolveEditableWorkstationBehavior(workstation),
    behaviorOptions: resolveEditableWorkstationBehaviorOptions(
      resolveEditableWorkstationBehavior(workstation),
    ),
    effectiveRunnerName: resolvedRunnerSelection.runnerId,
    factoryRunnerName: factory.runner ?? null,
    prompt: workstation.body ?? null,
    resolvedRunnerSelection,
    runnerName: workstation.runner ?? null,
    runnerOptions: BUILT_IN_RUNNER_IDS,
    runnerSelectionSource: resolvedRunnerSelection.source,
    sharedWorkerWorkstationNamesByWorkerName:
      resolveSharedWorkerWorkstationNamesByWorkerName(factory, workstation),
    sharedWorkerWorkstationNames: resolveSharedWorkerWorkstationNames(
      factory,
      workstation,
      workstationResolution.workstationIndex,
    ),
    workerTypeByName: resolveWorkerTypeByName(factory),
    workerModelProvider,
    workerName: workstation.worker,
    workerOptions: resolveWorkerOptions(factory),
    workstationName: workstation.name,
    workstationType: resolveEditableWorkstationType(workstation),
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

function resolveWorkerModelProvider(
  factory: CanonicalFactoryDefinition,
  workerName: string,
): string | null {
  const worker = (factory.workers ?? []).find((entry) => entry.name === workerName);
  return worker?.modelProvider ?? null;
}

function resolveWorkerOptions(factory: CanonicalFactoryDefinition): string[] {
  return (factory.workers ?? [])
    .map((worker) => worker.name)
    .filter((name) => name.length > 0);
}

function resolveSharedWorkerWorkstationNames(
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

function resolveSharedWorkerWorkstationNamesByWorkerName(
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
    otherWorkstationNamesByWorkerName.set(workstation.worker, sharedWorkstations);
  }

  return Object.fromEntries(otherWorkstationNamesByWorkerName);
}

function resolveWorkerTypeByName(factory: CanonicalFactoryDefinition) {
  return Object.fromEntries(
    (factory.workers ?? []).map((worker) => [worker.name, worker.type]),
  ) as Record<string, CanonicalWorker["type"] | undefined>;
}
