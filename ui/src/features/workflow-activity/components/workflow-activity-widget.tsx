import type { DashboardSnapshot } from "../../../api/dashboard/types";
import type { DashboardSelection } from "../../current-selection";
import type { CurrentActivityImportController } from "../current-activity-import-controller";
import { WorkflowActivityBentoCard } from "./workflow-activity-bento-card";

export interface WorkflowActivityWidgetProps {
  importController: CurrentActivityImportController;
  locale?: string;
  now: number;
  onSelectStateNode: (placeId: string) => void;
  onSelectWorkID: (workID: string) => void;
  onSelectWorkstation: (nodeId: string) => void;
  selection: DashboardSelection | null;
  snapshot: DashboardSnapshot;
}

export function WorkflowActivityWidget({
  importController,
  locale,
  now,
  onSelectStateNode,
  onSelectWorkID,
  onSelectWorkstation,
  selection,
  snapshot,
}: WorkflowActivityWidgetProps) {
  return (
    <WorkflowActivityBentoCard
      importController={importController}
      locale={locale}
      now={now}
      selection={selection}
      snapshot={snapshot}
      onSelectWorkID={onSelectWorkID}
      onSelectStateNode={onSelectStateNode}
      onSelectWorkstation={onSelectWorkstation}
    />
  );
}
