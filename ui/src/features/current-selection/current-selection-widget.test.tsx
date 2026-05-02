import { fireEvent, render, screen, within } from "@testing-library/react";

import { buildDashboardWorkstationRequestFixture } from "../../components/dashboard/fixtures";
import { semanticWorkflowDashboardSnapshot } from "../../components/dashboard/test-fixtures";
import { selectWorkItemExecutionDetails } from "../../state/executionDetails";
import { resetSelectionHistoryStore } from "../../state/selectionHistoryStore";
import { CurrentSelectionWidget } from "./current-selection-widget";
import type { CurrentSelectionState } from "./useCurrentSelection";
import type {
  DashboardSelection,
  TerminalWorkDetail,
} from "./types";

const DETAIL_CARD_NOW = Date.parse("2026-04-08T12:00:04Z");

function buildCurrentSelection(
  overrides: Partial<CurrentSelectionState> = {},
): CurrentSelectionState {
  return {
    canRedoSelection: false,
    canUndoSelection: false,
    completedWorkItems: [],
    failedWorkItems: [],
    openTerminalWorkDetail: () => undefined,
    redoSelection: () => undefined,
    selectedNode: null,
    selectedNodeActiveExecutions: [],
    selectedNodeProviderSessions: [],
    selectedStateCurrentWorkItems: [],
    selectedStatePlace: null,
    selectedStateTerminalHistoryWorkItems: [],
    selectedStateTokenCount: 0,
    selectedWorkDispatchAttempts: [],
    selectedWorkID: null,
    selectedWorkProviderSessions: [],
    selectedWorkRequestHistory: [],
    selectedWorkWorkstationRequests: [],
    selectedWorkstationRequest: null,
    selectedNodeWorkstationRequests: [],
    selection: null,
    selectWorkByID: () => undefined,
    selectStateNode: () => undefined,
    selectStateWorkItem: () => undefined,
    selectWorkItem: () => undefined,
    selectWorkstation: () => undefined,
    selectWorkstationRequest: () => undefined,
    terminalWorkDetail: null,
    undoSelection: () => undefined,
    ...overrides,
  };
}

function buildSelectedWorkItemFixture() {
  const snapshot = semanticWorkflowDashboardSnapshot;
  const dispatchId = snapshot.runtime.active_dispatch_ids?.[0] ?? "";
  const execution = snapshot.runtime.active_executions_by_dispatch_id?.[dispatchId];
  const workItem = execution?.work_items?.[0];
  const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
  const providerSessions = snapshot.runtime.session.provider_sessions?.filter((attempt) =>
    attempt.work_items?.some((candidate) => candidate.work_id === workItem?.work_id),
  );
  const selectedWorkRequestHistory = snapshot.runtime.workstation_requests_by_dispatch_id?.[
    dispatchId
  ]
    ? [snapshot.runtime.workstation_requests_by_dispatch_id[dispatchId]]
    : [];

  if (!execution || !workItem || !selectedNode) {
    throw new Error("expected semantic workflow fixture to include an active selected work item");
  }

  const selection: DashboardSelection = {
    dispatchId,
    execution,
    kind: "work-item",
    nodeId: selectedNode.node_id,
    workItem,
  };

  return {
    executionDetails: selectWorkItemExecutionDetails({
      activeExecution: execution,
      dispatchID: dispatchId,
      inferenceAttemptsByDispatchID: snapshot.runtime.inference_attempts_by_dispatch_id,
      providerSessions: providerSessions ?? [],
      selectedNode,
      workItem,
      workstationRequestsByDispatchID: snapshot.runtime.workstation_requests_by_dispatch_id,
    }),
    providerSessions: providerSessions ?? [],
    selectedWorkRequestHistory,
    selectedNode,
    selection,
    snapshot,
    workItem,
  };
}

