import { expect, within } from "storybook/test";

import "../../../../../styles.css";
import type {
  DashboardWorkItemRef,
  DashboardWorkMoveOperation,
} from "../../../../../api/dashboard/types";
import { dashboardWorkstationRequestFixtures } from "../../../../../components/dashboard/fixtures";
import {
  DETAIL_CARD_NOW,
  getSelectedWorkItemFixture,
  workstationRequest,
} from "../../../base/components/detail-card/detail-card-test-helpers";
import { CurrentSelectionLocaleProvider } from "../../../base/components/presentation/current-selection-locale";
import type { SelectedWorkRelationshipGraph } from "../../lib/selected-work-relationship-graph";
import { selectWorkItemExecutionDetails } from "../../state/executionDetails";
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

function buildReadyRelationshipGraph(
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
      label: workItem.display_name ?? workItem.work_id,
      state: workItem.state,
      traceID: workItem.trace_id,
      workID: workItem.work_id,
      workTypeID: workItem.work_type_id,
    },
    status: "ready",
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
          relationshipGraph={buildReadyRelationshipGraph(workItem)}
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
    await expect(
      await within(card).findByText("Durability confirmation: UNCONFIRMED"),
    ).toBeVisible();
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
    await expect(workstationCard.className).toContain(
      "bg-surface-container-low",
    );
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
    const relationshipGraph = await within(card).findByRole("region", {
      name: "Batch relation graph",
    });
    await expect(
      within(relationshipGraph).getByText("Dependency Story"),
    ).toBeVisible();
    await expect(
      within(relationshipGraph).queryByText("Relationship key"),
    ).toBeNull();
  },
};
