import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { vi } from "vitest";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { FACTORY_EVENT_TYPES } from "../../../api/events";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import { createReplayHarness } from "../../../testing/replay-harness";
import {
  FACTORY_TIMELINE_DEBUG_GLOBAL,
  FACTORY_TIMELINE_DEBUG_STORAGE_KEY,
} from "../../timeline/state/factoryTimelineDebug";
import {
  useFactoryTimelineStore,
  type WorldState,
} from "../../timeline/state/factoryTimelineStore";
import { emptyReplayWorldState } from "../../timeline/state/timeline/replayWorldStateSupport";
import { readTimelineCheckpoint } from "../../timeline/state/timelineCheckpointPersistence";
import * as factorySessionsAPI from "../../../api/factory-sessions";
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
    deleteObjectStore: () => undefined,
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
        put: (value: { sessionID: string; storageKey?: string }) =>
          indexedDBRequest(value.storageKey ?? value.sessionID, () => {
            records.set(value.storageKey ?? value.sessionID, value);
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

  return records;
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

function checkpointStorageKey(): string {
  return [
    "backend-scope-a",
    DEFAULT_FACTORY_SESSION_ID,
    "2026-06-26T00:00:00Z",
  ].join("::");
}

describe("useDashboardSnapshot composer", () => {
  let indexedDBRecords: Map<string, unknown>;
  let queryClient: QueryClient;
  let getFactorySessionSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    replayHarness.install();
    indexedDBRecords = installIndexedDBTestDouble();
    getFactorySessionSpy = vi
      .spyOn(factorySessionsAPI, "getFactorySession")
      .mockResolvedValue({
        session: {
          factoryDir: "/workspace/factory",
          folderPath: "/workspace",
          id: DEFAULT_FACTORY_SESSION_ID,
          isDefault: true,
          project: "factory",
          runtime: {
            lifecycle: {
              startedAt: "2026-06-26T00:00:00Z",
              updatedAt: "2026-06-26T00:00:00Z",
            },
            orchestratorKind: "STATIC",
            progress: {
              categories: {},
              factoryState: "IDLE",
              inFlightCount: 0,
              totalTokens: 0,
            },
            streamIdentity: {
              backendScopeID: "backend-scope-a",
              factorySessionID: DEFAULT_FACTORY_SESSION_ID,
              streamGenerationID: "2026-06-26T00:00:00Z",
            },
            status: "IDLE",
            usage: { resources: [] },
          },
          target: { kind: "default", name: DEFAULT_FACTORY_SESSION_ID },
        },
      });
    window.sessionStorage.clear();
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
    vi.unstubAllGlobals();
    getFactorySessionSpy.mockRestore();
    replayHarness.reset();
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

  it("preflights the selected session before restoring checkpoints or opening the stream", async () => {
    const checkpoint = {
      afterEventId: "event-2",
      afterSequence: 2,
      replayState: emptyReplayWorldState(SEEDED_SNAPSHOT.tick_count),
      selectedTick: SEEDED_SNAPSHOT.tick_count,
    };
    const readCheckpointSpy = vi
      .spyOn(
        await import("../../timeline/state/timelineCheckpointPersistence"),
        "readTimelineCheckpoint",
      )
      .mockResolvedValue(checkpoint);

    let resolvePreflight: (() => void) | null = null;
    getFactorySessionSpy.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolvePreflight = () =>
            resolve({
              session: {
                factoryDir: "/workspace/factory",
                folderPath: "/workspace",
                id: DEFAULT_FACTORY_SESSION_ID,
                isDefault: true,
                project: "factory",
                runtime: {
                  lifecycle: {
                    startedAt: "2026-06-26T00:00:00Z",
                    updatedAt: "2026-06-26T00:00:00Z",
                  },
                  orchestratorKind: "STATIC",
                  progress: {
                    categories: {},
                    factoryState: "IDLE",
                    inFlightCount: 0,
                    totalTokens: 0,
                  },
                  streamIdentity: {
                    backendScopeID: "backend-scope-a",
                    factorySessionID: DEFAULT_FACTORY_SESSION_ID,
                    streamGenerationID: "2026-06-26T00:00:00Z",
                  },
                  status: "IDLE",
                  usage: { resources: [] },
                },
                target: { kind: "default", name: DEFAULT_FACTORY_SESSION_ID },
              },
            });
        }),
    );

    renderHook(() => useDashboardSnapshot(), {
      wrapper: createWrapper(queryClient),
    });

    expect(replayHarness.getStreams()).toHaveLength(0);
    expect(readCheckpointSpy).not.toHaveBeenCalled();

    await act(async () => {
      resolvePreflight?.();
      await Promise.resolve();
    });

    await waitFor(() => {
      expect(readCheckpointSpy).toHaveBeenCalledWith(
        window.indexedDB,
        DEFAULT_FACTORY_SESSION_ID,
        {
          backendScopeID: "backend-scope-a",
          factorySessionID: DEFAULT_FACTORY_SESSION_ID,
          streamGenerationID: "2026-06-26T00:00:00Z",
        },
      );
    });
    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(1);
    });

    readCheckpointSpy.mockRestore();
  });

  it("surfaces a recoverable offline error and skips stream open when preflight cannot resolve the session", async () => {
    useFactoryTimelineStore.getState().reset();
    getFactorySessionSpy.mockRejectedValue(
      new Error("The selected session could not be resolved."),
    );

    const { result } = renderHook(() => useDashboardSnapshot(), {
      wrapper: createWrapper(queryClient),
    });

    await waitFor(() => {
      expect(result.current.error?.message).toBe(
        "The selected session could not be resolved.",
      );
    });
    expect(result.current.isInitialLoading).toBe(false);
    expect(replayHarness.getStreams()).toHaveLength(0);
    expect(useDashboardStreamStore.getState().streamState).toMatchObject({
      status: "offline",
      message: "The selected session could not be resolved.",
    });
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
        readTimelineCheckpoint(window.indexedDB, DEFAULT_FACTORY_SESSION_ID, {
          backendScopeID: "backend-scope-a",
          factorySessionID: DEFAULT_FACTORY_SESSION_ID,
          streamGenerationID: "2026-06-26T00:00:00Z",
        }),
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

  it("ignores and deletes legacy v1 checkpoints without reopening from a stale cursor", async () => {
    indexedDBRecords.set(checkpointStorageKey(), {
      checkpoint: {
        afterEventId: "legacy-event-7",
        afterSequence: 7,
        replayState: emptyReplayWorldState(7),
        selectedTick: 7,
      },
      schemaVersion: 1,
      sessionID: DEFAULT_FACTORY_SESSION_ID,
      storageKey: checkpointStorageKey(),
    });

    renderHook(() => useDashboardSnapshot(), {
      wrapper: createWrapper(queryClient),
    });

    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(1);
    });
    expect(replayHarness.getStreams()[0]?.url).toBe(
      `/factory-sessions/${DEFAULT_FACTORY_SESSION_ID}/events`,
    );
    expect(useFactoryTimelineStore.getState().selectedTick).not.toBe(7);
    await expect(
      readTimelineCheckpoint(window.indexedDB, DEFAULT_FACTORY_SESSION_ID, {
        backendScopeID: "backend-scope-a",
        factorySessionID: DEFAULT_FACTORY_SESSION_ID,
        streamGenerationID: "2026-06-26T00:00:00Z",
      }),
    ).resolves.toBe(null);
    expect(indexedDBRecords.has(checkpointStorageKey())).toBe(false);
  });

  it("clears a stale reconnect checkpoint and silently replays from scratch", async () => {
    indexedDBRecords.set(checkpointStorageKey(), {
      checkpoint: {
        afterEventId: "checkpoint-event-7",
        afterSequence: 7,
        replayState: emptyReplayWorldState(7),
        selectedTick: 7,
      },
      schemaVersion: 2,
      sessionID: DEFAULT_FACTORY_SESSION_ID,
      storageKey: checkpointStorageKey(),
      streamIdentity: {
        backendScopeID: "backend-scope-a",
        factorySessionID: DEFAULT_FACTORY_SESSION_ID,
        streamGenerationID: "2026-06-26T00:00:00Z",
      },
    });
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        body: {
          cancel: vi.fn().mockResolvedValue(undefined),
        },
        ok: false,
        status: 400,
      }),
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
    expect(useFactoryTimelineStore.getState().selectedTick).toBe(0);
    await expect(
      readTimelineCheckpoint(window.indexedDB, DEFAULT_FACTORY_SESSION_ID, {
        backendScopeID: "backend-scope-a",
        factorySessionID: DEFAULT_FACTORY_SESSION_ID,
        streamGenerationID: "2026-06-26T00:00:00Z",
      }),
    ).resolves.toBe(null);
    expect(indexedDBRecords.has(checkpointStorageKey())).toBe(false);
  });

  it("reopens the stream without a stale persisted cursor after same-session refresh", async () => {
    useFactoryTimelineStore.getState().reset();

    const { rerender } = renderHook(
      ({ refreshToken }: { refreshToken: number }) =>
        useDashboardSnapshot({ refreshToken }),
      {
        initialProps: { refreshToken: 0 },
        wrapper: createWrapper(queryClient),
      },
    );

    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(1);
    });
    const stream = replayHarness.getStreams()[0];

    await act(async () => {
      stream.emit("message", {
        context: {
          eventTime: "2026-04-25T20:00:01Z",
          sequence: 29,
          sessionSequence: 29,
          tick: 29,
        },
        id: "factory-event/dispatch-completed/stale-cursor",
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
        readTimelineCheckpoint(
          window.indexedDB,
          DEFAULT_FACTORY_SESSION_ID,
          {
            backendScopeID: "backend-scope-a",
            factorySessionID: DEFAULT_FACTORY_SESSION_ID,
            streamGenerationID: "2026-06-26T00:00:00Z",
          },
        ),
      ).resolves.toEqual(
        expect.objectContaining({
          afterEventId: "factory-event/dispatch-completed/stale-cursor",
          afterSequence: 29,
        }),
      );
    });

    act(() => {
      rerender({ refreshToken: 1 });
    });

    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(2);
    });
    expect(replayHarness.getStreams().at(-1)?.url).toBe(
      `/factory-sessions/${DEFAULT_FACTORY_SESSION_ID}/events`,
    );
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
