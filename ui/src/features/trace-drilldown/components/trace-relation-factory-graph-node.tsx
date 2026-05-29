import type { NodeProps } from "@xyflow/react";

import { GraphNodeButton } from "../../../components/ui/graph-node-button";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
} from "../../../components/ui/dashboard-typography";
import { cn } from "../../../lib/cn";
import {
  ActivityGraphNodeBadge,
  activityGraphNodeTitleClassName,
} from "../../flowchart/components/current-activity-node-chrome";
import {
  ActivityGraphNodeShell,
  type PlaceNodeType,
} from "../../flowchart/components/current-activity-node-shell";
import { GraphSemanticIcon } from "../../flowchart/components/graph-semantic-icon";
import type { FactoryGraphNodeKind } from "../../factory-graph-editor/lib/factory-graph-draft-types";
import type { GraphSemanticIconKind } from "../../flowchart/components/graph-semantic-icon";
import type { TraceRelationFlowNode } from "../lib/trace-relation-factory-graph-flow";
import { getTraceDrilldownMessages } from "../messages/trace-drilldown";

const RELATION_NODE_ACTIVE_CLASS =
  "hover:border-af-accent-border hover:bg-af-accent-surface";
const RELATION_NODE_BADGE_CLASS =
  "inline-flex rounded-full border px-2 py-0.5 text-[0.68rem] font-semibold uppercase tracking-[0.08em]";
const RELATION_NODE_CLASS =
  "min-w-0 w-full justify-start overflow-hidden text-left shadow-af-card";
const RELATION_STATE_BADGE_DANGER_CLASS =
  "border-af-danger-border bg-af-danger-surface text-af-danger-text";
const RELATION_STATE_BADGE_SUCCESS_CLASS =
  "border-af-success-border bg-af-success-surface text-af-success-text";
const RELATION_STATE_BADGE_WARNING_CLASS =
  "border-af-warning-border bg-af-warning-surface text-af-warning-text";
const RELATION_NODE_TONE_DEFAULT_CLASS = "border-af-border bg-af-surface";
const RELATION_NODE_TONE_DANGER_CLASS =
  "border-af-danger-border bg-af-danger-surface";
const RELATION_NODE_TONE_SUCCESS_CLASS =
  "border-af-success-border bg-af-success-surface";
const RELATION_NODE_TONE_WARNING_CLASS =
  "border-af-warning-border bg-af-warning-surface";

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
            <span
              className={cn(
                RELATION_NODE_BADGE_CLASS,
                "border-af-info-border bg-af-info-surface text-af-info",
                DASHBOARD_SUPPORTING_LABEL_CLASS,
              )}
              key={relationType}
            >
              {messages.localizeRelationType(relationType)}
            </span>
          ))}
          {data.relationStates.slice(0, 1).map((relationState) => (
            <span
              className={cn(
                RELATION_NODE_BADGE_CLASS,
                relationStateBadgeClassName(relationState),
                DASHBOARD_SUPPORTING_LABEL_CLASS,
              )}
              key={relationState}
            >
              {messages.localizeRelationState(relationState)}
            </span>
          ))}
        </div>
        <p
          className={cn(
            "m-0 [overflow-wrap:anywhere]",
            activityGraphNodeTitleClassName("text-sm"),
            DASHBOARD_BODY_TEXT_CLASS,
          )}
          data-factory-entity-title
          title={data.workID ?? data.displayLabel}
        >
          {data.displayLabel}
        </p>
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

function relationShellNodeType(kind: FactoryGraphNodeKind): PlaceNodeType | "workstation" {
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
      return "text-af-success";
    case "worker":
      return "text-af-info";
    case "workstation":
      return "text-af-text";
    case "work-type":
      return "text-af-info";
    case "work-state":
      return "text-af-text-muted";
  }
}

function relationStateBadgeClassName(relationState: string): string {
  const tone = relationStateToneClassName(relationState);
  if (tone === "danger") {
    return RELATION_STATE_BADGE_DANGER_CLASS;
  }
  if (tone === "success") {
    return RELATION_STATE_BADGE_SUCCESS_CLASS;
  }
  return RELATION_STATE_BADGE_WARNING_CLASS;
}

function relationStateToneClassName(relationState: string): "danger" | "success" | "warning" {
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
    return RELATION_NODE_TONE_DEFAULT_CLASS;
  }

  const tone = relationStateToneClassName(primaryState);
  if (tone === "danger") {
    return RELATION_NODE_TONE_DANGER_CLASS;
  }
  if (tone === "success") {
    return RELATION_NODE_TONE_SUCCESS_CLASS;
  }

  return RELATION_NODE_TONE_WARNING_CLASS;
}
