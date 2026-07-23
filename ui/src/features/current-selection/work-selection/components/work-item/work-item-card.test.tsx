import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { DashboardWorkItemRef } from "../../../../../api/dashboard/types";
import { dashboardWorkstationRequestFixtures } from "../../../../../components/dashboard/fixtures";
import { installDashboardBrowserTestShims } from "../../../../../components/dashboard/test-browser-shims";
import {
  formatDurationMillis,
  formatLocalDateTime,
} from "../../../../../components/ui/formatters";
import { providerSessionSelectionKey } from "../../../../provider-session-detail/lib/provider-session-ref";
import {
  DETAIL_CARD_NOW,
  getSelectedWorkItemFixture,
  inferenceAttempt,
  workstationRequest,
} from "../../../base/components/detail-card/detail-card-test-helpers";
import { CurrentSelectionLocaleProvider } from "../../../base/components/presentation/current-selection-locale";
import type { SelectedWorkRelationshipGraph } from "../../lib/selected-work-relationship-graph";
import type { SelectedWorkItemExecutionDetails } from "../../state/executionDetails";
import { selectWorkItemExecutionDetails } from "../../state/executionDetails";
import { WorkItemDetailCard } from "./work-item-card";

type QueryScope = HTMLElement | ReturnType<typeof within>;

function scopedQueries(scope: QueryScope): ReturnType<typeof within> {
  return scope instanceof HTMLElement ? within(scope) : scope;
}

function getDetailRow(container: HTMLElement, label: string): HTMLElement {
  const queries = within(container);
  const term =
    queries.queryByText(label, { selector: "dt" }) ?? queries.getByText(label);
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
  container: QueryScope,
  title: string,
  expandLabel = "Expand",
): HTMLElement {
  const queries = scopedQueries(container);
  const section = queries.getByRole("region", { name: title });
  const toggle = within(section).getByRole("button", { name: expandLabel });

  if (toggle.getAttribute("aria-expanded") === "false") {
    fireEvent.click(toggle);
    expect(toggle.getAttribute("aria-expanded")).toBe("true");
  } else {
    expect(toggle.getAttribute("aria-expanded")).toBe("true");
  }

  return section;
}

