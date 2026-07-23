import { FactoryDefinitionAPIError, normalizeFactoryDefinition } from "./api";

const modelInvokeFactoryPayload = {
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
          config: [{ type: "text", text: "fallback copy" }],
          defaultContent: [{ type: "TEXT", text: "default copy" }],
        },
      ],
      inputs: [{ state: "init", workType: "story" }],
      outputs: [{ state: "complete", workType: "story" }],
    },
  ],
  workTypes: [],
};

describe("normalizeFactoryDefinition model invoke contract", () => {
  it("accepts worker operations and MODEL_INVOKE workstation fields", () => {
    const normalized = normalizeFactoryDefinition(modelInvokeFactoryPayload);

    expect(normalized.workers?.[0]?.operations).toEqual([
      {
        name: "TTS",
        inputs: [
          { name: "text", contentTypes: ["TEXT"], required: true },
          { name: "voice", contentTypes: ["JSON"] },
        ],
        outputs: [{ name: "audio", contentTypes: ["AUDIO"] }],
      },
    ]);
    expect(normalized.workstations?.[0]).toMatchObject({
      type: "MODEL_INVOKE",
      operation: "TTS",
      operationBindings: [
        {
          slot: "text",
          selector: { label: "utterance", type: "TEXT" },
          config: [{ type: "text", text: "fallback copy" }],
          defaultContent: [{ type: "TEXT", text: "default copy" }],
        },
      ],
    });
    expect(
      normalizeFactoryDefinition(JSON.parse(JSON.stringify(normalized))),
    ).toEqual(normalized);
  });

  it("rejects unknown worker operation keys", () => {
    expect(() =>
      normalizeFactoryDefinition({
        name: "invalid-factory",
        workers: [
          {
            name: "tts-worker",
            operations: [{ name: "TTS", unexpected: true }],
          },
        ],
      }),
    ).toThrow(
      new FactoryDefinitionAPIError(
        "factory.workers[0].operations[0].unexpected is not allowed by the generated factory contract.",
      ),
    );
  });

  it("rejects unknown MODEL_INVOKE workstation binding keys", () => {
    expect(() =>
      normalizeFactoryDefinition({
        name: "invalid-factory",
        workstations: [
          {
            name: "speak-story",
            type: "MODEL_INVOKE",
            worker: "tts-worker",
            operationBindings: [{ slot: "text", unexpected: true }],
            inputs: [{ state: "init", workType: "story" }],
          },
        ],
      }),
    ).toThrow(
      new FactoryDefinitionAPIError(
        "factory.workstations[0].operationBindings[0].unexpected is not allowed by the generated factory contract.",
      ),
    );
  });
});
