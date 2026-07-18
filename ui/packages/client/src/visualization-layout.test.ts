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
        {
          nodeId: "work-state:support-request:resolved",
          content: { kind: "image" },
        },
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

describe("Factory visualization note content safety", () => {
  it("preserves valid multiline notes and accepts inclusive geometry boundaries", () => {
    const input = exampleLayout();
    input.annotations = [
      {
        id: "boundary-note",
        kind: "note",
        position: { x: -100_000, y: 100_000 },
        size: { width: 0.01, height: 10_000 },
        title: "Shift handoff",
        body: "First authored line\nSecond authored line",
        tone: "warning",
      },
      {
        id: "boundary-image",
        kind: "image",
        position: { x: 100_000, y: -100_000 },
        size: { width: 10_000, height: 0.01 },
        altText: "Support mark",
        source: {
          kind: "embedded",
          mediaType: "image/png",
          base64: "iVBORw0KGgo=",
        },
      },
    ];

    const result = safeParseFactoryVisualizationLayout(input, exampleFactory());

    expect(result).toMatchObject({ success: true });
    if (result.success) {
      expect(result.data.annotations?.[0]).toMatchObject({
        body: "First authored line\nSecond authored line",
      });
    }
  });
});

describe("Factory visualization note content diagnostics", () => {
  it.each([
    ["blank body", "   ", "empty_text"],
    ["overlong body", "a".repeat(4_001), "text_too_long"],
    ["HTML", "Use <strong>priority</strong> handling", "unsafe_html"],
    ["Markdown", "See [runbook](docs/runbook)", "unsafe_markdown"],
    ["URI", "Open https://example.com/runbook", "unsafe_uri"],
  ])("rejects %s with a content-specific diagnostic", (_, body, code) => {
    const input = exampleLayout();
    input.annotations = [
      {
        id: "unsafe-note",
        kind: "note",
        position: { x: 0, y: 0 },
        body,
      },
    ];

    const result = safeParseFactoryVisualizationLayout(input, exampleFactory());

    expect(result).toMatchObject({ success: false });
    if (!result.success) {
      expect(result.issues).toContainEqual(
        expect.objectContaining({
          code,
          path: ["annotations", 0, "body"],
        }),
      );
    }
  });

  it.each([
    ["asterisk emphasis", "Use *priority* handling"],
    ["underscore emphasis", "Use _priority_ handling"],
    ["Setext headings", "Heading\n---"],
    ["reference-style links", "[runbook][ops]"],
  ])("rejects %s in note bodies as Markdown", (_, body) => {
    const input = exampleLayout();
    input.annotations = [
      {
        id: "markdown-note",
        kind: "note",
        position: { x: 0, y: 0 },
        body,
      },
    ];

    const result = safeParseFactoryVisualizationLayout(input, exampleFactory());

    expect(result).toMatchObject({ success: false });
    if (!result.success) {
      expect(result.issues).toContainEqual(
        expect.objectContaining({
          code: "unsafe_markdown",
          path: ["annotations", 0, "body"],
        }),
      );
    }
  });

  it("rejects overlong and unsafe titles plus unsupported tones", () => {
    const input = exampleLayout();
    input.annotations = [
      {
        id: "invalid-note",
        kind: "note",
        position: { x: 0, y: 0 },
        title: `<img src="x">${"a".repeat(161)}`,
        body: "Plain body",
        tone: "critical",
      },
    ];

    const result = safeParseFactoryVisualizationLayout(input, exampleFactory());

    expect(result).toMatchObject({ success: false });
    if (!result.success) {
      expect(result.issues).toEqual(
        expect.arrayContaining([
          expect.objectContaining({
            code: "text_too_long",
            path: ["annotations", 0, "title"],
          }),
          expect.objectContaining({
            code: "unsafe_html",
            path: ["annotations", 0, "title"],
          }),
          expect.objectContaining({
            code: "unsupported_note_tone",
            path: ["annotations", 0, "tone"],
          }),
        ]),
      );
    }
  });

  it("rejects executable and connection-like note fields with content-specific diagnostics", () => {
    const input = exampleLayout();
    input.annotations = [
      {
        id: "active-note",
        kind: "note",
        position: { x: 0, y: 0 },
        body: "Plain body",
        callback: "run",
        connection: "workstation:triage",
      },
    ];

    const result = safeParseFactoryVisualizationLayout(input, exampleFactory());

    expect(result).toMatchObject({ success: false });
    if (!result.success) {
      expect(
        result.issues.filter(({ code }) => code === "unsafe_content_field"),
      ).toEqual([
        expect.objectContaining({ path: ["annotations", 0, "callback"] }),
        expect.objectContaining({ path: ["annotations", 0, "connection"] }),
      ]);
    }
  });
});

describe("Factory visualization invalid annotation geometry", () => {
  it.each([
    ["NaN x", { x: Number.NaN, y: 0 }, ["position", "x"]],
    ["infinite y", { x: 0, y: Number.POSITIVE_INFINITY }, ["position", "y"]],
    ["low x", { x: -100_001, y: 0 }, ["position", "x"]],
    ["high y", { x: 0, y: 100_001 }, ["position", "y"]],
  ])("rejects %s", (_, position, pathSuffix) => {
    const input = exampleLayout();
    input.annotations = [
      { id: "bad-position", kind: "note", position, body: "Plain body" },
    ];

    const result = safeParseFactoryVisualizationLayout(input, exampleFactory());

    expect(result).toMatchObject({ success: false });
    if (!result.success) {
      expect(result.issues).toContainEqual(
        expect.objectContaining({
          code: "invalid_coordinate",
          path: ["annotations", 0, ...pathSuffix],
        }),
      );
    }
  });

  it.each([
    ["zero width", { width: 0, height: 1 }, "width"],
    ["negative height", { width: 1, height: -1 }, "height"],
    ["oversized width", { width: 10_001, height: 1 }, "width"],
    [
      "infinite height",
      { width: 1, height: Number.NEGATIVE_INFINITY },
      "height",
    ],
  ])("rejects %s", (_, size, dimension) => {
    const input = exampleLayout();
    input.annotations = [
      {
        id: "bad-size",
        kind: "note",
        position: { x: 0, y: 0 },
        size,
        body: "Plain body",
      },
    ];

    const result = safeParseFactoryVisualizationLayout(input, exampleFactory());

    expect(result).toMatchObject({ success: false });
    if (!result.success) {
      expect(result.issues).toContainEqual(
        expect.objectContaining({
          code: "invalid_dimension",
          path: ["annotations", 0, "size", dimension],
        }),
      );
    }
  });
});
