import type { Node, NodeProps } from "@xyflow/react";

import type {
  DashboardActiveExecution,
  DashboardWorkItemRef,
  DashboardWorkstationNode,
} from "../../api/dashboard/types";
import { cx } from "../../components/dashboard/classnames";
import {
  formatDurationFromISO,
  formatWorkItemLabel,
} from "../../components/ui/formatters";
import { GraphSemanticIcon } from "./graph-semantic-icon";
import { ActivityGraphNodeShell } from "./current-activity-node-shell";
import { workstationIconMetadata } from "./workstation-icon-metadata";

export interface WorkstationNodeData extends Record<string, unknown> {
  active: boolean;
  activeFlow: boolean;
  executions: DashboardActiveExecution[];
  incomingHandleCount: number;
  muted: boolean;
  now: number;
  outgoingHandleCount: number;
  selectedWorkID: string | null;
  selectedWorkstation: boolean;
  workstation: DashboardWorkstationNode;
  onSelectWorkstation: (nodeId: string) => void;
  onSelectWorkItem: (
    dispatchId: string,
    nodeId: string,
    execution: DashboardActiveExecution,
    workItem: DashboardWorkItemRef,
  ) => void;
}

export type CurrentActivityWorkstationNode = Node<WorkstationNodeData, "workstation">;

const WORKSTATION_SUMMARY_DOT_LIMIT = 10;
const WORKSTATION_VISIBLE_WORK_ITEM_LIMIT = 3;
const WORKSTATION_TITLE_COMPACT_LENGTH = 16;
const WORKSTATION_TITLE_DENSE_LENGTH = 28;
const WORK_ITEM_LABEL_COMPACT_LENGTH = 28;
const WORK_ITEM_LABEL_DENSE_LENGTH = 48;