function getDispatchHistoryCard(
  container: QueryScope,
  dispatchID: string,
): HTMLElement {
  const card = scopedQueries(container).getByRole("article", {
    name: new RegExp(dispatchID),
  });

  if (!(card instanceof HTMLElement)) {
    throw new Error(`expected dispatch history card for ${dispatchID}`);
  }

  return card;
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

function getTraceGraphNodeButton(
  container: HTMLElement,
  label: string,
): HTMLButtonElement {
  const button = container.querySelector(`button[aria-label="${label}"]`);

  if (!(button instanceof HTMLButtonElement)) {
    throw new Error(`expected trace graph node button for ${label}`);
  }

  return button;
}

function getTraceGraphNodeShell(
  container: HTMLElement,
  label: string,
): HTMLElement {
  const nodeText = within(container).getByText(label);
  const shell = nodeText.closest("article");

  if (!(shell instanceof HTMLElement)) {
    throw new Error(`expected trace graph node shell for ${label}`);
  }

  return shell;
}

function buildRelationshipGraph(
  workItem: DashboardWorkItemRef,
): SelectedWorkRelationshipGraph {
  return {
    edges: [
      {
        relationship: "PARENT",
        sourceWorkID: workItem.work_id,
        targetWorkID: "work-parent-story",
      },
      {
        relationship: "DEPENDS_ON",
        requiredState: "ready",
        sourceWorkID: workItem.work_id,
        targetWorkID: "work-dependency-story",
      },
      {
        relationship: "REQUIRED_BY",
        requiredState: "approved",
        sourceWorkID: workItem.work_id,
        targetWorkID: "work-blocked-story",
      },
      {
        relationship: "CHILD",
        sourceWorkID: workItem.work_id,
        targetWorkID: "work-child-story",
      },
    ],
    relatedWork: [
      {
        label: "Blocked Story",
        state: "queued",
        traceID: "trace-blocked-story",
        workID: "work-blocked-story",
        workTypeID: "story",
      },
      {
        label: "Child Story",
        state: "running",
        traceID: "trace-child-story",
        workID: "work-child-story",
        workTypeID: "task",
      },
      {
        label: "Dependency Story",
        state: "ready",
        traceID: "trace-dependency-story",
        workID: "work-dependency-story",
        workTypeID: "dependency",
      },
      {
        label: "Parent Story",
        state: "done",
        traceID: "trace-parent-story",
        workID: "work-parent-story",
        workTypeID: "epic",
      },
    ],
    selectedWork: {
      label: "Active Story",
      state: "in_progress",
      traceID: workItem.trace_id,
      workID: workItem.work_id,
      workTypeID: workItem.work_type_id,
    },
    status: "ready",
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
        name: "Select provider session codex / Session ID / sess-current-selection-only for dispatch dispatch-review-active",
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
      name: "Select provider session codex / Session ID / sess-ready-request for dispatch dispatch-review-active",
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
        name: "Select provider session codex / Path / sess-unsupported for dispatch dispatch-review-active",
      }),
    ).toBeNull();
    expandInferenceAttempt(inferenceAttemptsSection, 1);
    expect(
      inferenceAttempts.getByText("Session details unavailable"),
    ).toBeTruthy();
    expect(
      inferenceAttempts.getByText("codex / Path / sess-unsupported"),
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
    const primaryTitle = currentSelection.querySelector(".type-display-large");
    expect(primaryTitle?.textContent).toBe("Active Story");
    const topLevelSummary = within(currentSelection)
      .getAllByRole("region", { name: "Summary" })
      .find(
        (section) =>
          section.getAttribute("aria-labelledby") ===
          "current-selection-work-item-summary-heading",
      );
    if (!(topLevelSummary instanceof HTMLElement)) {
      throw new Error("expected top-level Summary section");
    }
    expect(
      within(topLevelSummary)
        .getByRole("button", { name: "Collapse" })
        .getAttribute("aria-expanded"),
    ).toBe("true");

    for (const sectionName of [
      "Work contents",
      "Work relationships",
      "Workstation dispatches",
    ]) {
      const section = within(currentSelection)
        .getByRole("heading", { name: sectionName })
        .closest("section");

      if (!(section instanceof HTMLElement)) {
        throw new Error(`expected ${sectionName} section`);
      }

      expect(
        within(section)
          .getByRole("button", { name: "Collapse" })
          .getAttribute("aria-expanded"),
      ).toBe("true");
    }
    const dispatchHistory = within(
      screen.getByRole("region", { name: "Workstation dispatches" }),
    );
    const requestDetailsRegion = expandDispatchSection(
      dispatchHistory,
      "Summary",
    );
    const requestDetails = within(requestDetailsRegion);
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
      requestDetails.getByRole("button", { name: "trace-active-story" }),
    ).toBeTruthy();
    expect(
      within(getDetailRow(requestDetailsRegion, "Output work")).getByRole(
        "button",
        {
          name: "Select work item Active Story",
        },
      ),
    ).toBeTruthy();
    expect(
      within(getDetailRow(requestDetailsRegion, "Trace IDs")).getByRole(
        "button",
        {
          name: "trace-active-story",
        },
      ),
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
    const activeCard = getDispatchHistoryCard(dispatchHistory, dispatchID);
    const historicalCard = getDispatchHistoryCard(
      dispatchHistory,
      historicalDispatchID,
    );

    if (
      !(activeCard instanceof HTMLElement) ||
      !(historicalCard instanceof HTMLElement)
    ) {
      throw new Error("expected active and historical dispatch cards");
    }

    expect(within(activeCard).getByText("Current dispatch")).toBeTruthy();
    expect(
      within(activeCard).getByText("Current dispatch").className,
    ).toContain("text-on-secondary-container");
    expect(activeCard.className).toContain("border-outline-variant");
    expect(within(historicalCard).queryByText("Current dispatch")).toBeNull();
    expect(historicalCard.className).not.toContain("border-outline-variant");
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
    expect(screen.queryByRole("button", { name: "Open trace" })).toBeNull();
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
    const requestDetails = within(
      expandDispatchSection(dispatchHistory, "Summary"),
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
      requestDetails.getByRole("button", { name: "trace-active-story" }),
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

    const traceButton = within(
      expandDispatchSection(
        screen.getByRole("region", { name: "Workstation dispatches" }),
        "Summary",
      ),
    ).getByRole("button", { name: "trace-active-story" });

    fireEvent.click(traceButton);

    expect(onSelectTraceID).toHaveBeenCalledWith("trace-active-story");
  });
});

