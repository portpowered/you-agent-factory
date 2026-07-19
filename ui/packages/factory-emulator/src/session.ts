import type {
  FactoryDefinition,
  FactoryEvent,
} from "@you-agent-factory/client";
import { inspectFactoryEmulatorCompatibility } from "./compatibility.js";
import type { FactoryEventSink } from "./event-sink.js";
import { safeParseFactoryEmulatorScenario } from "./scenario.js";
import type { FactoryEmulatorScenario } from "./scenario-contracts.js";
import {
  DEFAULT_FACTORY_EMULATOR_LIMITS,
  FACTORY_EMULATOR_LIMIT_HARD_CAPS,
  type FactoryEmulatorBudgetUsage,
  type FactoryEmulatorConfigurationDiagnostic,
  FactoryEmulatorConfigurationError,
  FactoryEmulatorLifecycleError,
  type FactoryEmulatorLimits,
  FactoryEmulatorPendingCommandError,
  type FactoryEmulatorSession,
  type FactoryEmulatorSessionError,
  type FactoryEmulatorSessionOptions,
  type FactoryEmulatorSessionState,
  type FactoryEmulatorSessionStatus,
  type FactoryEmulatorStartReceipt,
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

interface ValidatedConfiguration {
  readonly factory: FactoryDefinition;
  readonly scenario: FactoryEmulatorScenario;
  readonly sink: FactoryEventSink;
  readonly limits: ResolvedFactoryEmulatorLimits;
  readonly yieldControl?: () => void | PromiseLike<void>;
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

function sessionIdentity(
  factory: FactoryDefinition,
  scenario: FactoryEmulatorScenario,
): string {
  return `emulator-session-${deterministicHash(
    [factory.id ?? factory.name, scenario.id, scenario.seed].join("\u0000"),
  )}`;
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
      payload: {
        factoryId: factory.id ?? factory.name,
        startedAt: eventTime,
      },
    },
  ];
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

/** Creates one validated, caller-owned deterministic Factory emulator session. */
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
  let commandInFlight = false;
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
        reason: "The event sink rejected the pending start transaction.",
        pendingTransaction: {
          command: "start",
          phase: "sink-write",
          eventCount: pendingBatch?.length ?? 0,
        },
        error: lastError,
      });
    }
    if (commandInFlight) {
      return clone({
        ...common,
        phase: "active",
        reason: "The start transaction is awaiting the event sink.",
        pendingTransaction: {
          command: "start",
          phase: "sink-write",
          eventCount: pendingBatch?.length ?? 0,
        },
      });
    }
    if (committedState.lifecycle === "pre-start") {
      return clone({
        ...common,
        phase: "idle",
        reason: "The session is ready to start.",
      });
    }
    const hasInitialWork =
      (configuration.scenario.initialSubmissions?.length ?? 0) > 0;
    return clone({
      ...common,
      phase: hasInitialWork ? "ready" : "idle",
      reason: hasInitialWork
        ? "Initial Work is ready for a later execution command."
        : "The open session has no unfinished Work.",
    });
  };

  const start = async (): Promise<FactoryEmulatorStartReceipt> => {
    if (commandInFlight) {
      throw new FactoryEmulatorPendingCommandError("start", "start");
    }
    if (committedState.lifecycle !== "pre-start") {
      throw new FactoryEmulatorLifecycleError(
        "start",
        committedState.lifecycle,
      );
    }
    pendingBatch ??= bootstrapEvents(
      configuration.factory,
      configuration.scenario,
      sessionId,
    );
    commandInFlight = true;
    lastError = undefined;
    try {
      await configuration.sink.write(clone(pendingBatch));
      const batch = pendingBatch;
      committedState = {
        lifecycle: "started",
        sessionId,
        virtualTime: configuration.scenario.startAt,
        virtualElapsedMs: 0,
        counters: {
          commands: 1,
          events: batch.length,
          completedDispatches: 0,
        },
      };
      pendingBatch = undefined;
      return clone({
        status: "started",
        batches: [batch],
        state: committedState,
      });
    } catch (rejection) {
      lastError = {
        code: "sink_write_rejected",
        operation: "write",
        command: "start",
        message: rejectionMessage(rejection),
      };
      throw rejection;
    } finally {
      commandInFlight = false;
    }
  };

  return {
    start,
    state: () => clone(committedState),
    status,
  };
}
