import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import type { DashboardWorkstationNode } from "../../../api/dashboard/types";
import type { components } from "../../../api/generated/openapi";
import { BUILT_IN_RUNNER_IDS } from "../../current-selection/workstation-selection/editing/runner-metadata";
import type { ApiRunnerID } from "../../current-selection/workstation-selection/messages/runner-openapi-enums";
import {
  type ResolvedRunnerSelection,
  type RunnerSelectionSource,
  resolveRunnerSelection,
} from "./runner-selection";
import {
  isPollerRunWorkstationType,
  isScriptRunWorkstationType,
  isWorkerCompatibleWithWorkstationType,
  preferredInferenceRunWorkstationType,
} from "./worker-workstation-taxonomy";
import {
  applyEditableWorkstationInputs,
  resolveCanonicalWorkstation,
  resolveEditableWorkstationGuards,
  resolveEditableWorkstationInputs,
  resolveSharedWorkerWorkstationNames,
  resolveSharedWorkerWorkstationNamesByWorkerName,
  resolveWorkerModelProvider,
  resolveWorkerOptions,
  resolveWorkerTypeByName,
} from "./workstation/workstation-editable-resolution";
import type { EditableModelInvokeBindingDraft } from "./workstation/workstation-model-invoke";
import {
  buildCanonicalModelInvokeBindingsFromDraft,
  isModelInvokeWorkstationType,
  resolveCompatibleModelWorkerNames,
  resolveEditableModelInvokeBindings,
  resolveModelOperationByName,
  resolveModelOperationsByWorkerName,
  resolveModelWorkerOperations,
  syncEditableModelInvokeBindingsForOperation,
} from "./workstation/workstation-model-invoke";
import {
  type EditableWorkstationType,
  resolveEditableWorkstationType,
  resolveEditableWorkstationTypeOptions,
} from "./workstation/workstation-type";
import {
  DEFAULT_WORKSTATION_BEHAVIOR,
  type EditableWorkstationBehavior,
  resolveEditableWorkstationBehavior,
  resolveEditableWorkstationBehaviorOptions,
  workstationUsesRunnerEditing,
} from "./workstation-behavior";
import {
  normalizeEditableInputGuards,
  resolveFactoryWorkstationNameOptions,
  rewriteWorkstationVisitCountReferences,
} from "./workstation-guards";
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
type CanonicalInputGuard = NonNullable<
  CanonicalWorkstationInput["guards"]
>[number];

export type EditableWorkstationCronDraft = {
  /** The editor currently authors cron expressions; empty until provided. */
  schedule: string;
  triggerAtStart: components["schemas"]["WorkstationCron"]["triggerAtStart"];
  /** Empty when omitted from the factory definition. */
  expiryWindow: string;
  /** Empty when omitted from the factory definition. */
  jitter: string;
};

export interface EditableWorkstationInputDraft {
  guards: CanonicalInputGuard[];
  state: string;
  workType: string;
}

export interface EditableWorkstationValues {
  behavior: EditableWorkstationBehavior;
  behaviorOptions: EditableWorkstationBehavior[];
  cron: EditableWorkstationCronDraft | null;
  effectiveRunnerName: ApiRunnerID;
  factoryRunnerName: ApiRunnerID | null;
  modelInvokeWorkerOptions: string[];
  modelOperationsByWorkerName: ReturnType<
    typeof resolveModelOperationsByWorkerName
  >;
  operation: string;
  operationBindings: EditableModelInvokeBindingDraft[];
  prompt: string | null;
  resolvedRunnerSelection: ResolvedRunnerSelection;
  runnerName: ApiRunnerID | null;
  runnerOptions: ApiRunnerID[];
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
  workstationTypeOptions: readonly EditableWorkstationType[];
}

