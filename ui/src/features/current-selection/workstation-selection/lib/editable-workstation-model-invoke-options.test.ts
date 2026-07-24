import { describe, expect, it } from "vitest";

import type { EditableWorkstationDraft } from "../../../current-factory-definition/lib/workstation-editable-values";
import {
  resolveModelInvokeBindingInputSlots,
  resolveModelInvokeOperationOptionsState,
  resolveSelectedModelInvokeOperation,
} from "./editable-workstation-model-invoke-options";

const messages = {
  editableConfigurationModelInvokeOperationMissing: "missing operation",
  editableConfigurationModelInvokeOperationOptionsEmpty: "no operations",
  editableConfigurationModelInvokeWorkerRequired: "worker required",
};

const baseDraft: EditableWorkstationDraft = {
  behavior: "STANDARD",
  cron: null,
  guards: [],
  inputs: [],
  name: "Invoke",
  operation: "TTS",
  operationBindings: [],
  prompt: "",
  runnerName: null,
  workerName: "tts-worker",
  workstationType: "MODEL_INVOKE",
};

const selectedEditableValues = {
  modelOperationsByWorkerName: {
    "tts-worker": [
      {
        name: "TTS",
        inputs: [{ name: "text", contentTypes: ["TEXT"], required: true }],
        outputs: [{ name: "audio", contentTypes: ["AUDIO"] }],
      },
    ],
  },
} as const;

describe("editable workstation model invoke options", () => {
  it("requires a worker before exposing operation options", () => {
    expect(
      resolveModelInvokeOperationOptionsState(
        { ...baseDraft, workerName: "  " },
        selectedEditableValues,
        messages,
      ),
    ).toEqual({
      message: messages.editableConfigurationModelInvokeWorkerRequired,
      status: "empty",
    });
  });

  it("reports empty operation catalogs for workers without declared operations", () => {
    expect(
      resolveModelInvokeOperationOptionsState(
        { ...baseDraft, workerName: "missing-worker" },
        selectedEditableValues,
        messages,
      ),
    ).toEqual({
      message: messages.editableConfigurationModelInvokeOperationOptionsEmpty,
      status: "empty",
    });
  });

  it("flags operations that are not declared on the selected worker", () => {
    expect(
      resolveModelInvokeOperationOptionsState(
        { ...baseDraft, operation: "STT" },
        selectedEditableValues,
        messages,
      ),
    ).toEqual({
      message: messages.editableConfigurationModelInvokeOperationMissing,
      status: "error",
    });
  });

  it("returns ready operation options when the draft operation is blank", () => {
    expect(
      resolveModelInvokeOperationOptionsState(
        { ...baseDraft, operation: "" },
        selectedEditableValues,
        messages,
      ),
    ).toEqual({
      operations:
        selectedEditableValues.modelOperationsByWorkerName["tts-worker"],
      options: ["TTS"],
      status: "ready",
    });
  });

  it("returns ready operation options and binding slots for valid selections", () => {
    expect(
      resolveModelInvokeOperationOptionsState(
        baseDraft,
        selectedEditableValues,
        messages,
      ),
    ).toEqual({
      operations:
        selectedEditableValues.modelOperationsByWorkerName["tts-worker"],
      options: ["TTS"],
      status: "ready",
    });

    expect(
      resolveSelectedModelInvokeOperation(baseDraft, selectedEditableValues)
        ?.name,
    ).toBe("TTS");
    expect(
      resolveModelInvokeBindingInputSlots(baseDraft, selectedEditableValues),
    ).toEqual([
      expect.objectContaining({
        name: "text",
      }),
    ]);
  });
});
