import { describe, expect, expectTypeOf, it } from "vitest";
import customerSupportRecording from "../examples/customer-support.factory-recording.v1.json";
import customerSupportLayout from "../examples/customer-support.factory-visualization-layout.v1.json";

import {
  type FactoryDefinition,
  type FactoryVisualizationLayoutV1,
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
      diagnostics: [],
    });
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

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: The note safety matrix keeps related content guards together.
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
    ["indented code blocks", "    const x = run()"],
    ["thematic breaks", "***"],
    ["GFM pipe tables", "a | b\n--- | ---\nx | y"],
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
      expect(result.diagnostics).toEqual(
        expect.arrayContaining([
          expect.objectContaining({
            code: "unsupported_field",
            path: ["annotations", 0, "callback"],
          }),
          expect.objectContaining({
            code: "unsupported_field",
            path: ["annotations", 0, "connection"],
          }),
        ]),
      );
    }
  });
});

describe("Factory visualization plain-data boundary", () => {
  it("rejects inherited executable data and returns a fresh plain result", () => {
    const inheritedInput = Object.create({
      schemaVersion: "factory-visualization-layout/v1",
      callback: () => "ran",
    }) as Record<string, unknown>;
    inheritedInput.annotations = [];

    expect(
      safeParseFactoryVisualizationLayout(inheritedInput, exampleFactory()),
    ).toEqual({
      success: false,
      issues: [
        {
          category: "structure",
          code: "non_plain_data",
          path: [],
          message:
            "Expected plain data with standard containers and own data properties.",
        },
      ],
      diagnostics: [],
    });

    const input = exampleLayout();
    const result = safeParseFactoryVisualizationLayout(input, exampleFactory());
    expect(result).toMatchObject({ success: true });
    if (result.success) {
      expect(result.data).not.toBe(input);
      expect(Object.getPrototypeOf(result.data)).toBe(Object.prototype);
    }
  });

  it("rejects behavior-bearing nested records before reading accessors", () => {
    const input = exampleLayout();
    const annotations = input.annotations as Record<string, unknown>[];
    const note = annotations[0];
    if (!note) throw new Error("Expected example note annotation.");

    let getterCalled = false;
    const position = Object.create({ callback: () => "ran" }) as Record<
      string,
      unknown
    >;
    Object.defineProperty(position, "x", {
      enumerable: true,
      get: () => {
        getterCalled = true;
        return 0;
      },
    });
    position.y = 0;
    note.position = position;

    const result = safeParseFactoryVisualizationLayout(input, exampleFactory());

    expect(result).toEqual({
      success: false,
      issues: [
        expect.objectContaining({
          code: "non_plain_data",
          path: ["annotations", 0, "position"],
        }),
      ],
      diagnostics: [],
    });
    expect(getterCalled).toBe(false);
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
