import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { DEFAULT_FACTORY_SESSION_ID } from "../api/session-routing";
import {
  resetDashboardSessionStore,
  useDashboardSessionStore,
} from "../features/dashboard/state/dashboardSessionStore";
import {
  DashboardSessionTestProvider,
  seedDashboardSessionForTest,
} from "./dashboard-session-test-provider";

describe("dashboard-session-test-provider", () => {
  it("defaults selected session to ~default", () => {
    resetDashboardSessionStore();
    useDashboardSessionStore.setState({
      selectedSessionID: "session-beta",
    });

    render(
      <DashboardSessionTestProvider>
        <span>child</span>
      </DashboardSessionTestProvider>,
    );

    expect(useDashboardSessionStore.getState().selectedSessionID).toBe(
      DEFAULT_FACTORY_SESSION_ID,
    );
    expect(screen.getByText("child")).toBeTruthy();
  });

  it("seeds a non-default sessionID override", () => {
    resetDashboardSessionStore();

    render(
      <DashboardSessionTestProvider sessionID="session-review">
        <span>child</span>
      </DashboardSessionTestProvider>,
    );

    expect(useDashboardSessionStore.getState().selectedSessionID).toBe(
      "session-review",
    );
  });

  it("seedDashboardSessionForTest aligns with session-routing defaults", () => {
    resetDashboardSessionStore();
    seedDashboardSessionForTest();

    expect(useDashboardSessionStore.getState().selectedSessionID).toBe(
      DEFAULT_FACTORY_SESSION_ID,
    );

    seedDashboardSessionForTest("session-review");
    expect(useDashboardSessionStore.getState().selectedSessionID).toBe(
      "session-review",
    );
  });
});
