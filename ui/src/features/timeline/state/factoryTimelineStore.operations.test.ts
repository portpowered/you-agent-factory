// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: timeline store operation and session lifecycle replay cases share fixture events.
import type { FactoryEvent } from "../../../api/events";
import { FACTORY_EVENT_TYPES } from "../../../api/events";
import {
  canonicalSessionLifecycleControlReplayEvents,
  canonicalSessionLifecycleReplayEvents,
} from "../../../testing/session-lifecycle-replay-fixtures";
import {
  buildFactoryTimelineSnapshot,
  useFactoryTimelineStore,
} from "./factoryTimelineStore";

const eventTime = "2026-05-31T12:00:00.000Z";
const exactIdentity = {
  backendScopeID: "timeline-ops-backend",
  factorySessionID: "11111111-1111-4111-8111-111111111111",
  logicalSessionKeyID: "timeline-ops-logical-session",
  streamGenerationID: "timeline-ops-generation",
};

function timelineEvent(
  id: string,
  tick: number,
  type: FactoryEvent["type"],
  payload: FactoryEvent["payload"],
): FactoryEvent {
  return {
    context: {
      eventTime,
      sequence: tick,
      tick,
    },
    id,
    payload,
    type,
  };
}

const initialStructure = timelineEvent(
  "timeline-ops-initial",
  1,
  FACTORY_EVENT_TYPES.initialStructureRequest,
  {
    factory: {
      workers: [],
      workTypes: [
        {
          name: "task",
          states: [
            { name: "init", type: "INITIAL" },
            { name: "review", type: "PROCESSING" },
          ],
        },
      ],
      workstations: [],
    },
  },
);

const workRequest = timelineEvent(
  "timeline-ops-work",
  2,
  FACTORY_EVENT_TYPES.workRequest,
  {
    type: "FACTORY_REQUEST_BATCH",
    works: [
      {
        name: "Ops Story",
        trace_id: "trace-ops",
        work_id: "work-ops",
        work_type_id: "task",
      },
    ],
  },
);

