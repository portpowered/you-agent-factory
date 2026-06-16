import type { components } from "../../../api/generated/openapi";
import { WorkerType, WorkstationType } from "../../../api/generated/openapi";

export type ApiWorkerType = components["schemas"]["WorkerType"];
export type ApiWorkstationType = components["schemas"]["WorkstationType"];

export const DEFAULT_WORKER_TYPE: ApiWorkerType =
  WorkerType.WorkerTypeInferenceWorker;

export const DEFAULT_WORKSTATION_TYPE: ApiWorkstationType =
  WorkstationType.WorkstationTypeAgentRun;

/** Compatible with `DEFAULT_WORKER_TYPE` for graph add-workstation drafts. */
export const DEFAULT_FACTORY_GRAPH_ADD_WORKSTATION_TYPE: ApiWorkstationType =
  WorkstationType.WorkstationTypeInferenceRun;

export const EDITABLE_WORKER_TYPES = [
  WorkerType.WorkerTypeInferenceWorker,
  WorkerType.WorkerTypeAgentWorker,
  WorkerType.WorkerTypeScriptWorker,
  WorkerType.WorkerTypePollerWorker,
] as const satisfies readonly ApiWorkerType[];

export const EDITABLE_WORKSTATION_TYPE_CONVERSION_OPTIONS = [
  WorkstationType.WorkstationTypeAgentRun,
  WorkstationType.WorkstationTypeInferenceRun,
] as const satisfies readonly ApiWorkstationType[];

export const FACTORY_GRAPH_ADD_WORKER_TYPES = EDITABLE_WORKER_TYPES;

export type FactoryGraphAddWorkerType =
  (typeof FACTORY_GRAPH_ADD_WORKER_TYPES)[number];

export const FACTORY_GRAPH_ADD_WORKSTATION_TYPES = [
  WorkstationType.WorkstationTypeInferenceRun,
  WorkstationType.WorkstationTypeAgentRun,
  WorkstationType.WorkstationTypeScriptRun,
  WorkstationType.WorkstationTypePollerRun,
  WorkstationType.WorkstationTypeLogicalMove,
] as const satisfies readonly ApiWorkstationType[];

export function isInferenceWorkerType(
  workerType: ApiWorkerType | string | null | undefined,
): boolean {
  return (
    workerType === WorkerType.WorkerTypeInferenceWorker ||
    workerType === WorkerType.WorkerTypeModelWorker
  );
}

export function isAgentWorkerType(
  workerType: ApiWorkerType | string | null | undefined,
): boolean {
  return workerType === WorkerType.WorkerTypeAgentWorker;
}

export function isModelProviderWorkerType(
  workerType: ApiWorkerType | string | null | undefined,
): boolean {
  return isInferenceWorkerType(workerType) || isAgentWorkerType(workerType);
}

export function isScriptWorkerType(
  workerType: ApiWorkerType | string | null | undefined,
): boolean {
  return workerType === WorkerType.WorkerTypeScriptWorker;
}

export function isPollerWorkerType(
  workerType: ApiWorkerType | string | null | undefined,
): boolean {
  return (
    workerType === WorkerType.WorkerTypePollerWorker ||
    workerType === WorkerType.WorkerTypeHostedWorker
  );
}

export function isInferenceRunWorkstationType(
  workstationType: ApiWorkstationType | string | null | undefined,
): boolean {
  return (
    workstationType === WorkstationType.WorkstationTypeInferenceRun ||
    workstationType === WorkstationType.WorkstationTypeModelInvoke
  );
}

export function isAgentRunWorkstationType(
  workstationType: ApiWorkstationType | string | null | undefined,
): boolean {
  return (
    workstationType === WorkstationType.WorkstationTypeAgentRun ||
    workstationType === WorkstationType.WorkstationTypeModelWorkstation
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
  return workstationType === WorkstationType.WorkstationTypePollerRun;
}

export function isLegacyWorkerType(
  workerType: ApiWorkerType | string | null | undefined,
): boolean {
  return (
    workerType === WorkerType.WorkerTypeModelWorker ||
    workerType === WorkerType.WorkerTypeHostedWorker
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
  if (workstationType === WorkstationType.WorkstationTypeLogicalMove) {
    return [WorkstationType.WorkstationTypeLogicalMove];
  }
  if (
    workstationType === WorkstationType.WorkstationTypeClassifierWorkstation
  ) {
    return [WorkstationType.WorkstationTypeClassifierWorkstation];
  }

  const preferred = EDITABLE_WORKSTATION_TYPE_CONVERSION_OPTIONS;
  if (
    (preferred as readonly ApiWorkstationType[]).includes(workstationType)
  ) {
    return preferred;
  }
  if (isLegacyRunnableWorkstationType(workstationType)) {
    return [workstationType, ...preferred];
  }
  return preferred;
}

export function preferredInferenceRunWorkstationType(): ApiWorkstationType {
  return WorkstationType.WorkstationTypeInferenceRun;
}
