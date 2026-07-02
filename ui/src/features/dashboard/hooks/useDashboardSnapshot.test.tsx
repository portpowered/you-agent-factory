import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { vi } from "vitest";

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
import { emptyReplayWorldState } from "../../timeline/state/timeline/replayWorldStateSupport";
import { readTimelineCheckpoint } from "../../timeline/state/timelineCheckpointPersistence";
import * as syncPreflightAPI from "../../../api/factory-sessions/sync-preflight";
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
        getAll: () => indexedDBRequest([...records.values()]),
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

const RESOLVED_DEFAULT_SESSION_UUID = "a1b2c3d4-e5f6-4789-a012-3456789abcde";

function resolvedDefaultStreamIdentity() {
  return {
    backendScopeID: "backend-scope-a",
    factorySessionID: RESOLVED_DEFAULT_SESSION_UUID,
    logicalSessionKeyID: "logical-default",
    streamGenerationID: "2026-06-26T00:00:00Z",
  };
}

function checkpointStorageKey(): string {
  const identity = resolvedDefaultStreamIdentity();
  return [
    identity.backendScopeID,
    identity.factorySessionID,
    identity.streamGenerationID,
  ].join("::");
}

function okSyncPreflightResponse(
  overrides: Partial<syncPreflightAPI.FactorySessionSyncPreflightResponse> = {},
) {
  return {
    backendScopeId: resolvedDefaultStreamIdentity().backendScopeID,
    checkpointReusable: true,
    factorySessionId: RESOLVED_DEFAULT_SESSION_UUID,
    logicalSessionKeyId: resolvedDefaultStreamIdentity().logicalSessionKeyID,
    reasonCode: "ok" as const,
    reconnectCursor: {
      provided: false,
      validForStreamGeneration: true,
    },
    requestedSessionId: DEFAULT_FACTORY_SESSION_ID,
    streamGenerationId: resolvedDefaultStreamIdentity().streamGenerationID,
    ...overrides,
  };
}

function okSyncPreflightResponseForSession(
  sessionID: string,
  reconnectCursor?: syncPreflightAPI.FactorySessionSyncPreflightRequest["reconnectCursor"],
) {
  if (sessionID === "session-beta") {
    return okSyncPreflightResponse({
      backendScopeId: "backend-scope-b",
      factorySessionId: "session-beta",
      logicalSessionKeyId: "logical-beta",
      requestedSessionId: "session-beta",
      reconnectCursor: {
        afterEventId: reconnectCursor?.afterEventId,
        afterSequence: reconnectCursor?.afterSequence,
        provided: Boolean(
          reconnectCursor?.afterEventId ||
            reconnectCursor?.afterSequence != null,
        ),
        validForStreamGeneration: true,
      },
    });
  }
  return okSyncPreflightResponse({
    checkpointReusable: true,
    reconnectCursor: {
      afterEventId: reconnectCursor?.afterEventId,
      afterSequence: reconnectCursor?.afterSequence,
      provided: Boolean(
        reconnectCursor?.afterEventId || reconnectCursor?.afterSequence != null,
      ),
      validForStreamGeneration: true,
    },
  });
}

