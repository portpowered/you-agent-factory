import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { formatLocalDateTime } from "../../../components/ui/formatters";
import type {
  DashboardTrace,
  DashboardWorkItemRef,
} from "../../../api/dashboard/types";
import { dashboardWorkstationRequestFixtures } from "../../../components/dashboard/fixtures";
import { CurrentSelectionLocaleProvider } from "./current-selection-locale";
import {
  DETAIL_CARD_NOW,
  getSelectedWorkItemFixture,
  inferenceAttempt,
  workstationRequest,
} from "./detail-card-test-helpers";
import type { SelectedWorkItemExecutionDetails } from "../state/executionDetails";
import { selectWorkItemExecutionDetails } from "../state/executionDetails";
import { providerSessionSelectionKey } from "../../provider-session-detail/lib/provider-session-ref";
import { WorkItemDetailCard } from "./work-item-card";

function getDetailRow(container: HTMLElement, label: string): HTMLElement {
  const term = within(container).getByText(label, { selector: "dt" });
  const row = term.closest("div");

  if (!(row instanceof HTMLElement)) {
    throw new Error(`expected detail row for ${label}`);
  }

  return row;
}

function expectDispatchCardToHideTransitionId(
  dispatchCard: HTMLElement,
  transitionIdLabel: string,
  transitionIdValue: string | undefined,
): void {
  expect(within(dispatchCard).queryByText(transitionIdLabel)).toBeNull();
  expect(within(dispatchCard).queryByText(transitionIdValue ?? "")).toBeNull();
}

function expandDispatchSection(
  container: HTMLElement,
  title: string,
  expandLabel = "Expand",
): HTMLElement {
  const section = within(container).getByRole("region", { name: title });
  const toggle = within(section).getByRole("button", { name: expandLabel });

  expect(toggle.getAttribute("aria-expanded")).toBe("false");
  fireEvent.click(toggle);
  expect(toggle.getAttribute("aria-expanded")).toBe("true");

  return section;
}

function expandInferenceAttempt(
  container: HTMLElement,
  attemptNumber: number,
): HTMLElement {
  const attemptCard = within(container).getByRole("article", {
    name: `Inference attempt ${attemptNumber}`,
  });
  const toggle = within(attemptCard).getByRole("button", {
    name: `Expand attempt ${attemptNumber}`,
  });

  expect(toggle.getAttribute("aria-expanded")).toBe("false");
  fireEvent.click(toggle);
  expect(toggle.getAttribute("aria-expanded")).toBe("true");

  return attemptCard;
}

function expandAttemptBody(
  attemptCard: HTMLElement,
  bodyLabel: "Request body" | "Response body",
): HTMLElement {
  const toggle = within(attemptCard).getByRole("button", {
    name: `Expand ${bodyLabel.toLowerCase()}`,
  });

  expect(toggle.getAttribute("aria-expanded")).toBe("false");
  fireEvent.click(toggle);
  expect(toggle.getAttribute("aria-expanded")).toBe("true");

  return within(attemptCard).getByRole("region", { name: bodyLabel });
}

function buildSelectedTrace(workItem: DashboardWorkItemRef): DashboardTrace {
  return {
    dispatches: [],
    relations: [
      {
        source_work_id: "work-parent-story",
        source_work_name: "Parent Story",
        target_work_id: workItem.work_id,
        type: "PARENT",
      },
      {
        required_state: "ready",
        source_work_id: workItem.work_id,
        target_work_id: "work-dependency-story",
        target_work_name: "Dependency Story",
        type: "DEPENDS_ON",
      },
      {
        required_state: "approved",
        source_work_id: "work-blocked-story",
        source_work_name: "Blocked Story",
        target_work_id: workItem.work_id,
        type: "DEPENDS_ON",
      },
      {
        source_work_id: workItem.work_id,
        target_work_id: "work-child-story",
        target_work_name: "Child Story",
        type: "PARENT",
      },
    ],
    trace_id: workItem.trace_id ?? "trace-active-story",
    transition_ids: [],
    work_ids: [workItem.work_id],
    workstation_sequence: [],
  };
}

describe("WorkItemDetailCard provider-session selection", () => {
  it("keeps provider-session selection controls in dispatch history without embedding duplicate session detail", async () => {
    const user = userEvent.setup();
    const { dispatchID, execution, selectedNode, workItem } =
      getSelectedWorkItemFixture();

    render(
      <WorkItemDetailCard
        dispatchAttempts={[]}
        executionDetails={selectWorkItemExecutionDetails({
          activeExecution: execution,
          dispatchID,
          selectedNode,
          workItem,
        })}
        onSelectProviderSession={vi.fn()}
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
            inference_attempts: [
              inferenceAttempt(dispatchID, {
                attempt: 1,
                outcome: "SUCCEEDED",
                provider_session: {
                  id: "sess-current-selection-only",
                  kind: "session_id",
                  provider: "codex",
                },
                response: "Use the dedicated widget for session detail.",
              }),
            ],
            outcome: "ACCEPTED",
            response_view: {
              outcome: "ACCEPTED",
              output_work_items: [workItem],
            },
            trace_ids: ["trace-active-story"],
            work_items: [workItem],
          }),
        ]}
      />,
    );

    const dispatchHistory = screen.getByRole("region", {
      name: "Workstation dispatches",
    });
    const dispatchCard = within(dispatchHistory).getAllByRole("article")[0];

    if (!(dispatchCard instanceof HTMLElement)) {
      throw new Error("expected dispatch history card with inference attempts");
    }

    const inferenceAttemptsSection = expandDispatchSection(
      dispatchCard,
      "Inference attempts",
    );
    expandInferenceAttempt(inferenceAttemptsSection, 1);
    await user.click(
      within(inferenceAttemptsSection).getByRole("button", {
        name: "Select provider session codex / session_id / sess-current-selection-only for dispatch dispatch-review-active",
      }),
    );

    expect(
      screen.queryByRole("heading", { name: "Selected session details" }),
    ).toBeNull();
  });
});

