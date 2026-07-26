import { validateEditableWorkstationCronDraft } from "../../../current-factory-definition/lib/editable-workstation-cron-validation";
import type { EditableWorkstationType } from "../../../current-factory-definition/lib/workstation/workstation-type";
import {
  DEFAULT_WORKSTATION_BEHAVIOR,
  type EditableWorkstationBehavior,
  resolveFactoryGraphAddWorkstationBehaviorOptions,
  workerSupportsPollerBehavior,
} from "../../../current-factory-definition/lib/workstation-behavior";
import {
  buildCanonicalWorkstationCronFromDraft,
  createEmptyEditableWorkstationCronDraft,
  type EditableWorkstationCronDraft,
} from "../../../current-factory-definition/lib/workstation-editable-values";
import {
  DEFAULT_FACTORY_GRAPH_ADD_WORKSTATION_TYPE,
  DEFAULT_WORKER_TYPE,
  type FactoryGraphAddWorkerType,
  isPollerRunWorkstationType,
} from "../../../current-factory-definition/lib/worker-workstation-taxonomy";
import type {
  FactoryGraphAddModelOperationDraft,
  FactoryGraphAddModelOperationValidationErrors,
} from "../factory-graph-add-model-operation-draft";
import {
  applyFactoryGraphAddWorkerDraft,
  validateFactoryGraphAddWorkerDraft,
} from "../factory-graph-editor-additions.worker";
import {
  applyFactoryGraphDocAddEntityDraft,
  createFactoryGraphDocAddEntityDraft,
  validateFactoryGraphDocAddEntityDraft,
} from "../factory-graph-editor-doc-additions";

export type { FactoryGraphAddWorkerType };

import { workstationRequiresWorkerAssignment } from "../../../current-factory-definition/lib/workstation-worker-assignment";

export { buildFactoryGraphAddEntityMenuActions } from "../factory-graph-editor-add-menu";

import type {
  CanonicalFactoryDefinition,
  FactoryGraphDraft,
  FactoryWorkState,
} from "../draft/factory-graph-draft-types";

export type FactoryGraphAddEntityKind =
  | "doc"
  | "resource"
  | "worker"
  | "work-type"
  | "work-state"
  | "workstation";

export type FactoryGraphAddEntityDraft =
  | {
      fileName: string;
      inlineContent: string;
      kind: "doc";
    }
  | {
      capacity: string;
      kind: "resource";
      name: string;
    }
  | {
      argsText: string;
      command: string;
      kind: "worker";
      model: string;
      modelProvider: string;
      name: string;
      operations: FactoryGraphAddModelOperationDraft[];
      provider: string;
      workerType: FactoryGraphAddWorkerType;
    }
  | {
      initialStateName: string;
      kind: "work-type";
      name: string;
    }
  | {
      kind: "work-state";
      name: string;
      stateType: FactoryWorkState["type"];
      workTypeName: string;
    }
  | {
      behavior: EditableWorkstationBehavior;
      body: string;
      cron: EditableWorkstationCronDraft | null;
      kind: "workstation";
      name: string;
      workerName: string;
      workstationType: EditableWorkstationType;
    };

export type FactoryGraphAddEntityFieldErrors = Partial<
  Record<
    | "args"
    | "capacity"
    | "command"
    | "fileName"
    | "inlineContent"
    | "initialStateName"
    | "model"
    | "modelProvider"
    | "name"
    | "provider"
    | "stateType"
    | "behavior"
    | "cronExpiryWindow"
    | "cronJitter"
    | "cronSchedule"
    | "workTypeName"
    | "workerName",
    string
  >
> & {
  modelOperations?: FactoryGraphAddModelOperationValidationErrors;
};

const FACTORY_GRAPH_ADD_CRON_VALIDATION_MESSAGES = {
  cronExpiryWindowInvalid: (value: string) =>
    `Cron expiry window "${value}" must be a positive Go duration.`,
  cronJitterInvalid: (value: string) =>
    `Cron jitter "${value}" must be a non-negative Go duration.`,
  cronScheduleInvalid: (schedule: string, detail: string) =>
    `Cron schedule "${schedule}" is invalid: ${detail}`,
  cronScheduleRequired: "Enter a cron schedule before adding this workstation.",
} as const;

const DEFAULT_RESOURCE_CAPACITY = "1";
const DEFAULT_WORK_STATE_TYPE: FactoryWorkState["type"] = "PROCESSING";

