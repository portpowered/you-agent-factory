import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { CurrentSelectionLocaleProvider } from "./current-selection-locale";
import { DETAIL_CARD_NOW, inferenceAttempt } from "./detail-card-test-helpers";
import {
  ExecutionDetailsSection,
  InferenceAttemptsSection,
} from "./execution-details";
import type { SelectedWorkItemExecutionDetails } from "./state/executionDetails";

describe("ExecutionDetailsSection", () => {
  it("renders available execution details with trace actions and workstation request projection guidance", () => {
    const onSelectTraceID = vi.fn();
    const details: SelectedWorkItemExecutionDetails = {
      dispatchID: "dispatch-review",
      elapsedStartTimestamp: "2026-04-08T12:00:00Z",
      inferenceAttempts: [],
      traceIDs: ["trace-alpha", "trace-beta"],
      workstationName: "Review",
      workstationRequest: {
        counts: {
          dispatchedCount: 2,
          errored_count: 1,
          responded_count: 1,
        },
        dispatch_id: "dispatch-review",
        request: {
          startedAt: "2026-04-08T12:00:00Z",
        },
        response: {
          duration_millis: 640,
          failure_message: "Provider timed out.",
          failure_reason: "provider_timeout",
          outcome: "FAILED",
        },
        transition_id: "review",
        workstation_name: "Review",
      },
      workID: "work-1",
    };

    render(
      <ExecutionDetailsSection
        activeTraceID="trace-alpha"
        details={details}
        now={DETAIL_CARD_NOW}
        onSelectTraceID={onSelectTraceID}
        traceTargetId="trace"
      />,
    );

    const section = screen.getByRole("region", { name: "Execution details" });
    expect(within(section).getByText("dispatch-review")).toBeTruthy();
    expect(within(section).getByText("Review")).toBeTruthy();
    expect(
      within(section).getByRole("link", { name: "trace-alpha (selected)" }),
    ).toBeTruthy();
    expect(
      within(section).getByRole("link", { name: "trace-beta" }),
    ).toBeTruthy();
    expect(
      within(section).getByRole("link", { name: "Open trace" }),
    ).toBeTruthy();

    const workstationRequest = within(section).getByRole("region", {
      name: "Workstation request",
    });
    expect(within(workstationRequest).getByText("2")).toBeTruthy();
    expect(within(workstationRequest).getAllByText("1")).toHaveLength(2);
    expect(within(workstationRequest).getByText("FAILED")).toBeTruthy();
    expect(within(workstationRequest).getByText("640ms")).toBeTruthy();
    expect(
      within(workstationRequest).getByText("provider_timeout"),
    ).toBeTruthy();
    expect(
      within(workstationRequest).getByText("Provider timed out."),
    ).toBeTruthy();
    expect(
      within(workstationRequest).getByText(
        "Prompt, provider-session, and response-body details are shown under Inference attempts.",
      ),
    ).toBeTruthy();
    expect(
      within(section).getByText(
        "No inference events are available for this selected work item.",
      ),
    ).toBeTruthy();

    fireEvent.click(within(section).getByRole("link", { name: "trace-beta" }));
    fireEvent.click(within(section).getByRole("link", { name: "Open trace" }));

    expect(onSelectTraceID).toHaveBeenNthCalledWith(1, "trace-beta");
    expect(onSelectTraceID).toHaveBeenNthCalledWith(2, "trace-alpha");
  });

  it("renders pending and unavailable execution states without trace or inference sections when omitted", () => {
    const details: SelectedWorkItemExecutionDetails = {
      dispatchID: undefined,
      elapsedStartTimestamp: undefined,
      inferenceAttempts: [],
      traceIDs: [],
      workstationName: undefined,
      workID: "work-2",
    };

    render(
      <ExecutionDetailsSection
        details={details}
        now={DETAIL_CARD_NOW}
        showInferenceAttempts={false}
        traceTargetId="trace"
      />,
    );

    const section = screen.getByRole("region", { name: "Execution details" });
    expect(
      within(section).getByText(
        "Dispatch ID is not available for this selected run.",
      ),
    ).toBeTruthy();
    expect(
      within(section).getByText(
        "Workstation details are not available for this selected run.",
      ),
    ).toBeTruthy();
    expect(
      within(section).getByText(
        "Elapsed time is not available for this selected run.",
      ),
    ).toBeTruthy();
    expect(
      within(section).getAllByText(
        "Trace details are not available for this selected run.",
      ).length,
    ).toBeGreaterThan(0);
    expect(
      within(section).queryByRole("link", { name: "Open trace" }),
    ).toBeNull();
    expect(
      within(section).queryByRole("region", { name: "Workstation request" }),
    ).toBeNull();
    expect(
      within(section).queryByRole("region", { name: "Inference attempts" }),
    ).toBeNull();
  });

  it("renders execution-details copy through the current-selection locale provider for a supported non-default locale", () => {
    const details: SelectedWorkItemExecutionDetails = {
      dispatchID: undefined,
      elapsedStartTimestamp: undefined,
      inferenceAttempts: [],
      traceIDs: ["trace-alpha"],
      workstationName: undefined,
      workID: "work-2",
    };

    render(
      <CurrentSelectionLocaleProvider locale="ja">
        <ExecutionDetailsSection
          activeTraceID="trace-alpha"
          details={details}
          now={DETAIL_CARD_NOW}
          traceTargetId="trace"
        />
      </CurrentSelectionLocaleProvider>,
    );

    const section = screen.getByRole("region", { name: "実行の詳細" });
    expect(within(section).getByText("ディスパッチ ID")).toBeTruthy();
    expect(within(section).getByText("ワークステーション")).toBeTruthy();
    expect(
      within(section).getByText(
        "この選択中の実行ではワークステーションの詳細を利用できません。",
      ),
    ).toBeTruthy();
    expect(
      within(section).getByText(
        "この作業項目のディスパッチ、再試行、ワークステーション出力を確認するにはトレースを開いてください。",
      ),
    ).toBeTruthy();
    expect(
      within(section).getByRole("link", { name: "trace-alpha（選択中）" }),
    ).toBeTruthy();
    expect(
      within(section).getByRole("link", { name: "トレースを開く" }),
    ).toBeTruthy();
    expect(
      within(section).getByText(
        "この選択中の作業項目では推論イベントを利用できません。",
      ),
    ).toBeTruthy();
  });
});

describe("InferenceAttemptsSection", () => {
  it("renders nested inference attempt cards when attempts exist", () => {
    render(
      <InferenceAttemptsSection
        attempts={[
          inferenceAttempt("dispatch-review", {
            attempt: 1,
            diagnostics: {
              provider: {
                model: "gpt-5.4-mini",
                provider: "codex",
              },
            },
            inference_request_id: "dispatch-review/inference-request/1",
            outcome: "SUCCEEDED",
            prompt: "Review the story.",
            response: "Looks good.",
            response_time: "2026-04-08T12:00:03Z",
          }),
        ]}
      />,
    );

    const section = screen.getByRole("region", { name: "Inference attempts" });
    expect(within(section).getByText("Attempt 1")).toBeTruthy();
    expect(
      within(section).getByText("dispatch-review/inference-request/1"),
    ).toBeTruthy();
    expect(within(section).getByText("Review the story.")).toBeTruthy();
    expect(within(section).getByText("Looks good.")).toBeTruthy();
    expect(
      within(section).queryByText(
        "No inference events are available for this selected work item.",
      ),
    ).toBeNull();
  });
});
