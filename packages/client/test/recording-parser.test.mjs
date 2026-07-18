import assert from "node:assert/strict";
import test from "node:test";
import {
  FactoryRecordingValidationError,
  parseFactoryRecording,
  safeParseFactoryRecording,
} from "@you-agent-factory/client";

function event(overrides = {}) {
  return {
    schemaVersion: "agent-factory.event.v1",
    id: "event-1",
    type: "INITIAL_STRUCTURE_REQUEST",
    context: {
      sequence: 0,
      tick: 0,
      eventTime: "2026-07-18T05:00:00Z",
      sessionId: "session-1",
    },
    payload: { factory: { name: "recording-test" } },
    ...overrides,
  };
}

function recording(events = [event()]) {
  return {
    schemaVersion: "agent-factory.recording.v1",
    sessionId: "session-1",
    events,
  };
}

function deeplyFreeze(value) {
  if (value && typeof value === "object") {
    Object.freeze(value);
    for (const child of Object.values(value)) {
      deeplyFreeze(child);
    }
  }
  return value;
}

test("both parsers preserve a valid recording without mutation", () => {
  const input = deeplyFreeze(
    recording([
      event(),
      event({
        id: "event-2",
        type: "SESSION_STARTED",
        context: {
          sequence: 1,
          tick: 1,
          eventTime: "2026-07-18T05:00:01Z",
          sessionId: "session-1",
        },
        payload: { startedAt: "2026-07-18T05:00:01Z" },
      }),
    ]),
  );
  const before = JSON.stringify(input);

  const safeResult = safeParseFactoryRecording(input);
  assert.equal(safeResult.success, true);
  assert.equal(safeResult.data, input);
  assert.equal(parseFactoryRecording(input), input);
  assert.equal(JSON.stringify(input), before);
});

const invalidCases = [
  {
    name: "invalid shape",
    code: "INVALID_SHAPE",
    input() {
      const input = recording();
      delete input.events[0].payload;
      return input;
    },
  },
  {
    name: "unsupported recording version",
    code: "UNSUPPORTED_RECORDING_VERSION",
    input: () => ({
      ...recording(),
      schemaVersion: "agent-factory.recording.v2",
    }),
  },
  {
    name: "unsupported event version",
    code: "UNSUPPORTED_EVENT_VERSION",
    input: () =>
      recording([event({ schemaVersion: "agent-factory.event.v2" })]),
  },
  {
    name: "duplicate event ids",
    code: "DUPLICATE_EVENT_ID",
    input: () =>
      recording([
        event(),
        event({
          context: {
            sequence: 1,
            tick: 1,
            eventTime: "2026-07-18T05:00:01Z",
            sessionId: "session-1",
          },
        }),
      ]),
  },
  {
    name: "mixed session identity",
    code: "MIXED_SESSION_ID",
    input: () =>
      recording([
        event({ context: { ...event().context, sessionId: "session-2" } }),
      ]),
  },
  {
    name: "non-canonical ordering",
    code: "NON_CANONICAL_ORDER",
    input: () =>
      recording([
        event({
          id: "event-2",
          context: {
            sequence: 1,
            tick: 1,
            eventTime: "2026-07-18T05:00:01Z",
            sessionId: "session-1",
          },
        }),
        event(),
      ]),
  },
  {
    name: "missing topology bootstrap",
    code: "MISSING_TOPOLOGY_BOOTSTRAP",
    input: () =>
      recording([
        event({
          type: "SESSION_STARTED",
          payload: { startedAt: "2026-07-18T05:00:00Z" },
        }),
      ]),
  },
];

for (const invalidCase of invalidCases) {
  test(`both parsers report ${invalidCase.name} with structured issues`, () => {
    const input = invalidCase.input();
    let safeResult;
    assert.doesNotThrow(() => {
      safeResult = safeParseFactoryRecording(input);
    });
    assert.equal(safeResult.success, false);
    assert.ok(safeResult.error instanceof FactoryRecordingValidationError);
    assert.equal(safeResult.issues, safeResult.error.issues);
    assert.ok(
      safeResult.issues.some(
        (issue) =>
          issue.code === invalidCase.code && issue.path.startsWith("/"),
      ),
      JSON.stringify(safeResult.issues, null, 2),
    );
    if (invalidCase.code === "INVALID_SHAPE") {
      assert.ok(
        safeResult.issues.some((issue) => issue.path === "/events/0/payload"),
      );
    }
    assert.throws(
      () => parseFactoryRecording(input),
      (error) =>
        error instanceof FactoryRecordingValidationError &&
        error.issues.some((issue) => issue.code === invalidCase.code),
    );
  });
}

test("canonical ordering compares eventTime and id after tick and sequence", () => {
  for (const secondEvent of [
    event({ id: "event-2", context: { ...event().context, sequence: 0 } }),
    event({ id: "event-0" }),
    event({
      id: "event-2",
      context: { ...event().context, eventTime: "2026-07-18T04:59:59Z" },
    }),
  ]) {
    const firstEvent = event({
      context:
        secondEvent.id === "event-2" &&
        secondEvent.context.eventTime === event().context.eventTime
          ? { ...event().context, sequence: 1 }
          : event().context,
    });
    const input = recording([firstEvent, secondEvent]);
    const before = JSON.stringify(input);
    const result = safeParseFactoryRecording(input);
    assert.equal(result.success, false);
    assert.ok(
      result.issues.some((issue) => issue.code === "NON_CANONICAL_ORDER"),
    );
    assert.equal(JSON.stringify(input), before);
  }
});
