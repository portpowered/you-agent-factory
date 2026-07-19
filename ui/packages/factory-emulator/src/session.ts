// biome-ignore-all lint/style/noExcessiveLinesPerFile: The kernel stays cohesive while its transaction model is extended in the next story.
import type {
  FactoryDefinition,
  FactoryEvent,
} from "@you-agent-factory/client";
import { inspectFactoryEmulatorCompatibility } from "./compatibility.js";
import type { FactoryEventSink } from "./event-sink.js";
import { safeParseFactoryEmulatorScenario } from "./scenario.js";
import type {
  FactoryEmulatorInitialSubmission,
  FactoryEmulatorOutcome,
  FactoryEmulatorRule,
  FactoryEmulatorScenario,
  FactoryEmulatorScenarioIssue,
} from "./scenario-contracts.js";
import {
  DEFAULT_FACTORY_EMULATOR_LIMITS,
  FACTORY_EMULATOR_LIMIT_HARD_CAPS,
  type FactoryEmulatorAdvanceReceipt,
  type FactoryEmulatorBudgetUsage,
  type FactoryEmulatorCommand,
  type FactoryEmulatorConfigurationDiagnostic,
  FactoryEmulatorConfigurationError,
  FactoryEmulatorDurationError,
  FactoryEmulatorLifecycleError,
  type FactoryEmulatorLimits,
  FactoryEmulatorPendingCommandError,
  type FactoryEmulatorSession,
  type FactoryEmulatorSessionError,
  type FactoryEmulatorSessionOptions,
  type FactoryEmulatorSessionState,
  type FactoryEmulatorSessionStatus,
  type FactoryEmulatorSessionWork,
  type FactoryEmulatorStartReceipt,
  FactoryEmulatorSubmissionError,
  type FactoryEmulatorSubmitReceipt,
  type ResolvedFactoryEmulatorLimits,
} from "./session-contracts.js";

export * from "./session-contracts.js";

const LIMIT_NAMES = Object.keys(
  DEFAULT_FACTORY_EMULATOR_LIMITS,
) as (keyof ResolvedFactoryEmulatorLimits)[];

const PRE_START_STATE = {
  lifecycle: "pre-start",
  virtualElapsedMs: 0,
  counters: { commands: 0, events: 0, completedDispatches: 0 },
} as const satisfies FactoryEmulatorSessionState;

type StartedState = Extract<
  FactoryEmulatorSessionState,
  { lifecycle: "started" }
>;

interface ValidatedConfiguration {
  readonly factory: FactoryDefinition;
  readonly scenario: FactoryEmulatorScenario;
  readonly sink: FactoryEventSink;
  readonly limits: ResolvedFactoryEmulatorLimits;
  readonly yieldControl?: () => void | PromiseLike<void>;
}

interface WorkExecution {
  readonly rule?: FactoryEmulatorRule;
  readonly outcome?: FactoryEmulatorOutcome;
  readonly workstation?: NonNullable<FactoryDefinition["workstations"]>[number];
}

function clone<Value>(value: Value): Value {
  return JSON.parse(JSON.stringify(value)) as Value;
}

function resolveLimits(
  configured: FactoryEmulatorLimits | undefined,
  diagnostics: FactoryEmulatorConfigurationDiagnostic[],
): ResolvedFactoryEmulatorLimits {
  const limits = { ...DEFAULT_FACTORY_EMULATOR_LIMITS, ...configured };
  for (const name of LIMIT_NAMES) {
    const value = limits[name];
    const hardCap = FACTORY_EMULATOR_LIMIT_HARD_CAPS[name];
    if (!Number.isSafeInteger(value) || value < 1 || value > hardCap) {
      diagnostics.push({
        code: "invalid_limit",
        path: ["limits", name],
        message: `${name} must be a positive safe integer no greater than ${hardCap}.`,
      });
    }
  }
  return Object.freeze(limits);
}

