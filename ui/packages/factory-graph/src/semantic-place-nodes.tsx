import type { Node, NodeProps } from "@xyflow/react";
import { GraphNodeButton } from "@you-agent-factory/components/graphs";
import type { ComponentPropsWithoutRef, ReactNode } from "react";
import type { FactoryGraphNodeInteractionOverlay } from "./node-interaction-overlay.js";
import type { FactoryGraphNodeResizeControlsProps } from "./node-resize-controls.js";
import {
  type FactoryGraphNodeHandle,
  FactoryGraphNodeShell,
  type FactoryGraphPlaceNodeType,
} from "./semantic-node-shell.js";
import {
  factoryGraphNodeHoverClassName,
  factoryGraphNodeSurfaceClassName,
  factoryGraphNodeWrappedTextClassName,
} from "./semantic-node-style.js";
import {
  FactoryGraphPlaceLabelText,
  FactoryGraphPlaceSemanticIcon,
  factoryGraphPlaceKindLabel,
  factoryGraphPlaceLabel,
} from "./semantic-place-content.js";
import { FactoryGraphPlaceTokenCount } from "./semantic-place-token-count.js";
import type { FactoryGraphPlaceRef } from "./semantic-support-nodes.js";
import { resolveFactoryGraphVisualState } from "./visual-state.js";
import { factoryGraphWorkProgressMode } from "./work-progress-presentation.js";
import {
  type FactoryGraphWorkStateTypeValue,
  factoryGraphUnknownWorkStateType,
  workStatePhaseSurfaceClassName,
} from "./work-state-presentation.js";

export interface FactoryGraphSemanticPlaceRef extends FactoryGraphPlaceRef {
  kind: "constraint" | "limit" | "resource" | "work_state" | (string & {});
  state_category?: FactoryGraphWorkStateTypeValue;
}

export interface FactoryGraphBasePlaceNodeData extends Record<string, unknown> {
  activeFlow: boolean;
  activeItemLabels: string[];
  focused?: boolean;
  factoryGraphNodeId?: string;
  handles: FactoryGraphNodeHandle[];
  interactionOverlay?: FactoryGraphNodeInteractionOverlay;
  kind?: string;
  locale?: string;
  muted: boolean;
  onSelectStateNode?: (placeId: string) => void;
  place: FactoryGraphSemanticPlaceRef;
  selectedStateNode: boolean;
  tokenCount: number;
  validationError?: boolean;
  validationMessage?: string;
  resizeControls?: FactoryGraphNodeResizeControlsProps;
}

export interface FactoryGraphStatePositionNodeData
  extends FactoryGraphBasePlaceNodeData {}
export interface FactoryGraphConstraintNodeData
  extends FactoryGraphBasePlaceNodeData {}
export type FactoryGraphStatePositionNode = Node<
  FactoryGraphStatePositionNodeData,
  "statePosition"
>;
export type FactoryGraphConstraintNode = Node<
  FactoryGraphConstraintNodeData,
  "constraint"
>;
export type FactoryGraphPlaceNode =
  | FactoryGraphConstraintNode
  | FactoryGraphStatePositionNode;

const CONTENT_CLASS = "flex min-w-0 w-full flex-col gap-0.5 overflow-hidden";

export function FactoryGraphStatePositionNodeView(
  props: NodeProps<FactoryGraphStatePositionNode>,
) {
  return <FactoryGraphPlaceNodeView {...props} />;
}
export function FactoryGraphConstraintNodeView(
  props: NodeProps<FactoryGraphConstraintNode>,
) {
  return <FactoryGraphPlaceNodeView {...props} />;
}

