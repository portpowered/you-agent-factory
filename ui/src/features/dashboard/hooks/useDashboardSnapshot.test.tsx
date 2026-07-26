import "../../../testing/vitest-dom-capabilities.setup";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { vi } from "vitest";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { FACTORY_EVENT_TYPES } from "../../../api/events";
import * as factorySessionsAPI from "../../../api/factory-sessions";
import { FACTORY_SESSIONS_QUERY_KEY } from "../../../api/factory-sessions/query-keys";
import { FactorySessionSyncPreflightReasonCode } from "../../../api/generated/openapi";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import { DashboardSessionStoreTestProvider } from "../../../testing/dashboard-session-test-provider";
import { createReplayHarness } from "../../../testing/replay-harness";
import {
  type canonicalSessionLifecycleReplayEvents,
  sessionLifecycleControlPauseEvent,
  sessionLifecycleControlResumeEvent,
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
import { createMaterializedWorkOutcomeState } from "../../work-outcome/public/materializer";
import {
  readSessionPersistenceDiagnosticRecords,
  resetSessionPersistenceDiagnosticRecords,
} from "../lib/session-persistence/diagnostics";
import { useDashboardSessionStore } from "../state/dashboardSessionStore";
import {
  createDefaultDashboardStreamState,
  useDashboardStreamStore,
} from "../state/dashboardStreamStore";
import { useDashboardSnapshot } from "./useDashboardSnapshot";

const replayHarness = createReplayHarness();

const DEFAULT_STREAM_GENERATION_ID = "2026-06-26T00:00:00Z";
const DEFAULT_BACKEND_SCOPE_ID = "backend-scope-a";
const DEFAULT_LOGICAL_SESSION_KEY_ID = "lsk-default-folder";
const DEFAULT_RUNTIME_FACTORY_SESSION_ID =
  "550e8400-e29b-41d4-a716-446655440000";
const RESOLVED_DEFAULT_SESSION_UUID = DEFAULT_RUNTIME_FACTORY_SESSION_ID;
const STALE_FACTORY_SESSION_UUID = "66666666-6666-4666-8666-666666666666";
const BETA_FACTORY_SESSION_UUID = "77777777-7777-4777-8777-777777777777";
const REMAPPED_FACTORY_SESSION_UUID = "88888888-8888-4888-8888-888888888888";

function defaultStreamIdentity(
  factorySessionID = RESOLVED_DEFAULT_SESSION_UUID,
) {
  return {
    backendScopeID: DEFAULT_BACKEND_SCOPE_ID,
    factorySessionID,
    logicalSessionKeyID: DEFAULT_LOGICAL_SESSION_KEY_ID,
    streamGenerationID: DEFAULT_STREAM_GENERATION_ID,
  };
}

function buildSyncPreflightResponse(
  overrides: Partial<factorySessionsAPI.FactorySessionSyncPreflightResponse> = {},
): factorySessionsAPI.FactorySessionSyncPreflightResponse {
  return {
    backendScopeId: DEFAULT_BACKEND_SCOPE_ID,
    checkpointReusable: true,
    factorySessionId: RESOLVED_DEFAULT_SESSION_UUID,
    logicalSessionKeyId: DEFAULT_LOGICAL_SESSION_KEY_ID,
    reasonCode: FactorySessionSyncPreflightReasonCode.ok,
    reconnectCursor: {
      provided: false,
      validForStreamGeneration: true,
    },
    requestedSessionId: DEFAULT_FACTORY_SESSION_ID,
    streamGenerationId: DEFAULT_STREAM_GENERATION_ID,
    ...overrides,
  };
}
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
    transaction: () => {
      const transaction = {
        oncomplete: null,
        objectStore: () => ({
          delete: (key: string) =>
            indexedDBRequest(
              undefined,
              () => {
                records.delete(key);
              },
              () =>
                (transaction.oncomplete as ((event: Event) => void) | null)?.(
                  {} as Event,
                ),
            ),
          get: (key: string) => indexedDBRequest(records.get(key)),
          getAll: () => indexedDBRequest([...records.values()]),
          put: (value: { sessionID: string; storageKey?: string }) =>
            indexedDBRequest(value.storageKey ?? value.sessionID, () => {
              records.set(value.storageKey ?? value.sessionID, value);
            }),
        }),
      };
      return transaction;
    },
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

function indexedDBRequest<T>(
  result: T,
  beforeSuccess?: () => void,
  afterSuccess?: () => void,
) {
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
    afterSuccess?.();
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
  factorySessionID = RESOLVED_DEFAULT_SESSION_UUID,
): string {
  return [
    DEFAULT_BACKEND_SCOPE_ID,
    factorySessionID,
    DEFAULT_LOGICAL_SESSION_KEY_ID,
    DEFAULT_STREAM_GENERATION_ID,
  ].join("::");
}

describe("useDashboardSnapshot composer", () => {
  let indexedDBRecords: Map<string, unknown>;
  let queryClient: QueryClient;
  let getFactorySessionSyncPreflightSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    resetSessionPersistenceDiagnosticRecords();
    replayHarness.install();
    window.history.replaceState({}, "", "/");
    indexedDBRecords = installIndexedDBTestDouble();
    getFactorySessionSyncPreflightSpy = vi
      .spyOn(factorySessionsAPI, "getFactorySessionSyncPreflight")
      .mockResolvedValue(buildSyncPreflightResponse());
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
    resetSessionPersistenceDiagnosticRecords();
    window.history.replaceState({}, "", "/");
    vi.unstubAllGlobals();
    getFactorySessionSyncPreflightSpy.mockRestore();
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

  it("preflights an unresolved default without borrowing concrete identity hints", async () => {
    indexedDBRecords.set(checkpointStorageKey(), {
      checkpoint: {
        afterEventId: "event-2",
        afterSequence: 2,
        materializedWorkOutcomeState: createMaterializedWorkOutcomeState(),
        replayState: emptyReplayWorldState(17),
        selectedTick: 17,
      },
      schemaVersion: 4,
      storageKey: checkpointStorageKey(),
      streamIdentity: defaultStreamIdentity(),
    });
    getFactorySessionSyncPreflightSpy.mockResolvedValue(
      buildSyncPreflightResponse({
        checkpointReusable: true,
        reconnectCursor: {
          afterEventId: "event-2",
          afterSequence: 2,
          provided: true,
          validForStreamGeneration: true,
        },
      }),
    );

    renderHook(() => useDashboardSnapshot(), {
      wrapper: createWrapper(queryClient),
    });

    await waitFor(() => {
      expect(getFactorySessionSyncPreflightSpy).toHaveBeenCalledWith(
        DEFAULT_FACTORY_SESSION_ID,
        undefined,
        { signal: expect.any(AbortSignal) },
      );
    });
    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(1);
    });
    await waitFor(() => {
      expect(useFactoryTimelineStore.getState().selectedTick).toBe(17);
    });
  });

  it("surfaces a recoverable preflight recovery state when sync preflight cannot resolve the session", async () => {
    useFactoryTimelineStore.getState().reset();
    getFactorySessionSyncPreflightSpy.mockResolvedValue(
      buildSyncPreflightResponse({
        checkpointReusable: false,
        reasonCode: FactorySessionSyncPreflightReasonCode.session_not_found,
        reconnectCursor: {
          provided: false,
          validForStreamGeneration: false,
        },
      }),
    );

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
    getFactorySessionSyncPreflightSpy.mockImplementation(async (sessionID) =>
      buildSyncPreflightResponse({
        factorySessionId:
          sessionID === DEFAULT_FACTORY_SESSION_ID
            ? RESOLVED_DEFAULT_SESSION_UUID
            : sessionID,
        requestedSessionId: sessionID,
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

    act(() => {
      useDashboardSessionStore.getState().setSelectedSessionID("session-beta");
    });

    await waitFor(() => {
      expect(replayHarness.getStreams().length).toBeGreaterThanOrEqual(2);
    });
    expect(replayHarness.getStreams().at(-1)?.url).toBe(
      "/factory-sessions/session-beta/events",
    );
    await waitFor(() => {
      expect(useDashboardStreamStore.getState().resolvedStreamIdentity).toEqual(
        defaultStreamIdentity("session-beta"),
      );
    });
  });

  it("routes streamed events through the composer into timeline state", async () => {
    useFactoryTimelineStore.getState().reset();

    renderHook(() => useDashboardSnapshot(), {
      wrapper: createWrapper(queryClient),
    });

    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(1);
    });
    expect(useDashboardStreamStore.getState().resolvedStreamIdentity).toEqual(
      defaultStreamIdentity(),
    );
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

  it("advances replay and materialized outcomes when checkpoint persistence is disabled", async () => {
    window.history.replaceState({}, "", "/?afDisableTimelineCheckpoint=1");
    useFactoryTimelineStore.getState().reset();

    const { result } = renderHook(() => useDashboardSnapshot(), {
      wrapper: createWrapper(queryClient),
    });

    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(1);
    });
    expect(getFactorySessionSyncPreflightSpy).not.toHaveBeenCalled();

    await act(async () => {
      replayHarness.getStreams()[0]?.emit("message", {
        context: {
          eventTime: "2026-07-13T10:00:01Z",
          sequence: 1,
          tick: 1,
        },
        id: "disabled-checkpoint-work-request",
        payload: {
          source: "api",
          type: "FACTORY_REQUEST_BATCH",
          works: [
            {
              name: "Diagnostic timeline story",
              traceId: "trace-disabled-checkpoint",
              workId: "work-disabled-checkpoint",
              workTypeName: "story",
            },
          ],
        },
        type: FACTORY_EVENT_TYPES.workRequest,
      });
      await new Promise<void>((resolve) => {
        window.setTimeout(() => resolve(), 20);
      });
    });

    await waitFor(() => {
      expect(result.current.snapshot?.tick_count).toBe(1);
    });
    expect(useFactoryTimelineStore.getState()).toMatchObject({
      currentReplayCheckpoint: {
        afterEventId: "disabled-checkpoint-work-request",
        afterSequence: 1,
        selectedTick: 1,
      },
      events: [{ id: "disabled-checkpoint-work-request" }],
      latestTick: 1,
      materializedWorkOutcomeState: {
        accumulator: { appliedEventCount: 1 },
        cursor: {
          eventID: "disabled-checkpoint-work-request",
          sequence: 1,
          tick: 1,
        },
      },
    });
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
        readTimelineCheckpoint(window.indexedDB, defaultStreamIdentity()),
      ).resolves.toEqual(
        expect.objectContaining({
          afterEventId: "checkpoint-event-7",
          afterSequence: 7,
          selectedTick: 7,
        }),
      );
    });
    await act(async () => {
      await new Promise<void>((resolve) => {
        window.setTimeout(() => resolve(), 800);
      });
    });

    unmount();
    replayHarness.reset();
    replayHarness.install();
    useFactoryTimelineStore.getState().reset();
    getFactorySessionSyncPreflightSpy.mockResolvedValue(
      buildSyncPreflightResponse({
        checkpointReusable: true,
        reconnectCursor: {
          afterEventId: "checkpoint-event-7",
          afterSequence: 7,
          provided: true,
          validForStreamGeneration: true,
        },
      }),
    );

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
    useFactoryTimelineStore.getState().reset();
    indexedDBRecords.set(checkpointStorageKey(), {
      checkpoint: {
        afterEventId: "legacy-event-7",
        afterSequence: 7,
        replayState: emptyReplayWorldState(7),
        selectedTick: 7,
      },
      schemaVersion: 1,
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
      readTimelineCheckpoint(window.indexedDB, defaultStreamIdentity()),
    ).resolves.toBe(null);
    expect(indexedDBRecords.has(checkpointStorageKey())).toBe(false);
  });

  it("does not clear a concrete checkpoint for an unresolved default", async () => {
    useFactoryTimelineStore.getState().reset();
    indexedDBRecords.set(checkpointStorageKey(), {
      checkpoint: {
        afterEventId: "checkpoint-event-7",
        afterSequence: 7,
        materializedWorkOutcomeState: createMaterializedWorkOutcomeState(),
        replayState: emptyReplayWorldState(7),
        selectedTick: 7,
      },
      schemaVersion: 4,
      storageKey: checkpointStorageKey(),
      streamIdentity: defaultStreamIdentity(),
    });
    getFactorySessionSyncPreflightSpy.mockResolvedValue(
      buildSyncPreflightResponse({
        checkpointReusable: false,
        reasonCode: FactorySessionSyncPreflightReasonCode.cursor_stale,
        reconnectCursor: {
          afterEventId: "checkpoint-event-7",
          afterSequence: 7,
          provided: true,
          validForStreamGeneration: false,
        },
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
    await waitFor(() => {
      expect(useFactoryTimelineStore.getState().selectedTick).toBe(0);
    });
    await expect(
      readTimelineCheckpoint(window.indexedDB, defaultStreamIdentity()),
    ).resolves.toEqual(expect.objectContaining({ selectedTick: 7 }));
    expect(indexedDBRecords.has(checkpointStorageKey())).toBe(true);
    const recoveryDiagnostics =
      readSessionPersistenceDiagnosticRecords().filter(({ outcome }) =>
        ["stale_cursor", "cursor_free_replay_fallback"].includes(outcome),
      );
    expect(recoveryDiagnostics.map((record) => record.outcome)).toEqual([
      "stale_cursor",
      "cursor_free_replay_fallback",
    ]);
    expect(recoveryDiagnostics[0]?.correlationToken).toBe(
      recoveryDiagnostics[1]?.correlationToken,
    );
  });

  it("remaps the ~default alias to the resolved UUID runtime identity", async () => {
    useDashboardSessionStore.setState({
      pausedSessionIDs: [],
      selectedSessionID: DEFAULT_FACTORY_SESSION_ID,
    });
    getFactorySessionSyncPreflightSpy.mockResolvedValue(
      buildSyncPreflightResponse({
        checkpointReusable: true,
        factorySessionId: DEFAULT_RUNTIME_FACTORY_SESSION_ID,
        requestedSessionId: DEFAULT_FACTORY_SESSION_ID,
        streamGenerationId: `${DEFAULT_BACKEND_SCOPE_ID}::${DEFAULT_RUNTIME_FACTORY_SESSION_ID}`,
      }),
    );

    renderHook(() => useDashboardSnapshot(), {
      wrapper: createWrapper(queryClient),
    });

    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(1);
    });
    expect(replayHarness.getStreams()[0]?.url).toBe(
      `/factory-sessions/${DEFAULT_RUNTIME_FACTORY_SESSION_ID}/events`,
    );
    expect(useDashboardSessionStore.getState().selectedSessionID).toBe(
      DEFAULT_FACTORY_SESSION_ID,
    );
    expect(indexedDBRecords.has(checkpointStorageKey())).toBe(false);
  });

  it("remaps a stale factory session id through logical identity without reusing the reconnect cursor", async () => {
    const remappedSessionID = REMAPPED_FACTORY_SESSION_UUID;
    const staleSessionID = STALE_FACTORY_SESSION_UUID;
    const betaSessionID = BETA_FACTORY_SESSION_UUID;
    const staleStreamIdentity = defaultStreamIdentity(staleSessionID);
    indexedDBRecords.set(checkpointStorageKey(staleSessionID), {
      checkpoint: {
        afterEventId: "checkpoint-event-7",
        afterSequence: 7,
        materializedWorkOutcomeState: createMaterializedWorkOutcomeState(),
        replayState: emptyReplayWorldState(7),
        selectedTick: 7,
        syncIdentity: {
          backendScopeId: DEFAULT_BACKEND_SCOPE_ID,
          factorySessionId: staleSessionID,
          logicalSessionKeyId: DEFAULT_LOGICAL_SESSION_KEY_ID,
          streamGenerationId: DEFAULT_STREAM_GENERATION_ID,
        },
      },
      schemaVersion: 4,
      storageKey: checkpointStorageKey(staleSessionID),
      streamIdentity: staleStreamIdentity,
    });
    indexedDBRecords.set(checkpointStorageKey(betaSessionID), {
      checkpoint: {
        afterEventId: "beta-event-9",
        afterSequence: 9,
        materializedWorkOutcomeState: createMaterializedWorkOutcomeState(),
        replayState: emptyReplayWorldState(9),
        selectedTick: 9,
      },
      schemaVersion: 4,
      storageKey: checkpointStorageKey(betaSessionID),
      streamIdentity: defaultStreamIdentity(betaSessionID),
    });
    queryClient.setQueryData(["session-isolation", staleSessionID], {
      value: "stale",
    });
    queryClient.setQueryData(["session-isolation", betaSessionID], {
      value: "beta",
    });
    queryClient.setQueryData(FACTORY_SESSIONS_QUERY_KEY, [
      {
        factoryDir: "/workspace/default",
        folderPath: "/workspace/default",
        id: staleSessionID,
        isDefault: true,
        project: "default",
        target: { kind: "default" },
      },
      {
        factoryDir: "/workspace/beta",
        folderPath: "/workspace/beta",
        id: betaSessionID,
        isDefault: false,
        project: "beta",
        target: { kind: "named", name: "beta" },
      },
    ]);
    useDashboardSessionStore.setState({
      pausedSessionIDs: [staleSessionID, betaSessionID],
      selectedSessionID: staleSessionID,
      sessionTabOrder: [staleSessionID, betaSessionID],
    });
    getFactorySessionSyncPreflightSpy.mockImplementation(async (sessionID) =>
      buildSyncPreflightResponse({
        checkpointReusable: false,
        factorySessionId: remappedSessionID,
        reasonCode:
          sessionID === remappedSessionID
            ? FactorySessionSyncPreflightReasonCode.ok
            : FactorySessionSyncPreflightReasonCode.logical_session_remap,
        reconnectCursor: {
          afterEventId: "checkpoint-event-7",
          afterSequence: 7,
          provided: true,
          validForStreamGeneration: false,
        },
        requestedSessionId: sessionID,
        streamGenerationId: DEFAULT_STREAM_GENERATION_ID,
      }),
    );

    renderHook(() => useDashboardSnapshot(), {
      wrapper: createWrapper(queryClient),
    });

    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(1);
    });
    expect(replayHarness.getStreams()[0]?.url).toBe(
      `/factory-sessions/${remappedSessionID}/events`,
    );
    expect(useFactoryTimelineStore.getState().selectedTick).toBe(0);
    expect(useDashboardSessionStore.getState().selectedSessionID).toBe(
      remappedSessionID,
    );
    expect(useDashboardSessionStore.getState().sessionTabOrder).toEqual([
      remappedSessionID,
      betaSessionID,
    ]);
    expect(useDashboardSessionStore.getState().pausedSessionIDs).toEqual([
      betaSessionID,
    ]);
    expect(indexedDBRecords.has(checkpointStorageKey(staleSessionID))).toBe(
      false,
    );
    expect(indexedDBRecords.has(checkpointStorageKey(betaSessionID))).toBe(
      true,
    );
    expect(
      queryClient.getQueryData(["session-isolation", betaSessionID]),
    ).toEqual({ value: "beta" });
    expect(
      queryClient
        .getQueryData<Array<{ id: string }>>(FACTORY_SESSIONS_QUERY_KEY)
        ?.map((session) => session.id),
    ).toEqual([remappedSessionID, betaSessionID]);
  });

  it("remaps a stale session using envelope stream identity when checkpoint sync identity is absent", async () => {
    const remappedSessionID = REMAPPED_FACTORY_SESSION_UUID;
    const staleSessionID = STALE_FACTORY_SESSION_UUID;
    const staleStreamIdentity = defaultStreamIdentity(staleSessionID);
    indexedDBRecords.set(checkpointStorageKey(staleSessionID), {
      checkpoint: {
        afterEventId: "checkpoint-event-7",
        afterSequence: 7,
        materializedWorkOutcomeState: createMaterializedWorkOutcomeState(),
        replayState: emptyReplayWorldState(7),
        selectedTick: 7,
      },
      schemaVersion: 4,
      storageKey: checkpointStorageKey(staleSessionID),
      streamIdentity: staleStreamIdentity,
    });
    useDashboardSessionStore.setState({
      pausedSessionIDs: [],
      selectedSessionID: staleSessionID,
    });
    getFactorySessionSyncPreflightSpy.mockImplementation(async (sessionID) =>
      buildSyncPreflightResponse({
        checkpointReusable: false,
        factorySessionId: remappedSessionID,
        reasonCode:
          sessionID === remappedSessionID
            ? FactorySessionSyncPreflightReasonCode.ok
            : FactorySessionSyncPreflightReasonCode.logical_session_remap,
        reconnectCursor: {
          afterEventId: "checkpoint-event-7",
          afterSequence: 7,
          provided: true,
          validForStreamGeneration: false,
        },
        requestedSessionId: sessionID,
        streamGenerationId: DEFAULT_STREAM_GENERATION_ID,
      }),
    );

    renderHook(() => useDashboardSnapshot(), {
      wrapper: createWrapper(queryClient),
    });

    await waitFor(() => {
      expect(getFactorySessionSyncPreflightSpy).toHaveBeenCalledWith(
        staleSessionID,
        {
          afterEventId: "checkpoint-event-7",
          afterSequence: 7,
        },
        expect.objectContaining({
          backendScopeId: DEFAULT_BACKEND_SCOPE_ID,
          logicalSessionKeyId: DEFAULT_LOGICAL_SESSION_KEY_ID,
        }),
      );
    });
    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(1);
    });
    expect(replayHarness.getStreams()[0]?.url).toBe(
      `/factory-sessions/${remappedSessionID}/events`,
    );
    expect(useDashboardSessionStore.getState().selectedSessionID).toBe(
      remappedSessionID,
    );
    expect(indexedDBRecords.has(checkpointStorageKey(staleSessionID))).toBe(
      false,
    );
  });

  it("surfaces typed preflight recovery for unresolved logical targets", async () => {
    getFactorySessionSyncPreflightSpy.mockResolvedValue({
      checkpointReusable: false,
      reasonCode: FactorySessionSyncPreflightReasonCode.session_not_found,
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
        reasonCode: FactorySessionSyncPreflightReasonCode.session_not_found,
        requestedSessionId: DEFAULT_FACTORY_SESSION_ID,
      });
    });
    expect(result.current.error).toBeNull();
    expect(replayHarness.getStreams()).toHaveLength(0);
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
      .spyOn(factorySessionsAPI, "getFactorySessionSyncPreflight")
      .mockImplementation(async (sessionID) =>
        buildSyncPreflightResponse({
          factorySessionId:
            sessionID === DEFAULT_FACTORY_SESSION_ID
              ? RESOLVED_DEFAULT_SESSION_UUID
              : sessionID,
          requestedSessionId: sessionID,
        }),
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

  it("projects paused and resumed Factory Session lifecycle from streamed SESSION_LIFECYCLE_CONTROL events", async () => {
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
    await emitStreamMessage(stream, sessionLifecycleControlPauseEvent);

    await waitFor(() => {
      expect(result.current.snapshot?.runtime?.session?.bracket).toMatchObject({
        lifecycle_control_status: "PAUSED",
        paused_at: "2026-06-09T12:00:02Z",
        session_id: "session-alpha",
      });
    });

    await emitStreamMessage(stream, sessionLifecycleControlResumeEvent);

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

  it("keeps SESSION_LIFECYCLE_CONTROL lifecycle reflection after event-stream reconnect", async () => {
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
    await emitStreamMessage(stream, sessionLifecycleControlPauseEvent);

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
      "after_event_id=session-lifecycle-control%2Fsession-alpha%2F2",
    );
    expect(reconnectStream.url).toContain("after_sequence=2");

    await emitStreamMessage(
      reconnectStream,
      sessionLifecycleControlResumeEvent,
    );

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
        <DashboardSessionStoreTestProvider>
          {children}
        </DashboardSessionStoreTestProvider>
      </QueryClientProvider>
    );
  };
}
