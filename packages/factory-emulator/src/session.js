import { parseEmulatorScenario } from "./parser.js";
import {
  canonicalStringify,
  deriveFactoryEmulatorIdentity,
} from "./identity.js";
import { calculateNextSchedulerBatch } from "./scheduler.js";
import { selectEmulatorRule } from "./semantics.js";
import {
  FactoryEmulatorDurationError,
  stateAtElapsed,
  validateDuration,
  virtualTimeAt,
} from "./virtual-time.js";

export { FactoryEmulatorDurationError } from "./virtual-time.js";

const EVENT_SCHEMA_VERSION = "agent-factory.event.v1";
const WORK_REQUEST_TYPE = "FACTORY_REQUEST_BATCH";

export class FactoryEmulatorConfigurationError extends Error {
  constructor(diagnostics) {
    super("Factory emulator configuration is not valid or supported");
    this.name = "FactoryEmulatorConfigurationError";
    this.code = "INVALID_CONFIGURATION";
    this.diagnostics = copy(diagnostics);
  }
}

export class FactoryEmulatorLifecycleError extends Error {
  constructor(command, phase) {
    super(`Factory emulator cannot ${command} while lifecycle phase is ${phase}`);
    this.name = "FactoryEmulatorLifecycleError";
    this.code = "INVALID_LIFECYCLE";
    this.command = command;
    this.phase = phase;
  }
}

export class FactoryEmulatorSubmissionError extends Error {
  constructor(diagnostics) {
    super("Factory emulator submission is not valid");
    this.name = "FactoryEmulatorSubmissionError";
    this.code = "INVALID_SUBMISSION";
    this.diagnostics = copy(diagnostics);
  }
}

/**
 * Creates one caller-owned deterministic Factory emulator session.
 *
 * Validation and event calculation happen before the first sink write. The
 * immutable inputs are detached at construction, and all runtime state is
 * local to the returned session.
 */