function validateConfiguration(
  options: FactoryEmulatorSessionOptions,
): ValidatedConfiguration {
  const diagnostics: FactoryEmulatorConfigurationDiagnostic[] = [];
  const factory = options?.factory;
  if (
    factory === null ||
    typeof factory !== "object" ||
    typeof factory.name !== "string" ||
    factory.name.trim() === ""
  ) {
    diagnostics.push({
      code: "invalid_factory",
      path: ["factory"],
      message: "factory must be a Factory definition with a non-empty name.",
    });
  }

  let scenario: FactoryEmulatorScenario | undefined;
  if (diagnostics.length === 0) {
    const parsed = safeParseFactoryEmulatorScenario(options.scenario, factory);
    if (parsed.success) scenario = parsed.data;
    else diagnostics.push(...parsed.issues);
  }
  if (diagnostics.length === 0) {
    const compatibility = inspectFactoryEmulatorCompatibility(factory);
    if (!compatibility.supported) {
      diagnostics.push(
        ...compatibility.diagnostics.map((issue) => ({
          code: "incompatible_factory" as const,
          path: ["factory", ...issue.path],
          message: issue.message,
        })),
      );
    }
  }
  if (options?.sink === null || typeof options?.sink?.write !== "function") {
    diagnostics.push({
      code: "invalid_sink",
      path: ["sink"],
      message: "sink must provide an asynchronous write(events) function.",
    });
  }
  if (
    options?.yieldControl !== undefined &&
    typeof options.yieldControl !== "function"
  ) {
    diagnostics.push({
      code: "invalid_yield_control",
      path: ["yieldControl"],
      message: "yieldControl must be a function when provided.",
    });
  }
  const limits = resolveLimits(options?.limits, diagnostics);
  if (diagnostics.length > 0 || scenario === undefined) {
    throw new FactoryEmulatorConfigurationError(diagnostics);
  }
  return {
    factory: clone(factory),
    scenario,
    sink: options.sink,
    limits,
    yieldControl: options.yieldControl,
  };
}

function deterministicHash(value: string): string {
  let hash = 0xcbf29ce484222325n;
  for (const character of value) {
    hash ^= BigInt(character.codePointAt(0) ?? 0);
    hash = BigInt.asUintN(64, hash * 0x100000001b3n);
  }
  return hash.toString(16).padStart(16, "0");
}

function identity(kind: string, ...coordinates: readonly unknown[]): string {
  return `emulator-${kind}-${deterministicHash(JSON.stringify(coordinates))}`;
}

function sessionIdentity(
  factory: FactoryDefinition,
  scenario: FactoryEmulatorScenario,
): string {
  return identity(
    "session",
    factory.id ?? factory.name,
    scenario.id,
    scenario.seed,
  );
}

function virtualTimeAt(scenario: FactoryEmulatorScenario, elapsedMs: number) {
  const timestamp = Date.parse(scenario.startAt) + elapsedMs;
  const instant = new Date(timestamp);
  if (
    !Number.isSafeInteger(elapsedMs) ||
    !Number.isFinite(timestamp) ||
    !Number.isFinite(instant.getTime())
  ) {
    throw new FactoryEmulatorDurationError(elapsedMs);
  }
  return instant.toISOString();
}

function eventContext(
  state: StartedState,
  sequence: number,
  eventTime: string,
  context: Partial<FactoryEvent["context"]> = {},
): FactoryEvent["context"] {
  return {
    sequence,
    tick: state.counters.commands,
    eventTime,
    sessionId: state.sessionId,
    sessionSequence: sequence,
    orchestratorKind: "PETRI",
    source: "emulator",
    ...context,
  };
}

