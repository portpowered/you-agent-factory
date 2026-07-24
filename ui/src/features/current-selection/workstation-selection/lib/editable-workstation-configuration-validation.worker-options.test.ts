import { describe, expect, it } from "vitest";
import {
  baseEditableWorkstationDraft,
  editableWorkstationValidationMessages,
  modelWorkstationValues,
} from "../../../../testing/editable-workstation-configuration-validation-fixtures";
import type { resolveEditableWorkstationValues } from "../../../current-factory-definition/lib/workstation-editable-values";
import { resolveWorkerOptionsState } from "./validation/editable-workstation-configuration-validation";

describe("resolveWorkerOptionsState logical move", () => {
  it("returns ready options without worker membership checks for LOGICAL_MOVE", () => {
    const state = resolveWorkerOptionsState(
      {
        ...baseEditableWorkstationDraft,
        workerName: "missing-worker",
        workstationType: "LOGICAL_MOVE",
      },
      {
        behavior: "STANDARD",
        behaviorOptions: ["STANDARD"],
        effectiveRunnerName: "gemini",
        factoryRunnerName: "codex",
        guards: [],
        inputs: [],
        modelInvokeWorkerOptions: [],
        modelOperationsByWorkerName: {},
        operation: "",
        operationBindings: [],
        prompt: "",
        resolvedRunnerSelection: {
          runnerId: "gemini",
          source: "workstation",
        },
        runnerName: "gemini",
        runnerOptions: ["gemini"],
        runnerSelectionSource: "workstation",
        sharedWorkerWorkstationNames: [],
        sharedWorkerWorkstationNamesByWorkerName: {},
        workerModelProvider: null,
        workerName: "reviewer",
        workerOptions: ["reviewer"],
        workerTypeByName: { reviewer: "MODEL_WORKER" },
        workstationName: "Move",
        workstationOptions: ["Move"],
        workstationType: "LOGICAL_MOVE",
        workstationTypeOptions: ["LOGICAL_MOVE"],
      },
      editableWorkstationValidationMessages,
    );

    expect(state).toEqual({
      options: ["reviewer"],
      status: "ready",
    });
  });
});

describe("resolveWorkerOptionsState model workstation", () => {
  it("returns error when editable values are unavailable", () => {
    const state = resolveWorkerOptionsState(
      baseEditableWorkstationDraft,
      undefined as unknown as ReturnType<
        typeof resolveEditableWorkstationValues
      >,
      editableWorkstationValidationMessages,
    );

    expect(state).toEqual({
      message: editableWorkstationValidationMessages.editableConfigurationEmpty,
      status: "error",
    });
  });

  it("returns empty and error states for MODEL_WORKSTATION worker selection", () => {
    const emptyState = resolveWorkerOptionsState(
      { ...baseEditableWorkstationDraft, workerName: "reviewer" },
      {
        ...modelWorkstationValues,
        workerOptions: [],
      },
      editableWorkstationValidationMessages,
    );

    expect(emptyState).toEqual({
      message:
        editableWorkstationValidationMessages.editableConfigurationWorkerOptionsEmpty,
      status: "empty",
    });

    const missingWorkerState = resolveWorkerOptionsState(
      { ...baseEditableWorkstationDraft, workerName: "missing-worker" },
      modelWorkstationValues,
      editableWorkstationValidationMessages,
    );

    expect(missingWorkerState).toEqual({
      message:
        editableWorkstationValidationMessages.editableConfigurationWorkerMissing,
      status: "error",
    });

    const readyState = resolveWorkerOptionsState(
      { ...baseEditableWorkstationDraft, workerName: "reviewer" },
      modelWorkstationValues,
      editableWorkstationValidationMessages,
    );

    expect(readyState).toEqual({
      options: ["reviewer", "planner"],
      status: "ready",
    });
  });
});
