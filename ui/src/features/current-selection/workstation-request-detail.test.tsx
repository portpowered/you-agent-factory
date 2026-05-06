import { render, screen, within } from "@testing-library/react";
import { inferenceAttempt, workstationRequest } from "./detail-card-test-helpers";
import { WorkstationRequestDetailCard } from "./workstation-request-detail";

describe("WorkstationRequestDetailCard", () => {
  it("keeps inference-backed request and response detail inside inference attempts", () => {
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
    const inferenceAttempts = within(screen.getByRole("region", { name: "Inference attempts" }));

    expect(within(currentSelection).getByRole("heading", { name: "Current selection" })).toBeTruthy();
    expect(within(currentSelection).getAllByText("request-ready-story").length).toBeGreaterThan(0);
    expect(within(currentSelection).getByText("Dispatch ID")).toBeTruthy();
    expect(within(currentSelection).getByRole("heading", { name: "Request counts" })).toBeTruthy();
    expect(
      requestDetails.getByText(
        "Prompt, request payload, working-directory, and worktree details are shown under Inference attempts when available.",
      ),
    ).toBeTruthy();
    expect(responseDetails.getByText("trace-active-story")).toBeTruthy();
    expect(
      responseDetails.getByText(
        "Response, provider-session, and inference metadata details are shown under Inference attempts when available.",
      ),
    ).toBeTruthy();
    expect(inferenceAttempts.getByText("Retry the review with the latest context.")).toBeTruthy();
    expect(inferenceAttempts.getByText("Ready for the next workstation.")).toBeTruthy();
    expect(inferenceAttempts.getByText("codex / session_id / sess-ready-request")).toBeTruthy();
    expect(within(currentSelection).getByText("1m 3s")).toBeTruthy();
    expect(screen.queryByRole("region", { name: "Request metadata" })).toBeNull();
    expect(screen.queryByRole("region", { name: "Response metadata" })).toBeNull();
    expect(screen.queryByRole("heading", { name: "Workstation summary" })).toBeNull();
    expect(screen.queryByText("Runtime labels")).toBeNull();
  });

  it("renders no-response workstation-request details with clear inference-attempt pending copy", () => {
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
      screen.getByText(
        "Response, provider-session, and inference metadata details are shown under Inference attempts when available.",
      ),
    ).toBeTruthy();
    expect(
      screen.getByText("Total duration is not available for this workstation request yet."),
    ).toBeTruthy();
    expect(
      screen.getByText("No inference events are available for this selected work item."),
    ).toBeTruthy();
    expect(screen.queryByRole("region", { name: "Request metadata" })).toBeNull();
    expect(screen.queryByRole("region", { name: "Response metadata" })).toBeNull();
    expect(screen.queryByRole("heading", { name: "Error details" })).toBeNull();
  });

  it("renders request summary fallbacks when projected request identifiers are sparse", () => {
    render(
      <WorkstationRequestDetailCard
        request={workstationRequest("dispatch-review-sparse", {
          request_id: "",
          trace_ids: [],
          transition_id: "review",
          workstation_name: "",
        })}
      />,
    );

    const currentSelection = screen.getByRole("article", { name: "Current selection" });
    const responseDetails = within(screen.getByRole("region", { name: "Response details" }));

    expect(within(currentSelection).getAllByText("dispatch-review-sparse").length).toBeGreaterThan(0);
    expect(
      within(currentSelection).getByText(
        "Request ID is not available for this workstation request.",
      ),
    ).toBeTruthy();
    expect(
      within(currentSelection).getByText(
        "Workstation details are not available for this request.",
      ),
    ).toBeTruthy();
    expect(
      responseDetails.getByText(
        "Trace details are not available for this workstation request yet.",
      ),
    ).toBeTruthy();
  });

  it("renders request-authored prompt bodies inside inference attempts", () => {
    render(
      <WorkstationRequestDetailCard
        request={workstationRequest("dispatch-review-markdown", {
          inference_attempts: [
            inferenceAttempt("dispatch-review-markdown", {
              attempt: 1,
              inference_request_id: "dispatch-review-markdown/inference-request/1",
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
            }),
          ],
          request_id: "request-markdown-story",
        })}
      />,
    );

    const inferenceAttempts = within(screen.getByRole("region", { name: "Inference attempts" }));

    const requestBody = within(inferenceAttempts.getByRole("region", { name: "Request body" }));

    expect(requestBody.getByText(/## Review checklist/)).toBeTruthy();
    expect(requestBody.getByText(/- Check the latest diff/)).toBeTruthy();
    expect(requestBody.getByText(/```text/)).toBeTruthy();
    expect(requestBody.getByText(/bun test/)).toBeTruthy();
  });

  it("renders ordered-list prompt bodies verbatim inside inference attempts", () => {
    render(
      <WorkstationRequestDetailCard
        request={workstationRequest("dispatch-review-ordered", {
          inference_attempts: [
            inferenceAttempt("dispatch-review-ordered", {
              attempt: 1,
              inference_request_id: "dispatch-review-ordered/inference-request/1",
              prompt: [
                "1. Run `bun run lint`",
                "2. `bun run test:unit`",
              ].join("\n"),
            }),
          ],
          request_id: "request-ordered-story",
        })}
      />,
    );

    const inferenceAttempts = within(screen.getByRole("region", { name: "Inference attempts" }));
    const requestBody = within(inferenceAttempts.getByRole("region", { name: "Request body" }));

    expect(requestBody.getByText(/1\. Run `bun run lint`/)).toBeTruthy();
    expect(requestBody.getByText(/2\. `bun run test:unit`/)).toBeTruthy();
  });

  it("renders plain-text prompts as readable request bodies inside inference attempts", () => {
    render(
      <WorkstationRequestDetailCard
        request={workstationRequest("dispatch-review-plain-text", {
          inference_attempts: [
            inferenceAttempt("dispatch-review-plain-text", {
              attempt: 1,
              inference_request_id: "dispatch-review-plain-text/inference-request/1",
              prompt: [
                "Review the current story before approval.",
                "Keep the existing response rendering unchanged.",
              ].join("\n"),
            }),
          ],
          request_id: "request-plain-text-story",
        })}
      />,
    );

    const inferenceAttemptsRegion = screen.getByRole("region", { name: "Inference attempts" });
    const inferenceAttempts = within(inferenceAttemptsRegion);

    expect(inferenceAttempts.queryByRole("heading", { level: 1 })).toBeNull();
    expect(inferenceAttempts.queryByRole("heading", { level: 2 })).toBeNull();
    expect(inferenceAttempts.queryByRole("heading", { level: 3 })).toBeNull();
    expect(inferenceAttempts.queryByRole("list")).toBeNull();
    expect(inferenceAttemptsRegion.querySelectorAll("pre")).toHaveLength(1);
    expect(inferenceAttempts.getByText(/Review the current story before approval\./)).toBeTruthy();
    expect(
      inferenceAttempts.getByText(/Keep the existing response rendering unchanged\./),
    ).toBeTruthy();
  });

  it("renders embedded raw html in prompts as inert text inside inference attempts", () => {
    const { container } = render(
      <WorkstationRequestDetailCard
        request={workstationRequest("dispatch-review-html", {
          inference_attempts: [
            inferenceAttempt("dispatch-review-html", {
              attempt: 1,
              inference_request_id: "dispatch-review-html/inference-request/1",
              prompt: '<button>danger</button>\n\n<script>alert("xss")</script>',
            }),
          ],
          request_id: "request-html-story",
        })}
      />,
    );

    const inferenceAttempts = within(screen.getByRole("region", { name: "Inference attempts" }));

    expect(inferenceAttempts.queryByRole("button", { name: "danger" })).toBeNull();
    expect(container.querySelector("script")).toBeNull();
    expect(inferenceAttempts.getByText(/<button>danger<\/button>/)).toBeTruthy();
    expect(inferenceAttempts.getByText(/<script>alert\("xss"\)<\/script>/)).toBeTruthy();
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
        "Response, provider-session, and inference metadata details are shown under Inference attempts when available.",
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
      responseDetails.queryByText("Provider session details are not available for this workstation request."),
    ).toBeNull();
    expect(screen.queryByRole("heading", { name: "Inference attempts" })).toBeNull();
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

    const requestDetails = within(screen.getByRole("region", { name: "Request details" }));
    const responseDetails = within(screen.getByRole("region", { name: "Response details" }));

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
            script_request_id: "dispatch-review-script-minimal/script-request/1",
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

    const responseDetails = within(screen.getByRole("region", { name: "Response details" }));

    expect(
      responseDetails.getByText(
        "Script response details are not available for this workstation request.",
      ),
    ).toBeTruthy();
    expect(
      responseDetails.getByText("Duration details are not available for this script response yet."),
    ).toBeTruthy();
    expect(
      responseDetails.getByText("Failure type is not available for this script response."),
    ).toBeTruthy();
    expect(responseDetails.getByText("Outcome details are not available yet.")).toBeTruthy();
    expect(responseDetails.getByText("No stdout was recorded for this script response.")).toBeTruthy();
    expect(responseDetails.getByText("No stderr was recorded for this script response.")).toBeTruthy();
  });
});
