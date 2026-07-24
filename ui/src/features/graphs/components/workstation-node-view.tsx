import type { Node, NodeProps } from "@xyflow/react";
import { GraphNodeButton } from "@you-agent-factory/components/graphs";
import type {
  DashboardActiveExecution,
  DashboardWorkItemRef,
  DashboardWorkstationNode,
} from "../../../api/dashboard/types";
import {
  formatDurationFromISO,
  formatGraphDurationFromISO,
  formatWorkItemLabel,
} from "../../../components/ui/formatters";
import { cn } from "../../../lib/cn";
import type { WorkstationProgressOutcomeRouteContext } from "../../current-factory-definition/lib/workstation-progress-outcome-routes";
import {
  activityGraphNodeSurfaceClassName,
  activityGraphNodeTitleClassName,
} from "../../flowchart/components/current-activity-node-chrome";
import { CurrentActivityWorkProgressMarker } from "../../flowchart/components/current-activity-work-progress-marker";
import { GraphSemanticIcon } from "../../flowchart/components/graph-semantic-icon";
import { currentActivityGraphNodeHoverClassName } from "../../flowchart/lib/current-activity-graph-hover";
import { workstationGraphPresentation } from "../../flowchart/lib/workstation-graph-presentation";
import { getActivityGraphMessages } from "../../flowchart/messages/activity-graph";
import { getWorkflowActivityShellMessages } from "../../workflow-activity/messages/activity-shell";
import type {
  ActivityGraphNodeHandle,
  ZAxisIncompleteHints,
} from "./graph-node-shell";
import { ActivityGraphNodeShell } from "./graph-node-shell";

