import { act, render, screen } from "@testing-library/react";

import {
  resetDashboardSessionStore,
  useDashboardSessionStore,
} from "../state/dashboardSessionStore";
import {
  DashboardSessionProvider,
  useDashboardSession,
} from "./dashboard-session-provider";

function SessionScopeProbe() {
  const { eventsPath, factoryPath, isPaused, sessionID, workPath } =
    useDashboardSession();

  return (
    <div data-testid="session-scope-probe">
      {sessionID}|{factoryPath}|{workPath}|{eventsPath}|{String(isPaused)}
    </div>
  );
}

describe("DashboardSessionProvider", () => {
  beforeEach(() => {
    resetDashboardSessionStore();
  });

  it("projects the active tab session into scope for descendants", () => {
    useDashboardSessionStore.setState({
      pausedSessionIDs: [],
      selectedSessionID: "~default",
    });

    render(
      <DashboardSessionProvider>
        <SessionScopeProbe />
      </DashboardSessionProvider>,
    );

    expect(screen.getByTestId("session-scope-probe").textContent).toBe(
      "~default|/factory-sessions/~default/factory|/factory-sessions/~default/work|/factory-sessions/~default/events|false",
    );
  });

  it("updates scope when setSelectedSessionID changes", () => {
    useDashboardSessionStore.setState({
      pausedSessionIDs: [],
      selectedSessionID: "~default",
    });

    render(
      <DashboardSessionProvider>
        <SessionScopeProbe />
      </DashboardSessionProvider>,
    );

    act(() => {
      useDashboardSessionStore.getState().setSelectedSessionID("session-beta");
    });

    expect(screen.getByTestId("session-scope-probe").textContent).toBe(
      "session-beta|/factory-sessions/session-beta/factory|/factory-sessions/session-beta/work|/factory-sessions/session-beta/events|false",
    );
  });

  it("reflects pause state for the selected session", () => {
    useDashboardSessionStore.setState({
      pausedSessionIDs: ["session-beta"],
      selectedSessionID: "session-beta",
    });

    render(
      <DashboardSessionProvider>
        <SessionScopeProbe />
      </DashboardSessionProvider>,
    );

    expect(screen.getByTestId("session-scope-probe").textContent).toContain("|true");
  });

  it("throws when useDashboardSession is called outside the provider", () => {
    expect(() => render(<SessionScopeProbe />)).toThrow(
      "useDashboardSession must be used within DashboardSessionProvider.",
    );
  });
});
