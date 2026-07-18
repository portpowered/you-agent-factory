import { describe, expect, it } from "vitest";
import customerSupportRecording from "../examples/customer-support.factory-recording.v1.json";
import customerSupportLayout from "../examples/customer-support.factory-visualization-layout.v1.json";

import {
  type FactoryDefinition,
  safeParseFactoryVisualizationLayout,
} from "./index.js";

function exampleFactory(): FactoryDefinition {
  return structuredClone(customerSupportRecording.factory) as FactoryDefinition;
}

function exampleLayout(): Record<string, unknown> {
  return structuredClone(customerSupportLayout) as Record<string, unknown>;
}

describe("valid Factory visualization node empty states", () => {
  it("accepts text and image content for workstation and work-state topology nodes", () => {
    const factory = exampleFactory();
    const originalFactory = structuredClone(factory);

    expect(
      safeParseFactoryVisualizationLayout(exampleLayout(), factory),
    ).toMatchObject({ success: true });
    expect(factory).toEqual(originalFactory);
  });

  it("uses durable entity IDs from the canonical topology projection", () => {
    const factory: FactoryDefinition = {
      name: "durable-identifiers",
      workTypes: [
        {
          id: "request-id",
          name: "request",
          states: [{ id: "queued-id", name: "queued", type: "INITIAL" }],
        },
      ],
      workstations: [
        {
          id: "triage-id",
          inputs: [{ state: "queued", workType: "request" }],
          name: "triage",
          worker: "",
        },
      ],
    };
    const input = {
      schemaVersion: "factory-visualization-layout/v1",
      nodeEmptyStates: [
        {
          nodeId: "workstation:triage-id",
          content: { kind: "text", text: "No work is waiting." },
        },
        {
          nodeId: "work-state:request-id:queued-id",
          content: { kind: "text", text: "Nothing is queued." },
        },
      ],
    };

    expect(safeParseFactoryVisualizationLayout(input, factory)).toMatchObject({
      success: true,
    });
  });
});

describe("invalid Factory visualization node empty states", () => {
  it.each([
    ["blank text", "   ", "empty_text"],
    ["overlong text", "a".repeat(501), "text_too_long"],
    ["HTML", "No <strong>requests</strong>", "unsafe_html"],
    ["Markdown", "See [queue](queues/current)", "unsafe_markdown"],
    ["URI", "Open https://example.com/queue", "unsafe_uri"],
  ])("rejects %s as inert empty-state text", (_, text, code) => {
    const input = exampleLayout();
    input.nodeEmptyStates = [
      {
        nodeId: "workstation:triage",
        content: { kind: "text", text },
      },
    ];

    const result = safeParseFactoryVisualizationLayout(input, exampleFactory());

    expect(result).toMatchObject({ success: false });
    if (!result.success) {
      expect(result.issues).toContainEqual(
        expect.objectContaining({
          code,
          path: ["nodeEmptyStates", 0, "content", "text"],
        }),
      );
    }
  });

  it("rejects blank, unknown, and every duplicate canonical node reference without changing the Factory", () => {
    const factory = exampleFactory();
    const originalFactory = structuredClone(factory);
    const input = exampleLayout();
    input.nodeEmptyStates = [
      {
        nodeId: "",
        content: { kind: "text", text: "Blank reference" },
      },
      {
        nodeId: "workstation:missing",
        content: { kind: "text", text: "Unknown reference" },
      },
      {
        nodeId: "workstation:triage",
        content: { kind: "text", text: "First duplicate" },
      },
      {
        nodeId: "workstation:triage",
        content: { kind: "text", text: "Second duplicate" },
      },
    ];

    const result = safeParseFactoryVisualizationLayout(input, factory);

    expect(factory).toEqual(originalFactory);
    expect(result).toMatchObject({ success: false });
    if (!result.success) {
      expect(result.issues).toEqual(
        expect.arrayContaining([
          expect.objectContaining({
            code: "empty_node_id",
            path: ["nodeEmptyStates", 0, "nodeId"],
          }),
          expect.objectContaining({
            code: "unknown_canonical_node_id",
            path: ["nodeEmptyStates", 1, "nodeId"],
          }),
        ]),
      );
      expect(
        result.issues.filter(({ code }) => code === "duplicate_node_id"),
      ).toEqual([
        expect.objectContaining({ path: ["nodeEmptyStates", 2, "nodeId"] }),
        expect.objectContaining({ path: ["nodeEmptyStates", 3, "nodeId"] }),
      ]);
    }
  });

  it("rejects content that mixes the text and image variants", () => {
    const input = exampleLayout();
    input.nodeEmptyStates = [
      {
        nodeId: "workstation:triage",
        content: {
          kind: "text",
          text: "No work is waiting.",
          altText: "Unexpected image",
          source: {
            kind: "embedded",
            mediaType: "image/png",
            base64: "iVBORw0KGgo=",
          },
          callback: "run",
          connection: "workstation:triage",
        },
      },
    ];

    const result = safeParseFactoryVisualizationLayout(input, exampleFactory());

    expect(result).toMatchObject({ success: false });
    if (!result.success) {
      expect(result.issues).toEqual(
        expect.arrayContaining([
          expect.objectContaining({
            code: "unsupported_field",
            path: ["nodeEmptyStates", 0, "content", "altText"],
          }),
          expect.objectContaining({
            code: "unsupported_field",
            path: ["nodeEmptyStates", 0, "content", "source"],
          }),
          expect.objectContaining({
            code: "unsafe_content_field",
            path: ["nodeEmptyStates", 0, "content", "callback"],
          }),
          expect.objectContaining({
            code: "unsafe_content_field",
            path: ["nodeEmptyStates", 0, "content", "connection"],
          }),
        ]),
      );
    }
  });
});
