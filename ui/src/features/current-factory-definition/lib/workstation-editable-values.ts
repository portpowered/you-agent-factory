import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import type { DashboardWorkstationNode } from "../../../api/dashboard/types";
import type { components } from "../../../api/generated/openapi";
import {
  BUILT_IN_RUNNER_IDS,
  type RunnerID,
} from "../../current-selection/workstation-selection/public";
import {
  type ResolvedRunnerSelection,
  type RunnerSelectionSource,
  resolveRunnerSelection,
} from "./runner-selection";
import {
  DEFAULT_WORKSTATION_BEHAVIOR,
  type EditableWorkstationBehavior,
  resolveEditableWorkstationBehavior,
  resolveEditableWorkstationBehaviorOptions,
} from "./workstation-behavior";
import {
  normalizeEditableInputGuards,
  resolveFactoryWorkstationNameOptions,
} from "./workstation-guards";
import {
  type EditableWorkstationType,
  resolveEditableWorkstationType,
} from "./workstation-type";
import { workstationRequiresWorkerAssignment } from "./workstation-worker-assignment";

type CanonicalWorkstation = NonNullable<
  CanonicalFactoryDefinition["workstations"]
>[number];
type CanonicalWorker = NonNullable<
  CanonicalFactoryDefinition["workers"]
>[number];
type CanonicalWorkstationCron = NonNullable<CanonicalWorkstation["cron"]>;
type CanonicalWorkstationGuard = NonNullable<
  CanonicalWorkstation["guards"]
>[number];
type CanonicalWorkstationInput = NonNullable<
  CanonicalWorkstation["inputs"]
>[number];

export type EditableWorkstationCronDraft = Pick<
  components["schemas"]["WorkstationCron"],
  "schedule" | "triggerAtStart"
> & {
  /** Empty when omitted from the factory definition. */
  expiryWindow: string;
  /** Empty when omitted from the factory definition. */
  jitter: string;
};

export interface EditableWorkstationInputDraft {
  guards: CanonicalWorkstationGuard[];
  state: string;
  workType: string;
}

export interface EditableWorkstationValues {
  behavior: EditableWorkstationBehavior;
  behaviorOptions: EditableWorkstationBehavior[];
  cron: EditableWorkstationCronDraft | null;
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
  guards: CanonicalWorkstationGuard[];
  inputs: EditableWorkstationInputDraft[];
  workstationName: string;
  workstationOptions: string[];
  workstationType: EditableWorkstationType;
}

export interface EditableWorkstationDraft {
  behavior: EditableWorkstationBehavior;
  cron: EditableWorkstationCronDraft | null;
  guards: CanonicalWorkstationGuard[];
  inputs: EditableWorkstationInputDraft[];
  prompt: string;
  runnerName: RunnerID | null;
  workerName: string;
}

export function resolveEditableWorkstationValues(
  factory: CanonicalFactoryDefinition,
  selectedNode: DashboardWorkstationNode,
): EditableWorkstationValues | null {
  const workstationResolution = resolveCanonicalWorkstation(
    factory,
    selectedNode,
  );
  if (!workstationResolution) {
    return null;
  }

  const { workstation } = workstationResolution;
  const behavior = resolveEditableWorkstationBehavior(workstation);
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
    behavior,
    behaviorOptions: resolveEditableWorkstationBehaviorOptions(behavior),
    cron:
      behavior === "CRON" ? resolveEditableWorkstationCron(workstation) : null,
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
    guards: resolveEditableWorkstationGuards(workstation),
    inputs: resolveEditableWorkstationInputs(workstation),
    workstationName: workstation.name,
    workstationOptions: resolveFactoryWorkstationNameOptions(factory),
    workstationType: resolveEditableWorkstationType(workstation),
  };
}

