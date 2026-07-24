import { describe, expect, it, vi } from "vitest";

import type { EditableWorkstationDraft } from "../../../../current-factory-definition/lib/workstation-editable-values";
import {
  resolveModelInvokeDraftForOperationChange,
  resolveModelInvokeDraftForWorkerChange,
  updateEditableModelInvokeBindingDraft,
} from "./editable-workstation-model-invoke-mutators";

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
      selector: { label: "", role: "", slot: "", type: "" },
    },
  ],
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
      {
        name: "STT",
        inputs: [{ name: "audio", contentTypes: ["AUDIO"], required: true }],
        outputs: [{ name: "text", contentTypes: ["TEXT"] }],
      },
    ],
  },
} as const;

describe("editable workstation model invoke mutators", () => {
  it("re-seeds worker and operation bindings when the worker changes", () => {
    expect(
      resolveModelInvokeDraftForWorkerChange(
        baseDraft,
        "tts-worker",
        selectedEditableValues,
      ),
    ).toMatchObject({
      workerName: "tts-worker",
      operation: "TTS",
      operationBindings: [expect.objectContaining({ slot: "text" })],
    });
  });

  it("syncs bindings when the operation changes", () => {
    expect(
      resolveModelInvokeDraftForOperationChange(
        baseDraft,
        "STT",
        selectedEditableValues,
      ),
    ).toMatchObject({
      operation: "STT",
      operationBindings: [expect.objectContaining({ slot: "audio" })],
    });
  });

  it("clears operation bindings when the worker has no declared operations", () => {
    expect(
      resolveModelInvokeDraftForWorkerChange(baseDraft, "missing-worker", {
        modelOperationsByWorkerName: {},
      } as typeof selectedEditableValues),
    ).toMatchObject({
      workerName: "missing-worker",
      operation: "",
      operationBindings: [],
    });
  });

  it("updates only the targeted binding slot", () => {
    const updater = vi.fn(
      (binding: (typeof baseDraft.operationBindings)[number]) => ({
        ...binding,
        configText: "updated",
      }),
    );

    expect(
      updateEditableModelInvokeBindingDraft(
        baseDraft.operationBindings,
        "text",
        updater,
      ),
    ).toEqual([
      expect.objectContaining({
        slot: "text",
        configText: "updated",
      }),
    ]);
    expect(updater).toHaveBeenCalledTimes(1);
  });
});
