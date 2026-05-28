import type { Node, NodeProps } from "@xyflow/react";

import type { DashboardPlaceRef } from "../../../api/dashboard/types";
import { cn } from "../../../lib/cn";
import { getActivityGraphMessages } from "../messages/activity-graph";
import type { ActivityGraphNodeHandle } from "./current-activity-node-shell";
import { ActivityGraphNodeShell } from "./current-activity-node-shell";
import { GraphSemanticIcon } from "./graph-semantic-icon";

export interface WorkerNodeData extends Record<string, unknown> {
  activeFlow: boolean;
  factoryGraphNodeId?: string;
  handles: ActivityGraphNodeHandle[];
  kind: "worker";
  locale?: string;
  muted: boolean;
  place: DashboardPlaceRef;
}

export type CurrentActivityWorkerNode = Node<WorkerNodeData, "worker">;

export function WorkerNodeView({ data }: NodeProps<CurrentActivityWorkerNode>) {
  const messages = getActivityGraphMessages(data.locale);
  const workerName =
    data.place.state_value ??
    data.factoryGraphNodeId?.replace(/^worker:/, "") ??
    data.place.place_id.replace(/^worker:/, "");
  const label = `worker:${workerName}`;
  const workerLabel = messages.graphSemanticIconLabel("worker");

  return (
    <ActivityGraphNodeShell
      className={cn(
        "justify-center border-af-border-strong bg-af-surface text-left text-af-text",
        data.activeFlow && "border-af-success-border shadow-af-success-chip",
        data.muted && "opacity-[0.45]",
      )}
      handles={data.handles}
      nodeType="worker"
    >
      <div className="grid min-w-0 gap-0.5 overflow-hidden">
        <span
          aria-label={label}
          className="flex min-w-0 items-center gap-1.5 overflow-hidden"
          data-worker-label-zone
          role="img"
          title={label}
        >
          <span className="sr-only">{label}</span>
          <GraphSemanticIcon
            className="h-3.5 w-3.5 shrink-0 text-af-worker"
            kind="worker"
            label={workerLabel}
            locale={data.locale}
          />
          <span className="grid min-w-0 gap-px overflow-hidden">
            <span className="block min-w-0 overflow-hidden text-ellipsis whitespace-nowrap text-[0.62rem] font-bold uppercase leading-none text-af-worker-text">
              {workerLabel}
            </span>
            <strong className="block min-w-0 truncate whitespace-nowrap font-mono text-[0.8rem] font-bold leading-tight text-af-text">
              {workerName}
            </strong>
          </span>
        </span>
      </div>
    </ActivityGraphNodeShell>
  );
}
