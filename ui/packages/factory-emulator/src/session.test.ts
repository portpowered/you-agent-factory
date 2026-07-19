// biome-ignore-all lint/style/noExcessiveLinesPerFile: Lifecycle and execution assertions share one public-session fixture.
import type {
  FactoryDefinition,
  FactoryEvent,
} from "@you-agent-factory/client";
import { describe, expect, expectTypeOf, it } from "vitest";
import type { FactoryEventSink } from "./event-sink.js";
import { RecordingFactoryEventSink } from "./recording-sink.js";
import type { FactoryEmulatorScenario } from "./scenario.js";
import {
  createFactoryEmulatorSession,
  DEFAULT_FACTORY_EMULATOR_LIMITS,
  FACTORY_EMULATOR_LIMIT_HARD_CAPS,
  FactoryEmulatorConfigurationError,
  FactoryEmulatorDurationError,
  FactoryEmulatorLifecycleError,
  FactoryEmulatorPendingCommandError,
  type FactoryEmulatorSession,
  type FactoryEmulatorSubmissionError,
} from "./session.js";

const factory = {
  name: "lifecycle-factory",
  orchestrator: { kind: "PETRI" },
  workTypes: [
    {
      name: "task",
      states: [{ name: "ready", type: "INITIAL" }],
    },
  ],
} satisfies FactoryDefinition;

const scenario = {
  schemaVersion: "factory-emulator-scenario/v1",
  id: "lifecycle-scenario",
  factory: { name: "lifecycle-factory" },
  seed: "lifecycle-seed",
  startAt: "2026-07-18T16:00:00.000Z",
  rules: [],
  unmatched: { behavior: "error" },
} satisfies FactoryEmulatorScenario;

function deferred() {
  let resolvePromise: (() => void) | undefined;
  const promise = new Promise<void>((resolve) => {
    resolvePromise = resolve;
  });
  return { promise, resolve: () => resolvePromise?.() };
}

function harness(
  sink: FactoryEventSink = { write: async () => undefined },
  scenarioOverride: FactoryEmulatorScenario = scenario,
) {
  return createFactoryEmulatorSession({
    factory,
    scenario: scenarioOverride,
    sink,
  });
}

describe("Factory emulator session lifecycle", () => {
  it("exports the typed public lifecycle factory", () => {
    expectTypeOf(
      createFactoryEmulatorSession,
    ).returns.toMatchTypeOf<FactoryEmulatorSession>();
    expect(DEFAULT_FACTORY_EMULATOR_LIMITS).toMatchObject({
      maxCompletedDispatches: 1_000,
      maxEvents: 10_000,
      maxVirtualElapsedMs: 3_600_000,
    });
    expect(FACTORY_EMULATOR_LIMIT_HARD_CAPS.maxEvents).toBeGreaterThan(
      DEFAULT_FACTORY_EMULATOR_LIMITS.maxEvents,
    );
  });

  it("validates Factory, scenario, sink, limits, and yield options before writing", () => {
    let writes = 0;
    const create = () =>
      createFactoryEmulatorSession({
        factory,
        scenario: {
          ...scenario,
          factory: { name: "a-different-factory" },
        },
        sink: {
          write: async () => {
            writes += 1;
          },
        },
        limits: { maxEvents: 0 },
        yieldControl: "later" as never,
      });

    expect(create).toThrow(FactoryEmulatorConfigurationError);
    try {
      create();
    } catch (error) {
      expect(error).toBeInstanceOf(FactoryEmulatorConfigurationError);
      if (!(error instanceof FactoryEmulatorConfigurationError)) return;
      expect(error.diagnostics.map(({ code }) => code)).toEqual(
        expect.arrayContaining([
          "invalid_factory_identity",
          "invalid_limit",
          "invalid_yield_control",
        ]),
      );
    }
    expect(writes).toBe(0);
  });
});

const executionFactory = {
  name: "execution-factory",
  orchestrator: { kind: "PETRI" },
  workTypes: [
    {
      name: "task",
      states: [
        { name: "ready", type: "INITIAL" },
        { name: "done", type: "TERMINAL" },
      ],
    },
  ],
  workers: [{ name: "scripted-worker", type: "AGENT_WORKER" }],
  workstations: [
    {
      name: "scripted-run",
      type: "AGENT_RUN",
      worker: "scripted-worker",
      inputs: [{ workType: "task", state: "ready" }],
      outputs: [{ workType: "task", state: "done" }],
    },
  ],
} satisfies FactoryDefinition;

