import { render, screen, within } from "@testing-library/react";
import { inferenceAttempt, workstationRequest } from "./detail-card-test-helpers";
import { WorkstationRequestDetailCard } from "./workstation-request-detail";

describe("WorkstationRequestDetailCard", () => {
  it("renders dedicated workstation-request details with request and response metadata", () => {
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

    const currentSelection = screen.getByRole("article", { name: "Current selection" });
    const requestDetails = within(screen.getByRole("region", { name: "Request details" }));
    const responseDetails = within(screen.getByRole("region", { name: "Response details" }));
    const requestMetadata = within(screen.getByRole("region", { name: "Request metadata" }));
    const responseMetadata = within(screen.getByRole("region", { name: "Response metadata" }));

    expect(within(currentSelection).getByRole("heading", { name: "Current selection" })).toBeTruthy();
    expect(within(currentSelection).getAllByText("request-ready-story").length).toBeGreaterThan(0);
    expect(within(currentSelection).getByText("Dispatch ID")).toBeTruthy();
    expect(within(currentSelection).getByRole("heading", { name: "Request counts" })).toBeTruthy();
    expect(requestDetails.getByText("Review the active story and decide whether it is ready.")).toBeTruthy();
    expect(requestDetails.getByText("C:\\work\\portos")).toBeTruthy();
    expect(requestDetails.getByText("C:\\work\\portos\\.worktrees\\active-story")).toBeTruthy();
    expect(responseDetails.getByText("codex / session_id / sess-ready-request")).toBeTruthy();
    expect(responseDetails.getByText("trace-active-story")).toBeTruthy();
    expect(responseDetails.getByText("Ready for the next workstation.")).toBeTruthy();
    expect(requestMetadata.getByText("factory-renderer")).toBeTruthy();
    expect(responseMetadata.getByText("stop")).toBeTruthy();
    expect(within(currentSelection).getByText("1m 3s")).toBeTruthy();
    expect(screen.queryByRole("heading", { name: "Workstation summary" })).toBeNull();
    expect(screen.queryByText("Runtime labels")).toBeNull();
  });

  it("renders no-response workstation-request details with clear unavailable copy", () => {
    render(
      <WorkstationRequestDetailCard
        request={workstationRequest("dispatch-review-pending", {
          prompt: "Review the active story while the provider response is still pending.",
          request_id: "request-pending-story",
          request_metadata: {
            prompt_source: "factory-renderer",
          },
          working_directory: "C:\\work\\portos",
          worktree: "C:\\work\\portos\\.worktrees\\pending-story",
        })}
      />,
    );

    expect(screen.getAllByText("request-pending-story").length).toBeGreaterThan(0);
    expect(
      screen.getByText("Response text is not available for this workstation request yet."),
    ).toBeTruthy();
    expect(
      screen.getByText("Response metadata is not available for this workstation request yet."),
    ).toBeTruthy();
    expect(
      screen.getByText("Total duration is not available for this workstation request yet."),
    ).toBeTruthy();
    expect(screen.getByText("factory-renderer")).toBeTruthy();
    expect(screen.queryByRole("heading", { name: "Error details" })).toBeNull();
  });

  it("renders request-authored markdown prompts through the shared request renderer", () => {
    render(
      <WorkstationRequestDetailCard
        request={workstationRequest("dispatch-review-markdown", {
          prompt: [
            "## Review checklist",
            "",
            "- Check the latest diff",
            "- Run `bun test` before approval",
            "",
            "```text",
            "bun test",
            "```",
          ].join("\n"),
          request_id: "request-markdown-story",
        })}
      />,
    );

    const requestDetails = within(screen.getByRole("region", { name: "Request details" }));

    expect(requestDetails.getByRole("heading", { level: 2, name: "Review checklist" })).toBeTruthy();
    expect(requestDetails.getByRole("list")).toBeTruthy();
    expect(requestDetails.getByText("Check the latest diff")).toBeTruthy();
    expect(requestDetails.getAllByText("bun test", { selector: "code" })).toHaveLength(2);
    expect(requestDetails.getAllByText("bun test", { selector: "pre code" })).toHaveLength(1);
  });

  it("renders plain-text prompts as readable fallback through the shared request renderer", () => {
    render(
      <WorkstationRequestDetailCard
        request={workstationRequest("dispatch-review-plain-text", {
          prompt: [
            "Review the current story before approval.",
            "Keep the existing response rendering unchanged.",
          ].join("\n"),
          request_id: "request-plain-text-story",
        })}
      />,
    );

    const requestDetailsRegion = screen.getByRole("region", { name: "Request details" });
    const requestDetails = within(requestDetailsRegion);

    expect(requestDetails.queryByRole("heading", { level: 1 })).toBeNull();
    expect(requestDetails.queryByRole("heading", { level: 2 })).toBeNull();
    expect(requestDetails.queryByRole("heading", { level: 3 })).toBeNull();
    expect(requestDetails.queryByRole("list")).toBeNull();
    expect(requestDetailsRegion.querySelectorAll("dd p")).toHaveLength(1);
    expect(requestDetails.getByText(/Review the current story before approval\./)).toBeTruthy();
    expect(requestDetails.getByText(/Keep the existing response rendering unchanged\./)).toBeTruthy();
  });

  it("renders embedded raw html in prompts as inert text", () => {
    const { container } = render(
      <WorkstationRequestDetailCard
        request={workstationRequest("dispatch-review-html", {
          prompt: '<button>danger</button>\n\n<script>alert("xss")</script>',
          request_id: "request-html-story",
        })}
      />,
    );

    const requestDetails = within(screen.getByRole("region", { name: "Request details" }));

    expect(requestDetails.queryByRole("button", { name: "danger" })).toBeNull();
    expect(container.querySelector("script")).toBeNull();
    expect(requestDetails.getByText(/<button>danger<\/button>/)).toBeTruthy();
    expect(requestDetails.getByText(/<script>alert\("xss"\)<\/script>/)).toBeTruthy();
  });

  it("renders errored workstation-request details from projected failure fields", () => {
    render(
      <WorkstationRequestDetailCard
        request={workstationRequest("dispatch-review-error", {
          errored_request_count: 1,
          failure_message: "Provider rate limit exceeded while reviewing the story.",
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

    const errorDetails = within(screen.getByRole("region", { name: "Error details" }));

    expect(screen.getByRole("heading", { name: "Error details" })).toBeTruthy();
    expect(errorDetails.getByText("provider_rate_limit")).toBeTruthy();
    expect(errorDetails.getByText("Provider rate limit exceeded while reviewing the story.")).toBeTruthy();
    expect(
      screen.getByText(
        "Response text is unavailable because this workstation request ended with an error.",
      ),
    ).toBeTruthy();
    expect(
      screen.getByText(
        "Response metadata is unavailable because this workstation request ended with an error.",
      ),
    ).toBeTruthy();
    expect(screen.getAllByText("FAILED").length).toBeGreaterThan(0);
  });

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
            script_request_id: "dispatch-review-script-pending/script-request/1",
          },
        })}
      />,
    );

    const requestDetails = within(screen.getByRole("region", { name: "Request details" }));
    const responseDetails = within(screen.getByRole("region", { name: "Response details" }));

    expect(screen.getAllByText("request-script-pending-story").length).toBeGreaterThan(0);
    expect(screen.getByText("Provider details are not applicable to this script-backed workstation request.")).toBeTruthy();
    expect(screen.getByText("Model details are not applicable to this script-backed workstation request.")).toBeTruthy();
    expect(requestDetails.getByText("script-tool")).toBeTruthy();
    expect(requestDetails.getByText("--work")).toBeTruthy();
    expect(requestDetails.getByText("work-active-story")).toBeTruthy();
    expect(
      requestDetails.getByText("dispatch-review-script-pending/script-request/1"),
    ).toBeTruthy();
    expect(
      requestDetails.getByText(
        "Prompt details are not applicable to this script-backed workstation request.",
      ),
    ).toBeTruthy();
    expect(
      responseDetails.getByText(
        "Script response details are not available for this workstation request yet.",
      ),
    ).toBeTruthy();
    expect(
      responseDetails.queryByText("Provider session details are not available for this workstation request."),
    ).toBeNull();
    expect(screen.queryByRole("heading", { name: "Inference attempts" })).toBeNull();
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
            script_request_id: "dispatch-review-script-success/script-request/1",
          },
          script_response: {
            duration_millis: 222,
            outcome: "SUCCEEDED",
            script_request_id: "dispatch-review-script-success/script-request/1",
            stderr: "",
            stdout: "script success stdout\n",
          },
        })}
      />,
    );

    const responseDetails = within(screen.getByRole("region", { name: "Response details" }));

    expect(screen.getAllByText("request-script-success-story").length).toBeGreaterThan(0);
    expect(screen.getAllByText("SUCCEEDED").length).toBeGreaterThan(0);
    expect(screen.getAllByText("222ms").length).toBeGreaterThan(0);
    expect(
      responseDetails.getByText("dispatch-review-script-success/script-request/1"),
    ).toBeTruthy();
    expect(responseDetails.getByText("script success stdout")).toBeTruthy();
    expect(responseDetails.getByText("No stderr was recorded for this script response.")).toBeTruthy();
    expect(
      screen.getByText("Response metadata is not available for this script-backed workstation request."),
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

    const responseDetails = within(screen.getByRole("region", { name: "Response details" }));
    const errorDetails = within(screen.getByRole("region", { name: "Error details" }));

    expect(screen.getAllByText("request-script-failed-story").length).toBeGreaterThan(0);
    expect(screen.getAllByText("TIMED_OUT").length).toBeGreaterThan(0);
    expect(screen.getAllByText("500ms").length).toBeGreaterThan(0);
    expect(responseDetails.getByText("TIMEOUT")).toBeTruthy();
    expect(responseDetails.getByText("script timed out")).toBeTruthy();
    expect(responseDetails.getByText("No stdout was recorded for this script response.")).toBeTruthy();
    expect(errorDetails.getByText("script_timeout")).toBeTruthy();
    expect(errorDetails.getByText("Script timed out.")).toBeTruthy();
  });
});