export interface WorkstationNodeData extends Record<string, unknown> {
  active: boolean;
  activeFlow: boolean;
  executions: DashboardActiveExecution[];
  factoryGraphNodeId?: string;
  handles: ActivityGraphNodeHandle[];
  kind?: "workstation";
  locale?: string;
  muted: boolean;
  now: number;
  progressOutcomeRouteWorkstation?: WorkstationProgressOutcomeRouteContext;
  selectedWorkID: string | null;
  selectedWorkstation: boolean;
  summaryOnly?: boolean;
  workstation: DashboardWorkstationNode;
  zAxisIncompleteHints?: ZAxisIncompleteHints | null;
  onSelectWorkstation?: (nodeId: string) => void;
  onSelectWorkID?: (
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
const WORKSTATION_TITLE_COMPACT_LENGTH = 20;
const WORKSTATION_TITLE_DENSE_LENGTH = 34;
const WORK_ITEM_LABEL_COMPACT_LENGTH = 38;
const WORK_ITEM_LABEL_DENSE_LENGTH = 58;

export function WorkstationNodeView({
  data,
}: NodeProps<CurrentActivityWorkstationNode>) {
  const messages = getWorkflowActivityShellMessages(data.locale);
  const semanticIconMetadata = workstationGraphPresentation(
    data.workstation,
    data.locale,
  );
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
    activityGraphNodeSurfaceClassName("workstation"),
    "min-w-0 w-full justify-start overflow-hidden border-2",
    currentActivityGraphNodeHoverClassName(
      {
        muted: data.muted,
        selected: data.selectedWorkstation,
      },
      "primary",
    ),
    exhaustionRule
      ? "border-dashed border-af-danger-border"
      : "border-info-border",
    !exhaustionRule && semanticIconMetadata.borderClassName,
    !exhaustionRule &&
      data.active &&
      !data.selectedWorkstation &&
      "border-af-success-border shadow-af-success-chip",
    !exhaustionRule &&
      data.activeFlow &&
      !data.selectedWorkstation &&
      "agent-flow-node--active ring-2 ring-af-success-border",
    data.selectedWorkstation && "border-primary shadow-af-accent-selected",
    !exhaustionRule &&
      selectedWork &&
      "border-info-border shadow-af-info-selected",
    data.muted && "opacity-[0.45]",
  );

  return (
    <ActivityGraphNodeShell
      className={nodeClassName}
      handles={data.handles}
      nodeType="workstation"
      zAxisIncompleteHints={data.zAxisIncompleteHints}
    >
      {exhaustionRule ? (
        <ExhaustionRuleNodeButton
          data={data}
          messages={messages}
          semanticIconMetadata={semanticIconMetadata}
          workstationTitle={workstationTitle}
        />
      ) : data.summaryOnly ? (
        <SummaryWorkstationNodeContent
          data={data}
          messages={messages}
          semanticIconMetadata={semanticIconMetadata}
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

function SummaryWorkstationNodeContent({
  data,
  messages,
  semanticIconMetadata,
  workstationTitle,
}: {
  data: WorkstationNodeData;
  messages: ReturnType<typeof getWorkflowActivityShellMessages>;
  semanticIconMetadata: ReturnType<typeof workstationGraphPresentation>;
  workstationTitle: string;
}) {
  return (
    <GraphNodeButton
      aria-label={
        data.onSelectWorkstation
          ? messages.selectWorkstationLabel(workstationTitle)
          : undefined
      }
      aria-pressed={
        data.onSelectWorkstation ? data.selectedWorkstation : undefined
      }
      className="flex min-w-0 w-full items-center justify-between gap-2 overflow-hidden"
      data-selected-workstation={data.selectedWorkstation ? "true" : undefined}
      data-workstation-kind={semanticIconMetadata.semanticKind}
      disabled={data.onSelectWorkstation === undefined}
      onClick={
        data.onSelectWorkstation
          ? (event) => {
              event.stopPropagation();
              data.onSelectWorkstation?.(data.workstation.node_id);
            }
          : undefined
      }
      title={workstationTitle}
    >
      <WorkstationHeaderContent
        data={data}
        semanticIconMetadata={semanticIconMetadata}
        workstationTitle={workstationTitle}
      />
    </GraphNodeButton>
  );
}

function ExhaustionRuleNodeButton({
  data,
  messages,
  semanticIconMetadata,
  workstationTitle,
}: {
  data: WorkstationNodeData;
  messages: ReturnType<typeof getWorkflowActivityShellMessages>;
  semanticIconMetadata: ReturnType<typeof workstationGraphPresentation>;
  workstationTitle: string;
}) {
  const header = (
    <>
      <span
        className="flex min-h-4 items-center"
        data-workstation-semantic-icon
        title={semanticIconMetadata.label}
      >
        <GraphSemanticIcon
          className={cn("h-4 w-4", semanticIconMetadata.className)}
          kind={semanticIconMetadata.iconKind}
          label={semanticIconMetadata.label}
          locale={data.locale}
        />
      </span>
      <span
        className="block min-w-0 truncate whitespace-nowrap font-mono text-[0.74rem] font-bold leading-tight text-on-surface"
        data-workstation-title
      >
        {workstationTitle}
      </span>
    </>
  );

  if (data.onSelectWorkstation === undefined) {
    return (
      <div
        className="flex h-full min-w-0 w-full items-center gap-2 overflow-hidden"
        data-selected-workstation={
          data.selectedWorkstation ? "true" : undefined
        }
        data-workstation-kind={semanticIconMetadata.semanticKind}
        title={workstationTitle}
      >
        {header}
      </div>
    );
  }

  return (
    <GraphNodeButton
      aria-label={messages.selectExhaustionRuleLabel(workstationTitle)}
      aria-pressed={data.selectedWorkstation}
      className="flex h-full min-w-0 w-full items-center gap-2 overflow-hidden"
      data-selected-workstation={data.selectedWorkstation ? "true" : undefined}
      data-workstation-kind={semanticIconMetadata.semanticKind}
      onClick={(event) => {
        event.stopPropagation();
        data.onSelectWorkstation?.(data.workstation.node_id);
      }}
      title={workstationTitle}
    >
      {header}
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
  semanticIconMetadata: ReturnType<typeof workstationGraphPresentation>;
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
  const workstationHeaderSelectable = data.onSelectWorkstation !== undefined;

  return (
    <div
      className="grid h-full min-w-0 grid-rows-[auto_1fr_auto]"
      data-active={data.active ? "true" : undefined}
      data-selected-work={selectedWork ? "true" : undefined}
      data-selected-workstation={data.selectedWorkstation ? "true" : undefined}
      data-workstation-kind={semanticIconMetadata.semanticKind}
    >
      {workstationHeaderSelectable ? (
        <GraphNodeButton
          aria-label={messages.selectWorkstationLabel(workstationTitle)}
          aria-pressed={data.selectedWorkstation}
          className="flex min-w-0 w-full items-center justify-between gap-2 overflow-hidden"
          onClick={(event) => {
            event.stopPropagation();
            data.onSelectWorkstation?.(data.workstation.node_id);
          }}
          title={workstationTitle}
        >
          <WorkstationHeaderContent
            data={data}
            semanticIconMetadata={semanticIconMetadata}
            workstationTitle={workstationTitle}
          />
        </GraphNodeButton>
      ) : (
        <div
          className="flex min-w-0 w-full items-center justify-between gap-2 overflow-hidden"
          title={workstationTitle}
        >
          <WorkstationHeaderContent
            data={data}
            semanticIconMetadata={semanticIconMetadata}
            workstationTitle={workstationTitle}
          />
        </div>
      )}

      <ul className="mt-2 grid min-w-0 list-none content-start gap-1 p-0">
        {visibleWorkItemEntries.map(({ execution, workItem }) => {
          const workItemSelected = data.selectedWorkID === workItem.work_id;
          const workItemLabel = formatWorkItemLabel(workItem);
          const durationLabel = formatGraphDurationFromISO(
            execution.started_at,
            data.now,
            data.locale,
          );
          const durationTitle = formatDurationFromISO(
            execution.started_at,
            data.now,
            data.locale,
          );
          const workItemContent = (
            <>
              <span
                className={workItemLabelClassName(workItemLabel)}
                data-active-work-label
              >
                {workItemLabel}
              </span>
              <span
                className="shrink-0 whitespace-nowrap text-right font-mono text-[0.72rem] text-on-surface-subtle"
                data-active-work-duration
              >
                {durationLabel}
              </span>
            </>
          );
          const workItemClassName = cn(
            "grid min-w-0 w-full grid-cols-[minmax(0,1fr)_auto] items-center gap-1 overflow-hidden rounded-lg border border-outline bg-surface px-1.5 py-1 text-[0.74rem]",
            workItemSelected &&
              "border-info-border bg-info-container shadow-af-info-chip",
          );

          return (
            <li key={`${execution.dispatch_id}:${workItem.work_id}`}>
              {data.onSelectWorkID ? (
                <GraphNodeButton
                  aria-pressed={workItemSelected}
                  className={workItemClassName}
                  data-selected={workItemSelected ? "true" : undefined}
                  onClick={(event) => {
                    event.stopPropagation();
                    data.onSelectWorkID?.(workItem.work_id, {
                      dispatchID: execution.dispatch_id,
                      nodeID: data.workstation.node_id,
                    });
                  }}
                  title={`${workItemLabel} - ${durationTitle}`}
                >
                  {workItemContent}
                </GraphNodeButton>
              ) : (
                <div
                  className={workItemClassName}
                  data-selected={workItemSelected ? "true" : undefined}
                  title={`${workItemLabel} - ${durationTitle}`}
                >
                  {workItemContent}
                </div>
              )}
            </li>
          );
        })}
      </ul>
      {workstationOverflowMarkers(
        workItemEntries.length,
        visibleWorkItemEntries.length,
        data.locale,
      )}
    </div>
  );
}

function WorkstationHeaderContent({
  data,
  semanticIconMetadata,
  workstationTitle,
}: {
  data: WorkstationNodeData;
  semanticIconMetadata: ReturnType<typeof workstationGraphPresentation>;
  workstationTitle: string;
}) {
  return (
    <>
      <span
        className="flex min-h-5 shrink-0 items-center"
        data-workstation-semantic-icon
        title={semanticIconMetadata.label}
      >
        <GraphSemanticIcon
          className={cn("h-4 w-4", semanticIconMetadata.className)}
          kind={semanticIconMetadata.iconKind}
          label={semanticIconMetadata.label}
          locale={data.locale}
        />
      </span>
      <span
        className={workstationTitleClassName(workstationTitle)}
        data-workstation-title
      >
        {workstationTitle}
      </span>
    </>
  );
}

function workstationOverflowMarkers(
  totalCount: number,
  visibleCount: number,
  locale?: string,
) {
  const messages = getActivityGraphMessages(locale);
  const remainingCount = Math.max(0, totalCount - visibleCount);
  if (remainingCount === 0) {
    return null;
  }

  if (remainingCount > WORKSTATION_SUMMARY_DOT_LIMIT) {
    return (
      <CurrentActivityWorkProgressMarker
        ariaLabel={messages.activeItemCountLabel(totalCount)}
        className="mt-2 flex min-h-7 w-full rounded-lg px-3 py-1 text-[0.9rem]"
        count={totalCount}
        data-workstation-work-progress="numeric"
        kind="numeric"
      />
    );
  }

  return (
    <CurrentActivityWorkProgressMarker
      ariaLabel={messages.activeItemCountLabel(totalCount)}
      className="mt-2 flex min-h-7 gap-1 rounded-lg px-2"
      data-workstation-work-progress="dots"
      dotClassName="h-1.5 w-1.5"
      dotCount={remainingCount}
      dotDataAttribute="data-workstation-work-progress-dot"
      kind="dots"
      suffix={
        <span className="ml-1 font-mono text-[0.68rem] font-bold text-success">
          +{remainingCount}
        </span>
      }
    />
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
