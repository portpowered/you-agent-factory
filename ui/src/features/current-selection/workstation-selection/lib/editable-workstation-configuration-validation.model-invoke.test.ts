import { describe, expect, it } from "vitest";
import {
  baseEditableWorkstationDraft,
  editableWorkstationValidationMessages,
  modelWorkstationValues,
} from "../../../../testing/editable-workstation-configuration-validation-fixtures";
import {
  hasEditableWorkstationValidationErrors,
  validateEditableWorkstationDraft,
} from "./validation/editable-workstation-configuration-validation";

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: grouped model-invoke validation cases share one fixture harness.
describe("validateEditableWorkstationDraft model invoke", () => {
  const modelInvokeValues = {
    ...modelWorkstationValues,
    modelInvokeWorkerOptions: ["tts-worker"],
    modelOperationsByWorkerName: {
      "tts-worker": [
        {
          name: "TTS",
          inputs: [{ name: "text", contentTypes: ["TEXT"], required: true }],
          outputs: [{ name: "audio", contentTypes: ["AUDIO"] }],
        },
      ],
    },
    workstationType: "MODEL_INVOKE" as const,
    workstationTypeOptions: ["MODEL_WORKSTATION", "MODEL_INVOKE"] as const,
  };
  const modelInvokeDraft = {
    ...baseEditableWorkstationDraft,
    workstationType: "MODEL_INVOKE" as const,
  };

  it("requires operation but not prompt for MODEL_INVOKE drafts", () => {
    const errors = validateEditableWorkstationDraft(
      {
        ...modelInvokeDraft,
        workerName: "tts-worker",
        prompt: "",
      },
      modelInvokeValues,
      { status: "idle" },
      editableWorkstationValidationMessages,
    );

    expect(errors.operation).toBe(
      editableWorkstationValidationMessages.editableConfigurationModelInvokeOperationRequired,
    );
    expect(errors.prompt).toBeUndefined();
  });

  it("blocks save when required model-invoke bindings are empty", () => {
    const errors = validateEditableWorkstationDraft(
      {
        ...modelInvokeDraft,
        workerName: "tts-worker",
        operation: "TTS",
        operationBindings: [
          {
            slot: "text",
            configText: "",
            defaultContentText: "",
            selector: { label: "", role: "", slot: "", type: "" },
          },
        ],
        prompt: "",
      },
      modelInvokeValues,
      { status: "idle" },
      editableWorkstationValidationMessages,
    );

    expect(errors["operationBindings[text]"]).toBe(
      editableWorkstationValidationMessages.editableConfigurationModelInvokeBindingRequired(
        "text",
      ),
    );
    expect(errors.operationBindings).toBe(
      editableWorkstationValidationMessages.editableConfigurationModelInvokeBindingsSummary,
    );
    expect(hasEditableWorkstationValidationErrors(errors)).toBe(true);
  });

  it("requires a model worker before saving MODEL_INVOKE drafts", () => {
    const errors = validateEditableWorkstationDraft(
      {
        ...modelInvokeDraft,
        workerName: "",
        operation: "TTS",
        prompt: "",
      },
      modelInvokeValues,
      { status: "idle" },
      editableWorkstationValidationMessages,
    );

    expect(errors.workerName).toBe(
      editableWorkstationValidationMessages.editableConfigurationWorkerRequired,
    );
  });

  it("blocks duplicate model-invoke binding slots", () => {
    const errors = validateEditableWorkstationDraft(
      {
        ...modelInvokeDraft,
        workerName: "tts-worker",
        operation: "TTS",
        operationBindings: [
          {
            slot: "text",
            configText: "one",
            defaultContentText: "",
            selector: { label: "", role: "", slot: "", type: "" },
          },
          {
            slot: "text",
            configText: "two",
            defaultContentText: "",
            selector: { label: "", role: "", slot: "", type: "" },
          },
        ],
        prompt: "",
      },
      modelInvokeValues,
      { status: "idle" },
      editableWorkstationValidationMessages,
    );

    expect(errors["operationBindings[text]"]).toBe(
      editableWorkstationValidationMessages.editableConfigurationModelInvokeBindingDuplicate(
        "text",
      ),
    );
  });

  it("allows optional model-invoke bindings to remain omitted", () => {
    const valuesWithOptionalSlot = {
      ...modelInvokeValues,
      modelOperationsByWorkerName: {
        "tts-worker": [
          {
            name: "TTS",
            inputs: [
              { name: "text", contentTypes: ["TEXT"], required: true },
              { name: "voice", contentTypes: ["JSON"] },
            ],
            outputs: [{ name: "audio", contentTypes: ["AUDIO"] }],
          },
        ],
      },
    };
    const errors = validateEditableWorkstationDraft(
      {
        ...modelInvokeDraft,
        workerName: "tts-worker",
        operation: "TTS",
        operationBindings: [
          {
            slot: "text",
            configText: "",
            defaultContentText: "",
            selector: { label: "utterance", role: "", slot: "", type: "TEXT" },
          },
          {
            slot: "voice",
            configText: "",
            defaultContentText: "",
            selector: { label: "", role: "", slot: "", type: "" },
          },
        ],
        prompt: "",
      },
      valuesWithOptionalSlot,
      { status: "idle" },
      editableWorkstationValidationMessages,
    );

    expect(errors.operationBindings).toBeUndefined();
    expect(hasEditableWorkstationValidationErrors(errors)).toBe(false);
  });
});
