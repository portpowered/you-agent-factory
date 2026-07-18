import assert from "node:assert/strict";
import test from "node:test";
import {
  FactoryEmulatorConfigurationError,
  FactoryEmulatorLifecycleError,
  SUPPORTED_SCENARIO_VERSION,
  createFactoryEmulatorSession,
} from "@you-agent-factory/factory-emulator";

function supportedFactory() {
  return {
    name: "checkout",
    workTypes: [{ name: "checkout", states: [] }],
    workstations: [{ name: "complete", worker: "emulator", inputs: [] }],
  };
}

function scenario(overrides = {}) {
  return {
    version: SUPPORTED_SCENARIO_VERSION,
    id: "deterministic-checkout",
    seed: "seed-0001",
    startAt: "2026-07-18T07:30:00Z",
    initialSubmissions: [
      { id: "checkout-1", workType: "checkout", input: { total: 42 } },
      { id: "checkout-2", workType: "checkout", input: { total: 84 } },
    ],
    rules: [{
      id: "complete-checkout",
      match: { kind: "workType", workType: "checkout" },
      outcomes: [{ kind: "complete" }],
      exhaustionBehavior: { kind: "repeatLast" },
    }],
    unmatchedBehavior: { kind: "ignore" },
    ...overrides,
  };
}

function harness(scenarioDocument = scenario()) {
  const batches = [];
  const emulator = createFactoryEmulatorSession({
    factory: supportedFactory(),
    scenario: scenarioDocument,
    sink: {
      async close() {
        return { status: "closed" };
      },
      async write(batch) {
        batches.push(structuredClone(batch));
        return { status: "accepted" };
      },
    },
  });
  return { batches, emulator };
}

test("start emits deterministic bootstrap and initial submission batches at virtual time zero", async () => {
  const { batches, emulator } = harness();

  assert.deepEqual(emulator.state(), {
    lifecycle: "pre-start",
    virtualElapsedMs: 0,
    works: [],
    ruleCursors: {},
    counters: {
      commands: 0,
      events: 0,
      requests: 0,
      works: 0,
      dispatches: 0,
      completions: 0,
    },
  });

  const receipt = await emulator.start();

  assert.deepEqual(
    batches.map((batch) => batch.events.map((event) => event.type)),
    [["INITIAL_STRUCTURE_REQUEST", "RUN_REQUEST"], ["WORK_REQUEST"]],
  );
  assert.deepEqual(receipt.batches, batches);
  const events = batches.flatMap((batch) => batch.events);
  assert.deepEqual(events.map((event) => event.context.sequence), [0, 1, 2]);
  assert.ok(events.every(
    (event) => event.context.eventTime === "2026-07-18T07:30:00.000Z",
  ));
  assert.ok(events.every(
    (event) => event.context.sessionId === receipt.state.sessionId,
  ));
  assert.deepEqual(receipt.state, emulator.state());
  assert.deepEqual(receipt.state.works.map(({ submissionId, workType, input }) => ({
    submissionId,
    workType,
    input,
  })), [
    { submissionId: "checkout-1", workType: "checkout", input: { total: 42 } },
    { submissionId: "checkout-2", workType: "checkout", input: { total: 84 } },
  ]);
  assert.equal(new Set(events.map((event) => event.id)).size, events.length);
  assert.equal(new Set(receipt.state.works.map((work) => work.workId)).size, 2);
  assert.equal(new Set(receipt.state.works.map((work) => work.traceId)).size, 2);
});

test("invalid lifecycle commands and unsupported configuration emit no events", async () => {
  const { batches, emulator } = harness();

  assert.throws(
    () => emulator.reset(),
    (error) => error instanceof FactoryEmulatorLifecycleError
      && error.code === "INVALID_LIFECYCLE"
      && error.command === "reset"
      && error.phase === "pre-start",
  );
  await emulator.start();
  const acceptedCount = batches.length;
  await assert.rejects(
    emulator.start(),
    (error) => error instanceof FactoryEmulatorLifecycleError
      && error.code === "INVALID_LIFECYCLE"
      && error.command === "start"
      && error.phase === "started",
  );
  assert.equal(batches.length, acceptedCount);

  const invalid = harness(scenario({
    rules: [{
      id: "unknown-work-type",
      match: { kind: "workType", workType: "missing" },
      outcomes: [{ kind: "complete" }],
      exhaustionBehavior: { kind: "repeatLast" },
    }],
  }));
  await assert.rejects(
    invalid.emulator.start(),
    (error) => error instanceof FactoryEmulatorConfigurationError
      && error.code === "INVALID_CONFIGURATION"
      && error.diagnostics.some(
        (diagnostic) => diagnostic.code === "UNKNOWN_FACTORY_WORK_TYPE",
      ),
  );
  assert.deepEqual(invalid.batches, []);
  assert.equal(invalid.emulator.state().lifecycle, "pre-start");
});

test("reset and rerun reproduce event bytes and committed snapshots", async () => {
  const { batches, emulator } = harness();
  const first = await emulator.start();
  const firstBatches = structuredClone(batches);

  const reset = emulator.reset();
  assert.equal(reset.status, "reset");
  assert.equal(reset.state.lifecycle, "pre-start");
  assert.equal(batches.length, firstBatches.length);

  const second = await emulator.start();
  const secondBatches = batches.slice(firstBatches.length);

  assert.equal(JSON.stringify(secondBatches), JSON.stringify(firstBatches));
  assert.equal(JSON.stringify(second.state), JSON.stringify(first.state));
});

test("changing only the seed creates a distinct deterministic identity stream", async () => {
  const first = harness(scenario({ seed: "seed-a" }));
  const second = harness(scenario({ seed: "seed-b" }));

  const firstReceipt = await first.emulator.start();
  const secondReceipt = await second.emulator.start();
  const firstEvents = first.batches.flatMap((batch) => batch.events);
  const secondEvents = second.batches.flatMap((batch) => batch.events);

  assert.notEqual(firstReceipt.state.sessionId, secondReceipt.state.sessionId);
  assert.notDeepEqual(
    firstEvents.map((event) => event.id),
    secondEvents.map((event) => event.id),
  );
  assert.notDeepEqual(
    firstReceipt.state.works.map((work) => [work.requestId, work.traceId, work.workId]),
    secondReceipt.state.works.map((work) => [work.requestId, work.traceId, work.workId]),
  );
  assert.deepEqual(
    firstEvents.map((event) => event.context.eventTime),
    secondEvents.map((event) => event.context.eventTime),
  );
});
