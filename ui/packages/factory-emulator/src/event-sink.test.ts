import type { FactoryEvent } from "@you-agent-factory/client";
import { describe, expect, it, vi } from "vitest";

import {
  FactoryEventSinkError,
  MemoryFactoryEventSink,
} from "./event-sink.js";

function event(id: string, sequence: number): FactoryEvent {
  return {
    schemaVersion: "agent-factory.event.v1",
    id,
    type: "SESSION_STARTED",
    context: {
      sequence,
      sessionSequence: sequence,
      tick: sequence,
      eventTime: `2026-07-18T00:00:0${sequence}Z`,
      sessionId: "session-1",
    },
    payload: { startedAt: "2026-07-18T00:00:00Z" },
  };
}

function deferred(): {
  readonly promise: Promise<void>;
  readonly resolve: () => void;
  readonly reject: (reason: unknown) => void;
} {
  let resolve!: () => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<void>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
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

describe("MemoryFactoryEventSink", () => {
  it("applies backpressure and exposes a complete batch only after resolution", async () => {
    const gate = deferred();
    const sink = new MemoryFactoryEventSink({
      maxEvents: 2,
      beforeWrite: () => gate.promise,
    });
    const write = sink.write([event("event-1", 1), event("event-2", 2)]);

    await Promise.resolve();
    expect(sink.snapshot()).toEqual([]);
    gate.resolve();
    await write;
    expect(sink.snapshot().map(({ id }) => id)).toEqual([
      "event-1",
      "event-2",
    ]);
  });

  it("preserves caller state and prior history when a write rejects", async () => {
    const gate = deferred();
    let useGate = false;
    const sink = new MemoryFactoryEventSink({
      maxEvents: 3,
      beforeWrite: () => (useGate ? gate.promise : Promise.resolve()),
    });
    await sink.write([event("existing", 1)]);
    let callerState = { committedStep: 1 };
    const proposedState = { committedStep: 2 };
    useGate = true;

    const transact = async (): Promise<void> => {
      await sink.write([event("pending", 2)]);
      callerState = proposedState;
    };
    const transaction = transact();

    await Promise.resolve();
    expect(callerState).toEqual({ committedStep: 1 });
    expect(sink.snapshot().map(({ id }) => id)).toEqual(["existing"]);
    gate.reject(new Error("injected rejection"));
    await expect(transaction).rejects.toThrow("injected rejection");
    expect(callerState).toEqual({ committedStep: 1 });
    expect(sink.snapshot().map(({ id }) => id)).toEqual(["existing"]);
  });

  it("supports identical deterministic retry before one caller-state commit", async () => {
    const attemptedBatches: FactoryEvent[][] = [];
    let attempt = 0;
    const sink = new MemoryFactoryEventSink({
      maxEvents: 2,
      beforeWrite: async (batch) => {
        attemptedBatches.push(structuredClone(batch) as FactoryEvent[]);
        attempt += 1;
        if (attempt === 1) throw new Error("transient failure");
      },
    });
    const pendingBatch = [event("stable-event-1", 1), event("stable-event-2", 2)];
    let committedSteps = 0;

    await expect(sink.write(pendingBatch)).rejects.toThrow("transient failure");
    await sink.write(pendingBatch);
    committedSteps += 1;

    expect(attemptedBatches).toHaveLength(2);
    expect(attemptedBatches[1]).toEqual(attemptedBatches[0]);
    expect(sink.snapshot()).toEqual(pendingBatch);
    expect(committedSteps).toBe(1);
  });

  it("isolates retained events from input and snapshot mutation", async () => {
    const input = [event("event-1", 1)];
    const sink = new MemoryFactoryEventSink({ maxEvents: 1 });
    const write = sink.write(input);
    input[0]!.id = "mutated-input";
    await write;

    const snapshot = sink.snapshot() as FactoryEvent[];
    snapshot[0]!.id = "mutated-snapshot";
    expect(sink.snapshot()[0]?.id).toBe("event-1");
  });

  it("rejects overflow atomically without truncating retained history", async () => {
    const sink = new MemoryFactoryEventSink({ maxEvents: 2 });
    await sink.write([event("existing", 1)]);

    await expectSinkError(
      sink.write([event("overflow-1", 2), event("overflow-2", 3)]),
      "capacity_exceeded",
    );
    expect(sink.snapshot().map(({ id }) => id)).toEqual(["existing"]);
    await sink.write([event("boundary", 2)]);
    expect(sink.snapshot().map(({ id }) => id)).toEqual([
      "existing",
      "boundary",
    ]);
  });

  it("serializes concurrent calls in invocation order", async () => {
    const firstGate = deferred();
    const observed: string[] = [];
    const sink = new MemoryFactoryEventSink({
      maxEvents: 2,
      beforeWrite: async ([candidate]) => {
        observed.push(candidate!.id);
        if (candidate!.id === "first") await firstGate.promise;
      },
    });

    const first = sink.write([event("first", 1)]);
    const second = sink.write([event("second", 2)]);
    await Promise.resolve();
    expect(observed).toEqual(["first"]);
    firstGate.resolve();
    await Promise.all([first, second]);
    expect(observed).toEqual(["first", "second"]);
    expect(sink.snapshot().map(({ id }) => id)).toEqual(["first", "second"]);
  });

  it("drains accepted work during idempotent close and rejects later writes", async () => {
    const gate = deferred();
    const sink = new MemoryFactoryEventSink({
      maxEvents: 1,
      beforeWrite: () => gate.promise,
    });
    const write = sink.write([event("accepted", 1)]);
    const close = sink.close();
    expect(sink.close()).toBe(close);
    const closeSettled = vi.fn();
    void close.then(closeSettled);

    await Promise.resolve();
    expect(closeSettled).not.toHaveBeenCalled();
    await expectSinkError(sink.write([event("too-late", 2)]), "closed");
    expect(sink.snapshot()).toEqual([]);

    gate.resolve();
    await Promise.all([write, close]);
    expect(closeSettled).toHaveBeenCalledOnce();
    expect(sink.snapshot().map(({ id }) => id)).toEqual(["accepted"]);
  });

  it("rejects empty batches and invalid capacity with structured errors", async () => {
    expect(
      () => new MemoryFactoryEventSink({ maxEvents: 0 }),
    ).toThrowError(
      expect.objectContaining({
        name: "FactoryEventSinkError",
        code: "invalid_capacity",
      }),
    );
    const sink = new MemoryFactoryEventSink({ maxEvents: 1 });
    await expectSinkError(sink.write([]), "empty_batch");
    expect(sink.snapshot()).toEqual([]);
  });
});
