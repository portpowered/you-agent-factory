import assert from "node:assert/strict";
import test from "node:test";
import {
  FactoryEmulatorConfigurationError,
  FactoryEmulatorLifecycleError,
  FactoryEmulatorSubmissionError,
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

test("submit accepts single and batch Work requests in deterministic order", async () => {
  const first = harness();
  const second = harness();
  await first.emulator.start();
  await second.emulator.start();

  const single = { id: "checkout-3", workType: "checkout", input: { total: 126 } };
  const batch = [
    { id: "checkout-4", workType: "checkout", input: { total: 168 } },
    { id: "checkout-5", workType: "checkout", input: { total: 210 } },
  ];
  const firstSingle = await first.emulator.submit(single);
  const firstBatch = await first.emulator.submit(batch);
  const secondSingle = await second.emulator.submit(single);
  const secondBatch = await second.emulator.submit(batch);

  assert.equal(firstSingle.status, "submitted");
  assert.deepEqual(firstSingle.batch.events.map((event) => event.type), ["WORK_REQUEST"]);
  assert.deepEqual(
    firstBatch.batch.events[0].payload.works.map((work) => work.name),
    ["checkout-4", "checkout-5"],
  );
  assert.equal(firstBatch.state.counters.commands, 3);
  assert.equal(firstBatch.state.counters.requests, 3);
  assert.equal(firstBatch.state.counters.works, 5);
  assert.deepEqual(first.emulator.status(), { phase: "ready", reason: "work-ready" });
  assert.equal(JSON.stringify(firstSingle), JSON.stringify(secondSingle));
  assert.equal(JSON.stringify(firstBatch), JSON.stringify(secondBatch));

  const submittedWorks = firstBatch.state.works.slice(2);
  assert.equal(new Set(submittedWorks.map((work) => work.requestId)).size, 2);
  assert.equal(new Set(submittedWorks.map((work) => work.traceId)).size, 3);
  assert.equal(new Set(submittedWorks.map((work) => work.workId)).size, 3);

  first.emulator.reset();
  const restarted = await first.emulator.start();
  assert.deepEqual(
    restarted.state.works.map((work) => work.submissionId),
    ["checkout-1", "checkout-2"],
  );
});

test("submit rejects an invalid batch without events, counters, or Work", async () => {
  const { batches, emulator } = harness();

  await assert.rejects(
    emulator.submit({ id: "too-early", workType: "checkout" }),
    (error) => error instanceof FactoryEmulatorLifecycleError
      && error.command === "submit"
      && error.phase === "pre-start",
  );
  await emulator.start();
  const before = emulator.state();
  const acceptedCount = batches.length;

  await assert.rejects(
    emulator.submit([
      { id: "valid", workType: "checkout" },
      { id: "invalid", workType: "missing" },
    ]),
    (error) => error instanceof FactoryEmulatorSubmissionError
      && error.code === "INVALID_SUBMISSION"
      && error.diagnostics.some(
        (diagnostic) => diagnostic.path === "/submissions/1/workType",
      ),
  );
  await assert.rejects(
    emulator.submit([]),
    (error) => error instanceof FactoryEmulatorSubmissionError
      && error.diagnostics[0].path === "/submissions",
  );

  assert.equal(batches.length, acceptedCount);
  assert.deepEqual(emulator.state(), before);
});

test("an idle session exposes submission atomically and returns to ready", async () => {
  let releaseWrite;
  let gateSubmissions = false;
  const writes = [];
  const emulator = createFactoryEmulatorSession({
    factory: supportedFactory(),
    scenario: scenario({ initialSubmissions: [] }),
    sink: {
      async close() {
        return { status: "closed" };
      },
      async write(batch) {
        writes.push(structuredClone(batch));
        if (gateSubmissions) {
          await new Promise((resolve) => {
            releaseWrite = resolve;
          });
        }
        return { status: "accepted" };
      },
    },
  });
  await emulator.start();
  assert.deepEqual(emulator.status(), {
    phase: "idle",
    reason: "no-unfinished-work",
  });

  gateSubmissions = true;
  const before = emulator.state();
  const pendingSubmit = emulator.submit({
    id: "checkout-after-idle",
    workType: "checkout",
  });
  await Promise.resolve();

  assert.deepEqual(emulator.status(), { phase: "active", reason: "submitting" });
  assert.deepEqual(emulator.state(), before);
  assert.equal(writes.length, 2);
  releaseWrite();
  await pendingSubmit;

  assert.deepEqual(emulator.status(), { phase: "ready", reason: "work-ready" });
  assert.deepEqual(
    emulator.state().works.map((work) => work.submissionId),
    ["checkout-after-idle"],
  );
});

test("status distinguishes blocked unfinished Work from an idle session", async () => {
  const { emulator } = harness(scenario({ rules: [] }));

  await emulator.start();

  assert.ok(emulator.state().works.every((work) => work.phase === "waiting"));
  assert.deepEqual(emulator.status(), {
    phase: "waiting",
    reason: "work-waiting",
  });
});
