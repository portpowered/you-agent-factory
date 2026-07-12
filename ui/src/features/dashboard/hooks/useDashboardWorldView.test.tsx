import { act, renderHook } from "@testing-library/react";
import type { PropsWithChildren } from "react";

import { DEFAULT_FACTORY_SESSION_ID } from "../../../api/session-routing";
import { DashboardSessionStoreTestProvider } from "../../../testing/dashboard-session-test-provider";
import { useFactoryTimelineStore } from "../../timeline/state/factoryTimelineStore";
import { useDashboardSessionStore } from "../state/dashboardSessionStore";
import {
  createDefaultDashboardStreamState,
  useDashboardStreamStore,
} from "../state/dashboardStreamStore";
import { useDashboardWorldView } from "./useDashboardWorldView";

function createWrapper() {
  return function Wrapper({ children }: PropsWithChildren) {
    return (
      <DashboardSessionStoreTestProvider>
        {children}
      </DashboardSessionStoreTestProvider>
    );
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

  it("projects the selected tick snapshot and stream state without inferring shell readiness", () => {
    const { result } = renderHook(() => useDashboardWorldView(), {
      wrapper: createWrapper(),
    });

    expect(result.current.selectedTick).toBe(0);
    expect(result.current.snapshot?.tick_count).toBe(0);
    expect(result.current.streamState.status).toBe("connecting");
  });

  it("projects offline stream state without consulting timeline ticks or events", () => {
    const { result } = renderHook(() => useDashboardWorldView(), {
      wrapper: createWrapper(),
    });

    act(() => {
      useDashboardStreamStore.setState({
        streamState: {
          status: "offline",
          message:
            "Factory event stream disconnected. Showing last event state.",
        },
      });
    });

    expect(result.current.streamState.message).toBe(
      "Factory event stream disconnected. Showing last event state.",
    );
    expect(result.current.streamState.status).toBe("offline");
  });
});
