import type {
  DashboardActiveExecution,
  DashboardSnapshot,
  DashboardWorkItemRef,
} from "../../api/dashboard/types";
import type { DashboardSelection } from "../current-selection";
import type { CurrentActivityImportController } from "./current-activity-import-controller";
import { WorkflowActivityBentoCard } from "./workflow-activity-bento-card";

export interface WorkflowActivityWidgetProps {
  importController: CurrentActivityImportController;
  now: number;
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
  importController,
  now,
  onSelectStateNode,
  onSelectWorkItem,
  onSelectWorkstation,
  selection,
  snapshot,
}: WorkflowActivityWidgetProps) {
  return (
    <WorkflowActivityBentoCard
      importController={importController}
      now={now}
      selection={selection}
      snapshot={snapshot}
      onSelectWorkItem={onSelectWorkItem}
      onSelectStateNode={onSelectStateNode}
      onSelectWorkstation={onSelectWorkstation}
    />
  );
}
