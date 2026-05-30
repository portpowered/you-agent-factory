import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { PropsWithChildren } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { FACTORY_EVENT_TYPES } from "../../../api/events";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import { createReplayHarness } from "../../../testing/replay-harness";
import {
  CURRENT_FACTORY_DOCUMENT_QUERY_KEY,
  CURRENT_FACTORY_DEFINITION_QUERY_KEY,
} from "../../current-factory-definition/public";
import { useDashboardSessionStore } from "../state/dashboardSessionStore";
import { DashboardSessionProvider } from "../session/dashboard-session-provider";
import {
  type WorldState,
  useFactoryTimelineStore,
} from "../../timeline/state/factoryTimelineStore";
import {
  createDefaultDashboardStreamState,
  useDashboardStreamStore,
} from "../state/dashboardStreamStore";
import { useFactoryEventStream } from "./useFactoryEventStream";

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

function timelineSnapshot(snapshot: DashboardSnapshot): WorldState {
  return {
    ...snapshot,
    relationsByWorkID: {},
    tracesByWorkID: {},
    workstationRequestsByDispatchID: {},
    workRequestsByID: {},
  };
}

const CANONICAL_SELECTED_TICK_EVENTS = [
  {
    context: {
      eventTime: "2026-04-25T20:00:01Z",
      sequence: 1,
      tick: 1,
    },
    id: "event-1",
    payload: {
      factory: {
        workTypes: [{
          name: "story",
          states: [
            { name: "new", type: "INITIAL" },
            { name: "done", type: "TERMINAL" },
          ],
        }],
        workstations: [
          {
            id: "review",
            inputs: [{ state: "new", workType: "story" }],
            name: "Review",
            outputs: [{ state: "done", workType: "story" }],
            worker: "reviewer",
          },
        ],
        workers: [
          {
            model: "gpt-5.4",
            modelProvider: "codex",
            name: "reviewer",
            type: "MODEL_WORKER",
          },
        ],
      },
    },
    type: FACTORY_EVENT_TYPES.initialStructureRequest,
  },
  {
    context: {
      eventTime: "2026-04-25T20:00:02Z",
      requestId: "request-story-1",
      sequence: 2,
      tick: 2,
      traceIds: ["trace-story-1"],
      workIds: ["work-story-1"],
    },
    id: "event-2",
    payload: {
      type: "FACTORY_REQUEST_BATCH",
      works: [
        {
          name: "Canonical Story",
          trace_id: "trace-story-1",
          work_id: "work-story-1",
          work_type_name: "story",
        },
      ],
    },
    type: FACTORY_EVENT_TYPES.workRequest,
  },
];