export function createFactoryGraphAddEntityDraft(
  kind: FactoryGraphAddEntityKind,
  factoryDefinition: CanonicalFactoryDefinition | null,
): FactoryGraphAddEntityDraft {
  if (kind === "doc") {
    return createFactoryGraphDocAddEntityDraft(factoryDefinition);
  }

  if (kind === "resource") {
    return {
      capacity: DEFAULT_RESOURCE_CAPACITY,
      kind,
      name: "",
    };
  }

  if (kind === "worker") {
    return {
      argsText: "",
      command: "",
      kind,
      model: "",
      modelProvider: "",
      name: "",
      operations: [],
      provider: "",
      workerType: DEFAULT_WORKER_TYPE as FactoryGraphAddWorkerType,
    };
  }

  if (kind === "work-type") {
    return {
      initialStateName: "",
      kind,
      name: "",
    };
  }

  if (kind === "work-state") {
    return {
      kind,
      name: "",
      stateType: DEFAULT_WORK_STATE_TYPE,
      workTypeName: factoryDefinition?.workTypes?.[0]?.name ?? "",
    };
  }

  return {
    behavior: DEFAULT_WORKSTATION_BEHAVIOR,
    body: "",
    cron: null,
    kind,
    name: "",
    workerName: factoryDefinition?.workers?.[0]?.name ?? "",
    workstationType: DEFAULT_FACTORY_GRAPH_ADD_WORKSTATION_TYPE,
  };
}

export function validateFactoryGraphAddEntityDraft(
  draft: FactoryGraphAddEntityDraft,
  factoryDefinition: CanonicalFactoryDefinition | null,
  _locale?: string | null,
): FactoryGraphAddEntityFieldErrors {
  const errors: FactoryGraphAddEntityFieldErrors = {};

  if (draft.kind === "doc") {
    return validateFactoryGraphDocAddEntityDraft(draft, factoryDefinition);
  }

  const name = draft.name.trim();

  if (name.length === 0) {
    errors.name = "Enter an identifier before adding this entity.";
  }

  if (
    name.length > 0 &&
    entityNameExists(draft.kind, name, factoryDefinition)
  ) {
    errors.name = `A ${draft.kind} named "${name}" already exists in the draft.`;
  }

  if (draft.kind === "resource") {
    const capacity = Number.parseInt(draft.capacity, 10);
    if (!Number.isInteger(capacity) || capacity < 1) {
      errors.capacity =
        "Resource capacity must be a whole number greater than zero.";
    }
  }

  if (draft.kind === "worker") {
    Object.assign(errors, validateFactoryGraphAddWorkerDraft(draft));
  }

  if (
    draft.kind === "work-type" &&
    draft.initialStateName.trim().length === 0
  ) {
    errors.initialStateName =
      "Enter the first ordered work state for this work type.";
  }

  if (draft.kind === "work-state") {
    if (draft.workTypeName.trim().length === 0) {
      errors.workTypeName = "Choose a work type before adding a work state.";
    } else if (!workTypeExists(draft.workTypeName, factoryDefinition)) {
      errors.workTypeName = `Work type "${draft.workTypeName}" is not available in the current draft.`;
    }

    if (
      name.length > 0 &&
      workStateExists(draft.workTypeName, name, factoryDefinition)
    ) {
      errors.name = `Work type "${draft.workTypeName}" already defines a state named "${name}".`;
    }
  }

  if (draft.kind === "workstation") {
    const requiresWorkerAssignment = workstationRequiresWorkerAssignment({
      type: draft.workstationType,
    });

    if (requiresWorkerAssignment) {
      if (draft.workerName.trim().length === 0) {
        errors.workerName =
          "Choose an assigned worker before adding this workstation.";
      } else if (!workerExists(draft.workerName, factoryDefinition)) {
        errors.workerName = `Worker "${draft.workerName}" is not available in the current draft.`;
      } else if (
        draft.behavior === "POLLER" &&
        !workerSupportsPollerBehavior(
          (factoryDefinition?.workers ?? []).find(
            (worker) => worker.name === draft.workerName,
          ),
        )
      ) {
        errors.behavior =
          "Poller workstations must use a script or hosted worker.";
      }
    }

    if (draft.behavior === "CRON") {
      Object.assign(
        errors,
        validateEditableWorkstationCronDraft(
          draft.cron,
          FACTORY_GRAPH_ADD_CRON_VALIDATION_MESSAGES,
        ),
      );
    }
  }

  return errors;
}

