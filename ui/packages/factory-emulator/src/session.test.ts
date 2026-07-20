// biome-ignore-all lint/style/noExcessiveLinesPerFile: Lifecycle and execution assertions share one public-session fixture.
import type {
  FactoryDefinition,
  FactoryEvent,
} from "@you-agent-factory/client";
import { safeParseFactoryRecording } from "@you-agent-factory/client";
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
import { replayFactoryEmulatorSubmissions } from "./submission-replay.js";

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
        { name: "complete", type: "PROCESSING" },
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

const dependencyFactory = {
  name: "dependency-execution-factory",
  orchestrator: { kind: "PETRI" },
  workTypes: [
    {
      name: "task",
      states: [
        { name: "ready", type: "INITIAL" },
        { name: "complete", type: "TERMINAL" },
        { name: "failed", type: "FAILED" },
      ],
    },
    {
      name: "draft",
      states: [
        { name: "ready", type: "INITIAL" },
        { name: "review", type: "PROCESSING" },
      ],
    },
  ],
  workers: [{ name: "dependency-worker", type: "AGENT_WORKER" }],
  workstations: [
    {
      name: "complete-task-a",
      type: "AGENT_RUN",
      worker: "dependency-worker",
      inputs: [{ workType: "task", state: "ready" }],
      outputs: [{ workType: "task", state: "complete" }],
    },
    {
      name: "complete-task-b",
      type: "AGENT_RUN",
      worker: "dependency-worker",
      inputs: [{ workType: "task", state: "ready" }],
      outputs: [{ workType: "task", state: "complete" }],
    },
    {
      name: "review-draft",
      type: "AGENT_RUN",
      worker: "dependency-worker",
      inputs: [{ workType: "draft", state: "ready" }],
      outputs: [{ workType: "draft", state: "review" }],
    },
  ],
} satisfies FactoryDefinition;

function dependencyScenario(
  initialSubmissions: NonNullable<
    FactoryEmulatorScenario["initialSubmissions"]
  >,
): FactoryEmulatorScenario {
  return {
    schemaVersion: "factory-emulator-scenario/v1",
    id: "dependency-execution-scenario",
    factory: { name: dependencyFactory.name },
    seed: "dependency-execution-seed",
    startAt: "2026-07-18T16:00:00.000Z",
    initialSubmissions,
    rules: dependencyFactory.workstations.map((workstation) => ({
      id: `${workstation.name}-outcome`,
      selector: { workstation: workstation.name },
      cursor: { scope: "lineage", input: "rootWorkId" },
      outcomes: [
        {
          result: "accepted" as const,
          durationMs: workstation.name === "review-draft" ? 20 : 10,
        },
      ],
      exhaustion: "repeat-last" as const,
    })),
    unmatched: { behavior: "error" },
  };
}

