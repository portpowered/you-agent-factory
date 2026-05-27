import type { Node, NodeProps } from "@xyflow/react";

import type {
  DashboardActiveExecution,
  DashboardWorkItemRef,
  DashboardWorkstationNode,
} from "../../../api/dashboard/types";
import { GraphNodeButton } from "../../../components/ui/graph-node-button";
import {
  formatDurationFromISO,
  formatWorkItemLabel,
} from "../../../components/ui/formatters";
import { cn } from "../../../lib/cn";
import { getWorkflowActivityShellMessages } from "../../workflow-activity/messages/activity-shell";
import { workstationIconMetadata } from "../lib/workstation-icon-metadata";
import {
  ActivityGraphNodeBadge,
  activityGraphNodeTitleClassName,
} from "./current-activity-node-chrome";
import type { ActivityGraphNodeHandle } from "./current-activity-node-shell";
import { ActivityGraphNodeShell } from "./current-activity-node-shell";
import { GraphSemanticIcon } from "./graph-semantic-icon";

export interface WorkstationNodeData extends Record<string, unknown> {
  active: boolean;
  activeFlow: boolean;
  executions: DashboardActiveExecution[];
  factoryGraphNodeId?: string;
  handles?: ActivityGraphNodeHandle[];
  incomingHandleCount: number;
  kind?: "workstation";
  locale?: string;
  muted: boolean;
  now: number;
  outgoingHandleCount: number;
  selectedWorkID: string | null;
  selectedWorkstation: boolean;
  workstation: DashboardWorkstationNode;
  onSelectWorkstation: (nodeId: string) => void;
  onSelectWorkID: (
    workID: string,
    hint?: { dispatchID?: string; nodeID?: string },
  ) => void;
}

export type CurrentActivityWorkstationNode = Node<
  WorkstationNodeData,
  "workstation"
>;

const WORKSTATION_SUMMARY_DOT_LIMIT = 10;
const WORKSTATION_VISIBLE_WORK_ITEM_LIMIT = 3;
const WORKSTATION_TITLE_COMPACT_LENGTH = 16;
const WORKSTATION_TITLE_DENSE_LENGTH = 28;
const WORK_ITEM_LABEL_COMPACT_LENGTH = 28;
const WORK_ITEM_LABEL_DENSE_LENGTH = 48;

export function WorkstationNodeView({
  data,
}: NodeProps<CurrentActivityWorkstationNode>) {
  const messages = getWorkflowActivityShellMessages(data.locale);
  const semanticIconMetadata = workstationIconMetadata(data.workstation);
  const exhaustionRule = semanticIconMetadata.semanticKind === "exhaustion";
  const selectedWork = data.selectedWorkID !== null;
  const workItemEntries = data.executions.flatMap((execution) =>
    (execution.work_items ?? []).map((workItem) => ({ execution, workItem })),
  );
  const visibleWorkItemEntries = workItemEntries.slice(
    0,
    WORKSTATION_VISIBLE_WORK_ITEM_LIMIT,
  );
  const workstationTitle =
    data.workstation.workstation_name ||
    data.workstation.transition_id ||
    data.workstation.node_id;
  const nodeClassName = cn(
    "min-w-0 w-full justify-start overflow-hidden border-2 bg-af-surface-raised",
    exhaustionRule
      ? "border-dashed border-af-danger-border"
      : "border-af-info-border",
    !exhaustionRule &&
      semanticIconMetadata.semanticKind === "repeater" &&
      "border-double",
    !exhaustionRule &&
      data.active &&
      !data.selectedWorkstation &&
      "border-af-success-border shadow-af-success-chip",
    !exhaustionRule &&
      data.activeFlow &&
      !data.selectedWorkstation &&
      "agent-flow-node--active ring-2 ring-af-success-border",
    data.selectedWorkstation &&
      "border-af-accent-border shadow-af-accent-selected",
    !exhaustionRule &&
      selectedWork &&
      "border-af-info-border shadow-af-info-selected",
    data.muted && "opacity-[0.45]",
  );

  return (
    <ActivityGraphNodeShell
      className={nodeClassName}
      handles={data.handles}
      incomingHandleCount={data.incomingHandleCount}
      nodeType="workstation"
      outgoingHandleCount={data.outgoingHandleCount}
    >
      {exhaustionRule ? (
        <ExhaustionRuleNodeButton
          data={data}
          messages={messages}
          workstationTitle={workstationTitle}
        />
      ) : (
        <ActiveWorkstationNodeContent
          data={data}
          messages={messages}
          semanticIconMetadata={semanticIconMetadata}
          selectedWork={selectedWork}
          visibleWorkItemEntries={visibleWorkItemEntries}
          workItemEntries={workItemEntries}
          workstationTitle={workstationTitle}
        />
      )}
    </ActivityGraphNodeShell>
  );
}

