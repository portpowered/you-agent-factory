import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { useDashboardSession } from "../features/dashboard/session/dashboard-session-provider";
import { DashboardSessionTestProvider } from "./dashboard-session-test-provider";

function SessionScopeProbe() {
  const { eventsPath, factoryPath, isPaused, sessionID, workPath } =
    useDashboardSession();

  return (
    <div data-testid="session-scope-probe">
      {sessionID}|{factoryPath}|{workPath}|{eventsPath}|{String(isPaused)}
    </div>
  );
}

describe("DashboardSessionTestProvider", () => {
  it("defaults to the ~default session scope", () => {
    render(
      <DashboardSessionTestProvider>
        <SessionScopeProbe />
      </DashboardSessionTestProvider>,
    );

    expect(screen.getByTestId("session-scope-probe").textContent).toBe(
      "~default|/factory-sessions/~default/factory|/factory-sessions/~default/work|/factory-sessions/~default/events|false",
    );
  });

  it("pins non-default session paths without dashboard session store selection", () => {
    render(
      <DashboardSessionTestProvider sessionID="session-beta">
        <SessionScopeProbe />
      </DashboardSessionTestProvider>,
    );

    expect(screen.getByTestId("session-scope-probe").textContent).toBe(
      "session-beta|/factory-sessions/session-beta/factory|/factory-sessions/session-beta/work|/factory-sessions/session-beta/events|false",
    );
  });

  it("reflects paused overrides for the pinned session", () => {
    render(
      <DashboardSessionTestProvider paused sessionID="session-beta">
        <SessionScopeProbe />
      </DashboardSessionTestProvider>,
    );

    expect(screen.getByTestId("session-scope-probe").textContent).toContain(
      "|true",
    );
  });
});
