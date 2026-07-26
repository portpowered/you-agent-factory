import type { ReactNode } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import type { DashboardSelection } from "../../current-selection/base/state/selection-types";
import type { CurrentActivityImportController } from "../hooks/current-activity-import-controller";
import { WorkflowActivityBentoCard } from "./workflow-activity-bento-card";

export interface WorkflowActivityWidgetProps {
  headerAction?: ReactNode;
  importController: CurrentActivityImportController;
  locale?: string;
  now: number;
  onDocAdded?: (targetPath: string) => void;
  onNodeRemovedFromDraft?: (nodeId: string) => void;
  onSelectDoc: (targetPath: string) => void;
  onSelectResource: (resourceName: string) => void;
  onSelectStateNode: (placeId: string) => void;
  onSelectWorkID: (
    workID: string,
    hint?: { dispatchID?: string; nodeID?: string },
  ) => void;
  onSelectWorker: (workerName: string) => void;
  onSelectWorkType: (workTypeName: string) => void;
  onSelectWorkstation: (nodeId: string) => void;
  selection: DashboardSelection | null;
  snapshot: DashboardSnapshot;
  widgetInstanceID?: string;
}

export function WorkflowActivityWidget({
  headerAction,
  importController,
  locale,
  now,
  onDocAdded,
  onNodeRemovedFromDraft,
  onSelectDoc,
  onSelectResource,
  onSelectStateNode,
  onSelectWorkID,
  onSelectWorker,
  onSelectWorkType,
  onSelectWorkstation,
  selection,
  snapshot,
  widgetInstanceID,
}: WorkflowActivityWidgetProps) {
  return (
    <WorkflowActivityBentoCard
      headerAction={headerAction}
      importController={importController}
      locale={locale}
      now={now}
      onDocAdded={onDocAdded}
      onNodeRemovedFromDraft={onNodeRemovedFromDraft}
      selection={selection}
      snapshot={snapshot}
      widgetInstanceID={widgetInstanceID}
      onSelectWorkID={onSelectWorkID}
      onSelectDoc={onSelectDoc}
      onSelectResource={onSelectResource}
      onSelectStateNode={onSelectStateNode}
      onSelectWorker={onSelectWorker}
      onSelectWorkType={onSelectWorkType}
      onSelectWorkstation={onSelectWorkstation}
    />
  );
}
