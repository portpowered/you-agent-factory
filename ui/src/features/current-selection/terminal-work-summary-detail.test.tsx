import { render, screen } from "@testing-library/react";
import { CurrentSelectionLocaleProvider } from "./current-selection-locale";
import { DETAIL_CARD_NOW } from "./detail-card-test-helpers";
import type { SelectedWorkItemExecutionDetails } from "./state/executionDetails";
import { TerminalWorkSummaryCard } from "./terminal-work-summary-detail";

describe("TerminalWorkSummaryCard", () => {
  it("renders terminal work summaries through a focused card", () => {
    render(
      <TerminalWorkSummaryCard label="work-failed-story" status="failed" />,
    );

    expect(
      screen.getByRole("heading", { name: "Current selection" }),
    ).toBeTruthy();
    expect(screen.getByText("work-failed-story")).toBeTruthy();
    expect(screen.getByText("Failed")).toBeTruthy();
    expect(screen.getByText("Failure reason")).toBeTruthy();
    expect(screen.getByText("Failure reason unavailable")).toBeTruthy();
    expect(screen.getByText("Failure message unavailable")).toBeTruthy();
    expect(
      screen.getByText(
        "Failure details are unavailable for this failed work item.",
      ),
    ).toBeTruthy();
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
    expect(
      screen.getByText(
        "Provider rate limit exceeded while generating the repair.",
      ),
    ).toBeTruthy();
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
    expect(
      screen.queryByText(
        "Failure details are unavailable for this failed work item.",
      ),
    ).toBeNull();
    expect(
      screen.getByText(
        "Completed terminal work is retained in the session summary.",
      ),
    ).toBeTruthy();
  });

  it("renders terminal summary copy through the current-selection locale provider for a supported non-default locale", () => {
    render(
      <CurrentSelectionLocaleProvider locale="ja">
        <TerminalWorkSummaryCard label="失敗したストーリー" status="failed" />
      </CurrentSelectionLocaleProvider>,
    );

    expect(screen.getByRole("heading", { name: "現在の選択" })).toBeTruthy();
    expect(screen.getByText("失敗したストーリー")).toBeTruthy();
    expect(screen.getByText("ステータス")).toBeTruthy();
    expect(screen.getByText("失敗")).toBeTruthy();
    expect(screen.getByText("失敗理由")).toBeTruthy();
    expect(screen.getByText("失敗理由を利用できません")).toBeTruthy();
    expect(screen.getByText("失敗メッセージ")).toBeTruthy();
    expect(
      screen.getByText("この失敗した作業項目では失敗の詳細を利用できません。"),
    ).toBeTruthy();
  });

  it("renders execution details for selected terminal work when diagnostics are retained", () => {
    const executionDetails: SelectedWorkItemExecutionDetails = {
      dispatchID: "dispatch-done-story",
      elapsedStartTimestamp: "2026-04-08T12:00:00Z",
      inferenceAttempts: [],
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

    expect(
      screen.getByRole("heading", { name: "Execution details" }),
    ).toBeTruthy();
    expect(screen.getByText("dispatch-done-story")).toBeTruthy();
    expect(screen.getByText("trace-done-story")).toBeTruthy();
    expect(
      screen.getByRole("link", { name: "Open trace" }).getAttribute("href"),
    ).toBe("#trace");
  });
});
