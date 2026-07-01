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
import {
  readSessionPersistenceInvalidationRecords,
  resetSessionPersistenceInvalidationRecords,
} from "../lib/session-persistence/diagnostics";
import { useDashboardSnapshot } from "./useDashboardSnapshot";

const BACKEND_SCOPE_A = "backend-scope-a";
const BACKEND_SCOPE_B = "backend-scope-b";

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

function checkpointStorageKey(
  identity: {
    backendScopeID?: string;
    factorySessionID?: string;
    streamGenerationID?: string;
  } | string = {},
): string {
  const resolved =
    typeof identity === "string" ? { backendScopeID: identity } : identity;
  return [
    resolved.backendScopeID ?? "backend-scope-a",
    resolved.factorySessionID ?? DEFAULT_FACTORY_SESSION_ID,
    resolved.streamGenerationID ?? "2026-06-26T00:00:00Z",
  ].join("::");
}

function storedCheckpointEnvelope(
  identity: {
    backendScopeID?: string;
    factorySessionID?: string;
    streamGenerationID?: string;
  } = {},
  options: {
    afterEventId?: string;
    afterSequence?: number;
    schemaVersion?: number;
    selectedTick?: number;
    streamIdentity?: {
      backendScopeID: string;
      factorySessionID: string;
      streamGenerationID: string;
    };
  } = {},
) {
  const selectedTick = options.selectedTick ?? 7;
  return {
    checkpoint: {
      afterEventId: options.afterEventId ?? "checkpoint-event-7",
      afterSequence: options.afterSequence ?? 7,
      replayState: emptyReplayWorldState(selectedTick),
      selectedTick,
    },
    schemaVersion: options.schemaVersion ?? 2,
    sessionID: identity.factorySessionID ?? DEFAULT_FACTORY_SESSION_ID,
    storageKey: checkpointStorageKey(identity),
    ...(options.streamIdentity ? { streamIdentity: options.streamIdentity } : {}),
  };
}

function sessionResponseForBackendScope(backendScopeID: string) {
  return {
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
          backendScopeID,
          factorySessionID: DEFAULT_FACTORY_SESSION_ID,
          streamGenerationID: "2026-06-26T00:00:00Z",
        },
        status: "IDLE",
        usage: { resources: [] },
      },
      target: { kind: "default", name: DEFAULT_FACTORY_SESSION_ID },
    },
  };
}

