import { describe, expect, it } from "vitest";

import type { CanonicalFactoryDefinition } from "../../../../api/current-factory-definition";
import {
  buildCanonicalModelInvokeBindingsFromDraft,
  editableModelInvokeBindingsEqual,
  isModelInvokeWorkstationType,
  modelInvokeBindingDraftHasContent,
  resolveCompatibleModelWorkerNames,
  resolveEditableModelInvokeBindings,
  resolveModelOperationByName,
  resolveModelOperationsByWorkerName,
  resolveModelWorkerOperations,
  syncEditableModelInvokeBindingsForOperation,
  validateEditableModelInvokeBindings,
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

describe("workstation model invoke type helpers", () => {
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
    expect(
      resolveModelWorkerOperations(modelInvokeFactory, "reviewer"),
    ).toEqual([]);
    expect(resolveModelOperationsByWorkerName(modelInvokeFactory)).toEqual({
      "tts-worker": resolveModelWorkerOperations(
        modelInvokeFactory,
        "tts-worker",
      ),
      reviewer: [],
    });
    expect(
      resolveModelOperationByName(
        resolveModelWorkerOperations(modelInvokeFactory, "tts-worker"),
        "TTS",
      )?.name,
    ).toBe("TTS");
    expect(
      resolveModelOperationByName(
        resolveModelWorkerOperations(modelInvokeFactory, "tts-worker"),
        "MISSING",
      ),
    ).toBeUndefined();
  });
});

describe("workstation model invoke binding projection", () => {
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
        configText: "",
        defaultContentText: "",
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

  it("round-trips config and default content bindings", () => {
    const bindings = resolveEditableModelInvokeBindings([
      {
        slot: "text",
        config: [{ text: "static copy", type: "text" }],
        defaultContent: [{ text: "fallback copy", type: "text" }],
      },
    ]);

    expect(bindings[0]).toMatchObject({
      configText: "static copy",
      defaultContentText: "fallback copy",
    });
    expect(buildCanonicalModelInvokeBindingsFromDraft(bindings)).toEqual([
      {
        slot: "text",
        config: [{ text: "static copy", type: "text" }],
        defaultContent: [{ text: "fallback copy", type: "text" }],
      },
    ]);
  });
});

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: binding validation cases share one operation fixture.
describe("workstation model invoke binding validation", () => {
  it("omits empty optional bindings from canonical projection", () => {
    const operation = resolveModelWorkerOperations(
      modelInvokeFactory,
      "tts-worker",
    )[0];
    const bindings = syncEditableModelInvokeBindingsForOperation(operation, [
      {
        slot: "text",
        configText: "",
        defaultContentText: "",
        selector: { label: "utterance", role: "", slot: "", type: "TEXT" },
      },
    ]);

    expect(buildCanonicalModelInvokeBindingsFromDraft(bindings)).toEqual([
      {
        slot: "text",
        selector: { label: "utterance", type: "TEXT" },
      },
    ]);
    expect(
      modelInvokeBindingDraftHasContent(
        bindings[1] as NonNullable<(typeof bindings)[1]>,
      ),
    ).toBe(false);
  });

  it("accepts valid model-invoke bindings without validation errors", () => {
    const operation = resolveModelWorkerOperations(
      modelInvokeFactory,
      "tts-worker",
    )[0];
    const messages = {
      bindingDuplicate: (slotName: string) => `duplicate ${slotName}`,
      bindingRequired: (slotName: string) => `required ${slotName}`,
      bindingSummary: "fix bindings",
    };

    expect(
      validateEditableModelInvokeBindings(
        [
          {
            slot: "text",
            configText: "hello",
            defaultContentText: "",
            selector: { label: "", role: "", slot: "", type: "" },
          },
        ],
        operation,
        messages,
      ),
    ).toEqual({});
    expect(
      modelInvokeBindingDraftHasContent({
        slot: "text",
        configText: "",
        defaultContentText: "",
        selector: { label: "utterance", role: "", slot: "", type: "TEXT" },
      }),
    ).toBe(true);
  });

  it("validates required slots and duplicate bindings", () => {
    const operation = resolveModelWorkerOperations(
      modelInvokeFactory,
      "tts-worker",
    )[0];
    const messages = {
      bindingDuplicate: (slotName: string) => `duplicate ${slotName}`,
      bindingRequired: (slotName: string) => `required ${slotName}`,
      bindingSummary: "fix bindings",
    };

    expect(
      validateEditableModelInvokeBindings(
        [
          {
            slot: "text",
            configText: "",
            defaultContentText: "",
            selector: { label: "", role: "", slot: "", type: "" },
          },
        ],
        operation,
        messages,
      ),
    ).toMatchObject({
      "operationBindings[text]": "required text",
      operationBindings: "fix bindings",
    });

    expect(
      validateEditableModelInvokeBindings(
        [
          {
            slot: "text",
            configText: "hello",
            defaultContentText: "",
            selector: { label: "", role: "", slot: "", type: "" },
          },
          {
            slot: "text",
            configText: "again",
            defaultContentText: "",
            selector: { label: "", role: "", slot: "", type: "" },
          },
        ],
        operation,
        messages,
      )["operationBindings[text]"],
    ).toBe("duplicate text");
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
    expect(synced[1]).toMatchObject({
      configText: "",
      defaultContentText: "",
    });
  });
});
