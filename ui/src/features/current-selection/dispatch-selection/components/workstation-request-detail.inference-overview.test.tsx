import { fireEvent, render, screen, within } from "@testing-library/react";
import { CurrentSelectionLocaleProvider } from "../../base/components/current-selection-locale";
import {
  inferenceAttempt,
  workstationRequest,
} from "../../base/components/detail-card-test-helpers";
import { INFERENCE_ATTEMPT_DETAIL_CLASS } from "../../base/components/detail-card-shared";
import { WorkstationRequestDetailCard } from "./workstation-request-detail";

function renderReadyInferenceRequestDetailCard() {
  render(
    <WorkstationRequestDetailCard
      request={workstationRequest("dispatch-review-ready", {
        dispatched_request_count: 2,
        errored_request_count: 1,
        inference_attempts: [
          inferenceAttempt("dispatch-review-ready", {
            attempt: 1,
            inference_request_id: "dispatch-review-ready/inference-request/1",
            outcome: "FAILED",
            response_time: "2026-04-08T12:00:02Z",
          }),
          inferenceAttempt("dispatch-review-ready", {
            attempt: 2,
            duration_millis: 740,
            inference_request_id: "dispatch-review-ready/inference-request/2",
            outcome: "SUCCEEDED",
            prompt: "Retry the review with the latest context.",
            provider_session: {
              id: "sess-ready-request",
              kind: "session_id",
              provider: "codex",
            },
            response: "Ready for the next workstation.",
            response_time: "2026-04-08T12:00:04Z",
          }),
        ],
        model: "gpt-5.4",
        outcome: "ACCEPTED",
        prompt: "Review the active story and decide whether it is ready.",
        provider: "codex",
        provider_session: {
          id: "sess-ready-request",
          kind: "session_id",
          provider: "codex",
        },
        request_id: "request-ready-story",
        request_metadata: {
          prompt_source: "factory-renderer",
          source: "dispatch-history",
        },
        request_view: {
          input_work_items: [],
          runner: {
            capabilities: {
              baselineCapabilities: ["prompt_submission", "tool_execution"],
              optionalCapabilities: [
                {
                  capability: "structured_output",
                  status: "unsupported",
                },
                {
                  capability: "working_directory",
                  status: "unsupported",
                },
              ],
            },
            displayName: "Gemini",
            runnerId: "gemini",
            selectionSource: "factory",
          },
        },
        responded_request_count: 1,
        response: "Ready for the next workstation.",
        response_metadata: {
          finish_reason: "stop",
          session_source: "codex",
        },
        total_duration_millis: 63_000,
        trace_ids: ["trace-active-story"],
        working_directory: "C:\\work\\portos",
        worktree: "C:\\work\\portos\\.worktrees\\active-story",
      })}
    />,
  );
}

function getReadyInferenceRegions() {
  const currentSelection = screen.getByRole("article", {
    name: "Current selection",
  });

  return {
    currentSelection: within(currentSelection),
    inferenceAttempts: within(
      screen.getByRole("region", { name: "Inference attempts" }),
    ),
    requestDetails: within(
      screen.getByRole("region", { name: "Request details" }),
    ),
    responseDetails: within(
      screen.getByRole("region", { name: "Response details" }),
    ),
  };
}

it("keeps inference-backed request and response detail inside inference attempts without visible request counts", () => {
  renderReadyInferenceRequestDetailCard();

  const {
    currentSelection,
    inferenceAttempts,
    requestDetails,
    responseDetails,
  } = getReadyInferenceRegions();

  expect(
    currentSelection.getByRole("heading", {
      name: "Current selection",
    }),
  ).toBeTruthy();
  expect(
    currentSelection.getByText("Active Story", {
      selector: "p",
    }),
  ).toBeTruthy();
  expect(currentSelection.getAllByText("request-ready-story")).toHaveLength(1);
  expect(currentSelection.getAllByText("Dispatch ID").length).toBeGreaterThan(
    0,
  );
  expect(currentSelection.getByText("Runner")).toBeTruthy();
  expect(currentSelection.getByText("Gemini")).toBeTruthy();
  expect(currentSelection.getByText("factory")).toBeTruthy();
  expect(currentSelection.getByText("Runner capability support")).toBeTruthy();
  expect(currentSelection.getByText("Structured output")).toBeTruthy();
  expect(currentSelection.getAllByText("Unsupported").length).toBeGreaterThan(
    0,
  );
  expect(
    currentSelection.queryByRole("heading", {
      name: "Request counts",
    }),
  ).toBeNull();
  expect(currentSelection.queryByText("dispatchedCount")).toBeNull();
  expect(currentSelection.queryByText("respondedCount")).toBeNull();
  expect(currentSelection.queryByText("erroredCount")).toBeNull();
  expect(responseDetails.getByText("trace-active-story")).toBeTruthy();
  expect(
    currentSelection.getByText("Dispatch ID").closest("dl")?.className,
  ).toContain(INFERENCE_ATTEMPT_DETAIL_CLASS);
  expect(currentSelection.getByText("1m 3s")).toBeTruthy();
  expect(
    requestDetails.queryByText(/Inference attempts when available/),
  ).toBeNull();
  expect(
    responseDetails.queryByText(/Inference attempts when available/),
  ).toBeNull();
  expect(
    screen.queryByRole("region", { name: "Request metadata" }),
  ).toBeNull();
  expect(
    screen.queryByRole("region", { name: "Response metadata" }),
  ).toBeNull();
  expect(
    screen.queryByRole("heading", { name: "Workstation summary" }),
  ).toBeNull();
  expect(screen.queryByText("Runtime labels")).toBeNull();
  expect(
    inferenceAttempts.getByRole("button", { name: "Expand attempt 1" }),
  ).toBeTruthy();
  expect(
    inferenceAttempts.getByRole("button", { name: "Expand attempt 2" }),
  ).toBeTruthy();
  expect(
    inferenceAttempts.queryByText("Retry the review with the latest context."),
  ).toBeNull();
  expect(
    inferenceAttempts.queryByText("Ready for the next workstation."),
  ).toBeNull();

  fireEvent.click(
    inferenceAttempts.getByRole("button", { name: "Expand attempt 2" }),
  );
  fireEvent.click(
    inferenceAttempts.getByRole("button", { name: "Expand request body" }),
  );
  fireEvent.click(
    inferenceAttempts.getByRole("button", { name: "Expand response body" }),
  );

  expect(
    inferenceAttempts.getByText("Retry the review with the latest context."),
  ).toBeTruthy();
  expect(
    inferenceAttempts.getByText("Ready for the next workstation."),
  ).toBeTruthy();
  expect(
    inferenceAttempts.getByText("codex / Session ID / sess-ready-request"),
  ).toBeTruthy();
});

