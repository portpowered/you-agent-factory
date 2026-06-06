import type { NodeProps } from "@xyflow/react";

import { cn } from "../../../lib/cn";
import {
  ActivityGraphNodeBadge,
  activityGraphNodeTitleClassName,
} from "../../flowchart/components/current-activity-node-chrome";
import { ActivityGraphNodeShell } from "../../flowchart/components/current-activity-node-shell";
import { GraphSemanticIcon } from "../../flowchart/components/graph-semantic-icon";
import type { TraceDispatchFlowNode } from "../lib/trace-dispatch-factory-graph-flow";

function TraceDispatchFactoryGraphNode({
  data,
}: NodeProps<TraceDispatchFlowNode>) {
  return (
    <ActivityGraphNodeShell
      className="min-w-0 w-full justify-start overflow-hidden border-primary bg-primary-container text-left shadow-none"
      handles={data.connectionAnchors.map((handle) => ({
        ...handle,
        hidden: true,
      }))}
      nodeType="workstation"
    >
      <div className="grid h-full min-w-0 content-start gap-2">
        <div className="flex items-start justify-between gap-2">
          <div className="flex min-w-0 items-center gap-2 overflow-hidden">
            <span
              className="flex min-h-5 shrink-0 items-center"
              data-factory-entity-semantic-icon
              title={data.kindLabel}
            >
              <GraphSemanticIcon
                className="h-4 w-4 text-on-surface"
                kind="workstation"
                label={data.kindLabel}
              />
            </span>
            <ActivityGraphNodeBadge weight="label">
              {data.kindLabel}
            </ActivityGraphNodeBadge>
          </div>
        </div>
        <p
          className={cn(
            "m-0 [overflow-wrap:anywhere]",
            activityGraphNodeTitleClassName("font-mono text-sm"),
          )}
          data-factory-entity-title
          title={data.displayLabel}
        >
          {data.displayLabel}
        </p>
      </div>
    </ActivityGraphNodeShell>
  );
}

export const TRACE_DISPATCH_FACTORY_GRAPH_NODE_TYPES = {
  factoryEntity: TraceDispatchFactoryGraphNode,
};
