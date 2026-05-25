import { render, screen, within } from "@testing-library/react";
import {
  inferenceAttempt,
  workstationRequest,
} from "./detail-card-test-helpers";
import { WorkstationRequestDetailCard } from "./workstation-request-detail";

describe("WorkstationRequestDetailCard failed outcomes", () => {
  it("surfaces failed outcome reason and message in the top-level outcome row", () => {
    render(
      <WorkstationRequestDetailCard
        request={workstationRequest("dispatch-review-failed", {
          failure_message:
            "Provider rate limit exceeded while generating the analysis.",
          failure_reason: "provider_rate_limit",
          outcome: "FAILED",
          request_id: "request-failed-story",
        })}
      />,
    );

    const currentSelection = screen.getByRole("article", {
      name: "Current selection",
    });
    const outcomeRow = within(currentSelection)
      .getByText("Outcome")
      .closest("div");

    expect(outcomeRow?.textContent).toContain("FAILED");
    expect(outcomeRow?.textContent).toContain(
      "Failure reason: provider_rate_limit",
    );
    expect(outcomeRow?.textContent).toContain(
      "Failure message: Provider rate limit exceeded while generating the analysis.",
    );
  });

  it("keeps failed outcome summary stable when no failure details are available", () => {
    render(
      <WorkstationRequestDetailCard
        request={workstationRequest("dispatch-review-failed-no-details", {
          outcome: "FAILED",
          request_id: "request-failed-no-details",
        })}
      />,
    );

    const currentSelection = screen.getByRole("article", {
      name: "Current selection",
    });
    const outcomeRow = within(currentSelection)
      .getByText("Outcome")
      .closest("div");

    expect(outcomeRow?.textContent).toContain("FAILED");
    expect(outcomeRow?.textContent).not.toContain("Failure reason:");
    expect(outcomeRow?.textContent).not.toContain("Failure message:");
  });

  it("renders errored workstation-request details from projected failure fields", () => {
    render(
      <WorkstationRequestDetailCard
        request={workstationRequest("dispatch-review-error", {
          errored_request_count: 1,
          failure_message:
            "Provider rate limit exceeded while reviewing the story.",
          failure_reason: "provider_rate_limit",
          inference_attempts: [
            inferenceAttempt("dispatch-review-error", {
              error_class: "provider_rate_limit",
              inference_request_id: "dispatch-review-error/inference-request/1",
              outcome: "FAILED",
              response_time: "2026-04-08T12:00:02Z",
            }),
          ],
          outcome: "FAILED",
          prompt: "Review the blocked story and explain the failure.",
          request_id: "request-error-story",
          responded_request_count: 0,
        })}
      />,
    );

    const errorDetails = within(
      screen.getByRole("region", { name: "Error details" }),
    );

    expect(screen.getByRole("heading", { name: "Error details" })).toBeTruthy();
    expect(errorDetails.getByText("provider_rate_limit")).toBeTruthy();
    expect(
      errorDetails.getByText(
        "Provider rate limit exceeded while reviewing the story.",
      ),
    ).toBeTruthy();
    expect(screen.getAllByText("FAILED").length).toBeGreaterThan(0);
  });
});
