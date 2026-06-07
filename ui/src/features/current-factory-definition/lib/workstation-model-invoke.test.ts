import { describe, expect, it } from "vitest";

import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import {
  buildCanonicalModelInvokeBindingsFromDraft,
  editableModelInvokeBindingsEqual,
  isModelInvokeWorkstationType,
  resolveCompatibleModelWorkerNames,
  resolveEditableModelInvokeBindings,
  resolveModelWorkerOperations,
  syncEditableModelInvokeBindingsForOperation,
  workstationUsesPromptOrientedEditing,
} from "./workstation-model-invoke";

const modelInvokeFactory: CanonicalFactoryDefinition = {
  name: "tts-factory",
  workers: [
    {
      name: "tts-worker",
      type: "MODEL_WORKER",
      operations: [
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
    {
      name: "reviewer",
      type: "MODEL_WORKER",
      model: "gpt-5.4",
    },
  ],
  workstations: [
    {
      name: "speak-story",
      type: "MODEL_INVOKE",
      worker: "tts-worker",
      operation: "TTS",
      operationBindings: [
        {
          slot: "text",
          selector: { label: "utterance", type: "TEXT" },
        },
      ],
      inputs: [{ state: "init", workType: "story" }],
      outputs: [{ state: "complete", workType: "story" }],
    },
  ],
  workTypes: [],
};

describe("workstation model invoke helpers", () => {
  it("detects model invoke workstation types", () => {
    expect(isModelInvokeWorkstationType("MODEL_INVOKE")).toBe(true);
    expect(isModelInvokeWorkstationType("MODEL_WORKSTATION")).toBe(false);
    expect(workstationUsesPromptOrientedEditing("MODEL_INVOKE")).toBe(false);
    expect(workstationUsesPromptOrientedEditing("MODEL_WORKSTATION")).toBe(
      true,
    );
  });

  it("resolves compatible model workers and operations from the factory", () => {
    expect(resolveCompatibleModelWorkerNames(modelInvokeFactory)).toEqual([
      "tts-worker",
    ]);
    expect(
      resolveModelWorkerOperations(modelInvokeFactory, "tts-worker").map(
        (operation) => operation.name,
      ),
    ).toEqual(["TTS"]);
    expect(resolveModelWorkerOperations(modelInvokeFactory, "reviewer")).toEqual(
      [],
    );
  });

  it("round-trips editable model invoke bindings", () => {
    const bindings = resolveEditableModelInvokeBindings([
      {
        slot: "text",
        selector: { label: "utterance", type: "TEXT" },
      },
    ]);

    expect(bindings).toEqual([
      {
        slot: "text",
        selector: { label: "utterance", role: "", slot: "", type: "TEXT" },
      },
    ]);
    expect(buildCanonicalModelInvokeBindingsFromDraft(bindings)).toEqual([
      {
        slot: "text",
        selector: { label: "utterance", type: "TEXT" },
      },
    ]);
    expect(editableModelInvokeBindingsEqual(bindings, bindings)).toBe(true);
  });

  it("syncs binding rows to the selected operation input slots", () => {
    const operation = resolveModelWorkerOperations(
      modelInvokeFactory,
      "tts-worker",
    )[0];
    const synced = syncEditableModelInvokeBindingsForOperation(operation, [
      {
        slot: "text",
        selector: { label: "utterance", role: "", slot: "", type: "TEXT" },
      },
    ]);

    expect(synced.map((binding) => binding.slot)).toEqual(["text", "voice"]);
    expect(synced[0]?.selector.label).toBe("utterance");
    expect(synced[1]?.selector).toEqual({
      label: "",
      role: "",
      slot: "",
      type: "",
    });
  });
});
