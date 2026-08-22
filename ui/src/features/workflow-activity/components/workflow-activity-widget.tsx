import type { ReactNode } from "react";

import type { DashboardSnapshot } from "../../../api/dashboard/types";
import type { DashboardSelection } from "../../current-selection/base/state/selection-types";
import type { CurrentActivityImportController } from "../hooks/current-activity-import-controller";
import type { WorkflowActivityBentoCardState } from "../hooks/workflow-activity-card-state";
import { WorkflowActivityBentoCard } from "./workflow-activity-bento-card";

export interface WorkflowActivityWidgetProps {
  headerAction?: ReactNode;
  importController: CurrentActivityImportController;
  locale?: string;
  now: number;
  onCardStateChange?: (state: WorkflowActivityBentoCardState) => void;
  onDocAdded?: (targetPath: string) => void;
  onDirtyStateChange?: (isDirty: boolean) => void;
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
  restoredCardState?: WorkflowActivityBentoCardState;
  widgetInstanceID?: string;
}

export function WorkflowActivityWidget({
  headerAction,
  importController,
  locale,
  now,
  onCardStateChange,
  onDocAdded,
  onDirtyStateChange,
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
  restoredCardState,
  widgetInstanceID,
}: WorkflowActivityWidgetProps) {
  return (
    <WorkflowActivityBentoCard
      headerAction={headerAction}
      importController={importController}
      locale={locale}
      now={now}
      onCardStateChange={onCardStateChange}
      onDocAdded={onDocAdded}
      onDirtyStateChange={onDirtyStateChange}
      onNodeRemovedFromDraft={onNodeRemovedFromDraft}
      selection={selection}
      snapshot={snapshot}
      restoredCardState={restoredCardState}
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
