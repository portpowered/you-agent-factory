import { parseEmulatorScenario } from "./parser.js";
import {
  canonicalStringify,
  deriveFactoryEmulatorIdentity,
} from "./identity.js";

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
  let startInProgress = false;

  return Object.freeze({
    async start() {
      const phase = startInProgress ? "starting" : committedState.lifecycle;
      if (phase !== "pre-start") {
        throw new FactoryEmulatorLifecycleError("start", phase);
      }

      const parsed = parseEmulatorScenario(configuredScenario, configuredFactory);
      if (!parsed.success) {
        throw new FactoryEmulatorConfigurationError(parsed.diagnostics);
      }

      const calculation = calculateStart(parsed.factory, parsed.scenario);
      startInProgress = true;
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
        startInProgress = false;
      }
    },
    reset() {
      const phase = startInProgress ? "starting" : committedState.lifecycle;
      if (phase !== "started") {
        throw new FactoryEmulatorLifecycleError("reset", phase);
      }
      committedState = preStartState();
      return copy({ status: "reset", state: committedState });
    },
    state() {
      return copy(committedState);
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
  const identityCoordinates = {
    factory: factoryIdentity,
    scenarioId: scenario.id,
    seed: scenario.seed,
    sessionId,
  };

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
  const requestId = initialSubmissions.length === 0
    ? undefined
    : deriveFactoryEmulatorIdentity("request", {
      ...identityCoordinates,
      command: "start",
      sequence: 0,
    });
  const works = initialSubmissions.map((submission, ordinal) => {
    const workCoordinates = {
      ...identityCoordinates,
      command: "start",
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
      ...(submission.input === undefined ? {} : { input: submission.input }),
    };
  });

  const batches = [{ events: bootstrapEvents }];
  if (works.length > 0) {
    batches.push({
      events: [createEvent({
        identityCoordinates,
        sequence: bootstrapEvents.length,
        sessionId,
        eventTime: normalizedStartAt,
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
      })],
    });
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

function createEvent({
  identityCoordinates,
  sequence,
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
      tick: 0,
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
