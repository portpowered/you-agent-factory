import type { ReactNode } from "react";
import type { Node, NodeProps } from "@xyflow/react";

import type { DashboardPlaceRef } from "../../api/dashboard/types";
import { cx } from "../../lib/cx";
import {
  formatDashboardPlaceLabel,
  getDashboardPlaceLabelParts,
} from "../../components/ui/place-labels";
import { GraphSemanticIcon } from "./graph-semantic-icon";
import type { GraphSemanticIconKind } from "./graph-semantic-icon";
import { ActivityGraphNodeShell, type PlaceNodeType } from "./current-activity-node-shell";

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

export type CurrentActivityStatePositionNode = Node<StatePositionNodeData, "statePosition">;
export type CurrentActivityResourceNode = Node<ResourceNodeData, "resource">;
export type CurrentActivityConstraintNode = Node<ConstraintNodeData, "constraint">;
export type CurrentActivityPlaceNode =
  | CurrentActivityConstraintNode
  | CurrentActivityResourceNode
  | CurrentActivityStatePositionNode;

const STATE_NODE_DOT_LIMIT = 10;
const STATE_POSITION_CONTENT_CONTAINER_CLASSNAME =
  "grid min-w-0 w-full grid-rows-[1.5rem_auto] gap-[0.1rem] overflow-hidden";
const RESOURCE_CONTENT_CONTAINER_CLASSNAME =
  "grid min-w-0 w-full grid-rows-[1.5rem_auto] overflow-hidden";

export function StatePositionNodeView(props: NodeProps<CurrentActivityStatePositionNode>) {
  return <PlaceNodeView {...props} />;
}

export function ResourceNodeView(props: NodeProps<CurrentActivityResourceNode>) {
  return <PlaceNodeView {...props} />;
}

export function ConstraintNodeView(props: NodeProps<CurrentActivityConstraintNode>) {
  return <PlaceNodeView {...props} />;
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
      className={cx("justify-center text-left", nodeClassName)}
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
          <StatePositionNodeContent place={data.place} tokenCount={data.tokenCount} />
        </button>
      ) : showStateMarkers ? (
        <StatePositionNodeContent place={data.place} tokenCount={data.tokenCount} />
      ) : (
        <StaticPlaceNodeContent place={data.place} tokenCount={data.tokenCount} />
      )}
    </ActivityGraphNodeShell>
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
  dataPrefix,
  place,
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
}: {
  place: DashboardPlaceRef;
  tokenCount: number;
}) {
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
}: {
  place: DashboardPlaceRef;
  tokenCount: number;
}) {
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
