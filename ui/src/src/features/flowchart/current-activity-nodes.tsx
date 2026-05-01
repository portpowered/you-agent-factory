import { Handle, Position, type Node, type NodeProps } from "@xyflow/react";
import type { ReactNode } from "react";

import { cx } from "../../components/dashboard/classnames";
import { formatDurationFromISO, formatWorkItemLabel } from "../../components/dashboard/formatters";
import {
  formatDashboardPlaceLabel,
  getDashboardPlaceLabelParts,
} from "../../components/dashboard/place-labels";
import { GraphSemanticIcon } from "./graph-semantic-icon";
import type { GraphSemanticIconKind } from "./graph-semantic-icon";
import { workstationIconMetadata } from "./workstation-icon-metadata";
import type {
  DashboardActiveExecution,
  DashboardPlaceRef,
  DashboardWorkItemRef,
  DashboardWorkstationNode,
} from "../../../api/dashboard/types";

type PlaceNodeType = "constraint" | "resource" | "statePosition";

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

export interface BasePlaceNodeData extends Record<string, unknown> {
  activeFlow: boolean;
  activeItemLabels: string[];
  incomingHandleCount: number;
  muted: boolean;
  onSelectStateNode?: (placeId: string) => void;
  outgoingHandleCount: number;
  place: DashboardPlaceRef;
  selectedStateNode: boolean;
  tokenCount: number;
}

export interface StatePositionNodeData extends BasePlaceNodeData {
  place: DashboardPlaceRef;
}

export interface ResourceNodeData extends BasePlaceNodeData {
  place: DashboardPlaceRef;
}

export interface ConstraintNodeData extends BasePlaceNodeData {
  place: DashboardPlaceRef;
}

const STATE_NODE_DOT_LIMIT = 10;
const STATE_POSITION_CONTENT_CONTAINER_CLASSNAME =
  "grid min-w-0 w-full grid-rows-[1.5rem_auto] gap-[0.1rem] overflow-hidden";
const RESOURCE_CONTENT_CONTAINER_CLASSNAME =
  "grid min-w-0 w-full grid-rows-[1.5rem_auto] overflow-hidden";
const WORKSTATION_SUMMARY_DOT_LIMIT = 10;
const WORKSTATION_VISIBLE_WORK_ITEM_LIMIT = 3;

export type CurrentActivityWorkstationNode = Node<WorkstationNodeData, "workstation">;
export type CurrentActivityStatePositionNode = Node<StatePositionNodeData, "statePosition">;
export type CurrentActivityResourceNode = Node<ResourceNodeData, "resource">;
export type CurrentActivityConstraintNode = Node<ConstraintNodeData, "constraint">;
export type CurrentActivityPlaceNode =
  | CurrentActivityConstraintNode
  | CurrentActivityResourceNode
  | CurrentActivityStatePositionNode;
export type CurrentActivityNode = CurrentActivityWorkstationNode | CurrentActivityPlaceNode;

interface ActivityGraphNodeShellProps {
  children: ReactNode;
  className?: string;
  incomingHandleCount: number;
  nodeType: "workstation" | PlaceNodeType;
  outgoingHandleCount: number;
}

interface StatePositionNodeContentProps {
  place: DashboardPlaceRef;
  tokenCount: number;
}

interface StaticPlaceNodeContentProps {
  place: DashboardPlaceRef;
  tokenCount: number;
}

const NODE_TYPES = {
  constraint: ConstraintNodeView,
  resource: ResourceNodeView,
  statePosition: StatePositionNodeView,
  workstation: WorkstationNodeView,
};

export { NODE_TYPES as CURRENT_ACTIVITY_NODE_TYPES };

const WORKSTATION_TITLE_COMPACT_LENGTH = 16;
const WORKSTATION_TITLE_DENSE_LENGTH = 28;
const WORK_ITEM_LABEL_COMPACT_LENGTH = 28;
const WORK_ITEM_LABEL_DENSE_LENGTH = 48;

function handlePosition(index: number, count: number): string {
  return `${((index + 1) * 100) / (count + 1)}%`;
}