describe("CurrentSelectionWidget", () => {
  beforeEach(() => {
    resetSelectionHistoryStore();
  });

  afterEach(() => {
    resetSelectionHistoryStore();
  });

  it("keeps the work item card visible even when terminal work metadata is present", () => {
    const {
      executionDetails,
      providerSessions,
      selectedNode,
      selectedWorkRequestHistory,
      selection,
      workItem,
    } =
      buildSelectedWorkItemFixture();
    const terminalWorkDetail: TerminalWorkDetail = {
      attempts: providerSessions,
      label: workItem.display_name ?? workItem.work_id,
      status: "failed",
      traceWorkID: workItem.work_id,
    };

    render(
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection({
          selectedNode,
          selectedNodeProviderSessions: providerSessions,
          selectedWorkProviderSessions: providerSessions,
          selectedWorkRequestHistory,
          selectedWorkDispatchAttempts: providerSessions,
          selection,
          terminalWorkDetail,
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={executionDetails}
        terminalWorkExecutionDetails={executionDetails}
      />,
    );

    const currentSelection = screen.getByRole("article", { name: "Current selection" });
    expect(within(currentSelection).getByText(workItem.work_id)).toBeTruthy();
    expect(within(currentSelection).getByRole("heading", { name: "Execution details" })).toBeTruthy();
    expect(within(currentSelection).queryByRole("heading", { name: "Inference attempts" })).toBeNull();
    expect(
      within(currentSelection).getByRole("heading", { name: "Workstation dispatches" }),
    ).toBeTruthy();
    expect(within(currentSelection).getByText("Current dispatch")).toBeTruthy();
    expect(
      within(currentSelection).queryByRole("heading", { name: "Work session runs list" }),
    ).toBeNull();
  });

  it("renders work item details when the active selection is a work item", () => {
    const {
      executionDetails,
      providerSessions,
      selectedNode,
      selectedWorkRequestHistory,
      selection,
      workItem,
    } =
      buildSelectedWorkItemFixture();

    render(
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection({
          selectedNode,
          selectedNodeProviderSessions: providerSessions,
          selectedWorkProviderSessions: providerSessions,
          selectedWorkRequestHistory,
          selectedWorkDispatchAttempts: providerSessions,
          selection,
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={executionDetails}
        terminalWorkExecutionDetails={null}
      />,
    );

    const currentSelection = screen.getByRole("article", { name: "Current selection" });
    expect(within(currentSelection).getByText(workItem.work_id)).toBeTruthy();
    expect(within(currentSelection).getByRole("heading", { name: "Execution details" })).toBeTruthy();
    expect(within(currentSelection).queryByRole("heading", { name: "Inference attempts" })).toBeNull();
    expect(
      within(currentSelection).getByRole("heading", { name: "Workstation dispatches" }),
    ).toBeTruthy();
    expect(within(currentSelection).getByText("Current dispatch")).toBeTruthy();
    expect(
      within(currentSelection).queryByRole("heading", { name: "Work session runs list" }),
    ).toBeNull();
  });

  it("renders selected state details when a state node is active", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedStatePlace =
      snapshot.topology.workstation_nodes_by_id.review.output_places?.find(
        (place) => place.place_id === "story:complete",
      ) ?? null;

    if (!selectedStatePlace) {
      throw new Error("expected semantic workflow fixture to include a terminal state place");
    }

    render(
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection({
          selectedStatePlace,
          selectedStateTerminalHistoryWorkItems: [
            {
              display_name: "Done Story",
              trace_id: "trace-done-story",
              work_id: "work-done-story",
              work_type_id: "story",
            },
          ],
          selectedStateTokenCount: 1,
          selection: { kind: "state-node", placeId: selectedStatePlace.place_id },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
        terminalWorkExecutionDetails={null}
      />,
    );

    const currentSelection = screen.getByRole("article", { name: "Current selection" });
    expect(within(currentSelection).getByTitle("story:complete")).toBeTruthy();
    expect(within(currentSelection).getByText("Current work")).toBeTruthy();
    expect(within(currentSelection).getByText("Done Story")).toBeTruthy();
  });

  it("forwards state-node work-item clicks into the current selection handler", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedStatePlace =
      snapshot.topology.workstation_nodes_by_id.review.output_places?.find(
        (place) => place.place_id === "story:complete",
      ) ?? null;
    const selectStateWorkItem = vi.fn();

    if (!selectedStatePlace) {
      throw new Error("expected semantic workflow fixture to include a terminal state place");
    }

    render(
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection({
          selectStateWorkItem,
          selectedStatePlace,
          selectedStateTerminalHistoryWorkItems: [
            {
              display_name: "Done Story",
              trace_id: "trace-done-story",
              work_id: "work-done-story",
              work_type_id: "story",
            },
          ],
          selectedStateTokenCount: 1,
          selection: { kind: "state-node", placeId: selectedStatePlace.place_id },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
        terminalWorkExecutionDetails={null}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Select work item Done Story" }));

    expect(selectStateWorkItem).toHaveBeenCalledWith(selectedStatePlace, {
      display_name: "Done Story",
      trace_id: "trace-done-story",
      work_id: "work-done-story",
      work_type_id: "story",
    });
  });

  it("renders workstation details when a workstation is active", () => {
    const snapshot = semanticWorkflowDashboardSnapshot;
    const selectedNode = snapshot.topology.workstation_nodes_by_id.review;
    const providerSessions = snapshot.runtime.session.provider_sessions?.filter(
      (attempt) =>
        attempt.transition_id === selectedNode.transition_id ||
        attempt.workstation_name === selectedNode.workstation_name,
    );

    render(
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection({
          selectedNode,
          selectedNodeProviderSessions: providerSessions ?? [],
          selection: { kind: "node", nodeId: selectedNode.node_id },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
        terminalWorkExecutionDetails={null}
      />,
    );

    const currentSelection = screen.getByRole("article", { name: "Current selection" });
    expect(within(currentSelection).getByRole("heading", { name: "Active work" })).toBeTruthy();
    expect(within(currentSelection).getByRole("heading", { name: "Run history" })).toBeTruthy();
  });

  it("renders workstation request details when a workstation request is selected", () => {
    const selectedWorkstationRequest = buildDashboardWorkstationRequestFixture(
      "dispatch-review-markdown",
      {
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
      },
    );

    render(
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection({
          selectedNode: semanticWorkflowDashboardSnapshot.topology.workstation_nodes_by_id.review,
          selectedWorkstationRequest,
          selection: {
            dispatchId: selectedWorkstationRequest.dispatch_id,
            kind: "workstation-request",
            nodeId: selectedWorkstationRequest.workstation_node_id,
            request: selectedWorkstationRequest,
          },
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
        terminalWorkExecutionDetails={null}
      />,
    );

    const currentSelection = screen.getByRole("article", { name: "Current selection" });
    expect(within(currentSelection).getAllByText("request-markdown-story").length).toBeGreaterThan(0);
    expect(
      within(currentSelection).getByRole("heading", { level: 2, name: "Review checklist" }),
    ).toBeTruthy();
    expect(within(currentSelection).getByRole("list")).toBeTruthy();
    expect(within(currentSelection).getByText("Check the latest diff")).toBeTruthy();
    expect(within(currentSelection).queryByRole("heading", { name: "Active work" })).toBeNull();
  });

  it("renders the empty current-selection guidance when nothing is selected", () => {
    render(
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection()}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
        terminalWorkExecutionDetails={null}
      />,
    );

    expect(screen.getByText("Select a workstation, work item, or state node to inspect live details.")).toBeTruthy();
  });

  it("renders disabled undo and redo controls in the shared current-selection header by default", () => {
    render(
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection()}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={null}
        terminalWorkExecutionDetails={null}
      />,
    );

    expect(
      screen.getByRole("button", { name: "Undo selection" }).getAttribute("disabled"),
    ).not.toBeNull();
    expect(
      screen.getByRole("button", { name: "Redo selection" }).getAttribute("disabled"),
    ).not.toBeNull();
  });
});
