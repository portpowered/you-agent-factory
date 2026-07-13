import { describe, expect, it } from "vitest";
import { FACTORY_EVENT_TYPES, type FactoryEvent } from "../../../../api/events";
import { createTimelineCheckpointIndexedDBTestDouble } from "../../../../testing/timeline-checkpoint-indexeddb-test-utils";
import {
  createMaterializedWorkOutcomeState,
  MATERIALIZED_WORK_OUTCOME_RETENTION,
  type MaterializedWorkOutcomeState,
  reduceMaterializedWorkOutcomeEvents,
} from "../../../work-outcome/public/materializer";
import { emptyReplayWorldState } from "../timeline/replayWorldStateSupport";
import type { FactoryTimelineCheckpoint } from "../timeline/storeState";
import {
  persistTimelineCheckpoint,
  readTimelineCheckpoint,
  type TimelineCheckpointStreamIdentity,
} from "../timelineCheckpointPersistence";

const SESSION_ID = "a1b2c3d4-e5f6-4789-a012-3456789abcde";

interface StoredEnvelope {
  checkpoint: FactoryTimelineCheckpoint;
}

describe("materialized timeline checkpoint round trip", () => {
  it("preserves the complete retained baseline without sharing mutable references", async () => {
    const fixture = createTimelineCheckpointIndexedDBTestDouble();
    const identity = streamIdentity("generation-round-trip");
    const checkpoint = representativeCheckpoint();
    const expected = structuredClone(checkpoint);

    await persistTimelineCheckpoint(fixture.indexedDB, checkpoint, identity);

    checkpoint.materializedWorkOutcomeState.failedWorkLabels[0] =
      "live-mutated";
    const liveWorkItem =
      checkpoint.materializedWorkOutcomeState.accumulator.workItemsByID[
        "work-1"
      ];
    if (!liveWorkItem) {
      throw new Error("expected the representative live work item");
    }
    liveWorkItem.displayName = "live-mutated";
    checkpoint.replayState.textBlobsByID.summary = "live-mutated";

    const restored = await readTimelineCheckpoint(fixture.indexedDB, identity);
    expect(restored).toEqual(expected);
    if (!restored) {
      throw new Error("expected a restored checkpoint");
    }

    const suffixEvent = continuationEvent();
    expect(
      reduceMaterializedWorkOutcomeEvents(
        restored.materializedWorkOutcomeState,
        [suffixEvent],
      ),
    ).toEqual(
      reduceMaterializedWorkOutcomeEvents(
        expected.materializedWorkOutcomeState,
        [suffixEvent],
      ),
    );

    const restoredSample = restored.materializedWorkOutcomeState.samples[0];
    const restoredDispatch =
      restored.materializedWorkOutcomeState.accumulator.activeDispatchesByID[
        "dispatch-1"
      ];
    if (!restoredSample || !restoredDispatch) {
      throw new Error("expected retained sample and dispatch details");
    }
    restoredSample.failedWorkLabels[0] = "hydrated-mutated";
    restoredDispatch.inputWorkIDs.push("hydrated-mutated");
    restored.replayState.textBlobsByID.summary = "hydrated-mutated";

    await expect(
      readTimelineCheckpoint(fixture.indexedDB, identity),
    ).resolves.toEqual(expected);
  });

  it("applies deterministic presentation limits while preserving the exact accumulator", async () => {
    const ascending = createTimelineCheckpointIndexedDBTestDouble();
    const descending = createTimelineCheckpointIndexedDBTestDouble();
    const ascendingState = overLimitState(false);
    const descendingState = overLimitState(true);

    await persistTimelineCheckpoint(
      ascending.indexedDB,
      checkpointWithState(ascendingState),
      streamIdentity("generation-bounded"),
    );
    await persistTimelineCheckpoint(
      descending.indexedDB,
      checkpointWithState(descendingState),
      streamIdentity("generation-bounded"),
    );

    const first = storedMaterializedState(ascending.records);
    const second = storedMaterializedState(descending.records);
    expect(JSON.stringify(first)).toBe(JSON.stringify(second));

    expect(first.counts).toEqual(ascendingState.counts);
    expect(first.samples).toHaveLength(
      MATERIALIZED_WORK_OUTCOME_RETENTION.samples,
    );
    expect(first.samples[0]?.tick).toBe(1);
    expect(first.samples.at(-1)?.tick).toBe(512);
    expect(Object.keys(first.failedByWorkType)).toHaveLength(
      MATERIALIZED_WORK_OUTCOME_RETENTION.breakdownEntries,
    );
    expect(first.failedWorkLabels).toHaveLength(
      MATERIALIZED_WORK_OUTCOME_RETENTION.labels,
    );
    expect(first.accumulator).toEqual(ascendingState.accumulator);
    expect(
      first.accumulator.activeDispatchesByID["dispatch-0000"]?.inputWorkIDs,
    ).toHaveLength(4);
    expect(first.failedWorkLabels[0]).toBe("label-0000");
    expect(first.failedWorkLabels.at(-1)).toBe("label-0127");
    expect(
      first.samples[0]?.failedWorkLabels.every(
        (label) =>
          label.length <= MATERIALIZED_WORK_OUTCOME_RETENTION.textChars,
      ),
    ).toBe(true);
  });
});