function bootstrapEvents(
  factory: FactoryDefinition,
  scenario: FactoryEmulatorScenario,
  sessionId: string,
): readonly FactoryEvent[] {
  const eventTime = scenario.startAt;
  const context = (sequence: number) => ({
    sequence,
    tick: 0,
    eventTime,
    sessionId,
    sessionSequence: sequence,
    orchestratorKind: "PETRI" as const,
    source: "emulator",
  });
  return [
    {
      schemaVersion: "agent-factory.event.v1",
      id: `${sessionId}/event/0/run-request`,
      type: "RUN_REQUEST",
      context: context(0),
      payload: { recordedAt: eventTime, factory: clone(factory) },
    },
    {
      schemaVersion: "agent-factory.event.v1",
      id: `${sessionId}/event/1/initial-structure-request`,
      type: "INITIAL_STRUCTURE_REQUEST",
      context: context(1),
      payload: { factory: clone(factory) },
    },
    {
      schemaVersion: "agent-factory.event.v1",
      id: `${sessionId}/event/2/session-started`,
      type: "SESSION_STARTED",
      context: context(2),
      payload: { factoryId: factory.id ?? factory.name, startedAt: eventTime },
    },
  ];
}

function ruleMatches(
  rule: FactoryEmulatorRule,
  submission: FactoryEmulatorInitialSubmission,
  workstation: NonNullable<FactoryDefinition["workstations"]>[number],
): boolean {
  const selector = rule.selector;
  return (
    (selector.workstation === undefined ||
      selector.workstation === workstation.name) &&
    (selector.worker === undefined || selector.worker === workstation.worker) &&
    (selector.input?.workType === undefined ||
      selector.input.workType === submission.workType) &&
    (selector.input?.state === undefined ||
      selector.input.state === submission.state) &&
    (selector.input?.name === undefined ||
      selector.input.name === submission.name)
  );
}

function executionFor(
  configuration: ValidatedConfiguration,
  submission: FactoryEmulatorInitialSubmission,
  invocationIndex = 0,
): WorkExecution {
  const workstation = configuration.factory.workstations?.find((candidate) =>
    candidate.inputs.some(
      (input) =>
        input.workType === submission.workType &&
        input.state === submission.state,
    ),
  );
  if (workstation === undefined) return {};
  const rule = configuration.scenario.rules.find((candidate) =>
    ruleMatches(candidate, submission, workstation),
  );
  if (rule === undefined) {
    return configuration.scenario.unmatched.behavior === "outcome"
      ? { workstation, outcome: configuration.scenario.unmatched.outcome }
      : { workstation };
  }
  const outcome =
    rule.outcomes[invocationIndex] ??
    (rule.exhaustion === "repeat-last" ? rule.outcomes.at(-1) : undefined);
  return { rule, workstation, outcome };
}

function workRequestCalculation(
  configuration: ValidatedConfiguration,
  state: StartedState,
  submissions: readonly FactoryEmulatorInitialSubmission[],
  command: "start" | "submit",
): {
  readonly event: FactoryEvent;
  readonly works: readonly FactoryEmulatorSessionWork[];
} {
  const requestId = identity(
    "request",
    state.sessionId,
    command,
    state.counters.commands,
  );
  const works = submissions.map((submission, ordinal) => {
    const workId = identity(
      "work",
      state.sessionId,
      command,
      state.counters.commands,
      ordinal,
      submission,
    );
    const traceId = identity("trace", workId, requestId);
    return {
      submissionId: submission.name,
      requestId,
      traceId,
      workId,
      workType: submission.workType,
      state: submission.state,
      ...(submission.input === undefined ? {} : { input: submission.input }),
      ...(submission.parent === undefined ? {} : { parent: submission.parent }),
      phase: "ready" as const,
    };
  });
  const sequence = state.counters.events;
  return {
    works,
    event: {
      schemaVersion: "agent-factory.event.v1",
      id: identity("event", state.sessionId, sequence, "WORK_REQUEST"),
      type: "WORK_REQUEST",
      context: eventContext(state, sequence, state.virtualTime, {
        requestId,
        traceIds: works.map(({ traceId }) => traceId),
        workIds: works.map(({ workId }) => workId),
      }),
      payload: {
        type: "FACTORY_REQUEST_BATCH",
        source: "emulator",
        works: works.map((work) => ({
          name: work.submissionId,
          workId: work.workId,
          requestId: work.requestId,
          workTypeName: work.workType,
          state: {
            name: work.state,
            type:
              configuration.factory.workTypes
                ?.find(({ name }) => name === work.workType)
                ?.states.find(({ name }) => name === work.state)?.type ??
              "INITIAL",
          },
          currentChainingTraceId: work.traceId,
          traceId: work.traceId,
          ...(work.input === undefined ? {} : { payload: work.input }),
        })),
      },
    },
  };
}

