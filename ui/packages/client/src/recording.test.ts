import { describe, expect, expectTypeOf, it } from "vitest";

import customerSupportRecording from "../examples/customer-support.factory-recording.v1.json";

import {
  type FactoryRecording,
  FactoryRecordingValidationError,
  parseFactoryRecording,
  safeParseFactoryRecording,
} from "./index.js";

async function readExample(): Promise<unknown> {
  return structuredClone(customerSupportRecording);
}

async function exampleRecording(): Promise<Record<string, unknown>> {
  return (await readExample()) as Record<string, unknown>;
}

function exampleEvents(
  input: Record<string, unknown>,
): Record<string, unknown>[] {
  return input.events as Record<string, unknown>[];
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

describe("Factory recording semantic validation", () => {
  it("accumulates unsupported event versions and every duplicate ID location", async () => {
    const input = await exampleRecording();
    const firstEvent = exampleEvents(input)[0] as Record<string, unknown>;
    const duplicateEvent = structuredClone(firstEvent);
    firstEvent.schemaVersion = "agent-factory.event.v2";
    input.events = [firstEvent, duplicateEvent];

    const result = safeParseFactoryRecording(input);

    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.issues).toEqual(
        expect.arrayContaining([
          expect.objectContaining({
            category: "semantic",
            code: "unsupported_event_schema_version",
            path: ["events", 0, "schemaVersion"],
          }),
          expect.objectContaining({
            code: "duplicate_event_id",
            path: ["events", 0, "id"],
          }),
          expect.objectContaining({
            code: "duplicate_event_id",
            path: ["events", 1, "id"],
          }),
        ]),
      );
    }
  });

  it("rejects mixed Factory Session identities without mutating the input", async () => {
    const input = await exampleRecording();
    delete input.factory;
    const firstEvent = exampleEvents(input)[0] as Record<string, unknown>;
    const secondEvent = structuredClone(firstEvent);
    secondEvent.id = "evt-topology-002";
    (secondEvent.context as Record<string, unknown>).sessionId =
      "another-session";
    input.events = [firstEvent, secondEvent];
    const original = structuredClone(input);

    const result = safeParseFactoryRecording(input);

    expect(result.success).toBe(false);
    expect(input).toEqual(original);
    if (!result.success) {
      expect(
        result.issues.filter(
          (issue) => issue.code === "mixed_factory_session_identity",
        ),
      ).toEqual([
        expect.objectContaining({
          path: ["events", 0, "context", "sessionId"],
        }),
        expect.objectContaining({
          path: ["events", 1, "context", "sessionId"],
        }),
      ]);
    }
  });

  it("accepts histories whose events consistently omit Factory Session identity", async () => {
    const input = await exampleRecording();
    for (const event of exampleEvents(input)) {
      delete (event.context as Record<string, unknown>).sessionId;
    }

    expect(safeParseFactoryRecording(input)).toMatchObject({ success: true });
  });

  it("rejects recordings without a usable topology bootstrap", async () => {
    const input = await exampleRecording();
    delete input.factory;
    const event = exampleEvents(input)[0] as Record<string, unknown>;
    event.type = "WORK_REQUEST";
    event.payload = {};

    expect(safeParseFactoryRecording(input)).toEqual({
      success: false,
      issues: [
        {
          category: "semantic",
          code: "missing_topology_bootstrap",
          path: ["events"],
          message:
            "Expected a usable top-level factory or a topology-establishing Factory event.",
        },
      ],
    });
  });

  it("accepts topology supplied by either the recording or an event", async () => {
    const topLevelTopology = await exampleRecording();
    const topLevelEvent = exampleEvents(topLevelTopology)[0] as Record<
      string,
      unknown
    >;
    topLevelEvent.type = "WORK_REQUEST";
    topLevelEvent.payload = {};

    const eventTopology = await exampleRecording();
    delete eventTopology.factory;

    expect(safeParseFactoryRecording(topLevelTopology)).toMatchObject({
      success: true,
    });
    expect(safeParseFactoryRecording(eventTopology)).toMatchObject({
      success: true,
    });
  });
});