export function editableWorkstationDraftFromValues(
  values: EditableWorkstationValues,
): EditableWorkstationDraft {
  return {
    behavior: values.behavior,
    cron: values.cron ? { ...values.cron } : null,
    guards: values.guards,
    inputs: values.inputs.map((input) => ({
      guards: normalizeEditableInputGuards([...input.guards]),
      state: input.state,
      workType: input.workType,
    })),
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
  const workstationResolution = resolveCanonicalWorkstation(
    factory,
    selectedNode,
  );
  if (!workstationResolution || !factory.workstations) {
    return null;
  }

  const { workstation, workstationIndex } = workstationResolution;

  if (!workstationRequiresWorkerAssignment(workstation)) {
    return {
      ...factory,
      workers: factory.workers,
      workstations: factory.workstations.map((entry, index) =>
        index === workstationIndex ? workstation : entry,
      ),
    };
  }

  if (!factory.workers) {
    return null;
  }

  if (!factory.workers.some((worker) => worker.name === draft.workerName)) {
    return null;
  }

  const {
    behavior: existingBehavior,
    cron: _existingCron,
    guards: _existingGuards,
    inputs: _existingInputs,
    runner: _existingRunner,
    ...workstationWithoutCronRunner
  } = workstation;
  const nextWorkstation = {
    ...workstationWithoutCronRunner,
    body: draft.prompt,
    inputs: applyEditableWorkstationInputs(draft.inputs),
    worker: draft.workerName,
    ...(draft.guards.length > 0 ? { guards: draft.guards } : {}),
    ...(draft.runnerName ? { runner: draft.runnerName } : {}),
    ...(draft.behavior === DEFAULT_WORKSTATION_BEHAVIOR &&
    existingBehavior === undefined
      ? {}
      : { behavior: draft.behavior }),
    ...(draft.behavior === "CRON" && draft.cron
      ? { cron: buildCanonicalWorkstationCronFromDraft(draft.cron) }
      : {}),
  };

  return {
    ...factory,
    workers: factory.workers,
    workstations: factory.workstations.map((entry, index) =>
      index === workstationIndex ? nextWorkstation : entry,
    ),
  };
}

export function createEmptyEditableWorkstationCronDraft(): EditableWorkstationCronDraft {
  return {
    schedule: "",
    triggerAtStart: false,
    jitter: "",
    expiryWindow: "",
  };
}

export function resolveEditableWorkstationCron(
  workstation: Pick<CanonicalWorkstation, "cron">,
): EditableWorkstationCronDraft {
  const cron = workstation.cron;
  return {
    schedule: cron?.schedule ?? "",
    triggerAtStart: cron?.triggerAtStart ?? false,
    jitter: cron?.jitter ?? "",
    expiryWindow: cron?.expiryWindow ?? "",
  };
}

export function buildCanonicalWorkstationCronFromDraft(
  draft: EditableWorkstationCronDraft,
): CanonicalWorkstationCron {
  const cron: CanonicalWorkstationCron = {
    schedule: draft.schedule,
    triggerAtStart: draft.triggerAtStart,
  };
  const jitter = draft.jitter.trim();
  if (jitter.length > 0) {
    cron.jitter = jitter;
  }
  const expiryWindow = draft.expiryWindow.trim();
  if (expiryWindow.length > 0) {
    cron.expiryWindow = expiryWindow;
  }
  return cron;
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
  const worker = (factory.workers ?? []).find(
    (entry) => entry.name === workerName,
  );
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
    otherWorkstationNamesByWorkerName.set(
      workstation.worker,
      sharedWorkstations,
    );
  }

  return Object.fromEntries(otherWorkstationNamesByWorkerName);
}

function resolveWorkerTypeByName(factory: CanonicalFactoryDefinition) {
  return Object.fromEntries(
    (factory.workers ?? []).map((worker) => [worker.name, worker.type]),
  ) as Record<string, CanonicalWorker["type"] | undefined>;
}

function resolveEditableWorkstationGuards(
  workstation: CanonicalWorkstation,
): CanonicalWorkstationGuard[] {
  return [...(workstation.guards ?? [])];
}

function resolveEditableWorkstationInputs(
  workstation: CanonicalWorkstation,
): EditableWorkstationInputDraft[] {
  return (workstation.inputs ?? []).map((input) => ({
    guards: normalizeEditableInputGuards([...(input.guards ?? [])]),
    state: input.state,
    workType: input.workType,
  }));
}

function applyEditableWorkstationInputs(
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
