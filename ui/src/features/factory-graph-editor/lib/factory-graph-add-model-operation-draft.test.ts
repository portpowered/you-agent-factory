import { ModelOperationContentType } from "../../../api/generated/openapi";
import {
  buildCanonicalModelOperationsFromDraft,
  createEmptyFactoryGraphAddModelOperationDraft,
  validateFactoryGraphAddModelOperationsDraft,
} from "./factory-graph-add-model-operation-draft";

describe("factory graph add model operation draft", () => {
  it("requires at least one operation for model worker contracts", () => {
    expect(validateFactoryGraphAddModelOperationsDraft([])).toEqual({
      summary: "Add at least one model-invocation operation.",
    });
  });

  it("validates uppercase operation names, slot uniqueness, and required slot contracts", () => {
    expect(
      validateFactoryGraphAddModelOperationsDraft([
        {
          inputs: [
            {
              contentTypes: [
                ModelOperationContentType.ModelOperationContentTypeText,
              ],
              name: "text",
              required: true,
            },
          ],
          name: "tts",
          outputs: [
            {
              contentTypes: [
                ModelOperationContentType.ModelOperationContentTypeAudio,
              ],
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
              contentTypes: [
                ModelOperationContentType.ModelOperationContentTypeText,
              ],
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
              contentTypes: [
                ModelOperationContentType.ModelOperationContentTypeText,
              ],
              name: "text",
              required: true,
            },
          ],
          name: "TTS",
          outputs: [
            {
              contentTypes: [
                ModelOperationContentType.ModelOperationContentTypeAudio,
              ],
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

  it("reports empty operation names and missing slot lists", () => {
    expect(
      validateFactoryGraphAddModelOperationsDraft([
        {
          inputs: [],
          name: "   ",
          outputs: [],
        },
      ]),
    ).toMatchObject({
      byIndex: {
        0: {
          inputs: "Add at least one input slot for each operation.",
          name: "Enter an uppercase operation name.",
          outputs: "Add at least one output slot for each operation.",
        },
      },
    });
  });

  it("builds canonical model operations from validated drafts", () => {
    const operation = createEmptyFactoryGraphAddModelOperationDraft();
    operation.name = "TTS";
    operation.inputs[0] = {
      contentTypes: [ModelOperationContentType.ModelOperationContentTypeText],
      name: "text",
      required: true,
    };
    operation.outputs[0] = {
      contentTypes: [ModelOperationContentType.ModelOperationContentTypeAudio],
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
