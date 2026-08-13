import type { GraphSemanticIconKind } from "./semantic-icon.js";
import { GraphSemanticIcon } from "./semantic-icon.js";
import { factoryGraphNodeVisualIconClassName } from "./semantic-node-style.js";
import type { FactoryGraphSemanticPlaceRef } from "./semantic-place-nodes.js";
import type { FactoryGraphVisualState } from "./visual-state.js";
import {
  workStatePhaseSemanticIconClassName,
  workStatePhaseSemanticIconKind,
} from "./work-state-presentation.js";

export function FactoryGraphPlaceSemanticIcon({
  locale,
  place,
  visualState,
}: {
  locale?: string;
  place: FactoryGraphSemanticPlaceRef;
  visualState: FactoryGraphVisualState;
}) {
  const kind = placeIconKind(place);
  const className = placeIconClassName(place);
  const label = factoryGraphPlaceSemanticLabel(place, locale);
  return (
    <span
      className="flex min-h-4 shrink-0 items-center"
      data-place-semantic-icon
      title={factoryGraphPlaceKindLabel(place, locale)}
    >
      <GraphSemanticIcon
        className={classNames(
          "h-3.5 w-3.5",
          factoryGraphNodeVisualIconClassName(visualState, className),
        )}
        kind={kind}
        label={label}
      />
    </span>
  );
}

export function FactoryGraphPlaceLabelText({
  dataPrefix,
  place,
}: {
  dataPrefix: "place" | "state";
  place: FactoryGraphSemanticPlaceRef;
}) {
  const label = factoryGraphPlaceLabel(place);
  const parts = placeLabelParts(place);
  return (
    <span className="grid min-w-0 gap-px overflow-hidden" title={label}>
      <span
        className="block min-w-0 overflow-hidden text-ellipsis whitespace-nowrap text-[0.62rem] font-bold uppercase leading-none text-on-surface-subtle"
        data-place-work-type={dataPrefix === "place" ? true : undefined}
        data-state-work-type={dataPrefix === "state" ? true : undefined}
        title={parts.workType}
      >
        {parts.workType}
      </span>
      <span
        className="block min-w-0 overflow-hidden truncate whitespace-nowrap font-mono text-[0.76rem] font-bold leading-[0.82rem] text-on-surface"
        data-place-state-value={dataPrefix === "place" ? true : undefined}
        data-state-value={dataPrefix === "state" ? true : undefined}
        title={parts.stateValue}
      >
        {parts.stateValue}
      </span>
    </span>
  );
}

export function factoryGraphPlaceLabel(
  place: FactoryGraphSemanticPlaceRef,
): string {
  return place.type_id && place.state_value
    ? `${place.type_id}:${place.state_value}`
    : place.place_id;
}

export function factoryGraphPlaceKindLabel(
  place: FactoryGraphSemanticPlaceRef,
  locale?: string,
): string {
  const chinese = locale === "zh-CN";
  if (place.kind === "work_state")
    return place.state_category === "TERMINAL"
      ? chinese
        ? "终止状态"
        : "Terminal"
      : place.state_category === "FAILED"
        ? chinese
          ? "失败状态"
          : "Failed"
        : chinese
          ? "队列"
          : "Queue";
  if (place.kind === "resource") return chinese ? "资源" : "Resource";
  return place.kind === "limit"
    ? chinese
      ? "限制"
      : "Limit"
    : chinese
      ? "约束"
      : "Constraint";
}

function factoryGraphPlaceSemanticLabel(
  place: FactoryGraphSemanticPlaceRef,
  locale?: string,
): string {
  return place.kind === "work_state" && place.state_category === "PROCESSING"
    ? locale === "zh-CN"
      ? "处理中状态"
      : "Processing state"
    : factoryGraphPlaceKindLabel(place, locale);
}

function placeLabelParts(place: FactoryGraphSemanticPlaceRef) {
  return {
    stateValue: place.state_value ?? place.place_id,
    workType: place.type_id ?? "work",
  };
}

function placeIconKind(
  place: FactoryGraphSemanticPlaceRef,
): GraphSemanticIconKind {
  return place.kind === "work_state"
    ? workStatePhaseSemanticIconKind(place.state_category)
    : place.kind === "resource"
      ? "resource"
      : place.kind === "limit"
        ? "limit"
        : "constraint";
}

function placeIconClassName(place: FactoryGraphSemanticPlaceRef): string {
  return place.kind === "work_state"
    ? workStatePhaseSemanticIconClassName(place.state_category)
    : place.kind === "resource"
      ? "text-success"
      : place.kind === "limit"
        ? "text-error"
        : "text-info";
}

function classNames(...values: Array<string | false | null | undefined>) {
  return values.filter(Boolean).join(" ");
}
