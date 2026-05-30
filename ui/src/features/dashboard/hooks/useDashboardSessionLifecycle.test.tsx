import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { vi } from "vitest";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import {
  CURRENT_FACTORY_DEFINITION_QUERY_KEY,
  CURRENT_FACTORY_DEFINITION_QUERY_KEY_PREFIX,
} from "../../current-factory-definition/public";
import {
  useFactoryTimelineStore,
  type WorldState,
} from "../../timeline/state/factoryTimelineStore";
import { DashboardSessionProvider } from "../session/dashboard-session-provider";
import { useDashboardSessionStore } from "../state/dashboardSessionStore";
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

function seedTimelineAtTick(tick: number): void {
  act(() => {
    useFactoryTimelineStore.setState({
      events: [{ id: "event-1" } as never],
      latestTick: tick,
      mode: "current",
      receivedEventIDs: ["event-1"],
      selectedTick: tick,
      worldViewCache: {
        [tick]: timelineSnapshot(SEEDED_SNAPSHOT),
      },
    });
  });
}

function renderLifecycleFromSessionStore(queryClient: QueryClient) {
  return renderHook(
    () => {
      const sessionID = useDashboardSessionStore(
        (state) => state.selectedSessionID,
      );
      return useDashboardSessionLifecycle({
        sessionID,
        refreshToken: 0,
      });
    },
    { wrapper: createWrapper(queryClient) },
  );
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

  describe("first mount and factory-definition cache", () => {
    it("does not reset timeline on first mount for refreshToken 0", () => {
      seedTimelineAtTick(6);

      renderHook(
        () =>
          useDashboardSessionLifecycle({
            sessionID: DEFAULT_FACTORY_SESSION_ID,
            refreshToken: 0,
          }),
        { wrapper: createWrapper(queryClient) },
      );

      expect(useFactoryTimelineStore.getState().events).toHaveLength(1);
      expect(useFactoryTimelineStore.getState().selectedTick).toBe(6);
    });

    it("removes factory-definition queries once when switching sessions", () => {
      const removeQueries = vi.spyOn(queryClient, "removeQueries");

      renderLifecycleFromSessionStore(queryClient);

      removeQueries.mockClear();

      act(() => {
        useDashboardSessionStore
          .getState()
          .setSelectedSessionID("session-beta");
      });

      expect(removeQueries).toHaveBeenCalledTimes(1);
      expect(removeQueries).toHaveBeenCalledWith({
        queryKey: [CURRENT_FACTORY_DEFINITION_QUERY_KEY_PREFIX],
        exact: false,
      });
    });

    it("does not remove factory-definition queries again when session key is unchanged", () => {
      const removeQueries = vi.spyOn(queryClient, "removeQueries");

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
        rerender({ refreshToken: 1 });
      });

      removeQueries.mockClear();

      act(() => {
        rerender({ refreshToken: 1 });
      });

      expect(removeQueries).not.toHaveBeenCalled();
    });
  });

  describe("session scoped timeline resets", () => {
    it("clears timeline state when switching to a different session", () => {
      renderLifecycleFromSessionStore(queryClient);
      seedTimelineAtTick(6);

      act(() => {
        useDashboardSessionStore
          .getState()
          .setSelectedSessionID("session-beta");
      });

      expect(useFactoryTimelineStore.getState().events).toEqual([]);
      expect(useFactoryTimelineStore.getState().selectedTick).toBe(0);
      expect(
        useFactoryTimelineStore.getState().worldViewCache[6],
      ).toBeUndefined();
    });

    it("resets timeline and factory-definition cache when the last session is deselected", () => {
      renderLifecycleFromSessionStore(queryClient);
      seedTimelineAtTick(6);

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

      seedTimelineAtTick(SEEDED_SNAPSHOT.tick_count);

      act(() => {
        rerender({ refreshToken: 1 });
      });

      expect(useFactoryTimelineStore.getState().selectedTick).toBe(0);
      expect(useFactoryTimelineStore.getState().events).toEqual([]);
    });
  });
});