describe("useDashboardSnapshot composer", () => {
  let indexedDBRecords: Map<string, unknown>;
  let queryClient: QueryClient;
  let getSyncPreflightSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    replayHarness.install();
    indexedDBRecords = installIndexedDBTestDouble();
    getSyncPreflightSpy = vi
      .spyOn(syncPreflightAPI, "getFactorySessionSyncPreflight")
      .mockImplementation(async (sessionID, reconnectCursor) =>
        okSyncPreflightResponseForSession(sessionID, reconnectCursor),
      );
    window.sessionStorage.clear();
    queryClient = new QueryClient({
      defaultOptions: {
        mutations: { retry: false },
        queries: { retry: false },
      },
    });
    useDashboardStreamStore.setState({
      resolvedStreamIdentity: null,
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
    getSyncPreflightSpy.mockRestore();
    replayHarness.reset();
    window.sessionStorage.clear();
    useDashboardStreamStore.setState({
      resolvedStreamIdentity: null,
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
    let preflightCallCount = 0;
    getSyncPreflightSpy.mockImplementation(async (_sessionID, reconnectCursor) => {
      preflightCallCount += 1;
      if (preflightCallCount === 1) {
        await new Promise<void>((resolve) => {
          resolvePreflight = resolve;
        });
      }
      return okSyncPreflightResponse({
        checkpointReusable: true,
        reconnectCursor: {
          afterEventId: reconnectCursor?.afterEventId,
          afterSequence: reconnectCursor?.afterSequence,
          provided: Boolean(
            reconnectCursor?.afterEventId ||
              reconnectCursor?.afterSequence != null,
          ),
          validForStreamGeneration: true,
        },
      });
    });

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
        resolvedDefaultStreamIdentity(),
      );
    });
    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(1);
    });

    readCheckpointSpy.mockRestore();
  });

  it("surfaces a recoverable preflight recovery state when sync preflight cannot resolve the session", async () => {
    useFactoryTimelineStore.getState().reset();
    getSyncPreflightSpy.mockResolvedValue({
      checkpointReusable: false,
      reasonCode: "session_not_found",
      reconnectCursor: {
        provided: false,
        validForStreamGeneration: false,
      },
      requestedSessionId: DEFAULT_FACTORY_SESSION_ID,
    });

    const { result } = renderHook(() => useDashboardSnapshot(), {
      wrapper: createWrapper(queryClient),
    });

    await waitFor(() => {
      expect(result.current.preflightRecovery).toEqual({
        reasonCode: "session_not_found",
        requestedSessionId: DEFAULT_FACTORY_SESSION_ID,
      });
    });
    expect(result.current.isInitialLoading).toBe(false);
    expect(replayHarness.getStreams()).toHaveLength(0);
  });

  it("reopens the event stream when the selected session tab changes", async () => {
    renderHook(() => useDashboardSnapshot(), {
      wrapper: createWrapper(queryClient),
    });

    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(1);
    });
    expect(replayHarness.getStreams()[0]?.url).toBe(
      `/factory-sessions/${RESOLVED_DEFAULT_SESSION_UUID}/events`,
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
        readTimelineCheckpoint(window.indexedDB, resolvedDefaultStreamIdentity()),
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
      `/factory-sessions/${RESOLVED_DEFAULT_SESSION_UUID}/events?after_event_id=checkpoint-event-7&after_sequence=7`,
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
      `/factory-sessions/${RESOLVED_DEFAULT_SESSION_UUID}/events`,
    );
    expect(useFactoryTimelineStore.getState().selectedTick).not.toBe(7);
    await expect(
      readTimelineCheckpoint(window.indexedDB, resolvedDefaultStreamIdentity()),
    ).resolves.toBe(null);
    expect(indexedDBRecords.has(checkpointStorageKey())).toBe(false);
  });

  it("clears a stale reconnect checkpoint and silently replays from scratch", async () => {
    useFactoryTimelineStore.getState().reset();
    indexedDBRecords.set(checkpointStorageKey(), {
      checkpoint: {
        afterEventId: "checkpoint-event-7",
        afterSequence: 7,
        replayState: emptyReplayWorldState(7),
        selectedTick: 7,
      },
      schemaVersion: 3,
      storageKey: checkpointStorageKey(),
      streamIdentity: resolvedDefaultStreamIdentity(),
    });
    getSyncPreflightSpy
      .mockResolvedValueOnce(okSyncPreflightResponse())
      .mockResolvedValueOnce(
        okSyncPreflightResponse({
          checkpointReusable: false,
          reasonCode: "cursor_stale",
          reconnectCursor: {
            afterEventId: "checkpoint-event-7",
            afterSequence: 7,
            provided: true,
            validForStreamGeneration: false,
          },
        }),
      );
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
      `/factory-sessions/${RESOLVED_DEFAULT_SESSION_UUID}/events`,
    );
    expect(useFactoryTimelineStore.getState().selectedTick).toBe(0);
    await expect(
      readTimelineCheckpoint(window.indexedDB, resolvedDefaultStreamIdentity()),
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
          resolvedDefaultStreamIdentity(),
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
      `/factory-sessions/${RESOLVED_DEFAULT_SESSION_UUID}/events`,
    );
  });
});

describe("useDashboardSnapshot session lifecycle replay", () => {
  let queryClient: QueryClient;
  let getSyncPreflightSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    replayHarness.install();
    installIndexedDBTestDouble();
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(null, {
          status: 200,
        }),
      ),
    );
    getSyncPreflightSpy = vi
      .spyOn(syncPreflightAPI, "getFactorySessionSyncPreflight")
      .mockImplementation(async (sessionID, reconnectCursor) =>
        okSyncPreflightResponseForSession(sessionID, reconnectCursor),
      );
    window.sessionStorage.clear();
    queryClient = new QueryClient({
      defaultOptions: {
        mutations: { retry: false },
        queries: { retry: false },
      },
    });
    useDashboardStreamStore.setState({
      resolvedStreamIdentity: null,
      streamState: createDefaultDashboardStreamState(),
    });
    useDashboardSessionStore.setState({
      pausedSessionIDs: [],
      selectedSessionID: DEFAULT_FACTORY_SESSION_ID,
    });
    useFactoryTimelineStore.getState().reset();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    getSyncPreflightSpy.mockRestore();
    replayHarness.reset();
    window.sessionStorage.clear();
    useDashboardStreamStore.setState({
      resolvedStreamIdentity: null,
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

    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(1);
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

    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(1);
    });
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

    await waitFor(() => {
      expect(result.current.streamState).toMatchObject({
        message: "Reconnecting event stream",
        status: "reconnecting",
      });
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
