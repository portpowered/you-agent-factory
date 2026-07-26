import type { Node, NodeProps } from "@xyflow/react";
import type { ReactNode } from "react";

import type { DashboardPlaceRef } from "../../../api/dashboard/types";
import {
  formatDashboardPlaceLabel,
  getDashboardPlaceLabelParts,
} from "../../../components/ui/place-labels";
import { cn } from "../../../lib/cn";
import type { FactoryGraphNodeKind } from "../../factory-graph-editor/lib/draft/factory-graph-draft-types";
import {
  workStatePhaseSemanticIconClassName,
  workStatePhaseSemanticIconKind,
  workStatePhaseSurfaceClassName,
} from "../../factory-graph-editor/lib/work-state/factory-graph-work-state-phase-styling";
import { GraphNodeButton } from "@you-agent-factory/components/graphs";
import { getWorkflowActivityShellMessages } from "../../workflow-activity/messages/activity-shell";
import { currentActivityGraphNodeHoverClassName } from "../lib/current-activity-graph-hover";
import { getActivityGraphMessages } from "../messages/activity-graph";
import {
  ActivityGraphNodeBadge,
  activityGraphNodeSurfaceClassName,
} from "./current-activity-node-chrome";
import {
  type ActivityGraphNodeHandle,
  ActivityGraphNodeShell,
  type PlaceNodeType,
} from "./current-activity-node-shell";
import { CurrentActivityWorkProgressMarker } from "./current-activity-work-progress-marker";
import type { GraphSemanticIconKind } from "./graph-semantic-icon";
import { GraphSemanticIcon } from "./graph-semantic-icon";

export interface BasePlaceNodeData extends Record<string, unknown> {
  activeFlow: boolean;
  activeItemLabels: string[];
  factoryGraphNodeId?: string;
  handles: ActivityGraphNodeHandle[];
  kind?: FactoryGraphNodeKind;
  locale?: string;
  muted: boolean;
  onSelectStateNode?: (placeId: string) => void;
  place: DashboardPlaceRef;
  selectedStateNode: boolean;
  tokenCount: number;
  validationError?: boolean;
  validationMessage?: string;
}

export interface StatePositionNodeData extends BasePlaceNodeData {
  place: DashboardPlaceRef;
}

export interface ConstraintNodeData extends BasePlaceNodeData {
  place: DashboardPlaceRef;
}

export type CurrentActivityStatePositionNode = Node<
  StatePositionNodeData,
  "statePosition"
>;
export type CurrentActivityConstraintNode = Node<
  ConstraintNodeData,
  "constraint"
>;
export type CurrentActivityPlaceNode =
  | CurrentActivityConstraintNode
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
    currentActivityGraphNodeHoverClassName({
      activeFlow: data.activeFlow,
      muted: data.muted,
      selected: data.selectedStateNode,
      validationError: data.validationError,
    }),
    data.activeFlow &&
      !data.selectedStateNode &&
      !data.validationError &&
      "border-af-success-border shadow-af-success-chip",
    data.selectedStateNode &&
      !data.validationError &&
      "border-primary shadow-af-accent-selected",
    data.validationError &&
      "ring-2 ring-af-danger-border motion-safe:animate-pulse",
    data.muted && "opacity-[0.45]",
  );

  return (
    <ActivityGraphNodeShell
      className={cn("justify-center text-left", nodeClassName)}
      handles={data.handles}
      nodeType={nodeType}
    >
      {selectable ? (
        <GraphNodeButton
          aria-invalid={data.validationError ? true : undefined}
          aria-label={
            data.validationMessage ?? messages.selectStateLabel(placeLabel)
          }
          aria-pressed={data.selectedStateNode}
          className={STATE_POSITION_CONTENT_CONTAINER_CLASSNAME}
          data-selected-state={data.selectedStateNode ? "true" : undefined}
          title={data.validationMessage}
          onClick={(event) => {
            event.stopPropagation();
            data.onSelectStateNode?.(data.place.place_id);
          }}
        >
          <StatePositionNodeContent
            locale={data.locale}
            place={data.place}
            tokenCount={data.tokenCount}
          />
        </GraphNodeButton>
      ) : showStateMarkers ? (
        <StatePositionNodeContent
          locale={data.locale}
          place={data.place}
          tokenCount={data.tokenCount}
        />
      ) : (
        <StaticPlaceNodeContent
          locale={data.locale}
          place={data.place}
          tokenCount={data.tokenCount}
        />
      )}
    </ActivityGraphNodeShell>
  );
}

function placeNodeClassName(place: DashboardPlaceRef): string {
  if (place.kind === "work_state") {
    return workStatePhaseSurfaceClassName(place.state_category);
  }

  if (place.kind === "resource") {
    return cn(activityGraphNodeSurfaceClassName("resource"), "text-on-surface");
  }

  return cn(
    activityGraphNodeSurfaceClassName("info"),
    "border-dashed text-on-surface",
  );
}

function placeSemanticIconKind(
  place: DashboardPlaceRef,
): GraphSemanticIconKind {
  if (place.kind === "work_state") {
    return workStatePhaseSemanticIconKind(place.state_category);
  }

  if (place.kind === "resource") {
    return "resource";
  }

  return place.kind === "limit" ? "limit" : "constraint";
}