describe("materialized timeline checkpoint accumulator overflow", () => {
  it("removes a prior checkpoint instead of hydrating a lossy accumulator", async () => {
    const fixture = createTimelineCheckpointIndexedDBTestDouble();
    const identity = streamIdentity("generation-overflow");
    const prefixEvents = overLimitQueuedEvents();
    const overLimit = reduceMaterializedWorkOutcomeEvents(
      createMaterializedWorkOutcomeState(),
      prefixEvents,
    );
    const suffix = continuationEvent();

    await persistTimelineCheckpoint(
      fixture.indexedDB,
      representativeCheckpoint(),
      identity,
    );
    expect(fixture.records.size).toBe(1);

    await persistTimelineCheckpoint(
      fixture.indexedDB,
      checkpointWithState(overLimit),
      identity,
    );

    await expect(
      readTimelineCheckpoint(fixture.indexedDB, identity),
    ).resolves.toBeNull();
    expect(fixture.records.size).toBe(0);

    const replayed = reduceMaterializedWorkOutcomeEvents(
      createMaterializedWorkOutcomeState(),
      [...prefixEvents, suffix],
    );
    const uninterrupted = reduceMaterializedWorkOutcomeEvents(overLimit, [
      suffix,
    ]);
    expect(replayed).toEqual(uninterrupted);
    expect(uninterrupted.counts.queued).toBe(
      MATERIALIZED_WORK_OUTCOME_RETENTION.accumulatorMapEntries + 1,
    );
  });
});

function representativeCheckpoint(): FactoryTimelineCheckpoint {
  const state = createMaterializedWorkOutcomeState();
  state.cursor = {
    eventID: "event-9",
    eventTime: "2026-07-13T12:00:00Z",
    sequence: 9,
    tick: 7,
  };
  state.accumulator = {
    activeDispatchesByID: {
      "dispatch-1": { inputWorkIDs: ["work-1"], systemOnly: false },
    },
    appliedEventCount: 10,
    completedAcceptedCount: 4,
    completedDispatchCount: 5,
    failedWorkItemsByID: {
      "failed-1": {
        displayName: "Failed Story",
        id: "failed-1",
        traceID: "trace-failed",
        workTypeID: "story",
      },
    },
    initialPlaceIDs: ["story:queued"],
    workItemsByID: {
      "work-1": {
        displayName: "Queued Story",
        id: "work-1",
        placeID: "story:queued",
        traceID: "trace-1",
        workTypeID: "story",
      },
    },
  };
  state.counts = {
    completed: 4,
    dispatched: 6,
    failed: 1,
    inFlight: 1,
    queued: 1,
  };
  state.failedByWorkType = { story: 1 };
  state.failedWorkLabels = ["Failed Story"];
  state.samples = [
    sample(6, { failedByWorkType: {}, failedWorkLabels: [] }),
    sample(7, {
      failedByWorkType: { story: 1 },
      failedWorkLabels: ["Failed Story"],
    }),
  ];

  const replayState = emptyReplayWorldState(7);
  replayState.textBlobsByID.summary = "retained replay state";
  return {
    afterEventId: "event-9",
    afterSequence: 9,
    materializedWorkOutcomeState: state,
    replayState,
    selectedTick: 7,
  };
}

