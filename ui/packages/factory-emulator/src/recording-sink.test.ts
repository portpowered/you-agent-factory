import {
  type FactoryEvent,
  safeParseFactoryRecording,
} from "@you-agent-factory/client";
import { describe, expect, it } from "vitest";

import exampleScenario from "../examples/customer-support.scenario.v1.json" with {
  type: "json",
};
import type { FactoryEventSinkError } from "./event-sink.js";
import { RecordingFactoryEventSink } from "./recording-sink.js";

function topologyEvent(
  id: string,
  sequence: number,
  options: { readonly factoryName?: string; readonly sessionId?: string } = {},
): FactoryEvent {
  return {
    schemaVersion: "agent-factory.event.v1",
    id,
    type: "INITIAL_STRUCTURE_REQUEST",
    context: {
      sequence,
      sessionSequence: sequence,
      tick: sequence,
      eventTime: `2026-07-18T00:00:0${sequence}Z`,
      sessionId: options.sessionId ?? "session-1",
    },
    payload: {
      factory: { name: options.factoryName ?? exampleScenario.factory.name },
    },
  };
}

function sessionEvent(
  id: string,
  sequence: number,
  sessionId = "session-1",
): FactoryEvent {
  return {
    schemaVersion: "agent-factory.event.v1",
    id,
    type: "SESSION_STARTED",
    context: {
      sequence,
      sessionSequence: sequence,
      tick: sequence,
      eventTime: `2026-07-18T00:00:0${sequence}Z`,
      sessionId,
    },
    payload: { startedAt: "2026-07-18T00:00:00Z" },
  };
}

function createSink(
  overrides: Partial<
    ConstructorParameters<typeof RecordingFactoryEventSink>[0]
  > = {},
): RecordingFactoryEventSink {
  return new RecordingFactoryEventSink({
    maxEvents: 10,
    recording: {
      schemaVersion: "factory-recording/v1",
      id: "customer-support-emulator-example",
      title: "Customer support emulator example",
      factory: { name: exampleScenario.factory.name },
    },
    ...overrides,
  });
}

async function expectSinkError(
  promise: Promise<unknown>,
  code: FactoryEventSinkError["code"],
): Promise<void> {
  await expect(promise).rejects.toMatchObject({
    name: "FactoryEventSinkError",
    code,
  });
}

