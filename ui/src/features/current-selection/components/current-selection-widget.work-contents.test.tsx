import { screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import {
  getSelectedWorkItemFixture,
  MULTIMODAL_SELECTED_WORK_PAYLOAD_CONTENT,
  multimodalSelectedWorkPayloadOverrides,
  withMultimodalSelectedWorkPayload,
} from "../base/components/detail-card-test-helpers";
import { selectWorkItemExecutionDetails } from "../state/executionDetails";
import type { DashboardSelection } from "../state/selection-types";
import type { CurrentSelectionState } from "../hooks/useCurrentSelection";
import { CurrentSelectionWidget } from "./current-selection-widget";
import { renderWithQueryClient } from "./current-selection-widget-test-utils";

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
    selectedWorker: null,
    selectedWorkerName: null,
    selectedWorkerWorkstationNames: [],
    selection: null,
    selectWorkByID: () => undefined,
    selectStateNode: () => undefined,
    selectStateWorkItem: () => undefined,
    selectWorkItem: () => undefined,
    selectWorker: () => undefined,
    selectWorkstation: () => undefined,
    selectWorkstationRequest: () => undefined,
    terminalWorkDetail: null,
    undoSelection: () => undefined,
    ...overrides,
  };
}

function buildMultimodalWorkItemSelection(): {
  executionDetails: ReturnType<typeof selectWorkItemExecutionDetails>;
  providerSessions: NonNullable<
    ReturnType<typeof getSelectedWorkItemFixture>["snapshot"]["runtime"]["session"]["provider_sessions"]
  >;
  selectedNode: ReturnType<typeof getSelectedWorkItemFixture>["selectedNode"];
  selectedWorkRequestHistory: ReturnType<
    typeof getSelectedWorkItemFixture
  >["snapshot"]["runtime"]["workstation_requests_by_dispatch_id"][string][];
  selection: DashboardSelection;
  workItem: ReturnType<typeof withMultimodalSelectedWorkPayload>;
} {
  const { dispatchID, execution, selectedNode, snapshot, workItem } =
    getSelectedWorkItemFixture();
  const selectedWorkItem = withMultimodalSelectedWorkPayload(workItem);
  const providerSessions =
    snapshot.runtime.session.provider_sessions?.filter((attempt) =>
      attempt.work_items?.some(
        (candidate) => candidate.work_id === selectedWorkItem.work_id,
      ),
    ) ?? [];
  const selectedWorkRequestHistory = snapshot.runtime
    .workstation_requests_by_dispatch_id?.[dispatchID]
    ? [snapshot.runtime.workstation_requests_by_dispatch_id[dispatchID]]
    : [];

  const selection: DashboardSelection = {
    dispatchId: dispatchID,
    execution: {
      ...execution,
      work_items: execution.work_items?.map((item) =>
        item.work_id === selectedWorkItem.work_id ? selectedWorkItem : item,
      ),
    },
    kind: "work-item",
    nodeId: selectedNode.node_id,
    workItem: selectedWorkItem,
  };

  return {
    executionDetails: selectWorkItemExecutionDetails({
      activeExecution: selection.execution,
      dispatchID,
      inferenceAttemptsByDispatchID:
        snapshot.runtime.inference_attempts_by_dispatch_id,
      providerSessions,
      selectedNode,
      workItem: selectedWorkItem,
      workstationRequestsByDispatchID:
        snapshot.runtime.workstation_requests_by_dispatch_id,
    }),
    providerSessions,
    selectedNode,
    selectedWorkRequestHistory,
    selection,
    workItem: selectedWorkItem,
  };
}

describe("CurrentSelectionWidget work contents", () => {
  it("lists multimodal payload parts in the Work contents region for a work-item selection", () => {
    const {
      executionDetails,
      providerSessions,
      selectedNode,
      selectedWorkRequestHistory,
      selection,
      workItem,
    } = buildMultimodalWorkItemSelection();

    renderWithQueryClient(
      <CurrentSelectionWidget
        currentSelection={buildCurrentSelection({
          selectedNode,
          selectedNodeProviderSessions: providerSessions,
          selectedWorkDispatchAttempts: providerSessions,
          selectedWorkID: workItem.work_id,
          selectedWorkProviderSessions: providerSessions,
          selectedWorkRequestHistory,
          selectedWorkWorkstationRequests: selectedWorkRequestHistory,
          selection,
        })}
        now={DETAIL_CARD_NOW}
        selectedWorkExecutionDetails={executionDetails}
      />,
    );

    const currentSelection = screen.getByRole("article", {
      name: "Current selection",
    });
    const workContents = within(currentSelection).getByRole("region", {
      name: "Work contents",
    });

    expect(
      within(workContents).getByText(
        MULTIMODAL_SELECTED_WORK_PAYLOAD_CONTENT.find((part) => part.type === "text")
          ?.text ?? "",
      ),
    ).toBeTruthy();
    expect(within(workContents).getByText(/"priority": 1/)).toBeTruthy();
    expect(within(workContents).getByText("Image: screenshot.png")).toBeTruthy();
    expect(workItem).toMatchObject(multimodalSelectedWorkPayloadOverrides());
  });
});