describe("WorkItemDetailCard request-detail fallbacks", () => {
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
    const dispatchCard = getDispatchHistoryCard(dispatchHistory, dispatchID);

    if (!(dispatchCard instanceof HTMLElement)) {
      throw new Error("expected markdown dispatch history card");
    }

    const requestDetails = within(
      within(dispatchCard).getByRole("region", { name: "Summary" }),
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

describe("WorkItemDetailCard relationship graph", () => {
  let restoreBrowserShims: (() => void) | undefined;

  beforeEach(() => {
    restoreBrowserShims = installDashboardBrowserTestShims();
  });

  afterEach(() => {
    restoreBrowserShims?.();
    restoreBrowserShims = undefined;
  });

  it("renders work relationships with the shared trace relation graph surface", async () => {
    const user = userEvent.setup();
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
        relationshipGraph={buildRelationshipGraph(workItem)}
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
    const traceGraph = await within(relationshipGraph).findByRole("region", {
      name: "Batch relation graph",
    });

    expect(traceGraph.getAttribute("data-dashboard-graph-frame")).toBe("true");
    expect(traceGraph.getAttribute("data-trace-relation-flow")).not.toBeNull();
    expect(within(traceGraph).getByText("Active Story")).toBeTruthy();
    expect(within(traceGraph).getByText("Parent Story")).toBeTruthy();
    expect(within(traceGraph).getByText("Dependency Story")).toBeTruthy();
    expect(within(traceGraph).getByText("Blocked Story")).toBeTruthy();
    expect(within(traceGraph).getByText("Child Story")).toBeTruthy();
    expect(within(traceGraph).queryByText("Selected work")).toBeNull();
    expect(within(traceGraph).queryByText("Relationship key")).toBeNull();
    expect(within(traceGraph).queryByText("ready")).toBeNull();
    expect(within(traceGraph).queryByText("queued")).toBeNull();
    expect(within(traceGraph).queryByText("trace-parent-story")).toBeNull();
    expect(
      traceGraph.querySelector('button[aria-label="Active Story"]'),
    ).toBeNull();

    const activeNode = getTraceGraphNodeShell(traceGraph, "Active Story");
    expect(activeNode.className).toContain("border-primary");
    expect(activeNode.className).toContain("bg-primary-container");

    const focusedSummary = within(relationshipGraph).getByRole("region", {
      name: "Focused work summary",
    });

    expect(within(focusedSummary).getByText("Relationship role")).toBeTruthy();
    expect(within(focusedSummary).getByText("Current selection")).toBeTruthy();
    expect(within(focusedSummary).getByText("work-active-story")).toBeTruthy();

    fireEvent.click(activeNode);
    expect(onSelectWorkID).not.toHaveBeenCalled();

    await user.click(getTraceGraphNodeButton(traceGraph, "Dependency Story"));
    getTraceGraphNodeButton(traceGraph, "Parent Story").focus();
    await user.keyboard("{Enter}");

    expect(onSelectWorkID).toHaveBeenNthCalledWith(1, "work-dependency-story");
    expect(onSelectWorkID).toHaveBeenNthCalledWith(2, "work-parent-story");
  });

  it("keeps focused-node trace actions in the graph summary when trace inspection is available", async () => {
    const { dispatchID, execution, selectedNode, workItem } =
      getSelectedWorkItemFixture();
    const onSelectTraceID = vi.fn();

    render(
      <WorkItemDetailCard
        activeTraceID="trace-active-story"
        dispatchAttempts={[]}
        executionDetails={selectWorkItemExecutionDetails({
          activeExecution: execution,
          dispatchID,
          selectedNode,
          workItem,
        })}
        now={DETAIL_CARD_NOW}
        onSelectTraceID={onSelectTraceID}
        selectedNode={selectedNode}
        relationshipGraph={buildRelationshipGraph(workItem)}
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

    await within(
      screen.getByRole("region", { name: "Work relationships" }),
    ).findByRole("region", { name: "Batch relation graph" });

    const focusedSummary = screen.getByRole("region", {
      name: "Focused work summary",
    });
    const traceAction = within(focusedSummary).getByRole("button", {
      name: "Open trace",
    });

    expect(
      within(focusedSummary).getByText("trace-active-story (selected)"),
    ).toBeTruthy();
    fireEvent.click(traceAction);

    expect(onSelectTraceID).toHaveBeenCalledWith("trace-active-story");
  });

  it("renders missing focused-node trace metadata explicitly when no trace is available", async () => {
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
        onSelectTraceID={vi.fn()}
        selectedNode={selectedNode}
        relationshipGraph={{
          edges: [
            {
              relationship: "CHILD",
              sourceWorkID: workItem.work_id,
              targetWorkID: "work-child-story",
            },
          ],
          relatedWork: [
            {
              label: "Child Story",
              state: "running",
              traceID: "trace-child-story",
              workID: "work-child-story",
              workTypeID: "task",
            },
          ],
          selectedWork: {
            label: "Active Story",
            state: "in_progress",
            workID: workItem.work_id,
            workTypeID: workItem.work_type_id,
          },
          status: "ready",
        }}
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

    await within(
      screen.getByRole("region", { name: "Work relationships" }),
    ).findByRole("region", { name: "Batch relation graph" });

    const focusedSummary = screen.getByRole("region", {
      name: "Focused work summary",
    });

    expect(within(focusedSummary).getByText("Unavailable")).toBeTruthy();
    expect(
      within(focusedSummary).queryByRole("button", { name: "Open trace" }),
    ).toBeNull();
  });

  it("re-renders the graph when the current work selection changes", async () => {
    const { dispatchID, execution, selectedNode, workItem } =
      getSelectedWorkItemFixture();
    const onSelectWorkID = vi.fn();
    const parentWorkItem = {
      display_name: "Parent Story",
      state: "done",
      trace_id: "trace-parent-story",
      work_id: "work-parent-story",
      work_type_id: "epic",
    } satisfies DashboardWorkItemRef;

    const { rerender } = render(
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
        relationshipGraph={buildRelationshipGraph(workItem)}
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

    const initialTraceGraph = await within(
      screen.getByRole("region", { name: "Work relationships" }),
    ).findByRole("region", { name: "Batch relation graph" });

    fireEvent.click(getTraceGraphNodeButton(initialTraceGraph, "Parent Story"));

    expect(onSelectWorkID).toHaveBeenCalledWith("work-parent-story");

    rerender(
      <WorkItemDetailCard
        dispatchAttempts={[]}
        executionDetails={selectWorkItemExecutionDetails({
          activeExecution: execution,
          dispatchID,
          selectedNode,
          workItem: parentWorkItem,
        })}
        now={DETAIL_CARD_NOW}
        onSelectWorkID={onSelectWorkID}
        selectedNode={selectedNode}
        relationshipGraph={{
          edges: [
            {
              relationship: "CHILD",
              sourceWorkID: parentWorkItem.work_id,
              targetWorkID: workItem.work_id,
            },
          ],
          relatedWork: [
            {
              label: "Active Story",
              state: "in_progress",
              traceID: workItem.trace_id,
              workID: workItem.work_id,
              workTypeID: workItem.work_type_id,
            },
          ],
          selectedWork: {
            label: "Parent Story",
            state: "done",
            traceID: parentWorkItem.trace_id,
            workID: parentWorkItem.work_id,
            workTypeID: parentWorkItem.work_type_id,
          },
          status: "ready",
        }}
        selection={{
          dispatchId: dispatchID,
          execution,
          kind: "work-item",
          nodeId: selectedNode.node_id,
          workItem: parentWorkItem,
        }}
        workstationRequests={[]}
      />,
    );

    const relationshipGraph = screen.getByRole("region", {
      name: "Work relationships",
    });
    const traceGraph = await within(relationshipGraph).findByRole("region", {
      name: "Batch relation graph",
    });

    expect(within(traceGraph).getByText("Parent Story")).toBeTruthy();
    expect(getTraceGraphNodeButton(traceGraph, "Active Story")).toBeTruthy();
    expect(
      traceGraph.querySelector('button[aria-label="Parent Story"]'),
    ).toBeNull();
    expect(
      getTraceGraphNodeShell(traceGraph, "Parent Story").className,
    ).toContain("border-primary");
    expect(
      within(
        screen.getByRole("region", { name: "Focused work summary" }),
      ).getByText(parentWorkItem.work_id),
    ).toBeTruthy();
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
        relationshipGraph={{
          edges: [],
          relatedWork: [],
          selectedWork: {
            label: "Active Story",
            workID: workItem.work_id,
          },
          status: "empty",
        }}
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

  it("renders an explicit loading state while work relationships are still loading", () => {
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
        relationshipGraph={{ status: "loading" }}
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
    const status = within(relationshipSection).getByRole("status");

    expect(status.textContent).toContain(
      "Work relationships are still loading for this work item.",
    );
    expect(within(relationshipSection).queryByText("Selected work")).toBeNull();
  });

  it("renders an explicit error state when work relationships fail to load", () => {
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
        relationshipGraph={{
          message:
            "Work relationship data is unavailable for the selected timeline snapshot.",
          selectedWork: {
            label: "Active Story",
            state: "in_progress",
            traceID: "trace-active-story",
            workID: "work-active-story",
            workTypeID: "story",
          },
          status: "error",
        }}
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
    const alert = within(relationshipSection).getByRole("alert");

    expect(alert.textContent).toContain(
      "Work relationships could not be loaded for this work item.",
    );
    expect(
      within(relationshipSection).getByText(
        "Work relationships could not be loaded for this work item.",
      ).className,
    ).toContain("!text-current");
    expect(
      within(relationshipSection).getByText(/selected timeline snapshot/i)
        .className,
    ).toContain("text-body-small");
    expect(within(relationshipSection).queryByText("Selected work")).toBeNull();
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
        name: `ディスパッチ ${dispatchID} の provider session codex / パス / sess-ja-unsupported を選択`,
      }),
    ).toBeNull();
    expect(
      inferenceAttempts.getByText("セッション詳細は利用できません"),
    ).toBeTruthy();
  });

  it("localizes dispatch-history started-at and duration rows for zh-CN", () => {
    const { dispatchID, execution, selectedNode, workItem } =
      getSelectedWorkItemFixture();

    render(
      <CurrentSelectionLocaleProvider locale="zh-CN">
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
          workstationRequests={[dashboardWorkstationRequestFixtures.ready]}
        />
      </CurrentSelectionLocaleProvider>,
    );

    const dispatchCard = within(
      screen.getByRole("region", { name: "工作站分派" }),
    )
      .getByText("Active Story", { selector: "strong" })
      .closest("article");

    if (!(dispatchCard instanceof HTMLElement)) {
      throw new Error("expected localized dispatch history card");
    }

    const requestDetails = expandDispatchSection(dispatchCard, "摘要", "展开");
    expect(within(requestDetails).getByText("开始时间")).toBeTruthy();
    expect(
      within(getDetailRow(requestDetails, "开始时间")).getByText(
        formatLocalDateTime("2026-04-08T12:00:01Z", "不可用", "zh-CN"),
      ),
    ).toBeTruthy();
    expect(within(requestDetails).getByText("耗时")).toBeTruthy();
    expect(
      within(getDetailRow(requestDetails, "耗时")).getByText(
        formatDurationMillis(63_000, "zh-CN"),
      ),
    ).toBeTruthy();
    expect(within(dispatchCard).queryByText("2026-04-08T12:00:01Z")).toBeNull();
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
    const activeCard = getDispatchHistoryCard(dispatchHistory, dispatchID);
    const historicalCard = getDispatchHistoryCard(
      dispatchHistory,
      historicalDispatchID,
    );

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
    const activeRequestBody = expandAttemptBody(
      expandedActiveAttempt,
      "Request body",
    );
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
    const dispatchCard = getDispatchHistoryCard(dispatchHistory, dispatchID);

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
    const firstAttemptCard = expandInferenceAttempt(
      inferenceAttemptsSection,
      1,
    );
    const secondAttemptCard = expandInferenceAttempt(
      inferenceAttemptsSection,
      2,
    );
    const secondRequestBody = expandAttemptBody(
      secondAttemptCard,
      "Request body",
    );
    const secondResponseBody = expandAttemptBody(
      secondAttemptCard,
      "Response body",
    );
    expect(
      within(firstAttemptCard).getByText(`${dispatchID}/inference-request/1`),
    ).toBeTruthy();
    expect(within(firstAttemptCard).getByText("gpt-5.4-mini")).toBeTruthy();
    expect(within(secondAttemptCard).getByText("codex")).toBeTruthy();
    expect(
      within(secondAttemptCard).getByText(
        "codex / Session ID / sess-ready-request",
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

    const requestDetails = expandDispatchSection(dispatchCard, "Summary");
    const traceButton = within(requestDetails).getByRole("button", {
      name: "trace-active-story",
    });
    fireEvent.click(traceButton);
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
      expandDispatchSection(document.body, "Failure details"),
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
    const failedRequestDetails = expandDispatchSection(
      dispatchHistory,
      "Summary",
    );
    expect(
      within(failedRequestDetails).getAllByRole("button", {
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
    const dispatchCard = getDispatchHistoryCard(
      dispatchHistory,
      dashboardWorkstationRequestFixtures.scriptPending.dispatch_id,
    );

    if (!(dispatchCard instanceof HTMLElement)) {
      throw new Error("expected pending script dispatch history card");
    }

    const requestDetails = expandDispatchSection(dispatchCard, "Summary");
    expect(
      within(requestDetails).getByText(
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
        within(dispatchCard).getByRole("region", { name: "Summary" }),
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
    expect(scriptAttempts.getByText("Pending")).toBeTruthy();
    expect(
      scriptAttempts.getByText(
        "No script response attempt has been recorded yet.",
      ),
    ).toBeTruthy();
    expect(
      within(dispatchCard).queryByText("No response yet for this dispatch."),
    ).toBeNull();
  });

  it("uses the selected work title as the dispatch heading while keeping the dispatch id inside summary details", () => {
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
    const requestDetails = expandDispatchSection(dispatchCard, "Summary");
    expect(
      within(requestDetails).getByText(
        dashboardWorkstationRequestFixtures.ready.dispatch_id,
      ),
    ).toBeTruthy();
    expect(within(requestDetails).getByText("Started at")).toBeTruthy();
    expect(
      within(getDetailRow(requestDetails, "Started at")).getByText(
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

  it("falls back to the unknown-dispatch title while keeping the dispatch id in summary details when no associated work label is available", () => {
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
    const dispatchCard = getDispatchHistoryCard(
      dispatchHistory,
      dashboardWorkstationRequestFixtures.noResponse.dispatch_id,
    );

    if (!(dispatchCard instanceof HTMLElement)) {
      throw new Error("expected fallback-title dispatch history card");
    }

    expect(
      within(dispatchCard).getByText("Unknown dispatch", {
        selector: "strong",
      }),
    ).toBeTruthy();
    expect(
      within(expandDispatchSection(dispatchCard, "Summary")).getByText(
        dashboardWorkstationRequestFixtures.noResponse.dispatch_id,
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
    const dispatchCard = getDispatchHistoryCard(
      dispatchHistory,
      dashboardWorkstationRequestFixtures.scriptPending.dispatch_id,
    );

    if (!(dispatchCard instanceof HTMLElement)) {
      throw new Error("expected fallback dispatch history card");
    }

    const fallbackRequestDetails = expandDispatchSection(
      dispatchCard,
      "Summary",
    );
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
    expect(
      within(fallbackRequestDetails).getByText("Workstation", {
        selector: "dt",
      }),
    ).toBeTruthy();
    expect(
      within(fallbackRequestDetails).queryByText("Resolved args"),
    ).toBeNull();
    const selectedWorkButton = within(dispatchCard).getByRole("button", {
      name: "Select work item Active Story",
    });
    expect(selectedWorkButton).toBeTruthy();
    expect(selectedWorkButton.textContent).toContain("Active Story");
    expect(selectedWorkButton.className).toContain("border-outline");
    expect(selectedWorkButton.className).not.toContain("bg-primary-container");
    const responseDetails = expandDispatchSection(
      dispatchCard,
      "Response details",
    );
    expect(
      within(getDetailRow(responseDetails, "Trace IDs")).getByRole("button", {
        name: "trace-active-story (selected)",
      }),
    ).toBeTruthy();
    const selectedTraceButton = within(responseDetails).getByRole("button", {
      name: "trace-active-story (selected)",
    });
    expect(selectedTraceButton).toBeTruthy();
    expect(selectedTraceButton.getAttribute("aria-pressed")).toBe("true");
    expect(selectedTraceButton.className).toContain("border-outline");
    expect(selectedTraceButton.className).not.toContain("bg-primary-container");
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
    const dispatchCard = getDispatchHistoryCard(
      dispatchHistory,
      dashboardWorkstationRequestFixtures.scriptSuccess.dispatch_id,
    );

    if (!(dispatchCard instanceof HTMLElement)) {
      throw new Error("expected script success dispatch history card");
    }

    const requestDetails = expandDispatchSection(dispatchCard, "Summary");
    expect(
      within(requestDetails).getAllByText("Succeeded").length,
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
        within(dispatchCard).getByRole("region", { name: "Summary" }),
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
    const responseDetails = expandDispatchSection(
      dispatchCard,
      "Response details",
    );
    expect(
      within(getDetailRow(responseDetails, "Trace IDs")).getByRole("button", {
        name: "trace-active-story",
      }),
    ).toBeTruthy();
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
    const dispatchCard = getDispatchHistoryCard(
      dispatchHistory,
      dashboardWorkstationRequestFixtures.scriptFailed.dispatch_id,
    );

    if (!(dispatchCard instanceof HTMLElement)) {
      throw new Error("expected script failure dispatch history card");
    }

    const requestDetails = expandDispatchSection(dispatchCard, "Summary");
    expect(
      within(requestDetails).getAllByText("Timed out").length,
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
    const dispatchCard = getDispatchHistoryCard(
      dispatchHistory,
      dashboardWorkstationRequestFixtures.rejected.dispatch_id,
    );

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
        within(dispatchCard).getByRole("region", { name: "Summary" }),
      ).queryByText(
        "Inference request details are shown under Inference attempts.",
      ),
    ).toBeNull();
    expect(
      inferenceAttempts.getByText(
        `codex / Session ID / ${dashboardWorkstationRequestFixtures.rejected.inference_attempts?.[0]?.provider_session?.id}`,
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
    const dispatchCard = getDispatchHistoryCard(
      dispatchHistory,
      dashboardWorkstationRequestFixtures.scriptPending.dispatch_id,
    );

    if (!(dispatchCard instanceof HTMLElement)) {
      throw new Error("expected localized dispatch history card");
    }

    expect(
      within(dispatchCard).getByRole("region", {
        name: "概要",
      }),
    ).toBeTruthy();
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
    const localizedRequestDetails = expandDispatchSection(
      dispatchCard,
      "概要",
      "展開",
    );
    expect(
      within(localizedRequestDetails).getByText("ワークステーション", {
        selector: "dt",
      }),
    ).toBeTruthy();
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
    const localizedResponseDetails = expandDispatchSection(
      dispatchCard,
      "応答の詳細",
      "展開",
    );
    expect(
      within(getDetailRow(localizedResponseDetails, "トレース ID")).getByRole(
        "button",
        {
          name: "trace-active-story（選択中）",
        },
      ),
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
    const dispatchCard = getDispatchHistoryCard(
      dispatchHistory,
      dashboardWorkstationRequestFixtures.scriptSuccess.dispatch_id,
    );

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