function FactoryGraphPlaceNodeView({
  data,
  selected: reactFlowSelected,
}: NodeProps<FactoryGraphPlaceNode>) {
  const placeLabel = factoryGraphPlaceLabel(data.place);
  const selected = data.selectedStateNode || reactFlowSelected;
  const selectable =
    data.place.kind === "work_state" && data.onSelectStateNode !== undefined;
  const stateNode = data.place.kind === "work_state";
  const nodeType: FactoryGraphPlaceNodeType = stateNode
    ? "statePosition"
    : data.place.kind === "resource"
      ? "resource"
      : "constraint";
  const className = classNames(
    placeNodeClassName(data.place),
    factoryGraphNodeHoverClassName({
      activeFlow: data.activeFlow,
      muted: data.muted,
      selected,
      validationError: data.validationError,
    }),
  );
  const holdsWork = data.tokenCount > 0;
  const visualState = resolveFactoryGraphVisualState({
    activeFlow: data.activeFlow,
    activeWork: holdsWork,
    family: stateNode
      ? "work-state"
      : data.place.kind === "resource"
        ? "resource"
        : "constraint",
    focused: data.focused,
    lifecycle: stateNode ? data.place.state_category : undefined,
    muted: data.muted,
    selected,
    validation: data.validationError,
  });
  const content = stateNode ? (
    <FactoryGraphStatePositionContent
      locale={data.locale}
      place={data.place}
      tokenCount={data.tokenCount}
      visualState={visualState}
    />
  ) : (
    <FactoryGraphStaticPlaceContent
      locale={data.locale}
      place={data.place}
      tokenCount={data.tokenCount}
      visualState={visualState}
    />
  );
  return (
    <FactoryGraphNodeShell
      className={classNames("justify-center text-left", className)}
      handles={data.handles}
      interactionOverlay={data.interactionOverlay}
      nodeType={nodeType}
      resizeControls={
        data.resizeControls
          ? { ...data.resizeControls, isVisible: selected }
          : undefined
      }
      visualState={{
        activeFlow: data.activeFlow,
        activeWork: holdsWork,
        focused: data.focused,
        lifecycle: stateNode ? data.place.state_category : undefined,
        muted: data.muted,
        selected,
        validation: data.validationError,
      }}
    >
      {selectable ? (
        <GraphNodeButton
          aria-invalid={data.validationError ? true : undefined}
          aria-label={
            data.validationMessage ?? selectStateLabel(placeLabel, data.locale)
          }
          aria-pressed={selected}
          className={CONTENT_CLASS}
          data-selected-state={selected ? "true" : undefined}
          title={data.validationMessage}
          onClick={(event) => {
            event.stopPropagation();
            data.onSelectStateNode?.(data.place.place_id);
          }}
        >
          {content}
        </GraphNodeButton>
      ) : (
        content
      )}
    </FactoryGraphNodeShell>
  );
}

function FactoryGraphStatePositionContent({
  locale,
  place,
  tokenCount,
  visualState,
}: {
  locale?: string;
  place: FactoryGraphSemanticPlaceRef;
  tokenCount: number;
  visualState: ReturnType<typeof resolveFactoryGraphVisualState>;
}) {
  const label = factoryGraphPlaceLabel(place);
  const unknownWorkStateType = factoryGraphUnknownWorkStateType(
    place.state_category,
  );
  return (
    <>
      <span
        className="grid min-h-6 min-w-0 grid-cols-[auto_minmax(0,1fr)] items-start gap-1.5 overflow-hidden"
        data-state-label-zone
      >
        <FactoryGraphPlaceSemanticIcon
          locale={locale}
          place={place}
          visualState={visualState}
        />
        <span className="grid min-w-0 gap-px overflow-hidden">
          {unknownWorkStateType ? (
            <span
              className="block overflow-hidden text-[0.62rem] font-bold uppercase leading-none text-on-surface-variant"
              data-state-category-label
              title={unknownWorkStateType}
            >
              {unknownWorkStateType}
            </span>
          ) : null}
          <FactoryGraphPlaceLabelText dataPrefix="state" place={place} />
        </span>
      </span>
      <span
        className="flex min-h-5 w-full shrink-0 items-center justify-center overflow-hidden"
        data-state-marker-zone
        title={label}
      >
        {stateMarkers(tokenCount, locale) ?? (
          <span className="sr-only">
            {activeItemCountLabel(tokenCount, locale)}
          </span>
        )}
      </span>
    </>
  );
}

function FactoryGraphStaticPlaceContent({
  locale,
  place,
  tokenCount,
  visualState,
}: {
  locale?: string;
  place: FactoryGraphSemanticPlaceRef;
  tokenCount: number;
  visualState: ReturnType<typeof resolveFactoryGraphVisualState>;
}) {
  const label = factoryGraphPlaceLabel(place);
  if (place.kind !== "resource")
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
          <FactoryGraphPlaceSemanticIcon
            locale={locale}
            place={place}
            visualState={visualState}
          />
          <strong
            className={factoryGraphNodeWrappedTextClassName(
              "block font-mono text-[0.86rem] font-bold leading-tight",
            )}
          >
            {label}
          </strong>
        </span>
        <span
          className="flex min-h-4 w-full shrink-0 items-center justify-start overflow-hidden"
          data-place-marker-zone
          title={label}
        >
          <FactoryGraphPlaceTokenCount
            ariaLabel={tokenCountLabel(place, tokenCount, locale)}
            count={tokenCount}
          />
        </span>
      </div>
    );
  return (
    <div
      className="flex min-w-0 w-full flex-col overflow-hidden"
      data-place-label-container
    >
      <span
        aria-label={label}
        className="grid min-h-6 min-w-0 grid-cols-[auto_minmax(0,1fr)] items-start gap-1.5 overflow-hidden"
        data-place-label-zone
        role="img"
      >
        <FactoryGraphPlaceSemanticIcon
          locale={locale}
          place={place}
          visualState={visualState}
        />
        <FactoryGraphPlaceLabelText dataPrefix="place" place={place} />
      </span>
      <span
        className="flex min-h-5 w-full shrink-0 items-center justify-start overflow-hidden"
        data-place-marker-zone
        title={label}
      >
        <FactoryGraphPlaceTokenCount
          ariaLabel={tokenCountLabel(place, tokenCount, locale)}
          count={tokenCount}
        />
      </span>
    </div>
  );
}