function executionScenario(
  overrides: Partial<FactoryEmulatorScenario> = {},
): FactoryEmulatorScenario {
  return {
    schemaVersion: "factory-emulator-scenario/v1",
    id: "execution-scenario",
    factory: { name: "execution-factory" },
    seed: "execution-seed",
    startAt: "2026-07-18T16:00:00.000Z",
    rules: [
      {
        id: "scripted-outcome",
        selector: { workstation: "scripted-run" },
        cursor: { scope: "lineage", input: "rootWorkId" },
        outcomes: [{ result: "accepted", durationMs: 25 }],
        exhaustion: "repeat-last",
      },
    ],
    unmatched: { behavior: "error" },
    ...overrides,
  };
}

function executionHarness(
  scenarioOverride = executionScenario(),
  sink: FactoryEventSink = { write: async () => undefined },
) {
  return createFactoryEmulatorSession({
    factory: executionFactory,
    scenario: scenarioOverride,
    sink,
  });
}

describe("Factory emulator Work submission", () => {
  it("normalizes initial, active, and idle submissions into atomic Work requests", async () => {
    const sink = new RecordingFactoryEventSink({
      recording: {
        schemaVersion: "factory-recording/v1",
        id: "execution-recording",
        title: "Execution recording",
      },
      maxEvents: 30,
    });
    const emulator = executionHarness(
      executionScenario({
        initialSubmissions: [
          { name: "initial", workType: "task", state: "ready" },
        ],
      }),
      sink,
    );

    const started = await emulator.start();
    expect(
      started.batches.map((batch) => batch.map(({ type }) => type)),
    ).toEqual([
      ["RUN_REQUEST", "INITIAL_STRUCTURE_REQUEST", "SESSION_STARTED"],
      ["WORK_REQUEST"],
    ]);
    await emulator.advanceToNext();
    expect(emulator.status().phase).toBe("active");

    const activeSubmission = await emulator.submit([
      { name: "active-one", workType: "task", state: "ready", input: "one" },
      { name: "active-two", workType: "task", state: "ready", input: "two" },
    ]);
    expect(activeSubmission.batch).toHaveLength(1);
    const workRequest = activeSubmission.batch[0];
    expect(workRequest?.type).toBe("WORK_REQUEST");
    if (workRequest === undefined)
      throw new Error("missing Work request event");
    expect(
      (workRequest.payload as { works: { name: string }[] }).works.map(
        ({ name }) => name,
      ),
    ).toEqual(["active-one", "active-two"]);

    await emulator.advanceBy(25);
    expect(emulator.status().phase).toBe("idle");
    await emulator.submit({
      name: "after-idle",
      workType: "task",
      state: "ready",
    });
    expect(emulator.status().phase).toBe("ready");
    expect(sink.snapshot().events.map(({ type }) => type)).toEqual(
      started.batches
        .flat()
        .map(({ type }) => type)
        .concat([
          "DISPATCH_REQUEST",
          "WORK_REQUEST",
          "DISPATCH_REQUEST",
          "DISPATCH_REQUEST",
          "DISPATCH_RESPONSE",
          "DISPATCH_RESPONSE",
          "DISPATCH_RESPONSE",
          "WORK_REQUEST",
        ]),
    );
  });

  it("rejects a malformed batch before writing or partially accepting Work", async () => {
    const writes: FactoryEvent[][] = [];
    const emulator = executionHarness(
      executionScenario({ initialSubmissions: [] }),
      {
        write: async (events) => {
          writes.push(structuredClone(events) as FactoryEvent[]);
        },
      },
    );
    await emulator.start();
    const before = emulator.state();

    await expect(
      emulator.submit([
        { name: "valid", workType: "task", state: "ready" },
        { name: "invalid", workType: "missing", state: "ready" },
      ]),
    ).rejects.toMatchObject({
      name: "FactoryEmulatorSubmissionError",
      code: "invalid_submission",
      diagnostics: [
        expect.objectContaining({ path: ["submissions", 1, "workType"] }),
      ],
    } satisfies Partial<FactoryEmulatorSubmissionError>);
    expect(emulator.state()).toEqual(before);
    expect(writes).toHaveLength(1);
  });
});

