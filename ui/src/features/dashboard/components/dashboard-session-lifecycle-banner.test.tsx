// biome-ignore-all lint/complexity/noExcessiveLinesPerFunction: lifecycle banner stream, bracket, and pause/resume cases share one render harness.
import { render, screen } from "@testing-library/react";

import { pausedDashboardStreamState } from "../lib/dashboard-event-stream";
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

  it("shows replay recovery failure as a distinct stream notice", () => {
    render(
      <DashboardSessionLifecycleBanner
        streamState={{
          message: "The dashboard could not restore this session automatically.",
          status: "recovery_failed",
        }}
      />,
    );

    expect(
      screen.getByText(
        "The dashboard could not restore this session automatically.",
      ),
    ).toBeTruthy();
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

  it("shows paused lifecycle state when bracket also has a partial result", () => {
    render(
      <DashboardSessionLifecycleBanner
        bracket={{
          lifecycle_control_status: "PAUSED",
          paused_at: "2026-06-09T12:00:02Z",
          result_status: "PARTIAL",
          result_summary: [{ text: "checkpoint saved before pause" }],
          started_at: "2026-06-09T12:00:00Z",
        }}
        streamState={{
          message: "Factory event stream connected.",
          status: "live",
        }}
      />,
    );

    expect(screen.getAllByText("Factory Session paused").length).toBeGreaterThan(0);
    expect(screen.getByText("2026-06-09T12:00:02Z")).toBeTruthy();
    expect(screen.getByText("PARTIAL")).toBeTruthy();
    expect(screen.queryByText("Partial result available")).toBeNull();
  });

  it("shows running lifecycle state when bracket also has a partial result", () => {
    render(
      <DashboardSessionLifecycleBanner
        bracket={{
          lifecycle_control_status: "RUNNING",
          resumed_at: "2026-06-09T12:00:04Z",
          result_status: "PARTIAL",
          result_summary: [{ text: "checkpoint still available after resume" }],
          started_at: "2026-06-09T12:00:00Z",
        }}
        streamState={{
          message: "Factory event stream connected.",
          status: "live",
        }}
      />,
    );

    expect(screen.getAllByText("Factory Session running").length).toBeGreaterThan(0);
    expect(screen.getByText("2026-06-09T12:00:04Z")).toBeTruthy();
    expect(screen.getByText("PARTIAL")).toBeTruthy();
    expect(screen.queryByText("Partial result available")).toBeNull();
  });

  it("shows paused Factory Session lifecycle state from replayed bracket data", () => {
    render(
      <DashboardSessionLifecycleBanner
        bracket={{
          lifecycle_control_status: "PAUSED",
          paused_at: "2026-06-09T12:00:02Z",
          started_at: "2026-06-09T12:00:00Z",
        }}
        streamState={{
          message: "Factory event stream connected.",
          status: "live",
        }}
      />,
    );

    expect(screen.getAllByText("Factory Session paused").length).toBeGreaterThan(0);
    expect(screen.getByText("2026-06-09T12:00:02Z")).toBeTruthy();
    expect(screen.getByText("PAUSED")).toBeTruthy();
  });

  it("shows running Factory Session lifecycle state after a canonical resume event", () => {
    render(
      <DashboardSessionLifecycleBanner
        bracket={{
          lifecycle_control_status: "RUNNING",
          resumed_at: "2026-06-09T12:00:04Z",
          started_at: "2026-06-09T12:00:00Z",
        }}
        streamState={{
          message: "Factory event stream connected.",
          status: "live",
        }}
      />,
    );

    expect(screen.getAllByText("Factory Session running").length).toBeGreaterThan(0);
    expect(screen.getByText("2026-06-09T12:00:04Z")).toBeTruthy();
    expect(screen.getByText("RUNNING")).toBeTruthy();
  });

  it("shows paused lifecycle state from canonical factory state when bracket status is absent", () => {
    render(
      <DashboardSessionLifecycleBanner
        factoryState="PAUSED"
        streamState={{
          message: "Factory event stream connected.",
          status: "live",
        }}
      />,
    );

    expect(screen.getAllByText("Factory Session paused").length).toBeGreaterThan(0);
    expect(screen.getByText("PAUSED")).toBeTruthy();
  });

  it("keeps paused Factory Session lifecycle visible alongside stale offline stream notice", () => {
    render(
      <DashboardSessionLifecycleBanner
        bracket={{
          lifecycle_control_status: "PAUSED",
          paused_at: "2026-06-09T12:00:02Z",
          started_at: "2026-06-09T12:00:00Z",
        }}
        streamState={{
          message: "",
          status: "offline",
        }}
      />,
    );

    expect(screen.getByText("Event stream stale")).toBeTruthy();
    expect(screen.getAllByText("Factory Session paused").length).toBeGreaterThan(0);
    expect(screen.getByText("PAUSED")).toBeTruthy();
    expect(
      screen.getByTestId("dashboard-session-lifecycle-banner").getAttribute(
        "aria-live",
      ),
    ).toBe("polite");
  });

  it("keeps paused Factory Session lifecycle visible while reconnecting to the event stream", () => {
    render(
      <DashboardSessionLifecycleBanner
        bracket={{
          lifecycle_control_status: "PAUSED",
          paused_at: "2026-06-09T12:00:02Z",
          started_at: "2026-06-09T12:00:00Z",
        }}
        streamState={{
          message: "Reconnecting to factory events...",
          status: "reconnecting",
        }}
      />,
    );

    expect(screen.getByText("Reconnecting event stream")).toBeTruthy();
    expect(screen.getAllByText("Factory Session paused").length).toBeGreaterThan(0);
    expect(screen.getByText("PAUSED")).toBeTruthy();
  });

  it("shows paused-dashboard offline stream copy when live updates are paused", () => {
    render(
      <DashboardSessionLifecycleBanner
        bracket={{
          lifecycle_control_status: "RUNNING",
          resumed_at: "2026-06-09T12:00:04Z",
          started_at: "2026-06-09T12:00:00Z",
        }}
        streamState={pausedDashboardStreamState()}
      />,
    );

    expect(
      screen.getByText("Live session updates paused. Showing last event state."),
    ).toBeTruthy();
    expect(screen.getAllByText("Factory Session running").length).toBeGreaterThan(0);
    expect(screen.getByText("RUNNING")).toBeTruthy();
  });
});
