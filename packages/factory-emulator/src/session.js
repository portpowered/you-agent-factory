import { parseEmulatorScenario } from "./parser.js";
import {
  canonicalStringify,
  deriveFactoryEmulatorIdentity,
} from "./identity.js";
import {
  calculateNextSchedulerBatch,
  inspectNextSchedulerBatch,
} from "./scheduler.js";
import {
  DEFAULT_FACTORY_EMULATOR_LIMITS,
  FACTORY_EMULATOR_LIMIT_HARD_CAPS,
  budgetExceededDiagnostic,
  normalizeFactoryEmulatorLimits,
  synchronousWorkLimitDiagnostic,
  zeroDurationCycleDiagnostic,
} from "./limits.js";
import { selectEmulatorRule } from "./semantics.js";
import {
  FactoryEmulatorDurationError,
  stateAtElapsed,
  validateDuration,
  virtualTimeAt,
} from "./virtual-time.js";
import { defineDataError } from "./data-error.js";
import { dataOnlyDiagnostics } from "./data-only.js";

export { FactoryEmulatorDurationError } from "./virtual-time.js";
export {
  DEFAULT_FACTORY_EMULATOR_LIMITS,
  FACTORY_EMULATOR_LIMIT_HARD_CAPS,
} from "./limits.js";

const EVENT_SCHEMA_VERSION = "agent-factory.event.v1";
const WORK_REQUEST_TYPE = "FACTORY_REQUEST_BATCH";
const UNPRINTABLE_SINK_ERROR_MESSAGE = "Sink rejected with an unprintable value";

export const FactoryEmulatorConfigurationError = defineDataError(
  "FactoryEmulatorConfigurationError",
  "INVALID_CONFIGURATION",
  {
    message: () => "Factory emulator configuration is not valid or supported",
    details: (diagnostics) => ({ diagnostics: copy(diagnostics) }),
  },
);

export const FactoryEmulatorLifecycleError = defineDataError(
  "FactoryEmulatorLifecycleError",
  "INVALID_LIFECYCLE",
  {
    message: (command, phase) =>
      `Factory emulator cannot ${command} while lifecycle phase is ${phase}`,
    details: (command, phase) => ({ command, phase }),
  },
);

export const FactoryEmulatorSubmissionError = defineDataError(
  "FactoryEmulatorSubmissionError",
  "INVALID_SUBMISSION",
  {
    message: () => "Factory emulator submission is not valid",
    details: (diagnostics) => ({ diagnostics: copy(diagnostics) }),
  },
);

export const FactoryEmulatorPendingCommandError = defineDataError(
  "FactoryEmulatorPendingCommandError",
  "PENDING_TRANSACTION",
  {
    message: (attemptedCommand, pendingCommand) =>
      `Factory emulator cannot ${attemptedCommand} while ${pendingCommand} has a rejected transaction`,
    details: (attemptedCommand, pendingCommand) => ({ attemptedCommand, pendingCommand }),
  },
);

export const FactoryEmulatorExecutionPausedError = defineDataError(
  "FactoryEmulatorExecutionPausedError",
  "EXECUTION_PAUSED",
  {
    message: (diagnostic) => `Factory emulator execution paused: ${diagnostic.kind}`,
    details: (diagnostic) => ({ diagnostic: copy(diagnostic) }),
  },
);

/**
 * Creates one caller-owned deterministic Factory emulator session.
 *
 * Validation and event calculation happen before the first sink write. The
 * immutable inputs are detached at construction, and all runtime state is
 * local to the returned session.
 */
