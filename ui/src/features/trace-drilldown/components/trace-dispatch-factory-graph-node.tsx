import type { NodeProps } from "@xyflow/react";

import { DashboardText } from "../../../components/ui";
import { formatTraceOutcome } from "../../../components/ui/formatters";
import { cn } from "../../../lib/cn";
import {
  ActivityGraphNodeBadge,
  activityGraphNodeSurfaceClassName,
  activityGraphNodeTitleClassName,
} from "../../flowchart/components/current-activity-node-chrome";
import { ActivityGraphNodeShell } from "../../flowchart/components/current-activity-node-shell";
import { GraphSemanticIcon } from "../../flowchart/components/graph-semantic-icon";
import type { TraceDispatchFlowNode } from "../lib/trace-dispatch-factory-graph-flow";
import { getTraceDrilldownMessages } from "../messages/trace-drilldown";

const WORKSTATION_NODE_CLASS =
  "min-w-0 w-full justify-start overflow-hidden text-left shadow-af-card";

function TraceDispatchFactoryGraphNode({
  data,
}: NodeProps<TraceDispatchFlowNode>) {
  const messages = getTraceDrilldownMessages(data.locale);

  return (
    <ActivityGraphNodeShell
      className={cn(WORKSTATION_NODE_CLASS, outcomeToneClassName(data.outcome))}
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
          <ActivityGraphNodeBadge
            className="shrink-0"
            tone={outcomeBadgeTone(data.outcome)}
            weight="label"
          >
            {data.outcome
              ? formatTraceOutcome(data.outcome)
              : messages.dispatchPathPendingOutcome}
          </ActivityGraphNodeBadge>
        </div>
        <DashboardText
          className={cn(
            "m-0 [overflow-wrap:anywhere]",
            activityGraphNodeTitleClassName("font-mono text-sm"),
          )}
          data-factory-entity-title
          title={data.displayLabel}
        >
          {data.displayLabel}
        </DashboardText>
        <DashboardText
          className="m-0 text-[0.76rem] text-on-surface-variant [overflow-wrap:anywhere]"
          variant="supporting"
        >
          {messages.dispatchPathInputPrefix}: {data.inputSummary}
        </DashboardText>
        <DashboardText
          className="m-0 text-[0.76rem] text-on-surface-variant [overflow-wrap:anywhere]"
          variant="supporting"
        >
          {messages.dispatchPathOutputPrefix}: {data.outputSummary}
        </DashboardText>
      </div>
    </ActivityGraphNodeShell>
  );
}

export const TRACE_DISPATCH_FACTORY_GRAPH_NODE_TYPES = {
  factoryEntity: TraceDispatchFactoryGraphNode,
};

function outcomeBadgeTone(
  outcome?: string,
): "danger" | "neutral" | "success" | "warning" {
  if (!outcome) {
    return "neutral";
  }

  const normalized = outcome.toUpperCase();
  if (normalized === "FAILED" || normalized === "REJECTED") {
    return "danger";
  }
  if (normalized === "CONTINUE") {
    return "warning";
  }
  return "success";
}

function outcomeToneClassName(outcome?: string): string {
  if (!outcome) {
    return activityGraphNodeSurfaceClassName("neutral");
  }

  const normalized = outcome.toUpperCase();
  if (normalized === "FAILED" || normalized === "REJECTED") {
    return activityGraphNodeSurfaceClassName("danger");
  }
  if (normalized === "CONTINUE") {
    return activityGraphNodeSurfaceClassName("warning");
  }
  return activityGraphNodeSurfaceClassName("success");
}