it("renders no-response workstation-request details with clear inference-attempt pending copy", () => {
  render(
    <WorkstationRequestDetailCard
      request={workstationRequest("dispatch-review-pending", {
        prompt:
          "Review the active story while the provider response is still pending.",
        request_id: "request-pending-story",
        request_metadata: {
          prompt_source: "factory-renderer",
        },
        working_directory: "C:\\work\\portos",
        worktree: "C:\\work\\portos\\.worktrees\\pending-story",
      })}
    />,
  );

  expect(screen.getByText("Active Story", { selector: "p" })).toBeTruthy();
  expect(screen.getAllByText("request-pending-story")).toHaveLength(1);
  expect(
    screen.getByText(
      "Total duration is not available for this workstation request yet.",
    ),
  ).toBeTruthy();
  expect(
    screen.getByText(
      "No inference events are available for this selected work item.",
    ),
  ).toBeTruthy();
  expect(screen.queryByRole("region", { name: "Request metadata" })).toBeNull();
  expect(
    screen.queryByRole("region", { name: "Response metadata" }),
  ).toBeNull();
  expect(screen.queryByRole("heading", { name: "Error details" })).toBeNull();
});

it("keeps explicit pending and unavailable inference response states readable", () => {
  render(
    <WorkstationRequestDetailCard
      request={workstationRequest("dispatch-review-response-states", {
        inference_attempts: [
          inferenceAttempt("dispatch-review-response-states", {
            attempt: 1,
            inference_request_id:
              "dispatch-review-response-states/inference-request/1",
            outcome: "FAILED",
            prompt: "Summarize the review findings.",
            response_time: "2026-04-08T12:00:02Z",
          }),
          inferenceAttempt("dispatch-review-response-states", {
            attempt: 2,
            inference_request_id:
              "dispatch-review-response-states/inference-request/2",
            prompt: "Retry after the failure.",
          }),
        ],
        request_id: "request-response-states",
      })}
    />,
  );

  const inferenceAttempts = within(
    screen.getByRole("region", { name: "Inference attempts" }),
  );
  fireEvent.click(
    inferenceAttempts.getByRole("button", { name: "Expand attempt 1" }),
  );
  fireEvent.click(
    inferenceAttempts.getByRole("button", { name: "Expand attempt 2" }),
  );

  expect(
    inferenceAttempts.getByText(
      "Provider response text is not available for this inference attempt.",
    ),
  ).toBeTruthy();
  expect(
    inferenceAttempts.getByText("Awaiting provider response."),
  ).toBeTruthy();
});

it("renders locale-sensitive inference detail copy while preserving runtime data values", () => {
  render(
    <CurrentSelectionLocaleProvider locale="zh-CN">
      <WorkstationRequestDetailCard
        request={workstationRequest("dispatch-review-zh", {
          inference_attempts: [
            inferenceAttempt("dispatch-review-zh", {
              attempt: 2,
              inference_request_id: "dispatch-review-zh/inference-request/2",
              outcome: "SUCCEEDED",
              prompt: "Retry with the latest runtime context.",
              provider_session: {
                id: "sess-zh",
                kind: "session_id",
                provider: "codex",
              },
              response: "Ready for handoff.",
            }),
          ],
          request_id: "request-zh-story",
          trace_ids: ["trace-zh-story"],
        })}
      />
    </CurrentSelectionLocaleProvider>,
  );

  expect(screen.getByRole("region", { name: "请求详情" })).toBeTruthy();
  expect(screen.getByRole("region", { name: "响应详情" })).toBeTruthy();
  expect(screen.getByRole("region", { name: "推理尝试" })).toBeTruthy();
  expect(screen.getByText("trace-zh-story")).toBeTruthy();

  fireEvent.click(screen.getByRole("button", { name: "展开尝试 2" }));

  expect(screen.getByText("推理请求 ID")).toBeTruthy();
  expect(screen.getByText("Provider session")).toBeTruthy();
  expect(screen.getByText("响应正文")).toBeTruthy();
  expect(
    screen.getByText("dispatch-review-zh/inference-request/2"),
  ).toBeTruthy();
  expect(screen.getByText("codex / 会话 ID / sess-zh")).toBeTruthy();
});
