import type { components } from "../../../api/generated/openapi";
import { WorkerType, WorkstationType } from "../../../api/generated/openapi";

export type ApiWorkerType = components["schemas"]["WorkerType"];
export type ApiWorkstationType = components["schemas"]["WorkstationType"];

export const DEFAULT_WORKER_TYPE: ApiWorkerType = WorkerType.INFERENCE_WORKER;

export const DEFAULT_WORKSTATION_TYPE: ApiWorkstationType =
  WorkstationType.AGENT_RUN;

/** Compatible with `DEFAULT_WORKER_TYPE` for graph add-workstation drafts. */
export const DEFAULT_FACTORY_GRAPH_ADD_WORKSTATION_TYPE: ApiWorkstationType =
  WorkstationType.INFERENCE_RUN;

export const EDITABLE_WORKER_TYPES = [
  WorkerType.INFERENCE_WORKER,
  WorkerType.AGENT_WORKER,
  WorkerType.SCRIPT_WORKER,
  WorkerType.POLLER_WORKER,
] as const satisfies readonly ApiWorkerType[];

export const EDITABLE_WORKSTATION_TYPE_CONVERSION_OPTIONS = [
  WorkstationType.AGENT_RUN,
  WorkstationType.INFERENCE_RUN,
] as const satisfies readonly ApiWorkstationType[];

export const FACTORY_GRAPH_ADD_WORKER_TYPES = EDITABLE_WORKER_TYPES;

export type FactoryGraphAddWorkerType =
  (typeof FACTORY_GRAPH_ADD_WORKER_TYPES)[number];

export const FACTORY_GRAPH_ADD_WORKSTATION_TYPES = [
  WorkstationType.INFERENCE_RUN,
  WorkstationType.AGENT_RUN,
  WorkstationType.SCRIPT_RUN,
  WorkstationType.POLLER_RUN,
  WorkstationType.LOGICAL_MOVE,
] as const satisfies readonly ApiWorkstationType[];

export function isInferenceWorkerType(
  workerType: ApiWorkerType | string | null | undefined,
): boolean {
  return (
    workerType === WorkerType.INFERENCE_WORKER ||
    workerType === WorkerType.MODEL_WORKER
  );
}

export function isAgentWorkerType(
  workerType: ApiWorkerType | string | null | undefined,
): boolean {
  return workerType === WorkerType.AGENT_WORKER;
}

export function isModelProviderWorkerType(
  workerType: ApiWorkerType | string | null | undefined,
): boolean {
  return isInferenceWorkerType(workerType) || isAgentWorkerType(workerType);
}

export function isScriptWorkerType(
  workerType: ApiWorkerType | string | null | undefined,
): boolean {
  return workerType === WorkerType.SCRIPT_WORKER;
}

export function isPollerWorkerType(
  workerType: ApiWorkerType | string | null | undefined,
): boolean {
  return (
    workerType === WorkerType.POLLER_WORKER ||
    workerType === WorkerType.HOSTED_WORKER
  );
}

export function isInferenceRunWorkstationType(
  workstationType: ApiWorkstationType | string | null | undefined,
): boolean {
  return (
    workstationType === WorkstationType.INFERENCE_RUN ||
    workstationType === WorkstationType.MODEL_INVOKE
  );
}

export function isAgentRunWorkstationType(
  workstationType: ApiWorkstationType | string | null | undefined,
): boolean {
  return (
    workstationType === WorkstationType.AGENT_RUN ||
    workstationType === WorkstationType.MODEL_WORKSTATION
  );
}

export function isLegacyRunnableWorkstationType(
  workstationType: ApiWorkstationType | string | null | undefined,
): boolean {
  return (
    isInferenceRunWorkstationType(workstationType) ||
    isAgentRunWorkstationType(workstationType)
  );
}

export function isPollerRunWorkstationType(
  workstationType: ApiWorkstationType | string | null | undefined,
): boolean {
  return workstationType === WorkstationType.POLLER_RUN;
}

export function isLegacyWorkerType(
  workerType: ApiWorkerType | string | null | undefined,
): boolean {
  return (
    workerType === WorkerType.MODEL_WORKER ||
    workerType === WorkerType.HOSTED_WORKER
  );
}

export function resolveEditableWorkerTypeOptions(
  workerType: ApiWorkerType,
): readonly ApiWorkerType[] {
  if (
    (EDITABLE_WORKER_TYPES as readonly ApiWorkerType[]).includes(workerType)
  ) {
    return EDITABLE_WORKER_TYPES;
  }
  if (isLegacyWorkerType(workerType)) {
    return [workerType, ...EDITABLE_WORKER_TYPES];
  }
  return EDITABLE_WORKER_TYPES;
}

export function resolveEditableWorkstationTypeConversionOptions(
  workstationType: ApiWorkstationType,
): readonly ApiWorkstationType[] {
  if (workstationType === WorkstationType.LOGICAL_MOVE) {
    return [WorkstationType.LOGICAL_MOVE];
  }
  if (workstationType === WorkstationType.CLASSIFIER_WORKSTATION) {
    return [WorkstationType.CLASSIFIER_WORKSTATION];
  }

  const preferred = EDITABLE_WORKSTATION_TYPE_CONVERSION_OPTIONS;
  if ((preferred as readonly ApiWorkstationType[]).includes(workstationType)) {
    return preferred;
  }
  if (isLegacyRunnableWorkstationType(workstationType)) {
    return [workstationType, ...preferred];
  }
  return preferred;
}

export function preferredInferenceRunWorkstationType(): ApiWorkstationType {
  return WorkstationType.INFERENCE_RUN;
}
