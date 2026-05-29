import type { NodeProps } from "@xyflow/react";

import {
  DASHBOARD_BODY_TEXT_CLASS,
} from "../../../components/ui/dashboard-typography";
import { formatTraceOutcome } from "../../../components/ui/formatters";
import { cn } from "../../../lib/cn";
import {
  ActivityGraphNodeBadge,
  activityGraphNodeTitleClassName,
} from "../../flowchart/components/current-activity-node-chrome";
import { ActivityGraphNodeShell } from "../../flowchart/components/current-activity-node-shell";
import { GraphSemanticIcon } from "../../flowchart/components/graph-semantic-icon";
import { getTraceDrilldownMessages } from "../messages/trace-drilldown";
import type { TraceDispatchFlowNode } from "../lib/trace-dispatch-factory-graph-flow";

const WORKSTATION_NODE_CLASS =
  "min-w-0 w-full justify-start overflow-hidden text-left shadow-af-card";
const DISPATCH_NODE_TONE_DEFAULT_CLASS = "border-af-border bg-af-surface";
const DISPATCH_NODE_TONE_DANGER_CLASS =
  "border-af-danger-border bg-af-danger-surface";
const DISPATCH_NODE_TONE_WARNING_CLASS =
  "border-af-warning-border bg-af-warning-surface";
const DISPATCH_NODE_TONE_SUCCESS_CLASS =
  "border-af-success-border bg-af-success-surface";

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
                className="h-4 w-4 text-af-text"
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
        <p
          className={cn(
            "m-0 [overflow-wrap:anywhere]",
            activityGraphNodeTitleClassName("font-mono text-sm"),
            DASHBOARD_BODY_TEXT_CLASS,
          )}
          data-factory-entity-title
          title={data.displayLabel}
        >
          {data.displayLabel}
        </p>
        <p className="m-0 text-[0.76rem] text-af-text-muted [overflow-wrap:anywhere]">
          {messages.dispatchPathInputPrefix}: {data.inputSummary}
        </p>
        <p className="m-0 text-[0.76rem] text-af-text-muted [overflow-wrap:anywhere]">
          {messages.dispatchPathOutputPrefix}: {data.outputSummary}
        </p>
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
    return DISPATCH_NODE_TONE_DEFAULT_CLASS;
  }

  const normalized = outcome.toUpperCase();
  if (normalized === "FAILED" || normalized === "REJECTED") {
    return DISPATCH_NODE_TONE_DANGER_CLASS;
  }
  if (normalized === "CONTINUE") {
    return DISPATCH_NODE_TONE_WARNING_CLASS;
  }
  return DISPATCH_NODE_TONE_SUCCESS_CLASS;
}
