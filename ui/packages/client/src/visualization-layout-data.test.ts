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

  it("returns structured diagnostics for cyclic and deeply nested input", () => {
    const cyclic: Record<string, unknown> = {
      schemaVersion: "factory-visualization-layout/v1",
      annotations: [],
    };
    cyclic.self = cyclic;

    expect(() =>
      safeParseFactoryVisualizationLayout(cyclic, exampleFactory()),
    ).not.toThrow();
    expect(
      safeParseFactoryVisualizationLayout(cyclic, exampleFactory()),
    ).toEqual({
      success: false,
      issues: [
        expect.objectContaining({
          code: "non_plain_data",
          path: ["self"],
        }),
      ],
    });

    const deeplyNested: Record<string, unknown> = {};
    let cursor = deeplyNested;
    for (let depth = 0; depth < 10_000; depth += 1) {
      const child: Record<string, unknown> = {};
      cursor.child = child;
      cursor = child;
    }
    const deepInput = {
      schemaVersion: "factory-visualization-layout/v1",
      annotations: [],
      extra: deeplyNested,
    };
    expect(() =>
      safeParseFactoryVisualizationLayout(deepInput, exampleFactory()),
    ).not.toThrow();
    expect(
      safeParseFactoryVisualizationLayout(deepInput, exampleFactory()),
    ).toMatchObject({ success: false });
  });

  it.each([
    ["position", "x", { y: 0 }],
    ["size", "width", { height: 1 }],
  ])(
    "rejects missing own %s.%s when Object.prototype supplies the field",
    (shape, inheritedField, geometry) => {
      const annotation = {
        id: "inherited-geometry",
        kind: "note",
        position: shape === "position" ? geometry : { x: 0, y: 0 },
        ...(shape === "size" ? { size: geometry } : {}),
        body: "Plain body",
      };
      const input = {
        schemaVersion: "factory-visualization-layout/v1",
        annotations: [annotation],
      };
      Object.defineProperty(Object.prototype, inheritedField, {
        value: 1,
        configurable: true,
      });
      try {
        const result = safeParseFactoryVisualizationLayout(
          input,
          exampleFactory(),
        );
        expect(result).toMatchObject({ success: false });
        if (!result.success) {
          expect(result.issues).toContainEqual(
            expect.objectContaining({
              code: "missing_required_field",
              path: ["annotations", 0, shape, inheritedField],
            }),
          );
        }
      } finally {
        delete (Object.prototype as Record<string, unknown>)[inheritedField];
      }
    },
  );
});