function deferred(): {
  readonly promise: Promise<void>;
  readonly resolve: () => void;
} {
  let resolve!: () => void;
  const promise = new Promise<void>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

describe("RecordingFactoryEventSink", () => {
  it("produces a client-valid recording from the checked-in scenario and canonical batches", async () => {
    const sink = createSink();
    await sink.write([topologyEvent("event-1", 1)]);
    await sink.write([sessionEvent("event-2", 2)]);

    const recording = sink.snapshot();
    expect(recording.events.map(({ id }) => id)).toEqual([
      "event-1",
      "event-2",
    ]);
    expect(safeParseFactoryRecording(recording)).toMatchObject({
      success: true,
    });
  });

  it("rejects mixed Factory and session identities without changing history", async () => {
    const sink = createSink();
    await sink.write([topologyEvent("event-1", 1)]);

    await expectSinkError(
      sink.write([topologyEvent("wrong-factory", 2, { factoryName: "other" })]),
      "mixed_factory_identity",
    );
    await expectSinkError(
      sink.write([sessionEvent("wrong-session", 2, "session-2")]),
      "mixed_session_identity",
    );
    expect(sink.snapshot().events.map(({ id }) => id)).toEqual(["event-1"]);
  });

  it("rejects duplicate IDs and non-canonical order atomically", async () => {
    const sink = createSink();
    await sink.write([topologyEvent("event-1", 1)]);

    await expectSinkError(
      sink.write([sessionEvent("event-1", 2)]),
      "duplicate_event_id",
    );
    await expectSinkError(
      sink.write([sessionEvent("event-3", 3), sessionEvent("event-2", 2)]),
      "invalid_event_order",
    );
    expect(sink.snapshot().events.map(({ id }) => id)).toEqual(["event-1"]);
  });

  it("rejects invalid event envelopes and capacity overflow atomically", async () => {
    const sink = createSink({ maxEvents: 2 });
    await sink.write([topologyEvent("event-1", 1)]);
    const invalid = sessionEvent("invalid", 2) as FactoryEvent & {
      unexpected?: boolean;
    };
    invalid.unexpected = true;

    await expectSinkError(sink.write([invalid]), "invalid_recording");
    await expectSinkError(
      sink.write([sessionEvent("event-2", 2), sessionEvent("event-3", 3)]),
      "capacity_exceeded",
    );
    expect(sink.snapshot().events.map(({ id }) => id)).toEqual(["event-1"]);
  });
});

describe("RecordingFactoryEventSink lifecycle", () => {
  it("preserves identical event IDs and payloads across an injected retry", async () => {
    const attempts: FactoryEvent[][] = [];
    let attempt = 0;
    const sink = createSink({
      beforeWrite: async (events) => {
        attempts.push(structuredClone(events) as FactoryEvent[]);
        attempt += 1;
        if (attempt === 1) throw new Error("transient recording failure");
      },
    });
    const batch = [topologyEvent("stable-event", 1)];

    await expect(sink.write(batch)).rejects.toThrow(
      "transient recording failure",
    );
    expect(sink.snapshot().events).toEqual([]);
    await sink.write(batch);

    expect(attempts).toHaveLength(2);
    expect(attempts[1]).toEqual(attempts[0]);
    expect(sink.snapshot().events).toEqual(batch);
  });

  it("isolates pending input, retained metadata, and returned snapshots", async () => {
    const gate = deferred();
    const metadata = {
      schemaVersion: "factory-recording/v1" as const,
      id: "isolated-recording",
      title: "Original title",
      factory: { name: exampleScenario.factory.name },
    };
    const sink = createSink({
      recording: metadata,
      beforeWrite: () => gate.promise,
    });
    const batch = [topologyEvent("event-1", 1)];
    const write = sink.write(batch);
    const inputEvent = batch[0];
    if (inputEvent === undefined) throw new Error("Expected input event.");
    inputEvent.id = "mutated-input";
    metadata.title = "Mutated title";
    gate.resolve();
    await write;

    const snapshot = sink.snapshot();
    const snapshotEvent = snapshot.events[0];
    if (snapshotEvent === undefined)
      throw new Error("Expected snapshot event.");
    snapshotEvent.id = "mutated-snapshot";
    snapshot.title = "Mutated snapshot title";
    expect(sink.snapshot()).toMatchObject({
      title: "Original title",
      events: [{ id: "event-1" }],
    });
  });

  it("serializes writes and drains accepted work during idempotent close", async () => {
    const gate = deferred();
    const observed: string[] = [];
    const sink = createSink({
      beforeWrite: async ([event]) => {
        if (event === undefined) throw new Error("Expected queued event.");
        observed.push(event.id);
        if (event.id === "event-1") await gate.promise;
      },
    });
    const first = sink.write([topologyEvent("event-1", 1)]);
    const second = sink.write([sessionEvent("event-2", 2)]);
    const close = sink.close();

    await Promise.resolve();
    expect(observed).toEqual(["event-1"]);
    expect(sink.close()).toBe(close);
    await expectSinkError(sink.write([sessionEvent("late", 3)]), "closed");
    gate.resolve();
    await Promise.all([first, second, close]);
    expect(observed).toEqual(["event-1", "event-2"]);
    expect(sink.snapshot().events).toHaveLength(2);
  });

  it("rejects empty batches and invalid capacity with structured errors", async () => {
    expect(() => createSink({ maxEvents: 0 })).toThrowError(
      expect.objectContaining({
        name: "FactoryEventSinkError",
        code: "invalid_capacity",
      }),
    );
    const sink = createSink();
    await expectSinkError(sink.write([]), "empty_batch");
  });
});