describe("useFactoryEventStream", () => {
  let queryClient: QueryClient;
  const receivedEvents: unknown[] = [];

  beforeEach(() => {
    replayHarness.install();
    receivedEvents.length = 0;
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

  it("opens the session-scoped events stream and delivers compacted events to onEvent", async () => {
    renderHook(
      () =>
        useFactoryEventStream({
          enabled: true,
          onEvent: (event) => {
            receivedEvents.push(event);
          },
          sessionID: DEFAULT_FACTORY_SESSION_ID,
        }),
      { wrapper: createWrapper(queryClient) },
    );

    expect(replayHarness.getStreams()).toHaveLength(1);
    expect(replayHarness.getStreams()[0]?.url).toBe(
      `/factory-sessions/${DEFAULT_FACTORY_SESSION_ID}/events`,
    );

    const stream = replayHarness.getStreams()[0];
    if (!stream) {
      throw new Error("expected dashboard stream to be opened");
    }

    await act(async () => {
      stream.emit("message", CANONICAL_SELECTED_TICK_EVENTS[0]);
      await new Promise<void>((resolve) => {
        window.setTimeout(() => resolve(), 20);
      });
    });

    await waitFor(() => {
      expect(receivedEvents).toHaveLength(1);
    });
  });

  it("does not open a stream when disabled for a paused session and keeps the paused offline message", async () => {
    renderHook(
      () =>
        useFactoryEventStream({
          enabled: false,
          onEvent: () => {},
          sessionID: DEFAULT_FACTORY_SESSION_ID,
        }),
      { wrapper: createWrapper(queryClient) },
    );

    expect(replayHarness.getStreams()).toHaveLength(0);
    expect(useDashboardStreamStore.getState().streamState).toMatchObject({
      status: "offline",
      message: "Live session updates paused. Showing last event state.",
    });
  });

  it("reopens the stream after refreshToken changes", async () => {
    const { rerender } = renderHook(
      ({ refreshToken }: { refreshToken: number }) =>
        useFactoryEventStream({
          enabled: true,
          onEvent: () => {},
          refreshToken,
          sessionID: DEFAULT_FACTORY_SESSION_ID,
        }),
      { initialProps: { refreshToken: 0 }, wrapper: createWrapper(queryClient) },
    );

    expect(replayHarness.getStreams()).toHaveLength(1);

    act(() => {
      rerender({ refreshToken: 1 });
    });

    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(2);
    });
    expect(replayHarness.getStreams()[1]?.url).toBe(
      `/factory-sessions/${DEFAULT_FACTORY_SESSION_ID}/events`,
    );
  });

  it("reopens the stream when resuming from pause without clearing onEvent delivery", async () => {
    const { rerender } = renderHook(
      ({ enabled }: { enabled: boolean }) =>
        useFactoryEventStream({
          enabled,
          onEvent: (event) => {
            receivedEvents.push(event);
          },
          sessionID: DEFAULT_FACTORY_SESSION_ID,
        }),
      { initialProps: { enabled: true }, wrapper: createWrapper(queryClient) },
    );

    expect(replayHarness.getStreams()).toHaveLength(1);

    act(() => {
      rerender({ enabled: false });
    });

    await waitFor(() => {
      expect(useDashboardStreamStore.getState().streamState.status).toBe("offline");
    });
    expect(replayHarness.getStreams()).toHaveLength(1);

    act(() => {
      rerender({ enabled: true });
    });

    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(2);
    });
  });

  it("updates factory definition queries from FACTORY_CHANGE events", async () => {
    queryClient.setQueryData(CURRENT_FACTORY_DOCUMENT_QUERY_KEY, {
      name: "factory",
      workers: [],
      workstations: [],
      workTypes: [],
      version: {
        logical: "7",
        physical: "2026-05-17T14:59:00Z",
      },
    });

    renderHook(
      () =>
        useFactoryEventStream({
          enabled: true,
          onEvent: () => {},
          sessionID: DEFAULT_FACTORY_SESSION_ID,
        }),
      { wrapper: createWrapper(queryClient) },
    );

    const stream = replayHarness.getStreams()[0];
    if (!stream) {
      throw new Error("expected dashboard stream to be opened");
    }

    await act(async () => {
      stream.emit("message", {
        context: {
          eventTime: "2026-05-17T15:00:00Z",
          sequence: 8,
          tick: 8,
        },
        id: "factory-event/factory-change/8",
        payload: {
          factory: {
            name: "factory",
            workers: [
              {
                model: "gpt-5.6",
                modelProvider: "CODEX",
                name: "reviewer",
                type: "MODEL_WORKER",
              },
            ],
            workTypes: [{
              name: "story",
              states: [{ name: "new", type: "INITIAL" }],
            }],
            workstations: [
              {
                body: "Updated prompt",
                id: "review",
                inputs: [{ state: "new", workType: "story" }],
                name: "Review",
                outputs: [],
                promptFile: "prompts/review.md",
                worker: "reviewer",
              },
            ],
          },
        },
        type: FACTORY_EVENT_TYPES.factoryChange,
      });
      await new Promise<void>((resolve) => {
        window.setTimeout(() => resolve(), 20);
      });
    });

    await waitFor(() => {
      expect(queryClient.getQueryData(CURRENT_FACTORY_DEFINITION_QUERY_KEY)).toMatchObject(
        {
          workers: [expect.objectContaining({ model: "gpt-5.6" })],
        },
      );
    });

    await waitFor(() => {
      expect(
        queryClient.getQueryState(
          CURRENT_FACTORY_DOCUMENT_QUERY_KEY,
        )?.isInvalidated,
      ).toBe(true);
    });
  });

  it("surfaces offline stream state when the connection fails before the first event", async () => {
    renderHook(
      () =>
        useFactoryEventStream({
          enabled: true,
          onEvent: () => {},
          sessionID: DEFAULT_FACTORY_SESSION_ID,
        }),
      { wrapper: createWrapper(queryClient) },
    );

    const stream = replayHarness.getStreams()[0];
    if (!stream) {
      throw new Error("expected dashboard stream to be opened");
    }

    act(() => {
      stream.onerror?.(new Event("error"));
    });

    await waitFor(() => {
      expect(useDashboardStreamStore.getState().streamState).toMatchObject({
        status: "offline",
        message: "Factory event stream disconnected. Showing last event state.",
      });
    });
  });

  it("does not open a stream when sessionID is null", () => {
    renderHook(
      () =>
        useFactoryEventStream({
          enabled: false,
          onEvent: () => {},
          sessionID: null,
        }),
      { wrapper: createWrapper(queryClient) },
    );

    expect(replayHarness.getStreams()).toHaveLength(0);
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
