import type { QueryClient } from "@tanstack/react-query";
import { QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "bun:test";
import type { PropsWithChildren } from "react";

import { DEFAULT_FACTORY_SESSION_ID } from "../../../../api/session-routing";
import { bunVi as vi } from "../../../../testing/bun/vi-compat";
import { DashboardSessionStoreTestProvider } from "../../../../testing/dashboard-session-test-provider";
import { createReplayHarness } from "../../../../testing/replay-harness";
import { useFactoryTimelineStore } from "../../../timeline/state/factoryTimelineStore";
import { useDashboardSessionStore } from "../../state/dashboardSessionStore";
import {
  createDefaultDashboardStreamState,
  useDashboardStreamStore,
} from "../../state/dashboardStreamStore";
import { useFactoryEventStream } from "./useFactoryEventStream";
import {
  CANONICAL_SELECTED_TICK_EVENTS,
  createFactoryEventStreamQueryClient,
  SEEDED_SNAPSHOT,
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

describe("useFactoryEventStream session-scoped reconnect URLs", () => {
  let queryClient = createFactoryEventStreamQueryClient();

  beforeEach(() => {
    replayHarness.install();
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(null, {
          status: 200,
        }),
      ),
    );
    queryClient = createFactoryEventStreamQueryClient();
    seedFactoryEventStreamStores();
  });

  afterEach(() => {
    resetFactoryEventStreamStores();
    vi.unstubAllGlobals();
  });

  it("reconnects on the selected session stream with after_event_id and after_sequence", async () => {
    renderHook(
      () =>
        useFactoryEventStream({
          enabled: true,
          locale: "en",
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
      stream.emit("message", CANONICAL_SELECTED_TICK_EVENTS[0]);
      await new Promise<void>((resolve) => {
        window.setTimeout(() => resolve(), 20);
      });
    });

    act(() => {
      stream.onerror?.(new Event("error"));
    });

    await waitFor(
      () => {
        expect(replayHarness.getStreams()).toHaveLength(2);
      },
      { timeout: 3000 },
    );
    expect(replayHarness.getStreams()[1]?.url).toBe(
      `/factory-sessions/${DEFAULT_FACTORY_SESSION_ID}/events?after_event_id=event-1&after_sequence=1`,
    );
    expect(replayHarness.getStreams()[1]?.url).not.toBe("/events");
  });

  it("opens reconnect cursors on the selected non-default session stream", async () => {
    renderHook(
      () =>
        useFactoryEventStream({
          enabled: true,
          initialReconnectCursor: {
            afterEventId: "event-3",
            afterSequence: 12,
          },
          onEvent: () => {},
          sessionID: "session-beta",
        }),
      { wrapper: createWrapper(queryClient) },
    );

    await waitFor(() => {
      expect(replayHarness.getStreams()).toHaveLength(1);
    });
    expect(replayHarness.getStreams()[0]?.url).toBe(
      "/factory-sessions/session-beta/events?after_event_id=event-3&after_sequence=12",
    );
    expect(replayHarness.getStreams()[0]?.url).not.toBe("/events");
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
