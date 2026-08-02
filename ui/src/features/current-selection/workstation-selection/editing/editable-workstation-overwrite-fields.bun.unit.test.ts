import { describe, expect, it } from "bun:test";

import type { EditableWorkstationDraft } from "../../../current-factory-definition/lib/workstation-editable-values";
import {
  formatEditableOverwriteFieldLabels,
  resolveEditableWorkstationOverwriteFields,
} from "./editable-workstation-overwrite-fields";

const baseDraft: EditableWorkstationDraft = {
  behavior: "STANDARD",
  cron: null,
  guards: [],
  inputs: [],
  name: "Invoke",
  operation: "TTS",
  operationBindings: [
    {
      slot: "text",
      configText: "",
      defaultContentText: "",
      selector: { label: "utterance", role: "", slot: "", type: "TEXT" },
    },
  ],
  prompt: "",
  runnerName: null,
  workerName: "tts-worker",
  workstationType: "MODEL_INVOKE",
};

const messages = {
  cronExpiryWindowFieldLabel: "Expiry window",
  cronJitterFieldLabel: "Jitter",
  cronScheduleFieldLabel: "Schedule",
  cronTriggerAtStartFieldLabel: "Trigger at start",
  kindLabel: "Kind",
  modelInvokeBindingsFieldLabel: "Operation bindings",
  modelInvokeOperationFieldLabel: "Operation",
  promptFieldLabel: "Prompt",
  runnerFieldLabel: "Runner",
  workerFieldLabel: "Worker",
  workstationNameFieldLabel: "Workstation name",
  workstationTypeLabel: "Workstation type",
};

describe("editable workstation overwrite fields", () => {
  it("flags model-invoke workstation type, operation, and binding drift", () => {
    const latestDefinitionDraft: EditableWorkstationDraft = {
      ...baseDraft,
      operation: "STT",
      operationBindings: [
        {
          slot: "audio",
          configText: "",
          defaultContentText: "",
          selector: { label: "", role: "", slot: "", type: "" },
        },
      ],
      workstationType: "MODEL_WORKSTATION",
    };
    const sessionStartDraft: EditableWorkstationDraft = {
      ...baseDraft,
      operation: "TTS",
    };
    const draft: EditableWorkstationDraft = {
      ...baseDraft,
      operation: "TTS",
      workstationType: "MODEL_INVOKE",
    };

    expect(
      resolveEditableWorkstationOverwriteFields(
        sessionStartDraft,
        draft,
        latestDefinitionDraft,
      ),
    ).toEqual(
      expect.arrayContaining([
        "workstationType",
        "operation",
        "operationBindings",
      ]),
    );
  });

  it("ignores model-invoke fields that already match the latest definition", () => {
    const latestDefinitionDraft: EditableWorkstationDraft = {
      ...baseDraft,
      operation: "STT",
    };

    expect(
      resolveEditableWorkstationOverwriteFields(
        baseDraft,
        {
          ...baseDraft,
          operation: "STT",
        },
        latestDefinitionDraft,
      ),
    ).toEqual([]);
  });

  it("flags behavior and runner drift for prompt-oriented workstations", () => {
    const sessionStartDraft: EditableWorkstationDraft = {
      ...baseDraft,
      behavior: "STANDARD",
      runnerName: "codex",
      workstationType: "MODEL_WORKSTATION",
      operation: "",
      operationBindings: [],
      prompt: "Review the story.",
    };
    const latestDefinitionDraft: EditableWorkstationDraft = {
      ...sessionStartDraft,
      behavior: "CRON",
      runnerName: "reviewer",
    };
    const draft: EditableWorkstationDraft = {
      ...sessionStartDraft,
      behavior: "STANDARD",
      runnerName: "codex",
    };

    expect(
      resolveEditableWorkstationOverwriteFields(
        sessionStartDraft,
        draft,
        latestDefinitionDraft,
      ),
    ).toEqual(expect.arrayContaining(["behavior", "runner"]));
  });

  it("formats model-invoke overwrite labels for the warning banner", () => {
    expect(
      formatEditableOverwriteFieldLabels(
        ["workstationType", "operation", "operationBindings"],
        messages,
      ),
    ).toBe("workstation type, operation, operation bindings");
  });
});
