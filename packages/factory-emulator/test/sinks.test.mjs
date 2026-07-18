import assert from "node:assert/strict";
import test from "node:test";
import {
  FactoryEmulatorAdvanceInProgressError,
  FactoryEventSinkCapacityError,
  FactoryEventSinkClosedError,
  createFactoryEmulator,
  createFactoryRecordingSink,
  createMemoryFactoryEventSink,
} from "@you-agent-factory/factory-emulator";

function event(id, sequence, payload = { nested: { value: id } }) {
  return {
    schemaVersion: "agent-factory.event.v1",
    id,
    type: "INITIAL_STRUCTURE_REQUEST",
    context: {
      sequence,
      tick: sequence,
      eventTime: `2026-07-18T05:00:0${sequence}Z`,
      sessionId: "session-1",
    },
    payload,
  };
}

test("memory sink retains only complete newest batches and isolates snapshots", async () => {
  const sink = createMemoryFactoryEventSink({ maxEvents: 2 });
  const first = event("event-1", 1);
  await sink.write({ events: [first] });
  first.payload.nested.value = "mutated after write";
  assert.equal(sink.batches()[0].events[0].payload.nested.value, "event-1");

  await sink.write({ events: [event("event-2", 2)] });
  await sink.write({ events: [event("event-3", 3)] });

  const history = sink.batches();
  assert.deepEqual(
    history.map((batch) => batch.events.map((accepted) => accepted.id)),
    [["event-2"], ["event-3"]],
  );
  history[0].events[0].payload.nested.value = "mutated returned history";
  assert.equal(
    sink.batches()[0].events[0].payload.nested.value,
    "event-2",
  );

  await assert.rejects(
    sink.write({
      events: [event("event-4", 4), event("event-5", 5), event("event-6", 6)],
    }),
    FactoryEventSinkCapacityError,
  );
  assert.deepEqual(
    sink.batches().map((batch) => batch.events.map((accepted) => accepted.id)),
    [["event-2"], ["event-3"]],
  );
});

test("recording sink keeps whole accepted batches, produces detached recordings, and closes", async () => {
  const sink = createFactoryRecordingSink({
    sessionId: "session-1",
    maxEvents: 3,
  });
  const first = event("event-1", 1);
  const second = event("event-2", 2);
  await sink.write({ events: [first, second] });
  first.payload.nested.value = "mutated after write";

  await assert.rejects(
    sink.write({ events: [event("event-3", 3), event("event-4", 4)] }),
    FactoryEventSinkCapacityError,
  );
  const recording = sink.recording();
  assert.deepEqual(
    recording.events.map((accepted) => accepted.id),
    ["event-1", "event-2"],
  );
  assert.equal(recording.events[0].payload.nested.value, "event-1");
  recording.events[0].payload.nested.value = "mutated returned recording";
  assert.equal(
    sink.recording().events[0].payload.nested.value,
    "event-1",
  );

  await sink.close();
  await assert.rejects(
    sink.write({ events: [event("event-3", 3)] }),
    FactoryEventSinkClosedError,
  );
  assert.deepEqual(
    sink.recording().events.map((accepted) => accepted.id),
    ["event-1", "event-2"],
  );
});

test("memory sink rejects writes after close", async () => {
  const sink = createMemoryFactoryEventSink({ maxEvents: 1 });
  await sink.close();
  await assert.rejects(
    sink.write({ events: [event("event-1", 1)] }),
    FactoryEventSinkClosedError,
  );
});

test("emulator commits its calculated state only after the logical-tick batch is accepted", async () => {
  const pendingWrite = deferred();
  const batches = [];
  const sink = {
    async close() {
      return { status: "closed" };
    },
    async write(batch) {
      batches.push(structuredClone(batch));
      await pendingWrite.promise;
      return { status: "accepted" };
    },
  };
  const emulator = createFactoryEmulator({
    initialState: { count: 0 },
    sink,
    calculateTick(state) {
      return {
        batch: { events: [event("event-1", 1)] },
        state: { count: state.count + 1 },
      };
    },
  });

  const advance = emulator.advance();
  assert.deepEqual(emulator.state(), { count: 0 });
  assert.deepEqual(batches.map((batch) => batch.events.map((accepted) => accepted.id)), [
    ["event-1"],
  ]);
  await assert.rejects(emulator.advance(), FactoryEmulatorAdvanceInProgressError);

  pendingWrite.resolve();
  const receipt = await advance;
  assert.deepEqual(receipt, {
    status: "committed",
    batch: { events: [event("event-1", 1)] },
  });
  assert.deepEqual(emulator.state(), { count: 1 });
});

function deferred() {
  let resolve;
  const promise = new Promise((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}
