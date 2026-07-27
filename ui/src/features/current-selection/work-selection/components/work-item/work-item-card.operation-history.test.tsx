import "../../../../../testing/vitest-dom-capabilities.setup";

import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type {
  DashboardWorkMoveOperation,
  DashboardWorkstationRequest,
} from "../../../../../api/dashboard/types";
import {
  getSelectedWorkItemFixture,
  workstationRequest,
} from "../../../base/components/detail-card/detail-card-test-helpers";
import { CurrentSelectionLocaleProvider } from "../../../base/components/presentation/current-selection-locale";
import type { SelectedWorkOperationHistoryItem } from "../../../hooks/helpers/selected-work-operation-history";
import { selectWorkItemExecutionDetails } from "../../state/executionDetails";
import { WorkItemDetailCard } from "./work-item-card";

function renderWorkItemWithOperationHistory(
  operationHistory: SelectedWorkOperationHistoryItem[],
  workstationRequests: DashboardWorkstationRequest[] = [],
) {
  const { dispatchID, execution, selectedNode, workItem } =
    getSelectedWorkItemFixture();

  render(
    <CurrentSelectionLocaleProvider>
      <WorkItemDetailCard
        dispatchAttempts={[]}
        executionDetails={selectWorkItemExecutionDetails({
          activeExecution: execution,
          dispatchID,
          selectedNode,
          workItem,
        })}
        operationHistory={operationHistory}
        selectedNode={selectedNode}
        selection={{
          dispatchId: dispatchID,
          execution,
          kind: "work-item",
          nodeId: selectedNode.node_id,
          workItem,
        }}
        workstationRequests={workstationRequests}
      />
    </CurrentSelectionLocaleProvider>,
  );
}

function buildMoveOperation(
  overrides: Partial<DashboardWorkMoveOperation> = {},
): DashboardWorkMoveOperation {
  return {
    event_time: "2026-04-08T14:00:00Z",
    from_place_id: "story:init",
    from_state: "init",
    sequence: 2,
    source: "api",
    tick: 3,
    to_place_id: "story:review",
    to_state: "review",
    work_id: "work-active-story",
    ...overrides,
  };
}

describe("WorkItemDetailCard operation history", () => {
  it("renders operator move and workstation rows with distinct accessible labels", () => {
    const modelRequest = workstationRequest("dispatch-model", {
      started_at: "2026-04-08T12:00:00Z",
      workstation_name: "Review",
    });
    const move = buildMoveOperation({ source: "cli" });

    renderWorkItemWithOperationHistory(
      [
        { kind: "operator-move", move },
        { kind: "workstation", request: modelRequest },
      ],
      [modelRequest],
    );

    const operationsRegion = screen.getByRole("region", {
      name: "Work operations",
    });

    const operatorMoveCard = within(operationsRegion).getByRole("article", {
      name: "Operator move init → review",
    });
    expect(operatorMoveCard.className).toContain("bg-surface-container-high");
    expect(within(operatorMoveCard).getByText("Operator move")).toBeTruthy();
    expect(within(operatorMoveCard).getByText("Move")).toBeTruthy();
    expect(within(operatorMoveCard).getByText("CLI")).toBeTruthy();
    expect(
      within(operatorMoveCard).queryByRole("heading", {
        name: "Inference attempts",
      }),
    ).toBeNull();

    const workstationCard = within(operationsRegion).getByRole("article", {
      name: "Workstation dispatch Active Story dispatch-model",
    });
    expect(workstationCard.className).toContain("bg-surface-container-high");
    expect(
      within(workstationCard).getByRole("heading", { name: "Summary" }),
    ).toBeTruthy();
    expect(
      within(workstationCard).getByRole("heading", {
        name: "Inference attempts",
      }),
    ).toBeTruthy();
  });

  it("renders logical move dispatch rows with move labeling and without inference sections", () => {
    const logicalMoveRequest = workstationRequest("dispatch-logical-move", {
      started_at: "2026-04-08T12:00:00Z",
      transition_id: "logical-move",
      workstation_name: "Logical Move",
      workstation_node_id: "logical-move",
    });

    renderWorkItemWithOperationHistory(
      [{ kind: "logical-move-dispatch", request: logicalMoveRequest }],
      [logicalMoveRequest],
    );

    const operationsRegion = screen.getByRole("region", {
      name: "Work operations",
    });

    const logicalMoveCard = within(operationsRegion).getByRole("article", {
      name: "Logical move dispatch Logical Move dispatch-logical-move",
    });
    expect(logicalMoveCard.className).toContain("bg-surface-container-high");
    expect(within(logicalMoveCard).getByText("Logical Move")).toBeTruthy();
    expect(within(logicalMoveCard).getByText("Move")).toBeTruthy();
    expect(
      within(logicalMoveCard).queryByRole("heading", {
        name: "Summary",
      }),
    ).toBeNull();
    expect(
      within(logicalMoveCard).queryByRole("heading", {
        name: "Inference attempts",
      }),
    ).toBeNull();
  });
});