describe("Factory emulator virtual-time advancement", () => {
  it("processes deadlines through an exact target and jumps to the next due instant", async () => {
    const emulator = executionHarness(
      executionScenario({
        initialSubmissions: [
          { name: "slow", workType: "task", state: "ready" },
          { name: "fast", workType: "task", state: "ready" },
        ],
        rules: [
          {
            id: "slow-rule",
            selector: { input: { name: "slow" } },
            cursor: { scope: "lineage", input: "rootWorkId" },
            outcomes: [{ result: "accepted", durationMs: 20 }],
            exhaustion: "repeat-last",
          },
          {
            id: "fast-rule",
            selector: { input: { name: "fast" } },
            cursor: { scope: "lineage", input: "rootWorkId" },
            outcomes: [
              { result: "rejected", durationMs: 10, feedback: "scripted" },
            ],
            exhaustion: "repeat-last",
          },
        ],
      }),
    );
    await emulator.start();

    const partial = await emulator.advanceBy(10);
    expect(
      partial.batches.map((batch) => batch.map(({ type }) => type)),
    ).toEqual([
      ["DISPATCH_REQUEST", "DISPATCH_REQUEST"],
      ["DISPATCH_RESPONSE"],
    ]);
    expect(partial.virtualTime).toBe("2026-07-18T16:00:00.010Z");
    expect(partial.state.works.map(({ phase }) => phase)).toEqual([
      "active",
      "completed",
    ]);

    const next = await emulator.advanceToNext();
    expect(next.virtualElapsedMs).toBe(20);
    expect(next.batches[0]?.[0]?.context.eventTime).toBe(
      "2026-07-18T16:00:00.020Z",
    );
    expect(next.state.works.every(({ phase }) => phase === "completed")).toBe(
      true,
    );
  });

  it("completes simultaneous deadlines in stable Work order and idles without events", async () => {
    const emulator = executionHarness(
      executionScenario({
        initialSubmissions: [
          { name: "first", workType: "task", state: "ready" },
          { name: "second", workType: "task", state: "ready" },
        ],
      }),
    );
    await emulator.start();
    const dispatched = await emulator.advanceToNext();
    const completed = await emulator.advanceToNext();

    expect(
      completed.batches[0]?.map(({ context }) => context.workIds?.[0]),
    ).toEqual(dispatched.state.works.map(({ workId }) => workId));
    expect(completed.virtualElapsedMs).toBe(25);
    const before = emulator.state();
    const idle = await emulator.advanceToNext();
    expect(idle).toMatchObject({ status: "idle", batches: [] });
    expect(emulator.state()).toEqual(before);
  });

  it("rejects invalid durations without changing virtual time or emitting events", async () => {
    const writes: FactoryEvent[][] = [];
    const emulator = executionHarness(undefined, {
      write: async (events) => {
        writes.push(structuredClone(events) as FactoryEvent[]);
      },
    });
    await emulator.start();
    const before = emulator.state();

    for (const duration of [
      -1,
      0.5,
      Number.POSITIVE_INFINITY,
      Number.MAX_SAFE_INTEGER,
    ]) {
      await expect(emulator.advanceBy(duration)).rejects.toBeInstanceOf(
        FactoryEmulatorDurationError,
      );
    }
    expect(emulator.state()).toEqual(before);
    expect(writes).toHaveLength(1);
  });
});