export function createFactoryEmulatorSession({
  factory,
  scenario,
  sink,
  limits,
  yieldControl = defaultYieldControl,
}) {
  if (!sink || typeof sink.write !== "function" || typeof sink.close !== "function") {
    throw new TypeError("sink must provide write and close functions");
  }
  if (typeof yieldControl !== "function") {
    throw new TypeError("yieldControl must be a function");
  }

  const configurationDiagnostics = [
    ...dataOnlyDiagnostics(scenario, { code: "INVALID_SCENARIO_SHAPE" }),
    ...dataOnlyDiagnostics(factory, { code: "INVALID_FACTORY_DEFINITION" }),
  ];
  if (configurationDiagnostics.length > 0) {
    throw new FactoryEmulatorConfigurationError(configurationDiagnostics);
  }
  const configuredFactory = copy(factory);
  const configuredScenario = copy(scenario);
  const normalizedLimits = normalizeFactoryEmulatorLimits(limits);
  if (!normalizedLimits.success) {
    throw new FactoryEmulatorConfigurationError(normalizedLimits.diagnostics);
  }
  const configuredLimits = normalizedLimits.limits;
  let committedState = preStartState();
  let commandInProgress;
  let pendingTransaction;
  let runtimeError;

  function assertCommandAvailable(command) {
    if (commandInProgress !== undefined) {
      throw new FactoryEmulatorLifecycleError(command, commandInProgress);
    }
    if (pendingTransaction !== undefined && pendingTransaction.command !== command) {
      throw new FactoryEmulatorPendingCommandError(command, pendingTransaction.command);
    }
    if (runtimeError?.code === "EXECUTION_PAUSED" && command !== "close") {
      throw new FactoryEmulatorExecutionPausedError(runtimeError.diagnostic);
    }
    if (
      pendingTransaction === undefined &&
      committedState.lifecycle !== (command === "start" ? "pre-start" : "started")
    ) {
      throw new FactoryEmulatorLifecycleError(command, committedState.lifecycle);
    }
  }

  function beginCommand(command, key, phase) {
    assertCommandAvailable(command);
    if (pendingTransaction !== undefined) {
      if (pendingTransaction.command !== command || pendingTransaction.key !== key) {
        throw new FactoryEmulatorPendingCommandError(command, pendingTransaction.command);
      }
      commandInProgress = phase;
      return pendingTransaction;
    }
    if (committedState.lifecycle !== (command === "start" ? "pre-start" : "started")) {
      throw new FactoryEmulatorLifecycleError(command, committedState.lifecycle);
    }
    commandInProgress = phase;
    return undefined;
  }

  async function writeCalculation(command, key, calculation, progress = {}) {
    assertCalculationWithinBudgets(command, calculation);
    try {
      const receipt = await sink.write(copy(calculation.batch));
      if (receipt?.status !== "accepted") {
        throw new TypeError("sink.write must return an accepted receipt");
      }
      committedState = copy(calculation.state);
      pendingTransaction = undefined;
      runtimeError = undefined;
    } catch (error) {
      pendingTransaction = copy({
        command,
        key,
        batch: calculation.batch,
        state: calculation.state,
        progress,
      });
      runtimeError = sinkError("SINK_WRITE_REJECTED", "write", command, error);
      throw error;
    }
  }

  function assertCalculationWithinBudgets(command, calculation) {
    // The terminal lifecycle event is always available so a paused session can
    // release its sink. All preceding canonical events still count as usage.
    if (command === "close") {
      return;
    }
    const checks = [
      {
        configured: configuredLimits.maxCompletedDispatches,
        limit: "completedDispatches",
        observed: calculation.state.counters.completions,
      },
      {
        configured: configuredLimits.maxEvents,
        limit: "events",
        observed: calculation.state.counters.events,
      },
      {
        configured: configuredLimits.maxVirtualElapsedMs,
        limit: "virtualElapsedMs",
        observed: calculation.state.virtualElapsedMs,
      },
    ];
    const exceeded = checks.find((check) => check.observed > check.configured);
    if (exceeded !== undefined) {
      pauseExecution(budgetExceededDiagnostic({
        ...exceeded,
        virtualTime: committedState.virtualTime,
        virtualElapsedMs: committedState.virtualElapsedMs,
      }));
    }
  }

  function pauseExecution(diagnostic) {
    runtimeError = {
      code: "EXECUTION_PAUSED",
      diagnostic: copy(diagnostic),
    };
    throw new FactoryEmulatorExecutionPausedError(diagnostic);
  }

  function assertSynchronousWorkWithinLimit(observed) {
    if (observed > configuredLimits.maxSynchronousWorkItems) {
      pauseExecution(synchronousWorkLimitDiagnostic({
        configured: configuredLimits.maxSynchronousWorkItems,
        observed,
        virtualTime: committedState.virtualTime,
        virtualElapsedMs: committedState.virtualElapsedMs,
      }));
    }
  }

  async function inspectSchedulerBatchWithinLimit() {
    let continuation;
    while (true) {
      const inspection = inspectNextSchedulerBatch({
        continuation,
        maximumWorkItems: configuredLimits.maxSynchronousWorkItems,
        state: committedState,
      });
      if (inspection.done) {
        if (inspection.batch !== undefined) {
          assertSynchronousWorkWithinLimit(inspection.batch.workItems);
        }
        return;
      }
      continuation = inspection.continuation;
      await yieldControl();
    }
  }

  function finishAdvance(command, fromVirtualTime, batches, madeProgress) {
    if (madeProgress || committedState.virtualTime !== fromVirtualTime) {
      committedState = {
        ...committedState,
        counters: {
          ...committedState.counters,
          commands: committedState.counters.commands + 1,
        },
      };
    }
    return advanceReceipt(command, fromVirtualTime, batches, madeProgress, committedState);
  }

  async function runAdvance(command, key, targetElapsedMs, retry) {
    const progress = retry?.progress ?? {
      fromVirtualTime: committedState.virtualTime,
      targetElapsedMs,
      batches: [],
    };
    const fromVirtualTime = progress.fromVirtualTime;
    targetElapsedMs = progress.targetElapsedMs;
    const batches = copy(progress.batches);
    let zeroDurationBatches = progress.zeroDurationBatches ?? 0;
    let synchronousBatches = 0;
    let madeProgress = batches.length > 0;

    if (retry !== undefined) {
      const beforeRetryElapsedMs = committedState.virtualElapsedMs;
      await writeCalculation(command, retry.key, retry, progress);
      batches.push(copy(retry.batch));
      zeroDurationBatches = committedState.virtualElapsedMs === beforeRetryElapsedMs
        ? zeroDurationBatches + 1
        : 0;
      madeProgress = true;
      if (command === "advanceToNext") {
        return finishAdvance(command, fromVirtualTime, batches, true);
      }
    }

    if (!hasUnfinishedWork(committedState) && batches.length === 0) {
      return copy({
        status: "idle",
        command,
        fromVirtualTime,
        virtualTime: committedState.virtualTime,
        virtualElapsedMs: committedState.virtualElapsedMs,
        batches: [],
        state: committedState,
      });
    }

    while (true) {
      await inspectSchedulerBatchWithinLimit();
      const calculation = calculateNextSchedulerBatch({
        createEvent,
        identityCoordinates: sessionIdentityCoordinates(
          configuredFactory,
          configuredScenario,
          committedState.sessionId,
        ),
        scenario: configuredScenario,
        state: committedState,
      });
      if (
        calculation === undefined ||
        (targetElapsedMs !== undefined && calculation.elapsedMs > targetElapsedMs)
      ) {
        break;
      }
      const nextZeroDurationBatches = calculation.elapsedMs === committedState.virtualElapsedMs
        ? zeroDurationBatches + 1
        : 0;
      if (nextZeroDurationBatches > configuredLimits.maxZeroDurationBatches) {
        pauseExecution(zeroDurationCycleDiagnostic({
          configured: configuredLimits.maxZeroDurationBatches,
          observed: nextZeroDurationBatches,
          virtualTime: committedState.virtualTime,
          virtualElapsedMs: committedState.virtualElapsedMs,
        }));
      }
      if (calculation.batch !== undefined) {
        await writeCalculation(command, key, calculation, {
          fromVirtualTime,
          targetElapsedMs,
          batches,
          zeroDurationBatches,
        });
        batches.push(copy(calculation.batch));
      } else {
        committedState = copy(calculation.state);
      }
      zeroDurationBatches = nextZeroDurationBatches;
      madeProgress = true;
      if (command === "advanceToNext" && calculation.batch !== undefined) {
        break;
      }
      synchronousBatches += 1;
      if (synchronousBatches >= configuredLimits.maxSynchronousBatches) {
        synchronousBatches = 0;
        await yieldControl();
      }
    }

    if (
      targetElapsedMs !== undefined &&
      committedState.virtualElapsedMs !== targetElapsedMs
    ) {
      const targetState = stateAtElapsed(configuredScenario, committedState, targetElapsedMs);
      assertCalculationWithinBudgets(command, { state: targetState });
      committedState = targetState;
    }
    return finishAdvance(command, fromVirtualTime, batches, madeProgress);
  }

  return Object.freeze({
    async start() {
      const retry = beginCommand("start", "start", "starting");
      try {
        let batches;
        if (retry !== undefined) {
          batches = copy(retry.progress.batches);
          await writeCalculation("start", "start", retry, retry.progress);
          batches.push(copy(retry.batch));
        } else {
          const parsed = parseEmulatorScenario(configuredScenario, configuredFactory);
          if (!parsed.success) {
            throw new FactoryEmulatorConfigurationError(parsed.diagnostics);
          }
          const transaction = calculateStart(parsed.factory, parsed.scenario);
          batches = [];
          await writeCalculation("start", "start", transaction, {
            batches,
            stage: "bootstrap",
          });
          batches.push(copy(transaction.batch));
        }

        if (committedState.counters.commands === 0) {
          assertSynchronousWorkWithinLimit(
            configuredScenario.initialSubmissions?.length ?? 0,
          );
          const initial = calculateInitialSubmission(
            configuredFactory,
            configuredScenario,
            committedState,
          );
          await writeCalculation("start", "start", initial, {
            batches,
            stage: "initial-submission",
          });
          batches.push(copy(initial.batch));
        }
        return copy({
          status: "started",
          batches,
          state: committedState,
        });
      } finally {
        commandInProgress = undefined;
      }
    },
    async submit(submissionOrBatch) {
      assertCommandAvailable("submit");
      const submissions = normalizeSubmissions(submissionOrBatch);
      assertSynchronousWorkWithinLimit(submissions.length);
      const key = commandKey("submit", canonicalStringify(submissions));
      const retry = beginCommand("submit", key, "submitting");
      try {
        if (retry !== undefined) {
          await writeCalculation("submit", key, retry);
          return copy({ status: "submitted", batch: retry.batch, state: committedState });
        }
        const parsed = parseEmulatorScenario(
          {
            ...configuredScenario,
            initialSubmissions: submissions,
            rules: [],
            unmatchedBehavior: { kind: "ignore" },
          },
          configuredFactory,
        );
        if (!parsed.success) {
          throw new FactoryEmulatorSubmissionError(
            parsed.diagnostics.map((diagnostic) => ({
              ...diagnostic,
              path: diagnostic.path.replace(/^\/initialSubmissions/, "/submissions"),
            })),
          );
        }

        const calculation = calculateSubmit(
          parsed.factory,
          configuredScenario,
          committedState,
          submissions,
        );
        await writeCalculation("submit", key, calculation);
        return copy({
          status: "submitted",
          batch: calculation.batch,
          state: committedState,
        });
      } finally {
        commandInProgress = undefined;
      }
    },
    async advanceBy(durationMs) {
      assertCommandAvailable("advanceBy");
      validateDuration(durationMs);
      const key = commandKey("advanceBy", durationMs);
      const retry = beginCommand("advanceBy", key, "advancing");
      const targetElapsedMs = retry?.progress.targetElapsedMs
        ?? committedState.virtualElapsedMs + durationMs;
      if (!Number.isSafeInteger(targetElapsedMs)) {
        commandInProgress = undefined;
        throw new FactoryEmulatorDurationError(durationMs);
      }
      try {
        virtualTimeAt(configuredScenario, targetElapsedMs);
        return await runAdvance("advanceBy", key, targetElapsedMs, retry);
      } finally {
        commandInProgress = undefined;
      }
    },
    async advanceToNext() {
      const retry = beginCommand("advanceToNext", "advanceToNext", "advancing");
      try {
        return await runAdvance("advanceToNext", "advanceToNext", undefined, retry);
      } finally {
        commandInProgress = undefined;
      }
    },
    async close() {
      const retry = beginCommand("close", "close", "closing");
      let terminal;
      try {
        terminal = retry ?? calculateClose(committedState, configuredFactory, configuredScenario);
        if (retry?.progress.terminalAccepted !== true) {
          await writeCalculation("close", "close", terminal, { terminalAccepted: false });
        }
        try {
          const receipt = await sink.close();
          if (receipt?.status !== "closed") {
            throw new TypeError("sink.close must return a closed receipt");
          }
          pendingTransaction = undefined;
          runtimeError = undefined;
          return copy({ status: "closed", batch: terminal.batch, state: committedState });
        } catch (error) {
          pendingTransaction = copy({
            command: "close",
            key: "close",
            batch: terminal.batch,
            state: committedState,
            progress: { terminalAccepted: true },
          });
          runtimeError = sinkError("SINK_CLOSE_REJECTED", "close", "close", error);
          throw error;
        }
      } finally {
        commandInProgress = undefined;
      }
    },
    reset() {
      const phase = commandInProgress ?? committedState.lifecycle;
      if (
        commandInProgress !== undefined ||
        committedState.lifecycle === "closed" ||
        (pendingTransaction === undefined &&
          committedState.lifecycle !== "started" &&
          runtimeError?.code !== "EXECUTION_PAUSED")
      ) {
        throw new FactoryEmulatorLifecycleError("reset", phase);
      }
      committedState = preStartState();
      pendingTransaction = undefined;
      runtimeError = undefined;
      return copy({ status: "reset", state: committedState });
    },
    pending() {
      return pendingTransaction === undefined ? undefined : copy(pendingTransaction.batch);
    },
    state() {
      return copy(committedState);
    },
    status() {
      return copy(sessionStatus({
        configuredLimits,
        commandInProgress,
        pendingTransaction,
        runtimeError,
        state: committedState,
      }));
    },
  });
}