describe("WorkItemDetailCard summary", () => {
  it("renders loadable inference-attempt provider sessions as selectable controls", async () => {
    const user = userEvent.setup();
    const { dispatchID, execution, selectedNode, workItem } =
      getSelectedWorkItemFixture();
    const onSelectProviderSession = vi.fn();
    const selectedSession = {
      dispatchID,
      id: "sess-ready-request",
      kind: "session_id",
      provider: "codex",
    } as const;

    render(
      <WorkItemDetailCard
        dispatchAttempts={[]}
        executionDetails={selectWorkItemExecutionDetails({
          activeExecution: execution,
          dispatchID,
          selectedNode,
          workItem,
        })}
        onSelectProviderSession={onSelectProviderSession}
        selectedNode={selectedNode}
        selectedProviderSessionKey={providerSessionSelectionKey(
          selectedSession,
        )}
        selection={{
          dispatchId: dispatchID,
          execution,
          kind: "work-item",
          nodeId: selectedNode.node_id,
          workItem,
        }}
        workstationRequests={[
          workstationRequest(dispatchID, {
            inference_attempts: [
              inferenceAttempt(dispatchID, {
                attempt: 1,
                diagnostics: {
                  provider: {
                    model: "gpt-5.4",
                    provider: "codex",
                  },
                },
                inference_request_id: `${dispatchID}/inference-request/1`,
                outcome: "SUCCEEDED",
                prompt: "Review the active story and return a concise result.",
                provider_session: {
                  id: selectedSession.id,
                  kind: selectedSession.kind,
                  provider: selectedSession.provider,
                },
                response: "Ready for the next workstation.",
              }),
            ],
            outcome: "ACCEPTED",
            response_view: {
              outcome: "ACCEPTED",
              output_work_items: [workItem],
            },
            trace_ids: ["trace-active-story"],
            work_items: [workItem],
          }),
        ]}
      />,
    );

    const dispatchHistory = screen.getByRole("region", {
      name: "Workstation dispatches",
    });
    const dispatchCard = within(dispatchHistory).getAllByRole("article")[0];

    if (!(dispatchCard instanceof HTMLElement)) {
      throw new Error("expected dispatch history card with inference attempts");
    }

    const inferenceAttemptsSection = expandDispatchSection(
      dispatchCard,
      "Inference attempts",
    );
    const inferenceAttempts = within(inferenceAttemptsSection);
    expandInferenceAttempt(inferenceAttemptsSection, 1);
    const selectSessionButton = inferenceAttempts.getByRole("button", {
      name: "Select provider session codex / session_id / sess-ready-request for dispatch dispatch-review-active",
    });

    expect(selectSessionButton.getAttribute("aria-pressed")).toBe("true");
    expect(inferenceAttempts.getByText("Session selected")).toBeTruthy();

    selectSessionButton.focus();
    await user.keyboard("{Enter}");
    await user.keyboard(" ");

    expect(onSelectProviderSession).toHaveBeenNthCalledWith(1, selectedSession);
    expect(onSelectProviderSession).toHaveBeenNthCalledWith(2, selectedSession);
  });

  it("shows an explicit unavailable state for unsupported inference-attempt provider sessions", () => {
    const { dispatchID, execution, selectedNode, workItem } =
      getSelectedWorkItemFixture();

    render(
      <WorkItemDetailCard
        dispatchAttempts={[]}
        executionDetails={selectWorkItemExecutionDetails({
          activeExecution: execution,
          dispatchID,
          selectedNode,
          workItem,
        })}
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
            inference_attempts: [
              inferenceAttempt(dispatchID, {
                attempt: 1,
                inference_request_id: `${dispatchID}/inference-request/1`,
                outcome: "FAILED",
                prompt: "Review the active story and return a concise result.",
                provider_session: {
                  id: "sess-unsupported",
                  kind: "path",
                  provider: "codex",
                },
              }),
            ],
            outcome: "FAILED",
            response_view: {
              outcome: "FAILED",
              output_work_items: [workItem],
            },
            trace_ids: ["trace-active-story"],
            work_items: [workItem],
          }),
        ]}
      />,
    );

    const dispatchCard = within(
      screen.getByRole("region", { name: "Workstation dispatches" }),
    ).getAllByRole("article")[0];

    if (!(dispatchCard instanceof HTMLElement)) {
      throw new Error("expected dispatch history card with inference attempts");
    }

    const inferenceAttemptsSection = expandDispatchSection(
      dispatchCard,
      "Inference attempts",
    );
    const inferenceAttempts = within(inferenceAttemptsSection);

    expect(
      inferenceAttempts.queryByRole("button", {
        name: "Select provider session codex / path / sess-unsupported for dispatch dispatch-review-active",
      }),
    ).toBeNull();
    expandInferenceAttempt(inferenceAttemptsSection, 1);
    expect(
      inferenceAttempts.getByText("Session details unavailable"),
    ).toBeTruthy();
    expect(
      inferenceAttempts.getByText("codex / path / sess-unsupported"),
    ).toBeTruthy();
  });

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

    expect(
      screen.getByRole("heading", { name: "Current selection" }),
    ).toBeTruthy();
    expect(screen.getByText(workItem.work_id)).toBeTruthy();
    const currentSelection = screen.getByRole("article", {
      name: "Current selection",
    });
    const dispatchHistory = within(
      screen.getByRole("region", { name: "Workstation dispatches" }),
    );
    const requestDetails = within(
      screen.getByRole("region", { name: "Request details" }),
    );
    const traceDetails = within(
      screen.getByRole("region", { name: "Trace details" }),
    );
    const inferenceAttempts = within(
      expandDispatchSection(document.body, "Inference attempts"),
    );
    expect(
      within(currentSelection).queryByRole("heading", {
        name: "Execution details",
      }),
    ).toBeNull();
    expect(
      within(
        getDetailRow(currentSelection, "Workstation dispatches"),
      ).getByText("1"),
    ).toBeTruthy();
    expect(screen.getAllByText(dispatchID).length).toBeGreaterThan(0);
    expect(screen.getAllByText("Review").length).toBeGreaterThan(0);
    expect(screen.getByText("Current dispatch")).toBeTruthy();
    expect(dispatchHistory.getByText("trace-active-story")).toBeTruthy();
    expect(
      requestDetails.queryByText(
        "Inference request details are shown under Inference attempts.",
      ),
    ).toBeNull();
    expect(
      inferenceAttempts.getByText(
        "No inference attempt details have been recorded for this dispatch yet.",
      ),
    ).toBeTruthy();
    expect(
      traceDetails.getByRole("link", { name: "trace-active-story" }),
    ).toBeTruthy();
    expect(
      screen.queryByText("Never expose this raw system prompt."),
    ).toBeNull();
    expect(
      screen.getAllByText("Workstation dispatches").length,
    ).toBeGreaterThan(0);
    expect(
      screen.getByRole("heading", { name: "Workstation dispatches" }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("heading", { name: "Work session runs list" }),
    ).toBeNull();
  });

  it("marks only the selected work item's active dispatch inside dispatch history", () => {
    const { dispatchID, execution, selectedNode, workItem } =
      getSelectedWorkItemFixture();
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

    const dispatchHistory = screen.getByRole("region", {
      name: "Workstation dispatches",
    });
    const activeCard = within(dispatchHistory)
      .getAllByText(dispatchID)[0]
      .closest("article");
    const historicalCard = within(dispatchHistory)
      .getByText(historicalDispatchID)
      .closest("article");

    if (
      !(activeCard instanceof HTMLElement) ||
      !(historicalCard instanceof HTMLElement)
    ) {
      throw new Error("expected active and historical dispatch cards");
    }

    expect(within(activeCard).getByText("Current dispatch")).toBeTruthy();
    expect(
      within(activeCard).getByText("Current dispatch").className,
    ).toContain("text-af-text");
    expect(activeCard.className).toContain("border-af-accent-border");
    expect(within(historicalCard).queryByText("Current dispatch")).toBeNull();
    expect(historicalCard.className).not.toContain("border-af-accent-border");
  });

  it("renders unavailable execution details with clear operator copy", () => {
    const { dispatchID, execution, selectedNode, workItem } =
      getSelectedWorkItemFixture();
    const executionDetails: SelectedWorkItemExecutionDetails = {
      dispatchID,
      elapsedStartTimestamp: execution.started_at,
      inferenceAttempts: [],
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

    const currentSelection = screen.getByRole("article", {
      name: "Current selection",
    });
    expect(
      within(currentSelection).queryByRole("heading", {
        name: "Execution details",
      }),
    ).toBeNull();
    expect(within(currentSelection).queryByText("Model")).toBeNull();
    expect(
      within(currentSelection).getByText(
        "No workstation dispatch has been recorded yet for this work item.",
      ),
    ).toBeTruthy();
    expect(screen.queryByRole("link", { name: "Open trace" })).toBeNull();
  });

  it("does not render a separate inference attempts section for selected work items", () => {
    const { dispatchID, execution, selectedNode, workItem } =
      getSelectedWorkItemFixture();

    render(
      <WorkItemDetailCard
        dispatchAttempts={[]}
        executionDetails={selectWorkItemExecutionDetails({
          activeExecution: execution,
          dispatchID,
          inferenceAttemptsByDispatchID: {
            [dispatchID]: {
              [`${dispatchID}/inference-request/1`]: inferenceAttempt(
                dispatchID,
                {
                  inference_request_id: `${dispatchID}/inference-request/1`,
                  outcome: "FAILED",
                },
              ),
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

    const currentSelection = screen.getByRole("article", {
      name: "Current selection",
    });
    expect(
      within(currentSelection).queryByRole("heading", {
        name: "Inference attempts",
      }),
    ).toBeNull();
    expect(
      within(currentSelection).queryByRole("region", {
        name: "Inference attempts",
      }),
    ).toBeNull();
    expect(
      within(currentSelection).getByRole("heading", {
        name: "Workstation dispatches",
      }),
    ).toBeTruthy();
  });

  it("renders a pending dispatch without provider-session metadata as a workstation dispatch", () => {
    const { dispatchID, execution, selectedNode, workItem } =
      getSelectedWorkItemFixture();
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

    const currentSelection = screen.getByRole("article", {
      name: "Current selection",
    });
    expect(
      within(
        getDetailRow(currentSelection, "Workstation dispatches"),
      ).getByText("1"),
    ).toBeTruthy();
    expect(
      within(currentSelection).getAllByText(dispatchID).length,
    ).toBeGreaterThan(0);
    expect(
      within(currentSelection).queryByText(selectedNode.transition_id),
    ).toBeNull();
    expect(
      within(currentSelection).getByText(selectedNode.workstation_name, {
        selector: "strong",
      }),
    ).toBeTruthy();
    expect(
      within(currentSelection).queryByText(
        "No workstation dispatch has been recorded yet for this work item.",
      ),
    ).toBeNull();
    expect(
      within(currentSelection).getByText("Session log unavailable"),
    ).toBeTruthy();
  });

  it("renders work relationships as a graph-shaped surface and keeps related work selectable", () => {
    const { dispatchID, execution, selectedNode, workItem } =
      getSelectedWorkItemFixture();
    const onSelectWorkID = vi.fn();

    render(
      <WorkItemDetailCard
        dispatchAttempts={[]}
        executionDetails={selectWorkItemExecutionDetails({
          activeExecution: execution,
          dispatchID,
          selectedNode,
          workItem,
        })}
        now={DETAIL_CARD_NOW}
        onSelectWorkID={onSelectWorkID}
        selectedNode={selectedNode}
        selectedTrace={buildSelectedTrace(workItem)}
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

    const relationshipGraph = screen.getByRole("region", {
      name: "Work relationships",
    });

    expect(within(relationshipGraph).getByText("Selected work")).toBeTruthy();
    expect(
      within(relationshipGraph).getByText("Active Story").className,
    ).toContain("text-af-text");
    expect(
      within(relationshipGraph).getByRole("region", {
        name: "Parent relationships",
      }),
    ).toBeTruthy();
    expect(
      within(relationshipGraph).getByRole("region", {
        name: "Depends on relationships",
      }),
    ).toBeTruthy();
    expect(
      within(relationshipGraph).getByRole("region", {
        name: "Required by relationships",
      }),
    ).toBeTruthy();
    expect(
      within(relationshipGraph).getByRole("region", {
        name: "Child relationships",
      }),
    ).toBeTruthy();
    expect(
      within(
        within(relationshipGraph).getByRole("region", {
          name: "Parent relationships",
        }),
      ).getByText("Parent Story"),
    ).toBeTruthy();
    expect(
      within(relationshipGraph).getByText("Depends on (ready)"),
    ).toBeTruthy();
    expect(
      within(relationshipGraph).getByText("Required by (approved)"),
    ).toBeTruthy();
    expect(
      within(
        within(relationshipGraph).getByRole("region", {
          name: "Child relationships",
        }),
      ).getByText("Child Story"),
    ).toBeTruthy();

    fireEvent.click(
      within(relationshipGraph).getByRole("button", {
        name: "Select related work item Dependency Story",
      }),
    );

    expect(onSelectWorkID).toHaveBeenCalledWith("work-dependency-story");
  });

  it("renders an explicit empty state when no work relationships are available", () => {
    const { dispatchID, execution, selectedNode, workItem } =
      getSelectedWorkItemFixture();

    render(
      <WorkItemDetailCard
        dispatchAttempts={[]}
        executionDetails={selectWorkItemExecutionDetails({
          activeExecution: execution,
          dispatchID,
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

    const relationshipSection = screen.getByRole("region", {
      name: "Work relationships",
    });

    expect(
      within(relationshipSection).getByText(
        "No parent, child, or dependency relationships are available for this work item.",
      ),
    ).toBeTruthy();
    expect(within(relationshipSection).queryByText("Selected work")).toBeNull();
  });

  it("omits the model row while preserving other execution details for historical selections", () => {
    const { dispatchID, execution, selectedNode, workItem } =
      getSelectedWorkItemFixture();
    const executionDetails: SelectedWorkItemExecutionDetails = {
      dispatchID,
      elapsedStartTimestamp: execution.started_at,
      inferenceAttempts: [],
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

    const currentSelection = screen.getByRole("article", {
      name: "Current selection",
    });
    expect(
      within(currentSelection).queryByRole("heading", {
        name: "Execution details",
      }),
    ).toBeNull();
    expect(within(currentSelection).queryByText("Model")).toBeNull();
    expect(
      within(currentSelection).getByText(
        "No workstation dispatch has been recorded yet for this work item.",
      ),
    ).toBeTruthy();
  });

  it("renders a unified pending dispatch-history row with attempt-owned request copy and no-response-yet copy", () => {
    const { dispatchID, execution, selectedNode, workItem } =
      getSelectedWorkItemFixture();

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
            prompt:
              "Review the active story while the provider response is still pending.",
            request_metadata: {
              prompt_source: "factory-renderer",
            },
            trace_ids: ["trace-active-story"],
            work_items: [workItem],
          }),
        ]}
      />,
    );

    const currentSelection = screen.getByRole("article", {
      name: "Current selection",
    });
    const dispatchHistory = within(
      screen.getByRole("region", { name: "Workstation dispatches" }),
    );
    const traceDetails = within(
      screen.getByRole("region", { name: "Trace details" }),
    );
    const inferenceAttempts = within(
      expandDispatchSection(document.body, "Inference attempts"),
    );
    expect(
      within(currentSelection).getByRole("heading", {
        name: "Workstation dispatches",
      }),
    ).toBeTruthy();
    expect(
      within(
        screen.getByRole("region", { name: "Request details" }),
      ).queryByText(
        "Inference request details are shown under Inference attempts.",
      ),
    ).toBeNull();
    expect(
      inferenceAttempts.getByText(
        "No inference attempt details have been recorded for this dispatch yet.",
      ),
    ).toBeTruthy();
    expect(
      within(currentSelection).queryByText(
        "Review the active story while the provider response is still pending.",
      ),
    ).toBeNull();
    expect(
      dispatchHistory.getByRole("button", {
        name: "Select work item Active Story",
      }),
    ).toBeTruthy();
    expect(
      traceDetails.getByRole("link", { name: "trace-active-story" }),
    ).toBeTruthy();
  });

  it("routes trace-link clicks through the selected dispatch response details", () => {
    const { dispatchID, execution, selectedNode, workItem } =
      getSelectedWorkItemFixture();
    const onSelectTraceID = vi.fn();

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
        onSelectTraceID={onSelectTraceID}
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
            prompt: "Review the active story and trace the result.",
            request_metadata: {
              prompt_source: "factory-renderer",
            },
            trace_ids: ["trace-active-story"],
            work_items: [workItem],
          }),
        ]}
      />,
    );

    const traceLink = within(
      screen.getByRole("region", { name: "Trace details" }),
    ).getByRole("link", { name: "trace-active-story" });

    expect(traceLink.getAttribute("href")).toBe("#trace");

    traceLink.addEventListener(
      "click",
      (event) => {
        event.preventDefault();
      },
      { once: true },
    );

    fireEvent.click(traceLink);

    expect(onSelectTraceID).toHaveBeenCalledWith("trace-active-story");
  });

  it("keeps markdown-authored inference prompts out of dispatch-level request details before attempts exist", () => {
    const { dispatchID, execution, selectedNode, workItem } =
      getSelectedWorkItemFixture();

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

    const dispatchHistory = screen.getByRole("region", {
      name: "Workstation dispatches",
    });
    const dispatchCard = within(dispatchHistory)
      .getAllByText(dispatchID)[0]
      .closest("article");

    if (!(dispatchCard instanceof HTMLElement)) {
      throw new Error("expected markdown dispatch history card");
    }

    const requestDetails = within(
      within(dispatchCard).getByRole("region", { name: "Request details" }),
    );

    expect(
      requestDetails.queryByText(
        "Inference request details are shown under Inference attempts.",
      ),
    ).toBeNull();
    expect(
      requestDetails.queryByRole("heading", {
        level: 2,
        name: "Review checklist",
      }),
    ).toBeNull();
    expect(requestDetails.queryByRole("list")).toBeNull();
    expect(requestDetails.queryByText("Check the latest diff")).toBeNull();
    expect(
      requestDetails.queryByText("bun test", { selector: "code" }),
    ).toBeNull();
    expect(within(dispatchCard).queryByText("## Review checklist")).toBeNull();
  });
});

describe("WorkItemDetailCard localization", () => {
  it("renders inference-attempt provider-session controls from the localized workstation-detail catalog", () => {
    const { dispatchID, execution, selectedNode, workItem } =
      getSelectedWorkItemFixture();

    render(
      <CurrentSelectionLocaleProvider locale="ja">
        <WorkItemDetailCard
          dispatchAttempts={[]}
          executionDetails={selectWorkItemExecutionDetails({
            activeExecution: execution,
            dispatchID,
            selectedNode,
            workItem,
          })}
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
              inference_attempts: [
                inferenceAttempt(dispatchID, {
                  attempt: 1,
                  inference_request_id: `${dispatchID}/inference-request/1`,
                  outcome: "FAILED",
                  prompt:
                    "Review the active story and return a concise result.",
                  provider_session: {
                    id: "sess-ja-unsupported",
                    kind: "path",
                    provider: "codex",
                  },
                }),
              ],
              outcome: "FAILED",
              response_view: {
                outcome: "FAILED",
                output_work_items: [workItem],
              },
              trace_ids: ["trace-active-story"],
              work_items: [workItem],
            }),
          ]}
        />
      </CurrentSelectionLocaleProvider>,
    );

    const dispatchCard = within(
      screen.getByRole("region", { name: "ワークステーションのディスパッチ" }),
    ).getAllByRole("article")[0];

    if (!(dispatchCard instanceof HTMLElement)) {
      throw new Error(
        "expected localized dispatch history card with inference attempts",
      );
    }

    const inferenceAttempts = within(
      expandDispatchSection(dispatchCard, "推論試行", "展開"),
    );
    expandInferenceAttempt(dispatchCard, 1);

    expect(
      inferenceAttempts.queryByRole("button", {
        name: `ディスパッチ ${dispatchID} の provider session codex / path / sess-ja-unsupported を選択`,
      }),
    ).toBeNull();
    expect(
      inferenceAttempts.getByText("セッション詳細は利用できません"),
    ).toBeTruthy();
  });
});

