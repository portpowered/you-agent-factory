import { act, renderHook } from "@testing-library/react";
import type { PropsWithChildren } from "react";

import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import { DashboardSessionProvider } from "../session/dashboard-session-provider";
import { useDashboardSessionStore } from "../state/dashboardSessionStore";
import {
  createDefaultDashboardStreamState,
  useDashboardStreamStore,
} from "../state/dashboardStreamStore";
import { useFactoryTimelineStore } from "../../timeline/state/factoryTimelineStore";
import { useDashboardWorldView } from "./useDashboardWorldView";

function createWrapper() {
  return function Wrapper({ children }: PropsWithChildren) {
    return <DashboardSessionProvider>{children}</DashboardSessionProvider>;
  };
}

describe("useDashboardWorldView", () => {
  beforeEach(() => {
    useDashboardStreamStore.setState({
      streamState: createDefaultDashboardStreamState(),
    });
    useDashboardSessionStore.setState({
      pausedSessionIDs: [],
      selectedSessionID: DEFAULT_FACTORY_SESSION_ID,
    });
    useFactoryTimelineStore.getState().reset();
  });

  it("projects the selected tick snapshot and shell flags without opening a stream", () => {
    const { result } = renderHook(() => useDashboardWorldView(), {
      wrapper: createWrapper(),
    });

    expect(result.current.selectedTick).toBe(0);
    expect(result.current.snapshot?.tick_count).toBe(0);
    expect(result.current.hasEvents).toBe(false);
    expect(result.current.isInitialLoading).toBe(true);
    expect(result.current.error).toBeNull();
    expect(result.current.streamState.status).toBe("connecting");
  });

  it("moves from loading to error when stream state goes offline before events", () => {
    const { result } = renderHook(() => useDashboardWorldView(), {
      wrapper: createWrapper(),
    });

    expect(result.current.isInitialLoading).toBe(true);

    act(() => {
      useDashboardStreamStore.setState({
        streamState: {
          status: "offline",
          message: "Factory event stream disconnected. Showing last event state.",
        },
      });
    });

    expect(result.current.isInitialLoading).toBe(false);
    expect(result.current.error?.message).toBe(
      "Factory event stream disconnected. Showing last event state.",
    );
    expect(result.current.hasEvents).toBe(false);
  });
});
