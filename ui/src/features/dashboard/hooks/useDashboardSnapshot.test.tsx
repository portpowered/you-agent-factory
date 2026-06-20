import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { PropsWithChildren } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { FACTORY_EVENT_TYPES } from "../../../api/events";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import { createReplayHarness } from "../../../testing/replay-harness";
import {
  type canonicalSessionLifecycleReplayEvents,
  sessionLifecyclePausedEvent,
  sessionLifecycleResumedEvent,
  sessionLifecycleStartedEvent,
} from "../../../testing/session-lifecycle-replay-fixtures";
import {
  FACTORY_TIMELINE_DEBUG_GLOBAL,
  FACTORY_TIMELINE_DEBUG_STORAGE_KEY,
} from "../../timeline/state/factoryTimelineDebug";
import {
  useFactoryTimelineStore,
  type WorldState,
} from "../../timeline/state/factoryTimelineStore";
import { DashboardSessionProvider } from "../session/dashboard-session-provider";
import { useDashboardSessionStore } from "../state/dashboardSessionStore";
import {
  createDefaultDashboardStreamState,
  useDashboardStreamStore,
} from "../state/dashboardStreamStore";
import { useDashboardSnapshot } from "./useDashboardSnapshot";

const replayHarness = createReplayHarness();

const SEEDED_SNAPSHOT: DashboardSnapshot = {
  factory_state: "IDLE",
  runtime: {
    in_flight_dispatch_count: 0,
    session: {
      completed_count: 0,
      dispatched_count: 0,
      failed_count: 0,
      has_data: true,
    },
  },
  tick_count: 3,
  topology: {
    edges: [],
    workstation_node_ids: [],
    workstation_nodes_by_id: {},
  },
  uptime_seconds: 12,
};

const REFRESHED_SNAPSHOT: DashboardSnapshot = {
  ...SEEDED_SNAPSHOT,
  factory_state: "RUNNING",
  tick_count: 1,
  uptime_seconds: 1,
};

function timelineSnapshot(snapshot: DashboardSnapshot): WorldState {
  return {
    ...snapshot,
    relationsByWorkID: {},
    tracesByWorkID: {},
    workstationRequestsByDispatchID: {},
    workRequestsByID: {},
  };
}

