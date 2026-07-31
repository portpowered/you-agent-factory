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
          modelProvider: "CODEX",
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
          modelProvider: "CODEX",
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
      contentTypes: [ModelOperationContentType.TEXT],
      name: "text",
      required: true,
    };
    operation.outputs[0] = {
      contentTypes: [ModelOperationContentType.AUDIO],
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
        modelProvider: "CODEX",
        name: "tts-worker",
        operations: [operation],
        provider: "",
        workerType: "INFERENCE_WORKER",
      },
    );

    expect(nextDraft.additions.workers).toEqual([
      {
        modelProvider: "CODEX",
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

describe("factory graph editor additions taxonomy", () => {
  it("persists agent workers and poller workers with their public taxonomy types", () => {
    const agentDraft = applyFactoryGraphAddEntityDraft(
      createEmptyFactoryGraphDraft(),
      {
        argsText: "",
        command: "",
        kind: "worker",
        model: "",
        modelProvider: "CODEX",
        name: "reviewer",
        operations: [],
        provider: "",
        workerType: "AGENT_WORKER",
      },
    );
    expect(agentDraft.additions.workers).toEqual([
      {
        modelProvider: "CODEX",
        name: "reviewer",
        type: "AGENT_WORKER",
      },
    ]);

    const pollerDraft = applyFactoryGraphAddEntityDraft(
      createEmptyFactoryGraphDraft(),
      {
        argsText: "",
        command: "",
        kind: "worker",
        model: "",
        modelProvider: "",
        name: "linear-poller",
        operations: [],
        provider: "LINEAR",
        workerType: "POLLER_WORKER",
      },
    );
    expect(pollerDraft.additions.workers).toEqual([
      {
        name: "linear-poller",
        provider: "LINEAR",
        type: "POLLER_WORKER",
      },
    ]);
  });

  it("rejects poller workers without a hosted provider", () => {
    expect(
      validateFactoryGraphAddEntityDraft(
        {
          argsText: "",
          command: "",
          kind: "worker",
          model: "",
          modelProvider: "",
          name: "linear-poller",
          operations: [],
          provider: "",
          workerType: "POLLER_WORKER",
        },
        baseFactoryDefinition,
      ),
    ).toEqual({
      provider: "Select a hosted provider for the new poller worker.",
    });
  });
});