function placeSemanticIconLabel(
  place: DashboardPlaceRef,
  locale?: string,
): string {
  return getActivityGraphMessages(locale).placeSemanticIconLabel(place);
}

function placeSemanticIconClassName(place: DashboardPlaceRef): string {
  if (place.kind === "work_state") {
    return workStatePhaseSemanticIconClassName(place.state_category);
  }

  if (place.kind === "resource") {
    return "text-success";
  }

  return place.kind === "limit" ? "text-error" : "text-info";
}

function activeItemCountLabel(count: number, locale?: string): string {
  return getActivityGraphMessages(locale).activeItemCountLabel(count);
}

function statePositionMarkers(count: number, locale?: string): ReactNode {
  if (count === 0) {
    return null;
  }

  if (count > STATE_NODE_DOT_LIMIT) {
    return (
      <CurrentActivityWorkProgressMarker
        ariaLabel={activeItemCountLabel(count, locale)}
        className="inline-flex min-h-5 min-w-7 rounded-full px-2 text-[0.76rem]"
        count={count}
        data-state-work-progress="numeric"
        kind="numeric"
      />
    );
  }

  return (
    <CurrentActivityWorkProgressMarker
      ariaLabel={activeItemCountLabel(count, locale)}
      className="inline-grid grid-cols-[repeat(5,0.5rem)] justify-center gap-1"
      data-state-work-progress="dots"
      dotCount={count}
      dotDataAttribute="data-state-work-progress-dot"
      kind="dots"
    />
  );
}

function tokenCountLabel(
  place: DashboardPlaceRef,
  count: number,
  locale?: string,
): string {
  return getActivityGraphMessages(locale).tokenCountLabel(place, count);
}

function placeTokenCountDisplay(
  place: DashboardPlaceRef,
  count: number,
  locale?: string,
): ReactNode {
  return (
    <ActivityGraphNodeBadge
      aria-label={tokenCountLabel(place, count, locale)}
      className="w-fit"
      data-place-token-count
      role="status"
    >
      {count}
    </ActivityGraphNodeBadge>
  );
}

function PlaceSemanticIcon({
  locale,
  place,
}: {
  locale?: string;
  place: DashboardPlaceRef;
}) {
  const messages = getActivityGraphMessages(locale);

  return (
    <span
      className="flex min-h-4 shrink-0 items-center"
      data-place-semantic-icon
      title={messages.placeKindLabel(place)}
    >
      <GraphSemanticIcon
        className={cn("h-3.5 w-3.5", placeSemanticIconClassName(place))}
        kind={placeSemanticIconKind(place)}
        label={placeSemanticIconLabel(place, locale)}
        locale={locale}
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
        className="block min-w-0 overflow-hidden text-ellipsis whitespace-nowrap text-[0.62rem] font-bold uppercase leading-none text-on-surface-subtle"
        data-place-work-type={dataPrefix === "place" ? true : undefined}
        data-state-work-type={dataPrefix === "state" ? true : undefined}
        title={labelParts.workType}
      >
        {labelParts.workType}
      </span>
      <span
        className="block min-w-0 overflow-hidden truncate whitespace-nowrap font-mono text-[0.76rem] font-bold leading-[0.82rem] text-on-surface"
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
  locale,
  place,
  tokenCount,
}: {
  locale?: string;
  place: DashboardPlaceRef;
  tokenCount: number;
}) {
  const label = formatDashboardPlaceLabel(place);
  const marker = statePositionMarkers(tokenCount, locale);

  return (
    <>
      <span
        className="grid h-6 max-h-6 min-w-0 grid-cols-[auto_minmax(0,1fr)] items-center gap-1.5 overflow-hidden"
        data-state-label-zone
      >
        <PlaceSemanticIcon locale={locale} place={place} />
        <PlaceLabelText dataPrefix="state" place={place} />
      </span>
      <span
        className="flex min-h-5 w-full shrink-0 items-center justify-center overflow-hidden"
        data-state-marker-zone
        title={label}
      >
        {marker ?? (
          <span className="sr-only">
            {activeItemCountLabel(tokenCount, locale)}
          </span>
        )}
      </span>
    </>
  );
}

function StaticPlaceNodeContent({
  locale,
  place,
  tokenCount,
}: {
  locale?: string;
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
          <PlaceSemanticIcon locale={locale} place={place} />
          <strong className="block min-w-0 truncate whitespace-nowrap font-mono text-[0.86rem] font-bold leading-tight">
            {label}
          </strong>
        </span>
        <span
          className="flex min-h-4 w-full shrink-0 items-center justify-start overflow-hidden"
          data-place-marker-zone
          title={label}
        >
          {placeTokenCountDisplay(place, tokenCount, locale)}
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
        <PlaceSemanticIcon locale={locale} place={place} />
        <PlaceLabelText dataPrefix="place" place={place} />
      </span>
      <span
        className="flex min-h-5 w-full shrink-0 items-center justify-start overflow-hidden"
        data-place-marker-zone
        title={label}
      >
        {placeTokenCountDisplay(place, tokenCount, locale)}
      </span>
    </div>
  );
}
