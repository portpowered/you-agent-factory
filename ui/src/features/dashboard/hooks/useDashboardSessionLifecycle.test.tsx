import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook } from "@testing-library/react";
import type { PropsWithChildren } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import {
  CURRENT_FACTORY_DEFINITION_QUERY_KEY,
} from "../../current-factory-definition/public";
import { useDashboardSessionStore } from "../state/dashboardSessionStore";
import { DashboardSessionProvider } from "../session/dashboard-session-provider";
import {
  type WorldState,
  useFactoryTimelineStore,
} from "../../timeline/state/factoryTimelineStore";
import { useDashboardSessionLifecycle } from "./useDashboardSessionLifecycle";

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

function timelineSnapshot(snapshot: DashboardSnapshot): WorldState {
  return {
    ...snapshot,
    relationsByWorkID: {},
    tracesByWorkID: {},
    workstationRequestsByDispatchID: {},
    workRequestsByID: {},
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

describe("useDashboardSessionLifecycle", () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    queryClient = new QueryClient({
      defaultOptions: {
        mutations: { retry: false },
        queries: { retry: false },
      },
    });
    queryClient.setQueryData(CURRENT_FACTORY_DEFINITION_QUERY_KEY, {
      workers: [],
      workstations: [],
      workTypes: [],
    });
    useDashboardSessionStore.setState({
      pausedSessionIDs: [],
      selectedSessionID: DEFAULT_FACTORY_SESSION_ID,
    });
    useFactoryTimelineStore.getState().reset();
  });

  afterEach(() => {
    useDashboardSessionStore.setState({
      pausedSessionIDs: [],
      selectedSessionID: DEFAULT_FACTORY_SESSION_ID,
    });
    useFactoryTimelineStore.getState().reset();
  });

  it("clears timeline state when switching to a different session", () => {
    renderHook(
      () => {
        const sessionID = useDashboardSessionStore((state) => state.selectedSessionID);
        return useDashboardSessionLifecycle({
          sessionID,
          refreshToken: 0,
        });
      },
      { wrapper: createWrapper(queryClient) },
    );

    act(() => {
      useFactoryTimelineStore.setState({
        events: [{ id: "event-1" } as never],
        latestTick: 6,
        mode: "current",
        receivedEventIDs: ["event-1"],
        selectedTick: 6,
        worldViewCache: {
          6: timelineSnapshot(SEEDED_SNAPSHOT),
        },
      });
    });

    act(() => {
      useDashboardSessionStore.getState().setSelectedSessionID("session-beta");
    });

    expect(useFactoryTimelineStore.getState().events).toEqual([]);
    expect(useFactoryTimelineStore.getState().selectedTick).toBe(0);
    expect(useFactoryTimelineStore.getState().worldViewCache[6]).toBeUndefined();
  });

  it("resets timeline and factory-definition cache when the last session is deselected", () => {
    renderHook(
      () => {
        const sessionID = useDashboardSessionStore((state) => state.selectedSessionID);
        return useDashboardSessionLifecycle({
          sessionID,
          refreshToken: 0,
        });
      },
      { wrapper: createWrapper(queryClient) },
    );

    act(() => {
      useFactoryTimelineStore.setState({
        events: [{ id: "event-1" } as never],
        latestTick: 6,
        mode: "current",
        receivedEventIDs: ["event-1"],
        selectedTick: 6,
        worldViewCache: {
          6: timelineSnapshot(SEEDED_SNAPSHOT),
        },
      });
    });

    act(() => {
      useDashboardSessionStore.getState().setSelectedSessionID(null);
    });

    expect(useFactoryTimelineStore.getState().events).toEqual([]);
    expect(useFactoryTimelineStore.getState().selectedTick).toBe(0);
    expect(
      queryClient.getQueryData(CURRENT_FACTORY_DEFINITION_QUERY_KEY),
    ).toBeUndefined();
  });

  it("resets timeline when refresh token changes", () => {
    const { rerender } = renderHook(
      ({ refreshToken }: { refreshToken: number }) =>
        useDashboardSessionLifecycle({
          sessionID: DEFAULT_FACTORY_SESSION_ID,
          refreshToken,
        }),
      {
        initialProps: { refreshToken: 0 },
        wrapper: createWrapper(queryClient),
      },
    );

    act(() => {
      useFactoryTimelineStore.setState({
        events: [{ id: "event-1" } as never],
        latestTick: SEEDED_SNAPSHOT.tick_count,
        mode: "current",
        receivedEventIDs: ["event-1"],
        selectedTick: SEEDED_SNAPSHOT.tick_count,
        worldViewCache: {
          [SEEDED_SNAPSHOT.tick_count]: timelineSnapshot(SEEDED_SNAPSHOT),
        },
      });
    });

    act(() => {
      rerender({ refreshToken: 1 });
    });

    expect(useFactoryTimelineStore.getState().selectedTick).toBe(0);
    expect(useFactoryTimelineStore.getState().events).toEqual([]);
  });
});