function hasUnfinishedWork(state) {
  return state.works.some((work) => work.phase !== "completed");
}

function calculateSubmit(factory, scenario, state, submissions) {
  const identityCoordinates = sessionIdentityCoordinates(
    factory,
    scenario,
    state.sessionId,
  );
  const commandSequence = state.counters.commands;
  const { event, works } = calculateWorkRequest({
    command: "submit",
    commandSequence,
    eventSequence: state.counters.events,
    eventTime: state.virtualTime,
    identityCoordinates,
    scenario,
    sessionId: state.sessionId,
    submissions,
  });
  return copy({
    batch: { events: [event] },
    state: {
      ...state,
      works: [...state.works, ...works],
      counters: {
        ...state.counters,
        commands: state.counters.commands + 1,
        events: state.counters.events + 1,
        requests: state.counters.requests + 1,
        works: state.counters.works + works.length,
      },
    },
  });
}

function calculateStart(factory, scenario) {
  const normalizedStartAt = normalizeStartAt(scenario.startAt);
  const factoryIdentity = canonicalStringify(factory);
  const sessionId = deriveFactoryEmulatorIdentity("session", {
    factory: factoryIdentity,
    scenarioId: scenario.id,
    scenarioVersion: scenario.version,
    seed: scenario.seed,
    startAt: normalizedStartAt,
  });
  const identityCoordinates = sessionIdentityCoordinates(
    factory,
    scenario,
    sessionId,
  );

  const bootstrapEvents = [
    createEvent({
      identityCoordinates,
      sequence: 0,
      sessionId,
      eventTime: normalizedStartAt,
      type: "INITIAL_STRUCTURE_REQUEST",
      payload: {
        factory,
        metadata: { emulatorScenarioId: scenario.id },
      },
    }),
    createEvent({
      identityCoordinates,
      sequence: 1,
      sessionId,
      eventTime: normalizedStartAt,
      type: "RUN_REQUEST",
      payload: { recordedAt: normalizedStartAt, factory },
    }),
  ];

  const initialSubmissions = scenario.initialSubmissions ?? [];
  const bootstrapState = {
    lifecycle: "started",
    sessionId,
    virtualTime: normalizedStartAt,
    virtualElapsedMs: 0,
    works: [],
    ruleCursors: {},
    counters: {
      commands: initialSubmissions.length === 0 ? 1 : 0,
      events: bootstrapEvents.length,
      requests: 0,
      works: 0,
      dispatches: 0,
      completions: 0,
    },
  };
  return copy({
    batch: { events: bootstrapEvents },
    state: bootstrapState,
  });
}

