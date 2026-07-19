// biome-ignore-all lint/style/noExcessiveLinesPerFile: The kernel stays cohesive while its transaction model is extended in the next story.
import type {
  FactoryDefinition,
  FactoryEvent,
} from "@you-agent-factory/client";
import { inspectFactoryEmulatorCompatibility } from "./compatibility.js";
import type { FactoryEventSink } from "./event-sink.js";
import {
  visitCountGuardsAllow,
  visitsAfterTransition,
} from "./logical-move.js";
import { safeParseFactoryEmulatorScenario } from "./scenario.js";
import type {
  FactoryEmulatorInitialSubmission,
  FactoryEmulatorOutcome,
  FactoryEmulatorRule,
  FactoryEmulatorScenario,
  FactoryEmulatorScenarioIssue,
} from "./scenario-contracts.js";
import {
  type FactorySchedulerCandidate,
  type FactorySchedulerResourceClaim,
  selectFactorySchedulerCandidates,
} from "./scheduler.js";
import {
  DEFAULT_FACTORY_EMULATOR_LIMITS,
  FACTORY_EMULATOR_LIMIT_HARD_CAPS,
  type FactoryEmulatorAdvanceReceipt,
  type FactoryEmulatorBudgetUsage,
  type FactoryEmulatorCloseReceipt,
  type FactoryEmulatorCommand,
  type FactoryEmulatorConfigurationDiagnostic,
  FactoryEmulatorConfigurationError,
  FactoryEmulatorDurationError,
  type FactoryEmulatorExecutionDiagnostic,
  FactoryEmulatorExecutionPausedError,
  FactoryEmulatorLifecycleError,
  type FactoryEmulatorLimits,
  FactoryEmulatorPendingCommandError,
  type FactoryEmulatorResetReceipt,
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

type ClosedState = Extract<
  FactoryEmulatorSessionState,
  { lifecycle: "closed" }
>;

interface PendingTransaction {
  readonly command: Exclude<FactoryEmulatorCommand, "reset">;
  readonly retryKey: string;
  readonly batch: readonly FactoryEvent[];
  readonly candidate: StartedState | ClosedState;
  phase: "sink-write" | "sink-close";
}

interface PendingAdvanceContext {
  readonly command: "advanceBy" | "advanceToNext";
  readonly retryKey: string;
  readonly fromVirtualTime: string;
  readonly target?: number;
  readonly wasUnfinished: boolean;
  readonly batches: (readonly FactoryEvent[])[];
  zeroDurationBatches: number;
}

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

interface DispatchCandidateValue {
  readonly indexes: readonly number[];
  readonly execution: WorkExecution;
  readonly invocation: number;
  readonly cursorKey?: string;
}

function clone<Value>(value: Value): Value {
  return JSON.parse(JSON.stringify(value)) as Value;
}

function canonicalJson(value: unknown): string {
  return JSON.stringify(value, (_key, nested: unknown) => {
    if (
      nested === null ||
      typeof nested !== "object" ||
      Array.isArray(nested)
    ) {
      return nested;
    }
    return Object.fromEntries(
      Object.entries(nested).sort(([left], [right]) =>
        left < right ? -1 : left > right ? 1 : 0,
      ),
    );
  });
}

function canonicalClone<Value>(value: Value): Value {
  return JSON.parse(canonicalJson(value)) as Value;
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
    factory: canonicalClone(factory),
    scenario: canonicalClone(scenario),
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
  return `emulator-${kind}-${deterministicHash(canonicalJson(coordinates))}`;
}

function sessionIdentity(
  factory: FactoryDefinition,
  scenario: FactoryEmulatorScenario,
): string {
  return identity("session", factory.id ?? factory.name, scenario);
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
  submissions: readonly FactoryEmulatorInitialSubmission[],
  workstation: NonNullable<FactoryDefinition["workstations"]>[number],
): boolean {
  const selector = rule.selector;
  return (
    (selector.workstation === undefined ||
      selector.workstation === workstation.name) &&
    (selector.worker === undefined || selector.worker === workstation.worker) &&
    (selector.input === undefined ||
      submissions.some(
        (submission) =>
          (selector.input?.workType === undefined ||
            selector.input.workType === submission.workType) &&
          (selector.input?.state === undefined ||
            selector.input.state === submission.state) &&
          (selector.input?.name === undefined ||
            selector.input.name === submission.name),
      ))
  );
}

function executionFor(
  configuration: ValidatedConfiguration,
  submissions: readonly FactoryEmulatorInitialSubmission[],
  workstation: NonNullable<FactoryDefinition["workstations"]>[number],
  invocationIndex = 0,
): WorkExecution {
  if (workstation.type === "LOGICAL_MOVE") {
    return {
      workstation,
      outcome: { result: "accepted", durationMs: 0 },
    };
  }
  const rule = configuration.scenario.rules.find((candidate) =>
    ruleMatches(candidate, submissions, workstation),
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

function submissionForWork(
  work: FactoryEmulatorSessionWork,
): FactoryEmulatorInitialSubmission {
  return {
    name: work.submissionId,
    workType: work.workType,
    state: work.state,
    ...(work.input === undefined ? {} : { input: work.input }),
    ...(work.parent === undefined ? {} : { parent: work.parent }),
  };
}

function bindingIndexesFor(
  state: StartedState,
  workstation: NonNullable<FactoryDefinition["workstations"]>[number],
): readonly (readonly number[])[] {
  const inputs = workstation.inputs.map((input) =>
    state.works.flatMap((work, index) =>
      work.phase === "ready" &&
      work.workType === input.workType &&
      work.state === input.state
        ? [index]
        : [],
    ),
  );
  if (inputs.some((matches) => matches.length === 0)) return [];

  const bindings: number[][] = [];
  const append = (slot: number, selected: number[]): void => {
    if (slot === inputs.length) {
      bindings.push(selected);
      return;
    }
    for (const index of inputs[slot] ?? []) {
      if (!selected.includes(index)) append(slot + 1, [...selected, index]);
    }
  };
  append(0, []);

  const distinct = new Map<string, readonly number[]>();
  for (const binding of bindings) {
    const key = [...binding]
      .map((index) => state.works[index]?.tokenId ?? "")
      .sort()
      .join("\u0000");
    if (!distinct.has(key)) distinct.set(key, binding);
  }
  return [...distinct.values()];
}

function schedulerCandidatesFor(
  configuration: ValidatedConfiguration,
  state: StartedState,
  cursors: Readonly<Record<string, number>>,
): {
  readonly candidates: readonly FactorySchedulerCandidate<DispatchCandidateValue>[];
  readonly executableWork: ReadonlySet<number>;
} {
  const candidates: FactorySchedulerCandidate<DispatchCandidateValue>[] = [];
  const executableWork = new Set<number>();
  for (const workstation of configuration.factory.workstations ?? []) {
    const potentiallyBound = state.works.flatMap((work, index) =>
      work.phase === "ready" &&
      workstation.inputs.some(
        (input) =>
          input.workType === work.workType && input.state === work.state,
      )
        ? [index]
        : [],
    );
    const bindings = bindingIndexesFor(state, workstation);
    if (bindings.length === 0) {
      for (const index of potentiallyBound) executableWork.add(index);
      continue;
    }
    for (const indexes of bindings) {
      const works = indexes.flatMap((index) => {
        const work = state.works[index];
        return work === undefined ? [] : [work];
      });
      const primary = works[0];
      if (primary === undefined || works.length !== workstation.inputs.length)
        continue;
      if (
        workstation.type === "LOGICAL_MOVE" &&
        !visitCountGuardsAllow(workstation.guards ?? [], primary.visits)
      ) {
        continue;
      }
      const submissions = works.map(submissionForWork);
      const initial = executionFor(configuration, submissions, workstation);
      const lineage = [
        ...new Set(works.map(({ rootWorkId }) => rootWorkId)),
      ].sort();
      const cursorKey =
        initial.rule === undefined
          ? undefined
          : `${initial.rule.id}:${canonicalJson(lineage)}`;
      const invocation =
        cursorKey === undefined ? 0 : (cursors[cursorKey] ?? 0);
      const execution = executionFor(
        configuration,
        submissions,
        workstation,
        invocation,
      );
      if (execution.outcome === undefined) continue;
      for (const index of indexes) executableWork.add(index);
      candidates.push({
        transitionId: workstation.name,
        workerId: workstation.worker,
        workstationKind:
          workstation.type === "LOGICAL_MOVE" ? "logical" : "normal",
        resources: (workstation.resources ?? []).map(({ name, capacity }) => ({
          name,
          capacity,
        })),
        tokens: works.map((work) => ({
          tokenId: work.tokenId,
          customerWork: true,
          processing:
            configuration.factory.workTypes
              ?.find(({ name }) => name === work.workType)
              ?.states.find(({ name }) => name === work.state)?.type ===
            "PROCESSING",
          queuedElapsedMs: work.queuedElapsedMs,
          ...(work.lastDispatchElapsedMs === undefined
            ? {}
            : { lastDispatchElapsedMs: work.lastDispatchElapsedMs }),
        })),
        value: {
          indexes,
          execution,
          invocation,
          ...(cursorKey === undefined ? {} : { cursorKey }),
        },
      });
    }
  }
  return { candidates, executableWork };
}

function availableResourcesFor(
  configuration: ValidatedConfiguration,
  state: StartedState,
): Readonly<Record<string, number>> {
  const allocated = state.works.reduce<Record<string, number>>(
    (claims, work) => {
      if (work.phase !== "active" || work.dispatch === undefined) return claims;
      for (const resource of work.dispatch.resources) {
        claims[resource.name] =
          (claims[resource.name] ?? 0) + resource.capacity;
      }
      return claims;
    },
    {},
  );
  return Object.fromEntries(
    (configuration.factory.resources ?? []).map((resource) => [
      resource.name,
      Math.max(0, resource.capacity - (allocated[resource.name] ?? 0)),
    ]),
  );
}

function eventResourcesFor(
  configuration: ValidatedConfiguration,
  claims: readonly FactorySchedulerResourceClaim[],
): { name: string; capacity: number }[] {
  const totals = new Map(
    (configuration.factory.resources ?? []).map(({ name, capacity }) => [
      name,
      capacity,
    ]),
  );
  return claims.map(({ name }) => ({ name, capacity: totals.get(name) ?? 0 }));
}

type Workstation = NonNullable<FactoryDefinition["workstations"]>[number];
type WorkstationRoute = Workstation["inputs"][number];

function defaultFailureRoutesFor(
  configuration: ValidatedConfiguration,
  workstation: Workstation,
): readonly WorkstationRoute[] {
  const routes: WorkstationRoute[] = [];
  const seen = new Set<string>();
  for (const input of workstation.inputs) {
    const failedState = configuration.factory.workTypes
      ?.find(({ name }) => name === input.workType)
      ?.states.find(({ type }) => type === "FAILED")?.name;
    if (failedState === undefined) continue;
    const key = `${input.workType}\u0000${failedState}`;
    if (seen.has(key)) continue;
    seen.add(key);
    routes.push({ workType: input.workType, state: failedState });
  }
  return routes;
}

function outcomeRoutesFor(
  configuration: ValidatedConfiguration,
  workstation: Workstation,
  result: FactoryEmulatorOutcome["result"],
): readonly WorkstationRoute[] {
  if (result === "accepted") return workstation.outputs ?? [];
  if (result === "continued") return workstation.onContinue ?? [];
  if (result === "failed" && (workstation.onFailure?.length ?? 0) > 0) {
    return workstation.onFailure ?? [];
  }
  if (result === "rejected" && (workstation.onRejection?.length ?? 0) > 0) {
    return workstation.onRejection ?? [];
  }

  if (result === "rejected" && workstation.behavior === "REPEATER") {
    return workstation.inputs;
  }
  return defaultFailureRoutesFor(configuration, workstation);
}

function routedWorksFor(
  configuration: ValidatedConfiguration,
  state: StartedState,
  dispatch: NonNullable<FactoryEmulatorSessionWork["dispatch"]>,
  inputs: readonly FactoryEmulatorSessionWork[],
): readonly FactoryEmulatorSessionWork[] {
  const workstation = configuration.factory.workstations?.find(
    ({ name }) => name === dispatch.workstation,
  );
  const first = inputs[0];
  if (workstation === undefined || first === undefined) return [];
  const routes = outcomeRoutesFor(
    configuration,
    workstation,
    dispatch.outcome.result,
  );
  const visits = visitsAfterTransition(
    inputs.map((input) => input.visits),
    workstation.name,
  );

  return routes.map((route, ordinal) => {
    const matching = inputs.find(({ workType }) => workType === route.workType);
    const lineageSource = matching ?? first;
    const workId =
      matching?.workId ??
      identity(
        "work",
        state.sessionId,
        "route",
        dispatch.dispatchId,
        ordinal,
        route,
      );
    const preserveInput =
      workstation.workPropagation?.mode === "PRESERVE_INPUT";
    const input = preserveInput
      ? lineageSource.input
      : (dispatch.outcome.output ?? lineageSource.input);
    const targetStateType = configuration.factory.workTypes
      ?.find(({ name }) => name === route.workType)
      ?.states.find(({ name }) => name === route.state)?.type;
    return {
      submissionId: `${first.submissionId}/${workstation.name}/${ordinal}`,
      requestId: first.requestId,
      traceId: lineageSource.traceId,
      tokenId: identity("token", dispatch.dispatchId, ordinal, route),
      workId,
      rootWorkId: lineageSource.rootWorkId,
      visits,
      workType: route.workType,
      state: route.state,
      ...(input === undefined ? {} : { input }),
      ...(matching !== undefined
        ? matching.parent === undefined
          ? {}
          : { parent: matching.parent }
        : { parent: first.workId }),
      queuedElapsedMs: dispatch.dueElapsedMs,
      lastDispatchElapsedMs: Math.min(
        lineageSource.lastDispatchElapsedMs ?? dispatch.dueElapsedMs,
        dispatch.dueElapsedMs,
      ),
      phase:
        targetStateType === "TERMINAL" || targetStateType === "FAILED"
          ? ("completed" as const)
          : ("ready" as const),
    };
  });
}

function eventWorkFor(
  configuration: ValidatedConfiguration,
  work: FactoryEmulatorSessionWork,
) {
  return {
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
        "PROCESSING",
    },
    currentChainingTraceId: work.traceId,
    traceId: work.traceId,
    ...(work.input === undefined ? {} : { payload: work.input }),
  };
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
      tokenId: identity("token", workId, submission.workType, submission.state),
      workId,
      rootWorkId: workId,
      visits: {},
      workType: submission.workType,
      state: submission.state,
      ...(submission.input === undefined ? {} : { input: submission.input }),
      ...(submission.parent === undefined ? {} : { parent: submission.parent }),
      queuedElapsedMs: state.virtualElapsedMs,
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

function logicalMoveCalculation(
  configuration: ValidatedConfiguration,
  state: StartedState,
  workstation: Workstation,
  outcome: FactoryEmulatorOutcome,
  inputs: readonly FactoryEmulatorSessionWork[],
  resources: readonly FactorySchedulerResourceClaim[],
  logicalMoveId: string,
  sequence: number,
): {
  readonly routedWorks: readonly FactoryEmulatorSessionWork[];
  readonly event: FactoryEvent;
} {
  const primary = inputs[0];
  if (primary === undefined) throw new Error("Logical move has no input Work.");
  const routedWorks = routedWorksFor(
    configuration,
    state,
    {
      dispatchId: logicalMoveId,
      completionId: identity("completion", logicalMoveId),
      transitionId: workstation.name,
      workstation: workstation.name,
      worker: "",
      startedElapsedMs: state.virtualElapsedMs,
      dueElapsedMs: state.virtualElapsedMs,
      inputTokenIds: inputs.map(({ tokenId }) => tokenId),
      resources,
      outcome,
    },
    inputs,
  );
  return {
    routedWorks,
    event: {
      schemaVersion: "agent-factory.event.v1",
      id: identity(
        "event",
        state.sessionId,
        sequence,
        "DISPATCH_RESPONSE",
        logicalMoveId,
      ),
      type: "DISPATCH_RESPONSE",
      context: eventContext(state, sequence, state.virtualTime, {
        requestId: primary.requestId,
        traceIds: inputs.map(({ traceId }) => traceId),
        workIds: inputs.map(({ workId }) => workId),
        currentChainingTraceId: primary.traceId,
        previousChainingTraceIds: inputs.slice(1).map(({ traceId }) => traceId),
      }),
      payload: {
        transitionId: workstation.name,
        outcome: "ACCEPTED",
        durationMillis: 0,
        ...(routedWorks.length === 0
          ? {}
          : {
              outputWork: routedWorks.map((routed) =>
                eventWorkFor(configuration, routed),
              ),
            }),
      },
    },
  };
}

function replaceInputPhases(
  replacements: FactoryEmulatorSessionWork[],
  state: StartedState,
  indexes: readonly number[],
  phase: FactoryEmulatorSessionWork["phase"],
): void {
  for (const index of indexes) {
    const consumed = state.works[index];
    if (consumed !== undefined) replacements[index] = { ...consumed, phase };
  }
}

function workerDispatchRequestEvent(
  state: StartedState,
  inputs: readonly FactoryEmulatorSessionWork[],
  transitionId: string,
  dispatchId: string,
  resources: readonly { name: string; capacity: number }[],
  sequence: number,
): FactoryEvent {
  const primary = inputs[0];
  if (primary === undefined)
    throw new Error("Worker dispatch has no input Work.");
  return {
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
      requestId: primary.requestId,
      traceIds: inputs.map(({ traceId }) => traceId),
      workIds: inputs.map(({ workId }) => workId),
      currentChainingTraceId: primary.traceId,
      previousChainingTraceIds: inputs.slice(1).map(({ traceId }) => traceId),
    }),
    payload: {
      transitionId,
      inputs: inputs.map(({ workId }) => ({ workId })),
      ...(resources.length === 0 ? {} : { resources: [...resources] }),
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
  let pendingTransaction: PendingTransaction | undefined;
  let pendingAdvanceContext: PendingAdvanceContext | undefined;
  let commandInFlight: FactoryEmulatorCommand | undefined;
  let lastError: FactoryEmulatorSessionError | undefined;

  const pauseExecution = (
    command: FactoryEmulatorCommand,
    diagnostic: FactoryEmulatorExecutionDiagnostic,
  ): never => {
    lastError = {
      code: "execution_paused",
      operation: "execute",
      command,
      message: `Factory emulator execution paused: ${diagnostic.kind}.`,
      diagnostic: clone(diagnostic),
    };
    throw new FactoryEmulatorExecutionPausedError(diagnostic);
  };

  const assertCandidateWithinBudgets = (
    command: FactoryEmulatorCommand,
    candidate: StartedState | ClosedState,
  ): void => {
    const checks = [
      {
        limit: "completedDispatches" as const,
        configured: configuration.limits.maxCompletedDispatches,
        observed: candidate.counters.completedDispatches,
      },
      {
        limit: "events" as const,
        configured: configuration.limits.maxEvents,
        observed: candidate.counters.events,
      },
      {
        limit: "virtualElapsedMs" as const,
        configured: configuration.limits.maxVirtualElapsedMs,
        observed: candidate.virtualElapsedMs,
      },
    ];
    const exceeded = checks.find(
      ({ configured, observed }) => observed > configured,
    );
    if (exceeded !== undefined) {
      pauseExecution(command, {
        kind: "budget-exceeded",
        ...exceeded,
        virtualTime: candidate.virtualTime,
        virtualElapsedMs: candidate.virtualElapsedMs,
      });
    }
  };

  const assertWorkBound = (
    command: FactoryEmulatorCommand,
    observed: number,
  ): void => {
    if (observed <= configuration.limits.maxSynchronousWorkItems) return;
    const state = committedState;
    pauseExecution(command, {
      kind: "bounded-work-exceeded",
      limit: "synchronousWorkItems",
      configured: configuration.limits.maxSynchronousWorkItems,
      observed,
      virtualTime:
        state.lifecycle === "pre-start"
          ? configuration.scenario.startAt
          : state.virtualTime,
      virtualElapsedMs: state.virtualElapsedMs,
    });
  };

  const status = (): FactoryEmulatorSessionStatus => {
    const common = {
      virtualTime:
        committedState.lifecycle !== "pre-start"
          ? committedState.virtualTime
          : configuration.scenario.startAt,
      virtualElapsedMs: committedState.virtualElapsedMs,
      budgetUsage: budgetUsage(committedState, configuration.limits),
    };
    if (lastError !== undefined) {
      return clone({
        ...common,
        phase: "error",
        reason:
          lastError.operation === "execute"
            ? "Execution paused at a configured safety boundary."
            : lastError.operation === "close"
              ? "The event sink rejected the pending close."
              : "The event sink rejected the pending transaction.",
        ...(pendingTransaction === undefined
          ? {}
          : {
              pendingTransaction: {
                command: pendingTransaction.command,
                phase: pendingTransaction.phase,
                eventCount: pendingTransaction.batch.length,
              },
            }),
        error: lastError,
      });
    }
    if (commandInFlight !== undefined) {
      return clone({
        ...common,
        phase: "active",
        reason: `${commandInFlight} is awaiting the event sink.`,
        ...(pendingTransaction === undefined
          ? {}
          : {
              pendingTransaction: {
                command: pendingTransaction.command,
                phase: pendingTransaction.phase,
                eventCount: pendingTransaction.batch.length,
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
    if (committedState.lifecycle === "closed") {
      return clone({
        ...common,
        phase: "closed",
        reason: "The session is closed.",
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

  const assertCommand = (
    command: FactoryEmulatorCommand,
    retryKey: string,
  ): PendingTransaction | undefined => {
    if (commandInFlight !== undefined) {
      throw new FactoryEmulatorPendingCommandError(command, commandInFlight);
    }
    if (pendingTransaction !== undefined) {
      if (
        command === pendingTransaction.command &&
        retryKey === pendingTransaction.retryKey
      ) {
        return pendingTransaction;
      }
      throw new FactoryEmulatorPendingCommandError(
        command,
        pendingTransaction.command,
      );
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
    return undefined;
  };

  const acceptPendingTransaction = async (): Promise<void> => {
    const transaction = pendingTransaction;
    if (transaction === undefined) return;
    try {
      if (transaction.phase === "sink-write") {
        await configuration.sink.write(clone(transaction.batch));
        if (transaction.command === "close") {
          transaction.phase = "sink-close";
        }
      }
      if (transaction.phase === "sink-close") {
        await configuration.sink.close?.();
      }
      committedState = transaction.candidate;
      pendingTransaction = undefined;
      lastError = undefined;
    } catch (rejection) {
      lastError =
        transaction.phase === "sink-close"
          ? {
              code: "sink_close_rejected",
              operation: "close",
              command: transaction.command,
              message: rejectionMessage(rejection),
            }
          : {
              code: "sink_write_rejected",
              operation: "write",
              command: transaction.command,
              message: rejectionMessage(rejection),
            };
      throw rejection;
    }
  };

  const write = async (
    command: Exclude<FactoryEmulatorCommand, "reset">,
    retryKey: string,
    batch: readonly FactoryEvent[],
    candidate: StartedState | ClosedState,
  ): Promise<void> => {
    assertCandidateWithinBudgets(command, candidate);
    pendingTransaction = {
      command,
      retryKey,
      batch: clone(batch),
      candidate: clone(candidate),
      phase: "sink-write",
    };
    await acceptPendingTransaction();
  };

  const start = async (): Promise<FactoryEmulatorStartReceipt> => {
    const retryKey = "start";
    const retry = assertCommand("start", retryKey);
    commandInFlight = "start";
    try {
      if (retry !== undefined) {
        await acceptPendingTransaction();
        const candidate = retry.candidate as StartedState;
        const batches = [
          retry.batch.slice(0, 3),
          ...(retry.batch.length > 3 ? [retry.batch.slice(3)] : []),
        ];
        return clone({ status: "started", batches, state: candidate });
      }
      lastError = undefined;
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
      assertWorkBound("start", initial.length);
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
      await write("start", retryKey, combined, candidate);
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
    const retryKey = canonicalJson(Array.isArray(value) ? value : [value]);
    const retry = assertCommand("submit", retryKey);
    if (retry !== undefined) {
      commandInFlight = "submit";
      try {
        await acceptPendingTransaction();
        return clone({
          status: "submitted",
          batch: retry.batch,
          state: retry.candidate as StartedState,
        });
      } finally {
        commandInFlight = undefined;
      }
    }
    const state = committedState as StartedState;
    assertWorkBound("submit", Array.isArray(value) ? value.length : 1);
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
      await write("submit", retryKey, batch, candidate);
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
    const { candidates, executableWork } = schedulerCandidatesFor(
      configuration,
      state,
      cursors,
    );
    for (const { value, resources } of selectFactorySchedulerCandidates(
      candidates,
      undefined,
      availableResourcesFor(configuration, state),
    )) {
      const { indexes, execution, invocation, cursorKey } = value;
      const works = indexes.flatMap((index) => {
        const work = state.works[index];
        return work === undefined ? [] : [work];
      });
      const work = works[0];
      if (
        work === undefined ||
        works.length !== indexes.length ||
        execution.workstation === undefined ||
        execution.outcome === undefined
      )
        continue;
      if (cursorKey !== undefined) cursors[cursorKey] = invocation + 1;
      const logicalMove = execution.workstation.type === "LOGICAL_MOVE";
      const transitionId = execution.workstation.name;
      const dispatchId = identity(
        logicalMove ? "logical-move" : "dispatch",
        works.map(({ tokenId }) => tokenId),
        transitionId,
        invocation,
      );
      const completionId = identity("completion", dispatchId);
      const dueElapsedMs =
        state.virtualElapsedMs + execution.outcome.durationMs;
      const eventResources = eventResourcesFor(configuration, resources);
      virtualTimeAt(configuration.scenario, dueElapsedMs);
      if (logicalMove) {
        const calculation = logicalMoveCalculation(
          configuration,
          state,
          execution.workstation,
          execution.outcome,
          works,
          resources,
          dispatchId,
          state.counters.events + events.length,
        );
        replaceInputPhases(replacements, state, indexes, "completed");
        replacements.push(...calculation.routedWorks);
        events.push(calculation.event);
        continue;
      }
      replaceInputPhases(replacements, state, indexes, "active");
      const primaryIndex = indexes[0];
      if (primaryIndex === undefined) continue;
      replacements[primaryIndex] = {
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
          inputTokenIds: works.map(({ tokenId }) => tokenId),
          resources,
          outcome: execution.outcome,
        },
      };
      const sequence = state.counters.events + events.length;
      events.push(
        workerDispatchRequestEvent(
          state,
          works,
          transitionId,
          dispatchId,
          eventResources,
          sequence,
        ),
      );
    }
    for (const [index, work] of state.works.entries()) {
      if (work.phase === "ready" && !executableWork.has(index)) {
        replacements[index] = { ...work, phase: "waiting" };
      }
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
    for (const work of state.works) {
      if (
        work.phase !== "active" ||
        work.dispatch?.dueElapsedMs !== dueElapsedMs
      )
        continue;
      const { dispatch } = work;
      const inputs = dispatch.inputTokenIds.flatMap((tokenId) => {
        const input = state.works.find(
          (candidate) => candidate.tokenId === tokenId,
        );
        return input === undefined ? [] : [input];
      });
      if (inputs.length !== dispatch.inputTokenIds.length) continue;
      const routedWorks = routedWorksFor(
        configuration,
        state,
        dispatch,
        inputs,
      );
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
          traceIds: inputs.map(({ traceId }) => traceId),
          workIds: inputs.map(({ workId }) => workId),
          currentChainingTraceId: work.traceId,
          previousChainingTraceIds: inputs
            .slice(1)
            .map(({ traceId }) => traceId),
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
          ...(routedWorks.length === 0
            ? {}
            : {
                outputWork: routedWorks.map((routed) =>
                  eventWorkFor(configuration, routed),
                ),
              }),
        },
      });
      for (const input of inputs) {
        const inputIndex = state.works.findIndex(
          ({ tokenId }) => tokenId === input.tokenId,
        );
        if (inputIndex >= 0)
          replacements[inputIndex] = { ...input, phase: "completed" };
      }
      replacements.push(...routedWorks);
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

  // biome-ignore lint/complexity/noExcessiveLinesPerFunction: Advancement keeps retry, budget, yield, and transaction state in one traceable command boundary.
  const runAdvance = async (
    command: "advanceBy" | "advanceToNext",
    requestedDuration?: number,
  ): Promise<FactoryEmulatorAdvanceReceipt> => {
    if (
      requestedDuration !== undefined &&
      (!Number.isSafeInteger(requestedDuration) || requestedDuration < 0)
    ) {
      throw new FactoryEmulatorDurationError(requestedDuration);
    }
    const retryKey = canonicalJson([command, requestedDuration]);
    const retry = assertCommand(command, retryKey);
    const original = committedState as StartedState;
    const target =
      retry === undefined
        ? requestedDuration === undefined
          ? undefined
          : original.virtualElapsedMs + requestedDuration
        : pendingAdvanceContext?.target;
    if (retry === undefined && target !== undefined) {
      virtualTimeAt(configuration.scenario, target);
    }
    const context =
      retry === undefined
        ? {
            command,
            retryKey,
            fromVirtualTime: original.virtualTime,
            ...(target === undefined ? {} : { target }),
            wasUnfinished: unfinished(original),
            batches: [],
            zeroDurationBatches: 0,
          }
        : pendingAdvanceContext;
    if (context === undefined) {
      throw new Error("Pending advancement context is unavailable.");
    }
    pendingAdvanceContext = context;
    commandInFlight = command;
    let synchronousBatches = 0;
    const acceptSchedulerBatch = async (calculation: {
      batch: readonly FactoryEvent[];
      state: StartedState;
    }): Promise<void> => {
      const beforeElapsedMs = (committedState as StartedState).virtualElapsedMs;
      const nextZeroDurationBatches =
        calculation.state.virtualElapsedMs === beforeElapsedMs
          ? context.zeroDurationBatches + 1
          : 0;
      if (
        nextZeroDurationBatches > configuration.limits.maxZeroDurationBatches
      ) {
        pauseExecution(command, {
          kind: "zero-duration-cycle",
          limit: "zeroDurationBatches",
          configured: configuration.limits.maxZeroDurationBatches,
          observed: nextZeroDurationBatches,
          virtualTime: (committedState as StartedState).virtualTime,
          virtualElapsedMs: beforeElapsedMs,
        });
      }
      assertWorkBound(command, calculation.batch.length);
      context.batches.push(calculation.batch);
      await write(command, retryKey, calculation.batch, calculation.state);
      context.zeroDurationBatches = nextZeroDurationBatches;
      synchronousBatches += 1;
      if (
        configuration.yieldControl !== undefined &&
        synchronousBatches >= configuration.limits.maxSynchronousBatches
      ) {
        synchronousBatches = 0;
        await configuration.yieldControl();
      }
    };
    try {
      if (retry !== undefined) {
        const beforeElapsedMs = original.virtualElapsedMs;
        await acceptPendingTransaction();
        context.zeroDurationBatches =
          (committedState as StartedState).virtualElapsedMs === beforeElapsedMs
            ? context.zeroDurationBatches + 1
            : 0;
        synchronousBatches = 1;
        if (
          configuration.yieldControl !== undefined &&
          synchronousBatches >= configuration.limits.maxSynchronousBatches
        ) {
          synchronousBatches = 0;
          await configuration.yieldControl();
        }
      } else lastError = undefined;
      const continueAdvancing = !(
        retry !== undefined && command === "advanceToNext"
      );
      while (continueAdvancing) {
        const state = committedState as StartedState;
        if (state.works.some(({ phase }) => phase === "ready")) {
          const calculation = dispatchCalculation(state);
          if (calculation.batch.length > 0) {
            await acceptSchedulerBatch(calculation);
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
        await acceptSchedulerBatch(calculation);
        if (command === "advanceToNext") break;
      }
      let finalState = committedState as StartedState;
      if (
        target !== undefined &&
        context.wasUnfinished &&
        finalState.virtualElapsedMs < target
      ) {
        finalState = {
          ...finalState,
          virtualElapsedMs: target,
          virtualTime: virtualTimeAt(configuration.scenario, target),
        };
      }
      const madeProgress =
        context.batches.length > 0 ||
        finalState.virtualTime !== context.fromVirtualTime;
      if (madeProgress) {
        finalState = {
          ...finalState,
          counters: {
            ...finalState.counters,
            commands: finalState.counters.commands + 1,
          },
        };
        assertCandidateWithinBudgets(command, finalState);
        committedState = finalState;
      }
      const receipt: FactoryEmulatorAdvanceReceipt = clone({
        status: madeProgress ? "advanced" : "idle",
        command,
        fromVirtualTime: context.fromVirtualTime,
        virtualTime: finalState.virtualTime,
        virtualElapsedMs: finalState.virtualElapsedMs,
        batches: context.batches,
        state: finalState,
      });
      pendingAdvanceContext = undefined;
      return receipt;
    } finally {
      commandInFlight = undefined;
    }
  };

  const close = async (): Promise<FactoryEmulatorCloseReceipt> => {
    const retryKey = "close";
    const retry = assertCommand("close", retryKey);
    commandInFlight = "close";
    try {
      if (retry !== undefined) {
        await acceptPendingTransaction();
        return clone({
          status: "closed",
          batch: retry.batch,
          state: retry.candidate as ClosedState,
        });
      }
      lastError = undefined;
      const state = committedState as StartedState;
      const sequence = state.counters.events;
      const event: FactoryEvent = {
        schemaVersion: "agent-factory.event.v1",
        id: identity("event", state.sessionId, sequence, "SESSION_COMPLETED"),
        type: "SESSION_COMPLETED",
        context: eventContext(state, sequence, state.virtualTime),
        payload: {
          finalStatus: "TERMINATED",
          completedAt: state.virtualTime,
          durationMillis: state.virtualElapsedMs,
        },
      };
      const candidate: ClosedState = {
        ...state,
        lifecycle: "closed",
        counters: {
          ...state.counters,
          commands: state.counters.commands + 1,
          events: state.counters.events + 1,
        },
      };
      const batch = [event];
      await write("close", retryKey, batch, candidate);
      return clone({ status: "closed", batch, state: candidate });
    } finally {
      commandInFlight = undefined;
    }
  };

  const reset = (): FactoryEmulatorResetReceipt => {
    if (commandInFlight !== undefined) {
      throw new FactoryEmulatorPendingCommandError("reset", commandInFlight);
    }
    if (committedState.lifecycle === "closed") {
      throw new FactoryEmulatorLifecycleError("reset", "closed");
    }
    pendingTransaction = undefined;
    pendingAdvanceContext = undefined;
    lastError = undefined;
    committedState = clone(PRE_START_STATE);
    return clone({ status: "reset", state: committedState });
  };

  return {
    start,
    submit,
    advanceBy: (durationMs) => runAdvance("advanceBy", durationMs),
    advanceToNext: () => runAdvance("advanceToNext"),
    close,
    reset,
    state: () => clone(committedState),
    status,
  };
}
