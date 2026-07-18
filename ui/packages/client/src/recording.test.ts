import { readFile } from "node:fs/promises";

import { describe, expect, expectTypeOf, it } from "vitest";

import {
  type FactoryRecording,
  FactoryRecordingValidationError,
  parseFactoryRecording,
  safeParseFactoryRecording,
} from "./index.js";

async function readExample(): Promise<unknown> {
  const source = await readFile(
    new URL(
      "../examples/customer-support.factory-recording.v1.json",
      import.meta.url,
    ),
    "utf8",
  );
  return JSON.parse(source);
}

describe("Factory recording validation", () => {
  it("validates the packaged customer recording without a runtime", async () => {
    const recording = parseFactoryRecording(await readExample());

    expect(recording.schemaVersion).toBe("factory-recording/v1");
    expect(recording.factory?.name).toBe("customer-support-triage");
    expect(recording.events).toHaveLength(1);
    expect(
      new Set(recording.events.map((event) => event.context.sessionId)),
    ).toEqual(new Set(["session-customer-support-example"]));
    expectTypeOf(recording).toEqualTypeOf<FactoryRecording>();
  });

  it("returns a discriminated failure without throwing", () => {
    const result = safeParseFactoryRecording({
      schemaVersion: "factory-recording/v1",
      id: "broken-recording",
      title: 42,
      events: "not-an-array",
    });

    expect(result).toEqual({
      success: false,
      issues: [
        {
          category: "structure",
          code: "invalid_type",
          path: ["title"],
          message: "Expected title to be a string.",
        },
        {
          category: "structure",
          code: "invalid_type",
          path: ["events"],
          message: "Expected events to be an array.",
        },
      ],
    });
  });

  it("rejects unsupported recording versions with a structured error", async () => {
    const input = (await readExample()) as Record<string, unknown>;
    input.schemaVersion = "factory-recording/v2";

    expect(() => parseFactoryRecording(input)).toThrowError(
      FactoryRecordingValidationError,
    );
    try {
      parseFactoryRecording(input);
    } catch (error) {
      expect(error).toMatchObject({
        issues: [
          {
            category: "structure",
            code: "unsupported_recording_schema_version",
            path: ["schemaVersion"],
          },
        ],
      });
    }
  });

  it("does not expose partially parsed events for malformed envelopes", async () => {
    const input = (await readExample()) as Record<string, unknown>;
    delete input.id;
    input.events = [{ id: "partial" }];

    const result = safeParseFactoryRecording(input);
    expect(result.success).toBe(false);
    expect(result).not.toHaveProperty("data");
    if (!result.success) {
      expect(result.issues).toEqual(
        expect.arrayContaining([
          expect.objectContaining({
            code: "missing_required_field",
            path: ["id"],
          }),
          expect.objectContaining({
            code: "missing_required_field",
            path: ["events", 0, "schemaVersion"],
          }),
        ]),
      );
    }
  });
});