describe("factory timeline store operations", () => {
  afterEach(() => {
    useFactoryTimelineStore.getState().reset();
  });

  it("replaceEvents rebuilds timeline state in current mode", () => {
    const store = useFactoryTimelineStore.getState();
    store.appendEvents([initialStructure, workRequest]);
    store.selectTick(1);

    store.replaceEvents([initialStructure]);

    const state = useFactoryTimelineStore.getState();
    expect(state.mode).toBe("current");
    expect(state.events).toHaveLength(1);
    expect(state.events[0]?.id).toBe("timeline-ops-initial");
    expect(state.latestTick).toBe(1);
    expect(state.selectedTick).toBe(1);
    expect(state.receivedEventIDs).toEqual(["timeline-ops-initial"]);
    expect(state.worldViewCache[1]).toBeDefined();
    expect(state.worldViewCache[2]).toBeUndefined();
  });

  it("ignores duplicate event ids on appendEvents", () => {
    const store = useFactoryTimelineStore.getState();
    store.appendEvents([initialStructure, workRequest]);

    const before = useFactoryTimelineStore.getState();
    store.appendEvents([initialStructure, workRequest]);

    const after = useFactoryTimelineStore.getState();
    expect(after.events).toEqual(before.events);
    expect(after.latestTick).toBe(before.latestTick);
    expect(after.selectedTick).toBe(before.selectedTick);
    expect(after.receivedEventIDs).toEqual(before.receivedEventIDs);
    expect(after.worldViewCache).toEqual(before.worldViewCache);
  });

  it("ignores duplicate event ids on appendEvent", () => {
    const store = useFactoryTimelineStore.getState();
    store.appendEvent(initialStructure);

    const before = useFactoryTimelineStore.getState();
    store.appendEvent(initialStructure);

    const after = useFactoryTimelineStore.getState();
    expect(after.events).toEqual(before.events);
    expect(after.latestTick).toBe(before.latestTick);
    expect(after.receivedEventIDs).toEqual(before.receivedEventIDs);
  });

  it("does not replace retained live entry state with a durable checkpoint", () => {
    const store = useFactoryTimelineStore.getState();
    store.activateEntry(exactIdentity);
    store.appendEventsForEntry(exactIdentity, [initialStructure]);
    const liveEntry = useFactoryTimelineStore
      .getState()
      .entryForIdentity(exactIdentity);
    const checkpoint = liveEntry?.currentReplayCheckpoint;
    expect(checkpoint).toBeDefined();
    if (!checkpoint) throw new Error("expected live timeline checkpoint");

    store.restoreCheckpointForEntry(exactIdentity, structuredClone(checkpoint));

    const retained = useFactoryTimelineStore
      .getState()
      .entryForIdentity(exactIdentity);
    expect(retained?.events.map(({ id }) => id)).toEqual([
      "timeline-ops-initial",
    ]);
    expect(retained?.receivedEventIDs).toEqual(["timeline-ops-initial"]);
  });

  it("preserves selected tick in fixed mode when appending later events", () => {
    const store = useFactoryTimelineStore.getState();
    store.appendEvents([initialStructure, workRequest]);
    store.selectTick(1);

    const workStateChange = timelineEvent(
      "timeline-ops-move",
      3,
      FACTORY_EVENT_TYPES.workStateChange,
      {
        from_place_id: "init",
        from_state: "init",
        source: "api",
        to_place_id: "review",
        to_state: "review",
        work_id: "work-ops",
        work_type_name: "task",
      },
    );
    store.appendEvent(workStateChange);

    const state = useFactoryTimelineStore.getState();
    expect(state.mode).toBe("fixed");
    expect(state.selectedTick).toBe(1);
    expect(state.latestTick).toBe(3);
    expect(state.events).toHaveLength(3);
  });

  it("projects operator move history at the latest tick without breaking dispatch projection", () => {
    const store = useFactoryTimelineStore.getState();
    const workStateChange = timelineEvent(
      "timeline-ops-move",
      3,
      FACTORY_EVENT_TYPES.workStateChange,
      {
        fromPlaceId: "task:init",
        fromState: "init",
        source: "api",
        toPlaceId: "task:review",
        toState: "review",
        workId: "work-ops",
        workTypeName: "task",
      },
    );
    store.appendEvents([initialStructure, workRequest, workStateChange]);

    const snapshot = buildFactoryTimelineSnapshot(
      useFactoryTimelineStore.getState().events,
      3,
    );

    expect(
      snapshot.runtime.work_move_operations_by_work_id?.["work-ops"],
    ).toEqual([
      expect.objectContaining({
        work_id: "work-ops",
        from_state: "init",
        to_state: "review",
        source: "api",
        tick: 3,
      }),
    ]);
    expect(
      snapshot.runtime.workstation_requests_by_dispatch_id,
    ).toBeUndefined();
    expect(snapshot.runtime.session.has_data).toBe(true);
  });

  it("setCurrentMode follows the latest tick after scrubbing", () => {
    const store = useFactoryTimelineStore.getState();
    store.appendEvents([initialStructure, workRequest]);
    store.selectTick(1);
    store.setCurrentMode();

    const state = useFactoryTimelineStore.getState();
    expect(state.mode).toBe("current");
    expect(state.selectedTick).toBe(2);
    expect(state.worldViewCache[2]).toBeDefined();
  });

  it("rebuilds paused and resumed session bracket lifecycle from canonical replay events", () => {
    const store = useFactoryTimelineStore.getState();
    store.replaceEvents([...canonicalSessionLifecycleReplayEvents]);

    const pausedSnapshot = buildFactoryTimelineSnapshot(
      useFactoryTimelineStore.getState().events,
      2,
    );
    expect(pausedSnapshot.runtime.session.bracket).toMatchObject({
      lifecycle_control_status: "PAUSED",
      paused_at: "2026-06-09T12:00:02Z",
      session_id: "session-alpha",
    });

    const runningSnapshot = buildFactoryTimelineSnapshot(
      useFactoryTimelineStore.getState().events,
      3,
    );
    expect(runningSnapshot.runtime.session.bracket).toMatchObject({
      lifecycle_control_status: "RUNNING",
      paused_at: "2026-06-09T12:00:02Z",
      resumed_at: "2026-06-09T12:00:04Z",
      session_id: "session-alpha",
    });
    expect(runningSnapshot.runtime.session.has_data).toBe(true);
  });

  it("rebuilds paused and resumed session bracket lifecycle from SESSION_LIFECYCLE_CONTROL replay events", () => {
    const store = useFactoryTimelineStore.getState();
    store.replaceEvents([...canonicalSessionLifecycleControlReplayEvents]);

    const pausedSnapshot = buildFactoryTimelineSnapshot(
      useFactoryTimelineStore.getState().events,
      2,
    );
    expect(pausedSnapshot.runtime.session.bracket).toMatchObject({
      lifecycle_control_status: "PAUSED",
      paused_at: "2026-06-09T12:00:02Z",
      session_id: "session-alpha",
    });

    const runningSnapshot = buildFactoryTimelineSnapshot(
      useFactoryTimelineStore.getState().events,
      3,
    );
    expect(runningSnapshot.runtime.session.bracket).toMatchObject({
      lifecycle_control_status: "RUNNING",
      paused_at: "2026-06-09T12:00:02Z",
      resumed_at: "2026-06-09T12:00:04Z",
      session_id: "session-alpha",
    });
  });
});
