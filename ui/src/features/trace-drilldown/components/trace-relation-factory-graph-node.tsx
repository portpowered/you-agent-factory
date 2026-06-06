import type { NodeProps } from "@xyflow/react";
import { DashboardText } from "../../../components/ui";
import { GraphNodeButton } from "../../../components/ui/graph-node-button";
import { cn } from "../../../lib/cn";
import type { FactoryGraphNodeKind } from "../../factory-graph-editor/lib/factory-graph-draft-types";
import {
  activityGraphNodeSurfaceClassName,
  activityGraphNodeTitleClassName,
} from "../../flowchart/components/current-activity-node-chrome";
import {
  ActivityGraphNodeShell,
  type PlaceNodeType,
} from "../../flowchart/components/current-activity-node-shell";
import type { TraceRelationFlowNode } from "../lib/trace-relation-factory-graph-flow";

const RELATION_NODE_ACTIVE_CLASS =
  "hover:border-primary hover:bg-primary-container";
const RELATION_NODE_SELECTED_CLASS =
  "border-primary bg-primary-container shadow-af-accent-selected";
const RELATION_NODE_CLASS =
  "min-w-0 w-full justify-start overflow-hidden text-left shadow-af-card";

function TraceRelationFactoryGraphNode({
  data,
}: NodeProps<TraceRelationFlowNode>) {
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
    const workID = data.workID;
    return (
      <GraphNodeButton
        aria-label={data.displayLabel}
        className="nodrag nopan h-full w-full"
        onClick={() => data.onSelectWorkID?.(workID)}
        title={workID}
      >
        {content}
      </GraphNodeButton>
    );
  }

  return content;
}

export const TRACE_RELATION_FACTORY_GRAPH_NODE_TYPES = {
  factoryEntity: TraceRelationFactoryGraphNode,
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