function ActivityGraphNodeShell({
  children,
  className = "",
  incomingHandleCount,
  nodeType,
  outgoingHandleCount,
}: ActivityGraphNodeShellProps) {
  return (
    <article
      className={cx(
        "flex h-full min-w-0 w-full flex-col gap-[0.35rem] overflow-hidden rounded-lg border border-af-overlay/9 bg-af-canvas p-[0.75rem] text-af-ink",
        className,
      )}
      data-current-activity-node-type={nodeType}
    >
      {Array.from({ length: incomingHandleCount }).map((_, index) => (
        <Handle
          className="opacity-0"
          id={`in-${index}`}
          key={`in-${index}`}
          position={Position.Left}
          style={{ top: handlePosition(index, incomingHandleCount) }}
          type="target"
        />
      ))}
      {Array.from({ length: outgoingHandleCount }).map((_, index) => (
        <Handle
          className="opacity-0"
          id={`out-${index}`}
          key={`out-${index}`}
          position={Position.Right}
          style={{ top: handlePosition(index, outgoingHandleCount) }}
          type="source"
        />
      ))}
      {children}
    </article>
  );
}

function placeNodeClassName(place: DashboardPlaceRef): string {
  const kindClassName = (() => {
    if (place.kind === "work_state") {
      return "border-af-overlay/22";
    }
    if (place.kind === "resource") {
      return "border-af-overlay/22 bg-af-canvas text-af-ink";
    }
    return "border-dashed border-af-info/36 bg-af-surface/78 text-af-ink";
  })();
  const stateClassName =
    place.state_category === "TERMINAL"
      ? "border-af-overlay/22"
      : place.state_category === "FAILED"
        ? "border-af-edge-danger-muted"
        : "";

  return cx(kindClassName, stateClassName);
}

function placeKindLabel(place: DashboardPlaceRef): string {
  if (place.kind === "work_state") {
    if (place.state_category === "TERMINAL") {
      return "Terminal";
    }
    if (place.state_category === "FAILED") {
      return "Failed";
    }
    return "Queue";
  }

  if (place.kind === "resource") {
    return "Resource";
  }

  return place.kind === "limit" ? "Limit" : "Constraint";
}

function placeSemanticIconKind(place: DashboardPlaceRef): GraphSemanticIconKind {
  if (place.kind === "work_state") {
    if (place.state_category === "TERMINAL") {
      return "terminal";
    }
    if (place.state_category === "FAILED") {
      return "failed";
    }
    if (place.state_category === "PROCESSING") {
      return "processing";
    }
    return "queue";
  }

  if (place.kind === "resource") {
    return "resource";
  }

  return place.kind === "limit" ? "limit" : "constraint";
}

function placeSemanticIconLabel(place: DashboardPlaceRef): string {
  if (place.kind === "work_state" && place.state_category === "PROCESSING") {
    return "Processing state";
  }

  return placeKindLabel(place);
}

function placeSemanticIconClassName(place: DashboardPlaceRef): string {
  if (place.kind === "work_state") {
    if (place.state_category === "TERMINAL") {
      return "text-af-success-ink/76";
    }
    if (place.state_category === "FAILED") {
      return "text-af-danger-ink/78";
    }
    if (place.state_category === "PROCESSING") {
      return "text-af-info/78";
    }
    return "text-af-ink/58";
  }

  if (place.kind === "resource") {
    return "text-af-success-ink/76";
  }

  return place.kind === "limit" ? "text-af-danger-ink/74" : "text-af-info/74";
}

function activeItemCountLabel(count: number): string {
  const itemLabel = count === 1 ? "item" : "items";
  return `${count} active ${itemLabel}`;
}

function statePositionMarkers(count: number): ReactNode {
  if (count === 0) {
    return null;
  }

  if (count > STATE_NODE_DOT_LIMIT) {
    return (
      <span
        aria-label={activeItemCountLabel(count)}
        className="inline-flex min-h-5 min-w-7 items-center justify-center rounded-full border border-af-success/25 bg-af-success/12 px-2 font-mono text-[0.76rem] font-bold leading-none text-af-success-ink"
        data-state-work-progress="numeric"
      >
        {count}
      </span>
    );
  }

  return (
    <span
      aria-label={activeItemCountLabel(count)}
      className="inline-grid grid-cols-[repeat(5,0.5rem)] justify-center gap-1"
      data-state-work-progress="dots"
    >
      {Array.from({ length: count }).map((_, index) => (
        <span
          key={`${index}-${count}`}
          aria-hidden="true"
          className="h-2 w-2 rounded-full bg-af-success"
          data-state-work-progress-dot={String(index)}
        />
      ))}
    </span>
  );
}

function tokenCountLabel(place: DashboardPlaceRef, count: number): string {
  if (place.kind === "resource") {
    return `${count} resource tokens`;
  }

  const tokenLabel = count === 1 ? "token" : "tokens";
  return `${count} ${placeKindLabel(place).toLowerCase()} ${tokenLabel}`;
}

