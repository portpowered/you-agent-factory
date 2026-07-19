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
  FactoryEmulatorExecutionPausedError,
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

describe("Factory emulator scheduler dispatch", () => {
  it("starts only the same highest-ranked 50 Work bindings per batch", async () => {
    const boundedScenario = executionScenario({
      initialSubmissions: Array.from({ length: 75 }, (_, index) => ({
        name: `queued-${String(74 - index).padStart(2, "0")}`,
        workType: "task",
        state: "ready",
      })),
    });
    const dispatchOnce = async () => {
      const emulator = executionHarness(boundedScenario);
      await emulator.start();
      return emulator.advanceToNext();
    };

    const first = await dispatchOnce();
    const second = await dispatchOnce();
    const dispatchedWorkIds = (receipt: typeof first) =>
      receipt.batches[0]?.map(({ context }) => context.workIds?.[0]);

    expect(first.batches[0]).toHaveLength(50);
    expect(
      first.state.works.filter(({ phase }) => phase === "active"),
    ).toHaveLength(50);
    expect(
      first.state.works.filter(({ phase }) => phase === "ready"),
    ).toHaveLength(25);
    expect(dispatchedWorkIds(second)).toEqual(dispatchedWorkIds(first));
  });

  it("enumerates competing bindings and gives a logical move the shared token", async () => {
    const competingFactory = {
      ...executionFactory,
      workstations: [
        {
          ...executionFactory.workstations[0],
          name: "a-worker-run",
        },
        {
          name: "z-logical-move",
          type: "LOGICAL_MOVE",
          worker: "",
          inputs: [{ workType: "task", state: "ready" }],
          outputs: [{ workType: "task", state: "done" }],
          guards: [
            {
              type: "VISIT_COUNT",
              workstation: "a-worker-run",
              maxVisits: 1,
            },
          ],
        },
      ],
    } satisfies FactoryDefinition;
    const competingScenario = executionScenario({
      factory: { name: competingFactory.name },
      initialSubmissions: [
        { name: "shared", workType: "task", state: "ready" },
      ],
      rules: [
        {
          id: "worker-outcome",
          selector: { workstation: "a-worker-run" },
          cursor: { scope: "lineage", input: "rootWorkId" },
          outcomes: [{ result: "accepted", durationMs: 1 }],
          exhaustion: "repeat-last",
        },
        {
          id: "logical-outcome",
          selector: { workstation: "z-logical-move" },
          cursor: { scope: "lineage", input: "rootWorkId" },
          outcomes: [{ result: "accepted", durationMs: 0 }],
          exhaustion: "repeat-last",
        },
      ],
    });
    const emulator = createFactoryEmulatorSession({
      factory: competingFactory,
      scenario: competingScenario,
      sink: { write: async () => undefined },
    });

    await emulator.start();
    const dispatched = await emulator.advanceToNext();

    expect(dispatched.batches).toHaveLength(1);
    expect(dispatched.batches[0]).toHaveLength(1);
    expect(dispatched.state.works[0]?.dispatch).toMatchObject({
      workstation: "z-logical-move",
      worker: "",
    });
  });
});

