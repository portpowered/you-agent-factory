import { describe, expect, it } from "vitest";
import customerSupportRecording from "../examples/customer-support.factory-recording.v1.json";
import {
  type FactoryDefinition,
  safeParseFactoryVisualizationLayout,
} from "./index.js";

function exampleFactory(): FactoryDefinition {
  return structuredClone(customerSupportRecording.factory) as FactoryDefinition;
}

describe("Factory visualization plain-data projection", () => {
  it("rejects non-enumerable required fields instead of returning malformed data", () => {
    const input: Record<string, unknown> = {};
    Object.defineProperty(input, "schemaVersion", {
      value: "factory-visualization-layout/v1",
      enumerable: false,
    });
    Object.defineProperty(input, "annotations", {
      value: [],
      enumerable: false,
    });

    expect(
      safeParseFactoryVisualizationLayout(input, exampleFactory()),
    ).toEqual({
      success: false,
      issues: [
        expect.objectContaining({
          code: "non_plain_data",
          path: ["schemaVersion"],
        }),
        expect.objectContaining({
          code: "non_plain_data",
          path: ["annotations"],
        }),
      ],
    });

    const validResult = safeParseFactoryVisualizationLayout(
      {
        schemaVersion: "factory-visualization-layout/v1",
        annotations: [],
      },
      exampleFactory(),
    );
    expect(validResult).toMatchObject({
      success: true,
      data: { schemaVersion: "factory-visualization-layout/v1" },
    });
  });
});
