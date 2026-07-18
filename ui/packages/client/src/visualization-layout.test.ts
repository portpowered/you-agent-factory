import { describe, expect, expectTypeOf, it } from "vitest";
import customerSupportRecording from "../examples/customer-support.factory-recording.v1.json";
import customerSupportLayout from "../examples/customer-support.factory-visualization-layout.v1.json";

import {
  type FactoryDefinition,
  type FactoryVisualizationLayoutV1,
  FactoryVisualizationLayoutValidationError,
  parseFactoryVisualizationLayout,
  safeParseFactoryVisualizationLayout,
} from "./index.js";

function exampleFactory(): FactoryDefinition {
  return structuredClone(customerSupportRecording.factory) as FactoryDefinition;
}

function exampleLayout(): Record<string, unknown> {
  return structuredClone(customerSupportLayout) as Record<string, unknown>;
}

describe("Factory visualization layout validation", () => {
  it("parses the packaged note, image, and node-empty-state example beside an unchanged Factory", () => {
    const factory = exampleFactory();
    const originalFactory = structuredClone(factory);

    const layout = parseFactoryVisualizationLayout(exampleLayout(), factory);

    expect(layout).toMatchObject({
      schemaVersion: "factory-visualization-layout/v1",
      annotations: [
        { id: "triage-guidance", kind: "note" },
        {
          id: "support-mark",
          kind: "image",
          source: { kind: "embedded", mediaType: "image/png" },
        },
      ],
      nodeEmptyStates: [
        { nodeId: "workstation:triage", content: { kind: "text" } },
        { nodeId: "work-state:resolved", content: { kind: "image" } },
      ],
    });
    expect(factory).toEqual(originalFactory);
    expect(layout).not.toHaveProperty("factory");
    expectTypeOf(layout).toEqualTypeOf<FactoryVisualizationLayoutV1>();
  });

  it("returns a stable diagnostic for an unsupported sidecar version", () => {
    const input = exampleLayout();
    input.schemaVersion = "factory-visualization-layout/v2";

    expect(
      safeParseFactoryVisualizationLayout(input, exampleFactory()),
    ).toEqual({
      success: false,
      issues: [
        {
          category: "structure",
          code: "unsupported_layout_schema_version",
          path: ["schemaVersion"],
          message:
            "Unsupported Factory visualization layout schema version: factory-visualization-layout/v2.",
        },
      ],
    });
  });
});

describe("Factory visualization layout structural diagnostics", () => {
  it("rejects unknown fields at each closed contract boundary", () => {
    const input = exampleLayout();
    input.callback = () => undefined;
    const annotations = input.annotations as Record<string, unknown>[];
    const [note, image] = annotations;
    if (!note || !image) throw new Error("Expected example annotations.");
    note.link = "https://example.com";
    (image.source as Record<string, unknown>).url =
      "https://example.com/image.png";
    const emptyStates = input.nodeEmptyStates as Record<string, unknown>[];
    const textEmptyState = emptyStates[0];
    if (!textEmptyState) throw new Error("Expected example empty state.");
    (textEmptyState.content as Record<string, unknown>).markdown = "**empty**";

    const result = safeParseFactoryVisualizationLayout(input, exampleFactory());

    expect(result).toMatchObject({ success: false });
    if (!result.success) {
      expect(result.issues).toEqual(
        expect.arrayContaining([
          expect.objectContaining({
            code: "unsupported_field",
            path: ["callback"],
          }),
          expect.objectContaining({
            code: "unsupported_field",
            path: ["annotations", 0, "link"],
          }),
          expect.objectContaining({
            code: "unsupported_field",
            path: ["annotations", 1, "source", "url"],
          }),
          expect.objectContaining({
            code: "unsupported_field",
            path: ["nodeEmptyStates", 0, "content", "markdown"],
          }),
        ]),
      );
    }
  });

  it("reports wrong annotation discriminants and missing required fields precisely", () => {
    const input = exampleLayout();
    input.annotations = [
      { id: "wrong-kind", kind: "video", position: { x: 0, y: 0 } },
      { id: "missing-body", kind: "note", position: { x: 0, y: 0 } },
      {
        id: "missing-source",
        kind: "image",
        position: { x: 0, y: 0 },
        size: { width: 10, height: 10 },
        altText: "Missing source",
      },
    ];

    const result = safeParseFactoryVisualizationLayout(input, exampleFactory());

    expect(result).toMatchObject({ success: false });
    if (!result.success) {
      expect(result.issues).toEqual(
        expect.arrayContaining([
          expect.objectContaining({
            code: "invalid_annotation_kind",
            path: ["annotations", 0, "kind"],
          }),
          expect.objectContaining({
            code: "missing_required_field",
            path: ["annotations", 1, "body"],
          }),
          expect.objectContaining({
            code: "missing_required_field",
            path: ["annotations", 2, "source"],
          }),
        ]),
      );
    }
  });

  it("reports every occurrence of a duplicate annotation ID", () => {
    const input = exampleLayout();
    const annotations = input.annotations as Record<string, unknown>[];
    const [firstAnnotation, secondAnnotation] = annotations;
    if (!firstAnnotation || !secondAnnotation) {
      throw new Error("Expected example annotations.");
    }
    secondAnnotation.id = firstAnnotation.id;

    const result = safeParseFactoryVisualizationLayout(input, exampleFactory());

    expect(result).toMatchObject({ success: false });
    if (!result.success) {
      expect(
        result.issues.filter(({ code }) => code === "duplicate_annotation_id"),
      ).toEqual([
        expect.objectContaining({ path: ["annotations", 0, "id"] }),
        expect.objectContaining({ path: ["annotations", 1, "id"] }),
      ]);
    }
  });

  it("throws the public validation error without returning partial data", () => {
    expect(() =>
      parseFactoryVisualizationLayout(
        { schemaVersion: "factory-visualization-layout/v1", annotations: [{}] },
        exampleFactory(),
      ),
    ).toThrowError(FactoryVisualizationLayoutValidationError);
  });
});