describe("WorkItemDetailCard dispatch diagnostics", () => {
  it("starts attempt sections collapsed and keeps expansion scoped to each dispatch card", () => {
    const { dispatchID, execution, selectedNode, workItem } =
      getSelectedWorkItemFixture();
    const historicalDispatchID = "dispatch-review-historical";

    render(
      <WorkItemDetailCard
        dispatchAttempts={[]}
        executionDetails={selectWorkItemExecutionDetails({
          activeExecution: execution,
          dispatchID,
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
        workstationRequests={[
          workstationRequest(historicalDispatchID, {
            inference_attempts: [
              inferenceAttempt(historicalDispatchID, {
                attempt: 1,
                inference_request_id: `${historicalDispatchID}/inference-request/1`,
                outcome: "FAILED",
                prompt: "Historical attempt prompt.",
              }),
            ],
            prompt: "Historical review request.",
            request_id: "request-review-historical",
            trace_ids: ["trace-historical-story"],
            work_items: [workItem],
          }),
          workstationRequest(dispatchID, {
            inference_attempts: [
              inferenceAttempt(dispatchID, {
                attempt: 1,
                inference_request_id: `${dispatchID}/inference-request/1`,
                outcome: "SUCCEEDED",
                prompt: "Active attempt prompt.",
              }),
            ],
            prompt: "Active review request.",
            request_id: "request-review-active",
            trace_ids: ["trace-active-story"],
            work_items: [workItem],
          }),
        ]}
      />,
    );

    const dispatchHistory = screen.getByRole("region", {
      name: "Workstation dispatches",
    });
    const activeCard = within(dispatchHistory)
      .getAllByText(dispatchID)[0]
      .closest("article");
    const historicalCard = within(dispatchHistory)
      .getByText(historicalDispatchID)
      .closest("article");

    if (
      !(activeCard instanceof HTMLElement) ||
      !(historicalCard instanceof HTMLElement)
    ) {
      throw new Error("expected active and historical dispatch cards");
    }

    const activeSection = within(activeCard).getByRole("region", {
      name: "Inference attempts",
    });
    const historicalSection = within(historicalCard).getByRole("region", {
      name: "Inference attempts",
    });
    const activeToggle = within(activeSection).getByRole("button", {
      name: "Expand",
    });
    const historicalToggle = within(historicalSection).getByRole("button", {
      name: "Expand",
    });

    expect(activeToggle.getAttribute("aria-expanded")).toBe("false");
    expect(historicalToggle.getAttribute("aria-expanded")).toBe("false");
    expect(
      within(activeSection).queryByText("Active attempt prompt."),
    ).toBeNull();
    expect(
      within(historicalSection).queryByText("Historical attempt prompt."),
    ).toBeNull();

    fireEvent.click(activeToggle);

    expect(activeToggle.getAttribute("aria-expanded")).toBe("true");
    expect(historicalToggle.getAttribute("aria-expanded")).toBe("false");
    const expandedActiveAttempt = expandInferenceAttempt(activeSection, 1);
    const activeRequestBody = expandAttemptBody(expandedActiveAttempt, "Request body");
    expect(
      within(activeRequestBody).getByText("Active attempt prompt."),
    ).toBeTruthy();
    expect(
      within(historicalSection).queryByText("Historical attempt prompt."),
    ).toBeNull();
  });

  it("renders nested inference attempts in order and preserves dispatch drilldowns", () => {
    const { dispatchID, execution, selectedNode, workItem } =
      getSelectedWorkItemFixture();
    const onSelectTraceID = vi.fn();
    const onSelectWorkID = vi.fn();

    render(
      <WorkItemDetailCard
        dispatchAttempts={[]}
        executionDetails={selectWorkItemExecutionDetails({
          activeExecution: execution,
          dispatchID,
          selectedNode,
          workItem,
        })}
        now={DETAIL_CARD_NOW}
        onSelectTraceID={onSelectTraceID}
        onSelectWorkID={onSelectWorkID}
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
            inference_attempts: [
              inferenceAttempt(dispatchID, {
                attempt: 2,
                diagnostics: {
                  provider: {
                    model: "gpt-5.4",
                    provider: "codex",
                  },
                },
                duration_millis: 740,
                inference_request_id: `${dispatchID}/inference-request/2`,
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
              inferenceAttempt(dispatchID, {
                attempt: 1,
                diagnostics: {
                  provider: {
                    model: "gpt-5.4-mini",
                    provider: "codex",
                  },
                },
                error_class: "provider_rate_limit",
                inference_request_id: `${dispatchID}/inference-request/1`,
                outcome: "FAILED",
                prompt: "Review the active story and return a concise result.",
                response_time: "2026-04-08T12:00:02Z",
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
            responded_request_count: 1,
            response: "Ready for the next workstation.",
            response_view: {
              duration_millis: 63_000,
              outcome: "ACCEPTED",
              output_work_items: [workItem],
              provider_session: {
                id: "sess-ready-request",
                kind: "session_id",
                provider: "codex",
              },
              response_text: "Ready for the next workstation.",
            },
            total_duration_millis: 63_000,
            trace_ids: ["trace-active-story"],
            working_directory: "C:\\work\\portos",
            worktree: "C:\\work\\portos\\.worktrees\\active-story",
          }),
        ]}
      />,
    );

    const dispatchHistory = screen.getByRole("region", {
      name: "Workstation dispatches",
    });
    const dispatchCard = within(dispatchHistory)
      .getAllByText(dispatchID)[0]
      .closest("article");

    if (!(dispatchCard instanceof HTMLElement)) {
      throw new Error(
        "expected dispatch history card with nested inference attempts",
      );
    }

    const inferenceAttemptsSection = expandDispatchSection(
      dispatchCard,
      "Inference attempts",
    );
    const inferenceAttempts = within(inferenceAttemptsSection);
    const attemptCards = inferenceAttempts.getAllByRole("article");
    expect(
      inferenceAttempts.getByRole("article", {
        name: "Inference attempt 1",
      }),
    ).toBe(attemptCards[0]);
    expect(
      inferenceAttempts.getByRole("article", {
        name: "Inference attempt 2",
      }),
    ).toBe(attemptCards[1]);

    expect(attemptCards).toHaveLength(2);
    expect(within(attemptCards[0]).getByText("Attempt 1")).toBeTruthy();
    expect(within(attemptCards[1]).getByText("Attempt 2")).toBeTruthy();
    const firstAttemptCard = expandInferenceAttempt(inferenceAttemptsSection, 1);
    const secondAttemptCard = expandInferenceAttempt(inferenceAttemptsSection, 2);
    const secondRequestBody = expandAttemptBody(secondAttemptCard, "Request body");
    const secondResponseBody = expandAttemptBody(secondAttemptCard, "Response body");
    expect(
      within(firstAttemptCard).getByText(`${dispatchID}/inference-request/1`),
    ).toBeTruthy();
    expect(within(firstAttemptCard).getByText("gpt-5.4-mini")).toBeTruthy();
    expect(within(secondAttemptCard).getByText("codex")).toBeTruthy();
    expect(
      within(secondAttemptCard).getByText(
        "codex / session_id / sess-ready-request",
      ),
    ).toBeTruthy();
    expect(within(secondAttemptCard).getByText("740ms")).toBeTruthy();
    expect(
      within(secondRequestBody).getByText(
        "Retry the review with the latest context.",
      ),
    ).toBeTruthy();
    expect(
      within(secondResponseBody).getByText("Ready for the next workstation."),
    ).toBeTruthy();

    const traceLink = within(dispatchCard).getByRole("link", {
      name: "trace-active-story",
    });
    traceLink.addEventListener(
      "click",
      (event) => {
        event.preventDefault();
      },
      { once: true },
    );
    fireEvent.click(traceLink);
    fireEvent.click(
      within(dispatchCard).getAllByRole("button", {
        name: "Select work item Active Story",
      })[0],
    );

    expect(onSelectTraceID).toHaveBeenCalledWith("trace-active-story");
    expect(onSelectWorkID).toHaveBeenCalledWith(workItem.work_id);
  });

  it("renders completed failed dispatch-history details from the same row", () => {
    const { dispatchID, execution, selectedNode, workItem } =
      getSelectedWorkItemFixture();

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
            failure_message:
              "Provider rate limit exceeded while reviewing the story.",
            failure_reason: "provider_rate_limit",
            outcome: "FAILED",
            response_view: {
              error_class: "provider_rate_limit",
              failure_message:
                "Provider rate limit exceeded while reviewing the story.",
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

    const failureDetails = within(
      screen.getByRole("region", { name: "Failure details" }),
    );
    const dispatchHistory = within(
      screen.getByRole("region", { name: "Workstation dispatches" }),
    );
    expect(
      failureDetails.getAllByText("provider_rate_limit").length,
    ).toBeGreaterThan(0);
    expect(
      failureDetails.getByText(
        "Provider rate limit exceeded while reviewing the story.",
      ),
    ).toBeTruthy();
    expect(
      screen.getAllByRole("region", { name: "Failure details" }),
    ).toHaveLength(1);
    expect(
      within(
        expandDispatchSection(document.body, "Inference attempts"),
      ).getByText(
        "No inference attempt details were recorded before this dispatch ended.",
      ),
    ).toBeTruthy();
    expect(
      within(
        screen.getByRole("article", { name: "Current selection" }),
      ).queryByText("Provider rate limit exceeded while reviewing the story."),
    ).toBeTruthy();
    expect(
      dispatchHistory.getAllByRole("button", {
        name: "Select work item Active Story",
      }).length,
    ).toBeGreaterThan(0);
  });

  it("renders pending script dispatch-history details for the selected work item", () => {
    const { dispatchID, execution, selectedNode, workItem } =
      getSelectedWorkItemFixture();

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
          dashboardWorkstationRequestFixtures.scriptPending,
        ]}
      />,
    );

    const dispatchHistory = screen.getByRole("region", {
      name: "Workstation dispatches",
    });
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
    const scriptAttempts = within(
      expandDispatchSection(dispatchCard, "Script attempts"),
    );
    expect(
      scriptAttempts.getByText(
        dashboardWorkstationRequestFixtures.scriptPending.script_request
          ?.command ?? "",
      ),
    ).toBeTruthy();
    expect(
      scriptAttempts.getAllByText(
        dashboardWorkstationRequestFixtures.scriptPending.script_request
          ?.script_request_id ?? "",
      ).length,
    ).toBeGreaterThan(0);
    expect(scriptAttempts.getByText("--work")).toBeTruthy();
    expect(
      within(
        within(dispatchCard).getByRole("region", { name: "Request details" }),
      ).queryByText("Resolved args"),
    ).toBeNull();
    expect(
      within(dispatchCard).queryByText(
        dashboardWorkstationRequestFixtures.scriptPending.script_request
          ?.command ?? "",
      ),
    ).toBeTruthy();
    expect(
      within(
        within(dispatchCard).getByRole("region", { name: "Response details" }),
      ).queryByText("No script response yet for this dispatch."),
    ).toBeNull();
    expect(scriptAttempts.getByText("Request attempt 1")).toBeTruthy();
    expect(scriptAttempts.getByText("PENDING")).toBeTruthy();
    expect(
      scriptAttempts.getByText(
        "No script response attempt has been recorded yet.",
      ),
    ).toBeTruthy();
    expect(
      within(dispatchCard).queryByText("No response yet for this dispatch."),
    ).toBeNull();
  });

  it("uses the selected work title as the dispatch heading while keeping the dispatch id secondary", () => {
    const { dispatchID, execution, selectedNode, workItem } =
      getSelectedWorkItemFixture();

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
        workstationRequests={[dashboardWorkstationRequestFixtures.ready]}
      />,
    );

    const dispatchHistory = screen.getByRole("region", {
      name: "Workstation dispatches",
    });
    const dispatchCard = within(dispatchHistory)
      .getByText("Active Story", { selector: "strong" })
      .closest("article");

    if (!(dispatchCard instanceof HTMLElement)) {
      throw new Error("expected ready dispatch history card");
    }

    expect(
      within(dispatchCard).getByText("Active Story", { selector: "strong" }),
    ).toBeTruthy();
    expect(
      within(dispatchCard).getByText(
        dashboardWorkstationRequestFixtures.ready.dispatch_id,
        { selector: "span" },
      ),
    ).toBeTruthy();
    expect(within(dispatchCard).getByText("Started at")).toBeTruthy();
    expect(
      within(getDetailRow(dispatchCard, "Started at")).getByText(
        formatLocalDateTime("2026-04-08T12:00:01Z", "Unavailable"),
      ),
    ).toBeTruthy();
    expectDispatchCardToHideTransitionId(
      dispatchCard,
      "Transition ID",
      dashboardWorkstationRequestFixtures.ready.transition_id,
    );
    expect(within(dispatchCard).queryByText("dispatchedCount")).toBeNull();
    expect(within(dispatchCard).queryByText("respondedCount")).toBeNull();
    expect(within(dispatchCard).queryByText("erroredCount")).toBeNull();
  });

  it("falls back to the dispatch id as the title when no associated work label is available", () => {
    const { dispatchID, execution, selectedNode, workItem } =
      getSelectedWorkItemFixture();

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
          {
            ...dashboardWorkstationRequestFixtures.noResponse,
            request_view: {
              ...dashboardWorkstationRequestFixtures.noResponse.request_view,
              input_work_items: [],
            },
            trace_ids: [],
            work_items: [],
          },
        ]}
      />,
    );

    const dispatchHistory = screen.getByRole("region", {
      name: "Workstation dispatches",
    });
    const dispatchCard = within(dispatchHistory)
      .getByText(dashboardWorkstationRequestFixtures.noResponse.dispatch_id, {
        selector: "strong",
      })
      .closest("article");

    if (!(dispatchCard instanceof HTMLElement)) {
      throw new Error("expected fallback-title dispatch history card");
    }

    expect(
      within(dispatchCard).getByText(
        dashboardWorkstationRequestFixtures.noResponse.dispatch_id,
        { selector: "strong" },
      ),
    ).toBeTruthy();
  });

  it("falls back to default dispatch-history copy for an unsupported locale", () => {
    const { dispatchID, execution, selectedNode, workItem } =
      getSelectedWorkItemFixture();

    render(
      <CurrentSelectionLocaleProvider locale="fr">
        <WorkItemDetailCard
          activeTraceID="trace-active-story"
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
            dashboardWorkstationRequestFixtures.scriptPending,
          ]}
        />
      </CurrentSelectionLocaleProvider>,
    );

    const dispatchHistory = screen.getByRole("region", {
      name: "Workstation dispatches",
    });
    const dispatchCard = within(dispatchHistory)
      .getByText(dashboardWorkstationRequestFixtures.scriptPending.dispatch_id)
      .closest("article");

    if (!(dispatchCard instanceof HTMLElement)) {
      throw new Error("expected fallback dispatch history card");
    }

    expect(
      within(dispatchCard).getByRole("region", {
        name: "Request details",
      }),
    ).toBeTruthy();
    const fallbackRequestDetails = within(dispatchCard).getByRole("region", {
      name: "Request details",
    });
    expect(
      within(dispatchCard).getByRole("region", {
        name: "Response details",
      }),
    ).toBeTruthy();
    expect(
      within(expandDispatchSection(dispatchCard, "Script attempts")).getByText(
        "No script response attempt has been recorded yet.",
      ),
    ).toBeTruthy();
    expect(
      within(dispatchCard).queryByText(
        "No script response yet for this dispatch.",
      ),
    ).toBeNull();
    expect(within(dispatchCard).getByText("Workstation")).toBeTruthy();
    expect(
      within(fallbackRequestDetails).queryByText("Resolved args"),
    ).toBeNull();
    const selectedWorkButton = within(dispatchCard).getByRole("button", {
      name: "Select work item Active Story",
    });
    expect(selectedWorkButton).toBeTruthy();
    expect(selectedWorkButton.textContent).toContain("Active Story");
    expect(selectedWorkButton.className).toContain("text-af-text");
    expect(within(dispatchCard).getByText("Trace IDs")).toBeTruthy();
    const selectedTraceLink = within(dispatchCard).getByRole("link", {
      name: "trace-active-story (selected)",
    });
    expect(selectedTraceLink).toBeTruthy();
    expect(selectedTraceLink.className).toContain("text-af-text");
  });

  it("renders selected-work script success details from the dispatch-history row", () => {
    const { dispatchID, execution, selectedNode, workItem } =
      getSelectedWorkItemFixture();

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
          dashboardWorkstationRequestFixtures.scriptSuccess,
        ]}
      />,
    );

    const dispatchHistory = screen.getByRole("region", {
      name: "Workstation dispatches",
    });
    const dispatchCard = within(dispatchHistory)
      .getByText(dashboardWorkstationRequestFixtures.scriptSuccess.dispatch_id)
      .closest("article");

    if (!(dispatchCard instanceof HTMLElement)) {
      throw new Error("expected script success dispatch history card");
    }

    expect(
      within(dispatchCard).getAllByText("SUCCEEDED").length,
    ).toBeGreaterThan(0);
    const scriptAttempts = within(
      expandDispatchSection(dispatchCard, "Script attempts"),
    );
    expect(scriptAttempts.getByText("Request attempt 1")).toBeTruthy();
    expect(scriptAttempts.getByText("Response attempt 1")).toBeTruthy();
    expect(
      scriptAttempts.getByText(
        dashboardWorkstationRequestFixtures.scriptSuccess.script_request
          ?.command ?? "",
      ),
    ).toBeTruthy();
    expect(
      scriptAttempts.getAllByText(
        dashboardWorkstationRequestFixtures.scriptSuccess.script_response
          ?.script_request_id ?? "",
      ).length,
    ).toBeGreaterThan(0);
    expect(scriptAttempts.getByText("222ms")).toBeTruthy();
    expect(scriptAttempts.getByText(/script success stdout/));
    expect(scriptAttempts.getAllByRole("article")).toHaveLength(2);
    expect(
      within(
        within(dispatchCard).getByRole("region", { name: "Request details" }),
      ).queryByText(
        dashboardWorkstationRequestFixtures.scriptSuccess.script_request
          ?.command ?? "",
      ),
    ).toBeNull();
    expect(
      within(
        within(dispatchCard).getByRole("region", { name: "Response details" }),
      ).queryByText(/script success stdout/),
    ).toBeNull();
    expect(within(dispatchCard).queryByText("Provider session")).toBeNull();
  });

  it("renders selected-work script failure details from the dispatch-history row", () => {
    const { dispatchID, execution, selectedNode, workItem } =
      getSelectedWorkItemFixture();

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

    const dispatchHistory = screen.getByRole("region", {
      name: "Workstation dispatches",
    });
    const dispatchCard = within(dispatchHistory)
      .getByText(dashboardWorkstationRequestFixtures.scriptFailed.dispatch_id)
      .closest("article");

    if (!(dispatchCard instanceof HTMLElement)) {
      throw new Error("expected script failure dispatch history card");
    }

    expect(
      within(dispatchCard).getAllByText("TIMED_OUT").length,
    ).toBeGreaterThan(0);
    const scriptAttempts = within(
      expandDispatchSection(dispatchCard, "Script attempts"),
    );
    expect(scriptAttempts.getByText("Request attempt 1")).toBeTruthy();
    expect(scriptAttempts.getByText("Response attempt 1")).toBeTruthy();
    expect(scriptAttempts.getByText("TIMEOUT")).toBeTruthy();
    expect(scriptAttempts.getByText(/script timed out/i)).toBeTruthy();
    expect(
      within(
        within(dispatchCard).getByRole("region", { name: "Response details" }),
      ).queryByText("TIMEOUT"),
    ).toBeNull();
    expect(
      within(dispatchCard).queryByText(
        "Response text is unavailable because this dispatch ended with an error.",
      ),
    ).toBeNull();
    expect(
      screen.getAllByRole("region", { name: "Failure details" }),
    ).toHaveLength(1);
  });

  it("keeps rejected dispatch request details and attempt diagnostics paired on the same history row", () => {
    const { dispatchID, execution, selectedNode, workItem } =
      getSelectedWorkItemFixture();

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

    const currentSelection = screen.getByRole("article", {
      name: "Current selection",
    });
    const dispatchHistory = screen.getByRole("region", {
      name: "Workstation dispatches",
    });
    const dispatchCard = within(dispatchHistory)
      .getAllByText(dashboardWorkstationRequestFixtures.rejected.dispatch_id)[0]
      .closest("article");

    if (!(dispatchCard instanceof HTMLElement)) {
      throw new Error("expected rejected dispatch history card");
    }

    expect(
      within(currentSelection).getByRole("heading", {
        name: "Workstation dispatches",
      }),
    ).toBeTruthy();
    expect(
      within(currentSelection).queryByRole("heading", {
        name: "Work session runs list",
      }),
    ).toBeNull();
    const inferenceAttempts = within(
      expandDispatchSection(dispatchCard, "Inference attempts"),
    );
    const expandedAttempt = expandInferenceAttempt(dispatchCard, 1);
    const requestBody = expandAttemptBody(expandedAttempt, "Request body");
    const responseBody = expandAttemptBody(expandedAttempt, "Response body");
    expect(
      within(requestBody).getByText(
        "Review the active story and explain what needs to change before approval.",
      ),
    ).toBeTruthy();
    expect(
      within(responseBody).getAllByText(
        "The active story needs revision before it can continue.",
      ).length,
    ).toBeGreaterThan(0);
    expect(
      within(
        within(dispatchCard).getByRole("region", { name: "Request details" }),
      ).queryByText(
        "Inference request details are shown under Inference attempts.",
      ),
    ).toBeNull();
    expect(
      inferenceAttempts.getByText(
        `codex / session_id / ${dashboardWorkstationRequestFixtures.rejected.inference_attempts?.[0]?.provider_session?.id}`,
      ),
    ).toBeTruthy();
    expect(
      within(dispatchCard).queryByText("No response yet for this dispatch."),
    ).toBeNull();
  });
});

