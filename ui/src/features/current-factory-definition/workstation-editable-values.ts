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
  model: string | null;
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

  return {
    model: worker.model ?? null,
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

  return {
    ...factory,
    workers: factory.workers.map((worker, index) =>
      index === resolution.workerIndex ? nextWorker : worker,
    ),
    workstations: factory.workstations.map((workstation, index) =>
      index === resolution.workstationIndex ? nextWorkstation : workstation,
    ),
  };
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