function dependencyHarness(
  initialSubmissions: NonNullable<
    FactoryEmulatorScenario["initialSubmissions"]
  >,
) {
  return createFactoryEmulatorSession({
    factory: dependencyFactory,
    scenario: dependencyScenario(initialSubmissions),
    sink: { write: async () => undefined },
  });
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: The table keeps every invalid relationship case on the same atomicity harness.
describe("Factory emulator dependency submission", () => {
  it("normalizes the shared scenario and runtime DEPENDS_ON batch contract", async () => {
    const batch = {
      works: [
        { name: "blocked", workType: "task", state: "ready" },
        { name: "prerequisite", workType: "task", state: "ready" },
      ],
      relations: [
        {
          type: "DEPENDS_ON" as const,
          sourceWorkName: "blocked",
          targetWorkName: "prerequisite",
        },
      ],
    };
    const initial = executionHarness(
      executionScenario({ initialSubmissions: batch }),
    );
    const initialState = (await initial.start()).state;
    const runtime = executionHarness(
      executionScenario({ initialSubmissions: [] }),
    );
    await runtime.start();
    const runtimeState = (await runtime.submit(batch)).state;

    for (const state of [initialState, runtimeState]) {
      const blocked = state.works.find(
        ({ submissionId }) => submissionId === "blocked",
      );
      const prerequisite = state.works.find(
        ({ submissionId }) => submissionId === "prerequisite",
      );
      expect(blocked?.relations).toEqual([
        {
          type: "DEPENDS_ON",
          sourceWorkName: "blocked",
          targetWorkName: "prerequisite",
          targetWorkId: prerequisite?.workId,
          requiredState: "complete",
        },
      ]);
      expect(blocked?.workId).toBeTruthy();
      expect(prerequisite?.workId).toBeTruthy();
    }

    const explicit = await runtime.submit({
      works: [
        { name: "explicit-blocked", workType: "task", state: "ready" },
        { name: "explicit-target", workType: "task", state: "ready" },
      ],
      relations: [
        {
          type: "DEPENDS_ON",
          sourceWorkName: "explicit-blocked",
          targetWorkName: "explicit-target",
          requiredState: "done",
        },
      ],
    });
    expect(
      explicit.state.works.find(
        ({ submissionId }) => submissionId === "explicit-blocked",
      )?.relations?.[0]?.requiredState,
    ).toBe("done");
  });

  it.each([
    [
      "duplicate relations",
      [
        {
          type: "DEPENDS_ON",
          sourceWorkName: "blocked",
          targetWorkName: "target",
        },
        {
          type: "DEPENDS_ON",
          sourceWorkName: "blocked",
          targetWorkName: "target",
        },
      ],
    ],
    [
      "missing source",
      [
        {
          type: "DEPENDS_ON",
          sourceWorkName: "missing",
          targetWorkName: "target",
        },
      ],
    ],
    [
      "missing target",
      [
        {
          type: "DEPENDS_ON",
          sourceWorkName: "blocked",
          targetWorkName: "missing",
        },
      ],
    ],
    [
      "self relation",
      [
        {
          type: "DEPENDS_ON",
          sourceWorkName: "blocked",
          targetWorkName: "blocked",
        },
      ],
    ],
    [
      "cycle",
      [
        {
          type: "DEPENDS_ON",
          sourceWorkName: "blocked",
          targetWorkName: "target",
        },
        {
          type: "DEPENDS_ON",
          sourceWorkName: "target",
          targetWorkName: "blocked",
        },
      ],
    ],
    [
      "invalid required state",
      [
        {
          type: "DEPENDS_ON",
          sourceWorkName: "blocked",
          targetWorkName: "target",
          requiredState: "missing",
        },
      ],
    ],
    [
      "parent-child",
      [
        {
          type: "PARENT_CHILD",
          sourceWorkName: "blocked",
          targetWorkName: "target",
        },
      ],
    ],
    [
      "spawned-by",
      [
        {
          type: "SPAWNED_BY",
          sourceWorkName: "blocked",
          targetWorkName: "target",
        },
      ],
    ],
    [
      "unknown relationship type",
      [
        {
          type: "BLOCKS" as never,
          sourceWorkName: "blocked",
          targetWorkName: "target",
        },
      ],
    ],
  ] as const)(
    "atomically rejects %s dependency batches",
    async (_label, relations) => {
      const writes: FactoryEvent[][] = [];
      const emulator = executionHarness(
        executionScenario({ initialSubmissions: [] }),
        {
          write: async (events) =>
            writes.push(structuredClone(events) as FactoryEvent[]),
        },
      );
      await emulator.start();
      const before = emulator.state();
      await expect(
        emulator.submit({
          works: [
            { name: "blocked", workType: "task", state: "ready" },
            { name: "target", workType: "task", state: "ready" },
          ],
          relations,
        }),
      ).rejects.toMatchObject({
        name: "FactoryEmulatorSubmissionError",
        code: "invalid_submission",
        diagnostics: expect.arrayContaining([expect.any(Object)]),
      } satisfies Partial<FactoryEmulatorSubmissionError>);
      expect(emulator.state()).toEqual(before);
      expect(writes).toHaveLength(1);
    },
  );

  it("does not consume deterministic identity after relationship rejection", async () => {
    const valid = {
      works: [
        { name: "blocked", workType: "task", state: "ready" },
        { name: "target", workType: "task", state: "ready" },
      ],
      relations: [
        {
          type: "DEPENDS_ON" as const,
          sourceWorkName: "blocked",
          targetWorkName: "target",
        },
      ],
    };
    const afterRejection = executionHarness(
      executionScenario({ initialSubmissions: [] }),
    );
    await afterRejection.start();
    await expect(
      afterRejection.submit({
        ...valid,
        relations: [{ ...valid.relations[0], targetWorkName: "missing" }],
      }),
    ).rejects.toBeInstanceOf(Error);
    const retried = await afterRejection.submit(valid);

    const firstTry = executionHarness(
      executionScenario({ initialSubmissions: [] }),
    );
    await firstTry.start();
    const first = await firstTry.submit(valid);
    expect(retried.state.works).toEqual(first.state.works);
    expect(retried.batch).toEqual(first.batch);
  });

  it("emits canonical request and relationship evidence in stable order", async () => {
    const emulator = executionHarness(
      executionScenario({ initialSubmissions: [] }),
    );
    const started = await emulator.start();
    const submitted = await emulator.submit({
      works: [
        {
          name: "blocked",
          workType: "task",
          state: "ready",
          input: "blocked payload",
        },
        { name: "first", workType: "task", state: "ready" },
        { name: "second", workType: "task", state: "ready" },
      ],
      relations: [
        {
          type: "DEPENDS_ON",
          sourceWorkName: "blocked",
          targetWorkName: "second",
          requiredState: "done",
        },
        {
          type: "DEPENDS_ON",
          sourceWorkName: "blocked",
          targetWorkName: "first",
        },
      ],
    });

    expect(submitted.batch.map(({ type }) => type)).toEqual([
      "WORK_REQUEST",
      "RELATIONSHIP_CHANGE_REQUEST",
      "RELATIONSHIP_CHANGE_REQUEST",
    ]);
    expect(submitted.batch.map(({ context }) => context.sequence)).toEqual([
      3, 4, 5,
    ]);
    const request = submitted.batch[0];
    if (request?.type !== "WORK_REQUEST") throw new Error("missing request");
    const relations = (request.payload as { relations?: unknown[] }).relations;
    expect(relations).toEqual(
      submitted.batch
        .slice(1)
        .map((event) => (event.payload as { relation: unknown }).relation),
    );
    expect(relations).toEqual([
      expect.objectContaining({
        sourceWorkName: "blocked",
        targetWorkName: "second",
        targetWorkId: expect.any(String),
        requiredState: "done",
      }),
      expect.objectContaining({
        sourceWorkName: "blocked",
        targetWorkName: "first",
        targetWorkId: expect.any(String),
        requiredState: "complete",
      }),
    ]);
    expect(
      safeParseFactoryRecording({
        schemaVersion: "factory-recording/v1",
        id: "dependency-recording",
        title: "Dependency recording",
        factory: executionFactory,
        events: [...started.batches.flat(), ...submitted.batch],
      }),
    ).toMatchObject({ success: true });

    const replayed = replayFactoryEmulatorSubmissions([
      ...started.batches.flat(),
      ...submitted.batch,
    ]);
    expect(replayed).toEqual(
      submitted.state.works.map((work) => ({
        submissionId: work.submissionId,
        workId: work.workId,
        requestId: work.requestId,
        traceId: work.traceId,
        workType: work.workType,
        state: work.state,
        ...(work.input === undefined ? {} : { input: work.input }),
        ...(work.relations === undefined ? {} : { relations: work.relations }),
      })),
    );

    emulator.reset();
    const rerunStart = await emulator.start();
    const rerunSubmission = await emulator.submit({
      works: [
        {
          name: "blocked",
          workType: "task",
          state: "ready",
          input: "blocked payload",
        },
        { name: "first", workType: "task", state: "ready" },
        { name: "second", workType: "task", state: "ready" },
      ],
      relations: [
        {
          type: "DEPENDS_ON",
          sourceWorkName: "blocked",
          targetWorkName: "second",
          requiredState: "done",
        },
        {
          type: "DEPENDS_ON",
          sourceWorkName: "blocked",
          targetWorkName: "first",
        },
      ],
    });
    expect(rerunStart.batches).toEqual(started.batches);
    expect(rerunSubmission).toEqual(submitted);
  });

  it("keeps relationship state and identity uncommitted until one sink batch succeeds", async () => {
    const attempts: FactoryEvent[][] = [];
    let rejectSubmission = true;
    const emulator = executionHarness(
      executionScenario({ initialSubmissions: [] }),
      {
        write: async (events) => {
          attempts.push(structuredClone(events) as FactoryEvent[]);
          if (events[0]?.type === "WORK_REQUEST" && rejectSubmission) {
            rejectSubmission = false;
            throw new Error("temporary relationship write failure");
          }
        },
      },
    );
    await emulator.start();
    const before = emulator.state();
    const batch = {
      works: [
        { name: "blocked", workType: "task", state: "ready" },
        { name: "target", workType: "task", state: "ready" },
      ],
      relations: [
        {
          type: "DEPENDS_ON" as const,
          sourceWorkName: "blocked",
          targetWorkName: "target",
        },
      ],
    };

    await expect(emulator.submit(batch)).rejects.toThrow(
      "temporary relationship write failure",
    );
    expect(emulator.state()).toEqual(before);
    expect(emulator.status()).toMatchObject({
      phase: "error",
      pendingTransaction: { command: "submit", eventCount: 2 },
    });
    const retried = await emulator.submit(batch);
    expect(attempts.slice(-2)).toEqual([retried.batch, retried.batch]);
    expect(retried.state.counters).toEqual({
      commands: before.counters.commands + 1,
      events: before.counters.events + 2,
      completedDispatches: before.counters.completedDispatches,
    });
  });
});

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

describe("Factory emulator dependency-aware scheduler dispatch", () => {
  it("keeps default-complete dependents blocked while unrelated Work dispatches", async () => {
    const emulator = dependencyHarness({
      works: [
        { name: "prerequisite", workType: "task", state: "ready" },
        { name: "dependent", workType: "task", state: "ready" },
        { name: "unrelated", workType: "task", state: "ready" },
      ],
      relations: [
        {
          type: "DEPENDS_ON",
          sourceWorkName: "dependent",
          targetWorkName: "prerequisite",
        },
      ],
    });

    const started = await emulator.start();
    const dependentId = started.state.works.find(
      ({ submissionId }) => submissionId === "dependent",
    )?.workId;
    const firstDispatch = await emulator.advanceToNext();

    expect(
      firstDispatch.batches[0]?.flatMap(({ context }) => context.workIds ?? []),
    ).not.toContain(dependentId);
    expect(
      firstDispatch.state.works.find(
        ({ submissionId }) => submissionId === "dependent",
      ),
    ).toMatchObject({ phase: "ready", state: "ready" });
    expect(
      firstDispatch.state.works.filter(({ phase }) => phase === "active"),
    ).toHaveLength(2);

    const completedAndUnblocked = await emulator.advanceBy(10);
    expect(
      completedAndUnblocked.batches.map((batch) => batch[0]?.type),
    ).toEqual(["DISPATCH_RESPONSE", "DISPATCH_REQUEST"]);
    expect(
      completedAndUnblocked.state.works.find(
        ({ submissionId, phase }) =>
          submissionId === "dependent" && phase === "active",
      ),
    ).toMatchObject({ workId: dependentId, state: "ready" });
  });

  it("requires every default and explicit dependency across all candidate bindings", async () => {
    const emulator = dependencyHarness({
      works: [
        { name: "complete-target", workType: "task", state: "ready" },
        { name: "review-target", workType: "draft", state: "ready" },
        { name: "dependent", workType: "task", state: "ready" },
      ],
      relations: [
        {
          type: "DEPENDS_ON",
          sourceWorkName: "dependent",
          targetWorkName: "complete-target",
        },
        {
          type: "DEPENDS_ON",
          sourceWorkName: "dependent",
          targetWorkName: "review-target",
          requiredState: "review",
        },
      ],
    });

    await emulator.start();
    await emulator.advanceToNext();
    const partiallySatisfied = await emulator.advanceBy(10);
    expect(partiallySatisfied.batches).toHaveLength(1);
    expect(
      partiallySatisfied.state.works.find(
        ({ submissionId }) => submissionId === "dependent",
      ),
    ).toMatchObject({ phase: "ready", state: "ready" });

    const fullySatisfied = await emulator.advanceBy(10);
    expect(fullySatisfied.batches.map((batch) => batch[0]?.type)).toEqual([
      "DISPATCH_RESPONSE",
      "DISPATCH_REQUEST",
    ]);
    expect(
      fullySatisfied.state.works.find(
        ({ submissionId, phase }) =>
          submissionId === "dependent" && phase === "active",
      ),
    ).toBeDefined();
    const reviewTargetId = fullySatisfied.state.works.find(
      ({ submissionId }) => submissionId === "review-target",
    )?.workId;
    expect(
      fullySatisfied.state.works.filter(
        ({ workId, state }) => workId === reviewTargetId && state === "review",
      ),
    ).toHaveLength(1);
  });
});

function dependencyFailureHarness(): FactoryEmulatorSession {
  const initialSubmissions = {
    works: [
      { name: "leaf", workType: "task", state: "ready" },
      { name: "root-a", workType: "task", state: "ready" },
      { name: "terminal", workType: "task", state: "complete" },
      { name: "middle", workType: "task", state: "ready" },
      { name: "root-b", workType: "task", state: "ready" },
      { name: "converging", workType: "task", state: "ready" },
      { name: "unrelated", workType: "task", state: "ready" },
      { name: "already-failed", workType: "task", state: "failed" },
    ],
    relations: [
      {
        type: "DEPENDS_ON" as const,
        sourceWorkName: "leaf",
        targetWorkName: "middle",
      },
      {
        type: "DEPENDS_ON" as const,
        sourceWorkName: "middle",
        targetWorkName: "root-a",
      },
      {
        type: "DEPENDS_ON" as const,
        sourceWorkName: "converging",
        targetWorkName: "root-a",
      },
      {
        type: "DEPENDS_ON" as const,
        sourceWorkName: "converging",
        targetWorkName: "root-b",
      },
      {
        type: "DEPENDS_ON" as const,
        sourceWorkName: "terminal",
        targetWorkName: "root-a",
      },
      {
        type: "DEPENDS_ON" as const,
        sourceWorkName: "already-failed",
        targetWorkName: "root-a",
      },
    ],
  };
  const baseScenario = dependencyScenario(initialSubmissions);
  const failureScenario: FactoryEmulatorScenario = {
    ...baseScenario,
    rules: [
      {
        id: "fail-roots",
        selector: { input: { name: "root-a" } },
        cursor: { scope: "lineage", input: "rootWorkId" },
        outcomes: [
          { result: "failed", durationMs: 10, error: "root-a failed" },
        ],
        exhaustion: "repeat-last",
      },
      {
        id: "fail-second-root",
        selector: { input: { name: "root-b" } },
        cursor: { scope: "lineage", input: "rootWorkId" },
        outcomes: [
          { result: "failed", durationMs: 10, error: "root-b failed" },
        ],
        exhaustion: "repeat-last",
      },
      ...baseScenario.rules,
    ],
  };
  return createFactoryEmulatorSession({
    factory: dependencyFactory,
    scenario: failureScenario,
    sink: { write: async () => undefined },
  });
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: These boundary regressions keep the atomic start and logical-tick assertions together.
describe("Factory emulator dependency failure closure boundaries", () => {
  it("cascades an initially failed dependency in the atomic start batch and retries exactly", async () => {
    const attempts: FactoryEvent[][] = [];
    let rejectStart = true;
    const emulator = createFactoryEmulatorSession({
      factory: dependencyFactory,
      scenario: dependencyScenario({
        works: [
          { name: "root", workType: "task", state: "failed" },
          { name: "dependent", workType: "task", state: "ready" },
          { name: "terminal", workType: "task", state: "complete" },
          { name: "unrelated", workType: "task", state: "ready" },
        ],
        relations: [
          {
            type: "DEPENDS_ON",
            sourceWorkName: "dependent",
            targetWorkName: "root",
          },
          {
            type: "DEPENDS_ON",
            sourceWorkName: "terminal",
            targetWorkName: "root",
          },
        ],
      }),
      sink: {
        write: async (events) => {
          attempts.push(structuredClone(events) as FactoryEvent[]);
          if (rejectStart) {
            rejectStart = false;
            throw new Error("initial cascade write rejected");
          }
        },
      },
    });

    await expect(emulator.start()).rejects.toThrow(
      "initial cascade write rejected",
    );
    expect(emulator.state()).toMatchObject({
      lifecycle: "pre-start",
      counters: { events: 0 },
    });

    const started = await emulator.start();
    const cascadeEvents = started.batches[1]?.filter(
      ({ type }) => type === "WORK_STATE_CHANGE",
    );
    expect(attempts).toEqual([started.batches.flat(), started.batches.flat()]);
    expect(cascadeEvents).toHaveLength(1);
    expect(cascadeEvents?.[0]).toMatchObject({
      context: { tick: 1 },
      payload: {
        source: "cascading-failure",
        fromState: "ready",
        toState: "failed",
      },
    });
    expect(
      started.state.works.map(({ submissionId, state, phase }) => ({
        submissionId,
        state,
        phase,
      })),
    ).toEqual([
      { submissionId: "root", state: "failed", phase: "ready" },
      { submissionId: "dependent", state: "failed", phase: "completed" },
      { submissionId: "terminal", state: "complete", phase: "ready" },
      { submissionId: "unrelated", state: "ready", phase: "ready" },
    ]);
  });

  it("cascades a logical-move failure before any dependent can dispatch", async () => {
    const logicalFailureFactory = {
      ...dependencyFactory,
      workstations: [
        {
          name: "a-fail-logically",
          type: "LOGICAL_MOVE",
          worker: "",
          inputs: [{ workType: "task", state: "ready" }],
          outputs: [{ workType: "task", state: "failed" }],
          guards: [
            {
              type: "VISIT_COUNT",
              workstation: "complete-task-a",
              maxVisits: 1,
            },
          ],
        },
        ...dependencyFactory.workstations.map((workstation) => ({
          ...workstation,
          outputs: [{ workType: "task", state: "ready" }],
        })),
      ],
    } satisfies FactoryDefinition;
    const emulator = createFactoryEmulatorSession({
      factory: logicalFailureFactory,
      scenario: {
        ...dependencyScenario({
          works: [
            { name: "root", workType: "task", state: "ready" },
            { name: "dependent", workType: "task", state: "ready" },
          ],
          relations: [
            {
              type: "DEPENDS_ON",
              sourceWorkName: "dependent",
              targetWorkName: "root",
            },
          ],
        }),
        id: "logical-failure-scenario",
        factory: { name: logicalFailureFactory.name },
        seed: "logical-failure-seed",
      },
      sink: { write: async () => undefined },
    });

    await emulator.start();
    await emulator.advanceToNext();
    await emulator.advanceToNext();
    const advanced = await emulator.advanceToNext();

    expect(advanced.batches).toHaveLength(1);
    expect(advanced.batches[0]?.map(({ type }) => type)).toEqual([
      "DISPATCH_RESPONSE",
      "WORK_STATE_CHANGE",
    ]);
    expect(advanced.batches[0]?.[0]).toMatchObject({
      payload: { outputWork: [{ state: { name: "failed" } }] },
    });
    expect(advanced.batches[0]?.[1]).toMatchObject({
      context: { tick: 3 },
      payload: {
        source: "cascading-failure",
        fromState: "ready",
        toState: "failed",
      },
    });
    expect(
      advanced.batches[0]?.some(
        ({ type, context }) =>
          type === "DISPATCH_REQUEST" &&
          context.workIds?.some((workId) =>
            advanced.state.works.some(
              (work) =>
                work.workId === workId && work.submissionId === "dependent",
            ),
          ),
      ),
    ).toBe(false);
    expect(
      advanced.state.works
        .filter(({ submissionId }) =>
          ["root", "dependent"].includes(submissionId),
        )
        .map(({ submissionId, state, phase }) => ({
          submissionId,
          state,
          phase,
        })),
    ).toEqual([
      { submissionId: "root", state: "ready", phase: "completed" },
      { submissionId: "dependent", state: "failed", phase: "completed" },
    ]);
  });
});

describe("Factory emulator dependency failure cascade", () => {
  it("fails every non-terminal transitive dependent once in the completion tick", async () => {
    const emulator = dependencyFailureHarness();
    const started = await emulator.start();
    const identityByName = new Map(
      started.state.works.map(({ submissionId, workId }) => [
        submissionId,
        workId,
      ]),
    );
    await emulator.advanceToNext();
    const beforeFailure = emulator.state();
    const completed = await emulator.advanceBy(10);

    expect(completed.batches).toHaveLength(1);
    const cascadeEvents = completed.batches[0]?.filter(
      ({ type }) => type === "WORK_STATE_CHANGE",
    );
    expect(cascadeEvents?.map(({ context }) => context.workIds?.[0])).toEqual([
      identityByName.get("middle"),
      identityByName.get("converging"),
      identityByName.get("leaf"),
    ]);
    expect(
      cascadeEvents?.map(({ context, payload }) => ({
        tick: context.tick,
        source: (payload as { source: string }).source,
        triggerWorkId: (payload as { triggerWorkId: string }).triggerWorkId,
      })),
    ).toEqual([
      {
        tick: cascadeEvents?.[0]?.context.tick,
        source: "cascading-failure",
        triggerWorkId: identityByName.get("root-a"),
      },
      {
        tick: cascadeEvents?.[0]?.context.tick,
        source: "cascading-failure",
        triggerWorkId: identityByName.get("root-a"),
      },
      {
        tick: cascadeEvents?.[0]?.context.tick,
        source: "cascading-failure",
        triggerWorkId: identityByName.get("middle"),
      },
    ]);
    expect(
      completed.state.works
        .filter(({ submissionId }) =>
          ["middle", "converging", "leaf"].includes(submissionId),
        )
        .map(({ submissionId, state, phase }) => ({
          submissionId,
          state,
          phase,
        })),
    ).toEqual([
      { submissionId: "leaf", state: "failed", phase: "completed" },
      { submissionId: "middle", state: "failed", phase: "completed" },
      { submissionId: "converging", state: "failed", phase: "completed" },
    ]);
    expect(
      completed.state.works.find(
        ({ submissionId }) => submissionId === "terminal",
      ),
    ).toEqual(
      beforeFailure.works.find(
        ({ submissionId }) => submissionId === "terminal",
      ),
    );
    expect(
      completed.state.works.find(
        ({ submissionId }) => submissionId === "already-failed",
      ),
    ).toEqual(
      beforeFailure.works.find(
        ({ submissionId }) => submissionId === "already-failed",
      ),
    );
    expect(
      completed.state.works.find(
        ({ workId, state }) =>
          workId === identityByName.get("unrelated") && state === "complete",
      ),
    ).toMatchObject({ phase: "completed" });
    expect(
      completed.batches[0]?.filter(({ type }) => type === "DISPATCH_REQUEST"),
    ).toHaveLength(0);
    expect(completed.state.counters.completedDispatches).toBe(3);
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

  it("routes an eligible logical move ahead of a competing worker without an active dispatch", async () => {
    const competingFactory = {
      ...executionFactory,
      workstations: [
        {
          ...executionFactory.workstations[0],
          name: "a-worker-run",
          outputs: [{ workType: "task", state: "ready" }],
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
      ],
    });
    const emulator = createFactoryEmulatorSession({
      factory: competingFactory,
      scenario: competingScenario,
      sink: { write: async () => undefined },
    });

    await emulator.start();
    const workerDispatch = await emulator.advanceToNext();
    const workerCompletion = await emulator.advanceToNext();
    const moved = await emulator.advanceToNext();

    expect(workerDispatch.batches[0]?.[0]?.type).toBe("DISPATCH_REQUEST");
    expect(workerCompletion.state.works.at(-1)?.visits).toEqual({
      "a-worker-run": 1,
    });
    expect(moved.batches[0]?.map(({ type }) => type)).toEqual([
      "DISPATCH_RESPONSE",
    ]);
    expect(moved.batches[0]?.[0]).toMatchObject({
      payload: {
        transitionId: "z-logical-move",
        outcome: "ACCEPTED",
        durationMillis: 0,
      },
    });
    expect(moved.batches[0]?.[0]?.context.dispatchId).toBeUndefined();
    expect(moved.state.works.some(({ phase }) => phase === "active")).toBe(
      false,
    );
    expect(moved.state.works.at(-1)).toMatchObject({
      state: "done",
      phase: "completed",
      visits: { "a-worker-run": 1, "z-logical-move": 1 },
    });
  });
});

describe("Factory emulator guarded logical moves", () => {
  it("preserves lineage, payload, and declared fan-out order", async () => {
    const logicalFactory = {
      name: "logical-routing-factory",
      orchestrator: { kind: "PETRI" },
      workTypes: [
        {
          name: "task",
          states: [
            { name: "ready", type: "INITIAL" },
            { name: "done", type: "TERMINAL" },
          ],
        },
        {
          name: "audit",
          states: [{ name: "recorded", type: "TERMINAL" }],
        },
      ],
      workers: [{ name: "worker", type: "AGENT_WORKER" }],
      workstations: [
        {
          name: "execute",
          type: "AGENT_RUN",
          worker: "worker",
          inputs: [{ workType: "task", state: "ready" }],
          outputs: [{ workType: "task", state: "ready" }],
        },
        {
          name: "loop-breaker",
          type: "LOGICAL_MOVE",
          worker: "",
          inputs: [{ workType: "task", state: "ready" }],
          outputs: [
            { workType: "audit", state: "recorded" },
            { workType: "task", state: "done" },
          ],
          guards: [
            { type: "VISIT_COUNT", workstation: "execute", maxVisits: 1 },
          ],
          workPropagation: { mode: "PRESERVE_INPUT" },
        },
      ],
    } satisfies FactoryDefinition;
    const logicalScenario = executionScenario({
      factory: { name: logicalFactory.name },
      initialSubmissions: [
        {
          name: "logical-input",
          workType: "task",
          state: "ready",
          input: "original",
        },
      ],
      rules: [
        {
          id: "execute-outcome",
          selector: { workstation: "execute" },
          cursor: { scope: "lineage", input: "rootWorkId" },
          outcomes: [
            { result: "accepted", durationMs: 1, output: "worker-output" },
          ],
          exhaustion: "repeat-last",
        },
      ],
    });
    const emulator = createFactoryEmulatorSession({
      factory: logicalFactory,
      scenario: logicalScenario,
      sink: { write: async () => undefined },
    });

    await emulator.start();
    await emulator.advanceToNext();
    await emulator.advanceToNext();
    const moved = await emulator.advanceToNext();
    const response = moved.batches[0]?.[0];
    if (response?.type !== "DISPATCH_RESPONSE") {
      throw new Error("missing logical-move response");
    }
    const outputWork = response.payload.outputWork ?? [];

    expect(
      outputWork.map(({ workTypeName, state, payload }) => ({
        workTypeName,
        state: state.name,
        payload,
      })),
    ).toEqual([
      { workTypeName: "audit", state: "recorded", payload: "worker-output" },
      { workTypeName: "task", state: "done", payload: "worker-output" },
    ]);
    expect(
      moved.state.works.slice(-2).map(({ workType, rootWorkId }) => ({
        workType,
        rootWorkId,
      })),
    ).toEqual([
      { workType: "audit", rootWorkId: moved.state.works[0]?.rootWorkId },
      { workType: "task", rootWorkId: moved.state.works[0]?.rootWorkId },
    ]);
  });
});

describe("Factory emulator logical-move cycle safety", () => {
  it("pauses a zero-duration logical cycle at the configured cooperative boundary", async () => {
    const cycleFactory = {
      ...executionFactory,
      workstations: [
        {
          ...executionFactory.workstations[0],
          name: "execute",
          outputs: [{ workType: "task", state: "ready" }],
        },
        {
          name: "cycle",
          type: "LOGICAL_MOVE",
          worker: "",
          inputs: [{ workType: "task", state: "ready" }],
          outputs: [{ workType: "task", state: "ready" }],
          guards: [
            { type: "VISIT_COUNT", workstation: "execute", maxVisits: 1 },
          ],
        },
      ],
    } satisfies FactoryDefinition;
    const writes: FactoryEvent[][] = [];
    let yields = 0;
    const emulator = createFactoryEmulatorSession({
      factory: cycleFactory,
      scenario: executionScenario({
        factory: { name: cycleFactory.name },
        initialSubmissions: [
          { name: "cycle-input", workType: "task", state: "ready" },
        ],
        rules: [
          {
            id: "execute-outcome",
            selector: { workstation: "execute" },
            cursor: { scope: "lineage", input: "rootWorkId" },
            outcomes: [{ result: "accepted", durationMs: 1 }],
            exhaustion: "repeat-last",
          },
        ],
      }),
      sink: {
        write: async (events) => {
          writes.push(structuredClone(events) as FactoryEvent[]);
        },
      },
      limits: { maxZeroDurationBatches: 3, maxSynchronousBatches: 1 },
      yieldControl: () => {
        yields += 1;
      },
    });

    await emulator.start();
    await expect(emulator.advanceBy(1)).rejects.toMatchObject({
      name: "FactoryEmulatorExecutionPausedError",
      diagnostic: {
        kind: "zero-duration-cycle",
        configured: 3,
        observed: 4,
      },
    } satisfies Partial<FactoryEmulatorExecutionPausedError>);

    expect(yields).toBeGreaterThan(0);
    expect(emulator.status()).toMatchObject({
      phase: "error",
      virtualElapsedMs: 1,
      error: { diagnostic: { kind: "zero-duration-cycle" } },
    });
    expect(emulator.state().lifecycle).toBe("started");
    const state = emulator.state();
    if (state.lifecycle !== "started") throw new Error("session not started");
    expect(state.works.at(-1)).toMatchObject({
      state: "ready",
      phase: "ready",
      visits: { execute: 1, cycle: 3 },
    });
    const logicalEvents = writes
      .flat()
      .filter(
        (event) =>
          event.type === "DISPATCH_RESPONSE" &&
          event.payload.transitionId === "cycle",
      );
    expect(logicalEvents).toHaveLength(3);
    expect(
      writes
        .flat()
        .some(
          (event) =>
            event.type === "DISPATCH_RESPONSE" &&
            event.payload.outcome === "FAILED",
        ),
    ).toBe(false);
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

const joinFactory = {
  name: "join-factory",
  orchestrator: { kind: "PETRI" },
  workTypes: [
    {
      name: "left",
      states: [
        { name: "ready", type: "INITIAL" },
        { name: "done", type: "TERMINAL" },
      ],
    },
    {
      name: "right",
      states: [
        { name: "ready", type: "INITIAL" },
        { name: "done", type: "TERMINAL" },
      ],
    },
    {
      name: "joined",
      states: [{ name: "ready", type: "INITIAL" }],
    },
  ],
  workers: [{ name: "join-worker", type: "AGENT_WORKER" }],
  workstations: [
    {
      name: "join",
      type: "AGENT_RUN",
      worker: "join-worker",
      inputs: [
        { workType: "left", state: "ready" },
        { workType: "right", state: "ready" },
      ],
      outputs: [
        { workType: "left", state: "done" },
        { workType: "joined", state: "ready" },
        { workType: "right", state: "done" },
      ],
    },
  ],
} satisfies FactoryDefinition;

function joinScenario(
  overrides: Partial<FactoryEmulatorScenario> = {},
): FactoryEmulatorScenario {
  return {
    schemaVersion: "factory-emulator-scenario/v1",
    id: "join-scenario",
    factory: { name: joinFactory.name },
    seed: "join-seed",
    startAt: "2026-07-18T16:00:00.000Z",
    rules: [
      {
        id: "join-outcomes",
        selector: { workstation: "join" },
        cursor: { scope: "lineage", input: "rootWorkId" },
        outcomes: [
          { result: "accepted", durationMs: 5, output: "joined output" },
        ],
        exhaustion: "repeat-last",
      },
    ],
    unmatched: { behavior: "error" },
    ...overrides,
  };
}

function joinHarness(
  scenarioInput: FactoryEmulatorScenario,
  factoryInput: FactoryDefinition = joinFactory,
) {
  return createFactoryEmulatorSession({
    factory: factoryInput,
    scenario: scenarioInput,
    sink: { write: async () => undefined },
  });
}

describe("Factory emulator multi-input binding and output routing", () => {
  it("keeps an incomplete join ready until every declared input is present", async () => {
    const emulator = joinHarness(
      joinScenario({
        initialSubmissions: [
          { name: "left-only", workType: "left", state: "ready" },
        ],
      }),
    );
    await emulator.start();

    const blocked = await emulator.advanceToNext();
    expect(blocked).toMatchObject({ status: "idle", batches: [] });
    expect(blocked.state.works[0]).toMatchObject({ phase: "ready" });

    await emulator.submit({
      name: "right-later",
      workType: "right",
      state: "ready",
    });
    const dispatched = await emulator.advanceToNext();
    expect(dispatched.batches[0]?.[0]).toMatchObject({
      type: "DISPATCH_REQUEST",
      payload: {
        inputs: [
          { workId: dispatched.state.works[0]?.workId },
          { workId: dispatched.state.works[1]?.workId },
        ],
      },
    });
    expect(dispatched.state.works.map(({ phase }) => phase)).toEqual([
      "active",
      "active",
    ]);
  });

  it("selects the same independent join bindings from competing inputs", async () => {
    const scenarioInput = joinScenario({
      initialSubmissions: [
        { name: "right-two", workType: "right", state: "ready" },
        { name: "left-one", workType: "left", state: "ready" },
        { name: "right-one", workType: "right", state: "ready" },
        { name: "left-two", workType: "left", state: "ready" },
      ],
    });
    const dispatchOnce = async () => {
      const emulator = joinHarness(scenarioInput);
      await emulator.start();
      const receipt = await emulator.advanceToNext();
      return receipt.batches[0]?.map(
        ({ payload }) => (payload as { inputs: { workId: string }[] }).inputs,
      );
    };

    const first = await dispatchOnce();
    expect(first).toHaveLength(2);
    expect(await dispatchOnce()).toEqual(first);
    expect(new Set(first?.flat().map(({ workId }) => workId)).size).toBe(4);
  });

  it("bounds deterministic join derivation with the synchronous Work budget", async () => {
    const initialSubmissions = ["left", "right"].flatMap((workType) =>
      Array.from({ length: 8 }, (_, index) => ({
        name: `${workType}-${String(7 - index).padStart(2, "0")}`,
        workType,
        state: "ready",
      })),
    );
    const dispatchOnce = async () => {
      const emulator = createFactoryEmulatorSession({
        factory: joinFactory,
        scenario: joinScenario({ initialSubmissions }),
        sink: { write: async () => undefined },
        limits: { maxSynchronousWorkItems: 72 },
      });
      await emulator.start();
      return emulator.advanceToNext();
    };

    const first = await dispatchOnce();
    const second = await dispatchOnce();
    expect(second.batches).toEqual(first.batches);
    expect(first.batches[0]?.length).toBeLessThanOrEqual(50);

    const bounded = createFactoryEmulatorSession({
      factory: joinFactory,
      scenario: joinScenario({ initialSubmissions }),
      sink: { write: async () => undefined },
      limits: { maxSynchronousWorkItems: 71 },
    });
    await bounded.start();
    const before = bounded.state();
    await expect(bounded.advanceToNext()).rejects.toMatchObject({
      diagnostic: {
        kind: "bounded-work-exceeded",
        limit: "synchronousWorkItems",
        configured: 71,
        observed: 72,
      },
    });
    expect(bounded.state()).toEqual(before);
  });
});

describe("Factory emulator multi-output routing and join cursors", () => {
  it("routes accepted fan-out in authored order with output payloads", async () => {
    const emulator = joinHarness(
      joinScenario({
        initialSubmissions: [
          {
            name: "left-input",
            workType: "left",
            state: "ready",
            input: "left payload",
          },
          {
            name: "right-input",
            workType: "right",
            state: "ready",
            input: "right payload",
          },
        ],
      }),
    );
    await emulator.start();
    await emulator.advanceToNext();
    const completed = await emulator.advanceToNext();
    const response = completed.batches[0]?.[0];
    if (response === undefined) throw new Error("missing dispatch response");
    const outputWork = (
      response.payload as {
        outputWork: {
          workTypeName: string;
          state: { name: string };
          payload: string;
        }[];
      }
    ).outputWork;

    expect(
      outputWork.map(({ workTypeName, state }) => [workTypeName, state.name]),
    ).toEqual([
      ["left", "done"],
      ["joined", "ready"],
      ["right", "done"],
    ]);
    expect(outputWork.map(({ payload }) => payload)).toEqual([
      "joined output",
      "joined output",
      "joined output",
    ]);
    expect(
      completed.state.works.slice(-3).map(({ workType, state, phase }) => ({
        workType,
        state,
        phase,
      })),
    ).toEqual([
      { workType: "left", state: "done", phase: "completed" },
      { workType: "joined", state: "ready", phase: "ready" },
      { workType: "right", state: "done", phase: "completed" },
    ]);
  });

  it("preserves matching inputs and advances a join cursor after continued re-entry", async () => {
    const loopingFactory = {
      ...joinFactory,
      workstations: [
        {
          ...joinFactory.workstations[0],
          workPropagation: { mode: "PRESERVE_INPUT" },
          onContinue: [
            { workType: "left", state: "ready" },
            { workType: "right", state: "ready" },
          ],
        },
      ],
    } satisfies FactoryDefinition;
    const emulator = joinHarness(
      joinScenario({
        initialSubmissions: [
          {
            name: "left-loop",
            workType: "left",
            state: "ready",
            input: "left payload",
          },
          {
            name: "right-loop",
            workType: "right",
            state: "ready",
            input: "right payload",
          },
        ],
        rules: [
          {
            id: "join-outcomes",
            selector: { workstation: "join" },
            cursor: { scope: "lineage", input: "rootWorkId" },
            outcomes: [
              { result: "continued", durationMs: 0, output: "ignored" },
              { result: "accepted", durationMs: 1, output: "second" },
            ],
            exhaustion: "fail",
          },
        ],
      }),
      loopingFactory,
    );
    await emulator.start();
    await emulator.advanceToNext();
    const continued = await emulator.advanceToNext();
    const response = continued.batches[0]?.[0];
    if (response === undefined) throw new Error("missing dispatch response");
    const continuedOutput = (
      response.payload as {
        outputWork: { payload: string }[];
      }
    ).outputWork;
    expect(continuedOutput.map(({ payload }) => payload)).toEqual([
      "left payload",
      "right payload",
    ]);

    const repeated = await emulator.advanceToNext();
    expect(
      repeated.state.works.find(({ phase }) => phase === "active")?.dispatch,
    ).toMatchObject({ outcome: { result: "accepted", output: "second" } });
    expect(Object.values(repeated.state.ruleCursors)).toEqual([2]);
  });
});

const outcomeRoutingFactory = {
  name: "outcome-routing-factory",
  orchestrator: { kind: "PETRI" },
  workTypes: [
    {
      name: "task",
      states: [
        { name: "ready", type: "INITIAL" },
        { name: "review", type: "PROCESSING" },
        { name: "retry", type: "PROCESSING" },
        { name: "done", type: "TERMINAL" },
        { name: "failed", type: "FAILED" },
      ],
    },
  ],
  workers: [{ name: "routing-worker", type: "AGENT_WORKER" }],
  workstations: [
    {
      name: "route-task",
      type: "AGENT_RUN",
      behavior: "STANDARD",
      worker: "routing-worker",
      inputs: [{ workType: "task", state: "ready" }],
      outputs: [{ workType: "task", state: "done" }],
      onContinue: [{ workType: "task", state: "retry" }],
      onRejection: [
        { workType: "task", state: "review" },
        { workType: "task", state: "retry" },
      ],
      onFailure: [{ workType: "task", state: "review" }],
    },
  ],
} satisfies FactoryDefinition;

function outcomeRoutingScenario(
  outcomes: FactoryEmulatorScenario["rules"][number]["outcomes"],
): FactoryEmulatorScenario {
  return {
    schemaVersion: "factory-emulator-scenario/v1",
    id: "outcome-routing-scenario",
    factory: { name: outcomeRoutingFactory.name },
    seed: "outcome-routing-seed",
    startAt: "2026-07-18T16:00:00.000Z",
    initialSubmissions: [
      { name: "routed-task", workType: "task", state: "ready" },
    ],
    rules: [
      {
        id: "route-outcomes",
        selector: { workstation: "route-task" },
        cursor: { scope: "lineage", input: "rootWorkId" },
        outcomes,
        exhaustion: "fail",
      },
    ],
    unmatched: { behavior: "error" },
  };
}

async function completeFirstOutcome(
  factoryInput: FactoryDefinition,
  scenarioInput: FactoryEmulatorScenario,
) {
  const emulator = createFactoryEmulatorSession({
    factory: factoryInput,
    scenario: scenarioInput,
    sink: { write: async () => undefined },
  });
  await emulator.start();
  await emulator.advanceToNext();
  return { emulator, receipt: await emulator.advanceToNext() };
}

function routedStatesFrom(response: FactoryEvent | undefined): string[] {
  return (
    (
      response?.payload as
        | { outputWork?: { state: { name: string } }[] }
        | undefined
    )?.outputWork?.map(({ state }) => state.name) ?? []
  );
}

describe("Factory emulator standard and repeater outcome routing", () => {
  it("uses the Factory workstation identity for worker dispatch events", async () => {
    const emulator = createFactoryEmulatorSession({
      factory: outcomeRoutingFactory,
      scenario: outcomeRoutingScenario([
        { result: "accepted", durationMs: 0, output: "routed output" },
      ]),
      sink: { write: async () => undefined },
    });
    await emulator.start();

    const dispatched = await emulator.advanceToNext();
    const completed = await emulator.advanceToNext();

    expect(dispatched.batches[0]?.[0]).toMatchObject({
      type: "DISPATCH_REQUEST",
      payload: { transitionId: "route-task" },
    });
    expect(completed.batches[0]?.[0]).toMatchObject({
      type: "DISPATCH_RESPONSE",
      payload: {
        transitionId: "route-task",
        outputWork: [
          {
            workTypeName: "task",
            state: { name: "done" },
            payload: "routed output",
          },
        ],
      },
    });
  });

  it.each([
    ["continued", "CONTINUE", ["retry"]],
    ["rejected", "REJECTED", ["review", "retry"]],
    ["failed", "FAILED", ["review"]],
  ] as const)(
    "gives explicit %s routes precedence in authored order",
    async (result, canonicalOutcome, expectedStates) => {
      const outcome =
        result === "failed"
          ? { result, durationMs: 0, error: "scripted failure" }
          : { result, durationMs: 0 };
      const { receipt } = await completeFirstOutcome(
        outcomeRoutingFactory,
        outcomeRoutingScenario([outcome]),
      );
      const response = receipt.batches[0]?.[0];

      expect(response).toMatchObject({
        type: "DISPATCH_RESPONSE",
        payload: { outcome: canonicalOutcome },
      });
      expect(routedStatesFrom(response)).toEqual(expectedStates);
    },
  );
});

describe("Factory emulator failure-lane payload routing", () => {
  it.each([
    ["failed", "explicit failure", "review"],
    ["failed", "implicit failure", "failed"],
    ["rejected", "explicit rejection-to-failure", "failed"],
    ["rejected", "implicit rejection-to-failure", "failed"],
  ] as const)(
    "preserves request payload for %s through %s routing",
    async (result, routeKind, expectedState) => {
      const factoryInput = structuredClone(
        outcomeRoutingFactory,
      ) as FactoryDefinition;
      const workstation = factoryInput.workstations?.[0];
      if (workstation === undefined) throw new Error("missing workstation");
      if (routeKind === "implicit failure") workstation.onFailure = [];
      if (routeKind === "explicit rejection-to-failure") {
        workstation.onRejection = [{ workType: "task", state: "failed" }];
      }
      if (routeKind === "implicit rejection-to-failure") {
        workstation.onRejection = [];
      }
      const outcome =
        result === "failed"
          ? {
              result,
              durationMs: 0,
              output: "worker output",
              error: "scripted failure",
            }
          : { result, durationMs: 0, output: "worker output" };
      const scenarioInput = outcomeRoutingScenario([outcome]);
      scenarioInput.initialSubmissions = [
        {
          name: "routed-task",
          workType: "task",
          state: "ready",
          input: "request payload",
        },
      ];

      const { receipt } = await completeFirstOutcome(
        factoryInput,
        scenarioInput,
      );
      const routed = receipt.state.works.at(-1);
      expect(routed).toMatchObject({
        state: expectedState,
        input: "request payload",
      });
      expect(receipt.batches[0]?.[0]).toMatchObject({
        payload: {
          output: "worker output",
          outputWork: [
            {
              state: { name: expectedState },
              payload: "request payload",
            },
          ],
        },
      });
    },
  );

  it("keeps worker output for rejection routes that are not failure states", async () => {
    const scenarioInput = outcomeRoutingScenario([
      { result: "rejected", durationMs: 0, output: "worker output" },
    ]);
    scenarioInput.initialSubmissions = [
      {
        name: "routed-task",
        workType: "task",
        state: "ready",
        input: "request payload",
      },
    ];
    const { receipt } = await completeFirstOutcome(
      outcomeRoutingFactory,
      scenarioInput,
    );

    expect(
      receipt.state.works.slice(-2).map(({ state, input }) => ({
        state,
        input,
      })),
    ).toEqual([
      { state: "review", input: "worker output" },
      { state: "retry", input: "worker output" },
    ]);
  });
});

describe("Factory emulator implicit outcome routing", () => {
  it.each([
    ["failed", "STANDARD", "failed"],
    ["rejected", "STANDARD", "failed"],
    ["rejected", "REPEATER", "ready"],
  ] as const)(
    "routes implicit %s for %s workstations to %s",
    async (result, behavior, expectedState) => {
      const factoryInput = structuredClone(
        outcomeRoutingFactory,
      ) as FactoryDefinition;
      const workstation = factoryInput.workstations?.[0];
      if (workstation === undefined) throw new Error("missing workstation");
      workstation.behavior = behavior;
      workstation.onFailure = [];
      workstation.onRejection = [];
      const outcome =
        result === "failed"
          ? { result, durationMs: 0, error: "scripted failure" }
          : { result, durationMs: 0 };
      const { receipt } = await completeFirstOutcome(
        factoryInput,
        outcomeRoutingScenario([outcome]),
      );

      expect(routedStatesFrom(receipt.batches[0]?.[0])).toEqual([
        expectedState,
      ]);
    },
  );

  it.each([
    ["REPEATER", false],
    ["STANDARD", true],
  ] as const)(
    "re-enters a %s rejection loop and advances its lineage cursor",
    async (behavior, explicitReviewRoute) => {
      const factoryInput = structuredClone(
        outcomeRoutingFactory,
      ) as FactoryDefinition;
      const workstation = factoryInput.workstations?.[0];
      if (workstation === undefined) throw new Error("missing workstation");
      workstation.behavior = behavior;
      workstation.onRejection = explicitReviewRoute
        ? [{ workType: "task", state: "ready" }]
        : [];
      const { emulator, receipt: rejected } = await completeFirstOutcome(
        factoryInput,
        outcomeRoutingScenario([
          { result: "rejected", durationMs: 0, feedback: "try again" },
          { result: "accepted", durationMs: 1, output: "finished" },
        ]),
      );

      expect(routedStatesFrom(rejected.batches[0]?.[0])).toEqual(["ready"]);
      const repeated = await emulator.advanceToNext();
      expect(
        repeated.state.works.find(({ phase }) => phase === "active")?.dispatch,
      ).toMatchObject({ outcome: { result: "accepted", output: "finished" } });
      expect(Object.values(repeated.state.ruleCursors)).toEqual([2]);
    },
  );
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
});

describe("Factory emulator scheduler batch separation", () => {
  it("routes simultaneous completions atomically before dispatching newly eligible Work", async () => {
    const chainFactory = {
      ...executionFactory,
      workTypes: [
        {
          name: "task",
          states: [
            { name: "ready", type: "INITIAL" },
            { name: "prepared", type: "INTERMEDIATE" },
            { name: "done", type: "TERMINAL" },
          ],
        },
      ],
      workstations: [
        {
          name: "prepare",
          type: "AGENT_RUN",
          worker: "scripted-worker",
          inputs: [{ workType: "task", state: "ready" }],
          outputs: [{ workType: "task", state: "prepared" }],
        },
        {
          name: "finish",
          type: "AGENT_RUN",
          worker: "scripted-worker",
          inputs: [{ workType: "task", state: "prepared" }],
          outputs: [{ workType: "task", state: "done" }],
        },
      ],
    } satisfies FactoryDefinition;
    const writes: FactoryEvent[][] = [];
    const emulator = createFactoryEmulatorSession({
      factory: chainFactory,
      scenario: executionScenario({
        initialSubmissions: [
          { name: "first", workType: "task", state: "ready" },
          { name: "second", workType: "task", state: "ready" },
        ],
        rules: [
          {
            id: "prepare-outcome",
            selector: { workstation: "prepare" },
            cursor: { scope: "lineage", input: "rootWorkId" },
            outcomes: [{ result: "accepted", durationMs: 25 }],
            exhaustion: "repeat-last",
          },
          {
            id: "finish-outcome",
            selector: { workstation: "finish" },
            cursor: { scope: "lineage", input: "rootWorkId" },
            outcomes: [{ result: "accepted", durationMs: 0 }],
            exhaustion: "repeat-last",
          },
        ],
      }),
      sink: {
        write: async (events) => writes.push(structuredClone(events)),
      },
    });
    await emulator.start();
    await emulator.advanceToNext();

    const completed = await emulator.advanceToNext();

    expect(completed.batches).toHaveLength(1);
    expect(completed.batches[0]?.map(({ type }) => type)).toEqual([
      "DISPATCH_RESPONSE",
      "DISPATCH_RESPONSE",
    ]);
    expect(
      completed.state.works.filter(({ phase }) => phase === "ready"),
    ).toHaveLength(2);
    expect(writes.at(-1)).toEqual(completed.batches[0]);

    const followingBatch = await emulator.advanceToNext();
    expect(followingBatch.batches[0]?.map(({ type }) => type)).toEqual([
      "DISPATCH_REQUEST",
      "DISPATCH_REQUEST",
    ]);
    expect(followingBatch.virtualElapsedMs).toBe(25);
  });
});

describe("Factory emulator completion transaction retry", () => {
  it("retries a rejected simultaneous completion without advancing any scheduler state", async () => {
    const resourceFactory = {
      ...executionFactory,
      resources: [{ name: "agent-slot", capacity: 2 }],
      workstations: executionFactory.workstations.map((workstation) => ({
        ...workstation,
        resources: [{ name: "agent-slot", capacity: 1 }],
      })),
    } satisfies FactoryDefinition;
    const attempts: FactoryEvent[][] = [];
    let rejectCompletion = true;
    const emulator = createFactoryEmulatorSession({
      factory: resourceFactory,
      scenario: executionScenario({
        initialSubmissions: [
          { name: "first", workType: "task", state: "ready" },
          { name: "second", workType: "task", state: "ready" },
        ],
      }),
      sink: {
        write: async (events) => {
          attempts.push(structuredClone(events));
          if (
            rejectCompletion &&
            events.some(({ type }) => type === "DISPATCH_RESPONSE")
          ) {
            rejectCompletion = false;
            throw new Error("completion recording unavailable");
          }
        },
      },
    });
    await emulator.start();
    await emulator.advanceToNext();
    const before = emulator.state();
    const budgetBefore = emulator.status().budgetUsage;
    expect(
      before.works.flatMap(({ dispatch }) => dispatch?.resources ?? []),
    ).toEqual([
      { name: "agent-slot", capacity: 1 },
      { name: "agent-slot", capacity: 1 },
    ]);
    expect(Object.values(before.ruleCursors)).toEqual([1, 1]);

    await expect(emulator.advanceToNext()).rejects.toThrow(
      "completion recording unavailable",
    );

    expect(emulator.state()).toEqual(before);
    expect(emulator.status()).toMatchObject({
      phase: "error",
      virtualTime: before.virtualTime,
      virtualElapsedMs: before.virtualElapsedMs,
      budgetUsage: budgetBefore,
      pendingTransaction: {
        command: "advanceToNext",
        phase: "sink-write",
        eventCount: 2,
      },
    });

    const retried = await emulator.advanceToNext();
    expect(attempts.at(-1)).toEqual(attempts.at(-2));
    expect(retried.batches).toEqual([attempts.at(-1)]);
    expect(retried.state.counters.completedDispatches).toBe(
      before.counters.completedDispatches + 2,
    );
  });
});

describe("Factory emulator virtual-time duration validation", () => {
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

  it("resets a budget-paused run and reproduces its exact failure boundary", async () => {
    const history: FactoryEvent[] = [];
    const emulator = limitedHarness(
      { maxEvents: 5, maxCompletedDispatches: 1 },
      undefined,
      {
        sink: {
          write: async (events) => history.push(...structuredClone(events)),
        },
      },
    );

    const runToBoundary = async () => {
      await emulator.start();
      await expect(emulator.advanceBy(25)).rejects.toMatchObject({
        diagnostic: {
          kind: "budget-exceeded",
          limit: "events",
          configured: 5,
          observed: 6,
        },
      });
      return {
        history: structuredClone(history),
        state: emulator.state(),
        status: emulator.status(),
      };
    };

    const first = await runToBoundary();
    const reset = emulator.reset();
    history.length = 0;

    expect(reset.state).toEqual({
      lifecycle: "pre-start",
      virtualElapsedMs: 0,
      counters: { commands: 0, events: 0, completedDispatches: 0 },
    });
    expect(emulator.status()).toMatchObject({ phase: "idle" });

    const second = await runToBoundary();
    expect(second).toEqual(first);
    expect(second.history).toHaveLength(5);
    expect(second.status).toMatchObject({
      phase: "error",
      error: {
        code: "execution_paused",
        diagnostic: { kind: "budget-exceeded", limit: "events" },
      },
    });
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