export function WorkstationNodeView({ data }: NodeProps<CurrentActivityWorkstationNode>) {
  const semanticIconMetadata = workstationIconMetadata(data.workstation);
  const exhaustionRule = semanticIconMetadata.semanticKind === "exhaustion";
  const selectedWork = data.selectedWorkID !== null;
  const workItemEntries = data.executions.flatMap((execution) =>
    (execution.work_items ?? []).map((workItem) => ({ execution, workItem })),
  );
  const visibleWorkItemEntries = workItemEntries.slice(0, WORKSTATION_VISIBLE_WORK_ITEM_LIMIT);
  const workstationTitle =
    data.workstation.workstation_name ||
    data.workstation.transition_id ||
    data.workstation.node_id;
  const nodeClassName = cx(
    "min-w-0 w-full justify-start overflow-hidden border-2 bg-af-surface/88",
    exhaustionRule ? "border-dashed border-af-danger/36" : "border-af-info/28",
    !exhaustionRule && semanticIconMetadata.semanticKind === "repeater" && "border-double",
    !exhaustionRule &&
      data.active &&
      !data.selectedWorkstation &&
      "border-af-success/50 shadow-af-success-chip",
    !exhaustionRule &&
      data.activeFlow &&
      !data.selectedWorkstation &&
      "agent-flow-node--active ring-2 ring-af-success/18",
    data.selectedWorkstation && "border-af-accent/70 shadow-af-accent-selected",
    !exhaustionRule && selectedWork && "border-af-info/70 shadow-af-info-selected",
    data.muted && "opacity-[0.45]",
  );

  return (
    <ActivityGraphNodeShell
      className={nodeClassName}
      incomingHandleCount={data.incomingHandleCount}
      nodeType="workstation"
      outgoingHandleCount={data.outgoingHandleCount}
    >
      {exhaustionRule ? (
        <ExhaustionRuleNodeButton data={data} workstationTitle={workstationTitle} />
      ) : (
        <ActiveWorkstationNodeContent
          data={data}
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
  workstationTitle,
}: {
  data: WorkstationNodeData;
  workstationTitle: string;
}) {
  const semanticIconMetadata = workstationIconMetadata(data.workstation);

  return (
    <button
      aria-label={`Select ${workstationTitle} exhaustion rule`}
      aria-pressed={data.selectedWorkstation}
      className="nodrag flex h-full min-w-0 w-full cursor-pointer items-center gap-2 overflow-hidden border-0 bg-transparent p-0 text-left text-inherit"
      data-selected-workstation={data.selectedWorkstation ? "true" : undefined}
      data-workstation-kind={semanticIconMetadata.semanticKind}
      onClick={() => data.onSelectWorkstation(data.workstation.node_id)}
      title={workstationTitle}
      type="button"
    >
      <span
        className="flex min-h-4 items-center"
        data-workstation-semantic-icon
        title={semanticIconMetadata.label}
      >
        <GraphSemanticIcon
          className={cx("h-4 w-4", semanticIconMetadata.className)}
          kind={semanticIconMetadata.iconKind}
          label={semanticIconMetadata.label}
        />
      </span>
      <span
        className="block min-w-0 truncate whitespace-nowrap font-mono text-[0.74rem] font-bold leading-tight text-af-ink/86"
        data-workstation-title
      >
        {workstationTitle}
      </span>
    </button>
  );
}

function ActiveWorkstationNodeContent({
  data,
  semanticIconMetadata,
  selectedWork,
  visibleWorkItemEntries,
  workItemEntries,
  workstationTitle,
}: {
  data: WorkstationNodeData;
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
      <button
        aria-label={`Select ${workstationTitle} workstation`}
        aria-pressed={data.selectedWorkstation}
        className="nodrag flex min-w-0 w-full cursor-pointer items-center justify-between gap-2 overflow-hidden border-0 bg-transparent p-0 text-left text-inherit"
        onClick={() => data.onSelectWorkstation(data.workstation.node_id)}
        title={workstationTitle}
        type="button"
      >
        <span
          className="flex min-h-5 shrink-0 items-center"
          data-workstation-semantic-icon
          title={semanticIconMetadata.label}
        >
          <GraphSemanticIcon
            className={cx("h-4 w-4", semanticIconMetadata.className)}
            kind={semanticIconMetadata.iconKind}
            label={semanticIconMetadata.label}
          />
        </span>
        <span className={workstationTitleClassName(workstationTitle)} data-workstation-title>
          {workstationTitle}
        </span>
        {data.active ? (
          <span
            className="inline-flex min-h-5 shrink-0 items-center justify-center rounded-full bg-af-success/15 px-1.5 py-[0.12rem] text-af-success-ink"
            data-workstation-active-icon
            title="Active"
          >
            <GraphSemanticIcon
              className="h-3.5 w-3.5 text-af-success-ink"
              kind="active-work"
              label="Active"
            />
          </span>
        ) : null}
      </button>

      <ul className="mt-[0.55rem] grid min-w-0 list-none content-start gap-[0.3rem] p-0">
        {visibleWorkItemEntries.map(({ execution, workItem }) => {
          const workItemSelected = data.selectedWorkID === workItem.work_id;
          const workItemLabel = formatWorkItemLabel(workItem);
          const durationLabel = formatDurationFromISO(execution.started_at, data.now);

          return (
            <li key={`${execution.dispatch_id}:${workItem.work_id}`}>
              <button
                aria-pressed={workItemSelected}
                className={cx(
                  "nodrag nopan grid min-w-0 w-full cursor-pointer grid-cols-[minmax(0,1fr)_auto] items-center gap-2 overflow-hidden rounded-lg border border-af-overlay/8 bg-af-surface px-2 py-[0.4rem] text-left text-[0.74rem] text-inherit",
                  workItemSelected && "border-af-info/60 bg-af-info/15 shadow-af-info-chip",
                )}
                data-selected={workItemSelected ? "true" : undefined}
                onClick={(event) => {
                  event.stopPropagation();
                  data.onSelectWorkItem(
                    execution.dispatch_id,
                    data.workstation.node_id,
                    execution,
                    workItem,
                  );
                }}
                title={`${workItemLabel} - ${durationLabel}`}
                type="button"
              >
                <span className={workItemLabelClassName(workItemLabel)} data-active-work-label>
                  {workItemLabel}
                </span>
                <span
                  className="shrink-0 whitespace-nowrap text-right font-mono text-[0.72rem] text-af-ink/68"
                  data-active-work-duration
                >
                  {durationLabel}
                </span>
              </button>
            </li>
          );
        })}
      </ul>
      {workstationOverflowMarkers(workItemEntries.length, visibleWorkItemEntries.length)}
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
        className="mt-[0.45rem] flex min-h-7 w-full items-center justify-center rounded-lg border border-af-success/25 bg-af-success/12 px-3 py-1 font-mono text-[0.9rem] font-bold leading-none text-af-success-ink"
        data-workstation-work-progress="numeric"
      >
        {totalCount}
      </span>
    );
  }

  return (
    <span
      aria-label={`${totalCount} active items`}
      className="mt-[0.45rem] flex min-h-7 items-center justify-center gap-1 rounded-lg border border-af-success/18 bg-af-success/10 px-2"
      data-workstation-work-progress="dots"
    >
      {Array.from({ length: remainingCount }).map((_, index) => (
        <span
          key={`${index}-${remainingCount}`}
          aria-hidden="true"
          className="h-1.5 w-1.5 rounded-full bg-af-success"
          data-workstation-work-progress-dot={String(index)}
        />
      ))}
      <span className="ml-1 font-mono text-[0.68rem] font-bold text-af-success-ink">
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

  return cx(
    "block min-w-0 basis-0 flex-1 truncate whitespace-nowrap font-bold leading-tight",
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

  return cx(
    "block min-w-0 basis-0 flex-1 truncate whitespace-nowrap leading-tight",
    textSizeClassName,
  );
}
