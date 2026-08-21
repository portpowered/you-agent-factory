import { describe, expect, it } from "vitest";
import customerSupportRecording from "../examples/customer-support.factory-recording.v1.json";
import customerSupportLayout from "../examples/customer-support.factory-visualization-layout.v1.json";

import {
  type FactoryDefinition,
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

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: The structural suite keeps tolerant and blocking layout guards together.
describe("Factory visualization layout structural diagnostics", () => {
  it("accepts unknown fields at each contract boundary with exact-path diagnostics", () => {
    const input = exampleLayout();
    input.futureMetadata = { revision: 2 };
    const annotations = input.annotations as Record<string, unknown>[];
    const [note, image] = annotations;
    if (!note || !image) throw new Error("Expected example annotations.");
    note.futureMetadata = { authoringTool: "new-dashboard" };
    (image.source as Record<string, unknown>).futureMetadata = {
      checksum: "known-only-to-newer-ui",
    };
    const emptyStates = input.nodeEmptyStates as Record<string, unknown>[];
    const textEmptyState = emptyStates[0];
    if (!textEmptyState) throw new Error("Expected example empty state.");
    (textEmptyState.content as Record<string, unknown>).futureMetadata = {
      source: "new-dashboard",
    };

    const result = safeParseFactoryVisualizationLayout(input, exampleFactory());

    expect(result).toMatchObject({
      success: true,
      data: {
        annotations: [
          { id: "triage-guidance" },
          { source: { mediaType: "image/png" } },
        ],
        nodeEmptyStates: [
          { nodeId: "workstation:triage", content: { kind: "text" } },
          {
            nodeId: "work-state:support-request:resolved",
            content: { kind: "image" },
          },
        ],
      },
    });
    if (result.success) {
      expect(result.diagnostics).toEqual(
        expect.arrayContaining([
          expect.objectContaining({
            code: "unsupported_field",
            path: ["futureMetadata"],
          }),
          expect.objectContaining({
            code: "unsupported_field",
            path: ["annotations", 0, "futureMetadata"],
          }),
          expect.objectContaining({
            code: "unsupported_field",
            path: ["annotations", 1, "source", "futureMetadata"],
          }),
          expect.objectContaining({
            code: "unsupported_field",
            path: ["nodeEmptyStates", 0, "content", "futureMetadata"],
          }),
        ]),
      );
    }
    expect(() =>
      parseFactoryVisualizationLayout(input, exampleFactory()),
    ).not.toThrow();
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

  it("keeps known type failures blocking while retaining additive diagnostics", () => {
    const input = exampleLayout();
    input.annotations = [
      {
        id: "wrong-body-type",
        kind: "note",
        position: { x: 0, y: 0 },
        body: 42,
        futureMetadata: "private-value-not-in-diagnostic",
      },
    ];

    const result = safeParseFactoryVisualizationLayout(input, exampleFactory());

    expect(result).toMatchObject({ success: false });
    if (!result.success) {
      expect(result.issues).toContainEqual(
        expect.objectContaining({
          code: "invalid_type",
          path: ["annotations", 0, "body"],
        }),
      );
      expect(result.diagnostics).toContainEqual(
        expect.objectContaining({
          code: "unsupported_field",
          path: ["annotations", 0, "futureMetadata"],
        }),
      );
      expect(result.diagnostics[0]?.message).not.toContain(
        "private-value-not-in-diagnostic",
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
