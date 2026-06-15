import type { CanonicalFactoryDefinition } from "../../../api/current-factory-definition";
import { ModelOperationContentType } from "../../../api/generated/openapi";
import { createEmptyFactoryGraphDraft } from "./draft/factory-graph-draft-types";
import {
  applyFactoryGraphAddEntityDraft,
  validateFactoryGraphAddEntityDraft,
} from "./editor/factory-graph-editor-additions";
import { createEmptyFactoryGraphAddModelOperationDraft } from "./factory-graph-add-model-operation-draft";

const baseFactoryDefinition: CanonicalFactoryDefinition = {
  name: "Current Factory",
  workers: [
    {
      model: "gpt-5",
      name: "writer",
      type: "MODEL_WORKER",
    },
  ],
  workTypes: [
    {
      name: "story",
      states: [
        {
          name: "queued",
          type: "INITIAL",
        },
      ],
    },
  ],
  workstations: [
    {
      inputs: [],
      name: "draft",
      outputs: [],
      type: "MODEL_WORKSTATION",
      worker: "writer",
    },
  ],
};

describe("factory graph editor additions model operations", () => {
  it("allows model workers with zero operations", () => {
    expect(
      validateFactoryGraphAddEntityDraft(
        {
          argsText: "",
          command: "",
          kind: "worker",
          model: "",
          modelProvider: "CURSOR",
          name: "tts-worker",
          operations: [],
          provider: "",
          workerType: "INFERENCE_WORKER",
        },
        baseFactoryDefinition,
      ),
    ).toEqual({});
  });

  it("rejects invalid model operation contracts on model workers", () => {
    const invalidOperation = createEmptyFactoryGraphAddModelOperationDraft();
    invalidOperation.name = "tts";

    expect(
      validateFactoryGraphAddEntityDraft(
        {
          argsText: "",
          command: "",
          kind: "worker",
          model: "",
          modelProvider: "CURSOR",
          name: "tts-worker",
          operations: [invalidOperation],
          provider: "",
          workerType: "INFERENCE_WORKER",
        },
        baseFactoryDefinition,
      ),
    ).toMatchObject({
      modelOperations: {
        byIndex: {
          0: {
            name: "Operation names must be uppercase letters, digits, or underscores.",
          },
        },
      },
    });
  });

  it("persists model operations on new model workers when valid", () => {
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

    const nextDraft = applyFactoryGraphAddEntityDraft(
      createEmptyFactoryGraphDraft(),
      {
        argsText: "",
        command: "",
        kind: "worker",
        model: "",
        modelProvider: "CURSOR",
        name: "tts-worker",
        operations: [operation],
        provider: "",
        workerType: "INFERENCE_WORKER",
      },
    );

    expect(nextDraft.additions.workers).toEqual([
      {
        modelProvider: "CURSOR",
        name: "tts-worker",
        operations: [
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
        ],
        type: "INFERENCE_WORKER",
      },
    ]);
  });
});
