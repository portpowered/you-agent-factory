import { render, screen } from "@testing-library/react";

import { DashboardSessionLifecycleBanner } from "./dashboard-session-lifecycle-banner";

describe("DashboardSessionLifecycleBanner", () => {
  it("announces reconnecting stream state for assistive technologies", () => {
    render(
      <DashboardSessionLifecycleBanner
        streamState={{
          message: "Reconnecting to factory events...",
          status: "reconnecting",
        }}
      />,
    );

    expect(screen.getByText("Reconnecting event stream")).toBeTruthy();
    expect(
      screen.getByTestId("dashboard-session-lifecycle-banner").getAttribute(
        "aria-live",
      ),
    ).toBe("polite");
  });

  it("shows partial and terminal lifecycle facts from replayed bracket state", () => {
    render(
      <DashboardSessionLifecycleBanner
        bracket={{
          artifact_ids: ["artifact-partial"],
          result_status: "PARTIAL",
          started_at: "2026-06-09T12:00:00Z",
        }}
        streamState={{
          message: "Factory event stream connected.",
          status: "live",
        }}
      />,
    );

    expect(screen.getAllByText("Partial result available").length).toBeGreaterThan(0);
    expect(screen.getByText("PARTIAL")).toBeTruthy();
    expect(screen.getByText("artifact-partial")).toBeTruthy();
  });

  it("shows terminal failure details from replayed bracket state", () => {
    render(
      <DashboardSessionLifecycleBanner
        bracket={{
          failure_message: "workflow execution failed",
          result_status: "FAILED_WITH_PARTIAL",
          terminal: true,
        }}
        streamState={{
          message: "Factory event stream connected.",
          status: "live",
        }}
      />,
    );

    expect(screen.getByText("Session failed")).toBeTruthy();
    expect(screen.getByText("workflow execution failed")).toBeTruthy();
  });

  it("shows stale stream and terminal success lifecycle notices", () => {
    render(
      <DashboardSessionLifecycleBanner
        bracket={{
          result_status: "FINAL",
          terminal: true,
        }}
        phase="execute"
        streamState={{
          message: "",
          status: "offline",
        }}
      />,
    );

    expect(screen.getByText("Event stream stale")).toBeTruthy();
    expect(screen.getByText("Session finished")).toBeTruthy();
    expect(screen.getByText("execute")).toBeTruthy();
  });

  it("shows phase-only lifecycle notice when bracket data is absent", () => {
    render(
      <DashboardSessionLifecycleBanner
        phase="plan"
        streamState={{
          message: "Factory event stream connected.",
          status: "live",
        }}
      />,
    );

    expect(screen.getAllByText("Current phase").length).toBeGreaterThan(0);
    expect(screen.getAllByText("plan").length).toBeGreaterThan(0);
  });
});
