import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { formatLocalDateTime } from "../../components/ui/formatters";
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
    const traceIdValue = within(section).getByText("trace-alpha (selected)").closest("dd");
    if (!traceIdValue) {
      throw new Error("Expected trace ID list container.");
    }
    expect(traceIdValue.className).toContain("gap-1.5");

    const workstationRequest = within(section).getByRole("region", {
      name: "Workstation request",
    });
    const expectedStartedAt = formatLocalDateTime(
      details.workstationRequest?.request.startedAt,
      "Elapsed time is not available for this selected run.",
    );
    expect(within(workstationRequest).getByText("2")).toBeTruthy();
    expect(within(workstationRequest).getAllByText("1")).toHaveLength(2);
    expect(within(workstationRequest).getByText("FAILED")).toBeTruthy();
    expect(within(workstationRequest).getByText("640ms")).toBeTruthy();
    expect(within(workstationRequest).getByText(expectedStartedAt)).toBeTruthy();
    expect(within(workstationRequest).queryByText("2026-04-08T12:00:00Z")).toBeNull();
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
      <CurrentSelectionLocaleProvider locale="zh-CN">
        <ExecutionDetailsSection
          activeTraceID="trace-alpha"
          details={details}
          now={DETAIL_CARD_NOW}
          traceTargetId="trace"
        />
      </CurrentSelectionLocaleProvider>,
    );

    const section = screen.getByRole("region", { name: "执行详情" });
    expect(within(section).getByText("分派 ID")).toBeTruthy();
    expect(within(section).getByText("工作站")).toBeTruthy();
    expect(
      within(section).getByText(
        "当前所选运行暂时没有工作站详情。",
      ),
    ).toBeTruthy();
    expect(
      within(section).getByText(
        "打开追踪以查看该工作项的分派、重试和工作站输出。",
      ),
    ).toBeTruthy();
    expect(
      within(section).getByRole("link", { name: "trace-alpha（已选中）" }),
    ).toBeTruthy();
    expect(
      within(section).getByRole("link", { name: "打开追踪" }),
    ).toBeTruthy();
    expect(
      within(section).getByText(
        "当前所选工作项暂时没有推理事件。",
      ),
    ).toBeTruthy();
  });
});

describe("InferenceAttemptsSection", () => {
  it("renders inference attempt cards collapsed by default and expands them independently", () => {
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
          inferenceAttempt("dispatch-review", {
            attempt: 2,
            diagnostics: {
              provider: {
                model: "gpt-5.4",
                provider: "codex",
              },
            },
            duration_millis: 740,
            inference_request_id: "dispatch-review/inference-request/2",
            outcome: "FAILED",
            prompt: "Retry the story.",
            response: "Needs more evidence.",
          }),
        ]}
      />,
    );

    const section = screen.getByRole("region", { name: "Inference attempts" });
    const expandAttempt1 = within(section).getByRole("button", {
      name: "Expand attempt 1",
    });
    const expandAttempt2 = within(section).getByRole("button", {
      name: "Expand attempt 2",
    });

    expect(within(section).getByText("Attempt 1")).toBeTruthy();
    expect(within(section).getByText("Attempt 2")).toBeTruthy();
    expect(expandAttempt1.getAttribute("aria-expanded")).toBe("false");
    expect(expandAttempt2.getAttribute("aria-expanded")).toBe("false");
    expect(
      within(section).getByText(
        `Response time: ${formatLocalDateTime("2026-04-08T12:00:03Z", "Unavailable")}`,
      ),
    ).toBeTruthy();
    expect(within(section).getByText("Elapsed time: 740ms")).toBeTruthy();
    expect(
      within(section).queryByText("dispatch-review/inference-request/1"),
    ).toBeNull();
    expect(within(section).queryByText("Review the story.")).toBeNull();
    expect(within(section).queryByText("Looks good.")).toBeNull();
    expect(within(section).queryByText("Retry the story.")).toBeNull();

    fireEvent.click(expandAttempt1);

    expect(expandAttempt1.getAttribute("aria-expanded")).toBe("true");
    expect(
      within(section).getByText("dispatch-review/inference-request/1"),
    ).toBeTruthy();
    const expandRequestBody = within(section).getByRole("button", {
      name: "Expand request body",
    });
    const expandResponseBody = within(section).getByRole("button", {
      name: "Expand response body",
    });
    expect(expandRequestBody.getAttribute("aria-expanded")).toBe("false");
    expect(expandResponseBody.getAttribute("aria-expanded")).toBe("false");
    expect(within(section).queryByText("Review the story.")).toBeNull();
    expect(within(section).queryByText("Looks good.")).toBeNull();

    fireEvent.click(expandRequestBody);

    expect(expandRequestBody.getAttribute("aria-expanded")).toBe("true");
    expect(within(section).getByText("Review the story.")).toBeTruthy();
    expect(within(section).queryByText("Looks good.")).toBeNull();

    fireEvent.click(expandResponseBody);

    expect(expandResponseBody.getAttribute("aria-expanded")).toBe("true");
    expect(within(section).getByText("Looks good.")).toBeTruthy();
    expect(within(section).queryByText("Retry the story.")).toBeNull();

    fireEvent.click(expandAttempt2);

    expect(expandAttempt2.getAttribute("aria-expanded")).toBe("true");
    expect(within(section).getByText("dispatch-review/inference-request/2")).toBeTruthy();
    expect(within(section).queryByText("Retry the story.")).toBeNull();
    expect(within(section).queryByText("Needs more evidence.")).toBeNull();
    expect(
      within(section).queryByText(
        "No inference events are available for this selected work item.",
      ),
    ).toBeNull();
  });

  it("keeps unavailable and pending response states explicit instead of rendering empty disclosure controls", () => {
    render(
      <InferenceAttemptsSection
        attempts={[
          inferenceAttempt("dispatch-review", {
            attempt: 1,
            outcome: "FAILED",
            prompt: "Investigate the failure.",
            response: "   ",
          }),
          inferenceAttempt("dispatch-review", {
            attempt: 2,
            outcome: undefined,
            prompt: "Wait for completion.",
            response: undefined,
          }),
        ]}
      />,
    );

    const section = screen.getByRole("region", { name: "Inference attempts" });
    fireEvent.click(
      within(section).getByRole("button", {
        name: "Expand attempt 1",
      }),
    );
    fireEvent.click(
      within(section).getByRole("button", {
        name: "Expand attempt 2",
      }),
    );

    expect(
      within(section).getAllByRole("button", { name: "Expand request body" }),
    ).toHaveLength(2);
    expect(
      within(section).queryByRole("button", { name: "Expand response body" }),
    ).toBeNull();
    expect(
      within(section).getByText(
        "Provider response text is not available for this inference attempt.",
      ),
    ).toBeTruthy();
    expect(
      within(section).getByText("Awaiting provider response."),
    ).toBeTruthy();
  });
});
