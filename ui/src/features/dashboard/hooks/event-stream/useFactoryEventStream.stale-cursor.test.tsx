import type { QueryClient } from "@tanstack/react-query";
import { QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { PropsWithChildren } from "react";

import { DEFAULT_FACTORY_SESSION_ID } from "../../../../api/session-routing";
import { createReplayHarness } from "../../../../testing/replay-harness";
import {
  CURRENT_FACTORY_DEFINITION_QUERY_KEY,
  CURRENT_FACTORY_DOCUMENT_QUERY_KEY,
} from "../../../current-factory-definition/hooks/useCurrentFactoryDefinition";
import * as timelinePublic from "../../../timeline/public";
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
    streamState: createDefaultDashboardStreamState(),
  });
  useDashboardSessionStore.setState({
    pausedSessionIDs: [],
    selectedSessionID: DEFAULT_FACTORY_SESSION_ID,
  });
  useFactoryTimelineStore.getState().reset();
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: stale-cursor scenarios share one store harness and query client setup.
describe("useFactoryEventStream stale cursor recovery", () => {
  let queryClient = createFactoryEventStreamQueryClient();

  beforeEach(() => {
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
      timelinePublic,
      "deleteTimelineCheckpoint",
    );

    queryClient.setQueryData(CURRENT_FACTORY_DEFINITION_QUERY_KEY, {
      workers: [],
      workstations: [],
      workTypes: [],
    });
    queryClient.setQueryData(CURRENT_FACTORY_DOCUMENT_QUERY_KEY, {
      name: "default",
      version: { logical: "1", physical: "2026-06-26T00:00:00Z" },
      workers: [],
      workstations: [],
      workTypes: [],
    });
    queryClient.setQueryData(
      ["factory-session-detail", DEFAULT_FACTORY_SESSION_ID],
      { status: "success" },
    );
    queryClient.setQueryData(["current-factory-definition", "session-beta"], {
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
    expect(deleteCheckpoint).toHaveBeenCalledWith(
      window.indexedDB,
      DEFAULT_FACTORY_SESSION_ID,
    );
    expect(
      queryClient.getQueryData(CURRENT_FACTORY_DEFINITION_QUERY_KEY),
    ).toBeUndefined();
    expect(
      queryClient.getQueryData(CURRENT_FACTORY_DOCUMENT_QUERY_KEY),
    ).toBeUndefined();
    expect(
      queryClient.getQueryData([
        "factory-session-detail",
        DEFAULT_FACTORY_SESSION_ID,
      ]),
    ).toBeUndefined();
    expect(
      queryClient.getQueryData(["current-factory-definition", "session-beta"]),
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