function validateSubmissions(
  configuration: ValidatedConfiguration,
  state: StartedState,
  value:
    | FactoryEmulatorInitialSubmission
    | readonly FactoryEmulatorInitialSubmission[],
): readonly FactoryEmulatorInitialSubmission[] {
  const submissions = Array.isArray(value) ? value : [value];
  const parsed = safeParseFactoryEmulatorScenario(
    { ...configuration.scenario, initialSubmissions: submissions },
    configuration.factory,
  );
  const issues: FactoryEmulatorScenarioIssue[] = parsed.success
    ? []
    : parsed.issues.map((issue) => ({
        ...issue,
        path: ["submissions", ...issue.path.slice(1)],
      }));
  const known = new Set(state.works.map(({ submissionId }) => submissionId));
  submissions.forEach((submission, index) => {
    if (known.has(submission?.name)) {
      issues.push({
        category: "semantic",
        code: "duplicate_identity",
        path: ["submissions", index, "name"],
        message: `Work identity ${submission.name} already exists in this session.`,
      });
    }
  });
  if (issues.length > 0) throw new FactoryEmulatorSubmissionError(issues);
  return clone(submissions);
}

function rejectionMessage(rejection: unknown): string {
  if (rejection instanceof Error) return String(rejection.message);
  try {
    return String(rejection);
  } catch {
    return "Sink rejected with an unprintable value.";
  }
}

function budgetUsage(
  state: FactoryEmulatorSessionState,
  limits: ResolvedFactoryEmulatorLimits,
): FactoryEmulatorBudgetUsage {
  return {
    completedDispatches: {
      used: state.counters.completedDispatches,
      limit: limits.maxCompletedDispatches,
    },
    events: { used: state.counters.events, limit: limits.maxEvents },
    virtualElapsedMs: {
      used: state.virtualElapsedMs,
      limit: limits.maxVirtualElapsedMs,
    },
  };
}

function unfinished(state: StartedState): boolean {
  return state.works.some(({ phase }) => phase !== "completed");
}