function stateMarkers(count: number, locale?: string): ReactNode {
  const mode = factoryGraphWorkProgressMode(count);
  if (mode === "empty") return null;
  return mode === "total" ? (
    <FactoryGraphWorkProgressMarker
      ariaLabel={activeItemCountLabel(count, locale)}
      className="inline-flex min-h-5 min-w-8 rounded-full px-2 text-sm"
      count={count}
      data-state-work-progress="numeric"
      kind="numeric"
    />
  ) : (
    <FactoryGraphWorkProgressMarker
      ariaLabel={activeItemCountLabel(count, locale)}
      className="inline-grid grid-cols-[repeat(5,0.5rem)] justify-center gap-1"
      data-state-work-progress="dots"
      dotCount={count}
      dotDataAttribute="data-state-work-progress-dot"
      kind="dots"
    />
  );
}

export function FactoryGraphWorkProgressMarker(
  props:
    | ({
        ariaLabel: string;
        className?: string;
        count: number;
        kind: "numeric";
      } & ComponentPropsWithoutRef<"span">)
    | ({
        ariaLabel: string;
        className?: string;
        dotClassName?: string;
        dotCount: number;
        dotDataAttribute: string;
        kind: "dots";
        suffix?: ReactNode;
      } & ComponentPropsWithoutRef<"span">),
) {
  if (props.kind === "numeric") {
    const { ariaLabel, className, count, kind: _kind, ...rest } = props;
    return (
      <span
        aria-label={ariaLabel}
        className={classNames(
          "items-center justify-center border border-af-success-border bg-success-container font-mono font-bold leading-none text-success",
          className,
        )}
        role="status"
        {...rest}
      >
        {count}
      </span>
    );
  }
  const {
    ariaLabel,
    className,
    dotClassName = "h-2 w-2",
    dotCount,
    dotDataAttribute,
    kind: _kind,
    suffix,
    ...rest
  } = props;
  return (
    <span
      aria-label={ariaLabel}
      className={classNames(
        "items-center justify-center border border-af-success-border bg-success-container",
        className,
      )}
      role="status"
      {...rest}
    >
      {Array.from({ length: dotCount }, (_, index) => `dot-${index}`).map(
        (key, index) => (
          <span
            key={key}
            aria-hidden="true"
            className={classNames("rounded-full bg-success", dotClassName)}
            data-current-activity-work-progress-dot={String(index)}
            {...{ [dotDataAttribute]: String(index) }}
          />
        ),
      )}
      {suffix}
    </span>
  );
}

function placeNodeClassName(place: FactoryGraphSemanticPlaceRef): string {
  return place.kind === "work_state"
    ? workStatePhaseSurfaceClassName(place.state_category)
    : place.kind === "resource"
      ? classNames(
          factoryGraphNodeSurfaceClassName("resource"),
          "text-on-surface",
        )
      : classNames(
          factoryGraphNodeSurfaceClassName("info"),
          "border-dashed text-on-surface",
        );
}
function classNames(
  ...values: Array<string | false | null | undefined>
): string {
  return values.filter(Boolean).join(" ");
}
function activeItemCountLabel(count: number, locale?: string): string {
  return locale === "zh-CN"
    ? `${count} 个活动项`
    : `${count} active ${count === 1 ? "item" : "items"}`;
}
function selectStateLabel(label: string, locale?: string): string {
  return locale === "zh-CN" ? `选择 ${label} 状态` : `Select ${label} state`;
}
function tokenCountLabel(
  place: FactoryGraphSemanticPlaceRef,
  count: number,
  locale?: string,
): string {
  if (locale === "zh-CN")
    return `${count} 个${factoryGraphPlaceKindLabel(place, locale)}令牌`;
  const token = count === 1 ? "token" : "tokens";
  return `${count} ${factoryGraphPlaceKindLabel(place, locale).toLowerCase()} ${token}`;
}
