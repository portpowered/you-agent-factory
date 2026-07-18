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
  plan,
  scenario,
  state,
}) {
  if (plan === undefined) {
    return undefined;
  }
  const plannedWorks = plan.workIndexes.map((index) => ({
    index,
    work: state.works[index],
  }));
  if (plan.kind === "completion") {
    return calculateCompletions({
      createEvent,
      identityCoordinates,
      scenario,
      state,
      completedWorks: plannedWorks,
    });
  }
  return calculateDispatchStarts({
    createEvent,
    identityCoordinates,
    scenario,
    state,
    readyWorks: plannedWorks,
    workIndexBySubmissionId: plan.workIndexBySubmissionId,
  });
}

/**
 * Inspects a bounded slice of retained Work before an atomic scheduler batch is
 * materialized. The returned continuation is explicit plain state and can be
 * resumed after the host receives a task turn.
 */
export function inspectNextSchedulerBatch({
  continuation,
  maximumWorkItems,
  state,
}) {
  const inspection = continuation === undefined
    ? {
        index: 0,
        earliestElapsedMs: undefined,
        earliestWorkIndexes: [],
        hasWaitingWork: false,
        readyWorkIndexes: [],
        workIndexBySubmissionId: {},
      }
    : continuation;
  let examined = 0;

  while (examined < maximumWorkItems && inspection.index < state.works.length) {
    const workIndex = inspection.index;
    const work = state.works[workIndex];
    inspection.index += 1;
    examined += 1;
    if (!Object.hasOwn(inspection.workIndexBySubmissionId, work.submissionId)) {
      Object.defineProperty(inspection.workIndexBySubmissionId, work.submissionId, {
        configurable: true,
        enumerable: true,
        value: workIndex,
        writable: true,
      });
    }
    if (work.phase === "active") {
      const deadline = work.dispatch.dueElapsedMs;
      if (
        inspection.earliestElapsedMs === undefined
        || deadline < inspection.earliestElapsedMs
      ) {
        inspection.earliestElapsedMs = deadline;
        inspection.earliestWorkIndexes = [workIndex];
      } else if (deadline === inspection.earliestElapsedMs) {
        boundedPush(
          inspection.earliestWorkIndexes,
          workIndex,
          maximumWorkItems,
        );
      }
    } else if (work.phase === "ready") {
      boundedPush(
        inspection.readyWorkIndexes,
        workIndex,
        maximumWorkItems,
      );
    } else if (work.phase === "waiting") {
      inspection.hasWaitingWork = true;
    }
  }

  if (inspection.index === state.works.length) {
    const completionIsDue = inspection.earliestElapsedMs !== undefined
      && inspection.earliestElapsedMs <= state.virtualElapsedMs;
    if (completionIsDue) {
      return completedInspection(
        "completion",
        inspection.earliestWorkIndexes,
        inspection.workIndexBySubmissionId,
      );
    }
    if (inspection.readyWorkIndexes.length > 0) {
      return completedInspection(
        "dispatch",
        inspection.readyWorkIndexes,
        inspection.workIndexBySubmissionId,
      );
    }
    return inspection.earliestElapsedMs === undefined
      ? { done: true, batch: undefined, hasWaitingWork: inspection.hasWaitingWork }
      : completedInspection(
          "completion",
          inspection.earliestWorkIndexes,
          inspection.workIndexBySubmissionId,
        );
  }
  return { done: false, continuation: inspection };
}

function boundedPush(values, value, maximum) {
  if (values.length <= maximum) {
    values.push(value);
  }
}

function completedInspection(kind, workIndexes, workIndexBySubmissionId) {
  return {
    done: true,
    batch: { kind, workItems: workIndexes.length },
    hasWaitingWork: false,
    plan: { kind, workIndexes, workIndexBySubmissionId },
  };
}

