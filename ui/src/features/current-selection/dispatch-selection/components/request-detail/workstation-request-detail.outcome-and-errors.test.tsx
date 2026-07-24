import { render, screen, within } from "@testing-library/react";
import {
  inferenceAttempt,
  workstationRequest,
} from "../../../base/components/detail-card/detail-card-test-helpers";
import { WorkstationRequestDetailCard } from "./workstation-request-detail";
import { buildWorkstationRequestDetailView } from "./workstation-request-detail-view";

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

describe("WorkstationRequestDetailCard agent run inspection", () => {
  function renderAgentRunRequestDetailCard(
    overrides: Parameters<typeof workstationRequest>[1] = {},
  ) {
    render(
      <WorkstationRequestDetailCard
        request={workstationRequest("dispatch-agent-run-inspection", {
          request_id: "request-agent-run-story",
          responded_request_count: 0,
          workstation_name: "agent-review",
          workstation_type: "AGENT_RUN",
          ...overrides,
        })}
      />,
    );
  }

  function getAgentRunInspectionRegion() {
    return within(screen.getByRole("region", { name: "Agent run inspection" }));
  }

  it("renders an explicit empty state for AGENT_RUN requests without inspection metadata", () => {
    renderAgentRunRequestDetailCard();

    expect(
      getAgentRunInspectionRegion().getByText(
        "Agent run inspection metadata is not available for this workstation request yet.",
      ),
    ).toBeTruthy();
    expect(
      screen.queryByRole("heading", { name: "Inference attempts" }),
    ).toBeNull();
  });

  it("renders populated agent-run inspection metadata separately from inference attempts", () => {
    renderAgentRunRequestDetailCard({
      agent_run_inspection: {
        execution_behavior: "agent_run",
        failure_class: "agent_run_tool_denied",
        recovery_action: "review_tool_policy",
        tool_policy: "READ_ONLY",
        tool_diagnostics: [
          {
            detail: "write_file is not allowed in read-only mode",
            phase: "denied",
            tool_name: "write_file",
          },
        ],
        transcript: [
          {
            role: "assistant",
            summary: "Attempted to write output file.",
          },
        ],
      },
      responded_request_count: 1,
    });

    const inspection = getAgentRunInspectionRegion();

    expect(inspection.getByText("READ_ONLY")).toBeTruthy();
    expect(inspection.getByText("agent_run_tool_denied")).toBeTruthy();
    expect(inspection.getByText("review_tool_policy")).toBeTruthy();
    expect(inspection.getByText("write_file · denied")).toBeTruthy();
    expect(
      inspection.getByText("write_file is not allowed in read-only mode"),
    ).toBeTruthy();
    expect(inspection.getByText("assistant")).toBeTruthy();
    expect(
      inspection.getByText("Attempted to write output file."),
    ).toBeTruthy();
    expect(
      screen.queryByRole("heading", { name: "Inference attempts" }),
    ).toBeNull();
  });
});

describe("buildWorkstationRequestDetailView agent taxonomy", () => {
  it("treats AGENT_RUN requests as agent-backed even when inspection metadata is missing", () => {
    const view = buildWorkstationRequestDetailView(
      workstationRequest("dispatch-agent-run-empty", {
        workstation_type: "AGENT_RUN",
      }),
    );

    expect(view.isAgentBackedRequest).toBe(true);
  });

  it("does not treat inference requests as agent-backed when inspection metadata is absent", () => {
    const view = buildWorkstationRequestDetailView(
      workstationRequest("dispatch-inference-empty", {
        workstation_type: "INFERENCE_RUN",
      }),
    );

    expect(view.isAgentBackedRequest).toBe(false);
  });

  it("treats legacy MODEL_WORKSTATION requests as agent-backed", () => {
    const view = buildWorkstationRequestDetailView(
      workstationRequest("dispatch-legacy-agent-empty", {
        workstation_type: "MODEL_WORKSTATION",
      }),
    );

    expect(view.isAgentBackedRequest).toBe(true);
  });
});
