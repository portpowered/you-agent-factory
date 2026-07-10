import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { FACTORY_EVENT_TYPES, type FactoryEvent } from "../../../api/events";
import { useFactoryTimelineStore } from "../../timeline/state/factoryTimelineStore";
import {
  persistTimelineCheckpoint,
  readTimelineCheckpoint,
  type TimelineCheckpointStreamIdentity,
} from "../../timeline/state/timelineCheckpointPersistence";
import {
  buildWorkOutcomeTimelineSamplesFromEvents,
  useWorkOutcomeChart,
} from "./useWorkOutcomeChart";

describe("buildWorkOutcomeTimelineSamplesFromEvents", () => {
  const baselineEvents = [
    event("run-started", 0, FACTORY_EVENT_TYPES.runRequest, {
      factory: {
        resources: [],
        workTypes: [
          {
            name: "story",
            states: [
              { name: "init", type: "INITIAL" },
              { name: "done", type: "TERMINAL" },
              { name: "failed", type: "FAILED" },
            ],
          },
          {
            name: "__system_time",
            states: [
              { name: "pending", type: "INITIAL" },
              { name: "expired", type: "TERMINAL" },
            ],
          },
        ],
        workers: [],
        workstations: [],
      },
      recordedAt: "2026-04-29T12:00:00Z",
    }),
    event("work-request", 1, FACTORY_EVENT_TYPES.workRequest, {
      type: "FACTORY_REQUEST_BATCH",
      works: [
        {
          name: "Story One",
          traceId: "trace-1",
          workId: "work-1",
          workTypeName: "story",
        },
      ],
    }),
    event("system-time-work-request", 1, FACTORY_EVENT_TYPES.workRequest, {
      type: "FACTORY_REQUEST_BATCH",
      works: [
        {
          name: "System clock",
          traceId: "trace-system-time",
          workId: "work-system-time",
          workTypeName: "__system_time",
        },
      ],
    }),
    event(
      "dispatch-request",
      2,
      FACTORY_EVENT_TYPES.dispatchRequest,
      {
        inputs: [{ workId: "work-1" }],
        transitionId: "review",
      },
      {
        dispatchId: "dispatch-1",
      },
    ),
    event(
      "system-time-dispatch-request",
      2,
      FACTORY_EVENT_TYPES.dispatchRequest,
      {
        inputs: [{ workId: "work-system-time" }],
        transitionId: "__system_time:expire",
      },
      {
        dispatchId: "dispatch-system-time",
      },
    ),
    event(
      "dispatch-response",
      3,
      FACTORY_EVENT_TYPES.dispatchResponse,
      {
        durationMillis: 100,
        outcome: "ACCEPTED",
        outputWork: [
          {
            name: "Story One",
            state: "done",
            traceId: "trace-1",
            workId: "work-1",
            workTypeName: "story",
          },
        ],
        transitionId: "review",
      },
      {
        dispatchId: "dispatch-1",
      },
    ),
    event(
      "system-time-dispatch-response",
      3,
      FACTORY_EVENT_TYPES.dispatchResponse,
      {
        durationMillis: 10,
        outcome: "ACCEPTED",
        outputWork: [
          {
            name: "System clock",
            state: "expired",
            traceId: "trace-system-time",
            workId: "work-system-time",
            workTypeName: "__system_time",
          },
        ],
        transitionId: "__system_time:expire",
      },
      {
        dispatchId: "dispatch-system-time",
      },
    ),
    event("work-request-2", 4, FACTORY_EVENT_TYPES.workRequest, {
      type: "FACTORY_REQUEST_BATCH",
      works: [
        {
          name: "Story Two",
          traceId: "trace-2",
          workId: "work-2",
          workTypeName: "story",
        },
      ],
    }),
    event(
      "dispatch-request-2",
      5,
      FACTORY_EVENT_TYPES.dispatchRequest,
      {
        inputs: [{ workId: "work-2" }],
        transitionId: "review",
      },
      {
        dispatchId: "dispatch-2",
      },
    ),
    event(
      "dispatch-response-2",
      6,
      FACTORY_EVENT_TYPES.dispatchResponse,
      {
        durationMillis: 100,
        failureMessage: "Rejected",
        failureReason: "review failed",
        outcome: "FAILED",
        outputWork: [
          {
            name: "Story Two",
            state: "failed",
            traceId: "trace-2",
            workId: "work-2",
            workTypeName: "story",
          },
        ],
        transitionId: "review",
      },
      {
        dispatchId: "dispatch-2",
      },
    ),
  ] satisfies FactoryEvent[];

  const tailEvent = event(
    "work-request-3",
    7,
    FACTORY_EVENT_TYPES.workRequest,
    {
      type: "FACTORY_REQUEST_BATCH",
      works: [
        {
          name: "Story Three",
          traceId: "trace-3",
          workId: "work-3",
          workTypeName: "story",
        },
      ],
    },
  );

  afterEach(() => {
    useFactoryTimelineStore.getState().reset();
  });

  it("projects the exact uninterrupted customer work-outcome series", () => {
    const samples = buildWorkOutcomeTimelineSamplesFromEvents(
      baselineEvents,
      6,
    );

    expect(samples).toEqual([
      {
        completedCount: 0,
        dispatchedCount: 0,
        failedByWorkType: {},
        failedCount: 0,
        failedWorkLabels: [],
        inFlightCount: 0,
        observedAt: 1777464000000,
        queuedCount: 0,
        tick: 0,
      },
      {
        completedCount: 0,
        dispatchedCount: 0,
        failedByWorkType: {},
        failedCount: 0,
        failedWorkLabels: [],
        inFlightCount: 0,
        observedAt: 1777464001000,
        queuedCount: 1,
        tick: 1,
      },
      {
        completedCount: 0,
        dispatchedCount: 1,
        failedByWorkType: {},
        failedCount: 0,
        failedWorkLabels: [],
        inFlightCount: 1,
        observedAt: 1777464002000,
        queuedCount: 0,
        tick: 2,
      },
      {
        completedCount: 1,
        dispatchedCount: 1,
        failedByWorkType: {},
        failedCount: 0,
        failedWorkLabels: [],
        inFlightCount: 0,
        observedAt: 1777464003000,
        queuedCount: 0,
        tick: 3,
      },
      {
        completedCount: 1,
        dispatchedCount: 1,
        failedByWorkType: {},
        failedCount: 0,
        failedWorkLabels: [],
        inFlightCount: 0,
        observedAt: 1777464004000,
        queuedCount: 1,
        tick: 4,
      },
      {
        completedCount: 1,
        dispatchedCount: 2,
        failedByWorkType: {},
        failedCount: 0,
        failedWorkLabels: [],
        inFlightCount: 1,
        observedAt: 1777464005000,
        queuedCount: 0,
        tick: 5,
      },
      {
        completedCount: 1,
        dispatchedCount: 2,
        failedByWorkType: { story: 1 },
        failedCount: 1,
        failedWorkLabels: ["Story Two"],
        inFlightCount: 0,
        observedAt: 1777464006000,
        queuedCount: 0,
        tick: 6,
      },
    ]);
  });

  it("characterizes checkpoint restore, tail replacement, and duplicate delivery", async () => {
    const { indexedDB } = createIndexedDBTestDouble();
    const streamIdentity = streamIdentityFixture();

    useFactoryTimelineStore.getState().appendEvents(baselineEvents);
    const uninterrupted = renderHook(() => useTimelineWorkOutcomeChart());
    expect(
      uninterrupted.result.current.samples.map((sample) => sample.tick),
    ).toEqual([0, 1, 2, 3, 4, 5, 6]);

    const baselineCheckpoint =
      useFactoryTimelineStore.getState().currentReplayCheckpoint;
    expect(baselineCheckpoint?.selectedTick).toBe(6);
    await persistTimelineCheckpoint(
      indexedDB,
      baselineCheckpoint,
      streamIdentity,
    );

    act(() => useFactoryTimelineStore.getState().reset());
    const restoredCheckpoint = await readTimelineCheckpoint(
      indexedDB,
      streamIdentity,
    );
    expect(restoredCheckpoint?.afterEventId).toBe("dispatch-response-2");
    act(() => {
      if (restoredCheckpoint) {
        useFactoryTimelineStore
          .getState()
          .restoreCheckpoint(restoredCheckpoint);
      }
    });

    const restored = renderHook(() => useTimelineWorkOutcomeChart());
    expect(restored.result.current.samples).toEqual([
      {
        completedCount: 1,
        dispatchedCount: 2,
        failedByWorkType: { story: 1 },
        failedCount: 1,
        failedWorkLabels: ["Story Two"],
        inFlightCount: 0,
        observedAt: 0,
        queuedCount: 0,
        tick: 6,
      },
    ]);

    act(() => useFactoryTimelineStore.getState().appendEvent(tailEvent));
    expect(restored.result.current.samples).toEqual([
      {
        completedCount: 0,
        dispatchedCount: 0,
        failedByWorkType: {},
        failedCount: 0,
        failedWorkLabels: [],
        inFlightCount: 0,
        observedAt: 1777464007000,
        queuedCount: 0,
        tick: 7,
      },
    ]);
    expect(
      [0, 1, 2, 3, 4, 5, 6].filter(
        (tick) =>
          !restored.result.current.samples.some(
            (sample) => sample.tick === tick,
          ),
      ),
    ).toEqual([0, 1, 2, 3, 4, 5, 6]);

    const samplesAfterFirstTail = restored.result.current.samples;
    act(() => useFactoryTimelineStore.getState().appendEvent(tailEvent));
    expect(restored.result.current.samples).toEqual(samplesAfterFirstTail);
    expect(
      useFactoryTimelineStore
        .getState()
        .events.filter((timelineEvent) => timelineEvent.id === tailEvent.id),
    ).toHaveLength(1);
  });

  it.fails("preserves the uninterrupted series across checkpoint restore and one tail event", async () => {
    const { indexedDB } = createIndexedDBTestDouble();
    const streamIdentity = streamIdentityFixture();
    useFactoryTimelineStore.getState().appendEvents(baselineEvents);
    const checkpoint =
      useFactoryTimelineStore.getState().currentReplayCheckpoint;
    await persistTimelineCheckpoint(indexedDB, checkpoint, streamIdentity);
    useFactoryTimelineStore.getState().reset();
    const restoredCheckpoint = await readTimelineCheckpoint(
      indexedDB,
      streamIdentity,
    );
    if (!restoredCheckpoint) {
      throw new Error("expected the persisted baseline checkpoint to restore");
    }
    useFactoryTimelineStore.getState().restoreCheckpoint(restoredCheckpoint);

    const repaired = renderHook(() => useTimelineWorkOutcomeChart());
    act(() => useFactoryTimelineStore.getState().appendEvent(tailEvent));
    act(() => useFactoryTimelineStore.getState().appendEvent(tailEvent));

    const uninterruptedThroughTail = buildWorkOutcomeTimelineSamplesFromEvents(
      [...baselineEvents, tailEvent],
      7,
    );
    expect(repaired.result.current.samples).toEqual(uninterruptedThroughTail);
  });
});

