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
      diagnostics: [],
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

describe("Canonical Factory recording schema validation", () => {
  it("rejects canonical context values and type-specific payloads that violate the event schema", async () => {
    const input = await exampleRecording();
    const event = exampleEvents(input)[0] as Record<string, unknown>;
    event.type = "WORK_REQUEST";
    event.payload = {};
    event.context = {
      sequence: -1.5,
      tick: -2,
      eventTime: "not-a-date",
    };

    const result = safeParseFactoryRecording(input);

    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.issues).toEqual(
        expect.arrayContaining([
          expect.objectContaining({
            path: ["events", 0, "context", "sequence"],
          }),
          expect.objectContaining({
            code: "invalid_value",
            path: ["events", 0, "context", "tick"],
          }),
          expect.objectContaining({
            code: "invalid_value",
            path: ["events", 0, "context", "eventTime"],
          }),
          expect.objectContaining({
            code: "missing_required_field",
            path: ["events", 0, "payload", "type"],
          }),
        ]),
      );
    }
  });
});

describe("Factory canonical compatibility diagnostics", () => {
  it("accepts future Factory and event data with compatibility diagnostics", async () => {
    const input = await exampleRecording();
    input.unexpectedRecordingField = "recording-secret";
    const factory = input.factory as Record<string, unknown>;
    factory.unexpectedFactoryField = "factory-secret";
    const factoryWorkTypes = factory.workTypes as Record<string, unknown>[];
    factoryWorkTypes[0].unexpectedWorkTypeField = "work-type-secret";
    const factoryStates = factoryWorkTypes[0].states as Record<
      string,
      unknown
    >[];
    factoryStates[0].type = "FUTURE_WORK_STATE";

    const event = exampleEvents(input)[0] as Record<string, unknown>;
    event.unexpectedEventField = "event-secret";
    (event.context as Record<string, unknown>).unexpectedContextField =
      "context-secret";
    (event.payload as Record<string, unknown>).unexpectedPayloadField =
      "payload-secret";
    const eventFactory = (event.payload as Record<string, unknown>)
      .factory as Record<string, unknown>;
    eventFactory.unexpectedFactoryField = "event-factory-secret";

    const futureEvent = structuredClone(event);
    futureEvent.id = "evt-future-002";
    futureEvent.type = "FUTURE_EVENT_TYPE";
    (futureEvent.context as Record<string, unknown>).sequence = 2;
    (futureEvent.context as Record<string, unknown>).sessionSequence = 2;
    futureEvent.payload = { futurePayload: "future-payload-secret" };
    input.events = [event, futureEvent];

    const result = safeParseFactoryRecording(input);

    expect(result.success).toBe(true);
    if (!result.success) return;

    expect(result.data.factory).toBe(input.factory);
    expect(result.data.events[0]).toBe(event);
    expect(result.data.events[1]?.type).toBe("FUTURE_EVENT_TYPE");
    expect(result.diagnostics).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          code: "unsupported_field",
          path: ["unexpectedRecordingField"],
        }),
        expect.objectContaining({
          code: "unsupported_field",
          path: ["factory", "unexpectedFactoryField"],
        }),
        expect.objectContaining({
          code: "unsupported_field",
          path: ["factory", "workTypes", 0, "unexpectedWorkTypeField"],
        }),
        expect.objectContaining({
          code: "unsupported_enum_value",
          path: ["factory", "workTypes", 0, "states", 0, "type"],
        }),
        expect.objectContaining({
          code: "unsupported_field",
          path: ["events", 0, "unexpectedEventField"],
        }),
        expect.objectContaining({
          code: "unsupported_field",
          path: ["events", 0, "context", "unexpectedContextField"],
        }),
        expect.objectContaining({
          code: "unsupported_field",
          path: ["events", 0, "payload", "unexpectedPayloadField"],
        }),
        expect.objectContaining({
          code: "unsupported_field",
          path: ["events", 0, "payload", "factory", "unexpectedFactoryField"],
        }),
        expect.objectContaining({
          code: "unsupported_event_type",
          path: ["events", 1, "type"],
        }),
      ]),
    );
    expect(
      result.diagnostics.some(({ message }) =>
        message.includes("FUTURE_WORK_STATE"),
      ),
    ).toBe(true);
    expect(
      result.diagnostics.every(
        ({ message }) =>
          !message.includes("recording-secret") &&
          !message.includes("factory-secret") &&
          !message.includes("work-type-secret") &&
          !message.includes("event-secret") &&
          !message.includes("context-secret") &&
          !message.includes("payload-secret"),
      ),
    ).toBe(true);
  });
});

describe("Factory canonical blocking failures", () => {
  it("keeps known required and typed-field failures blocking", async () => {
    const input = await exampleRecording();
    delete input.id;
    const event = exampleEvents(input)[0] as Record<string, unknown>;
    (event.context as Record<string, unknown>).sequence = "not-a-number";
    event.futureMetadata = "ignored-diagnostic";

    const result = safeParseFactoryRecording(input);

    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.issues).toEqual(
        expect.arrayContaining([
          expect.objectContaining({
            code: "missing_required_field",
            path: ["id"],
          }),
          expect.objectContaining({
            code: "invalid_type",
            path: ["events", 0, "context", "sequence"],
          }),
        ]),
      );
      expect(result.issues).not.toEqual(
        expect.arrayContaining([
          expect.objectContaining({ code: "unsupported_field" }),
        ]),
      );
      expect(result.diagnostics).toEqual([
        expect.objectContaining({
          code: "unsupported_field",
          path: ["events", 0, "futureMetadata"],
        }),
      ]);
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
    event.payload = { type: "FACTORY_REQUEST_BATCH" };

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
      diagnostics: [],
    });
  });

  it("accepts topology supplied by either the recording or an event", async () => {
    const topLevelTopology = await exampleRecording();
    const topLevelEvent = exampleEvents(topLevelTopology)[0] as Record<
      string,
      unknown
    >;
    topLevelEvent.type = "WORK_REQUEST";
    topLevelEvent.payload = { type: "FACTORY_REQUEST_BATCH" };

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