function ExhaustionRuleNodeButton({
  data,
  messages,
  workstationTitle,
}: {
  data: WorkstationNodeData;
  messages: ReturnType<typeof getWorkflowActivityShellMessages>;
  workstationTitle: string;
}) {
  const semanticIconMetadata = workstationIconMetadata(data.workstation);

  return (
    <GraphNodeButton
      aria-label={messages.selectExhaustionRuleLabel(workstationTitle)}
      aria-pressed={data.selectedWorkstation}
      className="flex h-full min-w-0 w-full items-center gap-2 overflow-hidden"
      data-selected-workstation={data.selectedWorkstation ? "true" : undefined}
      data-workstation-kind={semanticIconMetadata.semanticKind}
      onClick={() => data.onSelectWorkstation(data.workstation.node_id)}
      title={workstationTitle}
    >
      <span
        className="flex min-h-4 items-center"
        data-workstation-semantic-icon
        title={semanticIconMetadata.label}
      >
        <GraphSemanticIcon
          className={cn("h-4 w-4", semanticIconMetadata.className)}
          kind={semanticIconMetadata.iconKind}
          label={semanticIconMetadata.label}
        />
      </span>
      <span
        className="block min-w-0 truncate whitespace-nowrap font-mono text-[0.74rem] font-bold leading-tight text-af-text"
        data-workstation-title
      >
        {workstationTitle}
      </span>
    </GraphNodeButton>
  );
}

function ActiveWorkstationNodeContent({
  data,
  messages,
  semanticIconMetadata,
  selectedWork,
  visibleWorkItemEntries,
  workItemEntries,
  workstationTitle,
}: {
  data: WorkstationNodeData;
  messages: ReturnType<typeof getWorkflowActivityShellMessages>;
  semanticIconMetadata: ReturnType<typeof workstationIconMetadata>;
  selectedWork: boolean;
  visibleWorkItemEntries: Array<{
    execution: DashboardActiveExecution;
    workItem: DashboardWorkItemRef;
  }>;
  workItemEntries: Array<{
    execution: DashboardActiveExecution;
    workItem: DashboardWorkItemRef;
  }>;
  workstationTitle: string;
}) {
  return (
    <div
      className="grid h-full min-w-0 grid-rows-[auto_1fr_auto]"
      data-active={data.active ? "true" : undefined}
      data-selected-work={selectedWork ? "true" : undefined}
      data-selected-workstation={data.selectedWorkstation ? "true" : undefined}
      data-workstation-kind={semanticIconMetadata.semanticKind}
    >
      <GraphNodeButton
        aria-label={messages.selectWorkstationLabel(workstationTitle)}
        aria-pressed={data.selectedWorkstation}
        className="flex min-w-0 w-full items-center justify-between gap-2 overflow-hidden"
        onClick={() => data.onSelectWorkstation(data.workstation.node_id)}
        title={workstationTitle}
      >
        <span
          className="flex min-h-5 shrink-0 items-center"
          data-workstation-semantic-icon
          title={semanticIconMetadata.label}
        >
          <GraphSemanticIcon
            className={cn("h-4 w-4", semanticIconMetadata.className)}
            kind={semanticIconMetadata.iconKind}
            label={semanticIconMetadata.label}
          />
        </span>
        <span
          className={workstationTitleClassName(workstationTitle)}
          data-workstation-title
        >
          {workstationTitle}
        </span>
        {data.active ? (
          <ActivityGraphNodeBadge
            className="min-h-5 shrink-0 justify-center px-1.5"
            tone="success"
          >
            <GraphSemanticIcon
              className="h-3.5 w-3.5 text-af-success"
              kind="active-work"
              label="Active"
            />
          </ActivityGraphNodeBadge>
        ) : null}
      </GraphNodeButton>

      <ul className="mt-2 grid min-w-0 list-none content-start gap-1 p-0">
        {visibleWorkItemEntries.map(({ execution, workItem }) => {
          const workItemSelected = data.selectedWorkID === workItem.work_id;
          const workItemLabel = formatWorkItemLabel(workItem);
          const durationLabel = formatDurationFromISO(
            execution.started_at,
            data.now,
          );

          return (
            <li key={`${execution.dispatch_id}:${workItem.work_id}`}>
              <GraphNodeButton
                aria-pressed={workItemSelected}
                className={cn(
                  "grid min-w-0 w-full grid-cols-[minmax(0,1fr)_auto] items-center gap-2 overflow-hidden rounded-lg border border-af-border bg-af-surface px-2 py-1.5 text-[0.74rem]",
                  workItemSelected &&
                    "border-af-info-border bg-af-info-surface shadow-af-info-chip",
                )}
                data-selected={workItemSelected ? "true" : undefined}
                onClick={(event) => {
                  event.stopPropagation();
                  data.onSelectWorkID(workItem.work_id, {
                    dispatchID: execution.dispatch_id,
                    nodeID: data.workstation.node_id,
                  });
                }}
                title={`${workItemLabel} - ${durationLabel}`}
              >
                <span
                  className={workItemLabelClassName(workItemLabel)}
                  data-active-work-label
                >
                  {workItemLabel}
                </span>
                <span
                  className="shrink-0 whitespace-nowrap text-right font-mono text-[0.72rem] text-af-text-subtle"
                  data-active-work-duration
                >
                  {durationLabel}
                </span>
              </GraphNodeButton>
            </li>
          );
        })}
      </ul>
      {workstationOverflowMarkers(
        workItemEntries.length,
        visibleWorkItemEntries.length,
      )}
    </div>
  );
}

