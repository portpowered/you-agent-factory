import type { Node, NodeProps } from "@xyflow/react";
import { GraphNodeButton } from "@you-agent-factory/components/graphs";
import { Text } from "@you-agent-factory/components/primitives";
import { cn } from "../../../lib/cn";
import type { FactoryGraphNodeKind } from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";
import { activityGraphNodeSurfaceClassName } from "../../flowchart/components/current-activity-node-chrome";
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

const RELATION_LABEL_COMPACT_LENGTH = 24;
const RELATION_LABEL_DENSE_LENGTH = 40;
const RELATION_NODE_ACTIVE_CLASS =
  "hover:border-primary hover:bg-primary-container";
const RELATION_NODE_SELECTED_CLASS =
  "border-primary bg-primary-container shadow-af-accent-selected";
const RELATION_NODE_CLASS =
  "min-w-0 w-full justify-start text-left shadow-none";
const RELATION_WORKSTATION_SURFACE_CLASS = cn(
  activityGraphNodeSurfaceClassName("workstation"),
  "border-info-border",
);

export function WorkRelationNodeView({ data }: NodeProps<WorkRelationNode>) {
  const shellClassName = cn(
    RELATION_NODE_CLASS,
    "border-2",
    data.isSelectedWork
      ? RELATION_NODE_SELECTED_CLASS
      : data.selectable
        ? cn(RELATION_WORKSTATION_SURFACE_CLASS, "bg-info-container")
        : relationNodeToneClassName(data.kind, data.relationStates),
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
      <div className="grid h-full min-h-0 min-w-0 content-center">
        <Text
          className={relationNodeLabelClassName(data.displayLabel)}
          data-factory-entity-title
          title={data.workID ?? data.displayLabel}
        >
          {data.displayLabel}
        </Text>
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
    case "doc":
      return "doc";
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

function relationNodeLabelClassName(label: string): string {
  const textSizeClassName =
    label.length > RELATION_LABEL_DENSE_LENGTH
      ? "text-[0.72rem]"
      : label.length > RELATION_LABEL_COMPACT_LENGTH
        ? "text-[0.78rem]"
        : "text-sm";

  return cn(
    "m-0 block min-w-0 break-words font-bold leading-tight text-on-surface [overflow-wrap:anywhere]",
    textSizeClassName,
  );
}

function relationNodeToneClassName(
  kind: FactoryGraphNodeKind,
  relationStates: string[],
): string {
  switch (kind) {
    case "doc":
      return activityGraphNodeSurfaceClassName("neutral");
    case "resource":
      return activityGraphNodeSurfaceClassName("resource");
    case "work-state":
      return activityGraphNodeSurfaceClassName("workState");
    case "workstation":
      return RELATION_WORKSTATION_SURFACE_CLASS;
    case "work-type":
      return RELATION_WORKSTATION_SURFACE_CLASS;
    case "worker":
      break;
  }

  const primaryState = relationStates[0];
  if (!primaryState) {
    return RELATION_WORKSTATION_SURFACE_CLASS;
  }

  const tone = relationStateToneClassName(primaryState);
  return activityGraphNodeSurfaceClassName(tone);
}