function calculateInitialSubmission(factory, scenario, state) {
  const submissions = scenario.initialSubmissions ?? [];
  const { event, works } = calculateWorkRequest({
    command: "start",
    commandSequence: 0,
    eventSequence: state.counters.events,
    eventTime: state.virtualTime,
    identityCoordinates: sessionIdentityCoordinates(factory, scenario, state.sessionId),
    scenario,
    sessionId: state.sessionId,
    submissions,
  });
  return copy({
    batch: { events: [event] },
    state: {
      ...state,
      works,
      counters: {
        ...state.counters,
        commands: 1,
        events: state.counters.events + 1,
        requests: 1,
        works: works.length,
      },
    },
  });
}

function calculateWorkRequest({
  command,
  commandSequence,
  eventSequence,
  eventTime,
  identityCoordinates,
  scenario,
  sessionId,
  submissions,
}) {
  const requestId = deriveFactoryEmulatorIdentity("request", {
    ...identityCoordinates,
    command,
    sequence: commandSequence,
  });
  const works = submissions.map((submission, ordinal) => {
    const workCoordinates = {
      ...identityCoordinates,
      command,
      ...(command === "start" ? {} : { sequence: commandSequence }),
      ordinal,
      submissionId: submission.id,
      workType: submission.workType,
    };
    const traceId = deriveFactoryEmulatorIdentity("trace", workCoordinates);
    const tokenId = deriveFactoryEmulatorIdentity("token", {
      ...workCoordinates,
      lineage: { kind: "submission", submissionId: submission.id },
    });
    return {
      submissionId: submission.id,
      requestId,
      traceId,
      workId: deriveFactoryEmulatorIdentity("work", workCoordinates),
      tokenId,
      workType: submission.workType,
      phase: submissionPhase(scenario, submission),
      ...(submission.input === undefined ? {} : { input: submission.input }),
    };
  });
  return {
    works,
    event: createEvent({
      identityCoordinates,
      sequence: eventSequence,
      sessionId,
      eventTime,
      type: "WORK_REQUEST",
      context: {
        requestId,
        traceIds: works.map((work) => work.traceId),
        workIds: works.map((work) => work.workId),
      },
      payload: {
        type: WORK_REQUEST_TYPE,
        source: "emulator",
        works: works.map((work) => ({
          name: work.submissionId,
          workId: work.workId,
          requestId: work.requestId,
          workTypeName: work.workType,
          currentChainingTraceId: work.traceId,
          traceId: work.traceId,
          tags: { emulatorTokenId: work.tokenId },
          ...(work.input === undefined ? {} : { payload: work.input }),
        })),
      },
    }),
  };
}

