import { FACTORY_EVENT_TYPES, type FactoryEvent } from "../../../../api/events";
import type { StreamDerivedCacheIdentity } from "../../lib/stream-derived-cache-identity";
import { createTimelineCheckpointIndexedDBTestDouble } from "../../../../testing/timeline-checkpoint-indexeddb-test-utils";
import { useFactoryTimelineStore } from "../factoryTimelineStore";
import {
  persistTimelineCheckpoint,
  readTimelineCheckpoint,
} from "../timelineCheckpointPersistence";

const identity: StreamDerivedCacheIdentity = {
  backendScopeID: "backend-a",
  factorySessionID: "99999999-9999-4999-8999-999999999999",
  logicalSessionKeyID: "logical-session-a",
  streamGenerationID: "generation-1",
};

function event(
  id: string,
  tick: number,
  sequence: number,
  type: FactoryEvent["type"],
  payload: FactoryEvent["payload"],
  dispatchID?: string,
): FactoryEvent {
  return {
    context: {
      dispatchId: dispatchID,
      eventTime: `2026-07-13T12:00:0${tick}.000Z`,
      sequence,
      tick,
    },
    id,
    payload,
    type,
  };
}

const initialStructure = event(
  "event-initial",
  1,
  1,
  FACTORY_EVENT_TYPES.initialStructureRequest,
  {
    factory: {
      workers: [],
      workTypes: [
        {
          name: "story",
          states: [
            { name: "ready", type: "INITIAL" },
            { name: "done", type: "TERMINAL" },
          ],
        },
      ],
      workstations: [],
    },
  },
);

const workRequest = event("event-work", 2, 2, FACTORY_EVENT_TYPES.workRequest, {
  source: "api",
  type: "FACTORY_REQUEST_BATCH",
  works: [
    {
      name: "Timeline Story",
      traceId: "trace-1",
      workId: "work-1",
      workTypeName: "story",
    },
  ],
});

const dispatchRequest = event(
  "event-dispatch",
  3,
  3,
  FACTORY_EVENT_TYPES.dispatchRequest,
  {
    inputs: [{ workId: "work-1" }],
    transitionId: "review",
  },
  "dispatch-1",
);

const dispatchResponse = event(
  "event-response",
  4,
  4,
  FACTORY_EVENT_TYPES.dispatchResponse,
  {
    durationMillis: 10,
    outcome: "ACCEPTED",
    outputWork: [
      {
        name: "Timeline Story",
        state: "done",
        traceId: "trace-1",
        workId: "work-1",
        workTypeName: "story",
      },
    ],
    transitionId: "review",
  },
  "dispatch-1",
);

const reconnectSuffix = event(
  "event-work-2",
  5,
  5,
  FACTORY_EVENT_TYPES.workRequest,
  {
    source: "api",
    type: "FACTORY_REQUEST_BATCH",
    works: [
      {
        name: "Reconnect Suffix Story",
        traceId: "trace-2",
        workId: "work-2",
        workTypeName: "story",
      },
    ],
  },
);

function entry() {
  return useFactoryTimelineStore.getState().entryForIdentity(identity);
}

afterEach(() => {
  useFactoryTimelineStore.getState().reset();
});

describe("factory timeline ordered outcome append", () => {
  it("publishes replay and outcomes from one ordered, deduplicated tail", () => {
    const observedBoundaries: Array<[string | undefined, string | undefined]> =
      [];
    useFactoryTimelineStore.getState().activateEntry(identity);
    const unsubscribe = useFactoryTimelineStore.subscribe((state) => {
      observedBoundaries.push([
        state.currentReplayCheckpoint?.afterEventId,
        state.materializedWorkOutcomeState.cursor?.eventID,
      ]);
    });

    useFactoryTimelineStore
      .getState()
      .appendEventsForEntry(identity, [
        dispatchRequest,
        workRequest,
        initialStructure,
        workRequest,
      ]);
    unsubscribe();

    const current = entry();
    expect(current?.events.map(({ id }) => id)).toEqual([
      "event-initial",
      "event-work",
      "event-dispatch",
    ]);
    expect(current?.receivedEventIDs).toEqual([
      "event-initial",
      "event-work",
      "event-dispatch",
    ]);
    expect(current?.currentReplayCheckpoint).toMatchObject({
      afterEventId: "event-dispatch",
      afterSequence: 3,
      selectedTick: 3,
    });
    expect(
      current?.currentReplayCheckpoint?.replayState.activeDispatches[
        "dispatch-1"
      ],
    ).toBeDefined();
    expect(current?.materializedWorkOutcomeState).toMatchObject({
      accumulator: { appliedEventCount: 3 },
      counts: {
        completed: 0,
        dispatched: 1,
        failed: 0,
        inFlight: 1,
        queued: 0,
      },
      cursor: { eventID: "event-dispatch", sequence: 3, tick: 3 },
    });
    expect(observedBoundaries).toEqual([["event-dispatch", "event-dispatch"]]);
  });

  it("returns the existing entry for empty and duplicate-only batches", () => {
    const store = useFactoryTimelineStore.getState();
    store.appendEventsForEntry(identity, [initialStructure, workRequest]);
    const before = entry();

    store.appendEventsForEntry(identity, []);
    expect(entry()).toBe(before);

    store.appendEventsForEntry(identity, [workRequest, initialStructure]);
    expect(entry()).toBe(before);
  });
});