function overLimitState(
  reverseInsertion: boolean,
): MaterializedWorkOutcomeState {
  const state = createMaterializedWorkOutcomeState();
  const detailEntryCount = 4;
  const indexes = Array.from({ length: detailEntryCount }, (_, index) => index);
  if (reverseInsertion) {
    indexes.reverse();
  }
  const longPresentationText = "x".repeat(
    MATERIALIZED_WORK_OUTCOME_RETENTION.textChars + 20,
  );

  for (const index of indexes) {
    const suffix = String(index).padStart(4, "0");
    state.accumulator.workItemsByID[`work-${suffix}`] = {
      displayName: `Work ${suffix}`,
      id: `work-${suffix}`,
      placeID: "story:init",
      traceID: `trace-${suffix}`,
      workTypeID: "story",
    };
  }
  state.accumulator.activeDispatchesByID["dispatch-0000"] = {
    inputWorkIDs: [...indexes]
      .sort((left, right) => left - right)
      .map((index) => `input-${String(index).padStart(4, "0")}`),
    systemOnly: false,
  };
  state.accumulator.initialPlaceIDs = [...indexes]
    .sort((left, right) => left - right)
    .map((index) => `place-${String(index).padStart(4, "0")}`);
  state.counts = {
    completed: 50_000,
    dispatched: 60_000,
    failed: 20_000,
    inFlight: 10_000,
    queued: 30_000,
  };

  const breakdownCount =
    MATERIALIZED_WORK_OUTCOME_RETENTION.breakdownEntries + 1;
  const labelCount = MATERIALIZED_WORK_OUTCOME_RETENTION.labels + 1;
  for (
    let index = 0;
    index < Math.max(breakdownCount, labelCount);
    index += 1
  ) {
    const suffix = String(index).padStart(4, "0");
    if (index < breakdownCount)
      state.failedByWorkType[`type-${suffix}`] = index;
    if (index < labelCount) state.failedWorkLabels.push(`label-${suffix}`);
  }
  state.samples = Array.from(
    { length: MATERIALIZED_WORK_OUTCOME_RETENTION.samples + 1 },
    (_, tick) =>
      sample(tick, {
        failedByWorkType: state.failedByWorkType,
        failedWorkLabels: [...state.failedWorkLabels, longPresentationText],
      }),
  );
  return state;
}

function overLimitQueuedEvents(): FactoryEvent[] {
  const count = MATERIALIZED_WORK_OUTCOME_RETENTION.accumulatorMapEntries + 1;
  return [
    {
      context: { eventTime: "2026-07-13T11:59:58Z", sequence: 0, tick: 0 },
      id: "initial-overflow",
      payload: {
        factory: {
          workTypes: [
            { name: "story", states: [{ name: "init", type: "INITIAL" }] },
          ],
        },
      },
      type: FACTORY_EVENT_TYPES.initialStructureRequest,
    },
    {
      context: { eventTime: "2026-07-13T11:59:59Z", sequence: 1, tick: 1 },
      id: "work-overflow",
      payload: {
        works: Array.from({ length: count }, (_, index) => ({
          workId: `work-${String(index).padStart(4, "0")}`,
          workTypeName: "story",
        })),
      },
      type: FACTORY_EVENT_TYPES.workRequest,
    },
  ];
}

function sample(
  tick: number,
  details: {
    failedByWorkType: Record<string, number>;
    failedWorkLabels: string[];
  },
) {
  return {
    completedCount: tick,
    dispatchedCount: tick,
    failedByWorkType: details.failedByWorkType,
    failedCount: tick,
    failedWorkLabels: details.failedWorkLabels,
    inFlightCount: tick,
    observedAt: tick,
    queuedCount: tick,
    tick,
  };
}

function checkpointWithState(
  materializedWorkOutcomeState: MaterializedWorkOutcomeState,
): FactoryTimelineCheckpoint {
  return {
    materializedWorkOutcomeState,
    replayState: emptyReplayWorldState(512),
    selectedTick: 512,
  };
}

function streamIdentity(
  streamGenerationID: string,
): TimelineCheckpointStreamIdentity {
  return {
    backendScopeID: "backend-a",
    factorySessionID: SESSION_ID,
    logicalSessionKeyID: "logical-a",
    streamGenerationID,
  };
}

function continuationEvent(): FactoryEvent {
  return {
    context: {
      eventTime: "2026-07-13T12:00:01Z",
      sequence: 10,
      tick: 8,
    },
    id: "event-10",
    payload: {},
    type: FACTORY_EVENT_TYPES.sessionStarted,
  };
}

function storedMaterializedState(
  records: Map<string, { storageKey?: string }>,
): MaterializedWorkOutcomeState {
  const envelope = [...records.values()][0] as StoredEnvelope | undefined;
  if (!envelope) {
    throw new Error("expected a persisted checkpoint envelope");
  }
  return envelope.checkpoint.materializedWorkOutcomeState;
}
