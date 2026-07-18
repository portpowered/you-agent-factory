import { describe, expect, it } from "vitest";

import {
  createFactoryEventCursor,
  type FactoryEvent,
  FactoryReplayTextParseError,
  orderFactoryEvents,
  parseFactoryEventReplayText,
  safeParseFactoryEventReplayText,
  safeParseFactoryRecording,
} from "./index.js";

function event(
  id: string,
  tick: number,
  sequence: number,
  sessionSequence?: number,
): FactoryEvent {
  return {
    schemaVersion: "agent-factory.event.v1",
    id,
    type: "WORK_REQUEST",
    context: {
      sequence,
      tick,
      eventTime: `2026-07-18T00:00:0${sequence}Z`,
      ...(sessionSequence === undefined ? {} : { sessionSequence }),
    },
    payload: { type: "FACTORY_REQUEST_BATCH" },
  };
}

function frame(factoryEvent: FactoryEvent): string {
  return `data: ${JSON.stringify(factoryEvent)}\n\n`;
}

describe("Factory event replay ordering", () => {
  it("orders by logical tick and effective same-tick sequence", () => {
    const laterTick = event("later-tick", 2, 1, 1);
    const laterInTick = event("later-in-tick", 1, 2, 8);
    const earlierInTick = event("earlier-in-tick", 1, 9, 3);

    expect(
      orderFactoryEvents([laterTick, laterInTick, earlierInTick]).map(
        ({ id }) => id,
      ),
    ).toEqual(["earlier-in-tick", "later-in-tick", "later-tick"]);
    expect(
      orderFactoryEvents([earlierInTick, laterInTick, laterTick]).map(
        ({ id }) => id,
      ),
    ).toEqual(["earlier-in-tick", "later-in-tick", "later-tick"]);
  });

  it("uses event-log sequence as the fallback without mutating input", () => {
    const later = event("later", 1, 7);
    const earlier = event("earlier", 1, 2);
    const input = [later, earlier];
    const original = structuredClone(input);

    const ordered = orderFactoryEvents(input);

    expect(ordered.map(({ id }) => id)).toEqual(["earlier", "later"]);
    expect(ordered).not.toBe(input);
    expect(input).toEqual(original);
    expect(ordered[0]).toBe(earlier);
  });

  it("returns the acknowledged reconnect cursor", () => {
    expect(createFactoryEventCursor(event("acknowledged", 4, 17, 9))).toEqual({
      afterEventId: "acknowledged",
      afterSequence: 9,
      tick: 4,
    });
  });

  it("normalizes validated recording events without mutating the input", () => {
    const later = event("later", 2, 2);
    const earlier = event("earlier", 1, 1);
    const input = {
      schemaVersion: "factory-recording/v1",
      id: "out-of-order",
      title: "Out-of-order recording",
      factory: { name: "example" },
      events: [later, earlier],
    };
    const original = structuredClone(input);

    const result = safeParseFactoryRecording(input);

    expect(result.success).toBe(true);
    if (result.success) {
      expect(result.data.events.map(({ id }) => id)).toEqual([
        "earlier",
        "later",
      ]);
    }
    expect(input).toEqual(original);
  });
});

describe("Factory event SSE replay text", () => {
  it("ignores sparse comments and unrelated empty blocks", () => {
    const replayText = `: keepalive\n\nevent: ready\nid: ignored\n\n${frame(
      event("accepted", 0, 1),
    )}\n: trailing keepalive`;

    expect(parseFactoryEventReplayText(replayText).map(({ id }) => id)).toEqual(
      ["accepted"],
    );
  });

  it("parses multiple frames and returns canonical order", () => {
    const replayText = `${frame(event("second", 2, 2))}${frame(
      event("first", 1, 1),
    )}`;

    expect(parseFactoryEventReplayText(replayText).map(({ id }) => id)).toEqual(
      ["first", "second"],
    );
  });

  it("preserves multiline SSE data semantics", () => {
    const multilineEvent = JSON.stringify(event("multiline", 0, 1), null, 2)
      .split("\n")
      .map((line) => `data: ${line}`)
      .join("\r\n");

    expect(
      parseFactoryEventReplayText(`${multilineEvent}\r\n\r\n`)[0]?.id,
    ).toBe("multiline");
  });

  it("reports malformed JSON as a structured safe-parse failure", () => {
    expect(safeParseFactoryEventReplayText("data: {broken}\n\n")).toEqual({
      success: false,
      issues: [
        {
          code: "malformed_event_json",
          path: ["frames", 0, "data"],
          message: "Expected SSE data to contain valid Factory event JSON.",
        },
      ],
    });
    expect(() =>
      parseFactoryEventReplayText("data: {broken}\n\n"),
    ).toThrowError(FactoryReplayTextParseError);
  });

  it("reuses event-envelope and schema-version validation", () => {
    const invalidEnvelope = { ...event("invalid", 0, 1), payload: "wrong" };
    const unsupportedVersion = {
      ...event("unsupported", 0, 2),
      schemaVersion: "agent-factory.event.v2",
    };
    const result = safeParseFactoryEventReplayText(
      `${frame(invalidEnvelope as FactoryEvent)}${frame(
        unsupportedVersion as FactoryEvent,
      )}`,
    );

    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.issues).toEqual(
        expect.arrayContaining([
          expect.objectContaining({
            code: "invalid_type",
            path: ["frames", 0, "data", "payload"],
          }),
          expect.objectContaining({
            code: "unsupported_event_schema_version",
            path: ["frames", 1, "data", "schemaVersion"],
          }),
        ]),
      );
    }
  });

  it("rejects canonical numeric, date, payload, and additional-property violations", () => {
    const invalid = event("invalid-canonical-event", 0, 1) as unknown as Record<
      string,
      unknown
    >;
    invalid.context = {
      sequence: -1.5,
      tick: -2,
      eventTime: "not-a-date",
      unexpected: true,
    };
    invalid.payload = {};
    invalid.unexpected = true;

    const result = safeParseFactoryEventReplayText(
      `data: ${JSON.stringify(invalid)}\n\n`,
    );

    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.issues).toEqual(
        expect.arrayContaining([
          expect.objectContaining({
            path: ["frames", 0, "data", "context", "sequence"],
          }),
          expect.objectContaining({
            code: "invalid_value",
            path: ["frames", 0, "data", "context", "eventTime"],
          }),
          expect.objectContaining({
            code: "missing_required_field",
            path: ["frames", 0, "data", "payload", "type"],
          }),
          expect.objectContaining({
            code: "unsupported_field",
            path: ["frames", 0, "data", "unexpected"],
          }),
        ]),
      );
    }
  });
});