function mockFactorySessionStreamIdentity(
  spy: ReturnType<typeof vi.spyOn>,
  streamIdentity: {
    backendScopeID: string;
    factorySessionID: string;
    streamGenerationID: string;
  },
): void {
  spy.mockResolvedValue({
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
        streamIdentity,
        status: "IDLE",
        usage: { resources: [] },
      },
      target: { kind: "default", name: DEFAULT_FACTORY_SESSION_ID },
    },
  });
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
      .mockResolvedValue(sessionResponseForBackendScope(BACKEND_SCOPE_A));
    window.sessionStorage.clear();
    queryClient = new QueryClient({
      defaultOptions: {
        mutations: { retry: false },
        queries: { retry: false },
      },
    });
    useDashboardStreamStore.setState({
      backendRuntimeCacheScope: null,
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
      backendRuntimeCacheScope: null,
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
                    backendScopeID: BACKEND_SCOPE_A,
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
          backendScopeID: BACKEND_SCOPE_A,
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
          backendScopeID: BACKEND_SCOPE_A,
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
        backendScopeID: BACKEND_SCOPE_A,
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
        backendScopeID: BACKEND_SCOPE_A,
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
        backendScopeID: BACKEND_SCOPE_A,
        factorySessionID: DEFAULT_FACTORY_SESSION_ID,
        streamGenerationID: "2026-06-26T00:00:00Z",
      }),
    ).resolves.toBe(null);
    expect(indexedDBRecords.has(checkpointStorageKey())).toBe(false);
  });

  it("bypasses the previous backend scope checkpoint and opens without the old cursor when backendScopeID changes", async () => {
    useFactoryTimelineStore.getState().reset();
    indexedDBRecords.set(checkpointStorageKey(BACKEND_SCOPE_A), {
      checkpoint: {
        afterEventId: "checkpoint-event-7",
        afterSequence: 7,
        replayState: emptyReplayWorldState(7),
        selectedTick: 7,
      },
      schemaVersion: 2,
      sessionID: DEFAULT_FACTORY_SESSION_ID,
      storageKey: checkpointStorageKey(BACKEND_SCOPE_A),
      streamIdentity: {
        backendScopeID: BACKEND_SCOPE_A,
        factorySessionID: DEFAULT_FACTORY_SESSION_ID,
        streamGenerationID: "2026-06-26T00:00:00Z",
      },
    });
    getFactorySessionSpy.mockResolvedValue(
      sessionResponseForBackendScope(BACKEND_SCOPE_B),
    );

    renderHook(() => useDashboardSnapshot(), {
      wrapper: createWrapper(queryClient),
    });

    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(1);
    });
    const streamURL = replayHarness.getStreams()[0]?.url ?? "";
    expect(streamURL).toBe(
      `/factory-sessions/${DEFAULT_FACTORY_SESSION_ID}/events`,
    );
    expect(streamURL).not.toContain("after_event_id");
    expect(streamURL).not.toContain("after_sequence");
    expect(streamURL).not.toContain("checkpoint-event-7");
    expect(useFactoryTimelineStore.getState().selectedTick).toBe(0);
    expect(useDashboardStreamStore.getState().backendRuntimeCacheScope).toBe(
      BACKEND_SCOPE_B,
    );
    await expect(
      readTimelineCheckpoint(window.indexedDB, DEFAULT_FACTORY_SESSION_ID, {
        backendScopeID: BACKEND_SCOPE_B,
        factorySessionID: DEFAULT_FACTORY_SESSION_ID,
        streamGenerationID: "2026-06-26T00:00:00Z",
      }),
    ).resolves.toBe(null);
    expect(indexedDBRecords.has(checkpointStorageKey(BACKEND_SCOPE_A))).toBe(
      true,
    );
  });

  it("skips checkpoint restore when session identity is missing backend scope", async () => {
    useFactoryTimelineStore.getState().reset();
    indexedDBRecords.set(checkpointStorageKey(BACKEND_SCOPE_A), {
      checkpoint: {
        afterEventId: "checkpoint-event-7",
        afterSequence: 7,
        replayState: emptyReplayWorldState(7),
        selectedTick: 7,
      },
      schemaVersion: 2,
      sessionID: DEFAULT_FACTORY_SESSION_ID,
      storageKey: checkpointStorageKey(BACKEND_SCOPE_A),
      streamIdentity: {
        backendScopeID: BACKEND_SCOPE_A,
        factorySessionID: DEFAULT_FACTORY_SESSION_ID,
        streamGenerationID: "2026-06-26T00:00:00Z",
      },
    });
    getFactorySessionSpy.mockResolvedValue({
      session: {
        ...sessionResponseForBackendScope(BACKEND_SCOPE_A).session,
        runtime: {
          ...sessionResponseForBackendScope(BACKEND_SCOPE_A).session.runtime,
          streamIdentity: {
            factorySessionID: DEFAULT_FACTORY_SESSION_ID,
            streamGenerationID: "2026-06-26T00:00:00Z",
          },
        },
      },
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
    expect(useFactoryTimelineStore.getState().selectedTick).toBe(0);
    expect(useDashboardStreamStore.getState().backendRuntimeCacheScope).toBe(
      null,
    );
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
            backendScopeID: BACKEND_SCOPE_A,
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

describe("useDashboardSnapshot checkpoint identity recovery", () => {
  let indexedDBRecords: Map<string, unknown>;
  let queryClient: QueryClient;
  let getFactorySessionSpy: ReturnType<typeof vi.spyOn>;

  const currentStreamIdentity = {
    backendScopeID: "backend-scope-a",
    factorySessionID: DEFAULT_FACTORY_SESSION_ID,
    streamGenerationID: "2026-06-26T00:00:00Z",
  };

  beforeEach(() => {
    resetSessionPersistenceInvalidationRecords();
    replayHarness.install();
    indexedDBRecords = installIndexedDBTestDouble();
    getFactorySessionSpy = vi.spyOn(factorySessionsAPI, "getFactorySession");
    mockFactorySessionStreamIdentity(getFactorySessionSpy, currentStreamIdentity);
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
    resetSessionPersistenceInvalidationRecords();
    getFactorySessionSpy.mockRestore();
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

  it("reopens the event stream with the saved cursor when checkpoint identity matches", async () => {
    indexedDBRecords.set(
      checkpointStorageKey(currentStreamIdentity),
      storedCheckpointEnvelope(currentStreamIdentity, {
        streamIdentity: currentStreamIdentity,
      }),
    );

    renderHook(() => useDashboardSnapshot(), {
      wrapper: createWrapper(queryClient),
    });

    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(1);
    });
    expect(replayHarness.getStreams()[0]?.url).toBe(
      `/factory-sessions/${DEFAULT_FACTORY_SESSION_ID}/events?after_event_id=checkpoint-event-7&after_sequence=7`,
    );
    expect(useFactoryTimelineStore.getState().selectedTick).toBe(7);
  });

  it("ignores a stale checkpoint when backendScopeID changes and reconnects without the old cursor", async () => {
    const staleIdentity = {
      ...currentStreamIdentity,
      backendScopeID: "backend-scope-stale",
    };
    indexedDBRecords.set(
      checkpointStorageKey(staleIdentity),
      storedCheckpointEnvelope(staleIdentity, {
        streamIdentity: staleIdentity,
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
    expect(readSessionPersistenceInvalidationRecords()).toEqual([]);
  });

  it("ignores a stale checkpoint when streamGenerationID changes and reconnects without the old cursor", async () => {
    const staleIdentity = {
      ...currentStreamIdentity,
      streamGenerationID: "2026-06-25T00:00:00Z",
    };
    indexedDBRecords.set(
      checkpointStorageKey(staleIdentity),
      storedCheckpointEnvelope(staleIdentity, {
        streamIdentity: staleIdentity,
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
  });

  it("ignores a remapped factorySessionID checkpoint and reconnects without the old cursor", async () => {
    const staleIdentity = {
      ...currentStreamIdentity,
      factorySessionID: "session-remapped-uuid",
    };
    indexedDBRecords.set(
      checkpointStorageKey(staleIdentity),
      storedCheckpointEnvelope(staleIdentity, {
        streamIdentity: staleIdentity,
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
  });

  it("deletes identity-missing v2 checkpoints and reconnects without after_event_id or after_sequence", async () => {
    indexedDBRecords.set(
      checkpointStorageKey(currentStreamIdentity),
      storedCheckpointEnvelope(currentStreamIdentity, {
        schemaVersion: 2,
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
        backendScopeID: currentStreamIdentity.backendScopeID,
        factorySessionID: currentStreamIdentity.factorySessionID,
        streamGenerationID: currentStreamIdentity.streamGenerationID,
      }),
    ).resolves.toBe(null);
  });

  it("records a sanitized diagnostic when stored checkpoint identity mismatches the current stream identity", async () => {
    indexedDBRecords.set(
      checkpointStorageKey(currentStreamIdentity),
      storedCheckpointEnvelope(currentStreamIdentity, {
        streamIdentity: {
          backendScopeID: "backend-scope-a",
          factorySessionID: DEFAULT_FACTORY_SESSION_ID,
          streamGenerationID: "2026-06-25T00:00:00Z",
        },
      }),
    );

    renderHook(() => useDashboardSnapshot(), {
      wrapper: createWrapper(queryClient),
    });

    await waitFor(() => {
      expect(readSessionPersistenceInvalidationRecords()).toEqual([
        expect.objectContaining({
          reason: "stream_generation_changed",
          recoveryAction: "clear_stream_derived_state",
          requestedSessionID: DEFAULT_FACTORY_SESSION_ID,
        }),
      ]);
    });
    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(1);
    });
    expect(replayHarness.getStreams()[0]?.url).toBe(
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
