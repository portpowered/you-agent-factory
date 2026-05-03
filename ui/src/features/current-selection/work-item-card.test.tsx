import { render, screen, within } from "@testing-library/react";
import { dashboardWorkstationRequestFixtures } from "../../components/dashboard/fixtures";
import { selectWorkItemExecutionDetails } from "./state/executionDetails";
import type { SelectedWorkItemExecutionDetails } from "./state/executionDetails";
import {
  DETAIL_CARD_NOW,
  getSelectedWorkItemFixture,
  inferenceAttempt,
  workstationRequest,
} from "./detail-card-test-helpers";
import { WorkItemDetailCard } from "./work-item-card";
import { describe, it, expect } from "vitest";

function getDetailRow(container: HTMLElement, label: string): HTMLElement {
  const term = within(container).getByText(label, { selector: "dt" });
  const row = term.closest("div");

  if (!(row instanceof HTMLElement)) {
    throw new Error(`expected detail row for ${label}`);
  }

  return row;
}

describe("WorkItemDetailCard", () => {
  it("renders selected work item detail with safe execution details", () => {
    const { dispatchID, execution, selectedNode, workItem, snapshot } =
      getSelectedWorkItemFixture();
    const providerSessions = [
      {
        ...(snapshot.runtime.session.provider_sessions?.[0] ?? {
          dispatch_id: dispatchID,
          outcome: "ACCEPTED",
          transition_id: selectedNode.transition_id,
          work_items: [workItem],
          workstation_name: selectedNode.workstation_name,
        }),
        diagnostics: {
          provider: {
            model: "gpt-5.4",
            provider: "codex",
            request_metadata: {
              prompt_source: "factory-renderer",
            },
          },
          rendered_prompt: {
            system_prompt_hash: "sha256:system-runtime",
            user_message_hash: "sha256:user-runtime",
          },
        },
      },
    ];

    render(
      <WorkItemDetailCard
        dispatchAttempts={providerSessions}
        executionDetails={selectWorkItemExecutionDetails({
          activeExecution: execution,
          dispatchID,
          providerSessions,
          selectedNode,
          workItem,
          workstationRequestsByDispatchID: {
            [dispatchID]: {
              counts: {
                dispatched_count: 1,
                errored_count: 0,
                responded_count: 1,
              },
              dispatch_id: dispatchID,
              request: {
                input_work_items: [workItem],
                input_work_type_ids: [workItem.work_type_id ?? "story"],
                model: "gpt-5.4",
                prompt: "Review the active story and return a concise result.",
                provider: "codex",
                request_metadata: {
                  prompt_source: "factory-renderer",
                },
                request_time: "2026-04-08T12:00:01Z",
                started_at: "2026-04-08T12:00:00Z",
                trace_ids: ["trace-active-story"],
                working_directory: "C:\\work\\portos",
                worktree: "C:\\work\\portos\\.worktrees\\active-story",
              },
              response: {
                diagnostics: providerSessions[0].diagnostics,
                duration_millis: 4000,
                end_time: "2026-04-08T12:00:04Z",
                outcome: "ACCEPTED",
                provider_session: providerSessions[0].provider_session,
                response_text: "The active story is ready for handoff.",
              },
              transition_id: selectedNode.transition_id,
              workstation_name: selectedNode.workstation_name,
            },
          },
        })}
        now={DETAIL_CARD_NOW}
        selectedNode={selectedNode}
        selection={{
          dispatchId: dispatchID,
          execution,
        kind: "work-item",
        nodeId: selectedNode.node_id,
        workItem,
      }}
      workstationRequests={[
        {
          counts: {
            dispatched_count: 1,
            errored_count: 0,
            responded_count: 1,
          },
          dispatch_id: dispatchID,
          request: {
            input_work_items: [workItem],
            input_work_type_ids: [workItem.work_type_id ?? "story"],
            model: "gpt-5.4",
            prompt: "Review the active story and return a concise result.",
            provider: "codex",
            request_metadata: {
              prompt_source: "factory-renderer",
            },
            request_time: "2026-04-08T12:00:01Z",
            started_at: "2026-04-08T12:00:00Z",
            trace_ids: ["trace-active-story"],
            working_directory: "C:\\work\\portos",
            worktree: "C:\\work\\portos\\.worktrees\\active-story",
          },
          response: {
            diagnostics: providerSessions[0].diagnostics,
            duration_millis: 4000,
            end_time: "2026-04-08T12:00:04Z",
            outcome: "ACCEPTED",
            output_work_items: [workItem],
            provider_session: providerSessions[0].provider_session,
            response_text: "The active story is ready for handoff.",
          },
          transition_id: selectedNode.transition_id,
          workstation_name: selectedNode.workstation_name,
        },
      ]}
    />,
  );

    expect(screen.getByRole("heading", { name: "Current selection" })).toBeTruthy();
    expect(screen.getByText(workItem.work_id)).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Execution details" })).toBeTruthy();
    const currentSelection = screen.getByRole("article", { name: "Current selection" });
    const executionDetails = within(screen.getByRole("region", { name: "Execution details" }));
    expect(
      within(getDetailRow(currentSelection, "Dispatch ID")).getByText(dispatchID),
    ).toBeTruthy();
    expect(
      within(getDetailRow(screen.getByRole("region", { name: "Execution details" }), "Provider session"))
        .getByText("sess-active-story"),
    ).toBeTruthy();
    expect(
      within(getDetailRow(currentSelection, "Workstation dispatches")).getByText("1"),
    ).toBeTruthy();
    expect(screen.getAllByText(dispatchID).length).toBeGreaterThan(0);
    expect(screen.getAllByText("Review").length).toBeGreaterThan(0);
    expect(screen.getByText("Current dispatch")).toBeTruthy();
    expect(executionDetails.getByText("trace-active-story")).toBeTruthy();
    expect(screen.getByText(/Source:/)).toBeTruthy();
    expect(screen.getByText("factory-renderer")).toBeTruthy();
    expect(screen.getByText("sha256:system-runtime")).toBeTruthy();
    expect(screen.queryByText("sha256:user-runtime")).toBeNull();
    expect(executionDetails.getByText("Provider")).toBeTruthy();
    expect(executionDetails.getByText("codex")).toBeTruthy();
    expect(executionDetails.getByText("Model")).toBeTruthy();
    expect(executionDetails.getByText("gpt-5.4")).toBeTruthy();
    expect(
      within(currentSelection).queryByRole("heading", { name: "Inference attempts" }),
    ).toBeNull();
    expect(screen.queryByText("Never expose this raw system prompt.")).toBeNull();
    expect(screen.getAllByText("Workstation dispatches").length).toBeGreaterThan(0);
    expect(screen.getByRole("heading", { name: "Workstation dispatches" })).toBeTruthy();
    expect(screen.queryByRole("heading", { name: "Work session runs list" })).toBeNull();
  });

  it("marks only the selected work item's active dispatch inside dispatch history", () => {
    const { dispatchID, execution, selectedNode, workItem } = getSelectedWorkItemFixture();
    const historicalDispatchID = "dispatch-review-old";

    render(
      <WorkItemDetailCard
        executionDetails={selectWorkItemExecutionDetails({
          activeExecution: execution,
          dispatchID,
          selectedNode,
          workItem,
        })}
        now={DETAIL_CARD_NOW}
        dispatchAttempts={[]}
        selectedNode={selectedNode}
        selection={{
          dispatchId: dispatchID,
          execution,
          kind: "work-item",
          nodeId: selectedNode.node_id,
          workItem,
        }}
        workstationRequests={[
          workstationRequest(historicalDispatchID, {
            outcome: "REJECTED",
            prompt: "Historical review request.",
            request_id: "request-review-old",
            started_at: "2026-04-08T12:00:02Z",
          }),
          workstationRequest(dispatchID, {
            prompt: "Active review request.",
            request_id: "request-review-active",
            started_at: "2026-04-08T12:00:03Z",
          }),
        ]}
      />,
    );

    const dispatchHistory = screen.getByRole("region", { name: "Workstation dispatches" });
    const activeCard = within(dispatchHistory).getByText(dispatchID).closest("article");
    const historicalCard = within(dispatchHistory).getByText(historicalDispatchID).closest("article");

    if (!(activeCard instanceof HTMLElement) || !(historicalCard instanceof HTMLElement)) {
      throw new Error("expected active and historical dispatch cards");
    }

    expect(within(activeCard).getByText("Current dispatch")).toBeTruthy();
    expect(within(historicalCard).queryByText("Current dispatch")).toBeNull();
  });

  it("renders unavailable execution details with clear operator copy", () => {
    const { dispatchID, execution, selectedNode, workItem } = getSelectedWorkItemFixture();
    const executionDetails: SelectedWorkItemExecutionDetails = {
      dispatchID,
      elapsedStartTimestamp: execution.started_at,
      inferenceAttempts: [],
      model: { status: "omitted" },
      prompt: { status: "unavailable" },
      provider: { status: "unavailable" },
      providerSession: { status: "unavailable" },
      traceIDs: [],
      workstationName: selectedNode.workstation_name,
      workID: workItem.work_id,
    };

    render(
      <WorkItemDetailCard
        dispatchAttempts={[]}
        executionDetails={executionDetails}
        now={DETAIL_CARD_NOW}
        selectedNode={selectedNode}
        selection={{
          dispatchId: dispatchID,
          execution,
          kind: "work-item",
          nodeId: selectedNode.node_id,
          workItem,
        }}
        workstationRequests={[]}
      />,
    );

    const currentSelection = screen.getByRole("article", { name: "Current selection" });
    expect(within(currentSelection).getByRole("heading", { name: "Execution details" })).toBeTruthy();
    expect(within(currentSelection).queryByText("Model")).toBeNull();
    expect(
      within(currentSelection).queryByText("Model details are not available for this selected run."),
    ).toBeNull();
    expect(
      within(currentSelection).getByText("Prompt details are not available for this selected run."),
    ).toBeTruthy();
    expect(
      within(currentSelection).getByText(
        "Provider session details are not available for this selected run.",
      ),
    ).toBeTruthy();
    expect(
      within(currentSelection).getAllByText("Trace details are not available for this selected run.")
        .length,
    ).toBeGreaterThan(0);
    expect(
      within(currentSelection).getByText(
        "No workstation dispatch has been recorded yet for this work item.",
      ),
    ).toBeTruthy();
    expect(screen.queryByRole("link", { name: "Open trace" })).toBeNull();
  });

  it("does not render a separate inference attempts section for selected work items", () => {
    const { dispatchID, execution, selectedNode, workItem } = getSelectedWorkItemFixture();

    render(
      <WorkItemDetailCard
        dispatchAttempts={[]}
        executionDetails={selectWorkItemExecutionDetails({
          activeExecution: execution,
          dispatchID,
          inferenceAttemptsByDispatchID: {
            [dispatchID]: {
              [`${dispatchID}/inference-request/1`]: inferenceAttempt(dispatchID, {
                inference_request_id: `${dispatchID}/inference-request/1`,
                outcome: "FAILED",
              }),
            },
          },
          selectedNode,
          workItem,
        })}
        now={DETAIL_CARD_NOW}
        selectedNode={selectedNode}
        selection={{
          dispatchId: dispatchID,
          execution,
          kind: "work-item",
          nodeId: selectedNode.node_id,
          workItem,
        }}
        workstationRequests={[]}
      />,
    );

    const currentSelection = screen.getByRole("article", { name: "Current selection" });
    expect(
      within(currentSelection).queryByRole("heading", { name: "Inference attempts" }),
    ).toBeNull();
    expect(within(currentSelection).queryByRole("region", { name: "Inference attempts" })).toBeNull();
    expect(within(currentSelection).getByRole("heading", { name: "Workstation dispatches" })).toBeTruthy();
  });

  it("renders a pending dispatch without provider-session metadata as a workstation dispatch", () => {
    const { dispatchID, execution, selectedNode, workItem } = getSelectedWorkItemFixture();
    const dispatchAttempts = [
      {
        dispatch_id: dispatchID,
        outcome: "PENDING",
        transition_id: selectedNode.transition_id,
        work_items: [workItem],
        workstation_name: selectedNode.workstation_name,
      },
    ];

    render(
      <WorkItemDetailCard
        dispatchAttempts={dispatchAttempts}
        executionDetails={{
          dispatchID,
          elapsedStartTimestamp: execution.started_at,
          inferenceAttempts: [],
          model: { status: "omitted" },
          prompt: { status: "pending" },
          provider: { status: "pending" },
          providerSession: { status: "pending" },
          traceIDs: [workItem.trace_id ?? "trace-active-story"],
          workstationName: selectedNode.workstation_name,
          workID: workItem.work_id,
        }}
        now={DETAIL_CARD_NOW}
        selectedNode={selectedNode}
        selection={{
          dispatchId: dispatchID,
          execution,
          kind: "work-item",
          nodeId: selectedNode.node_id,
          workItem,
        }}
        workstationRequests={[]}
      />,
    );

    const currentSelection = screen.getByRole("article", { name: "Current selection" });
    expect(
      within(getDetailRow(currentSelection, "Workstation dispatches")).getByText("1"),
    ).toBeTruthy();
    expect(within(currentSelection).getAllByText(dispatchID).length).toBeGreaterThan(0);
    expect(
      within(currentSelection).queryByText(
        "No workstation dispatch has been recorded yet for this work item.",
      ),
    ).toBeNull();
    expect(within(currentSelection).getByText("Session log unavailable")).toBeTruthy();
  });

  it("omits the model row while preserving other execution details for historical selections", () => {
    const { dispatchID, execution, selectedNode, workItem } = getSelectedWorkItemFixture();
    const executionDetails: SelectedWorkItemExecutionDetails = {
      dispatchID,
      elapsedStartTimestamp: execution.started_at,
      inferenceAttempts: [],
      model: { status: "omitted" },
      prompt: {
        promptSource: "factory-renderer",
        source: "diagnostics",
        status: "available",
        systemPromptHash: "sha256:system-runtime",
      },
      provider: { source: "provider-diagnostics", status: "available", value: "codex" },
      providerSession: {
        source: "provider-session",
        status: "available",
        value: "sess-active-story",
      },
      traceIDs: ["trace-active-story"],
      workstationName: selectedNode.workstation_name,
      workID: workItem.work_id,
    };

    render(
      <WorkItemDetailCard
        dispatchAttempts={[]}
        executionDetails={executionDetails}
        now={DETAIL_CARD_NOW}
        selectedNode={selectedNode}
        selection={{
          dispatchId: dispatchID,
          execution,
          kind: "work-item",
          nodeId: selectedNode.node_id,
          workItem,
        }}
        workstationRequests={[]}
      />,
    );

    const currentSelection = screen.getByRole("article", { name: "Current selection" });
    expect(within(currentSelection).queryByText("Model")).toBeNull();
    expect(
      within(currentSelection).queryByText("Model details are not available for this selected run."),
    ).toBeNull();
    expect(
      within(getDetailRow(currentSelection, "Dispatch ID")).getByText(dispatchID),
    ).toBeTruthy();
    expect(
      within(getDetailRow(currentSelection, "Provider session")).getByText("sess-active-story"),
    ).toBeTruthy();
    expect(within(currentSelection).getAllByText(dispatchID).length).toBeGreaterThan(0);
    expect(within(currentSelection).getAllByText("Review").length).toBeGreaterThan(0);
    expect(within(currentSelection).getByText("trace-active-story")).toBeTruthy();
    expect(within(currentSelection).getByText("factory-renderer")).toBeTruthy();
    expect(within(currentSelection).getByText("sha256:system-runtime")).toBeTruthy();
  });

  it("renders a unified pending dispatch-history row with request details and no-response-yet copy", () => {
    const { dispatchID, execution, selectedNode, workItem } = getSelectedWorkItemFixture();

    render(
      <WorkItemDetailCard
        executionDetails={selectWorkItemExecutionDetails({
          activeExecution: execution,
          dispatchID,
          selectedNode,
          workItem,
        })}
        now={DETAIL_CARD_NOW}
        dispatchAttempts={[]}
        selectedNode={selectedNode}
        selection={{
          dispatchId: dispatchID,
          execution,
          kind: "work-item",
          nodeId: selectedNode.node_id,
          workItem,
        }}
        workstationRequests={[
          workstationRequest(dispatchID, {
            prompt: "Review the active story while the provider response is still pending.",
            request_metadata: {
              prompt_source: "factory-renderer",
            },
            trace_ids: ["trace-active-story"],
            work_items: [workItem],
          }),
        ]}
      />,
    );

    const currentSelection = screen.getByRole("article", { name: "Current selection" });
    const dispatchHistory = within(screen.getByRole("region", { name: "Workstation dispatches" }));
    const responseDetails = within(screen.getByRole("region", { name: "Response details" }));
    expect(within(currentSelection).getByRole("heading", { name: "Workstation dispatches" })).toBeTruthy();
    expect(within(currentSelection).getByText("Review the active story while the provider response is still pending.")).toBeTruthy();
    expect(within(currentSelection).getByText("No response yet for this dispatch.")).toBeTruthy();
    expect(dispatchHistory.getByRole("button", { name: "Select work item Active Story" })).toBeTruthy();
    expect(responseDetails.getByRole("link", { name: "trace-active-story" })).toBeTruthy();
  });

  it("renders markdown-authored dispatch-history prompts through the shared request renderer", () => {
    const { dispatchID, execution, selectedNode, workItem } = getSelectedWorkItemFixture();

    render(
      <WorkItemDetailCard
        executionDetails={selectWorkItemExecutionDetails({
          activeExecution: execution,
          dispatchID,
          selectedNode,
          workItem,
        })}
        now={DETAIL_CARD_NOW}
        dispatchAttempts={[]}
        selectedNode={selectedNode}
        selection={{
          dispatchId: dispatchID,
          execution,
          kind: "work-item",
          nodeId: selectedNode.node_id,
          workItem,
        }}
        workstationRequests={[
          workstationRequest(dispatchID, {
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
            request_metadata: {
              prompt_source: "factory-renderer",
            },
            trace_ids: ["trace-active-story"],
            work_items: [workItem],
          }),
        ]}
      />,
    );

    const dispatchHistory = screen.getByRole("region", { name: "Workstation dispatches" });
    const dispatchCard = within(dispatchHistory).getByText(dispatchID).closest("article");

    if (!(dispatchCard instanceof HTMLElement)) {
      throw new Error("expected markdown dispatch history card");
    }

    const requestDetails = within(within(dispatchCard).getByRole("region", { name: "Request details" }));

    expect(
      requestDetails.getByRole("heading", { level: 2, name: "Review checklist" }),
    ).toBeTruthy();
    expect(requestDetails.getByRole("list")).toBeTruthy();
    expect(requestDetails.getByText("Check the latest diff")).toBeTruthy();
    expect(requestDetails.getAllByText("bun test", { selector: "code" })).toHaveLength(2);
    expect(requestDetails.getAllByText("bun test", { selector: "pre code" })).toHaveLength(1);
    expect(within(dispatchCard).queryByText("## Review checklist")).toBeNull();
  });

  it("renders completed failed dispatch-history details from the same row", () => {
    const { dispatchID, execution, selectedNode, workItem } = getSelectedWorkItemFixture();

    render(
      <WorkItemDetailCard
        executionDetails={selectWorkItemExecutionDetails({
          activeExecution: execution,
          dispatchID,
          selectedNode,
          workItem,
        })}
        now={DETAIL_CARD_NOW}
        dispatchAttempts={[]}
        selectedNode={selectedNode}
        selection={{
          dispatchId: dispatchID,
          execution,
          kind: "work-item",
          nodeId: selectedNode.node_id,
          workItem,
        }}
        workstationRequests={[
          workstationRequest(dispatchID, {
            errored_request_count: 1,
            failure_message: "Provider rate limit exceeded while reviewing the story.",
            failure_reason: "provider_rate_limit",
            outcome: "FAILED",
            response_view: {
              error_class: "provider_rate_limit",
              failure_message: "Provider rate limit exceeded while reviewing the story.",
              failure_reason: "provider_rate_limit",
              outcome: "FAILED",
              output_work_items: [workItem],
            },
            trace_ids: ["trace-active-story"],
            work_items: [workItem],
          }),
        ]}
      />,
    );

    const failureDetails = within(screen.getByRole("region", { name: "Failure details" }));
    const dispatchHistory = within(screen.getByRole("region", { name: "Workstation dispatches" }));
    expect(failureDetails.getAllByText("provider_rate_limit").length).toBeGreaterThan(0);
    expect(failureDetails.getByText("Provider rate limit exceeded while reviewing the story.")).toBeTruthy();
    expect(within(screen.getByRole("region", { name: "Response details" })).getByText("Response text is unavailable because this dispatch ended with an error.")).toBeTruthy();
    expect(dispatchHistory.getAllByRole("button", { name: "Select work item Active Story" }).length).toBeGreaterThan(0);
  });

  it("renders pending script dispatch-history details for the selected work item", () => {
    const { dispatchID, execution, selectedNode, workItem } = getSelectedWorkItemFixture();

    render(
      <WorkItemDetailCard
        executionDetails={selectWorkItemExecutionDetails({
          activeExecution: execution,
          dispatchID,
          selectedNode,
          workItem,
        })}
        now={DETAIL_CARD_NOW}
        dispatchAttempts={[]}
        selectedNode={selectedNode}
        selection={{
          dispatchId: dispatchID,
          execution,
          kind: "work-item",
          nodeId: selectedNode.node_id,
          workItem,
        }}
        workstationRequests={[dashboardWorkstationRequestFixtures.scriptPending]}
      />,
    );

    const dispatchHistory = screen.getByRole("region", { name: "Workstation dispatches" });
    const dispatchCard = within(dispatchHistory)
      .getByText(dashboardWorkstationRequestFixtures.scriptPending.dispatch_id)
      .closest("article");

    if (!(dispatchCard instanceof HTMLElement)) {
      throw new Error("expected pending script dispatch history card");
    }

    expect(
      within(dispatchCard).getByText(
        "Prompt details are not applicable to this script-backed dispatch.",
      ),
    ).toBeTruthy();
    expect(
      within(dispatchCard).getByText(
        dashboardWorkstationRequestFixtures.scriptPending.script_request?.command ?? "",
      ),
    ).toBeTruthy();
    expect(
      within(dispatchCard).getByText(
        dashboardWorkstationRequestFixtures.scriptPending.script_request?.script_request_id ?? "",
      ),
    ).toBeTruthy();
    expect(within(dispatchCard).getByText("--work")).toBeTruthy();
    expect(within(dispatchCard).getByText("No script response yet for this dispatch.")).toBeTruthy();
    expect(within(dispatchCard).queryByText("No response yet for this dispatch.")).toBeNull();
  });

  it("renders selected-work script success details from the dispatch-history row", () => {
    const { dispatchID, execution, selectedNode, workItem } = getSelectedWorkItemFixture();

    render(
      <WorkItemDetailCard
        executionDetails={selectWorkItemExecutionDetails({
          activeExecution: execution,
          dispatchID,
          selectedNode,
          workItem,
        })}
        now={DETAIL_CARD_NOW}
        dispatchAttempts={[]}
        selectedNode={selectedNode}
        selection={{
          dispatchId: dispatchID,
          execution,
          kind: "work-item",
          nodeId: selectedNode.node_id,
          workItem,
        }}
        workstationRequests={[dashboardWorkstationRequestFixtures.scriptSuccess]}
      />,
    );

    const dispatchHistory = screen.getByRole("region", { name: "Workstation dispatches" });
    const dispatchCard = within(dispatchHistory)
      .getByText(dashboardWorkstationRequestFixtures.scriptSuccess.dispatch_id)
      .closest("article");

    if (!(dispatchCard instanceof HTMLElement)) {
      throw new Error("expected script success dispatch history card");
    }

    expect(within(dispatchCard).getAllByText("SUCCEEDED").length).toBeGreaterThan(0);
    expect(
      within(dispatchCard).getByText(
        dashboardWorkstationRequestFixtures.scriptSuccess.script_request?.command ?? "",
      ),
    ).toBeTruthy();
    expect(
      within(dispatchCard).getAllByText(
        dashboardWorkstationRequestFixtures.scriptSuccess.script_response?.script_request_id ?? "",
      ).length,
    ).toBeGreaterThan(0);
    expect(within(dispatchCard).getAllByText("222ms").length).toBeGreaterThan(0);
    expect(within(dispatchCard).getByText(/script success stdout/)).toBeTruthy();
    expect(within(dispatchCard).queryByText("Provider session")).toBeNull();
  });

  it("renders selected-work script failure details from the dispatch-history row", () => {
    const { dispatchID, execution, selectedNode, workItem } = getSelectedWorkItemFixture();

    render(
      <WorkItemDetailCard
        executionDetails={selectWorkItemExecutionDetails({
          activeExecution: execution,
          dispatchID,
          selectedNode,
          workItem,
        })}
        now={DETAIL_CARD_NOW}
        dispatchAttempts={[]}
        selectedNode={selectedNode}
        selection={{
          dispatchId: dispatchID,
          execution,
          kind: "work-item",
          nodeId: selectedNode.node_id,
          workItem,
        }}
        workstationRequests={[dashboardWorkstationRequestFixtures.scriptFailed]}
      />,
    );

    const dispatchHistory = screen.getByRole("region", { name: "Workstation dispatches" });
    const dispatchCard = within(dispatchHistory)
      .getByText(dashboardWorkstationRequestFixtures.scriptFailed.dispatch_id)
      .closest("article");

    if (!(dispatchCard instanceof HTMLElement)) {
      throw new Error("expected script failure dispatch history card");
    }

    expect(within(dispatchCard).getAllByText("TIMED_OUT").length).toBeGreaterThan(0);
    expect(within(dispatchCard).getAllByText("TIMEOUT").length).toBeGreaterThan(0);
    expect(within(dispatchCard).getAllByText(/script timed out/i).length).toBeGreaterThan(0);
    expect(
      within(dispatchCard).queryByText(
        "Response text is unavailable because this dispatch ended with an error.",
      ),
    ).toBeNull();
  });

  it("keeps rejected dispatch request and response details paired on the same history row", () => {
    const { dispatchID, execution, selectedNode, workItem } = getSelectedWorkItemFixture();

    render(
      <WorkItemDetailCard
        executionDetails={selectWorkItemExecutionDetails({
          activeExecution: execution,
          dispatchID,
          selectedNode,
          workItem,
        })}
        now={DETAIL_CARD_NOW}
        dispatchAttempts={[]}
        selectedNode={selectedNode}
        selection={{
          dispatchId: dispatchID,
          execution,
          kind: "work-item",
          nodeId: selectedNode.node_id,
          workItem,
        }}
        workstationRequests={[dashboardWorkstationRequestFixtures.rejected]}
      />,
    );

    const currentSelection = screen.getByRole("article", { name: "Current selection" });
    const dispatchHistory = screen.getByRole("region", { name: "Workstation dispatches" });
    const dispatchCard = within(dispatchHistory)
      .getByText(dashboardWorkstationRequestFixtures.rejected.dispatch_id)
      .closest("article");

    if (!(dispatchCard instanceof HTMLElement)) {
      throw new Error("expected rejected dispatch history card");
    }

    expect(within(currentSelection).getByRole("heading", { name: "Workstation dispatches" })).toBeTruthy();
    expect(within(currentSelection).queryByRole("heading", { name: "Work session runs list" })).toBeNull();
    expect(
      within(dispatchCard).getByText(
        "Review the active story and explain what needs to change before approval.",
      ),
    ).toBeTruthy();
    expect(
      within(dispatchCard).getByText(
        "The active story needs revision before it can continue.",
      ),
    ).toBeTruthy();
    expect(within(dispatchCard).getByText("codex / session_id / sess-rejected-story")).toBeTruthy();
    expect(within(dispatchCard).queryByText("No response yet for this dispatch.")).toBeNull();
  });

});

