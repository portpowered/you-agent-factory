import { fireEvent, render, screen, within } from "@testing-library/react";
import { formatLocalDateTime } from "../../../components/ui/formatters";
import { CurrentSelectionLocaleProvider } from "./current-selection-locale";
import {
  inferenceAttempt,
  workstationRequest,
} from "./detail-card-test-helpers";
import { WorkstationRequestDetailCard } from "./workstation-request-detail";

it("rerenders request and response timestamps for the active locale", () => {
  const requestTime = "2026-04-08T12:00:01Z";
  const responseTime = "2026-04-08T12:01:02Z";

  const request = workstationRequest("dispatch-review-timestamps", {
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
  });

  const { rerender } = render(
    <CurrentSelectionLocaleProvider locale="en">
      <WorkstationRequestDetailCard request={request} />
    </CurrentSelectionLocaleProvider>,
  );

  const inferenceAttempts = within(
    screen.getByRole("region", { name: "Inference attempts" }),
  );
  expect(
    screen.getByText("Times on this card are shown in your local timezone."),
  ).toBeTruthy();
  const expectedEnglishRequestTime = formatLocalDateTime(
    requestTime,
    "Unavailable",
    "en",
  );
  const expectedEnglishResponseTime = formatLocalDateTime(
    responseTime,
    "Unavailable",
    "en",
  );

  fireEvent.click(
    inferenceAttempts.getByRole("button", { name: "Expand attempt 1" }),
  );

  expect(inferenceAttempts.getAllByText(expectedEnglishRequestTime)).toHaveLength(
    1,
  );
  expect(inferenceAttempts.getAllByText(expectedEnglishResponseTime)).toHaveLength(
    1,
  );
  expect(
    inferenceAttempts.getByText(
      `Response time: ${expectedEnglishResponseTime}`,
    ),
  ).toBeTruthy();
  expect(inferenceAttempts.queryByText(requestTime)).toBeNull();
  expect(inferenceAttempts.queryByText(responseTime)).toBeNull();

  rerender(
    <CurrentSelectionLocaleProvider locale="zh-CN">
      <WorkstationRequestDetailCard request={request} />
    </CurrentSelectionLocaleProvider>,
  );

  const localizedInferenceAttempts = within(
    screen.getByRole("region", { name: "推理尝试" }),
  );
  expect(screen.getByText("此卡片中的时间会按你的本地时区显示。")).toBeTruthy();
  const expectedChineseRequestTime = formatLocalDateTime(
    requestTime,
    "不可用",
    "zh-CN",
  );
  const expectedChineseResponseTime = formatLocalDateTime(
    responseTime,
    "不可用",
    "zh-CN",
  );

  expect(localizedInferenceAttempts.getAllByText(expectedChineseRequestTime)).toHaveLength(
    1,
  );
  expect(localizedInferenceAttempts.getAllByText(expectedChineseResponseTime)).toHaveLength(
    1,
  );
  expect(
    localizedInferenceAttempts.getByText(
      `响应时间: ${expectedChineseResponseTime}`,
    ),
  ).toBeTruthy();
  expect(
    localizedInferenceAttempts.queryByText(expectedEnglishRequestTime),
  ).toBeNull();
  expect(
    localizedInferenceAttempts.queryByText(expectedEnglishResponseTime),
  ).toBeNull();
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
