import { describe, expect, it } from "bun:test";

import type { components } from "./openapi";

type GenericRequest = components["schemas"]["GenericModelInvocationRequest"];
type GenericResponse = components["schemas"]["GenericModelInvocationResponse"];
type GlobalConfig = components["schemas"]["GlobalConfig"];

describe("generated generic model invocation OpenAPI types", () => {
  it("supports ordered multimodal values, named outputs, and model overlays", () => {
    const request: GenericRequest = {
      scope: "scope-ui-001",
      holder: "generated-ui",
      model: { nameOrUri: "llm" },
      operation: "OMNI",
      inputs: [
        { name: "prompt", modality: "TEXT", content: "compare" },
        { name: "image", modality: "IMAGE", mediaType: "image/png", content: "first" },
        { name: "image", modality: "IMAGE", mediaType: "image/jpeg", content: "second" },
      ],
      parameters: [{ name: "temperature", value: 0.2 }],
      outputMode: "JSON",
      offline: true,
    };
    const response: GenericResponse = {
      outputs: [
        { name: "transcript", modality: "TEXT", content: "hello" },
        { name: "segments", modality: "JSON", artifact: { artifactRef: "artifact:segments" } },
      ],
    };
    const config: GlobalConfig = {
      models: {
        llm: { backend: "localai-llamacpp", operations: ["OMNI"] },
        custom: {
          source: "hf://example/custom.gguf",
          backend: "localai-llamacpp",
          loadPolicy: "ON_DEMAND",
          operations: ["EMBED"],
        },
      },
    };

    expect(JSON.stringify(request)).toContain('"name":"image"');
    expect(response.outputs.map((output) => output.name)).toEqual(["transcript", "segments"]);
    expect(config.models?.custom?.operations).toEqual(["EMBED"]);
  });
});
