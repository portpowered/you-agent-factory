import type { QueryClient } from "@tanstack/react-query";
import { QueryClientProvider } from "@tanstack/react-query";
import type { PropsWithChildren } from "react";

import { DEFAULT_FACTORY_SESSION_ID } from "../../../../api/session-routing";
import { DashboardSessionStoreTestProvider } from "../../../../testing/dashboard-session-test-provider";
import { createReplayHarness } from "../../../../testing/replay-harness";
import { useFactoryTimelineStore } from "../../../timeline/public/store";
import { useDashboardSessionStore } from "../../state/dashboardSessionStore";
import {
  createDefaultDashboardStreamState,
  useDashboardStreamStore,
} from "../../state/dashboardStreamStore";
import {
  createFactoryEventStreamQueryClient,
  SEEDED_SNAPSHOT,
  timelineSnapshot,
} from "./useFactoryEventStream.fixtures";

export const replayHarness = createReplayHarness();

export function seedFactoryEventStreamStores(): void {
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

export function resetFactoryEventStreamStores(): void {
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

export function createFactoryEventStreamTestWrapper(queryClient: QueryClient) {
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

export { createFactoryEventStreamQueryClient };