function workstationOverflowMarkers(totalCount: number, visibleCount: number) {
  const remainingCount = Math.max(0, totalCount - visibleCount);
  if (remainingCount === 0) {
    return null;
  }

  if (remainingCount > WORKSTATION_SUMMARY_DOT_LIMIT) {
    return (
      <span
        aria-label={`${totalCount} active items`}
        className="mt-2 flex min-h-7 w-full items-center justify-center rounded-lg border border-af-success-border bg-af-success-surface px-3 py-1 font-mono text-[0.9rem] font-bold leading-none text-af-success"
        data-workstation-work-progress="numeric"
        role="status"
      >
        {totalCount}
      </span>
    );
  }

  return (
    <span
      aria-label={`${totalCount} active items`}
      className="mt-2 flex min-h-7 items-center justify-center gap-1 rounded-lg border border-af-success-border bg-af-success-surface px-2"
      data-workstation-work-progress="dots"
      role="status"
    >
      {Array.from(
        { length: remainingCount },
        (_, dotNumber) => dotNumber + 1,
      ).map((dotNumber) => (
        <span
          key={`${remainingCount}-${dotNumber}`}
          aria-hidden="true"
          className="h-1.5 w-1.5 rounded-full bg-af-success"
          data-workstation-work-progress-dot={String(dotNumber - 1)}
        />
      ))}
      <span className="ml-1 font-mono text-[0.68rem] font-bold text-af-success">
        +{remainingCount}
      </span>
    </span>
  );
}

function workstationTitleClassName(label: string): string {
  const textSizeClassName =
    label.length > WORKSTATION_TITLE_DENSE_LENGTH
      ? "text-[0.78rem]"
      : label.length > WORKSTATION_TITLE_COMPACT_LENGTH
        ? "text-[0.88rem]"
        : "text-[1rem]";

  return cn(
    activityGraphNodeTitleClassName("basis-0 flex-1"),
    textSizeClassName,
  );
}

function workItemLabelClassName(label: string): string {
  const textSizeClassName =
    label.length > WORK_ITEM_LABEL_DENSE_LENGTH
      ? "text-[0.64rem]"
      : label.length > WORK_ITEM_LABEL_COMPACT_LENGTH
        ? "text-[0.68rem]"
        : "text-[0.74rem]";

  return cn(
    "block min-w-0 basis-0 flex-1 truncate whitespace-nowrap leading-tight",
    textSizeClassName,
  );
}
