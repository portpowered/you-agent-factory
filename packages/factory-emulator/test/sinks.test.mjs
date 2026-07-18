import assert from "node:assert/strict";
import test from "node:test";
import {
  FactoryEmulatorAdvanceInProgressError,
  FactoryEmulatorClosedError,
  FactoryEmulatorPendingTransactionError,
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

test("emulator retries a rejected tick unchanged before calculating later work", async () => {
  const attempts = [];
  let calculations = 0;
  let rejectNextWrite = true;
  const emulator = createFactoryEmulator({
    initialState: { count: 0 },
    sink: {
      async close() {
        return { status: "closed" };
      },
      async write(batch) {
        attempts.push(structuredClone(batch));
        if (rejectNextWrite) {
          rejectNextWrite = false;
          throw new Error("persistence unavailable");
        }
        return { status: "accepted" };
      },
    },
    calculateTick(state) {
      calculations += 1;
      return {
        batch: { events: [event(`event-${state.count + 1}`, state.count + 1)] },
        state: { count: state.count + 1 },
      };
    },
  });

  await assert.rejects(emulator.advance(), /persistence unavailable/);
  assert.deepEqual(emulator.state(), { count: 0 });
  assert.equal(calculations, 1);
  assert.deepEqual(emulator.status(), {
    phase: "pending",
    lastError: { operation: "write", message: "persistence unavailable" },
  });
  const inspectedPending = emulator.pending();
  inspectedPending.events[0].payload.nested.value = "mutated inspected pending";
  assert.equal(emulator.pending().events[0].payload.nested.value, "event-1");

  await assert.rejects(emulator.close(), FactoryEmulatorPendingTransactionError);
  const retryReceipt = await emulator.advance();
  assert.equal(calculations, 1);
  assert.deepEqual(retryReceipt.batch, attempts[0]);
  assert.deepEqual(attempts, [retryReceipt.batch, retryReceipt.batch]);
  assert.deepEqual(emulator.state(), { count: 1 });
  assert.deepEqual(emulator.status(), { phase: "open", lastError: undefined });

  await emulator.advance();
  assert.equal(calculations, 2);
  assert.deepEqual(
    attempts.map((batch) => batch.events[0].id),
    ["event-1", "event-1", "event-2"],
  );
});

test("reset explicitly discards a rejected batch and repeated recoveries commit once", async () => {
  let rejectNextWrite = true;
  let calculations = 0;
  const emulator = createFactoryEmulator({
    initialState: { count: 0 },
    sink: {
      async close() {
        return { status: "closed" };
      },
      async write() {
        if (rejectNextWrite) {
          rejectNextWrite = false;
          throw new Error("write rejected");
        }
        return { status: "accepted" };
      },
    },
    calculateTick(state) {
      calculations += 1;
      return {
        batch: { events: [event(`event-${calculations}`, state.count + 1)] },
        state: { count: state.count + 1 },
      };
    },
  });

  await assert.rejects(emulator.advance(), /write rejected/);
  emulator.reset();
  assert.equal(emulator.pending(), undefined);
  assert.deepEqual(emulator.status(), { phase: "open", lastError: undefined });
  await emulator.advance();
  assert.equal(calculations, 2);

  for (const expectedCount of [2, 3]) {
    rejectNextWrite = true;
    await assert.rejects(emulator.advance(), /write rejected/);
    assert.deepEqual(emulator.state(), { count: expectedCount - 1 });
    await emulator.advance();
    assert.deepEqual(emulator.state(), { count: expectedCount });
  }
  assert.equal(calculations, 4);
});

test("an idle emulator accepts later work, then writes terminal lifecycle events before closing", async () => {
  const operations = [];
  const emulator = createFactoryEmulator({
    initialState: { count: 0 },
    sink: {
      async close() {
        operations.push("close");
        return { status: "closed" };
      },
      async write(batch) {
        operations.push(structuredClone(batch));
        return { status: "accepted" };
      },
    },
    calculateClose(state) {
      return { events: [event("session-closed", state.count + 1)] };
    },
    calculateTick(state) {
      return {
        batch: { events: [event(`event-${state.count + 1}`, state.count + 1)] },
        state: { count: state.count + 1 },
      };
    },
  });

  assert.deepEqual(emulator.status(), { phase: "open", lastError: undefined });
  await emulator.advance();
  const closeReceipt = await emulator.close();
  assert.deepEqual(closeReceipt, {
    status: "closed",
    batch: { events: [event("session-closed", 2)] },
  });
  assert.deepEqual(operations, [
    { events: [event("event-1", 1)] },
    { events: [event("session-closed", 2)] },
    "close",
  ]);
  assert.deepEqual(emulator.status(), { phase: "closed", lastError: undefined });
  await assert.rejects(emulator.advance(), FactoryEmulatorClosedError);
});

test("emulator retains terminal lifecycle writes across close failures", async () => {
  const terminalAttempts = [];
  let closeCalculations = 0;
  let rejectTerminalWrite = true;
  let rejectSinkClose = true;
  const emulator = createFactoryEmulator({
    initialState: { count: 0 },
    sink: {
      async close() {
        if (rejectSinkClose) {
          rejectSinkClose = false;
          throw new Error("sink close unavailable");
        }
        return { status: "closed" };
      },
      async write(batch) {
        terminalAttempts.push(structuredClone(batch));
        if (rejectTerminalWrite) {
          rejectTerminalWrite = false;
          throw new Error("terminal write unavailable");
        }
        return { status: "accepted" };
      },
    },
    calculateClose(state) {
      closeCalculations += 1;
      return { events: [event("session-closed", state.count + 1)] };
    },
    calculateTick() {
      throw new Error("advance is not used by this test");
    },
  });

  await assert.rejects(emulator.close(), /terminal write unavailable/);
  assert.deepEqual(emulator.status(), {
    phase: "closing",
    lastError: { operation: "write", message: "terminal write unavailable" },
  });
  await assert.rejects(emulator.advance(), FactoryEmulatorPendingTransactionError);
  await assert.rejects(emulator.close(), /sink close unavailable/);
  assert.deepEqual(emulator.status(), {
    phase: "closing",
    lastError: { operation: "close", message: "sink close unavailable" },
  });
  await emulator.close();

  assert.equal(closeCalculations, 1);
  assert.deepEqual(terminalAttempts, [
    { events: [event("session-closed", 1)] },
    { events: [event("session-closed", 1)] },
  ]);
});

function deferred() {
  let resolve;
  const promise = new Promise((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}
