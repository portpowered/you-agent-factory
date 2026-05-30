import { render, screen, within } from "@testing-library/react";
import { workstationRequest } from "../../base/components/detail-card-test-helpers";
import { WorkstationRequestDetailCard } from "./workstation-request-detail";

it("renders pending script-backed workstation-request details without inference placeholders", () => {
  render(
    <WorkstationRequestDetailCard
      request={workstationRequest("dispatch-review-script-pending", {
        prompt: undefined,
        request_id: "request-script-pending-story",
        responded_request_count: 0,
        script_request: {
          args: ["--work", "work-active-story"],
          attempt: 1,
          command: "script-tool",
          script_request_id:
            "dispatch-review-script-pending/script-request/1",
        },
      })}
    />,
  );

  const requestDetails = within(
    screen.getByRole("region", { name: "Request details" }),
  );
  const responseDetails = within(
    screen.getByRole("region", { name: "Response details" }),
  );

  expect(
    screen.getAllByText("request-script-pending-story").length,
  ).toBeGreaterThan(0);
  expect(requestDetails.getByText("script-tool")).toBeTruthy();
  expect(requestDetails.getByText("--work")).toBeTruthy();
  expect(requestDetails.getByText("work-active-story")).toBeTruthy();
  expect(
    requestDetails.getByText("dispatch-review-script-pending/script-request/1"),
  ).toBeTruthy();
  expect(
    responseDetails.getByText(
      "Script response details are not available for this workstation request yet.",
    ),
  ).toBeTruthy();
  expect(
    responseDetails.queryByText(
      "Provider session details are not available for this workstation request.",
    ),
  ).toBeNull();
  expect(
    screen.queryByRole("heading", { name: "Inference attempts" }),
  ).toBeNull();
});

it("renders script-backed request fallbacks when projected script metadata is incomplete", () => {
  render(
    <WorkstationRequestDetailCard
      request={workstationRequest("dispatch-review-script-sparse", {
        request_id: "request-script-sparse-story",
        responded_request_count: 0,
        script_request: {
          args: [],
          attempt: undefined,
          command: "",
          script_request_id: "",
        },
        trace_ids: [],
      })}
    />,
  );

  const requestDetails = within(
    screen.getByRole("region", { name: "Request details" }),
  );
  const responseDetails = within(
    screen.getByRole("region", { name: "Response details" }),
  );

  expect(
    requestDetails.getByText(
      "Script request details are not available for this workstation request.",
    ),
  ).toBeTruthy();
  expect(requestDetails.getByText("Script attempt is not available yet.")).toBeTruthy();
  expect(
    requestDetails.getByText(
      "Script command details are not available for this workstation request.",
    ),
  ).toBeTruthy();
  expect(
    requestDetails.getByText(
      "Script arguments are not available for this workstation request.",
    ),
  ).toBeTruthy();
  expect(
    responseDetails.getByText(
      "Trace details are not available for this workstation request yet.",
    ),
  ).toBeTruthy();
});

it("renders successful script-backed workstation-request response details", () => {
  render(
    <WorkstationRequestDetailCard
      request={workstationRequest("dispatch-review-script-success", {
        request_id: "request-script-success-story",
        responded_request_count: 1,
        script_request: {
          args: ["--work", "work-active-story"],
          attempt: 1,
          command: "script-tool",
          script_request_id:
            "dispatch-review-script-success/script-request/1",
        },
        script_response: {
          duration_millis: 222,
          outcome: "SUCCEEDED",
          script_request_id:
            "dispatch-review-script-success/script-request/1",
          stderr: "",
          stdout: "script success stdout\n",
        },
      })}
    />,
  );

  const responseDetails = within(
    screen.getByRole("region", { name: "Response details" }),
  );

  expect(
    screen.getAllByText("request-script-success-story").length,
  ).toBeGreaterThan(0);
  expect(screen.getAllByText("SUCCEEDED").length).toBeGreaterThan(0);
  expect(screen.getAllByText("222ms").length).toBeGreaterThan(0);
  expect(
    responseDetails.getByText(
      "dispatch-review-script-success/script-request/1",
    ),
  ).toBeTruthy();
  expect(responseDetails.getByText("script success stdout")).toBeTruthy();
  expect(
    responseDetails.getByText(
      "No stderr was recorded for this script response.",
    ),
  ).toBeTruthy();
  expect(
    screen.getByText(
      "Response metadata is not available for this script-backed workstation request.",
    ),
  ).toBeTruthy();
});

it("renders failed script-backed workstation-request response details", () => {
  render(
    <WorkstationRequestDetailCard
      request={workstationRequest("dispatch-review-script-failed", {
        errored_request_count: 1,
        failure_message: "Script timed out.",
        failure_reason: "script_timeout",
        request_id: "request-script-failed-story",
        responded_request_count: 0,
        script_request: {
          args: ["--work", "work-active-story"],
          attempt: 1,
          command: "script-tool",
          script_request_id: "dispatch-review-script-failed/script-request/1",
        },
        script_response: {
          duration_millis: 500,
          failure_type: "TIMEOUT",
          outcome: "TIMED_OUT",
          script_request_id: "dispatch-review-script-failed/script-request/1",
          stderr: "script timed out\n",
          stdout: "",
        },
      })}
    />,
  );

  const responseDetails = within(
    screen.getByRole("region", { name: "Response details" }),
  );
  const errorDetails = within(
    screen.getByRole("region", { name: "Error details" }),
  );

  expect(
    screen.getAllByText("request-script-failed-story").length,
  ).toBeGreaterThan(0);
  expect(screen.getAllByText("TIMED_OUT").length).toBeGreaterThan(0);
  expect(screen.getAllByText("500ms").length).toBeGreaterThan(0);
  expect(responseDetails.getByText("TIMEOUT")).toBeTruthy();
  expect(responseDetails.getByText("script timed out")).toBeTruthy();
  expect(
    responseDetails.getByText(
      "No stdout was recorded for this script response.",
    ),
  ).toBeTruthy();
  expect(errorDetails.getByText("script_timeout")).toBeTruthy();
  expect(errorDetails.getByText("Script timed out.")).toBeTruthy();
});

it("renders script response field fallbacks when a response is present but sparse", () => {
  render(
    <WorkstationRequestDetailCard
      request={workstationRequest("dispatch-review-script-minimal", {
        request_id: "request-script-minimal-story",
        responded_request_count: 1,
        script_request: {
          args: ["--work", "work-active-story"],
          attempt: 1,
          command: "script-tool",
          script_request_id:
            "dispatch-review-script-minimal/script-request/1",
        },
        script_response: {
          duration_millis: undefined,
          failure_type: undefined,
          outcome: undefined,
          script_request_id: "",
          stderr: "   ",
          stdout: "  ",
        },
        trace_ids: [],
      })}
    />,
  );

  const responseDetails = within(
    screen.getByRole("region", { name: "Response details" }),
  );

  expect(
    responseDetails.getByText(
      "Script response details are not available for this workstation request.",
    ),
  ).toBeTruthy();
  expect(
    responseDetails.getByText(
      "Duration details are not available for this script response yet.",
    ),
  ).toBeTruthy();
  expect(
    responseDetails.getByText(
      "Failure type is not available for this script response.",
    ),
  ).toBeTruthy();
  expect(
    responseDetails.getByText("Outcome details are not available yet."),
  ).toBeTruthy();
  expect(
    responseDetails.getByText(
      "No stdout was recorded for this script response.",
    ),
  ).toBeTruthy();
  expect(
    responseDetails.getByText(
      "No stderr was recorded for this script response.",
    ),
  ).toBeTruthy();
});
