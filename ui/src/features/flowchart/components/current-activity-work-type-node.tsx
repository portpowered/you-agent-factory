import type { Node, NodeProps } from "@xyflow/react";

import type { DashboardPlaceRef } from "../../../api/dashboard/types";
import { GraphNodeButton } from "../../../components/ui/graph-node-button";
import { cn } from "../../../lib/cn";
import { getActivityGraphMessages } from "../messages/activity-graph";
import type { ActivityGraphNodeHandle } from "./current-activity-node-shell";
import { ActivityGraphNodeShell } from "./current-activity-node-shell";
import { GraphSemanticIcon } from "./graph-semantic-icon";

const WORK_TYPE_CONTENT_CONTAINER_CLASSNAME =
  "grid min-w-0 gap-0.5 overflow-hidden";

export interface WorkTypeNodeData extends Record<string, unknown> {
  activeFlow: boolean;
  factoryGraphNodeId?: string;
  handles: ActivityGraphNodeHandle[];
  kind: "work-type";
  locale?: string;
  muted: boolean;
  onSelectGraphNode?: (nodeId: string) => void;
  place: DashboardPlaceRef;
  selectedGraphNode?: boolean;
  validationError?: boolean;
  validationMessage?: string;
}

export type CurrentActivityWorkTypeNode = Node<WorkTypeNodeData, "workType">;

function workTypeName(place: DashboardPlaceRef): string {
  if (
    typeof place.state_value === "string" &&
    place.state_value.trim().length > 0
  ) {
    return place.state_value;
  }
  return place.place_id.replace(/^work-type:/, "");
}

export function WorkTypeNodeView({
  data,
}: NodeProps<CurrentActivityWorkTypeNode>) {
  const messages = getActivityGraphMessages(data.locale);
  const name = workTypeName(data.place);
  const label = `work-type:${name}`;
  const workTypeLabel = messages.graphSemanticIconLabel("work-type");

  const content = (
    <span
      aria-hidden={data.onSelectGraphNode ? true : undefined}
      className="flex min-w-0 items-center gap-1.5 overflow-hidden"
      data-work-type-label-zone
      {...(data.onSelectGraphNode
        ? {}
        : {
            "aria-label": label,
            role: "img" as const,
          })}
      title={data.validationMessage ?? label}
    >
      {data.onSelectGraphNode ? null : <span className="sr-only">{label}</span>}
      <GraphSemanticIcon
        className="h-3.5 w-3.5 shrink-0 text-af-info"
        kind="work-type"
        label={workTypeLabel}
        locale={data.locale}
      />
      <span className="grid min-w-0 gap-px overflow-hidden">
        <span className="block min-w-0 overflow-hidden text-ellipsis whitespace-nowrap text-[0.62rem] font-bold uppercase leading-none text-af-info">
          {workTypeLabel}
        </span>
        <strong className="block min-w-0 truncate whitespace-nowrap font-mono text-[0.8rem] font-bold leading-tight text-af-text">
          {name}
        </strong>
      </span>
    </span>
  );

  return (
    <ActivityGraphNodeShell
      className={cn(
        "justify-center border-dashed border-af-info-border bg-af-info-surface text-left text-af-text",
        data.activeFlow && "border-af-info shadow-af-info-chip",
        data.muted && "opacity-[0.45]",
        data.validationError &&
          "ring-2 ring-af-danger-border motion-safe:animate-pulse",
        data.selectedGraphNode &&
          !data.validationError &&
          "border-af-accent-border shadow-af-accent-selected",
      )}
      handles={data.handles}
      nodeType="workType"
    >
      {data.onSelectGraphNode && data.factoryGraphNodeId ? (
        <GraphNodeButton
          aria-invalid={data.validationError ? true : undefined}
          aria-label={data.validationMessage ?? label}
          aria-pressed={data.selectedGraphNode}
          className={WORK_TYPE_CONTENT_CONTAINER_CLASSNAME}
          onClick={(event) => {
            event.stopPropagation();
            data.onSelectGraphNode?.(data.factoryGraphNodeId ?? label);
          }}
        >
          {content}
        </GraphNodeButton>
      ) : (
        <div className={WORK_TYPE_CONTENT_CONTAINER_CLASSNAME}>{content}</div>
      )}
    </ActivityGraphNodeShell>
  );
}