function submissionPhase(scenario, submission) {
  if (selectEmulatorRule(scenario, submission) !== undefined) {
    return "ready";
  }
  return scenario.unmatchedBehavior.kind === "ignore" ? "waiting" : "ready";
}

function sessionIdentityCoordinates(factory, scenario, sessionId) {
  return {
    factory: canonicalStringify(factory),
    scenarioId: scenario.id,
    seed: scenario.seed,
    sessionId,
  };
}

function normalizeSubmissions(submissionOrBatch) {
  const submissionValues = Array.isArray(submissionOrBatch)
    ? submissionOrBatch
    : [submissionOrBatch];
  const diagnostics = dataOnlyDiagnostics(submissionValues, {
    code: "INVALID_SCENARIO_SHAPE",
    rootPath: "/submissions",
  });
  if (diagnostics.length > 0) {
    throw new FactoryEmulatorSubmissionError(diagnostics);
  }
  let submissions;
  try {
    submissions = copy(submissionValues);
  } catch {
    throw new FactoryEmulatorSubmissionError([{
      code: "INVALID_SCENARIO_SHAPE",
      path: "/submissions",
      message: "must contain structured-cloneable data values only",
      expectation: "plain objects, dense arrays, null, booleans, strings, or finite numbers",
    }]);
  }
  if (submissions.length === 0) {
    throw new FactoryEmulatorSubmissionError([{
      code: "INVALID_SCENARIO_SHAPE",
      path: "/submissions",
      message: "must contain at least one Work request",
      expectation: "at least one Work request",
    }]);
  }
  return submissions;
}

