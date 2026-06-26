import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { PropsWithChildren } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { FACTORY_EVENT_TYPES } from "../../../api/events";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import { createReplayHarness } from "../../../testing/replay-harness";
import { currentFactoryDefinitionQueryKey } from "../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import {
  FACTORY_TIMELINE_DEBUG_GLOBAL,
  FACTORY_TIMELINE_DEBUG_STORAGE_KEY,
} from "../../timeline/state/factoryTimelineDebug";
import {
  useFactoryTimelineStore,
  type WorldState,
} from "../../timeline/state/factoryTimelineStore";
import { readTimelineCheckpoint } from "../../timeline/state/timelineCheckpointPersistence";
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

function installIndexedDBTestDouble() {
  const records = new Map<string, unknown>();
  const database = {
    close: () => {},
    createObjectStore: () => undefined,
    objectStoreNames: {
      contains: () => true,
    },
    transaction: () => ({
      objectStore: () => ({
        delete: (key: string) =>
          indexedDBRequest(undefined, () => {
            records.delete(key);
          }),
        get: (key: string) => indexedDBRequest(records.get(key)),
        put: (value: { sessionID: string }) =>
          indexedDBRequest(value.sessionID, () => {
            records.set(value.sessionID, value);
          }),
      }),
    }),
  };
  const indexedDB = {
    open: () => {
      const request = indexedDBRequest(database);
      window.setTimeout(
        () => request.onupgradeneeded?.({} as IDBVersionChangeEvent),
        0,
      );
      return request;
    },
  };

  Object.defineProperty(window, "indexedDB", {
    configurable: true,
    value: indexedDB,
  });
}

