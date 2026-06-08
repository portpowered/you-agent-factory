import type { Node, NodeProps } from "@xyflow/react";

import { DashboardText } from "../../../components/ui";
import { cn } from "../../../lib/cn";
import type { FactoryGraphNodeKind } from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";
import {
  activityGraphNodeSurfaceClassName,
  activityGraphNodeTitleClassName,
} from "../../flowchart/components/current-activity-node-chrome";
import { GraphNodeButton } from "./graph-node-button";
import {
  type ActivityGraphNodeHandle,
  ActivityGraphNodeShell,
  type PlaceNodeType,
} from "./graph-node-shell";

export interface WorkRelationNodeData extends Record<string, unknown> {
  connectionAnchors: ActivityGraphNodeHandle[];
  displayLabel: string;
  isSelectedWork: boolean;
  kind: FactoryGraphNodeKind;
  onSelectWorkID?: (workID: string) => void;
  relationStates: string[];
  selectable: boolean;
  workID?: string;
}

export type WorkRelationNode = Node<WorkRelationNodeData, "workRelation">;

const RELATION_NODE_ACTIVE_CLASS =
  "hover:border-primary hover:bg-primary-container";
const RELATION_NODE_SELECTED_CLASS =
  "border-primary bg-primary-container shadow-af-accent-selected";
const RELATION_NODE_CLASS =
  "min-w-0 w-full justify-start overflow-hidden text-left shadow-none";

export function WorkRelationNodeView({ data }: NodeProps<WorkRelationNode>) {
  const shellClassName = cn(
    RELATION_NODE_CLASS,
    data.isSelectedWork
      ? RELATION_NODE_SELECTED_CLASS
      : relationNodeToneClassName(data.relationStates),
    data.selectable ? RELATION_NODE_ACTIVE_CLASS : undefined,
  );
  const content = (
    <ActivityGraphNodeShell
      className={shellClassName}
      handles={data.connectionAnchors.map((handle) => ({
        ...handle,
        hidden: true,
      }))}
      nodeType={relationShellNodeType(data.kind)}
    >
      <div className="grid h-full min-w-0 content-center">
        <DashboardText
          className={cn(
            "m-0 [overflow-wrap:anywhere]",
            activityGraphNodeTitleClassName("text-sm"),
          )}
          data-factory-entity-title
          title={data.workID ?? data.displayLabel}
        >
          {data.displayLabel}
        </DashboardText>
      </div>
    </ActivityGraphNodeShell>
  );

  if (data.selectable && data.workID && data.onSelectWorkID) {
    const { onSelectWorkID, workID } = data;
    return (
      <GraphNodeButton
        aria-label={data.displayLabel}
        className="nodrag nopan h-full w-full"
        onClick={() => onSelectWorkID(workID)}
        title={data.workID}
      >
        {content}
      </GraphNodeButton>
    );
  }

  return content;
}

export const WORK_RELATION_NODE_TYPES = {
  workRelation: WorkRelationNodeView,
};

function relationShellNodeType(
  kind: FactoryGraphNodeKind,
): PlaceNodeType | "workstation" {
  switch (kind) {
    case "work-state":
      return "statePosition";
    case "work-type":
      return "workType";
    case "workstation":
      return "workstation";
    case "resource":
      return "resource";
    case "worker":
      return "worker";
  }
}

function relationStateToneClassName(
  relationState: string,
): "danger" | "success" | "warning" {
  const normalizedState = relationState.trim().toUpperCase();
  if (
    normalizedState === "FAILED" ||
    normalizedState === "FAIL" ||
    normalizedState === "REJECTED"
  ) {
    return "danger";
  }

  if (
    normalizedState === "DONE" ||
    normalizedState === "ACCEPTED" ||
    normalizedState === "COMPLETED"
  ) {
    return "success";
  }

  return "warning";
}

function relationNodeToneClassName(relationStates: string[]): string {
  const primaryState = relationStates[0];
  if (!primaryState) {
    return activityGraphNodeSurfaceClassName("neutral");
  }

  const tone = relationStateToneClassName(primaryState);
  return activityGraphNodeSurfaceClassName(tone);
}
