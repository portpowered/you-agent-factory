import assert from "node:assert/strict";
import test from "node:test";
import {
  FactoryEmulatorConfigurationError,
  FactoryEmulatorDurationError,
  FactoryEmulatorExecutionPausedError,
  FactoryEmulatorLifecycleError,
  FactoryEmulatorPendingCommandError,
  FactoryEmulatorSubmissionError,
  DEFAULT_FACTORY_EMULATOR_LIMITS,
  FACTORY_EMULATOR_LIMIT_HARD_CAPS,
  SUPPORTED_SCENARIO_VERSION,
  createFactoryEmulatorSession,
} from "@you-agent-factory/factory-emulator";

function deferred() {
  let resolve;
  let reject;
  const promise = new Promise((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, reject, resolve };
}

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

function harness(scenarioDocument = scenario(), limits) {
  const batches = [];
  const emulator = createFactoryEmulatorSession({
    factory: supportedFactory(),
    scenario: scenarioDocument,
    limits,
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

function assertPaused(error, kind, limit, configured, observed) {
  assert.ok(error instanceof FactoryEmulatorExecutionPausedError);
  assert.equal(error.code, "EXECUTION_PAUSED");
  assert.deepEqual(error.diagnostic, {
    kind,
    limit,
    configured,
    observed,
    virtualTime: "2026-07-18T07:30:00.000Z",
    virtualElapsedMs: 0,
  });
  return true;
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

test("advanceToNext starts ready Work then completes simultaneous deadlines", async () => {
  const { batches, emulator } = harness(scenario({
    rules: [{
      id: "complete-checkout",
      match: { kind: "workType", workType: "checkout" },
      outcomes: [{ kind: "complete", durationMs: 25 }],
      exhaustionBehavior: { kind: "repeatLast" },
    }],
  }));
  await emulator.start();

  const started = await emulator.advanceToNext();
  assert.equal(started.status, "advanced");
  assert.deepEqual(
    started.batches[0].events.map((event) => event.type),
    ["DISPATCH_REQUEST", "DISPATCH_REQUEST"],
  );
  assert.equal(started.virtualTime, "2026-07-18T07:30:00.000Z");
  assert.ok(started.state.works.every((work) => work.phase === "active"));

  const completed = await emulator.advanceToNext();
  assert.deepEqual(
    completed.batches[0].events.map((event) => event.type),
    ["DISPATCH_RESPONSE", "DISPATCH_RESPONSE"],
  );
  assert.deepEqual(
    completed.batches[0].events.map((event) => event.context.workIds[0]),
    started.state.works.map((work) => work.workId),
  );
  assert.ok(completed.batches[0].events.every(
    (event) => event.context.eventTime === "2026-07-18T07:30:00.025Z",
  ));
  assert.equal(completed.virtualElapsedMs, 25);
  assert.deepEqual(emulator.status(), {
    phase: "idle",
    reason: "no-unfinished-work",
  });

  const idle = await emulator.advanceToNext();
  assert.equal(idle.status, "idle");
  assert.deepEqual(idle.batches, []);
  assert.equal(batches.length, 4);
});

test("advanceBy processes deadlines through an exact virtual target", async () => {
  const { emulator } = harness(scenario({
    rules: [
      {
        id: "slow-checkout",
        match: { kind: "submissionId", submissionId: "checkout-1" },
        outcomes: [{ kind: "complete", durationMs: 20 }],
        exhaustionBehavior: { kind: "repeatLast" },
      },
      {
        id: "fast-checkout",
        match: { kind: "submissionId", submissionId: "checkout-2" },
        outcomes: [{ kind: "reject", reason: "scripted", durationMs: 10 }],
        exhaustionBehavior: { kind: "repeatLast" },
      },
    ],
  }));
  await emulator.start();

  const first = await emulator.advanceBy(10);
  assert.deepEqual(
    first.batches.map((batch) => batch.events.map((event) => event.type)),
    [
      ["DISPATCH_REQUEST", "DISPATCH_REQUEST"],
      ["DISPATCH_RESPONSE"],
    ],
  );
  assert.equal(first.virtualTime, "2026-07-18T07:30:00.010Z");
  assert.deepEqual(
    first.state.works.map((work) => work.phase),
    ["active", "completed"],
  );
  assert.equal(first.state.works[1].rejectionReason, "scripted");

  const second = await emulator.advanceBy(10);
  assert.equal(second.batches.length, 1);
  assert.equal(
    second.batches[0].events[0].context.eventTime,
    "2026-07-18T07:30:00.020Z",
  );
  assert.equal(second.virtualElapsedMs, 20);
  assert.ok(second.state.works.every((work) => work.phase === "completed"));
});

test("advancement changes only virtual time when waiting and is a no-op when idle", async () => {
  const waiting = harness(scenario({ rules: [] }));
  await waiting.emulator.start();
  const waited = await waiting.emulator.advanceBy(50);
  assert.equal(waited.status, "advanced");
  assert.deepEqual(waited.batches, []);
  assert.equal(waited.virtualTime, "2026-07-18T07:30:00.050Z");
  assert.deepEqual(waiting.emulator.status(), {
    phase: "waiting",
    reason: "work-waiting",
  });

  const idle = harness(scenario({ initialSubmissions: [] }));
  await idle.emulator.start();
  const before = idle.emulator.state();
  const receipt = await idle.emulator.advanceBy(50);
  assert.equal(receipt.status, "idle");
  assert.deepEqual(receipt.batches, []);
  assert.deepEqual(receipt.state, before);
});

test("advanceBy rejects invalid and unrepresentable durations without changing state", async () => {
  const { emulator } = harness();
  await emulator.start();
  const before = emulator.state();

  for (const duration of [-1, Number.POSITIVE_INFINITY, 0.5, Number.MAX_SAFE_INTEGER + 1]) {
    await assert.rejects(
      emulator.advanceBy(duration),
      (error) => error instanceof FactoryEmulatorDurationError
        && error.code === "INVALID_DURATION",
    );
  }
  await assert.rejects(
    emulator.advanceBy(Number.MAX_SAFE_INTEGER),
    FactoryEmulatorDurationError,
  );
  assert.deepEqual(emulator.state(), before);
});

test("host pacing does not affect virtual-time batches or state", async () => {
  const pacedScenario = scenario({
    rules: [{
      id: "complete-checkout",
      match: { kind: "workType", workType: "checkout" },
      outcomes: [{ kind: "complete", durationMs: 5 }],
      exhaustionBehavior: { kind: "repeatLast" },
    }],
  });
  const immediate = harness(pacedScenario);
  const paced = harness(pacedScenario);
  await immediate.emulator.start();
  await paced.emulator.start();
  const immediateReceipt = await immediate.emulator.advanceBy(5);
  await Promise.resolve();
  await Promise.resolve();
  const pacedReceipt = await paced.emulator.advanceBy(5);

  assert.equal(JSON.stringify(pacedReceipt), JSON.stringify(immediateReceipt));
  assert.equal(JSON.stringify(paced.batches), JSON.stringify(immediate.batches));
});

test("exhausted rules defer ignored Work without inventing a dispatch", async () => {
  const { emulator } = harness(scenario({
    rules: [{
      id: "complete-once",
      match: { kind: "workType", workType: "checkout" },
      outcomes: [{ kind: "complete" }],
      exhaustionBehavior: { kind: "useUnmatchedBehavior" },
    }],
  }));
  await emulator.start();

  const started = await emulator.advanceToNext();
  assert.equal(started.batches.length, 1);
  assert.equal(started.batches[0].events.length, 1);
  assert.deepEqual(
    started.state.works.map((work) => work.phase),
    ["active", "waiting"],
  );
  await emulator.advanceToNext();
  assert.deepEqual(emulator.status(), {
    phase: "waiting",
    reason: "work-waiting",
  });
});

test("session publishes a submission only after acceptance and retries rejection unchanged", async () => {
  const writes = [];
  const blockedWrite = deferred();
  let blockNextWrite = false;
  const emulator = createFactoryEmulatorSession({
    factory: supportedFactory(),
    scenario: scenario({ initialSubmissions: [] }),
    sink: {
      async close() {
        return { status: "closed" };
      },
      async write(batch) {
        writes.push(structuredClone(batch));
        if (blockNextWrite) {
          blockNextWrite = false;
          await blockedWrite.promise;
        }
        return { status: "accepted" };
      },
    },
  });
  await emulator.start();
  const before = emulator.state();
  blockNextWrite = true;
  const submission = { id: "interactive", workType: "checkout", input: { value: 1 } };
  const pendingSubmit = emulator.submit(submission);
  await Promise.resolve();

  assert.deepEqual(emulator.state(), before);
  assert.deepEqual(emulator.status(), { phase: "active", reason: "submitting" });
  await assert.rejects(
    emulator.advanceToNext(),
    (error) => error instanceof FactoryEmulatorLifecycleError
      && error.phase === "submitting",
  );
  blockedWrite.reject(new Error("recording unavailable"));
  await assert.rejects(pendingSubmit, /recording unavailable/);

  const rejectedBatch = structuredClone(writes.at(-1));
  assert.deepEqual(emulator.state(), before);
  assert.deepEqual(emulator.pending(), rejectedBatch);
  assert.deepEqual(emulator.status(), {
    phase: "error",
    reason: "SINK_WRITE_REJECTED",
  });
  await assert.rejects(
    emulator.advanceToNext(),
    (error) => error instanceof FactoryEmulatorPendingCommandError
      && error.code === "PENDING_TRANSACTION"
      && error.pendingCommand === "submit",
  );

  const receipt = await emulator.submit(structuredClone(submission));
  assert.deepEqual(writes.at(-1), rejectedBatch);
  assert.deepEqual(receipt.batch, rejectedBatch);
  assert.equal(receipt.state.works.length, 1);
  assert.equal(emulator.pending(), undefined);
});

test("advanceBy resumes an original interval after an intermediate batch rejects", async () => {
  const attempts = [];
  let rejectAttempt = 5;
  const emulator = createFactoryEmulatorSession({
    factory: supportedFactory(),
    scenario: scenario({
      rules: [
        {
          id: "first",
          match: { kind: "submissionId", submissionId: "checkout-1" },
          outcomes: [{ kind: "complete", durationMs: 10 }],
          exhaustionBehavior: { kind: "repeatLast" },
        },
        {
          id: "second",
          match: { kind: "submissionId", submissionId: "checkout-2" },
          outcomes: [{ kind: "complete", durationMs: 20 }],
          exhaustionBehavior: { kind: "repeatLast" },
        },
      ],
    }),
    sink: {
      async close() {
        return { status: "closed" };
      },
      async write(batch) {
        attempts.push(structuredClone(batch));
        if (attempts.length === rejectAttempt) {
          rejectAttempt = -1;
          throw new Error("completion write rejected");
        }
        return { status: "accepted" };
      },
    },
  });
  await emulator.start();
  await assert.rejects(emulator.advanceBy(20), /completion write rejected/);
  const rejected = structuredClone(attempts.at(-1));
  assert.equal(emulator.state().virtualElapsedMs, 10);

  const receipt = await emulator.advanceBy(20);
  assert.deepEqual(attempts[4], rejected);
  assert.equal(receipt.virtualElapsedMs, 20);
  assert.equal(receipt.batches.length, 3);
  assert.ok(receipt.state.works.every((work) => work.phase === "completed"));
});

test("reset discards a rejected start transaction and preserves deterministic reruns", async () => {
  const attempts = [];
  let rejectInitial = true;
  const emulator = createFactoryEmulatorSession({
    factory: supportedFactory(),
    scenario: scenario(),
    sink: {
      async close() {
        return { status: "closed" };
      },
      async write(batch) {
        attempts.push(structuredClone(batch));
        if (rejectInitial && batch.events[0].type === "WORK_REQUEST") {
          rejectInitial = false;
          throw new Error("initial Work rejected");
        }
        return { status: "accepted" };
      },
    },
  });
  await assert.rejects(emulator.start(), /initial Work rejected/);
  const firstBootstrap = structuredClone(attempts[0]);
  const rejectedInitial = structuredClone(attempts[1]);
  assert.equal(emulator.state().works.length, 0);

  emulator.reset();
  const receipt = await emulator.start();
  assert.deepEqual(receipt.batches, [firstBootstrap, rejectedInitial]);
  assert.deepEqual(attempts.slice(2), [firstBootstrap, rejectedInitial]);
});

test("close retries terminal write and then only sink close without duplicate events", async () => {
  const writeAttempts = [];
  let rejectTerminal = true;
  let closeAttempts = 0;
  const emulator = createFactoryEmulatorSession({
    factory: supportedFactory(),
    scenario: scenario({ initialSubmissions: [] }),
    sink: {
      async close() {
        closeAttempts += 1;
        if (closeAttempts === 1) {
          throw new Error("close unavailable");
        }
        return { status: "closed" };
      },
      async write(batch) {
        writeAttempts.push(structuredClone(batch));
        if (batch.events[0].type === "RUN_RESPONSE" && rejectTerminal) {
          rejectTerminal = false;
          throw new Error("terminal write unavailable");
        }
        return { status: "accepted" };
      },
    },
  });
  await emulator.start();
  const beforeClose = emulator.state();

  await assert.rejects(emulator.close(), /terminal write unavailable/);
  const terminal = structuredClone(writeAttempts.at(-1));
  assert.deepEqual(emulator.state(), beforeClose);
  await assert.rejects(emulator.submit({ id: "later", workType: "checkout" }),
    FactoryEmulatorPendingCommandError);

  await assert.rejects(emulator.close(), /close unavailable/);
  assert.deepEqual(writeAttempts.at(-1), terminal);
  assert.equal(emulator.state().lifecycle, "closed");
  assert.deepEqual(emulator.status(), {
    phase: "error",
    reason: "SINK_CLOSE_REJECTED",
  });

  const writesBeforeCloseOnlyRetry = writeAttempts.length;
  const receipt = await emulator.close();
  assert.equal(writeAttempts.length, writesBeforeCloseOnlyRetry);
  assert.equal(closeAttempts, 2);
  assert.equal(receipt.status, "closed");
  assert.equal(receipt.batch.events[0].type, "RUN_RESPONSE");
  assert.deepEqual(emulator.status(), { phase: "closed", reason: "session-closed" });
  await assert.rejects(emulator.close(), FactoryEmulatorLifecycleError);
  await assert.rejects(
    emulator.submit({ id: "post-close", workType: "checkout" }),
    FactoryEmulatorLifecycleError,
  );
  assert.equal(writeAttempts.length, writesBeforeCloseOnlyRetry);
});

test("limits publish stable defaults and reject invalid policy before bootstrap", () => {
  assert.deepEqual(DEFAULT_FACTORY_EMULATOR_LIMITS, {
    maxCompletedDispatches: 1_000,
    maxEvents: 10_000,
    maxVirtualElapsedMs: 3_600_000,
    maxZeroDurationBatches: 1_000,
    maxSynchronousBatches: 100,
  });
  assert.ok(FACTORY_EMULATOR_LIMIT_HARD_CAPS.maxEvents > 10_000);

  for (const limits of [
    { maxEvents: 0 },
    { maxCompletedDispatches: 0.5 },
    { maxVirtualElapsedMs: Number.POSITIVE_INFINITY },
    { maxEvents: FACTORY_EMULATOR_LIMIT_HARD_CAPS.maxEvents + 1 },
    { unsupported: 1 },
  ]) {
    assert.throws(
      () => harness(scenario(), limits),
      (error) => error instanceof FactoryEmulatorConfigurationError
        && error.diagnostics[0].code === "INVALID_LIMIT_CONFIGURATION",
    );
  }
});

test("event budget accepts the exact limit and pauses before the next batch", async () => {
  const { batches, emulator } = harness(
    scenario({ initialSubmissions: [] }),
    { maxEvents: 3 },
  );
  await emulator.start();
  await emulator.submit({ id: "within-limit", workType: "checkout" });
  const before = emulator.state();
  assert.equal(before.counters.events, 3);

  await assert.rejects(
    emulator.submit({ id: "over-limit", workType: "checkout" }),
    (error) => assertPaused(error, "budget-exceeded", "events", 3, 4),
  );
  assert.deepEqual(emulator.state(), before);
  assert.equal(batches.flatMap((batch) => batch.events).length, 3);
  assert.deepEqual(emulator.status(), {
    phase: "error",
    reason: "budget-exceeded",
    diagnostic: {
      kind: "budget-exceeded",
      limit: "events",
      configured: 3,
      observed: 4,
      virtualTime: "2026-07-18T07:30:00.000Z",
      virtualElapsedMs: 0,
    },
  });
});

test("completed-dispatch budget preserves the exact boundary and rejects a whole completion batch", async () => {
  const { batches, emulator } = harness(
    scenario({ initialSubmissions: [{ id: "first", workType: "checkout" }] }),
    { maxCompletedDispatches: 1 },
  );
  await emulator.start();
  await emulator.advanceBy(0);
  assert.equal(emulator.state().counters.completions, 1);

  await emulator.submit({ id: "second", workType: "checkout" });
  await emulator.advanceToNext();
  const before = emulator.state();
  const acceptedEvents = batches.flatMap((batch) => batch.events).length;
  await assert.rejects(
    emulator.advanceToNext(),
    (error) => assertPaused(
      error,
      "budget-exceeded",
      "completedDispatches",
      1,
      2,
    ),
  );
  assert.deepEqual(emulator.state(), before);
  assert.equal(batches.flatMap((batch) => batch.events).length, acceptedEvents);
  assert.equal(before.works.at(-1).phase, "active");
  assert.ok(batches.flatMap((batch) => batch.events).every(
    (event) => event.type !== "WORK_FAILED",
  ));
});

test("virtual-time budget succeeds exactly at its limit and pauses before a later deadline", async () => {
  const timedScenario = scenario({
    initialSubmissions: [{ id: "first", workType: "checkout" }],
    rules: [{
      id: "complete-checkout",
      match: { kind: "workType", workType: "checkout" },
      outcomes: [{ kind: "complete", durationMs: 10 }],
      exhaustionBehavior: { kind: "repeatLast" },
    }],
  });
  const { emulator } = harness(timedScenario, { maxVirtualElapsedMs: 10 });
  await emulator.start();
  await emulator.advanceBy(10);
  assert.equal(emulator.state().virtualElapsedMs, 10);
  await emulator.submit({ id: "second", workType: "checkout" });
  await emulator.advanceToNext();
  const before = emulator.state();

  await assert.rejects(
    emulator.advanceToNext(),
    (error) => {
      assert.ok(error instanceof FactoryEmulatorExecutionPausedError);
      assert.deepEqual(error.diagnostic, {
        kind: "budget-exceeded",
        limit: "virtualElapsedMs",
        configured: 10,
        observed: 20,
        virtualTime: "2026-07-18T07:30:00.010Z",
        virtualElapsedMs: 10,
      });
      return true;
    },
  );
  assert.deepEqual(emulator.state(), before);
});

test("zero-duration chains complete at the configured boundary and pause reproducibly beyond it", async () => {
  const finite = harness(scenario(), { maxZeroDurationBatches: 2 });
  await finite.emulator.start();
  await finite.emulator.advanceBy(0);
  assert.ok(finite.emulator.state().works.every((work) => work.phase === "completed"));

  const paused = harness(scenario(), { maxZeroDurationBatches: 1 });
  await paused.emulator.start();
  const before = paused.emulator.state();
  let firstDiagnostic;
  await assert.rejects(paused.emulator.advanceBy(0), (error) => {
    assert.ok(error instanceof FactoryEmulatorExecutionPausedError);
    firstDiagnostic = structuredClone(error.diagnostic);
    assert.deepEqual(firstDiagnostic, {
      kind: "zero-duration-cycle",
      limit: "zeroDurationBatches",
      configured: 1,
      observed: 2,
      virtualTime: "2026-07-18T07:30:00.000Z",
      virtualElapsedMs: 0,
    });
    return true;
  });
  const firstPausedState = paused.emulator.state();
  assert.equal(paused.emulator.state().counters.completions, 0);
  assert.ok(paused.emulator.state().works.every((work) => work.phase === "active"));

  paused.emulator.reset();
  await paused.emulator.start();
  await assert.rejects(paused.emulator.advanceBy(0), (error) => {
    assert.deepEqual(error.diagnostic, firstDiagnostic);
    return true;
  });
  assert.notDeepEqual(firstPausedState, before);
  assert.deepEqual(paused.emulator.state(), firstPausedState);
  const closed = await paused.emulator.close();
  assert.equal(closed.status, "closed");
});

test("cooperative scheduler yielding keeps the active command serialized", async () => {
  const { emulator } = harness(scenario({
    initialSubmissions: [{ id: "timed", workType: "checkout" }],
    rules: [{
      id: "timed",
      match: { kind: "all" },
      outcomes: [{ kind: "complete", durationMs: 1 }],
      exhaustionBehavior: { kind: "repeatLast" },
    }],
  }), { maxSynchronousBatches: 1 });
  await emulator.start();

  let statusAtBoundary;
  let overlappingError;
  const advance = emulator.advanceBy(1);
  queueMicrotask(async () => {
    statusAtBoundary = emulator.status();
    try {
      await emulator.submit({ id: "overlap", workType: "checkout" });
    } catch (error) {
      overlappingError = error;
    }
  });
  await advance;
  await Promise.resolve();

  assert.deepEqual(statusAtBoundary, { phase: "active", reason: "advancing" });
  assert.ok(overlappingError instanceof FactoryEmulatorLifecycleError);
  assert.equal(overlappingError.phase, "advancing");
  assert.equal(emulator.state().virtualElapsedMs, 1);
});