describe("Factory emulator session start", () => {
  it("writes a recording-compatible canonical bootstrap", async () => {
    const sink = new RecordingFactoryEventSink({
      recording: {
        schemaVersion: "factory-recording/v1",
        id: "lifecycle-recording",
        title: "Lifecycle recording",
      },
      maxEvents: 10,
    });
    const emulator = harness(sink);

    const receipt = await emulator.start();

    expect(sink.snapshot().events).toEqual(receipt.batches.flat());
  });

  it("commits canonical bootstrap state only after its atomic sink batch succeeds", async () => {
    const gate = deferred();
    const writes: unknown[] = [];
    const emulator = harness({
      write: async (events) => {
        writes.push(structuredClone(events));
        await gate.promise;
      },
    });

    const starting = emulator.start();
    expect(emulator.state()).toEqual({
      lifecycle: "pre-start",
      virtualElapsedMs: 0,
      counters: { commands: 0, events: 0, completedDispatches: 0 },
    });
    expect(emulator.status()).toMatchObject({
      phase: "active",
      pendingTransaction: {
        command: "start",
        phase: "sink-write",
        eventCount: 3,
      },
    });
    expect(writes).toHaveLength(1);
    gate.resolve();

    const receipt = await starting;
    const events = receipt.batches.flat();
    expect(events.map(({ type }) => type)).toEqual([
      "RUN_REQUEST",
      "INITIAL_STRUCTURE_REQUEST",
      "SESSION_STARTED",
    ]);
    expect(events.map(({ context }) => context.sequence)).toEqual([0, 1, 2]);
    expect(events.map(({ context }) => context.sessionSequence)).toEqual([
      0, 1, 2,
    ]);
    expect(events.every(({ context }) => context.tick === 0)).toBe(true);
    expect(
      events.every(({ context }) => context.eventTime === scenario.startAt),
    ).toBe(true);
    expect(new Set(events.map(({ id }) => id)).size).toBe(3);
    expect(receipt.state).toEqual(emulator.state());
    expect(emulator.status()).toMatchObject({
      phase: "idle",
      virtualTime: scenario.startAt,
      virtualElapsedMs: 0,
      budgetUsage: {
        events: { used: 3, limit: 10_000 },
      },
    });
  });

  it("rejects overlapping and repeated starts without extra writes or state changes", async () => {
    const gate = deferred();
    let writes = 0;
    const emulator = harness({
      write: async () => {
        writes += 1;
        await gate.promise;
      },
    });

    const first = emulator.start();
    await expect(emulator.start()).rejects.toBeInstanceOf(
      FactoryEmulatorPendingCommandError,
    );
    gate.resolve();
    await first;
    const state = emulator.state();
    await expect(emulator.start()).rejects.toBeInstanceOf(
      FactoryEmulatorLifecycleError,
    );
    expect(emulator.state()).toEqual(state);
    expect(writes).toBe(1);
  });
});

describe("Factory emulator session snapshots", () => {
  it("returns detached structured-cloneable state and status snapshots", async () => {
    const emulator = harness();
    const before = emulator.status();
    expect(structuredClone(before)).toEqual(before);
    const mutableBefore = before as { reason: string };
    mutableBefore.reason = "caller mutation";
    expect(emulator.status().reason).toBe("The session is ready to start.");

    await emulator.start();
    const state = emulator.state();
    const status = emulator.status();
    expect(structuredClone(state)).toEqual(state);
    expect(structuredClone(status)).toEqual(status);
    (state.counters as { events: number }).events = 999;
    expect(emulator.state().counters.events).toBe(3);
    expect(Object.hasOwn(state, "events")).toBe(false);
    expect(Object.hasOwn(state, "history")).toBe(false);
  });

  it("keeps an open session ready when validated initial Work exists", async () => {
    const emulator = harness(
      { write: async () => undefined },
      {
        ...scenario,
        initialSubmissions: [
          { name: "later-work", workType: "task", state: "ready" },
        ],
      },
    );

    await emulator.start();
    expect(emulator.status()).toMatchObject({
      phase: "ready",
      reason: "Initial Work is ready for a later execution command.",
    });
  });

  it("preserves pre-start state and exposes a data-only error after sink rejection", async () => {
    const failure = new Error("recording unavailable");
    const emulator = harness({
      write: async () => {
        throw failure;
      },
    });

    await expect(emulator.start()).rejects.toBe(failure);
    expect(emulator.state().lifecycle).toBe("pre-start");
    const status = emulator.status();
    expect(structuredClone(status)).toEqual(status);
    expect(status).toMatchObject({
      phase: "error",
      error: {
        code: "sink_write_rejected",
        operation: "write",
        command: "start",
        message: "recording unavailable",
      },
      pendingTransaction: { eventCount: 3 },
    });
  });
});