function placeTokenCountDisplay(place: DashboardPlaceRef, count: number): ReactNode {
  return (
    <span
      aria-label={tokenCountLabel(place, count)}
      className="inline-flex w-fit rounded-full border border-af-overlay/12 bg-af-overlay/8 px-2 py-[0.1rem] font-mono text-[0.68rem] text-af-ink/64"
      data-place-token-count
    >
      {count}
    </span>
  );
}

function workstationOverflowMarkers(totalCount: number, visibleCount: number): ReactNode {
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

function PlaceSemanticIcon({ place }: { place: DashboardPlaceRef }) {
  return (
    <span
      className="flex min-h-4 shrink-0 items-center"
      data-place-semantic-icon
      title={placeKindLabel(place)}
    >
      <GraphSemanticIcon
        className={cx("h-3.5 w-3.5", placeSemanticIconClassName(place))}
        kind={placeSemanticIconKind(place)}
        label={placeSemanticIconLabel(place)}
      />
    </span>
  );
}

function PlaceLabelText({
  place,
  dataPrefix,
}: {
  dataPrefix: "place" | "state";
  place: DashboardPlaceRef;
}) {
  const label = formatDashboardPlaceLabel(place);
  const labelParts = getDashboardPlaceLabelParts(place);

  return (
    <span className="grid min-w-0 gap-[0.06rem] overflow-hidden" title={label}>
      <span
        className="block min-w-0 overflow-hidden text-ellipsis whitespace-nowrap text-[0.62rem] font-bold uppercase leading-none text-af-ink/52"
        data-place-work-type={dataPrefix === "place" ? true : undefined}
        data-state-work-type={dataPrefix === "state" ? true : undefined}
        title={labelParts.workType}
      >
        {labelParts.workType}
      </span>
      <span
        className="block min-w-0 overflow-hidden truncate whitespace-nowrap font-mono text-[0.76rem] font-bold leading-[0.82rem] text-af-ink"
        data-place-state-value={dataPrefix === "place" ? true : undefined}
        data-state-value={dataPrefix === "state" ? true : undefined}
        title={labelParts.stateValue}
      >
        {labelParts.stateValue}
      </span>
    </span>
  );
}

function StatePositionNodeContent({
  place,
  tokenCount,
}: StatePositionNodeContentProps) {
  const label = formatDashboardPlaceLabel(place);
  const marker = statePositionMarkers(tokenCount);

  return (
    <>
      <span
        className="grid h-[1.5rem] max-h-[1.5rem] min-w-0 grid-cols-[auto_minmax(0,1fr)] items-center gap-1.5 overflow-hidden"
        data-state-label-zone
      >
        <PlaceSemanticIcon place={place} />
        <PlaceLabelText dataPrefix="state" place={place} />
      </span>
      <span
        className="flex min-h-[1.25rem] w-full shrink-0 items-center justify-center overflow-hidden"
        data-state-marker-zone
        title={label}
      >
        {marker ?? <span className="sr-only">{activeItemCountLabel(tokenCount)}</span>}
      </span>
    </>
  );
}

function StaticPlaceNodeContent({
  place,
  tokenCount,
}: StaticPlaceNodeContentProps) {
  const label = formatDashboardPlaceLabel(place);

  if (place.kind !== "resource") {
    return (
      <div
        className="grid min-w-0 gap-[0.1rem] overflow-hidden"
        aria-label={label}
        data-place-label-container
      >
        <span
          className="flex min-w-0 items-center gap-1.5 overflow-hidden"
          data-place-label-zone
          title={label}
        >
          <PlaceSemanticIcon place={place} />
          <strong className="block min-w-0 truncate whitespace-nowrap font-mono text-[0.86rem] font-bold leading-tight">
            {label}
          </strong>
        </span>
        <span
          className="flex min-h-[1rem] w-full shrink-0 items-center justify-start overflow-hidden"
          data-place-marker-zone
          title={label}
        >
          {placeTokenCountDisplay(place, tokenCount)}
        </span>
      </div>
    );
  }

  return (
    <div
      className={RESOURCE_CONTENT_CONTAINER_CLASSNAME}
      aria-label={label}
      data-place-label-container
    >
      <span
        className="grid h-[1.5rem] max-h-[1.5rem] min-w-0 grid-cols-[auto_minmax(0,1fr)] items-center gap-1.5 overflow-hidden"
        data-place-label-zone
      >
        <PlaceSemanticIcon place={place} />
        <PlaceLabelText dataPrefix="place" place={place} />
      </span>
      <span
        className="flex min-h-[1.25rem] w-full shrink-0 items-center justify-start overflow-hidden"
        data-place-marker-zone
        title={label}
      >
        {placeTokenCountDisplay(place, tokenCount)}
      </span>
    </div>
  );
}

function PlaceNodeView({ data }: NodeProps<CurrentActivityPlaceNode>) {
  const placeLabel = formatDashboardPlaceLabel(data.place);
  const selectable = data.place.kind === "work_state" && data.onSelectStateNode !== undefined;
  const showStateMarkers = data.place.kind === "work_state";
  const nodeType: PlaceNodeType =
    data.place.kind === "work_state"
      ? "statePosition"
      : data.place.kind === "resource"
        ? "resource"
        : "constraint";
  const nodeClassName = cx(
    placeNodeClassName(data.place),
    data.activeFlow && !data.selectedStateNode && "border-af-success/70 shadow-af-success-chip",
    data.selectedStateNode && "border-af-accent/70 shadow-af-accent-selected",
    data.muted && "opacity-[0.45]",
  );

  return (
    <ActivityGraphNodeShell
      className={cx(
        "justify-center text-left",
        nodeClassName,
      )}
      incomingHandleCount={data.incomingHandleCount}
      nodeType={nodeType}
      outgoingHandleCount={data.outgoingHandleCount}
    >
      {selectable ? (
        <button
          aria-label={`Select ${placeLabel} state`}
          aria-pressed={data.selectedStateNode}
          className={cx(
            "nodrag nopan cursor-pointer border-0 bg-transparent p-0 text-left text-inherit",
            STATE_POSITION_CONTENT_CONTAINER_CLASSNAME,
          )}
          data-selected-state={data.selectedStateNode ? "true" : undefined}
          onClick={(event) => {
            event.stopPropagation();
            data.onSelectStateNode?.(data.place.place_id);
          }}
          type="button"
        >
          <StatePositionNodeContent
            place={data.place}
            tokenCount={data.tokenCount}
          />
        </button>
      ) : (
        <>
          {showStateMarkers ? (
            <StatePositionNodeContent
              place={data.place}
              tokenCount={data.tokenCount}
            />
          ) : (
            <StaticPlaceNodeContent
              place={data.place}
              tokenCount={data.tokenCount}
            />
          )}
        </>
      )}
    </ActivityGraphNodeShell>
  );
}

export function StatePositionNodeView(props: NodeProps<CurrentActivityStatePositionNode>) {
  return <PlaceNodeView {...props} />;
}

export function ResourceNodeView(props: NodeProps<CurrentActivityResourceNode>) {
  return <PlaceNodeView {...props} />;
}

export function ConstraintNodeView(props: NodeProps<CurrentActivityConstraintNode>) {
  return <PlaceNodeView {...props} />;
}

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
    !exhaustionRule && data.active && !data.selectedWorkstation && "border-af-success/50 shadow-af-success-chip",
    !exhaustionRule && data.activeFlow && !data.selectedWorkstation && "agent-flow-node--active ring-2 ring-af-success/18",
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
      ) : (
        <div
          className="grid h-full min-w-0 grid-rows-[auto_1fr_auto]"
          data-active={data.active ? "true" : undefined}
          data-selected-work={selectedWork ? "true" : undefined}
          data-selected-workstation={data.selectedWorkstation ? "true" : undefined}
          data-workstation-kind={semanticIconMetadata.semanticKind}
        >
          <button
            aria-label={`Select ${workstationTitle} workstation`}
            className="nodrag flex min-w-0 w-full cursor-pointer items-center justify-between gap-2 overflow-hidden border-0 bg-transparent p-0 text-left text-inherit"
            onClick={() => data.onSelectWorkstation(data.workstation.node_id)}
            aria-pressed={data.selectedWorkstation}
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
            <span
              className={workstationTitleClassName(workstationTitle)}
              data-workstation-title
            >
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
                    aria-pressed={workItemSelected}
                    title={`${workItemLabel} - ${durationLabel}`}
                    type="button"
                  >
                    <span
                      className={workItemLabelClassName(workItemLabel)}
                      data-active-work-label
                    >
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
          {workstationOverflowMarkers(
            workItemEntries.length,
            visibleWorkItemEntries.length,
          )}
        </div>
      )}
    </ActivityGraphNodeShell>
  );
}
