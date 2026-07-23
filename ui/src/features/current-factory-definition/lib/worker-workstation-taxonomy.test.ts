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
    expect(DEFAULT_WORKER_TYPE).toBe(WorkerType.INFERENCE_WORKER);
    expect(DEFAULT_WORKSTATION_TYPE).toBe(WorkstationType.AGENT_RUN);
    expect(DEFAULT_FACTORY_GRAPH_ADD_WORKSTATION_TYPE).toBe(
      WorkstationType.INFERENCE_RUN,
    );
    expect(FACTORY_GRAPH_ADD_WORKER_TYPES).toEqual(EDITABLE_WORKER_TYPES);
    expect(FACTORY_GRAPH_ADD_WORKSTATION_TYPES).toEqual([
      WorkstationType.INFERENCE_RUN,
      WorkstationType.AGENT_RUN,
      WorkstationType.SCRIPT_RUN,
      WorkstationType.POLLER_RUN,
      WorkstationType.LOGICAL_MOVE,
    ]);
    expect(EDITABLE_WORKER_TYPES).toEqual([
      WorkerType.INFERENCE_WORKER,
      WorkerType.AGENT_WORKER,
      WorkerType.SCRIPT_WORKER,
      WorkerType.POLLER_WORKER,
    ]);
    expect(EDITABLE_WORKSTATION_TYPE_CONVERSION_OPTIONS).toEqual([
      WorkstationType.AGENT_RUN,
      WorkstationType.INFERENCE_RUN,
    ]);
  });

  it("accepts legacy worker aliases for model-provider and poller behavior", () => {
    expect(isInferenceWorkerType(WorkerType.INFERENCE_WORKER)).toBe(true);
    expect(isInferenceWorkerType(WorkerType.MODEL_WORKER)).toBe(true);
    expect(isModelProviderWorkerType(WorkerType.AGENT_WORKER)).toBe(true);
    expect(isPollerWorkerType(WorkerType.HOSTED_WORKER)).toBe(true);
    expect(isPollerWorkerType(WorkerType.POLLER_WORKER)).toBe(true);
  });

  it("accepts legacy workstation aliases for inference and agent runs", () => {
    expect(isInferenceRunWorkstationType(WorkstationType.INFERENCE_RUN)).toBe(
      true,
    );
    expect(isInferenceRunWorkstationType(WorkstationType.MODEL_INVOKE)).toBe(
      true,
    );
    expect(isAgentRunWorkstationType(WorkstationType.AGENT_RUN)).toBe(true);
    expect(isAgentRunWorkstationType(WorkstationType.MODEL_WORKSTATION)).toBe(
      true,
    );
    expect(
      isLegacyRunnableWorkstationType(WorkstationType.MODEL_WORKSTATION),
    ).toBe(true);
  });

  it("keeps legacy worker types selectable while preferring new taxonomy options", () => {
    expect(resolveEditableWorkerTypeOptions(WorkerType.MODEL_WORKER)).toEqual([
      WorkerType.MODEL_WORKER,
      ...EDITABLE_WORKER_TYPES,
    ]);
    expect(
      resolveEditableWorkerTypeOptions(WorkerType.INFERENCE_WORKER),
    ).toEqual(EDITABLE_WORKER_TYPES);
  });

  it("keeps legacy workstation types selectable while preferring new conversion options", () => {
    expect(
      resolveEditableWorkstationTypeConversionOptions(
        WorkstationType.MODEL_WORKSTATION,
      ),
    ).toEqual([
      WorkstationType.MODEL_WORKSTATION,
      ...EDITABLE_WORKSTATION_TYPE_CONVERSION_OPTIONS,
    ]);
    expect(
      resolveEditableWorkstationTypeConversionOptions(
        WorkstationType.AGENT_RUN,
      ),
    ).toEqual(EDITABLE_WORKSTATION_TYPE_CONVERSION_OPTIONS);
  });

  it("returns classifier-only conversion options for classifier workstations", () => {
    expect(
      resolveEditableWorkstationTypeConversionOptions(
        WorkstationType.CLASSIFIER_WORKSTATION,
      ),
    ).toEqual([WorkstationType.CLASSIFIER_WORKSTATION]);
  });

  it("returns preferred conversion options for unsupported workstation types", () => {
    expect(
      resolveEditableWorkstationTypeConversionOptions(
        WorkstationType.SCRIPT_RUN,
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
