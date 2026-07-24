import { ModelOperationContentType } from "../../../api/generated/openapi";
import {
  buildCanonicalModelOperationsFromDraft,
  createEmptyFactoryGraphAddModelOperationDraft,
  validateFactoryGraphAddModelOperationsDraft,
} from "./factory-graph-add-model-operation-draft";

describe("factory graph add model operation draft", () => {
  it("allows model workers without model operation contracts", () => {
    expect(validateFactoryGraphAddModelOperationsDraft([])).toEqual({});
  });

  it("validates uppercase operation names, slot uniqueness, and required slot contracts", () => {
    expect(
      validateFactoryGraphAddModelOperationsDraft([
        {
          inputs: [
            {
              contentTypes: [ModelOperationContentType.TEXT],
              name: "text",
              required: true,
            },
          ],
          name: "tts",
          outputs: [
            {
              contentTypes: [ModelOperationContentType.AUDIO],
              name: "audio",
              required: false,
            },
          ],
        },
      ]),
    ).toMatchObject({
      byIndex: {
        0: {
          name: "Operation names must be uppercase letters, digits, or underscores.",
        },
      },
    });

    expect(
      validateFactoryGraphAddModelOperationsDraft([
        {
          inputs: [
            {
              contentTypes: [],
              name: "text",
              required: true,
            },
            {
              contentTypes: [ModelOperationContentType.TEXT],
              name: "text",
              required: false,
            },
          ],
          name: "TTS",
          outputs: [
            {
              contentTypes: [],
              name: "audio",
              required: false,
            },
          ],
        },
        {
          inputs: [
            {
              contentTypes: [ModelOperationContentType.TEXT],
              name: "text",
              required: true,
            },
          ],
          name: "TTS",
          outputs: [
            {
              contentTypes: [ModelOperationContentType.AUDIO],
              name: "audio",
              required: false,
            },
          ],
        },
      ]),
    ).toMatchObject({
      byIndex: {
        0: {
          inputSlots: {
            0: {
              contentTypes: "Select at least one content type.",
            },
            1: {
              name: 'Slot name "text" is already used in this input list.',
            },
          },
          outputSlots: {
            0: {
              contentTypes: "Select at least one content type.",
            },
          },
        },
        1: {
          name: 'Operation name "TTS" is already used on this worker.',
        },
      },
    });
  });

  it("builds canonical model operations from validated drafts", () => {
    const operation = createEmptyFactoryGraphAddModelOperationDraft();
    operation.name = "TTS";
    operation.inputs[0] = {
      contentTypes: [ModelOperationContentType.TEXT],
      name: "text",
      required: true,
    };
    operation.outputs[0] = {
      contentTypes: [ModelOperationContentType.AUDIO],
      name: "audio",
      required: false,
    };

    expect(buildCanonicalModelOperationsFromDraft([operation])).toEqual([
      {
        inputs: [
          {
            contentTypes: ["TEXT"],
            name: "text",
            required: true,
          },
        ],
        name: "TTS",
        outputs: [
          {
            contentTypes: ["AUDIO"],
            name: "audio",
          },
        ],
      },
    ]);
    expect(buildCanonicalModelOperationsFromDraft([])).toBeUndefined();
  });
});