export interface EditableWorkstationDraft {
  behavior: EditableWorkstationBehavior;
  cron: EditableWorkstationCronDraft | null;
  guards: CanonicalWorkstationGuard[];
  inputs: EditableWorkstationInputDraft[];
  name: string;
  operation: string;
  operationBindings: EditableModelInvokeBindingDraft[];
  prompt: string;
  runnerName: ApiRunnerID | null;
  workerName: string;
  workstationType: EditableWorkstationType;
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
  const workstationType = resolveEditableWorkstationType(workstation);
  const behavior = resolveEditableWorkstationBehavior(workstation);
  const workerModelProvider = resolveWorkerModelProvider(
    factory,
    workstation.worker ?? "",
  );
  const resolvedRunnerSelection = resolveRunnerSelection(
    workstation.runner,
    factory.runner,
    workerModelProvider,
  );
  const modelOperationsByWorkerName =
    resolveModelOperationsByWorkerName(factory);

  return {
    behavior,
    behaviorOptions: resolveEditableWorkstationBehaviorOptions(
      behavior,
      workstationType,
    ),
    cron:
      behavior === "CRON" ? resolveEditableWorkstationCron(workstation) : null,
    effectiveRunnerName: resolvedRunnerSelection.runnerId,
    factoryRunnerName: factory.runner ?? null,
    modelInvokeWorkerOptions: resolveCompatibleModelWorkerNames(factory),
    modelOperationsByWorkerName,
    operation: workstation.operation ?? "",
    operationBindings: isModelInvokeWorkstationType(workstationType)
      ? syncEditableModelInvokeBindingsForOperation(
          resolveModelOperationByName(
            resolveModelWorkerOperations(factory, workstation.worker ?? ""),
            workstation.operation ?? "",
          ),
          resolveEditableModelInvokeBindings(workstation.operationBindings),
        )
      : [],
    prompt:
      isScriptRunWorkstationType(workstationType) ||
      isPollerRunWorkstationType(workstationType)
        ? null
        : (workstation.body ?? null),
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
    workerName: workstation.worker ?? "",
    workerOptions: resolveWorkerOptions(factory, workstation.type),
    guards: resolveEditableWorkstationGuards(workstation),
    inputs: resolveEditableWorkstationInputs(workstation),
    workstationName: workstation.name,
    workstationOptions: resolveFactoryWorkstationNameOptions(factory),
    workstationType,
    workstationTypeOptions:
      resolveEditableWorkstationTypeOptions(workstationType),
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
    name: values.workstationName,
    operation: values.operation ?? "",
    operationBindings: (values.operationBindings ?? []).map((binding) => ({
      slot: binding.slot,
      configText: binding.configText,
      defaultContentText: binding.defaultContentText,
      selector: { ...binding.selector },
    })),
    prompt: values.prompt ?? "",
    runnerName: values.runnerName,
    workerName: values.workerName,
    workstationType: values.workstationType,
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
  const trimmedName = draft.name.trim();
  const previousWorkstationName = workstation.name;

  if (!workstationRequiresWorkerAssignment({ type: draft.workstationType })) {
    return applyWorkstationNameChangeToFactory(
      factory,
      workstationIndex,
      trimmedName,
      previousWorkstationName,
      (entry) => ({ ...entry, name: trimmedName }),
    );
  }

  if (!factory.workers) {
    return null;
  }

  if (!factory.workers.some((worker) => worker.name === draft.workerName)) {
    return null;
  }

  const legacyDefaultTypeIsUnchanged =
    !workstation.type &&
    draft.workstationType === resolveEditableWorkstationType(workstation);
  const compatibilityWorkstationType = legacyDefaultTypeIsUnchanged
    ? workstation.type
    : draft.workstationType;

  if (
    !isWorkerCompatibleWithWorkstationType(
      factory.workers.find((worker) => worker.name === draft.workerName)?.type,
      compatibilityWorkstationType,
    )
  ) {
    return null;
  }

  const nextWorkstation = isModelInvokeWorkstationType(draft.workstationType)
    ? buildModelInvokeWorkstationFromDraft(workstation, draft, trimmedName)
    : buildPromptOrScriptWorkstationFromDraft(workstation, draft, trimmedName);

  return applyWorkstationNameChangeToFactory(
    factory,
    workstationIndex,
    trimmedName,
    previousWorkstationName,
    () => nextWorkstation,
  );
}

function buildPromptOrScriptWorkstationFromDraft(
  workstation: CanonicalWorkstation,
  draft: EditableWorkstationDraft,
  trimmedName: string,
): CanonicalWorkstation {
  const {
    body: _existingBody,
    behavior: existingBehavior,
    cron: _existingCron,
    guards: _existingGuards,
    inputs: _existingInputs,
    operation: _operation,
    operationBindings: _operationBindings,
    runner: _existingRunner,
    ...workstationWithoutCronRunner
  } = workstation;

  const behavior = isPollerRunWorkstationType(draft.workstationType)
    ? "POLLER"
    : draft.behavior;

  const nextWorkstation = {
    ...workstationWithoutCronRunner,
    inputs: applyEditableWorkstationInputs(draft.inputs),
    name: trimmedName,
    type: draft.workstationType,
    worker: draft.workerName,
    ...(draft.guards.length > 0 ? { guards: draft.guards } : {}),
    ...(workstationUsesRunnerEditing(draft.workstationType, behavior) &&
    draft.runnerName
      ? { runner: draft.runnerName }
      : {}),
    ...(behavior === DEFAULT_WORKSTATION_BEHAVIOR &&
    existingBehavior === undefined
      ? {}
      : { behavior }),
    ...(behavior === "CRON" && draft.cron
      ? { cron: buildCanonicalWorkstationCronFromDraft(draft.cron) }
      : {}),
  };

  const isPromptless =
    isScriptRunWorkstationType(draft.workstationType) ||
    isPollerRunWorkstationType(draft.workstationType);
  if (isPromptless) {
    const { promptFile: _promptFile, ...promptlessWorkstation } =
      nextWorkstation;
    return promptlessWorkstation;
  }

  return { ...nextWorkstation, body: draft.prompt };
}

function buildModelInvokeWorkstationFromDraft(
  workstation: CanonicalWorkstation,
  draft: EditableWorkstationDraft,
  trimmedName: string,
): CanonicalWorkstation {
  const {
    body: _body,
    cron: _cron,
    operation: _existingOperation,
    operationBindings: _existingBindings,
    runner: _runner,
    ...workstationWithoutPromptOrientedFields
  } = workstation;
  const trimmedOperation = draft.operation.trim();
  const operationBindings = buildCanonicalModelInvokeBindingsFromDraft(
    draft.operationBindings,
  );

  return {
    ...workstationWithoutPromptOrientedFields,
    inputs: applyEditableWorkstationInputs(draft.inputs),
    name: trimmedName,
    type: preferredInferenceRunWorkstationType(),
    worker: draft.workerName,
    ...(trimmedOperation.length > 0 ? { operation: trimmedOperation } : {}),
    ...(operationBindings.length > 0 ? { operationBindings } : {}),
    ...(draft.guards.length > 0 ? { guards: draft.guards } : {}),
  };
}

function applyWorkstationNameChangeToFactory(
  factory: CanonicalFactoryDefinition,
  workstationIndex: number,
  trimmedName: string,
  previousWorkstationName: string,
  buildUpdatedWorkstation: (
    workstation: CanonicalWorkstation,
  ) => CanonicalWorkstation,
): CanonicalFactoryDefinition {
  const workstations = factory.workstations ?? [];
  const updatedWorkstations = workstations.map((entry, index) =>
    index === workstationIndex ? buildUpdatedWorkstation(entry) : entry,
  );
  const nextFactory: CanonicalFactoryDefinition = {
    ...factory,
    workers: factory.workers,
    workstations: updatedWorkstations,
  };

  if (previousWorkstationName === trimmedName) {
    return nextFactory;
  }

  return {
    ...nextFactory,
    workstations: (nextFactory.workstations ?? []).map((entry) =>
      rewriteWorkstationVisitCountReferences(
        entry,
        previousWorkstationName,
        trimmedName,
      ),
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