function indexedDBRequest<T>(result: T, beforeSuccess?: () => void) {
  const request = {
    error: null,
    onblocked: null,
    onerror: null,
    onsuccess: null,
    onupgradeneeded: null,
    result,
  } as unknown as IDBRequest<T> & {
    onblocked?: ((event: Event) => void) | null;
    onupgradeneeded?: ((event: IDBVersionChangeEvent) => void) | null;
  };

  window.setTimeout(() => {
    beforeSuccess?.();
    request.onsuccess?.({} as Event);
  }, 0);

  return request;
}

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
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    replayHarness.install();
    installIndexedDBTestDouble();
    window.sessionStorage.clear();
    fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = new URL(String(input), "http://localhost");
      const afterEventId = url.searchParams.get("after_event_id") ?? undefined;
      const afterSequenceRaw = url.searchParams.get("after_sequence");

      return new Response(
        JSON.stringify(
          buildSyncPreflightResponse({
            reconnectCursor:
              afterEventId || afterSequenceRaw
                ? {
                    afterEventId,
                    afterSequence: afterSequenceRaw
                      ? Number(afterSequenceRaw)
                      : undefined,
                    provided: true,
                    validForStreamGeneration: true,
                  }
                : {
                    provided: false,
                    validForStreamGeneration: true,
                  },
            requestedSessionId: url.pathname.includes("session-beta")
              ? "session-beta"
              : DEFAULT_FACTORY_SESSION_ID,
          }),
        ),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 200,
        },
      );
    });
    vi.stubGlobal("fetch", fetchMock);
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
    vi.unstubAllGlobals();
    window.sessionStorage.clear();
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
    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(1);
    });
    expect(fetchMock).toHaveBeenCalledWith(
      `/factory-sessions/${DEFAULT_FACTORY_SESSION_ID}/sync-preflight`,
      expect.objectContaining({ method: "GET" }),
    );

    act(() => {
      rerender({ refreshToken: 1 });
    });

    await waitFor(() => {
      expect(result.current.isInitialLoading).toBe(true);
    });
    expect(useFactoryTimelineStore.getState().selectedTick).toBe(0);
    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(2);
    });

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

    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(1);
    });
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

    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(1);
    });
    const stream = replayHarness.getStreams()[0];

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

  it("hydrates a persisted checkpoint and opens the stream after its cursor", async () => {
    useFactoryTimelineStore.getState().reset();

    const { unmount } = renderHook(() => useDashboardSnapshot(), {
      wrapper: createWrapper(queryClient),
    });
    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(1);
    });
    const stream = replayHarness.getStreams()[0];

    await act(async () => {
      stream.emit("message", {
        context: {
          eventTime: "2026-04-25T20:00:01Z",
          sequence: 7,
          tick: 7,
        },
        id: "checkpoint-event-7",
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
      expect(
        useFactoryTimelineStore.getState().currentReplayCheckpoint,
      ).toEqual(
        expect.objectContaining({
          afterEventId: "checkpoint-event-7",
          afterSequence: 7,
          selectedTick: 7,
        }),
      );
    });
    await waitFor(async () => {
      await expect(
        readTimelineCheckpoint(window.indexedDB, DEFAULT_FACTORY_SESSION_ID),
      ).resolves.toEqual(
        expect.objectContaining({
          afterEventId: "checkpoint-event-7",
          afterSequence: 7,
          selectedTick: 7,
        }),
      );
    });

    unmount();
    replayHarness.reset();
    replayHarness.install();
    useFactoryTimelineStore.getState().reset();

    renderHook(() => useDashboardSnapshot(), {
      wrapper: createWrapper(queryClient),
    });

    await waitFor(() => {
      expect(useFactoryTimelineStore.getState().selectedTick).toBe(7);
    });
    expect(replayHarness.getStreams().at(-1)?.url).toBe(
      `/factory-sessions/${DEFAULT_FACTORY_SESSION_ID}/events?after_event_id=checkpoint-event-7&after_sequence=7`,
    );
  });

  it("drops stale reconnect cursors and reopens the stream without cursor params", async () => {
    useFactoryTimelineStore.getState().reset();

    const { unmount } = renderHook(() => useDashboardSnapshot(), {
      wrapper: createWrapper(queryClient),
    });
    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(1);
    });
    const stream = replayHarness.getStreams()[0];

    await act(async () => {
      stream.emit("message", {
        context: {
          eventTime: "2026-04-25T20:00:01Z",
          sequence: 7,
          tick: 7,
        },
        id: "checkpoint-event-7",
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

    await waitFor(async () => {
      await expect(
        readTimelineCheckpoint(window.indexedDB, DEFAULT_FACTORY_SESSION_ID),
      ).resolves.toEqual(
        expect.objectContaining({
          afterEventId: "checkpoint-event-7",
          afterSequence: 7,
          selectedTick: 7,
          syncIdentity: expect.objectContaining({
            backendScopeId: "backend-a",
          }),
        }),
      );
    });

    unmount();
    replayHarness.reset();
    replayHarness.install();
    useFactoryTimelineStore.getState().reset();
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify(
          buildSyncPreflightResponse({
            checkpointReusable: false,
            reasonCode: "cursor_stale",
            reconnectCursor: {
              afterEventId: "checkpoint-event-7",
              afterSequence: 7,
              provided: true,
              validForStreamGeneration: false,
            },
          }),
        ),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 200,
        },
      ),
    );

    renderHook(() => useDashboardSnapshot(), {
      wrapper: createWrapper(queryClient),
    });

    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(1);
    });
    expect(replayHarness.getStreams()[0]?.url).toBe(
      `/factory-sessions/${DEFAULT_FACTORY_SESSION_ID}/events`,
    );
    await expect(
      readTimelineCheckpoint(window.indexedDB, DEFAULT_FACTORY_SESSION_ID),
    ).resolves.toBeNull();
  });

  it("clears invalid checkpoint scope before replay when the logical session remaps", async () => {
    useFactoryTimelineStore.getState().reset();
    queryClient.setQueryData(
      currentFactoryDefinitionQueryKey(DEFAULT_FACTORY_SESSION_ID),
      {
        name: "stale cached factory",
      },
    );

    const { unmount } = renderHook(() => useDashboardSnapshot(), {
      wrapper: createWrapper(queryClient),
    });
    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(1);
    });
    const stream = replayHarness.getStreams()[0];

    await act(async () => {
      stream.emit("message", {
        context: {
          eventTime: "2026-04-25T20:00:01Z",
          sequence: 9,
          tick: 9,
        },
        id: "checkpoint-event-9",
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

    await waitFor(async () => {
      await expect(
        readTimelineCheckpoint(window.indexedDB, DEFAULT_FACTORY_SESSION_ID),
      ).resolves.toEqual(
        expect.objectContaining({
          afterEventId: "checkpoint-event-9",
          afterSequence: 9,
          selectedTick: 9,
          syncIdentity: expect.objectContaining({
            factorySessionId: DEFAULT_FACTORY_SESSION_ID,
          }),
        }),
      );
    });

    unmount();
    replayHarness.reset();
    replayHarness.install();
    useFactoryTimelineStore.getState().reset();
    queryClient.setQueryData(
      currentFactoryDefinitionQueryKey(DEFAULT_FACTORY_SESSION_ID),
      {
        name: "stale cached factory",
      },
    );
    fetchMock.mockResolvedValueOnce(
      new Response(
        JSON.stringify(
          buildSyncPreflightResponse({
            checkpointReusable: false,
            factorySessionId: "session-promoted",
            reasonCode: "logical_session_remap",
            streamGenerationId: "stream-promoted",
          }),
        ),
        {
          headers: {
            "Content-Type": "application/json",
          },
          status: 200,
        },
      ),
    );

    renderHook(() => useDashboardSnapshot(), {
      wrapper: createWrapper(queryClient),
    });

    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(1);
    });
    expect(replayHarness.getStreams()[0]?.url).toBe(
      `/factory-sessions/${DEFAULT_FACTORY_SESSION_ID}/events`,
    );
    await expect(
      readTimelineCheckpoint(window.indexedDB, DEFAULT_FACTORY_SESSION_ID),
    ).resolves.toBeNull();
    expect(
      queryClient.getQueryData(
        currentFactoryDefinitionQueryKey(DEFAULT_FACTORY_SESSION_ID),
      ),
    ).toBeUndefined();
  });
});

function buildSyncPreflightResponse(
  overrides: Partial<{
    backendScopeId: string;
    checkpointReusable: boolean;
    factorySessionId: string;
    logicalSessionKeyId: string;
    reasonCode: string;
    reconnectCursor: {
      afterEventId?: string;
      afterSequence?: number;
      provided: boolean;
      validForStreamGeneration: boolean;
    };
    requestedSessionId: string;
    streamGenerationId: string;
  }> = {},
) {
  return {
    backendScopeId: "backend-a",
    checkpointReusable: true,
    factorySessionId: DEFAULT_FACTORY_SESSION_ID,
    logicalSessionKeyId: "logical-default",
    reasonCode: "ok",
    reconnectCursor: {
      provided: false,
      validForStreamGeneration: true,
      ...overrides.reconnectCursor,
    },
    requestedSessionId: DEFAULT_FACTORY_SESSION_ID,
    streamGenerationId: "stream-default",
    ...overrides,
  };
}

function createWrapper(queryClient: QueryClient) {
  return function Wrapper({ children }: PropsWithChildren) {
    return (
      <QueryClientProvider client={queryClient}>
        <DashboardSessionProvider>{children}</DashboardSessionProvider>
      </QueryClientProvider>
    );
  };
}
