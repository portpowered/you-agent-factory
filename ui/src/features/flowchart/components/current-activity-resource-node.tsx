import type { Node, NodeProps } from "@xyflow/react";

import type { DashboardPlaceRef } from "../../../api/dashboard/types";
import { cn } from "../../../lib/cn";
import { getActivityGraphMessages } from "../messages/activity-graph";
import { ActivityGraphNodeBadge } from "./current-activity-node-chrome";
import type { ActivityGraphNodeHandle } from "./current-activity-node-shell";
import { ActivityGraphNodeShell } from "./current-activity-node-shell";
import { GraphSemanticIcon } from "./graph-semantic-icon";

export interface ResourceNodeData extends Record<string, unknown> {
  activeFlow: boolean;
  factoryGraphNodeId?: string;
  handles: ActivityGraphNodeHandle[];
  kind: "resource";
  locale?: string;
  muted: boolean;
  place: DashboardPlaceRef;
  tokenCount: number;
}

export type CurrentActivityResourceNode = Node<ResourceNodeData, "resource">;

function resourceName(place: DashboardPlaceRef): string {
  if (typeof place.type_id === "string" && place.type_id.trim().length > 0) {
    return place.type_id;
  }
  return place.place_id.replace(/:available$/, "");
}

export function ResourceNodeView({
  data,
}: NodeProps<CurrentActivityResourceNode>) {
  const messages = getActivityGraphMessages(data.locale);
  const label = resourceName(data.place);
  const resourceLabel = messages.graphSemanticIconLabel("resource");

  return (
    <ActivityGraphNodeShell
      className={cn(
        "justify-center border-af-border-strong bg-af-surface text-left text-af-text",
        data.activeFlow && "border-af-success-border shadow-af-success-chip",
        data.muted && "opacity-[0.45]",
      )}
      handles={data.handles}
      nodeType="resource"
    >
      <div className="flex min-w-0 w-full flex-col overflow-hidden">
        <span
          aria-label={label}
          className="grid h-6 max-h-6 min-w-0 grid-cols-[auto_minmax(0,1fr)] items-center gap-1.5 overflow-hidden"
          data-resource-label-zone
          role="img"
        >
          <span
            className="flex min-h-4 shrink-0 items-center"
            title={resourceLabel}
          >
            <GraphSemanticIcon
              className="h-3.5 w-3.5 text-af-success"
              kind="resource"
              label={resourceLabel}
              locale={data.locale}
            />
          </span>
          <span className="flex min-w-0 overflow-hidden" title={label}>
            <span
              className="block min-w-0 overflow-hidden truncate whitespace-nowrap font-mono text-[0.76rem] font-bold leading-[0.82rem] text-af-text"
              data-resource-name
              title={label}
            >
              {label}
            </span>
          </span>
        </span>
        <span
          className="flex min-h-5 w-full shrink-0 items-center justify-start overflow-hidden"
          data-resource-token-zone
          title={label}
        >
          <ActivityGraphNodeBadge
            aria-label={messages.tokenCountLabel(data.place, data.tokenCount)}
            className="w-fit"
            data-resource-token-count
            role="status"
          >
            {data.tokenCount}
          </ActivityGraphNodeBadge>
        </span>
      </div>
    </ActivityGraphNodeShell>
  );
}