export function applyFactoryGraphAddEntityDraft(
  currentDraft: FactoryGraphDraft,
  entityDraft: FactoryGraphAddEntityDraft,
): FactoryGraphDraft {
  const nextDraft = structuredClone(currentDraft);

  if (entityDraft.kind === "doc") {
    return applyFactoryGraphDocAddEntityDraft(nextDraft, entityDraft);
  }

  if (entityDraft.kind === "resource") {
    nextDraft.additions.resources.push({
      capacity: Number.parseInt(entityDraft.capacity, 10),
      name: entityDraft.name.trim(),
    });
    return nextDraft;
  }

  if (entityDraft.kind === "worker") {
    return applyFactoryGraphAddWorkerDraft(nextDraft, entityDraft);
  }

  if (entityDraft.kind === "work-type") {
    nextDraft.additions.workTypes.push({
      name: entityDraft.name.trim(),
      states: [
        {
          name: entityDraft.initialStateName.trim(),
          type: "INITIAL",
        },
      ],
    });
    return nextDraft;
  }

  if (entityDraft.kind === "work-state") {
    nextDraft.additions.workStates.push({
      state: {
        name: entityDraft.name.trim(),
        type: entityDraft.stateType,
      },
      workTypeName: entityDraft.workTypeName.trim(),
    });
    return nextDraft;
  }

  const requiresWorkerAssignment = workstationRequiresWorkerAssignment({
    type: entityDraft.workstationType,
  });
  const trimmedBody = entityDraft.body.trim();
  const behavior = isPollerRunWorkstationType(entityDraft.workstationType)
    ? "POLLER"
    : entityDraft.behavior;

  nextDraft.additions.workstations.push({
    ...(behavior === DEFAULT_WORKSTATION_BEHAVIOR ? {} : { behavior }),
    ...(trimmedBody.length > 0 ? { body: trimmedBody } : {}),
    ...(entityDraft.behavior === "CRON" && entityDraft.cron
      ? { cron: buildCanonicalWorkstationCronFromDraft(entityDraft.cron) }
      : {}),
    inputs: [],
    name: entityDraft.name.trim(),
    outputs: [],
    type: entityDraft.workstationType,
    ...(requiresWorkerAssignment
      ? { worker: entityDraft.workerName.trim() }
      : { worker: "" }),
  });
  return nextDraft;
}

export function resolveFactoryGraphAddWorkstationDraftForTypeChange(
  draft: Extract<FactoryGraphAddEntityDraft, { kind: "workstation" }>,
  workstationType: EditableWorkstationType,
  options?: { defaultWorkerName?: string },
): Extract<FactoryGraphAddEntityDraft, { kind: "workstation" }> {
  if (workstationType === draft.workstationType) {
    return draft;
  }

  const nextRequiresWorker = workstationRequiresWorkerAssignment({
    type: workstationType,
  });
  let behavior = draft.behavior;
  if (isPollerRunWorkstationType(workstationType)) {
    behavior = "POLLER";
  } else if (draft.behavior === "POLLER") {
    behavior = DEFAULT_WORKSTATION_BEHAVIOR;
  }

  return {
    ...draft,
    behavior,
    body: nextRequiresWorker ? draft.body : "",
    cron: behavior === "CRON" ? draft.cron : null,
    workerName: nextRequiresWorker
      ? draft.workerName || options?.defaultWorkerName || ""
      : "",
    workstationType,
  };
}

export function resolveFactoryGraphAddWorkstationDraftForBehaviorChange(
  draft: Extract<FactoryGraphAddEntityDraft, { kind: "workstation" }>,
  behavior: EditableWorkstationBehavior,
): Extract<FactoryGraphAddEntityDraft, { kind: "workstation" }> {
  if (behavior === "CRON") {
    return {
      ...draft,
      behavior,
      cron: draft.cron ?? createEmptyEditableWorkstationCronDraft(),
    };
  }

  return {
    ...draft,
    behavior,
    cron: null,
  };
}

function entityNameExists(
  kind: FactoryGraphAddEntityDraft["kind"],
  name: string,
  factoryDefinition: CanonicalFactoryDefinition | null,
) {
  if (kind === "resource") {
    return (factoryDefinition?.resources ?? []).some(
      (resource) => resource.name === name,
    );
  }
  if (kind === "worker") {
    return (factoryDefinition?.workers ?? []).some(
      (worker) => worker.name === name,
    );
  }
  if (kind === "work-type") {
    return (factoryDefinition?.workTypes ?? []).some(
      (workType) => workType.name === name,
    );
  }
  if (kind === "workstation") {
    return (factoryDefinition?.workstations ?? []).some(
      (workstation) => workstation.name === name,
    );
  }

  return false;
}

function workTypeExists(
  workTypeName: string,
  factoryDefinition: CanonicalFactoryDefinition | null,
) {
  return (factoryDefinition?.workTypes ?? []).some(
    (workType) => workType.name === workTypeName,
  );
}

function workStateExists(
  workTypeName: string,
  stateName: string,
  factoryDefinition: CanonicalFactoryDefinition | null,
) {
  return (factoryDefinition?.workTypes ?? [])
    .find((workType) => workType.name === workTypeName)
    ?.states.some((state) => state.name === stateName);
}

function workerExists(
  workerName: string,
  factoryDefinition: CanonicalFactoryDefinition | null,
) {
  return (factoryDefinition?.workers ?? []).some(
    (worker) => worker.name === workerName,
  );
}

export function editableWorkstationBehaviorOptions() {
  return resolveFactoryGraphAddWorkstationBehaviorOptions();
}