function sessionStatus({
  configuredLimits,
  commandInProgress,
  pendingTransaction,
  runtimeError,
  state,
}) {
  const status = executionStatus(state, commandInProgress, runtimeError);
  return {
    ...status,
    ...(state.virtualTime === undefined ? {} : { virtualTime: state.virtualTime }),
    virtualElapsedMs: state.virtualElapsedMs,
    budgetUsage: {
      completedDispatches: {
        used: state.counters.completions,
        limit: configuredLimits.maxCompletedDispatches,
      },
      events: {
        used: state.counters.events,
        limit: configuredLimits.maxEvents,
      },
      virtualElapsedMs: {
        used: state.virtualElapsedMs,
        limit: configuredLimits.maxVirtualElapsedMs,
      },
    },
    ...(pendingTransaction === undefined
      ? {}
      : { pendingTransaction: pendingTransactionStatus(pendingTransaction) }),
    ...(runtimeError === undefined ? {} : { error: runtimeError }),
  };
}

function executionStatus(state, commandInProgress, runtimeError) {
  if (runtimeError !== undefined) {
    return runtimeError.code === "EXECUTION_PAUSED"
      ? {
          phase: "error",
          reason: runtimeError.diagnostic.kind,
          diagnostic: runtimeError.diagnostic,
        }
      : { phase: "error", reason: runtimeError.code };
  }
  if (state.lifecycle === "closed") {
    return { phase: "closed", reason: "session-closed" };
  }
  if (commandInProgress !== undefined) {
    return { phase: "active", reason: commandInProgress };
  }
  if (state.lifecycle === "pre-start") {
    return { phase: "idle", reason: "not-started" };
  }
  if (state.works.some((work) => work.phase === "active")) {
    return { phase: "active", reason: "work-active" };
  }
  if (state.works.some((work) => work.phase === undefined || work.phase === "ready")) {
    return { phase: "ready", reason: "work-ready" };
  }
  if (state.works.some((work) => work.phase === "waiting")) {
    return { phase: "waiting", reason: "work-waiting" };
  }
  return { phase: "idle", reason: "no-unfinished-work" };
}