function calculateDispatchStarts({
  createEvent,
  identityCoordinates,
  scenario,
  state,
  readyWorks,
  workIndexBySubmissionId,
}) {
  const ruleCursors = { ...state.ruleCursors };
  const replacements = new Map();
  let eventOrdinal = 0;
  const events = readyWorks.flatMap(({ index, work }) => {
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
      replacements.set(index, { ...work, phase: "waiting" });
      return [];
    }
    const transitionId = rule?.id ?? "emulator-unmatched";
    const lineageTokenId = resolveLineageTokenId({
      identityCoordinates,
      outcome,
      scenario,
      state,
      work,
      workIndexBySubmissionId,
    });
    const dispatchCoordinates = {
      ...identityCoordinates,
      workId: work.workId,
      tokenId: work.tokenId,
      lineageTokenId,
      transitionId,
      invocationIndex,
    };
    const dispatchId = deriveFactoryEmulatorIdentity("dispatch", dispatchCoordinates);
    const completionId = deriveFactoryEmulatorIdentity("completion", dispatchCoordinates);
    const outputTokenId = deriveFactoryEmulatorIdentity("token", {
      ...dispatchCoordinates,
      dispatchId,
      completionId,
    });
    const durationMs = outcome.durationMs ?? 0;
    const dueElapsedMs = state.virtualElapsedMs + durationMs;
    if (!Number.isSafeInteger(dueElapsedMs)) {
      throw new FactoryEmulatorDurationError(durationMs);
    }
    virtualTimeAt(scenario, dueElapsedMs);
    replacements.set(index, {
      ...work,
      phase: "active",
      dispatch: {
        dispatchId,
        completionId,
        lineageTokenId,
        outputTokenId,
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
  return {
    elapsedMs: state.virtualElapsedMs,
    batch: events.length === 0 ? undefined : { events },
    state: {
      ...state,
      ruleCursors,
      counters: {
        ...state.counters,
        events: state.counters.events + events.length,
        dispatches: state.counters.dispatches + events.length,
      },
    },
    workReplacements: replacements,
  };
}

function calculateCompletions({
  createEvent,
  identityCoordinates,
  scenario,
  state,
  completedWorks,
}) {
  const elapsedMs = completedWorks[0].work.dispatch.dueElapsedMs;
  const eventTime = virtualTimeAt(scenario, elapsedMs);
  const replacements = new Map();
  const events = completedWorks.map(({ index, work }, ordinal) => {
    const { dispatch } = work;
    const accepted = dispatch.outcome.kind === "complete";
    replacements.set(index, {
      ...work,
      phase: "completed",
      tokenId: dispatch.outputTokenId,
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
        metadata: {
          emulatorLineageTokenId: dispatch.lineageTokenId,
          emulatorTokenId: dispatch.outputTokenId,
        },
      },
    });
  });
  return {
    elapsedMs,
    batch: { events },
    state: {
      ...stateAtElapsed(scenario, state, elapsedMs),
      counters: {
        ...state.counters,
        events: state.counters.events + events.length,
        completions: state.counters.completions + events.length,
      },
    },
    workReplacements: replacements,
  };
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

function resolveLineageTokenId({
  identityCoordinates,
  outcome,
  scenario,
  state,
  work,
  workIndexBySubmissionId,
}) {
  const cursor = outcome?.kind === "complete" ? outcome.lineageCursor : undefined;
  if (cursor?.kind === "initialSubmission") {
    return state.works[workIndexBySubmissionId[cursor.submissionId]]?.tokenId;
  }
  if (cursor?.kind === "scriptedOutcome") {
    const ruleIndex = scenario.rules.findIndex((rule) => rule.id === cursor.ruleId);
    return deriveFactoryEmulatorIdentity("token", {
      ...identityCoordinates,
      lineage: {
        kind: "scriptedOutcome",
        ruleIndex,
        ruleId: cursor.ruleId,
        outcomeIndex: cursor.outcomeIndex,
      },
    });
  }
  return work.tokenId;
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