describe("Factory emulator resource-capacity dispatch", () => {
  it("starts independent Work up to capacity and reuses released capacity", async () => {
    const resourceFactory = {
      ...executionFactory,
      resources: [{ name: "agent-slot", capacity: 2 }],
      workers: [
        {
          ...executionFactory.workers[0],
          resources: [{ name: "agent-slot", capacity: 2 }],
        },
      ],
      workstations: [
        {
          ...executionFactory.workstations[0],
          resources: [{ name: "agent-slot", capacity: 1 }],
        },
      ],
    } satisfies FactoryDefinition;
    const resourceScenario = executionScenario({
      initialSubmissions: ["third", "first", "second"].map((name) => ({
        name,
        workType: "task",
        state: "ready",
      })),
    });
    const runFirstBatch = async () => {
      const emulator = createFactoryEmulatorSession({
        factory: resourceFactory,
        scenario: resourceScenario,
        sink: { write: async () => undefined },
      });
      await emulator.start();
      return { emulator, receipt: await emulator.advanceToNext() };
    };

    const first = await runFirstBatch();
    const repeated = await runFirstBatch();
    const activeWorkIds = (receipt: typeof first.receipt) =>
      receipt.state.works
        .filter(({ phase }) => phase === "active")
        .map(({ workId }) => workId);

    expect(activeWorkIds(first.receipt)).toHaveLength(2);
    expect(activeWorkIds(repeated.receipt)).toEqual(
      activeWorkIds(first.receipt),
    );
    expect(first.receipt.batches[0]?.map(({ payload }) => payload)).toEqual([
      expect.objectContaining({
        resources: [{ name: "agent-slot", capacity: 2 }],
      }),
      expect.objectContaining({
        resources: [{ name: "agent-slot", capacity: 2 }],
      }),
    ]);

    const completed = await first.emulator.advanceToNext();
    expect(completed.batches[0]).toHaveLength(2);
    expect(
      completed.state.works.filter(({ phase }) => phase === "ready"),
    ).toHaveLength(1);

    const released = await first.emulator.advanceToNext();
    expect(released.batches[0]).toHaveLength(1);
    expect(
      released.state.works.filter(({ phase }) => phase === "active"),
    ).toHaveLength(1);
  });

  it("leaves Work ready without events when total capacity is insufficient", async () => {
    const resourceFactory = {
      ...executionFactory,
      resources: [{ name: "agent-slot", capacity: 1 }],
      workstations: [
        {
          ...executionFactory.workstations[0],
          resources: [{ name: "agent-slot", capacity: 2 }],
        },
      ],
    } satisfies FactoryDefinition;
    const emulator = createFactoryEmulatorSession({
      factory: resourceFactory,
      scenario: executionScenario({
        initialSubmissions: [
          { name: "blocked", workType: "task", state: "ready" },
        ],
      }),
      sink: { write: async () => undefined },
    });

    await emulator.start();
    const receipt = await emulator.advanceToNext();

    expect(receipt).toMatchObject({ status: "idle", batches: [] });
    expect(receipt.state.works[0]).toMatchObject({
      submissionId: "blocked",
      phase: "ready",
    });
    expect(receipt.state.works[0]?.dispatch).toBeUndefined();
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

const runDeterministicHistory = async (
  factoryInput: FactoryDefinition,
  scenarioInput: FactoryEmulatorScenario,
  runtimeSubmission: { readonly name: string; readonly input: string },
) => {
  const history: FactoryEvent[] = [];
  const emulator = createFactoryEmulatorSession({
    factory: factoryInput,
    scenario: scenarioInput,
    sink: {
      write: async (events) => {
        history.push(...structuredClone(events));
      },
    },
  });
  await emulator.start();
  await emulator.advanceToNext();
  const dispatched = emulator.state();
  await emulator.advanceToNext();
  await emulator.submit({
    ...runtimeSubmission,
    workType: "task",
    state: "ready",
  });
  await emulator.advanceBy(25);
  return { emulator, history, dispatched };
};

const deterministicScenario = executionScenario({
  initialSubmissions: [{ name: "initial", workType: "task", state: "ready" }],
});

describe("Factory emulator deterministic reruns", () => {
  it("reproduces canonical history and cursor state across fresh and reset runs", async () => {
    const first = await runDeterministicHistory(
      executionFactory,
      deterministicScenario,
      { name: "runtime", input: "stable input" },
    );
    const second = await runDeterministicHistory(
      executionFactory,
      deterministicScenario,
      { name: "runtime", input: "stable input" },
    );

    expect(JSON.stringify(second.history)).toBe(JSON.stringify(first.history));
    expect(second.dispatched).toEqual(first.dispatched);
    const sequences = first.history.map(({ context }) => context.sequence);
    expect(sequences).toEqual(sequences.map((_, index) => index));
    expect(first.history.map(({ context }) => context.sessionSequence)).toEqual(
      sequences,
    );
    expect(
      first.history.map(({ type, context }) => [type, context.tick]),
    ).toEqual([
      ["RUN_REQUEST", 0],
      ["INITIAL_STRUCTURE_REQUEST", 0],
      ["SESSION_STARTED", 0],
      ["WORK_REQUEST", 0],
      ["DISPATCH_REQUEST", 1],
      ["DISPATCH_RESPONSE", 2],
      ["WORK_REQUEST", 3],
      ["DISPATCH_REQUEST", 4],
      ["DISPATCH_RESPONSE", 4],
    ]);

    const resetHistory: FactoryEvent[] = [];
    const resetSession = createFactoryEmulatorSession({
      factory: executionFactory,
      scenario: deterministicScenario,
      sink: {
        write: async (events) => {
          resetHistory.push(...structuredClone(events));
        },
      },
    });
    await resetSession.start();
    await resetSession.advanceToNext();
    await resetSession.advanceToNext();
    await resetSession.submit({
      name: "runtime",
      workType: "task",
      state: "ready",
      input: "stable input",
    });
    await resetSession.advanceBy(25);
    resetSession.reset();
    resetHistory.length = 0;
    await resetSession.start();
    await resetSession.advanceToNext();
    const resetDispatched = resetSession.state();
    await resetSession.advanceToNext();
    await resetSession.submit({
      name: "runtime",
      workType: "task",
      state: "ready",
      input: "stable input",
    });
    await resetSession.advanceBy(25);

    expect(JSON.stringify(resetHistory)).toBe(JSON.stringify(first.history));
    expect(resetDispatched).toEqual(first.dispatched);
  });
});

describe("Factory emulator canonical inputs", () => {
  it("canonicalizes equivalent object key order before deriving history", async () => {
    const reorderedFactory = {
      workstations: executionFactory.workstations,
      workers: executionFactory.workers,
      workTypes: executionFactory.workTypes,
      orchestrator: executionFactory.orchestrator,
      name: executionFactory.name,
    } satisfies FactoryDefinition;
    const reorderedScenario = {
      initialSubmissions: deterministicScenario.initialSubmissions,
      unmatched: deterministicScenario.unmatched,
      rules: deterministicScenario.rules,
      startAt: deterministicScenario.startAt,
      seed: deterministicScenario.seed,
      factory: deterministicScenario.factory,
      id: deterministicScenario.id,
      schemaVersion: deterministicScenario.schemaVersion,
    } satisfies FactoryEmulatorScenario;

    const canonical = await runDeterministicHistory(
      executionFactory,
      deterministicScenario,
      { name: "runtime", input: "stable input" },
    );
    const reordered = await runDeterministicHistory(
      reorderedFactory,
      reorderedScenario,
      { input: "stable input", name: "runtime" },
    );

    expect(JSON.stringify(reordered.history)).toBe(
      JSON.stringify(canonical.history),
    );
  });
});

describe("Factory emulator identity inputs", () => {
  it("changes identities for a changed seed or semantically relevant input", async () => {
    const baseline = await runDeterministicHistory(
      executionFactory,
      deterministicScenario,
      { name: "runtime", input: "stable input" },
    );
    const changedSeed = await runDeterministicHistory(
      executionFactory,
      { ...deterministicScenario, seed: "changed-seed" },
      { name: "runtime", input: "stable input" },
    );
    const changedInput = await runDeterministicHistory(
      executionFactory,
      deterministicScenario,
      { name: "runtime", input: "changed input" },
    );
    const runtimeWorkId = (history: readonly FactoryEvent[]) =>
      history.filter(({ type }) => type === "WORK_REQUEST").at(-1)?.context
        .workIds?.[0];

    expect(changedSeed.history[0]?.id).not.toBe(baseline.history[0]?.id);
    expect(runtimeWorkId(changedInput.history)).not.toBe(
      runtimeWorkId(baseline.history),
    );
    expect(
      changedSeed.history.every(
        ({ context }, index) =>
          context.eventTime === baseline.history[index]?.context.eventTime,
      ),
    ).toBe(true);
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

describe("Factory emulator atomic transaction recovery", () => {
  it("keeps a rejected scheduler batch exact and blocks incompatible commands", async () => {
    const attempts: FactoryEvent[][] = [];
    let rejectNextDispatch = true;
    const emulator = executionHarness(
      executionScenario({
        initialSubmissions: [
          { name: "retry-work", workType: "task", state: "ready" },
        ],
      }),
      {
        write: async (events) => {
          attempts.push(structuredClone(events));
          if (
            rejectNextDispatch &&
            events.some(({ type }) => type === "DISPATCH_REQUEST")
          ) {
            rejectNextDispatch = false;
            throw new Error("temporary recording failure");
          }
        },
      },
    );
    await emulator.start();
    const before = emulator.state();

    await expect(emulator.advanceToNext()).rejects.toThrow(
      "temporary recording failure",
    );
    expect(emulator.state()).toEqual(before);
    expect(emulator.status()).toMatchObject({
      phase: "error",
      pendingTransaction: {
        command: "advanceToNext",
        phase: "sink-write",
        eventCount: 1,
      },
    });
    await expect(
      emulator.submit({ name: "blocked", workType: "task", state: "ready" }),
    ).rejects.toBeInstanceOf(FactoryEmulatorPendingCommandError);
    await expect(emulator.close()).rejects.toBeInstanceOf(
      FactoryEmulatorPendingCommandError,
    );

    const retry = await emulator.advanceToNext();
    expect(attempts.at(-1)).toEqual(attempts.at(-2));
    expect(retry.batches.at(-1)).toEqual(attempts.at(-1));
    expect(retry.state.works[0]?.phase).toBe("active");
    expect(retry.state.counters.commands).toBe(before.counters.commands + 1);
  });

  it("discards a rejected transaction on reset without consuming counters", async () => {
    const writes: FactoryEvent[][] = [];
    let rejectSubmission = true;
    const emulator = executionHarness(executionScenario(), {
      write: async (events) => {
        writes.push(structuredClone(events));
        if (
          rejectSubmission &&
          events.some(({ type }) => type === "WORK_REQUEST")
        ) {
          rejectSubmission = false;
          throw new Error("submission rejected");
        }
      },
    });
    await emulator.start();

    await expect(
      emulator.submit({ name: "discarded", workType: "task", state: "ready" }),
    ).rejects.toThrow("submission rejected");
    const rejectedBatch = writes.at(-1);
    const reset = emulator.reset();

    expect(reset).toEqual({
      status: "reset",
      state: {
        lifecycle: "pre-start",
        virtualElapsedMs: 0,
        counters: { commands: 0, events: 0, completedDispatches: 0 },
      },
    });
    expect(emulator.status()).toMatchObject({ phase: "idle" });
    const restarted = await emulator.start();
    expect(restarted.state.counters).toEqual({
      commands: 1,
      events: 3,
      completedDispatches: 0,
    });
    expect(writes.at(-1)).not.toEqual(rejectedBatch);
  });
});

describe("Factory emulator terminal transaction", () => {
  it("writes a recording-compatible canonical terminal lifecycle batch", async () => {
    const sink = new RecordingFactoryEventSink({
      recording: {
        schemaVersion: "factory-recording/v1",
        id: "closed-session-recording",
        title: "Closed session recording",
      },
      maxEvents: 10,
    });
    const emulator = harness(sink);
    await emulator.start();

    const receipt = await emulator.close();

    expect(sink.snapshot().events.at(-1)).toEqual(receipt.batch[0]);
    expect(receipt.batch[0]).toMatchObject({
      type: "SESSION_COMPLETED",
      payload: {
        finalStatus: "TERMINATED",
        completedAt: scenario.startAt,
        durationMillis: 0,
      },
    });
  });

  it("waits for terminal write and sink close before exposing closed state", async () => {
    const writeGate = deferred();
    const closeGate = deferred();
    const closeStarted = deferred();
    let writes = 0;
    let closes = 0;
    const emulator = harness({
      write: async (events) => {
        writes += 1;
        if (events.some(({ type }) => type === "SESSION_COMPLETED")) {
          await writeGate.promise;
        }
      },
      close: async () => {
        closes += 1;
        closeStarted.resolve();
        await closeGate.promise;
      },
    });
    await emulator.start();

    const closing = emulator.close();
    expect(emulator.state().lifecycle).toBe("started");
    expect(emulator.status()).toMatchObject({
      phase: "active",
      pendingTransaction: { command: "close", phase: "sink-write" },
    });
    await expect(emulator.advanceToNext()).rejects.toBeInstanceOf(
      FactoryEmulatorPendingCommandError,
    );
    writeGate.resolve();
    await closeStarted.promise;
    expect(emulator.state().lifecycle).toBe("started");
    expect(emulator.status()).toMatchObject({
      pendingTransaction: { command: "close", phase: "sink-close" },
    });
    closeGate.resolve();

    const receipt = await closing;
    expect(writes).toBe(2);
    expect(closes).toBe(1);
    expect(receipt.batch.map(({ type }) => type)).toEqual([
      "SESSION_COMPLETED",
    ]);
    expect(receipt.state.lifecycle).toBe("closed");
    expect(emulator.status()).toMatchObject({ phase: "closed" });
    await expect(emulator.close()).rejects.toBeInstanceOf(
      FactoryEmulatorLifecycleError,
    );
    expect(() => emulator.reset()).toThrow(FactoryEmulatorLifecycleError);
  });
});

describe("Factory emulator terminal retry", () => {
  it("retries sink close without duplicating an accepted terminal event", async () => {
    const writes: FactoryEvent[][] = [];
    let closeAttempts = 0;
    const emulator = harness({
      write: async (events) => {
        writes.push(structuredClone(events));
      },
      close: async () => {
        closeAttempts += 1;
        if (closeAttempts === 1) throw new Error("close unavailable");
      },
    });
    await emulator.start();

    await expect(emulator.close()).rejects.toThrow("close unavailable");
    expect(emulator.state().lifecycle).toBe("started");
    expect(emulator.status()).toMatchObject({
      phase: "error",
      error: {
        code: "sink_close_rejected",
        operation: "close",
        command: "close",
      },
      pendingTransaction: { phase: "sink-close" },
    });
    const terminalBatch = writes.at(-1);

    const receipt = await emulator.close();
    expect(receipt.batch).toEqual(terminalBatch);
    expect(writes).toHaveLength(2);
    expect(closeAttempts).toBe(2);
    expect(emulator.state().lifecycle).toBe("closed");
  });

  it("retries a rejected terminal write with identical event identity", async () => {
    const writes: FactoryEvent[][] = [];
    let rejectTerminal = true;
    const emulator = harness({
      write: async (events) => {
        writes.push(structuredClone(events));
        if (
          rejectTerminal &&
          events.some(({ type }) => type === "SESSION_COMPLETED")
        ) {
          rejectTerminal = false;
          throw new Error("terminal write rejected");
        }
      },
      close: async () => undefined,
    });
    await emulator.start();

    await expect(emulator.close()).rejects.toThrow("terminal write rejected");
    const beforeRetry = emulator.state();
    const receipt = await emulator.close();

    expect(writes.at(-1)).toEqual(writes.at(-2));
    expect(receipt.batch).toEqual(writes.at(-1));
    expect(beforeRetry.lifecycle).toBe("started");
    expect(receipt.state.lifecycle).toBe("closed");
  });
});

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: Safety edge cases share one configured-session fixture.
describe("Factory emulator execution safety limits", () => {
  function limitedHarness(
    limits: Parameters<typeof createFactoryEmulatorSession>[0]["limits"],
    scenarioOverride: FactoryEmulatorScenario = executionScenario({
      initialSubmissions: [
        { name: "initial", workType: "task", state: "ready" },
      ],
    }),
    options: {
      readonly sink?: FactoryEventSink;
      readonly yieldControl?: () => void | PromiseLike<void>;
    } = {},
  ) {
    return createFactoryEmulatorSession({
      factory: executionFactory,
      scenario: scenarioOverride,
      sink: options.sink ?? { write: async () => undefined },
      limits,
      ...(options.yieldControl === undefined
        ? {}
        : { yieldControl: options.yieldControl }),
    });
  }

  it("accepts exact event and dispatch limits and pauses a whole one-over batch", async () => {
    const exact = limitedHarness({
      maxEvents: 6,
      maxCompletedDispatches: 1,
    });
    await exact.start();
    await exact.advanceBy(25);
    expect(exact.state().counters).toMatchObject({
      events: 6,
      completedDispatches: 1,
    });

    const writes: FactoryEvent[][] = [];
    const paused = limitedHarness(
      { maxEvents: 5, maxCompletedDispatches: 1 },
      undefined,
      {
        sink: {
          write: async (events) => writes.push(structuredClone(events)),
        },
      },
    );
    await paused.start();
    await expect(paused.advanceBy(25)).rejects.toMatchObject({
      diagnostic: {
        kind: "budget-exceeded",
        limit: "events",
        configured: 5,
        observed: 6,
      },
    });
    expect(paused.state().counters).toMatchObject({
      events: 5,
      completedDispatches: 0,
    });
    expect(writes.flat().some(({ type }) => type === "WORK_FAILED")).toBe(
      false,
    );
    expect(paused.status()).toMatchObject({
      phase: "error",
      error: { code: "execution_paused" },
    });

    const dispatchPaused = limitedHarness({
      maxEvents: 20,
      maxCompletedDispatches: 1,
    });
    await dispatchPaused.start();
    await dispatchPaused.advanceBy(25);
    await dispatchPaused.submit({
      name: "second",
      workType: "task",
      state: "ready",
    });
    await dispatchPaused.advanceToNext();
    const before = dispatchPaused.state();
    await expect(dispatchPaused.advanceToNext()).rejects.toMatchObject({
      diagnostic: {
        kind: "budget-exceeded",
        limit: "completedDispatches",
        configured: 1,
        observed: 2,
      },
    });
    expect(dispatchPaused.state()).toEqual(before);
  });

  it("enforces the virtual-hour boundary without moving time or writing past it", async () => {
    const exact = limitedHarness({ maxVirtualElapsedMs: 25 });
    await exact.start();
    await exact.advanceBy(25);
    expect(exact.state().virtualElapsedMs).toBe(25);
    await exact.submit({ name: "second", workType: "task", state: "ready" });
    await exact.advanceToNext();
    const before = exact.state();

    await expect(exact.advanceToNext()).rejects.toMatchObject({
      diagnostic: {
        kind: "budget-exceeded",
        limit: "virtualElapsedMs",
        configured: 25,
        observed: 50,
        virtualElapsedMs: 50,
      },
    });
    expect(exact.state()).toEqual(before);
  });

  it("distinguishes a finite zero-duration chain from a cycle threshold", async () => {
    const immediate = executionScenario({
      initialSubmissions: [
        { name: "initial", workType: "task", state: "ready" },
      ],
      rules: [
        {
          id: "immediate",
          selector: { workstation: "scripted-run" },
          cursor: { scope: "lineage", input: "rootWorkId" },
          outcomes: [{ result: "accepted", durationMs: 0 }],
          exhaustion: "repeat-last",
        },
      ],
    });
    const finite = limitedHarness({ maxZeroDurationBatches: 2 }, immediate);
    await finite.start();
    await finite.advanceBy(0);
    expect(finite.status().phase).toBe("idle");

    const paused = limitedHarness({ maxZeroDurationBatches: 1 }, immediate);
    await paused.start();
    await expect(paused.advanceBy(0)).rejects.toMatchObject({
      diagnostic: {
        kind: "zero-duration-cycle",
        limit: "zeroDurationBatches",
        configured: 1,
        observed: 2,
      },
    });
    expect(paused.state()).toMatchObject({
      virtualElapsedMs: 0,
      counters: { completedDispatches: 0 },
    });
    expect(
      (paused.state() as { works: { phase: string }[] }).works[0]?.phase,
    ).toBe("active");
  });

  it("rejects oversized initial and runtime Work batches without partial events", async () => {
    const initialWrites: FactoryEvent[][] = [];
    const oversizedInitial = limitedHarness(
      { maxSynchronousWorkItems: 1 },
      executionScenario({
        initialSubmissions: [
          { name: "first", workType: "task", state: "ready" },
          { name: "second", workType: "task", state: "ready" },
        ],
      }),
      {
        sink: {
          write: async (events) => initialWrites.push(structuredClone(events)),
        },
      },
    );
    await expect(oversizedInitial.start()).rejects.toBeInstanceOf(
      FactoryEmulatorExecutionPausedError,
    );
    expect(initialWrites).toEqual([]);
    expect(oversizedInitial.state().lifecycle).toBe("pre-start");

    const runtime = limitedHarness(
      { maxSynchronousWorkItems: 1 },
      executionScenario({ initialSubmissions: [] }),
    );
    await runtime.start();
    const before = runtime.state();
    await expect(
      runtime.submit([
        { name: "first", workType: "task", state: "ready" },
        { name: "second", workType: "task", state: "ready" },
      ]),
    ).rejects.toMatchObject({
      diagnostic: {
        kind: "bounded-work-exceeded",
        configured: 1,
        observed: 2,
      },
    });
    expect(runtime.state()).toEqual(before);
  });

  it("awaits the caller-owned yield cadence and resumes deterministically", async () => {
    let yields = 0;
    const emulator = limitedHarness({ maxSynchronousBatches: 1 }, undefined, {
      yieldControl: async () => {
        yields += 1;
        await Promise.resolve();
      },
    });
    await emulator.start();
    const receipt = await emulator.advanceBy(25);

    expect(yields).toBe(2);
    expect(receipt.batches).toHaveLength(2);
    expect(receipt.state.works[0]?.phase).toBe("completed");
    expect(() => structuredClone(receipt)).not.toThrow();
    expect(() => structuredClone(emulator.status())).not.toThrow();
  });
});
