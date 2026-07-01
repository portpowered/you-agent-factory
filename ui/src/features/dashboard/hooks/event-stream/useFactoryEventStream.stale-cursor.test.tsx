import type { QueryClient } from "@tanstack/react-query";
import { QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { PropsWithChildren } from "react";

import { DEFAULT_FACTORY_SESSION_ID } from "../../../../api/session-routing";
import { createReplayHarness } from "../../../../testing/replay-harness";
import {
  currentFactoryDefinitionQueryKey,
  currentFactoryDocumentQueryKey,
} from "../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import { factorySessionDetailQueryKey } from "../../../factory-session-detail/hooks/use-factory-session-detail";
import * as timelineCheckpointPersistence from "../../../timeline/state/timelineCheckpointPersistence";
import { useFactoryTimelineStore } from "../../../timeline/state/factoryTimelineStore";
import { DashboardSessionProvider } from "../../session/dashboard-session-provider";
import { useDashboardSessionStore } from "../../state/dashboardSessionStore";
import {
  createDefaultDashboardStreamState,
  useDashboardStreamStore,
} from "../../state/dashboardStreamStore";
import { useFactoryEventStream } from "./useFactoryEventStream";
import {
  SEEDED_SNAPSHOT,
  createFactoryEventStreamQueryClient,
  timelineSnapshot,
} from "./useFactoryEventStream.fixtures";

const replayHarness = createReplayHarness();

function seedFactoryEventStreamStores(): void {
  useDashboardStreamStore.setState({
    backendRuntimeCacheScope: TEST_BACKEND_SCOPE,
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
}

function resetFactoryEventStreamStores(): void {
  replayHarness.reset();
  useDashboardStreamStore.setState({
    backendRuntimeCacheScope: null,
    streamState: createDefaultDashboardStreamState(),
  });
  useDashboardSessionStore.setState({
    pausedSessionIDs: [],
    selectedSessionID: DEFAULT_FACTORY_SESSION_ID,
  });
  useFactoryTimelineStore.getState().reset();
}

const TEST_BACKEND_SCOPE = "backend-scope-a";

function installIndexedDBStub(): void {
  Object.defineProperty(window, "indexedDB", {
    configurable: true,
    value: {
      open: () => {
        const request = {
          onsuccess: null,
          onerror: null,
          onupgradeneeded: null,
          result: {
            close: () => {},
            transaction: () => ({
              objectStore: () => ({
                delete: () => {
                  const deleteRequest = {
                    onsuccess: null,
                    onerror: null,
                  };
                  queueMicrotask(() => deleteRequest.onsuccess?.({} as Event));
                  return deleteRequest;
                },
              }),
            }),
          },
        };
        queueMicrotask(() => request.onsuccess?.({} as Event));
        return request;
      },
    },
  });
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: stale-cursor scenarios share one store harness and query client setup.
describe("useFactoryEventStream stale cursor recovery", () => {
  let queryClient = createFactoryEventStreamQueryClient();

  beforeEach(() => {
    replayHarness.install();
    installIndexedDBStub();
    queryClient = createFactoryEventStreamQueryClient();
    seedFactoryEventStreamStores();
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(null, {
          status: 200,
        }),
      ),
    );
  });

  afterEach(() => {
    resetFactoryEventStreamStores();
    vi.unstubAllGlobals();
  });

  it("clears only the affected session checkpoint and runtime queries before replaying from scratch", async () => {
    const probeRecovery = vi.fn().mockResolvedValue({
      factorySessionId: DEFAULT_FACTORY_SESSION_ID,
      outcome: "CURSOR_STALE",
      retry: {
        omitAfterEventId: true,
        omitAfterSequence: true,
      },
    });
    const deleteCheckpoint = vi.spyOn(
      timelineCheckpointPersistence,
      "clearTimelineCheckpoint",
    );

    queryClient.setQueryData(
      currentFactoryDefinitionQueryKey(
        DEFAULT_FACTORY_SESSION_ID,
        TEST_BACKEND_SCOPE,
      ),
      {
      workers: [],
      workstations: [],
      workTypes: [],
    });
    queryClient.setQueryData(
      currentFactoryDocumentQueryKey(
        DEFAULT_FACTORY_SESSION_ID,
        TEST_BACKEND_SCOPE,
      ),
      {
      name: "default",
      version: { logical: "1", physical: "2026-06-26T00:00:00Z" },
      workers: [],
      workstations: [],
      workTypes: [],
    });
    queryClient.setQueryData(
      factorySessionDetailQueryKey(
        DEFAULT_FACTORY_SESSION_ID,
        TEST_BACKEND_SCOPE,
      ),
      { status: "success" },
    );
    queryClient.setQueryData(
      currentFactoryDefinitionQueryKey("session-beta", TEST_BACKEND_SCOPE),
      {
      workers: [{ name: "kept" }],
    });

    renderHook(
      () =>
        useFactoryEventStream({
          enabled: true,
          initialReconnectCursor: {
            afterEventId: "checkpoint-event-7",
            afterSequence: 7,
          },
          locale: "en",
          onEvent: () => {},
          probeRecovery,
          sessionID: DEFAULT_FACTORY_SESSION_ID,
          streamIdentity: {
            backendScopeID: TEST_BACKEND_SCOPE,
            factorySessionID: DEFAULT_FACTORY_SESSION_ID,
            streamGenerationID: "2026-06-26T00:00:00Z",
          },
          validateReconnectCursor: vi.fn().mockResolvedValue({ ok: true }),
        }),
      { wrapper: createWrapper(queryClient) },
    );

    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(1);
    });
    const initialStream = replayHarness.getStreams()[0];
    if (!initialStream) {
      throw new Error("expected initial reconnect stream to be opened");
    }

    act(() => {
      initialStream.onerror?.(new Event("error"));
    });

    await waitFor(() => {
      expect(probeRecovery).toHaveBeenCalledWith(
        DEFAULT_FACTORY_SESSION_ID,
        {
          afterEventId: "checkpoint-event-7",
          afterSequence: 7,
        },
      );
    });

    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(2);
    });
    expect(replayHarness.getStreams()[1]?.url).toBe(
      `/factory-sessions/${DEFAULT_FACTORY_SESSION_ID}/events`,
    );
    expect(deleteCheckpoint).toHaveBeenCalledWith(window.indexedDB, {
      backendScopeID: TEST_BACKEND_SCOPE,
      factorySessionID: DEFAULT_FACTORY_SESSION_ID,
      streamGenerationID: "2026-06-26T00:00:00Z",
    });
    expect(
      queryClient.getQueryData(
        currentFactoryDefinitionQueryKey(
          DEFAULT_FACTORY_SESSION_ID,
          TEST_BACKEND_SCOPE,
        ),
      ),
    ).toBeUndefined();
    expect(
      queryClient.getQueryData(
        currentFactoryDocumentQueryKey(
          DEFAULT_FACTORY_SESSION_ID,
          TEST_BACKEND_SCOPE,
        ),
      ),
    ).toBeUndefined();
    expect(
      queryClient.getQueryData(
        factorySessionDetailQueryKey(
          DEFAULT_FACTORY_SESSION_ID,
          TEST_BACKEND_SCOPE,
        ),
      ),
    ).toBeUndefined();
    expect(
      queryClient.getQueryData(
        currentFactoryDefinitionQueryKey("session-beta", TEST_BACKEND_SCOPE),
      ),
    ).toEqual({ workers: [{ name: "kept" }] });
    expect(useDashboardStreamStore.getState().streamState.status).not.toBe(
      "recovery_failed",
    );
  });

  it("shows a recoverable stream state when replay from scratch cannot reopen the session", async () => {
    const probeRecovery = vi.fn().mockResolvedValue({
      factorySessionId: DEFAULT_FACTORY_SESSION_ID,
      outcome: "CURSOR_STALE",
      retry: {
        omitAfterEventId: true,
        omitAfterSequence: true,
      },
    });

    renderHook(
      () =>
        useFactoryEventStream({
          enabled: true,
          initialReconnectCursor: {
            afterEventId: "checkpoint-event-7",
            afterSequence: 7,
          },
          locale: "en",
          onEvent: () => {},
          probeRecovery,
          sessionID: DEFAULT_FACTORY_SESSION_ID,
          streamIdentity: {
            backendScopeID: TEST_BACKEND_SCOPE,
            factorySessionID: DEFAULT_FACTORY_SESSION_ID,
            streamGenerationID: "2026-06-26T00:00:00Z",
          },
          validateReconnectCursor: vi.fn().mockResolvedValue({ ok: true }),
        }),
      { wrapper: createWrapper(queryClient) },
    );

    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(1);
    });
    const initialStream = replayHarness.getStreams()[0];
    if (!initialStream) {
      throw new Error("expected initial reconnect stream to be opened");
    }

    act(() => {
      initialStream.onerror?.(new Event("error"));
    });

    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(2);
    });

    const replayStream = replayHarness.getStreams()[1];
    if (!replayStream) {
      throw new Error("expected replay-from-scratch stream to be opened");
    }

    act(() => {
      replayStream.onerror?.(new Event("error"));
    });

    await waitFor(() => {
      expect(useDashboardStreamStore.getState().streamState).toMatchObject({
        message:
          "The dashboard could not restore this session automatically.",
        status: "recovery_failed",
      });
    });
    expect(replayHarness.getStreams()).toHaveLength(2);
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
