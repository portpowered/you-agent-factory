import type { CanonicalFactoryDefinition } from "../../api/current-factory-definition";
import type { DashboardWorkstationNode } from "../../api/dashboard/types";

type CanonicalWorkstation = NonNullable<
  CanonicalFactoryDefinition["workstations"]
>[number];
type CanonicalWorker = NonNullable<
  CanonicalFactoryDefinition["workers"]
>[number];
type EditableWorkstationResolution = {
  worker: CanonicalWorker;
  workerIndex: number;
  workstation: CanonicalWorkstation;
  workstationIndex: number;
};

export interface EditableWorkstationValues {
  isModelEditable: boolean;
  model: string | null;
  modelEditBlockedReason: string | null;
  prompt: string | null;
  promptFile: string | null;
  workerName: string;
  workstationName: string;
}

export interface EditableWorkstationDraft {
  model: string;
  prompt: string;
  promptFile: string;
}

export function resolveEditableWorkstationValues(
  factory: CanonicalFactoryDefinition,
  selectedNode: DashboardWorkstationNode,
): EditableWorkstationValues | null {
  const resolution = resolveEditableWorkstation(factory, selectedNode);
  if (!resolution) {
    return null;
  }

  const { worker, workstation } = resolution;
  const sharedWorkerWorkstationNames = resolveSharedWorkerWorkstationNames(
    factory,
    worker.name,
  );
  const isModelEditable = sharedWorkerWorkstationNames.length === 1;

  return {
    isModelEditable,
    model: worker.model ?? null,
    modelEditBlockedReason: isModelEditable
      ? null
      : `Model edits are disabled here because worker "${worker.name}" is shared with ${formatSharedWorkstationList(sharedWorkerWorkstationNames)}.`,
    prompt: workstation.body ?? null,
    promptFile: workstation.promptFile ?? null,
    workerName: worker.name,
    workstationName: workstation.name,
  };
}

export function editableWorkstationDraftFromValues(
  values: EditableWorkstationValues,
): EditableWorkstationDraft {
  return {
    model: values.model ?? "",
    prompt: values.prompt ?? "",
    promptFile: values.promptFile ?? "",
  };
}

export function applyEditableWorkstationDraft(
  factory: CanonicalFactoryDefinition,
  selectedNode: DashboardWorkstationNode,
  draft: EditableWorkstationDraft,
): CanonicalFactoryDefinition | null {
  const resolution = resolveEditableWorkstation(factory, selectedNode);
  if (!resolution || !factory.workers || !factory.workstations) {
    return null;
  }

  const nextWorker = {
    ...resolution.worker,
    model: draft.model.trim(),
  };
  const nextWorkstation = {
    ...resolution.workstation,
    body: draft.prompt,
    promptFile: normalizePromptFileDraft(draft.promptFile),
  };
  const nextWorkers = buildUpdatedWorkers(
    factory,
    resolution.worker.name,
    resolution.workerIndex,
    nextWorker,
    draft,
  );
  if (!nextWorkers) {
    return null;
  }

  return {
    ...factory,
    workers: nextWorkers,
    workstations: factory.workstations.map((workstation, index) =>
      index === resolution.workstationIndex ? nextWorkstation : workstation,
    ),
  };
}

function buildUpdatedWorkers(
  factory: CanonicalFactoryDefinition,
  workerName: string,
  workerIndex: number,
  nextWorker: CanonicalWorker,
  draft: EditableWorkstationDraft,
) {
  const sharedWorkerWorkstationNames = resolveSharedWorkerWorkstationNames(
    factory,
    workerName,
  );
  const currentModel =
    factory.workers?.[workerIndex]?.model?.trim() ?? "";
  const nextModel = draft.model.trim();

  if (sharedWorkerWorkstationNames.length > 1) {
    if (currentModel !== nextModel) {
      return null;
    }

    return factory.workers ?? null;
  }

  return factory.workers?.map((worker, index) =>
    index === workerIndex ? nextWorker : worker,
  );
}

function normalizePromptFileDraft(promptFile: string): string | undefined {
  const trimmedPromptFile = promptFile.trim();
  return trimmedPromptFile.length > 0 ? trimmedPromptFile : undefined;
}

function resolveEditableWorkstation(
  factory: CanonicalFactoryDefinition,
  selectedNode: DashboardWorkstationNode,
): EditableWorkstationResolution | null {
  const workstationResolution = resolveCanonicalWorkstation(
    factory,
    selectedNode,
  );
  if (!workstationResolution) {
    return null;
  }

  const workerResolution = resolveCanonicalWorker(
    factory,
    workstationResolution.workstation.worker,
  );
  if (!workerResolution) {
    return null;
  }

  return {
    worker: workerResolution.worker,
    workerIndex: workerResolution.workerIndex,
    workstation: workstationResolution.workstation,
    workstationIndex: workstationResolution.workstationIndex,
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

function resolveCanonicalWorker(
  factory: CanonicalFactoryDefinition,
  workerName: string,
): { worker: CanonicalWorker; workerIndex: number } | null {
  const workerIndex =
    factory.workers?.findIndex((worker) => worker.name === workerName) ?? -1;
  if (workerIndex < 0 || !factory.workers) {
    return null;
  }

  return {
    worker: factory.workers[workerIndex],
    workerIndex,
  };
}

function resolveSharedWorkerWorkstationNames(
  factory: CanonicalFactoryDefinition,
  workerName: string,
): string[] {
  return (factory.workstations ?? [])
    .filter((workstation) => workstation.worker === workerName)
    .map((workstation) => workstation.name);
}

function formatSharedWorkstationList(workstationNames: string[]): string {
  if (workstationNames.length === 0) {
    return "no workstations";
  }

  if (workstationNames.length === 1) {
    return `"${workstationNames[0]}"`;
  }

  if (workstationNames.length === 2) {
    return `"${workstationNames[0]}" and "${workstationNames[1]}"`;
  }

  return `${workstationNames
    .slice(0, -1)
    .map((name) => `"${name}"`)
    .join(", ")}, and "${workstationNames.at(-1)}"`;
}
