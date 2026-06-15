import { describe, expect, it } from "vitest";

import { WorkerType, WorkstationType } from "../../../api/generated/openapi";
import {
  DEFAULT_FACTORY_GRAPH_ADD_WORKSTATION_TYPE,
  DEFAULT_WORKER_TYPE,
  DEFAULT_WORKSTATION_TYPE,
  EDITABLE_WORKER_TYPES,
  EDITABLE_WORKSTATION_TYPE_CONVERSION_OPTIONS,
  FACTORY_GRAPH_ADD_WORKER_TYPES,
  FACTORY_GRAPH_ADD_WORKSTATION_TYPES,
  isAgentRunWorkstationType,
  isInferenceRunWorkstationType,
  isInferenceWorkerType,
  isLegacyRunnableWorkstationType,
  isModelProviderWorkerType,
  isPollerWorkerType,
  resolveEditableWorkerTypeOptions,
  resolveEditableWorkstationTypeConversionOptions,
} from "./worker-workstation-taxonomy";

describe("worker-workstation taxonomy helpers", () => {
  it("prefers new taxonomy defaults for factory creation", () => {
    expect(DEFAULT_WORKER_TYPE).toBe(WorkerType.WorkerTypeInferenceWorker);
    expect(DEFAULT_WORKSTATION_TYPE).toBe(WorkstationType.WorkstationTypeAgentRun);
    expect(DEFAULT_FACTORY_GRAPH_ADD_WORKSTATION_TYPE).toBe(
      WorkstationType.WorkstationTypeInferenceRun,
    );
    expect(FACTORY_GRAPH_ADD_WORKER_TYPES).toEqual(EDITABLE_WORKER_TYPES);
    expect(FACTORY_GRAPH_ADD_WORKSTATION_TYPES).toEqual([
      WorkstationType.WorkstationTypeInferenceRun,
      WorkstationType.WorkstationTypeAgentRun,
      WorkstationType.WorkstationTypeScriptRun,
      WorkstationType.WorkstationTypePollerRun,
      WorkstationType.WorkstationTypeLogicalMove,
    ]);
    expect(EDITABLE_WORKER_TYPES).toEqual([
      WorkerType.WorkerTypeInferenceWorker,
      WorkerType.WorkerTypeAgentWorker,
      WorkerType.WorkerTypeScriptWorker,
      WorkerType.WorkerTypePollerWorker,
    ]);
    expect(EDITABLE_WORKSTATION_TYPE_CONVERSION_OPTIONS).toEqual([
      WorkstationType.WorkstationTypeAgentRun,
      WorkstationType.WorkstationTypeInferenceRun,
    ]);
  });

  it("accepts legacy worker aliases for model-provider and poller behavior", () => {
    expect(isInferenceWorkerType(WorkerType.WorkerTypeInferenceWorker)).toBe(
      true,
    );
    expect(isInferenceWorkerType(WorkerType.WorkerTypeModelWorker)).toBe(true);
    expect(isModelProviderWorkerType(WorkerType.WorkerTypeAgentWorker)).toBe(
      true,
    );
    expect(isPollerWorkerType(WorkerType.WorkerTypeHostedWorker)).toBe(true);
    expect(isPollerWorkerType(WorkerType.WorkerTypePollerWorker)).toBe(true);
  });

  it("accepts legacy workstation aliases for inference and agent runs", () => {
    expect(
      isInferenceRunWorkstationType(WorkstationType.WorkstationTypeInferenceRun),
    ).toBe(true);
    expect(
      isInferenceRunWorkstationType(WorkstationType.WorkstationTypeModelInvoke),
    ).toBe(true);
    expect(isAgentRunWorkstationType(WorkstationType.WorkstationTypeAgentRun)).toBe(
      true,
    );
    expect(
      isAgentRunWorkstationType(
        WorkstationType.WorkstationTypeModelWorkstation,
      ),
    ).toBe(true);
    expect(
      isLegacyRunnableWorkstationType(
        WorkstationType.WorkstationTypeModelWorkstation,
      ),
    ).toBe(true);
  });

  it("keeps legacy worker types selectable while preferring new taxonomy options", () => {
    expect(
      resolveEditableWorkerTypeOptions(WorkerType.WorkerTypeModelWorker),
    ).toEqual([
      WorkerType.WorkerTypeModelWorker,
      ...EDITABLE_WORKER_TYPES,
    ]);
    expect(
      resolveEditableWorkerTypeOptions(WorkerType.WorkerTypeInferenceWorker),
    ).toEqual(EDITABLE_WORKER_TYPES);
  });

  it("keeps legacy workstation types selectable while preferring new conversion options", () => {
    expect(
      resolveEditableWorkstationTypeConversionOptions(
        WorkstationType.WorkstationTypeModelWorkstation,
      ),
    ).toEqual([
      WorkstationType.WorkstationTypeModelWorkstation,
      ...EDITABLE_WORKSTATION_TYPE_CONVERSION_OPTIONS,
    ]);
    expect(
      resolveEditableWorkstationTypeConversionOptions(
        WorkstationType.WorkstationTypeAgentRun,
      ),
    ).toEqual(EDITABLE_WORKSTATION_TYPE_CONVERSION_OPTIONS);
  });

  it("returns classifier-only conversion options for classifier workstations", () => {
    expect(
      resolveEditableWorkstationTypeConversionOptions(
        WorkstationType.WorkstationTypeClassifierWorkstation,
      ),
    ).toEqual([WorkstationType.WorkstationTypeClassifierWorkstation]);
  });

  it("returns preferred conversion options for unsupported workstation types", () => {
    expect(
      resolveEditableWorkstationTypeConversionOptions(
        WorkstationType.WorkstationTypeScriptRun,
      ),
    ).toEqual(EDITABLE_WORKSTATION_TYPE_CONVERSION_OPTIONS);
  });

  it("returns preferred worker options for unsupported worker types", () => {
    expect(
      resolveEditableWorkerTypeOptions(
        "UNSUPPORTED_WORKER" as (typeof EDITABLE_WORKER_TYPES)[number],
      ),
    ).toEqual(EDITABLE_WORKER_TYPES);
  });
});
