import type { NodeProps } from "@xyflow/react";
import { DashboardText } from "../../../components/ui";
import { GraphNodeButton } from "../../../components/ui/graph-node-button";
import { cn } from "../../../lib/cn";
import type { FactoryGraphNodeKind } from "../../factory-graph-editor/lib/factory-graph-draft-types";
import {
  ActivityGraphNodeBadge,
  activityGraphNodeSurfaceClassName,
  activityGraphNodeTitleClassName,
} from "../../flowchart/components/current-activity-node-chrome";
import {
  ActivityGraphNodeShell,
  type PlaceNodeType,
} from "../../flowchart/components/current-activity-node-shell";
import type { GraphSemanticIconKind } from "../../flowchart/components/graph-semantic-icon";
import { GraphSemanticIcon } from "../../flowchart/components/graph-semantic-icon";
import type { TraceRelationFlowNode } from "../lib/trace-relation-factory-graph-flow";
import { getTraceDrilldownMessages } from "../messages/trace-drilldown";

const RELATION_NODE_ACTIVE_CLASS =
  "hover:border-primary hover:bg-primary-container";
const RELATION_NODE_CLASS =
  "min-w-0 w-full justify-start overflow-hidden text-left shadow-af-card";

function TraceRelationFactoryGraphNode({
  data,
}: NodeProps<TraceRelationFlowNode>) {
  const messages = getTraceDrilldownMessages(data.locale);
  const shellClassName = cn(
    RELATION_NODE_CLASS,
    relationNodeToneClassName(data.relationStates),
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
      <div className="grid h-full min-w-0 content-start gap-2">
        <div className="flex flex-wrap items-center gap-1.5">
          <span
            className="flex min-h-5 shrink-0 items-center"
            data-factory-entity-semantic-icon
            title={data.kindLabel}
          >
            <GraphSemanticIcon
              className={cn("h-4 w-4", semanticIconClassName(data.kind))}
              kind={semanticIconKind(data.kind)}
              label={data.kindLabel}
            />
          </span>
          <ActivityGraphNodeBadge weight="label">
            {data.kindLabel}
          </ActivityGraphNodeBadge>
          {data.relationTypes.slice(0, 1).map((relationType) => (
            <ActivityGraphNodeBadge
              key={relationType}
              tone="info"
              weight="label"
            >
              {messages.localizeRelationType(relationType)}
            </ActivityGraphNodeBadge>
          ))}
          {data.relationStates.slice(0, 1).map((relationState) => (
            <ActivityGraphNodeBadge
              key={relationState}
              tone={relationStateToneClassName(relationState)}
              weight="label"
            >
              {messages.localizeRelationState(relationState)}
            </ActivityGraphNodeBadge>
          ))}
        </div>
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

function semanticIconKind(kind: FactoryGraphNodeKind): GraphSemanticIconKind {
  switch (kind) {
    case "resource":
      return "resource";
    case "worker":
      return "active-work";
    case "workstation":
      return "workstation";
    case "work-type":
      return "constraint";
    case "work-state":
      return "queue";
  }
}

function semanticIconClassName(kind: FactoryGraphNodeKind): string {
  switch (kind) {
    case "resource":
      return "text-success";
    case "worker":
      return "text-info";
    case "workstation":
      return "text-on-surface";
    case "work-type":
      return "text-info";
    case "work-state":
      return "text-on-surface-variant";
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
