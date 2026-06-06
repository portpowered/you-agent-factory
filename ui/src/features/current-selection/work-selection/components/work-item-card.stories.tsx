import { expect, within } from "storybook/test";

import "../../../../styles.css";
import type { DashboardWorkMoveOperation } from "../../../../api/dashboard/types";
import { dashboardWorkstationRequestFixtures } from "../../../../components/dashboard/fixtures";
import { CurrentSelectionLocaleProvider } from "../../base/components/current-selection-locale";
import {
  DETAIL_CARD_NOW,
  getSelectedWorkItemFixture,
  workstationRequest,
} from "../../base/components/detail-card-test-helpers";
import { selectWorkItemExecutionDetails } from "../state/executionDetails";
import { WorkItemDetailCard } from "./work-item-card";

export default {
  title: "you-agent-factory/Current Selection/Work Item Detail",
  component: WorkItemDetailCard,
  tags: ["test"],
};

function buildMoveOperation(
  overrides: Partial<DashboardWorkMoveOperation> = {},
): DashboardWorkMoveOperation {
  return {
    event_time: "2026-04-08T14:00:00Z",
    from_place_id: "story:init",
    from_state: "init",
    sequence: 2,
    source: "cli",
    tick: 3,
    to_place_id: "story:review",
    to_state: "review",
    work_id: "work-active-story",
    ...overrides,
  };
}

function SelectedWorkDispatchHistoryStory() {
  const { dispatchID, execution, selectedNode, workItem } =
    getSelectedWorkItemFixture();
  const logicalMoveRequest = workstationRequest("dispatch-logical-move", {
    started_at: "2026-04-08T12:00:00Z",
    transition_id: "logical-move",
    workstation_name: "Logical Move",
    workstation_node_id: "logical-move",
  });
  const request = dashboardWorkstationRequestFixtures.scriptPending;

  return (
    <CurrentSelectionLocaleProvider>
      <div style={{ maxWidth: "720px", padding: "1rem" }}>
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
          onSelectTraceID={() => undefined}
          onSelectWorkID={() => undefined}
          operationHistory={[
            { kind: "operator-move", move: buildMoveOperation() },
            { kind: "workstation", request },
            { kind: "logical-move-dispatch", request: logicalMoveRequest },
          ]}
          selectedNode={selectedNode}
          selection={{
            dispatchId: dispatchID,
            execution,
            kind: "work-item",
            nodeId: selectedNode.node_id,
            workItem,
          }}
          workstationRequests={[request, logicalMoveRequest]}
        />
      </div>
    </CurrentSelectionLocaleProvider>
  );
}

export const DispatchHistoryStandardActions = {
  render: () => <SelectedWorkDispatchHistoryStory />,
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const card = await canvas.findByRole("article", {
      name: "Current selection",
    });
    const operationHistory = await within(card).findByRole("region", {
      name: "Work operations",
    });
    const operatorMoveCard = await within(operationHistory).findByRole(
      "article",
      {
        name: "Operator move init → review",
      },
    );
    const workstationCard = await within(operationHistory).findByRole(
      "article",
      {
        name: "Workstation dispatch Active Story dispatch-review-script-pending",
      },
    );
    const logicalMoveCard = await within(operationHistory).findByRole(
      "article",
      {
        name: "Logical move dispatch Logical Move dispatch-logical-move",
      },
    );

    await expect(
      within(workstationCard).getByRole("button", {
        name: "Select work item Active Story",
      }),
    ).toHaveAttribute("aria-pressed", "true");
    await expect(workstationCard.className).toContain("bg-surface-container-low");
    await expect(operatorMoveCard.className).toContain(
      "bg-surface-container-low",
    );
    await expect(logicalMoveCard.className).toContain(
      "bg-surface-container-low",
    );
    await expect(
      within(operatorMoveCard).queryByRole("heading", {
        name: "Inference attempts",
      }),
    ).toBeNull();
    await expect(
      within(logicalMoveCard).queryByRole("heading", {
        name: "Inference attempts",
      }),
    ).toBeNull();
    await expect(within(workstationCard).getByText("Trace IDs")).toBeVisible();
    await expect(
      within(workstationCard).getByRole("button", {
        name: "trace-active-story (selected)",
      }),
    ).toHaveAttribute("aria-pressed", "true");
  },
};