describe("useDashboardSnapshot composer", () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    replayHarness.install();
    queryClient = new QueryClient({
      defaultOptions: {
        mutations: { retry: false },
        queries: { retry: false },
      },
    });
    useDashboardStreamStore.setState({
      streamState: createDefaultDashboardStreamState(),
    });
    useDashboardSessionStore.setState({
      pausedSessionIDs: [],
      selectedSessionID: DEFAULT_FACTORY_SESSION_ID,
    });
    useFactoryTimelineStore.setState({
      events: [],
      latestTick: SEEDED_SNAPSHOT.tick_count,
      mode: "current",
      receivedEventIDs: [],
      selectedTick: SEEDED_SNAPSHOT.tick_count,
      worldViewCache: {
        [SEEDED_SNAPSHOT.tick_count]: timelineSnapshot(SEEDED_SNAPSHOT),
      },
    });
  });

  afterEach(() => {
    replayHarness.reset();
    useDashboardStreamStore.setState({
      streamState: createDefaultDashboardStreamState(),
    });
    useDashboardSessionStore.setState({
      pausedSessionIDs: [],
      selectedSessionID: DEFAULT_FACTORY_SESSION_ID,
    });
    useFactoryTimelineStore.getState().reset();
  });

  it("composes lifecycle, stream, and world view on refresh", async () => {
    const { result, rerender } = renderHook(
      ({ refreshToken }: { refreshToken: number }) =>
        useDashboardSnapshot({ refreshToken }),
      {
        initialProps: { refreshToken: 0 },
        wrapper: createWrapper(queryClient),
      },
    );

    expect(result.current.snapshot?.tick_count).toBe(
      SEEDED_SNAPSHOT.tick_count,
    );
    expect(replayHarness.getStreams()).toHaveLength(1);

    act(() => {
      rerender({ refreshToken: 1 });
    });

    await waitFor(() => {
      expect(result.current.isInitialLoading).toBe(true);
    });
    expect(useFactoryTimelineStore.getState().selectedTick).toBe(0);
    expect(replayHarness.getStreams()).toHaveLength(2);

    act(() => {
      replayHarness.emitSnapshot(REFRESHED_SNAPSHOT);
    });

    await waitFor(() => {
      expect(useFactoryTimelineStore.getState().selectedTick).toBe(
        REFRESHED_SNAPSHOT.tick_count,
      );
    });
    expect(result.current.isInitialLoading).toBe(false);
    expect(result.current.error).toBeNull();
  });

  it("reopens the event stream when the selected session tab changes", async () => {
    renderHook(() => useDashboardSnapshot(), {
      wrapper: createWrapper(queryClient),
    });

    expect(replayHarness.getStreams()).toHaveLength(1);
    expect(replayHarness.getStreams()[0]?.url).toBe(
      `/factory-sessions/${DEFAULT_FACTORY_SESSION_ID}/events`,
    );

    act(() => {
      useDashboardSessionStore.getState().setSelectedSessionID("session-beta");
    });

    await waitFor(() => {
      expect(replayHarness.getStreams().length).toBeGreaterThanOrEqual(2);
    });
    expect(replayHarness.getStreams().at(-1)?.url).toBe(
      "/factory-sessions/session-beta/events",
    );
  });

  it("routes streamed events through the composer into timeline state", async () => {
    useFactoryTimelineStore.getState().reset();

    renderHook(() => useDashboardSnapshot(), {
      wrapper: createWrapper(queryClient),
    });

    const stream = replayHarness.getStreams()[0];
    if (!stream) {
      throw new Error("expected dashboard stream to be opened");
    }

    await act(async () => {
      stream.emit("message", {
        context: {
          eventTime: "2026-04-25T20:00:01Z",
          sequence: 1,
          tick: 1,
        },
        id: "event-1",
        payload: {
          factory: {
            workTypes: [
              {
                name: "story",
                states: [{ name: "new", type: "INITIAL" }],
              },
            ],
            workstations: [],
            workers: [],
          },
        },
        type: FACTORY_EVENT_TYPES.initialStructureRequest,
      });
      await new Promise<void>((resolve) => {
        window.setTimeout(() => resolve(), 20);
      });
    });

    await waitFor(() => {
      expect(useFactoryTimelineStore.getState().events).toHaveLength(1);
    });
    expect(useFactoryTimelineStore.getState().latestTick).toBe(1);
    expect(window[FACTORY_TIMELINE_DEBUG_GLOBAL]).toBeUndefined();
    expect(
      window.localStorage.getItem(FACTORY_TIMELINE_DEBUG_STORAGE_KEY),
    ).toBeNull();
  });
});

