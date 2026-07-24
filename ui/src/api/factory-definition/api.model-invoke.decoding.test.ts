import { FactoryDefinitionAPIError, normalizeFactoryDefinition } from "./api";
import {
  decodeModelOperation,
  decodeWorkstationOperationBinding,
} from "./api.model-invoke";

describe("normalizeFactoryDefinition model invoke decoding", () => {
  it("decodes all supported work content part shapes in bindings", () => {
    const normalized = normalizeFactoryDefinition({
      name: "content-factory",
      workstations: [
        {
          name: "invoke-all-content",
          type: "MODEL_INVOKE",
          worker: "worker",
          operation: "RUN",
          operationBindings: [
            {
              slot: "text",
              selector: {
                slot: "text",
                label: "utterance",
                role: "input",
                type: "TEXT",
              },
              config: [
                {
                  type: "text",
                  text: "hello",
                  slot: "text",
                  label: "copy",
                  role: "input",
                  contentType: "TEXT",
                  artifactId: "artifact-1",
                  metadata: { source: "config" },
                },
                {
                  type: "image",
                  url: "https://example.com/a.png",
                  file: "a.png",
                },
                { type: "IMAGE", url: "https://example.com/b.png" },
                {
                  type: "AUDIO",
                  url: "https://example.com/a.mp3",
                  file: "a.mp3",
                },
                { type: "JSON", json: { voice: "alloy" } },
                {
                  type: "BINARY",
                  url: "https://example.com/a.bin",
                  file: "a.bin",
                },
              ],
            },
          ],
          inputs: [{ state: "init", workType: "story" }],
        },
      ],
    });

    expect(normalized.workstations?.[0]?.operationBindings).toEqual([
      {
        slot: "text",
        selector: {
          slot: "text",
          label: "utterance",
          role: "input",
          type: "TEXT",
        },
        config: [
          {
            type: "text",
            text: "hello",
            slot: "text",
            label: "copy",
            role: "input",
            contentType: "TEXT",
            artifactId: "artifact-1",
            metadata: { source: "config" },
          },
          { type: "image", url: "https://example.com/a.png", file: "a.png" },
          { type: "IMAGE", url: "https://example.com/b.png" },
          { type: "AUDIO", url: "https://example.com/a.mp3", file: "a.mp3" },
          { type: "JSON", json: { voice: "alloy" } },
          { type: "BINARY", url: "https://example.com/a.bin", file: "a.bin" },
        ],
      },
    ]);
  });

  it("rejects unsupported work content part types", () => {
    expect(() =>
      normalizeFactoryDefinition({
        name: "invalid-factory",
        workstations: [
          {
            name: "invoke",
            type: "MODEL_INVOKE",
            worker: "worker",
            operationBindings: [
              {
                slot: "text",
                config: [{ type: "VIDEO", url: "https://example.com/a.mp4" }],
              },
            ],
            inputs: [{ state: "init", workType: "story" }],
          },
        ],
      }),
    ).toThrow(
      new FactoryDefinitionAPIError(
        "factory.workstations[0].operationBindings[0].config[0].type must be one of text, TEXT, image, IMAGE, AUDIO, JSON, BINARY.",
      ),
    );
  });
});

describe("decodeModelOperation", () => {
  it("decodes operation names with optional input and output slots", () => {
    expect(
      decodeModelOperation(
        {
          name: "TTS",
          inputs: [{ name: "text", contentTypes: ["TEXT"], required: true }],
          outputs: [{ name: "audio", contentTypes: ["AUDIO"] }],
        },
        "factory.workers[0].operations[0]",
      ),
    ).toEqual({
      name: "TTS",
      inputs: [{ name: "text", contentTypes: ["TEXT"], required: true }],
      outputs: [{ name: "audio", contentTypes: ["AUDIO"] }],
    });
  });

  it("rejects unknown operation slot keys", () => {
    expect(() =>
      decodeModelOperation(
        {
          name: "TTS",
          inputs: [{ name: "text", contentTypes: ["TEXT"], unexpected: true }],
        },
        "factory.workers[0].operations[0]",
      ),
    ).toThrow(
      new FactoryDefinitionAPIError(
        "factory.workers[0].operations[0].inputs[0].unexpected is not allowed by the generated factory contract.",
      ),
    );
  });
});

describe("decodeWorkstationOperationBinding", () => {
  it("decodes selector-only bindings", () => {
    expect(
      decodeWorkstationOperationBinding(
        {
          slot: "voice",
          selector: {
            slot: "voice",
            label: "voice",
            role: "config",
            type: "JSON",
          },
        },
        "factory.workstations[0].operationBindings[0]",
      ),
    ).toEqual({
      slot: "voice",
      selector: { slot: "voice", label: "voice", role: "config", type: "JSON" },
    });
  });
});
