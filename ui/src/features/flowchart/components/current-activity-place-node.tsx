import type { Node, NodeProps } from "@xyflow/react";
import type { ReactNode } from "react";

import type { DashboardPlaceRef } from "../../../api/dashboard/types";
import {
  formatDashboardPlaceLabel,
  getDashboardPlaceLabelParts,
} from "../../../components/ui/place-labels";
import { cn } from "../../../lib/cn";
import type { FactoryGraphNodeKind } from "../../factory-graph-editor/lib/factory-graph-draft-types";
import { getWorkflowActivityShellMessages } from "../../workflow-activity/messages/activity-shell";
import { ActivityGraphNodeBadge } from "./current-activity-node-chrome";
import {
  type ActivityGraphNodeHandle,
  ActivityGraphNodeShell,
  type PlaceNodeType,
} from "./current-activity-node-shell";
import type { GraphSemanticIconKind } from "./graph-semantic-icon";
import { GraphSemanticIcon } from "./graph-semantic-icon";

export interface BasePlaceNodeData extends Record<string, unknown> {
  activeFlow: boolean;
  activeItemLabels: string[];
  factoryGraphNodeId?: string;
  handles?: ActivityGraphNodeHandle[];
  incomingHandleCount: number;
  kind?: FactoryGraphNodeKind;
  locale?: string;
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

export type CurrentActivityStatePositionNode = Node<
  StatePositionNodeData,
  "statePosition"
>;
export type CurrentActivityResourceNode = Node<ResourceNodeData, "resource">;
export type CurrentActivityConstraintNode = Node<
  ConstraintNodeData,
  "constraint"
>;
export type CurrentActivityPlaceNode =
  | CurrentActivityConstraintNode
  | CurrentActivityResourceNode
  | CurrentActivityStatePositionNode;

const STATE_NODE_DOT_LIMIT = 10;
const STATE_POSITION_CONTENT_CONTAINER_CLASSNAME =
  "flex min-w-0 w-full flex-col gap-0.5 overflow-hidden";
const RESOURCE_CONTENT_CONTAINER_CLASSNAME =
  "flex min-w-0 w-full flex-col overflow-hidden";

export function StatePositionNodeView(
  props: NodeProps<CurrentActivityStatePositionNode>,
) {
  return <PlaceNodeView {...props} />;
}

export function ResourceNodeView(
  props: NodeProps<CurrentActivityResourceNode>,
) {
  return <PlaceNodeView {...props} />;
}

export function ConstraintNodeView(
  props: NodeProps<CurrentActivityConstraintNode>,
) {
  return <PlaceNodeView {...props} />;
}

function PlaceNodeView({ data }: NodeProps<CurrentActivityPlaceNode>) {
  const messages = getWorkflowActivityShellMessages(data.locale);
  const placeLabel = formatDashboardPlaceLabel(data.place);
  const selectable =
    data.place.kind === "work_state" && data.onSelectStateNode !== undefined;
  const showStateMarkers = data.place.kind === "work_state";
  const nodeType: PlaceNodeType =
    data.place.kind === "work_state"
      ? "statePosition"
      : data.place.kind === "resource"
        ? "resource"
        : "constraint";
  const nodeClassName = cn(
    placeNodeClassName(data.place),
    data.activeFlow &&
      !data.selectedStateNode &&
      "border-af-success-border shadow-af-success-chip",
    data.selectedStateNode &&
      "border-af-accent-border shadow-af-accent-selected",
    data.muted && "opacity-[0.45]",
  );

  return (
    <ActivityGraphNodeShell
      className={cn("justify-center text-left", nodeClassName)}
      handles={data.handles}
      incomingHandleCount={data.incomingHandleCount}
      nodeType={nodeType}
      outgoingHandleCount={data.outgoingHandleCount}
    >
      {selectable ? (
        <button
          aria-label={messages.selectStateLabel(placeLabel)}
          aria-pressed={data.selectedStateNode}
          className={cn(
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
      ) : showStateMarkers ? (
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
    </ActivityGraphNodeShell>
  );
}

function placeNodeClassName(place: DashboardPlaceRef): string {
  const kindClassName = (() => {
    if (place.kind === "work_state") {
      return "border-af-border-strong";
    }
    if (place.kind === "resource") {
      return "border-af-border-strong bg-af-surface text-af-text";
    }
    return "border-dashed border-af-info-border bg-af-surface-subtle text-af-text";
  })();
  const stateClassName =
    place.state_category === "TERMINAL"
      ? "border-af-border-strong"
      : place.state_category === "FAILED"
        ? "border-af-danger-border"
        : "";

  return cn(kindClassName, stateClassName);
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

function placeSemanticIconKind(
  place: DashboardPlaceRef,
): GraphSemanticIconKind {
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
      return "text-af-success";
    }
    if (place.state_category === "FAILED") {
      return "text-af-danger";
    }
    if (place.state_category === "PROCESSING") {
      return "text-af-info";
    }
    return "text-af-text-subtle";
  }

  if (place.kind === "resource") {
    return "text-af-success";
  }

  return place.kind === "limit" ? "text-af-danger" : "text-af-info";
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
        className="inline-flex min-h-5 min-w-7 items-center justify-center rounded-full border border-af-success-border bg-af-success-surface px-2 font-mono text-[0.76rem] font-bold leading-none text-af-success"
        data-state-work-progress="numeric"
        role="status"
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
      role="status"
    >
      {Array.from({ length: count }, (_, dotNumber) => dotNumber + 1).map(
        (dotNumber) => (
          <span
            key={`${count}-${dotNumber}`}
            aria-hidden="true"
            className="h-2 w-2 rounded-full bg-af-success"
            data-state-work-progress-dot={String(dotNumber - 1)}
          />
        ),
      )}
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

function placeTokenCountDisplay(
  place: DashboardPlaceRef,
  count: number,
): ReactNode {
  return (
    <ActivityGraphNodeBadge
      aria-label={tokenCountLabel(place, count)}
      className="w-fit"
      data-place-token-count
      role="status"
    >
      {count}
    </ActivityGraphNodeBadge>
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
        className={cn("h-3.5 w-3.5", placeSemanticIconClassName(place))}
        kind={placeSemanticIconKind(place)}
        label={placeSemanticIconLabel(place)}
      />
    </span>
  );
}

function PlaceLabelText({
  dataPrefix,
  place,
}: {
  dataPrefix: "place" | "state";
  place: DashboardPlaceRef;
}) {
  const label = formatDashboardPlaceLabel(place);
  const labelParts = getDashboardPlaceLabelParts(place);

  return (
    <span className="grid min-w-0 gap-px overflow-hidden" title={label}>
      <span
        className="block min-w-0 overflow-hidden text-ellipsis whitespace-nowrap text-[0.62rem] font-bold uppercase leading-none text-af-text-subtle"
        data-place-work-type={dataPrefix === "place" ? true : undefined}
        data-state-work-type={dataPrefix === "state" ? true : undefined}
        title={labelParts.workType}
      >
        {labelParts.workType}
      </span>
      <span
        className="block min-w-0 overflow-hidden truncate whitespace-nowrap font-mono text-[0.76rem] font-bold leading-[0.82rem] text-af-text"
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
}: {
  place: DashboardPlaceRef;
  tokenCount: number;
}) {
  const label = formatDashboardPlaceLabel(place);
  const marker = statePositionMarkers(tokenCount);

  return (
    <>
      <span
        className="grid h-6 max-h-6 min-w-0 grid-cols-[auto_minmax(0,1fr)] items-center gap-1.5 overflow-hidden"
        data-state-label-zone
      >
        <PlaceSemanticIcon place={place} />
        <PlaceLabelText dataPrefix="state" place={place} />
      </span>
      <span
        className="flex min-h-5 w-full shrink-0 items-center justify-center overflow-hidden"
        data-state-marker-zone
        title={label}
      >
        {marker ?? (
          <span className="sr-only">{activeItemCountLabel(tokenCount)}</span>
        )}
      </span>
    </>
  );
}

function StaticPlaceNodeContent({
  place,
  tokenCount,
}: {
  place: DashboardPlaceRef;
  tokenCount: number;
}) {
  const label = formatDashboardPlaceLabel(place);

  if (place.kind !== "resource") {
    return (
      <div
        className="grid min-w-0 gap-0.5 overflow-hidden"
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
          className="flex min-h-4 w-full shrink-0 items-center justify-start overflow-hidden"
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
      data-place-label-container
    >
      <span
        aria-label={label}
        className="grid h-6 max-h-6 min-w-0 grid-cols-[auto_minmax(0,1fr)] items-center gap-1.5 overflow-hidden"
        data-place-label-zone
        role="img"
      >
        <PlaceSemanticIcon place={place} />
        <PlaceLabelText dataPrefix="place" place={place} />
      </span>
      <span
        className="flex min-h-5 w-full shrink-0 items-center justify-start overflow-hidden"
        data-place-marker-zone
        title={label}
      >
        {placeTokenCountDisplay(place, tokenCount)}
      </span>
    </div>
  );
}