export function createFactoryEmulatorSession({ factory, scenario, sink }) {
  if (!sink || typeof sink.write !== "function") {
    throw new TypeError("sink must provide a write function");
  }

  const configuredFactory = copy(factory);
  const configuredScenario = copy(scenario);
  let committedState = preStartState();
  let commandInProgress;

  async function runAdvance(command, targetElapsedMs) {
    const fromVirtualTime = committedState.virtualTime;
    if (!hasUnfinishedWork(committedState)) {
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

    const batches = [];
    let madeProgress = false;
    commandInProgress = "advancing";
    try {
      while (true) {
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
        if (calculation.batch !== undefined) {
          await sink.write(copy(calculation.batch));
          batches.push(calculation.batch);
        }
        committedState = calculation.state;
        madeProgress = true;
        if (command === "advanceToNext" && calculation.batch !== undefined) {
          break;
        }
      }

      if (
        targetElapsedMs !== undefined &&
        committedState.virtualElapsedMs !== targetElapsedMs
      ) {
        committedState = stateAtElapsed(configuredScenario, committedState, targetElapsedMs);
      }
      if (madeProgress || committedState.virtualTime !== fromVirtualTime) {
        committedState = {
          ...committedState,
          counters: {
            ...committedState.counters,
            commands: committedState.counters.commands + 1,
          },
        };
      }
      return copy({
        status: !madeProgress && batches.length === 0 && committedState.virtualTime === fromVirtualTime
          ? "idle"
          : "advanced",
        command,
        fromVirtualTime,
        virtualTime: committedState.virtualTime,
        virtualElapsedMs: committedState.virtualElapsedMs,
        batches,
        state: committedState,
      });
    } finally {
      commandInProgress = undefined;
    }
  }

  return Object.freeze({
    async start() {
      const phase = commandInProgress ?? committedState.lifecycle;
      if (phase !== "pre-start") {
        throw new FactoryEmulatorLifecycleError("start", phase);
      }

      const parsed = parseEmulatorScenario(configuredScenario, configuredFactory);
      if (!parsed.success) {
        throw new FactoryEmulatorConfigurationError(parsed.diagnostics);
      }

      const calculation = calculateStart(parsed.factory, parsed.scenario);
      commandInProgress = "starting";
      try {
        for (const batch of calculation.batches) {
          await sink.write(copy(batch));
        }
        committedState = calculation.state;
        return copy({
          status: "started",
          batches: calculation.batches,
          state: committedState,
        });
      } finally {
        commandInProgress = undefined;
      }
    },
    async submit(submissionOrBatch) {
      const phase = commandInProgress ?? committedState.lifecycle;
      if (phase !== "started") {
        throw new FactoryEmulatorLifecycleError("submit", phase);
      }

      const submissions = normalizeSubmissions(submissionOrBatch);
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
      commandInProgress = "submitting";
      try {
        await sink.write(copy(calculation.batch));
        committedState = calculation.state;
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
      const phase = commandInProgress ?? committedState.lifecycle;
      if (phase !== "started") {
        throw new FactoryEmulatorLifecycleError("advanceBy", phase);
      }
      validateDuration(durationMs);
      const targetElapsedMs = committedState.virtualElapsedMs + durationMs;
      if (!Number.isSafeInteger(targetElapsedMs)) {
        throw new FactoryEmulatorDurationError(durationMs);
      }
      virtualTimeAt(configuredScenario, targetElapsedMs);
      return runAdvance("advanceBy", targetElapsedMs);
    },
    async advanceToNext() {
      const phase = commandInProgress ?? committedState.lifecycle;
      if (phase !== "started") {
        throw new FactoryEmulatorLifecycleError("advanceToNext", phase);
      }
      return runAdvance("advanceToNext");
    },
    reset() {
      const phase = commandInProgress ?? committedState.lifecycle;
      if (phase !== "started") {
        throw new FactoryEmulatorLifecycleError("reset", phase);
      }
      committedState = preStartState();
      return copy({ status: "reset", state: committedState });
    },
    state() {
      return copy(committedState);
    },
    status() {
      return copy(sessionStatus(committedState, commandInProgress));
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
  const initialRequest = initialSubmissions.length === 0 ? undefined : calculateWorkRequest({
    command: "start",
    commandSequence: 0,
    eventSequence: bootstrapEvents.length,
    eventTime: normalizedStartAt,
    identityCoordinates,
    scenario,
    sessionId,
    submissions: initialSubmissions,
  });
  const works = initialRequest?.works ?? [];

  const batches = [{ events: bootstrapEvents }];
  if (initialRequest) {
    batches.push({ events: [initialRequest.event] });
  }

  return copy({
    batches,
    state: {
      lifecycle: "started",
      sessionId,
      virtualTime: normalizedStartAt,
      virtualElapsedMs: 0,
      works,
      ruleCursors: {},
      counters: {
        commands: 1,
        events: bootstrapEvents.length + (works.length > 0 ? 1 : 0),
        requests: works.length > 0 ? 1 : 0,
        works: works.length,
        dispatches: 0,
        completions: 0,
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
    return {
      submissionId: submission.id,
      requestId,
      traceId,
      workId: deriveFactoryEmulatorIdentity("work", workCoordinates),
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
  let submissions;
  try {
    submissions = copy(Array.isArray(submissionOrBatch)
      ? submissionOrBatch
      : [submissionOrBatch]);
  } catch {
    throw new FactoryEmulatorSubmissionError([{
      code: "INVALID_SCENARIO",
      path: "/submissions",
      message: "must contain structured-cloneable data values only",
    }]);
  }
  if (submissions.length === 0) {
    throw new FactoryEmulatorSubmissionError([{
      code: "INVALID_SCENARIO",
      path: "/submissions",
      message: "must contain at least one Work request",
    }]);
  }
  return submissions;
}

function sessionStatus(state, commandInProgress) {
  if (state.error !== undefined) {
    return { phase: "error", reason: state.error.code ?? "runtime-error" };
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
