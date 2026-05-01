import type {
  DashboardActiveExecution,
  DashboardSnapshot,
  DashboardWorkItemRef,
} from "../../api/dashboard/types";
import { AgentBentoCard } from "../../components/dashboard/bento";
import type { DashboardSelection } from "../current-selection";
import {
  ReactFlowCurrentActivityCard,
  type CurrentActivitySelection,
} from "./react-flow-current-activity-card";

interface WorkflowActivityBentoCardProps {
  now: number;
  onFactoryActivated?: () => void;
  selection: DashboardSelection | null;
  snapshot: DashboardSnapshot;
  onSelectWorkItem: (
    dispatchId: string,
    nodeId: string,
    execution: DashboardActiveExecution,
    workItem: DashboardWorkItemRef,
  ) => void;
  onSelectStateNode: (placeId: string) => void;
  onSelectWorkstation: (nodeId: string) => void;
}

const GRAPH_PANEL_SHELL_CLASS = "relative h-full min-h-0";

export function WorkflowActivityBentoCard({
  now,
  onFactoryActivated,
  selection,
  snapshot,
  onSelectWorkItem,
  onSelectStateNode,
  onSelectWorkstation,
}: WorkflowActivityBentoCardProps) {
  return (
    <AgentBentoCard title="Factory graph">
      <section className={GRAPH_PANEL_SHELL_CLASS}>
        <ReactFlowCurrentActivityCard
          now={now}
          onFactoryActivated={onFactoryActivated}
          selection={toCurrentActivitySelection(selection)}
          snapshot={snapshot}
          onSelectWorkItem={onSelectWorkItem}
          onSelectStateNode={onSelectStateNode}
          onSelectWorkstation={onSelectWorkstation}
        />
      </section>
    </AgentBentoCard>
  );
}

function toCurrentActivitySelection(
  selection: DashboardSelection | null,
): CurrentActivitySelection | null {
  if (selection?.kind === "workstation-request") {
    return { kind: "node", nodeId: selection.nodeId };
  }

  if (selection?.kind !== "work-item") {
    return selection;
  }

  return {
    kind: "work-item",
    dispatchId: selection.dispatchId ?? "",
    nodeId: selection.nodeId,
    workID: selection.workItem.work_id,
  };
}
