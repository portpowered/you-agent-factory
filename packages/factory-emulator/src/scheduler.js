import { deriveFactoryEmulatorIdentity } from "./identity.js";
import {
  resolveEmulatorScenarioResult,
  selectEmulatorRule,
} from "./semantics.js";
import {
  FactoryEmulatorDurationError,
  stateAtElapsed,
  virtualTimeAt,
} from "./virtual-time.js";

/** Calculates one deterministic scheduler batch without performing sink IO. */
export function calculateNextSchedulerBatch({
  createEvent,
  identityCoordinates,
  scenario,
  state,
}) {
  const dueNow = activeWorksAtEarliestDeadline(state)
    .filter((work) => work.dispatch.dueElapsedMs <= state.virtualElapsedMs);
  if (dueNow.length > 0) {
    return calculateCompletions({
      createEvent,
      identityCoordinates,
      scenario,
      state,
      completedWorks: dueNow,
    });
  }

  const readyWorks = state.works.filter((work) => work.phase === "ready");
  if (readyWorks.length > 0) {
    return calculateDispatchStarts({
      createEvent,
      identityCoordinates,
      scenario,
      state,
      readyWorks,
    });
  }

  const earliest = activeWorksAtEarliestDeadline(state);
  return earliest.length === 0
    ? undefined
    : calculateCompletions({
        createEvent,
        identityCoordinates,
        scenario,
        state,
        completedWorks: earliest,
      });
}

function calculateDispatchStarts({
  createEvent,
  identityCoordinates,
  scenario,
  state,
  readyWorks,
}) {
  const ruleCursors = { ...state.ruleCursors };
  const replacements = new Map();
  let eventOrdinal = 0;
  const events = readyWorks.flatMap((work) => {
    const submission = { ...work, id: work.submissionId };
    const rule = selectEmulatorRule(scenario, submission);
    const invocationIndex = rule === undefined ? 0 : (ruleCursors[rule.id] ?? 0);
    const resolution = resolveEmulatorScenarioResult(
      scenario,
      submission,
      invocationIndex,
    );
    if (rule !== undefined) {
      ruleCursors[rule.id] = invocationIndex + 1;
    }
    const outcome = resolutionOutcome(resolution);
    if (outcome === undefined) {
      replacements.set(work.workId, { ...work, phase: "waiting" });
      return [];
    }
    const transitionId = rule?.id ?? "emulator-unmatched";
    const dispatchCoordinates = {
      ...identityCoordinates,
      workId: work.workId,
      transitionId,
      invocationIndex,
    };
    const dispatchId = deriveFactoryEmulatorIdentity("dispatch", dispatchCoordinates);
    const completionId = deriveFactoryEmulatorIdentity("completion", dispatchCoordinates);
    const durationMs = outcome.durationMs ?? 0;
    const dueElapsedMs = state.virtualElapsedMs + durationMs;
    if (!Number.isSafeInteger(dueElapsedMs)) {
      throw new FactoryEmulatorDurationError(durationMs);
    }
    virtualTimeAt(scenario, dueElapsedMs);
    replacements.set(work.workId, {
      ...work,
      phase: "active",
      dispatch: {
        dispatchId,
        completionId,
        transitionId,
        startedElapsedMs: state.virtualElapsedMs,
        dueElapsedMs,
        outcome,
      },
    });
    const sequence = state.counters.events + eventOrdinal;
    eventOrdinal += 1;
    return [createEvent({
      identityCoordinates,
      sequence,
      tick: state.counters.events,
      sessionId: state.sessionId,
      eventTime: state.virtualTime,
      type: "DISPATCH_REQUEST",
      context: dispatchContext(work, dispatchId),
      payload: {
        transitionId,
        inputs: [{ workId: work.workId }],
      },
    })];
  });
  return copy({
    elapsedMs: state.virtualElapsedMs,
    batch: events.length === 0 ? undefined : { events },
    state: {
      ...state,
      works: replaceWorks(state.works, replacements),
      ruleCursors,
      counters: {
        ...state.counters,
        events: state.counters.events + events.length,
        dispatches: state.counters.dispatches + events.length,
      },
    },
  });
}

function calculateCompletions({
  createEvent,
  identityCoordinates,
  scenario,
  state,
  completedWorks,
}) {
  const elapsedMs = completedWorks[0].dispatch.dueElapsedMs;
  const eventTime = virtualTimeAt(scenario, elapsedMs);
  const replacements = new Map();
  const events = completedWorks.map((work, ordinal) => {
    const { dispatch } = work;
    const accepted = dispatch.outcome.kind === "complete";
    replacements.set(work.workId, {
      ...work,
      phase: "completed",
      ...(accepted && dispatch.outcome.output !== undefined
        ? { output: dispatch.outcome.output }
        : {}),
      ...(!accepted ? { rejectionReason: dispatch.outcome.reason } : {}),
    });
    return createEvent({
      identityCoordinates,
      sequence: state.counters.events + ordinal,
      tick: state.counters.events,
      sessionId: state.sessionId,
      eventTime,
      type: "DISPATCH_RESPONSE",
      context: dispatchContext(work, dispatch.dispatchId),
      payload: {
        completionId: dispatch.completionId,
        transitionId: dispatch.transitionId,
        outcome: accepted ? "ACCEPTED" : "REJECTED",
        ...(!accepted ? { feedback: dispatch.outcome.reason } : {}),
        durationMillis: dispatch.dueElapsedMs - dispatch.startedElapsedMs,
      },
    });
  });
  return copy({
    elapsedMs,
    batch: { events },
    state: {
      ...stateAtElapsed(scenario, state, elapsedMs),
      works: replaceWorks(state.works, replacements),
      counters: {
        ...state.counters,
        events: state.counters.events + events.length,
        completions: state.counters.completions + events.length,
      },
    },
  });
}

function resolutionOutcome(resolution) {
  if (resolution.kind === "outcome") {
    return resolution.outcome;
  }
  if (resolution.behavior.kind === "reject") {
    return { kind: "reject", reason: resolution.behavior.reason };
  }
  return undefined;
}

function activeWorksAtEarliestDeadline(state) {
  const active = state.works.filter((work) => work.phase === "active");
  if (active.length === 0) {
    return [];
  }
  const earliest = Math.min(...active.map((work) => work.dispatch.dueElapsedMs));
  return active.filter((work) => work.dispatch.dueElapsedMs === earliest);
}

function dispatchContext(work, dispatchId) {
  return {
    dispatchId,
    requestId: work.requestId,
    traceIds: [work.traceId],
    workIds: [work.workId],
    currentChainingTraceId: work.traceId,
  };
}

function replaceWorks(works, replacements) {
  return works.map((work) => replacements.get(work.workId) ?? work);
}

function copy(value) {
  return structuredClone(value);
}