describe("WorkItemDetailCard localized dispatch diagnostics", () => {
  it("renders localized dispatch-history card copy for a supported non-default locale", () => {
    const { dispatchID, execution, selectedNode, workItem } =
      getSelectedWorkItemFixture();

    render(
      <CurrentSelectionLocaleProvider locale="ja">
        <WorkItemDetailCard
          activeTraceID="trace-active-story"
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
            dashboardWorkstationRequestFixtures.scriptPending,
          ]}
        />
      </CurrentSelectionLocaleProvider>,
    );

    const dispatchHistory = screen.getByRole("region", {
      name: "ワークステーションのディスパッチ",
    });
    const dispatchCard = within(dispatchHistory)
      .getByText(dashboardWorkstationRequestFixtures.scriptPending.dispatch_id)
      .closest("article");

    if (!(dispatchCard instanceof HTMLElement)) {
      throw new Error("expected localized dispatch history card");
    }

    expect(
      within(dispatchCard).getByRole("region", {
        name: "リクエストの詳細",
      }),
    ).toBeTruthy();
    const localizedRequestDetails = within(dispatchCard).getByRole("region", {
      name: "リクエストの詳細",
    });
    expect(
      within(dispatchCard).getByRole("region", {
        name: "応答の詳細",
      }),
    ).toBeTruthy();
    expect(
      within(
        expandDispatchSection(dispatchCard, "スクリプト試行", "展開"),
      ).getByText("スクリプト応答の試行はまだ記録されていません。"),
    ).toBeTruthy();
    expect(
      within(dispatchCard).queryByText(
        "このディスパッチにはまだスクリプト応答がありません。",
      ),
    ).toBeNull();
    expect(within(dispatchCard).getByText("ワークステーション")).toBeTruthy();
    expectDispatchCardToHideTransitionId(
      dispatchCard,
      "遷移 ID",
      dashboardWorkstationRequestFixtures.scriptPending.transition_id,
    );
    expect(within(dispatchCard).getByText("開始時刻")).toBeTruthy();
    expect(within(dispatchCard).getAllByText("保留中").length).toBeGreaterThan(
      0,
    );
    expect(within(dispatchCard).queryByText("ディスパッチ数")).toBeNull();
    expect(within(dispatchCard).queryByText("応答数")).toBeNull();
    expect(within(dispatchCard).queryByText("エラー数")).toBeNull();
    expect(
      within(localizedRequestDetails).queryByText("解決済み引数"),
    ).toBeNull();
    expect(
      within(dispatchCard).getByRole("button", {
        name: "作業項目 Active Story を選択",
      }),
    ).toBeTruthy();
    expect(within(dispatchCard).getByText("トレース ID")).toBeTruthy();
    expect(
      within(dispatchCard).getByRole("link", {
        name: "trace-active-story（選択中）",
      }),
    ).toBeTruthy();
  });
});