function useTimelineWorkOutcomeChart() {
  const events = useFactoryTimelineStore((state) => state.events);
  const selectedTick = useFactoryTimelineStore((state) => state.selectedTick);
  const worldViewCache = useFactoryTimelineStore(
    (state) => state.worldViewCache,
  );
  return useWorkOutcomeChart({
    selectedTimelineTick: selectedTick,
    timelineEvents: events,
    worldViewCache,
  });
}

function event(
  id: string,
  tick: number,
  type: FactoryEvent["type"],
  payload: FactoryEvent["payload"],
  context: Partial<FactoryEvent["context"]> = {},
): FactoryEvent {
  return {
    context: {
      eventTime: `2026-04-29T12:00:0${tick}Z`,
      sequence: tick,
      tick,
      ...context,
    },
    id,
    payload,
    type,
  };
}

function streamIdentityFixture(): TimelineCheckpointStreamIdentity {
  return {
    backendScopeID: "backend-scope-a",
    factorySessionID: "a1b2c3d4-e5f6-4789-a012-3456789abcde",
    logicalSessionKeyID: "logical-session-a",
    streamGenerationID: "2026-04-29T12:00:00Z",
  };
}

function createIndexedDBTestDouble() {
  const records = new Map<string, unknown>();
  const database = {
    close: () => {},
    createObjectStore: () => {},
    deleteObjectStore: () => {},
    objectStoreNames: { contains: () => true },
    transaction: () => ({
      objectStore: () => ({
        delete: (key: string) =>
          indexedDBRequest(undefined, () => records.delete(key)),
        get: (key: string) => indexedDBRequest(records.get(key)),
        put: (value: { storageKey: string }) =>
          indexedDBRequest(value.storageKey, () =>
            records.set(value.storageKey, value),
          ),
      }),
    }),
  };

  return {
    indexedDB: {
      open: () => indexedDBRequest(database),
    } as unknown as IDBFactory,
  };
}

function indexedDBRequest<T>(result: T, beforeSuccess?: () => void) {
  const request = {
    error: null,
    onblocked: null,
    onerror: null,
    onsuccess: null,
    onupgradeneeded: null,
    result,
  } as unknown as IDBRequest<T>;
  queueMicrotask(() => {
    beforeSuccess?.();
    request.onsuccess?.({} as Event);
  });
  return request;
}
