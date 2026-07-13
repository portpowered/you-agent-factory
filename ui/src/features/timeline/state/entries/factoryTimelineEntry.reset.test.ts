import { FACTORY_EVENT_TYPES, type FactoryEvent } from "../../../../api/events";
import type { StreamDerivedCacheIdentity } from "../../lib/stream-derived-cache-identity";
import { useFactoryTimelineStore } from "../factoryTimelineStore";

function identity(
  factorySessionID: string,
  streamGenerationID: string,
): StreamDerivedCacheIdentity {
  return {
    backendScopeID: "backend-a",
    factorySessionID,
    logicalSessionKeyID: `logical-${factorySessionID}`,
    streamGenerationID,
  };
}

function event(
  id: string,
  tick: number,
  type: FactoryEvent["type"],
  payload: FactoryEvent["payload"],
  dispatchID?: string,
): FactoryEvent {
  return {
    context: {
      dispatchId: dispatchID,
      eventTime: `2026-07-13T15:00:0${tick}.000Z`,
      sequence: tick,
      tick,
    },
    id,
    payload,
    type,
  };
}

const initialStructure = event(
  "reset-initial",
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

const workRequest = event("reset-work", 2, FACTORY_EVENT_TYPES.workRequest, {
  source: "api",
  type: "FACTORY_REQUEST_BATCH",
  works: [
    {
      name: "Reset Story",
      traceId: "trace-reset",
      workId: "work-reset",
      workTypeName: "story",
    },
  ],
});

const dispatchRequest = event(
  "reset-dispatch",
  3,
  FACTORY_EVENT_TYPES.dispatchRequest,
  {
    inputs: [{ workId: "work-reset" }],
    transitionId: "review",
  },
  "dispatch-reset",
);

const fullHistory = [initialStructure, workRequest, dispatchRequest];

function entry(streamIdentity: StreamDerivedCacheIdentity) {
  return useFactoryTimelineStore.getState().entryForIdentity(streamIdentity);
}

afterEach(() => {
  useFactoryTimelineStore.getState().reset();
});

describe("factory timeline entry reset", () => {
  it("resets only the targeted entry to complete empty state", () => {
    const sessionA = identity("session-a-uuid", "generation-1");
    const sessionB = identity("session-b-uuid", "generation-1");
    const store = useFactoryTimelineStore.getState();
    store.appendEventsForEntry(sessionA, fullHistory);
    store.selectTickForEntry(sessionA, 1);
    store.activateEntry(sessionA);
    store.appendEventsForEntry(sessionB, [initialStructure, workRequest]);
    const sessionBBeforeReset = entry(sessionB);
    const materializedBeforeReset =
      entry(sessionA)?.materializedWorkOutcomeState;

    store.resetEntry(sessionA);

    const reset = entry(sessionA);
    expect(reset).toMatchObject({
      currentReplayCheckpoint: undefined,
      events: [],
      identity: sessionA,
      latestTick: 0,
      mode: "current",
      receivedEventIDs: [],
      selectedTick: 0,
    });
    expect(Object.keys(reset?.worldViewCache ?? {})).toEqual(["0"]);
    expect(reset?.materializedWorkOutcomeState).toEqual({
      accumulator: {
        activeDispatchesByID: {},
        appliedEventCount: 0,
        completedAcceptedCount: 0,
        completedDispatchCount: 0,
        failedWorkItemsByID: {},
        initialPlaceIDs: [],
        workItemsByID: {},
      },
      counts: {
        completed: 0,
        dispatched: 0,
        failed: 0,
        inFlight: 0,
        queued: 0,
      },
      cursor: null,
      failedByWorkType: {},
      failedWorkLabels: [],
      samples: [],
      version: 1,
    });
    expect(reset?.materializedWorkOutcomeState).not.toBe(
      materializedBeforeReset,
    );
    expect(entry(sessionB)).toBe(sessionBBeforeReset);
    expect(useFactoryTimelineStore.getState().events).toEqual([]);
    expect(
      useFactoryTimelineStore.getState().materializedWorkOutcomeState,
    ).toBe(reset?.materializedWorkOutcomeState);
  });
});

describe("factory timeline full-history replacement", () => {
  it("rebuilds full history like uninterrupted append without inherited state", () => {
    const stream = identity("session-a-uuid", "generation-1");
    const reference = identity("session-reference-uuid", "generation-1");
    const store = useFactoryTimelineStore.getState();
    store.appendEventsForEntry(reference, fullHistory);
    store.appendEventsForEntry(stream, [
      initialStructure,
      workRequest,
      dispatchRequest,
      event(
        "old-response",
        4,
        FACTORY_EVENT_TYPES.dispatchResponse,
        {
          durationMillis: 10,
          outcome: "ACCEPTED",
          outputWork: [],
          transitionId: "review",
        },
        "dispatch-reset",
      ),
    ]);
    const inheritedAccumulator = entry(stream)?.materializedWorkOutcomeState;

    store.replaceEventsForEntry(stream, [
      dispatchRequest,
      initialStructure,
      workRequest,
      workRequest,
    ]);

    const replaced = entry(stream);
    const uninterrupted = entry(reference);
    expect(replaced?.events.map(({ id }) => id)).toEqual(
      uninterrupted?.events.map(({ id }) => id),
    );
    expect(replaced?.receivedEventIDs).toEqual(uninterrupted?.receivedEventIDs);
    expect(replaced?.currentReplayCheckpoint).toEqual(
      uninterrupted?.currentReplayCheckpoint,
    );
    expect(replaced?.materializedWorkOutcomeState).toEqual(
      uninterrupted?.materializedWorkOutcomeState,
    );
    expect(replaced?.materializedWorkOutcomeState).not.toBe(
      inheritedAccumulator,
    );
    expect(replaced).toMatchObject({
      latestTick: 3,
      mode: "current",
      selectedTick: 3,
    });
  });
});

describe("factory timeline stream-generation replacement", () => {
  it("keeps replacement generations empty under late old-generation callbacks", () => {
    const oldGeneration = identity("session-a-uuid", "generation-1");
    const newGeneration = identity("session-a-uuid", "generation-2");
    const store = useFactoryTimelineStore.getState();
    store.appendEventsForEntry(oldGeneration, fullHistory);
    store.activateEntry(newGeneration);
    const replacementBeforeLateCallback = entry(newGeneration);

    store.appendEventForEntry(
      oldGeneration,
      event(
        "late-old-generation",
        4,
        FACTORY_EVENT_TYPES.dispatchResponse,
        {
          durationMillis: 10,
          outcome: "ACCEPTED",
          outputWork: [],
          transitionId: "review",
        },
        "dispatch-reset",
      ),
    );

    expect(entry(newGeneration)).toBe(replacementBeforeLateCallback);
    expect(entry(newGeneration)).toMatchObject({
      currentReplayCheckpoint: undefined,
      events: [],
      latestTick: 0,
      mode: "current",
      receivedEventIDs: [],
      selectedTick: 0,
    });
    expect(
      entry(newGeneration)?.materializedWorkOutcomeState.cursor,
    ).toBeNull();
    expect(entry(newGeneration)?.materializedWorkOutcomeState.counts).toEqual({
      completed: 0,
      dispatched: 0,
      failed: 0,
      inFlight: 0,
      queued: 0,
    });
    expect(entry(oldGeneration)?.events.at(-1)?.id).toBe("late-old-generation");
  });
});
