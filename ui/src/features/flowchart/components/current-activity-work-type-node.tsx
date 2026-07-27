import type { Node, NodeProps } from "@xyflow/react";

import type { DashboardPlaceRef } from "../../../api/dashboard/types";
import { cn } from "../../../lib/cn";
import { GraphNodeButton } from "@you-agent-factory/components/graphs";
import { getWorkflowActivityShellMessages } from "../../workflow-activity/messages/activity-shell";
import { currentActivityGraphNodeHoverClassName } from "../lib/current-activity-graph-hover";
import { getActivityGraphMessages } from "../messages/activity-graph";
import {
  ActivityGraphNodeBadge,
  activityGraphNodeSurfaceClassName,
} from "./current-activity-node-chrome";
import type { ActivityGraphNodeHandle } from "./current-activity-node-shell";
import { ActivityGraphNodeShell } from "./current-activity-node-shell";
import { GraphSemanticIcon } from "./graph-semantic-icon";

const WORK_TYPE_CONTENT_CONTAINER_CLASSNAME =
  "grid min-w-0 gap-0.5 overflow-hidden";

export interface WorkTypeNodeData extends Record<string, unknown> {
  activeFlow: boolean;
  factoryGraphNodeId?: string;
  handles: ActivityGraphNodeHandle[];
  isDefaultWorkType?: boolean;
  kind: "work-type";
  locale?: string;
  muted: boolean;
  onSelectWorkType?: (workTypeName: string) => void;
  place: DashboardPlaceRef;
  selectedWorkType?: boolean;
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
  const activityGraphMessages = getActivityGraphMessages(data.locale);
  const shellMessages = getWorkflowActivityShellMessages(data.locale);
  const name = workTypeName(data.place);
  const label = `work-type:${name}`;
  const workTypeLabel =
    activityGraphMessages.graphSemanticIconLabel("work-type");
  const selectable = data.onSelectWorkType !== undefined;

  const content = (
    <span
      aria-hidden={selectable ? true : undefined}
      className="flex min-w-0 items-center gap-1.5 overflow-hidden"
      data-work-type-label-zone
      {...(selectable
        ? {}
        : {
            "aria-label": label,
            role: "img" as const,
          })}
      title={data.validationMessage ?? label}
    >
      {selectable ? null : <span className="sr-only">{label}</span>}
      <GraphSemanticIcon
        className="h-3.5 w-3.5 shrink-0 text-info"
        kind="work-type"
        label={workTypeLabel}
        locale={data.locale}
      />
      <span className="grid min-w-0 gap-px overflow-hidden">
        <span className="flex min-w-0 items-center gap-1 overflow-hidden">
          <span className="block min-w-0 overflow-hidden text-ellipsis whitespace-nowrap text-[0.62rem] font-bold uppercase leading-none text-info">
            {workTypeLabel}
          </span>
          {data.isDefaultWorkType ? (
            <ActivityGraphNodeBadge
              className="max-w-full shrink"
              role="status"
              tone="info"
              weight="label"
            >
              {activityGraphMessages.defaultWorkTypeLabel}
            </ActivityGraphNodeBadge>
          ) : null}
        </span>
        <strong className="block min-w-0 truncate whitespace-nowrap font-mono text-[0.8rem] font-bold leading-tight text-on-surface">
          {name}
        </strong>
      </span>
    </span>
  );

  return (
    <ActivityGraphNodeShell
      className={cn(
        activityGraphNodeSurfaceClassName("info"),
        "justify-center border-dashed text-left text-on-surface",
        currentActivityGraphNodeHoverClassName({
          activeFlow: data.activeFlow,
          muted: data.muted,
          selected: data.selectedWorkType,
          validationError: data.validationError,
        }),
        data.activeFlow && "border-info shadow-af-info-chip",
        data.muted && "opacity-[0.45]",
        data.validationError &&
          "ring-2 ring-af-danger-border motion-safe:animate-pulse",
        data.selectedWorkType &&
          !data.validationError &&
          "border-primary shadow-af-accent-selected",
      )}
      handles={data.handles}
      nodeType="workType"
    >
      {selectable ? (
        <GraphNodeButton
          aria-invalid={data.validationError ? true : undefined}
          aria-label={shellMessages.selectWorkTypeLabel(name)}
          aria-pressed={data.selectedWorkType}
          className={WORK_TYPE_CONTENT_CONTAINER_CLASSNAME}
          data-selected-work-type={data.selectedWorkType ? "true" : undefined}
          onClick={(event) => {
            event.stopPropagation();
            data.onSelectWorkType?.(name);
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