describe("useDashboardSnapshot session lifecycle replay", () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    replayHarness.install();
    queryClient = new QueryClient({
      defaultOptions: {
        mutations: { retry: false },
        queries: { retry: false },
      },
    });
    useDashboardStreamStore.setState({
      streamState: createDefaultDashboardStreamState(),
    });
    useDashboardSessionStore.setState({
      pausedSessionIDs: [],
      selectedSessionID: DEFAULT_FACTORY_SESSION_ID,
    });
    useFactoryTimelineStore.getState().reset();
  });

  afterEach(() => {
    replayHarness.reset();
    useDashboardStreamStore.setState({
      streamState: createDefaultDashboardStreamState(),
    });
    useDashboardSessionStore.setState({
      pausedSessionIDs: [],
      selectedSessionID: DEFAULT_FACTORY_SESSION_ID,
    });
    useFactoryTimelineStore.getState().reset();
  });

  async function emitStreamMessage(
    stream: ReturnType<typeof replayHarness.getStreams>[number],
    event: (typeof canonicalSessionLifecycleReplayEvents)[number],
  ): Promise<void> {
    await act(async () => {
      stream.emit("message", event);
      await new Promise<void>((resolve) => {
        window.setTimeout(() => resolve(), 20);
      });
    });
  }

  it("projects paused and resumed Factory Session lifecycle from streamed canonical events", async () => {
    const { result } = renderHook(() => useDashboardSnapshot(), {
      wrapper: createWrapper(queryClient),
    });

    const stream = replayHarness.getStreams()[0];
    if (!stream) {
      throw new Error("expected dashboard stream to be opened");
    }

    await emitStreamMessage(stream, sessionLifecycleStartedEvent);
    await emitStreamMessage(stream, sessionLifecyclePausedEvent);

    await waitFor(() => {
      expect(result.current.snapshot?.runtime?.session?.bracket).toMatchObject({
        lifecycle_control_status: "PAUSED",
        paused_at: "2026-06-09T12:00:02Z",
        session_id: "session-alpha",
      });
    });

    await emitStreamMessage(stream, sessionLifecycleResumedEvent);

    await waitFor(() => {
      expect(result.current.snapshot?.runtime?.session?.bracket).toMatchObject({
        lifecycle_control_status: "RUNNING",
        resumed_at: "2026-06-09T12:00:04Z",
        session_id: "session-alpha",
      });
    });
    expect(useFactoryTimelineStore.getState().events).toHaveLength(3);
    expect(useFactoryTimelineStore.getState().selectedTick).toBe(3);
  });

  it("keeps paused and resumed lifecycle reflection after event-stream reconnect", async () => {
    const { result } = renderHook(
      () => useDashboardSnapshot({ locale: "en" }),
      {
        wrapper: createWrapper(queryClient),
      },
    );

    const stream = replayHarness.getStreams()[0];
    if (!stream) {
      throw new Error("expected dashboard stream to be opened");
    }

    await emitStreamMessage(stream, sessionLifecycleStartedEvent);
    await emitStreamMessage(stream, sessionLifecyclePausedEvent);

    await waitFor(() => {
      expect(
        result.current.snapshot?.runtime?.session?.bracket
          ?.lifecycle_control_status,
      ).toBe("PAUSED");
    });

    act(() => {
      stream.onerror?.(new Event("error"));
    });

    expect(result.current.streamState).toMatchObject({
      message: "Reconnecting event stream",
      status: "reconnecting",
    });

    await waitFor(
      () => {
        expect(replayHarness.getStreams()).toHaveLength(2);
      },
      { timeout: 3000 },
    );

    const reconnectStream = replayHarness.getStreams()[1];
    if (!reconnectStream) {
      throw new Error("expected reconnect stream to be opened");
    }

    expect(reconnectStream.url).toContain(
      "after_event_id=session-lifecycle-replay-paused",
    );
    expect(reconnectStream.url).toContain("after_sequence=2");

    await emitStreamMessage(reconnectStream, sessionLifecycleResumedEvent);

    await waitFor(() => {
      expect(result.current.snapshot?.runtime?.session?.bracket).toMatchObject({
        lifecycle_control_status: "RUNNING",
        paused_at: "2026-06-09T12:00:02Z",
        resumed_at: "2026-06-09T12:00:04Z",
        session_id: "session-alpha",
      });
    });
    expect(useFactoryTimelineStore.getState().events).toHaveLength(3);
  });
});

function createWrapper(queryClient: QueryClient) {
  return function Wrapper({ children }: PropsWithChildren) {
    return (
      <QueryClientProvider client={queryClient}>
        <DashboardSessionProvider>{children}</DashboardSessionProvider>
      </QueryClientProvider>
    );
  };
}
