import type { QueryClient } from "@tanstack/react-query";
import { QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "bun:test";
import type { PropsWithChildren } from "react";

import { DEFAULT_FACTORY_SESSION_ID } from "../../../../api/session-routing";
import { DashboardSessionStoreTestProvider } from "../../../../testing/dashboard-session-test-provider";
import { bunVi as vi } from "../../../../testing/bun/vi-compat";
import { createReplayHarness } from "../../../../testing/replay-harness";
import {
  currentFactoryDefinitionQueryKey,
  currentFactoryDocumentQueryKey,
} from "../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import * as timelinePublic from "../../../timeline/public/checkpoint-persistence";
import { useFactoryTimelineStore } from "../../../timeline/state/factoryTimelineStore";
import { factorySessionDetailQueryKey } from "../../lib/dashboard-session-lifecycle";
import {
  readSessionPersistenceDiagnosticRecords,
  resetSessionPersistenceDiagnosticRecords,
} from "../../lib/session-persistence/diagnostics";
import { useDashboardSessionStore } from "../../state/dashboardSessionStore";
import {
  createDefaultDashboardStreamState,
  useDashboardStreamStore,
} from "../../state/dashboardStreamStore";
import { useFactoryEventStream } from "./useFactoryEventStream";
import {
  createFactoryEventStreamQueryClient,
  SEEDED_SNAPSHOT,
  timelineSnapshot,
} from "./useFactoryEventStream.fixtures";

const replayHarness = createReplayHarness();
const RESOLVED_DEFAULT_SESSION_UUID = "a1b2c3d4-e5f6-4789-a012-3456789abcde";

function resolvedDefaultStreamIdentity() {
  return {
    backendScopeID: "backend-scope-a",
    factorySessionID: RESOLVED_DEFAULT_SESSION_UUID,
    logicalSessionKeyID: "logical-default",
    streamGenerationID: "2026-06-26T00:00:00Z",
  };
}

function seedFactoryEventStreamStores(): void {
  useDashboardStreamStore.setState({
    resolvedStreamIdentity: resolvedDefaultStreamIdentity(),
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
    resolvedStreamIdentity: null,
    streamState: createDefaultDashboardStreamState(),
  });
  useDashboardSessionStore.setState({
    pausedSessionIDs: [],
    selectedSessionID: DEFAULT_FACTORY_SESSION_ID,
  });
  useFactoryTimelineStore.getState().reset();
}

function seedStreamScopedQueryCaches(
  queryClient: QueryClient,
  streamIdentity: ReturnType<typeof resolvedDefaultStreamIdentity>,
) {
  queryClient.setQueryData(
    currentFactoryDefinitionQueryKey(
      DEFAULT_FACTORY_SESSION_ID,
      streamIdentity,
    ),
    { workers: [], workstations: [], workTypes: [] },
  );
  queryClient.setQueryData(
    currentFactoryDocumentQueryKey(DEFAULT_FACTORY_SESSION_ID, streamIdentity),
    {
      name: "default",
      version: { logical: "1", physical: "2026-06-26T00:00:00Z" },
      workers: [],
      workstations: [],
      workTypes: [],
    },
  );
  queryClient.setQueryData(
    factorySessionDetailQueryKey(
      RESOLVED_DEFAULT_SESSION_UUID,
      streamIdentity.backendScopeID,
    ),
    { status: "success" },
  );
  queryClient.setQueryData(["current-factory-definition", "session-beta"], {
    workers: [{ name: "kept" }],
  });
}

function expectClearedStreamScopedQueries(
  queryClient: QueryClient,
  streamIdentity: ReturnType<typeof resolvedDefaultStreamIdentity>,
) {
  expect(
    queryClient.getQueryData(
      currentFactoryDefinitionQueryKey(
        DEFAULT_FACTORY_SESSION_ID,
        streamIdentity,
      ),
    ),
  ).toBeUndefined();
  expect(
    queryClient.getQueryData(
      currentFactoryDocumentQueryKey(
        DEFAULT_FACTORY_SESSION_ID,
        streamIdentity,
      ),
    ),
  ).toBeUndefined();
  expect(
    queryClient.getQueryData(
      factorySessionDetailQueryKey(
        RESOLVED_DEFAULT_SESSION_UUID,
        streamIdentity.backendScopeID,
      ),
    ),
  ).toBeUndefined();
  expect(
    queryClient.getQueryData(["current-factory-definition", "session-beta"]),
  ).toEqual({ workers: [{ name: "kept" }] });
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: stale-cursor scenarios share one store harness and query client setup.
describe("useFactoryEventStream stale cursor recovery", () => {
  let queryClient = createFactoryEventStreamQueryClient();

  beforeEach(() => {
    resetSessionPersistenceDiagnosticRecords();
    replayHarness.install();
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
    resetSessionPersistenceDiagnosticRecords();
    resetFactoryEventStreamStores();
    vi.unstubAllGlobals();
  });

  it("clears only the affected session checkpoint and runtime queries before replaying from scratch", async () => {
    const streamIdentity = resolvedDefaultStreamIdentity();
    const probeRecovery = vi.fn().mockResolvedValue({
      factorySessionId: RESOLVED_DEFAULT_SESSION_UUID,
      outcome: "CURSOR_STALE",
      retry: {
        omitAfterEventId: true,
        omitAfterSequence: true,
      },
    });
    const deleteCheckpoint = vi.spyOn(
      timelinePublic,
      "deleteTimelineCheckpoint",
    );

    seedStreamScopedQueryCaches(queryClient, streamIdentity);

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
          streamIdentity,
          validateReconnectCursor: vi.fn().mockResolvedValue({ ok: true }),
        }),
      { wrapper: createWrapper(queryClient) },
    );

    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(1);
    });
    expect(replayHarness.getStreams()[0]?.url).toBe(
      `/factory-sessions/${RESOLVED_DEFAULT_SESSION_UUID}/events?after_event_id=checkpoint-event-7&after_sequence=7`,
    );
    const initialStream = replayHarness.getStreams()[0];
    if (!initialStream) {
      throw new Error("expected initial reconnect stream to be opened");
    }

    act(() => {
      initialStream.onerror?.(new Event("error"));
    });

    await waitFor(() => {
      expect(probeRecovery).toHaveBeenCalledWith(
        RESOLVED_DEFAULT_SESSION_UUID,
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
      `/factory-sessions/${RESOLVED_DEFAULT_SESSION_UUID}/events`,
    );
    expect(deleteCheckpoint).toHaveBeenCalledWith(
      window.indexedDB,
      streamIdentity,
    );
    expectClearedStreamScopedQueries(queryClient, streamIdentity);
    expect(useDashboardStreamStore.getState().streamState.status).not.toBe(
      "recovery_failed",
    );
    const diagnostics = readSessionPersistenceDiagnosticRecords();
    expect(
      diagnostics.map(({ outcome, recoveryAction }) => ({
        outcome,
        recoveryAction,
      })),
    ).toEqual([
      {
        outcome: "stale_cursor",
        recoveryAction: "invalidate_reconnect_cursor",
      },
      {
        outcome: "cursor_free_replay_fallback",
        recoveryAction: "replay_without_cursor",
      },
    ]);
    expect(diagnostics[0]?.correlationToken).toBe(
      diagnostics[1]?.correlationToken,
    );
  });

  it("shows a recoverable stream state when replay from scratch cannot reopen the session", async () => {
    const streamIdentity = resolvedDefaultStreamIdentity();
    const probeRecovery = vi.fn().mockResolvedValue({
      factorySessionId: RESOLVED_DEFAULT_SESSION_UUID,
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
          streamIdentity,
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
        message: "The dashboard could not restore this session automatically.",
        status: "recovery_failed",
      });
    });
    expect(replayHarness.getStreams()).toHaveLength(2);
    expect(
      readSessionPersistenceDiagnosticRecords().map((record) => record.outcome),
    ).toEqual(["stale_cursor", "cursor_free_replay_fallback"]);
  });

  it("keeps a cancelled recovery probe silent", async () => {
    const deferredProbe = Promise.withResolvers<{
      factorySessionId: string;
      outcome: string;
      retry: { omitAfterEventId: boolean; omitAfterSequence: boolean };
    }>();
    const probeRecovery = vi.fn(() => deferredProbe.promise);
    const hook = renderHook(
      () =>
        useFactoryEventStream({
          enabled: true,
          initialReconnectCursor: { afterEventId: "event-7", afterSequence: 7 },
          onEvent: () => {},
          probeRecovery,
          sessionID: DEFAULT_FACTORY_SESSION_ID,
          streamIdentity: resolvedDefaultStreamIdentity(),
          validateReconnectCursor: vi.fn().mockResolvedValue({ ok: true }),
        }),
      { wrapper: createWrapper(queryClient) },
    );
    await waitFor(() => expect(replayHarness.getStreams()).toHaveLength(1));
    act(() => replayHarness.getStreams()[0]?.onerror?.(new Event("error")));
    await waitFor(() => expect(probeRecovery).toHaveBeenCalledTimes(1));
    hook.unmount();
    deferredProbe.resolve({
      factorySessionId: RESOLVED_DEFAULT_SESSION_UUID,
      outcome: "CURSOR_STALE",
      retry: { omitAfterEventId: true, omitAfterSequence: true },
    });
    await act(async () => Promise.resolve());
    expect(readSessionPersistenceDiagnosticRecords()).toEqual([]);
    expect(replayHarness.getStreams()).toHaveLength(1);
  });

  it.each([
    {
      label: "failed",
      probe: () => Promise.reject(new Error("probe unavailable")),
    },
    {
      label: "mismatched-session",
      probe: () =>
        Promise.resolve({
          factorySessionId: "different-session",
          outcome: "CURSOR_STALE",
          retry: { omitAfterEventId: true, omitAfterSequence: true },
        }),
    },
    {
      label: "generic-reconnect",
      probe: () =>
        Promise.resolve({
          factorySessionId: RESOLVED_DEFAULT_SESSION_UUID,
          outcome: "RECONNECT",
          retry: { omitAfterEventId: false, omitAfterSequence: false },
        }),
    },
  ])("keeps a $label recovery probe silent", async ({ probe }) => {
    const probeRecovery = vi.fn(probe);
    const hook = renderHook(
      () =>
        useFactoryEventStream({
          enabled: true,
          initialReconnectCursor: { afterEventId: "event-7", afterSequence: 7 },
          onEvent: () => {},
          probeRecovery,
          sessionID: DEFAULT_FACTORY_SESSION_ID,
          streamIdentity: resolvedDefaultStreamIdentity(),
          validateReconnectCursor: vi.fn().mockResolvedValue({ ok: true }),
        }),
      { wrapper: createWrapper(queryClient) },
    );
    await waitFor(() => expect(replayHarness.getStreams()).toHaveLength(1));
    act(() => replayHarness.getStreams()[0]?.onerror?.(new Event("error")));
    await waitFor(() => expect(probeRecovery).toHaveBeenCalledTimes(1));
    await act(async () => Promise.resolve());
    expect(readSessionPersistenceDiagnosticRecords()).toEqual([]);
    hook.unmount();
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