describe("factory timeline persisted append continuation", () => {
  it("restores one persisted boundary and applies only a reconnect suffix", async () => {
    const store = useFactoryTimelineStore.getState();
    store.appendEventsForEntry(identity, [
      initialStructure,
      workRequest,
      dispatchRequest,
      dispatchResponse,
    ]);
    const checkpoint = entry()?.currentReplayCheckpoint;
    if (!checkpoint) {
      throw new Error("expected checkpoint after initial timeline append");
    }
    const { indexedDB } = createTimelineCheckpointIndexedDBTestDouble();
    await persistTimelineCheckpoint(indexedDB, checkpoint, identity);

    store.resetEntry(identity);
    const restored = await readTimelineCheckpoint(indexedDB, identity);
    if (!restored) {
      throw new Error("expected persisted timeline checkpoint");
    }
    store.restoreCheckpointForEntry(identity, restored);
    const beforeReconnect = entry();

    store.appendEventsForEntry(identity, [
      dispatchRequest,
      dispatchResponse,
      reconnectSuffix,
    ]);

    const current = entry();
    expect(
      beforeReconnect?.currentReplayCheckpoint?.replayState.completedDispatches,
    ).toHaveLength(1);
    expect(beforeReconnect?.materializedWorkOutcomeState).toMatchObject({
      accumulator: { appliedEventCount: 4, completedDispatchCount: 1 },
      counts: { completed: 1, dispatched: 1 },
      cursor: { eventID: "event-response", sequence: 4, tick: 4 },
    });
    expect(current?.events.map(({ id }) => id)).toEqual(["event-work-2"]);
    expect(current?.receivedEventIDs).toEqual(["event-work-2"]);
    expect(
      current?.currentReplayCheckpoint?.replayState.completedDispatches,
    ).toHaveLength(1);
    expect(current?.currentReplayCheckpoint).toMatchObject({
      afterEventId: "event-work-2",
      afterSequence: 5,
      selectedTick: 5,
    });
    expect(current?.materializedWorkOutcomeState).toMatchObject({
      accumulator: {
        appliedEventCount: 5,
        completedAcceptedCount: 1,
        completedDispatchCount: 1,
      },
      counts: {
        completed: 1,
        dispatched: 1,
        failed: 0,
        inFlight: 0,
        queued: 1,
      },
      cursor: { eventID: "event-work-2", sequence: 5, tick: 5 },
    });
    expect(current?.materializedWorkOutcomeState.samples).toHaveLength(5);
    expect(current?.currentReplayCheckpoint?.afterEventId).toBe(
      current?.materializedWorkOutcomeState.cursor?.eventID,
    );
  });
});

describe("factory timeline append continuation", () => {
  it("applies only the new suffix from reconnect overlap", () => {
    const store = useFactoryTimelineStore.getState();
    store.appendEventsForEntry(identity, [
      initialStructure,
      workRequest,
      dispatchRequest,
    ]);

    store.appendEventsForEntry(identity, [
      workRequest,
      dispatchRequest,
      dispatchResponse,
      dispatchResponse,
    ]);

    const current = entry();
    expect(current?.events).toHaveLength(4);
    expect(current?.currentReplayCheckpoint).toMatchObject({
      afterEventId: "event-response",
      afterSequence: 4,
      selectedTick: 4,
    });
    expect(
      current?.currentReplayCheckpoint?.replayState.completedDispatches,
    ).toHaveLength(1);
    expect(current?.materializedWorkOutcomeState).toMatchObject({
      accumulator: {
        appliedEventCount: 4,
        completedAcceptedCount: 1,
        completedDispatchCount: 1,
      },
      counts: {
        completed: 1,
        dispatched: 1,
        failed: 0,
        inFlight: 0,
        queued: 0,
      },
      cursor: { eventID: "event-response", sequence: 4, tick: 4 },
    });
  });

  it("keeps historical selection separate while live replay and outcomes advance", () => {
    const store = useFactoryTimelineStore.getState();
    store.appendEventsForEntry(identity, [initialStructure, workRequest]);
    const materializedBeforeSelection = entry()?.materializedWorkOutcomeState;

    store.selectTickForEntry(identity, 1);
    expect(entry()?.materializedWorkOutcomeState).toBe(
      materializedBeforeSelection,
    );

    store.appendEventForEntry(identity, dispatchRequest);
    const fixed = entry();
    expect(fixed).toMatchObject({ mode: "fixed", selectedTick: 1 });
    expect(fixed?.currentReplayCheckpoint?.afterEventId).toBe("event-dispatch");
    expect(fixed?.materializedWorkOutcomeState.cursor?.eventID).toBe(
      "event-dispatch",
    );
    expect(fixed?.materializedWorkOutcomeState.counts.inFlight).toBe(1);

    const liveMaterialized = fixed?.materializedWorkOutcomeState;
    store.setCurrentModeForEntry(identity);
    expect(entry()).toMatchObject({ mode: "current", selectedTick: 3 });
    expect(entry()?.materializedWorkOutcomeState).toBe(liveMaterialized);
  });

  it("advances both projections for a later event at the current tick", () => {
    const sameTickDispatch = {
      ...dispatchRequest,
      context: { ...dispatchRequest.context, tick: 2 },
    };
    const store = useFactoryTimelineStore.getState();
    store.appendEventsForEntry(identity, [initialStructure, workRequest]);

    store.appendEventForEntry(identity, sameTickDispatch);

    const current = entry();
    expect(
      current?.currentReplayCheckpoint?.replayState.activeDispatches[
        "dispatch-1"
      ],
    ).toBeDefined();
    expect(current?.currentReplayCheckpoint).toMatchObject({
      afterEventId: "event-dispatch",
      afterSequence: 3,
      selectedTick: 2,
    });
    expect(current?.materializedWorkOutcomeState.cursor).toMatchObject({
      eventID: "event-dispatch",
      sequence: 3,
      tick: 2,
    });
  });
});