function pendingTransactionStatus(transaction) {
  return {
    command: transaction.command,
    phase: transaction.progress.terminalAccepted === true
      ? "sink-close"
      : "sink-write",
    eventCount: transaction.batch.events.length,
  };
}

function calculateClose(state, factory, scenario) {
  const identityCoordinates = sessionIdentityCoordinates(factory, scenario, state.sessionId);
  const event = createEvent({
    identityCoordinates,
    sequence: state.counters.events,
    tick: state.counters.events,
    sessionId: state.sessionId,
    eventTime: state.virtualTime,
    type: "RUN_RESPONSE",
    payload: {
      state: "COMPLETED",
      reason: "emulator session closed",
    },
  });
  return copy({
    batch: { events: [event] },
    state: {
      ...state,
      lifecycle: "closed",
      counters: {
        ...state.counters,
        commands: state.counters.commands + 1,
        events: state.counters.events + 1,
      },
    },
  });
}

function advanceReceipt(command, fromVirtualTime, batches, madeProgress, state) {
  return copy({
    status: !madeProgress && batches.length === 0 && state.virtualTime === fromVirtualTime
      ? "idle"
      : "advanced",
    command,
    fromVirtualTime,
    virtualTime: state.virtualTime,
    virtualElapsedMs: state.virtualElapsedMs,
    batches,
    state,
  });
}

function commandKey(command, value) {
  return value === undefined ? command : `${command}:${canonicalStringify(value)}`;
}

function sinkError(code, operation, command, error) {
  return {
    code,
    operation,
    command,
    message: sinkErrorMessage(error),
  };
}

function sinkErrorMessage(error) {
  let message = error;
  try {
    if (error instanceof Error) {
      message = error.message;
    }
    return typeof message === "string" ? message : String(message);
  } catch {
    return UNPRINTABLE_SINK_ERROR_MESSAGE;
  }
}

function createEvent({
  identityCoordinates,
  sequence,
  tick = 0,
  sessionId,
  eventTime,
  type,
  context = {},
  payload,
}) {
  return {
    schemaVersion: EVENT_SCHEMA_VERSION,
    id: deriveFactoryEmulatorIdentity("event", {
      ...identityCoordinates,
      sequence,
      type,
      context,
    }),
    type,
    context: {
      sequence,
      tick,
      eventTime,
      sessionId,
      sessionSequence: sequence,
      source: "emulator",
      ...context,
    },
    payload,
  };
}

function preStartState() {
  return {
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
  };
}

function normalizeStartAt(startAt) {
  const instant = new Date(startAt);
  if (!Number.isFinite(instant.getTime())) {
    throw new TypeError("scenario startAt must identify a finite UTC instant");
  }
  return instant.toISOString();
}

function copy(value) {
  return structuredClone(value);
}

function defaultYieldControl() {
  if (typeof MessageChannel === "function") {
    return new Promise((resolve) => {
      const channel = new MessageChannel();
      channel.port1.onmessage = () => {
        channel.port1.close();
        channel.port2.close();
        resolve();
      };
      channel.port2.postMessage(undefined);
    });
  }
  if (typeof setImmediate === "function") {
    return new Promise((resolve) => setImmediate(resolve));
  }
  throw new TypeError("yieldControl is required when the host has no task scheduler");
}
