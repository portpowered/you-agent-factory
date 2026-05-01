import { render, screen } from "@testing-library/react";
import type { SelectedWorkItemExecutionDetails } from "../../state/executionDetails";
import { DETAIL_CARD_NOW } from "./detail-card-test-helpers";
import { TerminalWorkSummaryCard } from "./terminal-work-summary-detail";

describe("TerminalWorkSummaryCard", () => {
  it("renders terminal work summaries through a focused card", () => {
    render(<TerminalWorkSummaryCard label="work-failed-story" status="failed" />);

    expect(screen.getByRole("heading", { name: "Current selection" })).toBeTruthy();
    expect(screen.getByText("work-failed-story")).toBeTruthy();
    expect(screen.getByText("Failed")).toBeTruthy();
    expect(screen.getByText("Failure reason")).toBeTruthy();
    expect(screen.getByText("Failure reason unavailable")).toBeTruthy();
    expect(screen.getByText("Failure message unavailable")).toBeTruthy();
    expect(screen.getByText("Failure details are unavailable for this failed work item.")).toBeTruthy();
  });

  it("renders failure details for failed terminal work summaries", () => {
    render(
      <TerminalWorkSummaryCard
        failureMessage="Provider rate limit exceeded while generating the repair."
        failureReason="provider_rate_limit"
        label="Failed Story"
        status="failed"
      />,
    );

    expect(screen.getByText("Failure reason")).toBeTruthy();
    expect(screen.getByText("provider_rate_limit")).toBeTruthy();
    expect(screen.getByText("Failure message")).toBeTruthy();
    expect(screen.getByText("Provider rate limit exceeded while generating the repair.")).toBeTruthy();
  });

  it("does not render failure fields for completed terminal work summaries", () => {
    render(
      <TerminalWorkSummaryCard
        failureMessage="Provider rate limit exceeded."
        failureReason="provider_rate_limit"
        label="Done Story"
        status="completed"
      />,
    );

    expect(screen.getByText("Done Story")).toBeTruthy();
    expect(screen.getByText("Completed")).toBeTruthy();
    expect(screen.queryByText("Failure reason")).toBeNull();
    expect(screen.queryByText("provider_rate_limit")).toBeNull();
    expect(screen.queryByText("Failure details are unavailable for this failed work item.")).toBeNull();
    expect(screen.getByText("Completed terminal work is retained in the session summary.")).toBeTruthy();
  });

  it("renders execution details for selected terminal work when diagnostics are retained", () => {
    const executionDetails: SelectedWorkItemExecutionDetails = {
      dispatchID: "dispatch-done-story",
      elapsedStartTimestamp: "2026-04-08T12:00:00Z",
      inferenceAttempts: [],
      model: { source: "provider-diagnostics", status: "available", value: "gpt-5.4" },
      prompt: {
        promptSource: "factory-renderer",
        source: "diagnostics",
        status: "available",
        systemPromptHash: "sha256:system-runtime",
      },
      provider: { source: "provider-diagnostics", status: "available", value: "codex" },
      providerSession: {
        source: "provider-session",
        status: "available",
        value: "sess-done-story",
      },
      traceIDs: ["trace-done-story"],
      workstationName: "Complete",
      workID: "work-done-story",
    };

    render(
      <TerminalWorkSummaryCard
        executionDetails={executionDetails}
        label="Done Story"
        now={DETAIL_CARD_NOW}
        status="completed"
      />,
    );

    expect(screen.getByRole("heading", { name: "Execution details" })).toBeTruthy();
    expect(screen.getByText("sess-done-story")).toBeTruthy();
    expect(screen.getByText("dispatch-done-story")).toBeTruthy();
    expect(screen.getByText("trace-done-story")).toBeTruthy();
    expect(screen.getByText("factory-renderer")).toBeTruthy();
    expect(screen.getByText("sha256:system-runtime")).toBeTruthy();
    expect(screen.getByText("gpt-5.4")).toBeTruthy();
    expect(screen.getByText("codex")).toBeTruthy();
    expect(screen.getByRole("link", { name: "Open trace" }).getAttribute("href")).toBe(
      "#trace",
    );
  });
});
