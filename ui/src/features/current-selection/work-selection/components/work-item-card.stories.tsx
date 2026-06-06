import { expect, within } from "storybook/test";

import "../../../../styles.css";
import { dashboardWorkstationRequestFixtures } from "../../../../components/dashboard/fixtures";
import { CurrentSelectionLocaleProvider } from "../../base/components/current-selection-locale";
import {
  DETAIL_CARD_NOW,
  getSelectedWorkItemFixture,
} from "../../base/components/detail-card-test-helpers";
import { selectWorkItemExecutionDetails } from "../state/executionDetails";
import { WorkItemDetailCard } from "./work-item-card";

export default {
  title: "you-agent-factory/Current Selection/Work Item Detail",
  component: WorkItemDetailCard,
  tags: ["test"],
};

function SelectedWorkDispatchHistoryStory() {
  const { dispatchID, execution, selectedNode, workItem } =
    getSelectedWorkItemFixture();

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
          selectedNode={selectedNode}
          selection={{
            dispatchId: dispatchID,
            execution,
            kind: "work-item",
            nodeId: selectedNode.node_id,
            workItem,
          }}
          workstationRequests={[dashboardWorkstationRequestFixtures.scriptPending]}
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
    const dispatchHistory = await within(card).findByRole("region", {
      name: "Workstation dispatches",
    });

    await expect(
      within(dispatchHistory).getByRole("button", {
        name: "Select work item Active Story",
      }),
    ).toHaveAttribute("aria-pressed", "true");
    await expect(within(dispatchHistory).getByText("Trace IDs")).toBeVisible();
    await expect(
      within(dispatchHistory).getByRole("button", {
        name: "trace-active-story (selected)",
      }),
    ).toHaveAttribute("aria-pressed", "true");
  },
};