/** Creates one validated, caller-owned deterministic Factory emulator session. */
// biome-ignore lint/complexity/noExcessiveLinesPerFunction: Command closures intentionally share one explicit session state owner.
export function createFactoryEmulatorSession(
  options: FactoryEmulatorSessionOptions,
): FactoryEmulatorSession {
  const configuration = validateConfiguration(options);
  const sessionId = sessionIdentity(
    configuration.factory,
    configuration.scenario,
  );
  let committedState: FactoryEmulatorSessionState = clone(PRE_START_STATE);
  let pendingBatch: readonly FactoryEvent[] | undefined;
  let commandInFlight: FactoryEmulatorCommand | undefined;
  let lastError: FactoryEmulatorSessionError | undefined;

  const status = (): FactoryEmulatorSessionStatus => {
    const common = {
      virtualTime:
        committedState.lifecycle === "started"
          ? committedState.virtualTime
          : configuration.scenario.startAt,
      virtualElapsedMs: committedState.virtualElapsedMs,
      budgetUsage: budgetUsage(committedState, configuration.limits),
    };
    if (lastError !== undefined) {
      return clone({
        ...common,
        phase: "error",
        reason: "The event sink rejected the pending transaction.",
        pendingTransaction: {
          command: lastError.command,
          phase: "sink-write",
          eventCount: pendingBatch?.length ?? 0,
        },
        error: lastError,
      });
    }
    if (commandInFlight !== undefined) {
      return clone({
        ...common,
        phase: "active",
        reason: `${commandInFlight} is awaiting the event sink.`,
        ...(pendingBatch === undefined
          ? {}
          : {
              pendingTransaction: {
                command: commandInFlight,
                phase: "sink-write" as const,
                eventCount: pendingBatch.length,
              },
            }),
      });
    }
    if (committedState.lifecycle === "pre-start") {
      return clone({
        ...common,
        phase: "idle",
        reason: "The session is ready to start.",
      });
    }
    if (committedState.works.some(({ phase }) => phase === "active")) {
      return clone({ ...common, phase: "active", reason: "Work is active." });
    }
    if (committedState.works.some(({ phase }) => phase === "ready")) {
      return clone({
        ...common,
        phase: "ready",
        reason:
          committedState.counters.commands === 1
            ? "Initial Work is ready for a later execution command."
            : "Work is ready.",
      });
    }
    if (committedState.works.some(({ phase }) => phase === "waiting")) {
      return clone({
        ...common,
        phase: "waiting",
        reason: "Work is waiting for a supported outcome.",
      });
    }
    return clone({
      ...common,
      phase: "idle",
      reason: "The open session has no unfinished Work.",
    });
  };

  const assertCommand = (command: FactoryEmulatorCommand): void => {
    if (commandInFlight !== undefined) {
      throw new FactoryEmulatorPendingCommandError(command, commandInFlight);
    }
    if (
      (command === "start" && committedState.lifecycle !== "pre-start") ||
      (command !== "start" && committedState.lifecycle !== "started")
    ) {
      throw new FactoryEmulatorLifecycleError(
        command,
        committedState.lifecycle,
      );
    }
  };

  const write = async (
    command: FactoryEmulatorCommand,
    batch: readonly FactoryEvent[],
    candidate: StartedState,
  ): Promise<void> => {
    pendingBatch = batch;
    try {
      await configuration.sink.write(clone(batch));
      committedState = candidate;
      pendingBatch = undefined;
      lastError = undefined;
    } catch (rejection) {
      lastError = {
        code: "sink_write_rejected",
        operation: "write",
        command,
        message: rejectionMessage(rejection),
      };
      throw rejection;
    }
  };

  const start = async (): Promise<FactoryEmulatorStartReceipt> => {
    assertCommand("start");
    commandInFlight = "start";
    lastError = undefined;
    try {
      const bootstrap = bootstrapEvents(
        configuration.factory,
        configuration.scenario,
        sessionId,
      );
      let candidate: StartedState = {
        lifecycle: "started",
        sessionId,
        virtualTime: configuration.scenario.startAt,
        virtualElapsedMs: 0,
        works: [],
        ruleCursors: {},
        counters: {
          commands: 1,
          events: bootstrap.length,
          completedDispatches: 0,
        },
      };
      const batches: (readonly FactoryEvent[])[] = [bootstrap];
      const initial = configuration.scenario.initialSubmissions ?? [];
      let combined = bootstrap;
      if (initial.length > 0) {
        const calculation = workRequestCalculation(
          configuration,
          { ...candidate, counters: { ...candidate.counters, commands: 0 } },
          initial,
          "start",
        );
        const submissionBatch = [calculation.event];
        batches.push(submissionBatch);
        combined = [...bootstrap, ...submissionBatch];
        candidate = {
          ...candidate,
          works: calculation.works,
          counters: { ...candidate.counters, events: combined.length },
        };
      }
      await write("start", combined, candidate);
      return clone({ status: "started", batches, state: candidate });
    } finally {
      commandInFlight = undefined;
    }
  };

  const submit = async (
    value:
      | FactoryEmulatorInitialSubmission
      | readonly FactoryEmulatorInitialSubmission[],
  ): Promise<FactoryEmulatorSubmitReceipt> => {
    assertCommand("submit");
    const state = committedState as StartedState;
    const submissions = validateSubmissions(configuration, state, value);
    commandInFlight = "submit";
    lastError = undefined;
    try {
      const calculation = workRequestCalculation(
        configuration,
        state,
        submissions,
        "submit",
      );
      const batch = [calculation.event];
      const candidate: StartedState = {
        ...state,
        works: [...state.works, ...calculation.works],
        counters: {
          ...state.counters,
          commands: state.counters.commands + 1,
          events: state.counters.events + 1,
        },
      };
      await write("submit", batch, candidate);
      return clone({ status: "submitted", batch, state: candidate });
    } finally {
      commandInFlight = undefined;
    }
  };

  const dispatchCalculation = (
    state: StartedState,
  ): { batch: readonly FactoryEvent[]; state: StartedState } => {
    const replacements = [...state.works];
    const cursors = { ...state.ruleCursors };
    const events: FactoryEvent[] = [];
    for (const [index, work] of state.works.entries()) {
      if (work.phase !== "ready") continue;
      const submission = {
        name: work.submissionId,
        workType: work.workType,
        state: work.state,
        ...(work.input === undefined ? {} : { input: work.input }),
        ...(work.parent === undefined ? {} : { parent: work.parent }),
      };
      const initial = executionFor(configuration, submission);
      const cursorKey =
        initial.rule === undefined
          ? undefined
          : `${initial.rule.id}:${work.workId}`;
      const invocation =
        cursorKey === undefined ? 0 : (cursors[cursorKey] ?? 0);
      const execution = executionFor(configuration, submission, invocation);
      if (
        execution.workstation === undefined ||
        execution.outcome === undefined
      ) {
        replacements[index] = { ...work, phase: "waiting" };
        continue;
      }
      if (cursorKey !== undefined) cursors[cursorKey] = invocation + 1;
      const transitionId = execution.rule?.id ?? "emulator-unmatched";
      const dispatchId = identity(
        "dispatch",
        work.workId,
        transitionId,
        invocation,
      );
      const completionId = identity("completion", dispatchId);
      const dueElapsedMs =
        state.virtualElapsedMs + execution.outcome.durationMs;
      virtualTimeAt(configuration.scenario, dueElapsedMs);
      replacements[index] = {
        ...work,
        phase: "active",
        dispatch: {
          dispatchId,
          completionId,
          transitionId,
          workstation: execution.workstation.name,
          worker: execution.workstation.worker,
          startedElapsedMs: state.virtualElapsedMs,
          dueElapsedMs,
          outcome: execution.outcome,
        },
      };
      const sequence = state.counters.events + events.length;
      events.push({
        schemaVersion: "agent-factory.event.v1",
        id: identity(
          "event",
          state.sessionId,
          sequence,
          "DISPATCH_REQUEST",
          dispatchId,
        ),
        type: "DISPATCH_REQUEST",
        context: eventContext(state, sequence, state.virtualTime, {
          dispatchId,
          requestId: work.requestId,
          traceIds: [work.traceId],
          workIds: [work.workId],
          currentChainingTraceId: work.traceId,
        }),
        payload: { transitionId, inputs: [{ workId: work.workId }] },
      });
    }
    return {
      batch: events,
      state: {
        ...state,
        works: replacements,
        ruleCursors: cursors,
        counters: {
          ...state.counters,
          events: state.counters.events + events.length,
        },
      },
    };
  };

  const completionCalculation = (
    state: StartedState,
    dueElapsedMs: number,
  ): { batch: readonly FactoryEvent[]; state: StartedState } => {
    const replacements = [...state.works];
    const events: FactoryEvent[] = [];
    const eventTime = virtualTimeAt(configuration.scenario, dueElapsedMs);
    for (const [index, work] of state.works.entries()) {
      if (
        work.phase !== "active" ||
        work.dispatch?.dueElapsedMs !== dueElapsedMs
      )
        continue;
      const { dispatch } = work;
      const sequence = state.counters.events + events.length;
      const outcome = {
        accepted: "ACCEPTED",
        continued: "CONTINUE",
        rejected: "REJECTED",
        failed: "FAILED",
      } as const;
      events.push({
        schemaVersion: "agent-factory.event.v1",
        id: identity(
          "event",
          state.sessionId,
          sequence,
          "DISPATCH_RESPONSE",
          dispatch.dispatchId,
        ),
        type: "DISPATCH_RESPONSE",
        context: eventContext(state, sequence, eventTime, {
          dispatchId: dispatch.dispatchId,
          requestId: work.requestId,
          traceIds: [work.traceId],
          workIds: [work.workId],
          currentChainingTraceId: work.traceId,
        }),
        payload: {
          completionId: dispatch.completionId,
          transitionId: dispatch.transitionId,
          outcome: outcome[dispatch.outcome.result],
          durationMillis: dispatch.dueElapsedMs - dispatch.startedElapsedMs,
          ...(dispatch.outcome.output === undefined
            ? {}
            : { output: dispatch.outcome.output }),
          ...(dispatch.outcome.feedback === undefined
            ? {}
            : { feedback: dispatch.outcome.feedback }),
          ...(dispatch.outcome.error === undefined
            ? {}
            : { error: dispatch.outcome.error }),
        },
      });
      replacements[index] = { ...work, phase: "completed" };
    }
    return {
      batch: events,
      state: {
        ...state,
        virtualElapsedMs: dueElapsedMs,
        virtualTime: eventTime,
        works: replacements,
        counters: {
          ...state.counters,
          events: state.counters.events + events.length,
          completedDispatches:
            state.counters.completedDispatches + events.length,
        },
      },
    };
  };

  const runAdvance = async (
    command: "advanceBy" | "advanceToNext",
    requestedDuration?: number,
  ): Promise<FactoryEmulatorAdvanceReceipt> => {
    assertCommand(command);
    const original = committedState as StartedState;
    if (
      requestedDuration !== undefined &&
      (!Number.isSafeInteger(requestedDuration) || requestedDuration < 0)
    ) {
      throw new FactoryEmulatorDurationError(requestedDuration);
    }
    const target =
      requestedDuration === undefined
        ? undefined
        : original.virtualElapsedMs + requestedDuration;
    if (target !== undefined) virtualTimeAt(configuration.scenario, target);
    commandInFlight = command;
    lastError = undefined;
    const batches: (readonly FactoryEvent[])[] = [];
    const fromVirtualTime = original.virtualTime;
    const wasUnfinished = unfinished(original);
    try {
      while (true) {
        const state = committedState as StartedState;
        if (state.works.some(({ phase }) => phase === "ready")) {
          const calculation = dispatchCalculation(state);
          if (calculation.batch.length > 0) {
            await write(command, calculation.batch, calculation.state);
            batches.push(calculation.batch);
            if (command === "advanceToNext") break;
            continue;
          }
          committedState = calculation.state;
        }
        const activeDeadlines = (committedState as StartedState).works.flatMap(
          (work) =>
            work.phase === "active" && work.dispatch !== undefined
              ? [work.dispatch.dueElapsedMs]
              : [],
        );
        if (activeDeadlines.length === 0) break;
        const earliest = Math.min(...activeDeadlines);
        if (target !== undefined && earliest > target) break;
        const calculation = completionCalculation(
          committedState as StartedState,
          earliest,
        );
        await write(command, calculation.batch, calculation.state);
        batches.push(calculation.batch);
        if (command === "advanceToNext") break;
      }
      let finalState = committedState as StartedState;
      if (
        target !== undefined &&
        wasUnfinished &&
        finalState.virtualElapsedMs < target
      ) {
        finalState = {
          ...finalState,
          virtualElapsedMs: target,
          virtualTime: virtualTimeAt(configuration.scenario, target),
        };
      }
      const madeProgress =
        batches.length > 0 || finalState.virtualTime !== fromVirtualTime;
      if (madeProgress) {
        finalState = {
          ...finalState,
          counters: {
            ...finalState.counters,
            commands: finalState.counters.commands + 1,
          },
        };
        committedState = finalState;
      }
      return clone({
        status: madeProgress ? "advanced" : "idle",
        command,
        fromVirtualTime,
        virtualTime: finalState.virtualTime,
        virtualElapsedMs: finalState.virtualElapsedMs,
        batches,
        state: finalState,
      });
    } finally {
      commandInFlight = undefined;
    }
  };

  return {
    start,
    submit,
    advanceBy: (durationMs) => runAdvance("advanceBy", durationMs),
    advanceToNext: () => runAdvance("advanceToNext"),
    state: () => clone(committedState),
    status,
  };
}
