import type { Node, NodeProps } from "@xyflow/react";

import type { DashboardPlaceRef } from "../../../api/dashboard/types";
import { cn } from "../../../lib/cn";
import { getActivityGraphMessages } from "../messages/activity-graph";
import type { ActivityGraphNodeHandle } from "./current-activity-node-shell";
import { ActivityGraphNodeShell } from "./current-activity-node-shell";
import { GraphSemanticIcon } from "./graph-semantic-icon";

export interface WorkTypeNodeData extends Record<string, unknown> {
  activeFlow: boolean;
  factoryGraphNodeId?: string;
  handles: ActivityGraphNodeHandle[];
  kind: "work-type";
  locale?: string;
  muted: boolean;
  place: DashboardPlaceRef;
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

  return (
    <ActivityGraphNodeShell
      className={cn(
        "justify-center border-dashed border-af-info-border bg-af-info-surface text-left text-af-text",
        data.activeFlow && "border-af-info shadow-af-info-chip",
        data.muted && "opacity-[0.45]",
      )}
      handles={data.handles}
      nodeType="workType"
    >
      <div className="grid min-w-0 gap-0.5 overflow-hidden">
        <span
          aria-label={label}
          className="flex min-w-0 items-center gap-1.5 overflow-hidden"
          data-work-type-label-zone
          role="img"
          title={label}
        >
          <span className="sr-only">{label}</span>
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
      </div>
    </ActivityGraphNodeShell>
  );
}