describe("WorkItemDetailCard localized script attempts", () => {
  it("localizes expanded script-attempt labels for a supported non-default locale", () => {
    const { dispatchID, execution, selectedNode, workItem } =
      getSelectedWorkItemFixture();

    render(
      <CurrentSelectionLocaleProvider locale="ja">
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
            dashboardWorkstationRequestFixtures.scriptSuccess,
          ]}
        />
      </CurrentSelectionLocaleProvider>,
    );

    const dispatchHistory = screen.getByRole("region", {
      name: "ワークステーションのディスパッチ",
    });
    const dispatchCard = within(dispatchHistory)
      .getByText(dashboardWorkstationRequestFixtures.scriptSuccess.dispatch_id)
      .closest("article");

    if (!(dispatchCard instanceof HTMLElement)) {
      throw new Error(
        "expected localized script-success dispatch history card",
      );
    }

    const scriptAttempts = within(
      expandDispatchSection(dispatchCard, "スクリプト試行", "展開"),
    );

    expect(scriptAttempts.getByText("リクエスト試行 1")).toBeTruthy();
    expect(scriptAttempts.getByText("応答試行 1")).toBeTruthy();
    expect(
      scriptAttempts.getAllByText("スクリプトリクエスト ID").length,
    ).toBeGreaterThan(0);
    expect(
      scriptAttempts.getAllByText("スクリプト試行").length,
    ).toBeGreaterThan(0);
    expect(scriptAttempts.getByText("コマンド")).toBeTruthy();
    expect(scriptAttempts.getByText("解決済み引数")).toBeTruthy();
    expect(scriptAttempts.getByText("結果")).toBeTruthy();
    expect(scriptAttempts.getByText("所要時間")).toBeTruthy();
    expect(scriptAttempts.getByText("標準出力")).toBeTruthy();
    expect(scriptAttempts.getByText("標準エラー")).toBeTruthy();
    expect(
      scriptAttempts.getByText(
        "このスクリプト応答では stderr は記録されませんでした。",
      ),
    ).toBeTruthy();
    expect(scriptAttempts.queryByText("Script request ID")).toBeNull();
    expect(scriptAttempts.queryByText("Resolved args")).toBeNull();
    expect(scriptAttempts.queryByText("Stdout")).toBeNull();
    expect(scriptAttempts.queryByText("Stderr")).toBeNull();
    expect(
      scriptAttempts.queryByText(
        "No stderr was recorded for this script response.",
      ),
    ).toBeNull();
  });
});
