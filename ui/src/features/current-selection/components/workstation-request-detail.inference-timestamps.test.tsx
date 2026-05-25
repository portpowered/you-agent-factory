import { fireEvent, render, screen, within } from "@testing-library/react";
import { formatLocalDateTime } from "../../../components/ui/formatters";
import { CurrentSelectionLocaleProvider } from "./current-selection-locale";
import {
  inferenceAttempt,
  workstationRequest,
} from "./detail-card-test-helpers";
import { WorkstationRequestDetailCard } from "./workstation-request-detail";

it("renders request and response timestamps through the shared local-time formatter", () => {
  const requestTime = "2026-04-08T12:00:01Z";
  const responseTime = "2026-04-08T12:01:02Z";

  render(
    <WorkstationRequestDetailCard
      request={workstationRequest("dispatch-review-timestamps", {
        inference_attempts: [
          inferenceAttempt("dispatch-review-timestamps", {
            attempt: 1,
            inference_request_id:
              "dispatch-review-timestamps/inference-request/1",
            request_time: requestTime,
            response_time: responseTime,
          }),
        ],
        request_id: "request-timestamp-story",
      })}
    />,
  );

  const inferenceAttempts = within(
    screen.getByRole("region", { name: "Inference attempts" }),
  );
  const expectedRequestTime = formatLocalDateTime(requestTime, "Unavailable");
  const expectedResponseTime = formatLocalDateTime(responseTime, "Unavailable");

  fireEvent.click(
    inferenceAttempts.getByRole("button", { name: "Expand attempt 1" }),
  );

  expect(inferenceAttempts.getAllByText(expectedRequestTime)).toHaveLength(1);
  expect(inferenceAttempts.getAllByText(expectedResponseTime)).toHaveLength(1);
  expect(
    inferenceAttempts.getByText(`Response time: ${expectedResponseTime}`),
  ).toBeTruthy();
  expect(inferenceAttempts.queryByText(requestTime)).toBeNull();
  expect(inferenceAttempts.queryByText(responseTime)).toBeNull();
});

it("renders an explicit fallback for missing or invalid inference timestamps", () => {
  render(
    <CurrentSelectionLocaleProvider locale="zh-CN">
      <WorkstationRequestDetailCard
        request={workstationRequest("dispatch-review-missing-timestamps", {
          inference_attempts: [
            inferenceAttempt("dispatch-review-missing-timestamps", {
              attempt: 1,
              inference_request_id:
                "dispatch-review-missing-timestamps/inference-request/1",
              request_time: "not-a-date",
              response_time: undefined,
            }),
          ],
          request_id: "request-missing-timestamp-story",
        })}
      />
    </CurrentSelectionLocaleProvider>,
  );

  const inferenceAttempts = within(
    screen.getByRole("region", { name: "推理尝试" }),
  );
  fireEvent.click(
    inferenceAttempts.getByRole("button", { name: "展开尝试 1" }),
  );

  expect(inferenceAttempts.getAllByText("不可用")).toHaveLength(2);
  expect(inferenceAttempts.queryByText("not-a-date")).toBeNull();
});
