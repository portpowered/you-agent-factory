import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { useDashboardSession } from "../features/dashboard/session/dashboard-session-provider";
import {
  renderWithDashboardSessionTest,
  wrapWithDashboardSessionTest,
} from "./dashboard-session-test-utils";

function SessionScopeProbe() {
  const { sessionID } = useDashboardSession();
  return <div data-testid="session-id">{sessionID}</div>;
}

describe("dashboard-session-test-utils", () => {
  it("wrapWithDashboardSessionTest pins session scope for manual renders", () => {
    render(
      wrapWithDashboardSessionTest(<SessionScopeProbe />, {
        sessionID: "session-gamma",
      }),
    );

    expect(screen.getByTestId("session-id").textContent).toBe("session-gamma");
  });

  it("renderWithDashboardSessionTest wraps QueryClient and session provider", () => {
    renderWithDashboardSessionTest(<SessionScopeProbe />, {
      sessionID: "session-delta",
    });

    expect(screen.getByTestId("session-id").textContent).toBe("session-delta");
  });
});
