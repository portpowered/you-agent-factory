import type {
  DashboardActiveExecution,
  DashboardSnapshot,
  DashboardWorkItemRef,
} from "../../api/dashboard/types";
import type { DashboardSelection } from "../current-selection";
import { WorkflowActivityBentoCard } from "./workflow-activity-bento-card";

export interface WorkflowActivityWidgetProps {
  now: number;
  onFactoryActivated?: () => void;
  onSelectStateNode: (placeId: string) => void;
  onSelectWorkItem: (
    dispatchId: string,
    nodeId: string,
    execution: DashboardActiveExecution,
    workItem: DashboardWorkItemRef,
  ) => void;
  onSelectWorkstation: (nodeId: string) => void;
  selection: DashboardSelection | null;
  snapshot: DashboardSnapshot;
}

export function WorkflowActivityWidget({
  now,
  onFactoryActivated,
  onSelectStateNode,
  onSelectWorkItem,
  onSelectWorkstation,
  selection,
  snapshot,
}: WorkflowActivityWidgetProps) {
  return (
    <WorkflowActivityBentoCard
      now={now}
      onFactoryActivated={onFactoryActivated}
      selection={selection}
      snapshot={snapshot}
      onSelectWorkItem={onSelectWorkItem}
      onSelectStateNode={onSelectStateNode}
      onSelectWorkstation={onSelectWorkstation}
    />
  );
}
