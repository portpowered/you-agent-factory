// biome-ignore lint/style/noExcessiveLinesPerFile: support node renderers share one semantic data contract and presentation vocabulary.
import type { Node, NodeProps } from "@xyflow/react";
import { GraphNodeButton } from "@you-agent-factory/components/graphs";
import type { ComponentPropsWithoutRef, ReactNode } from "react";
import type { FactoryGraphNodeInteractionOverlay } from "./node-interaction-overlay.js";
import type { FactoryGraphNodeResizeControlsProps } from "./node-resize-controls.js";
import { GraphSemanticIcon } from "./semantic-icon.js";
import {
  type FactoryGraphNodeHandle,
  FactoryGraphNodeShell,
} from "./semantic-node-shell.js";
import {
  factoryGraphNodeHoverClassName,
  factoryGraphNodeSurfaceClassName,
  factoryGraphNodeVisualIconClassName,
  factoryGraphNodeWrappedTextClassName,
} from "./semantic-node-style.js";
import { resolveFactoryGraphVisualState } from "./visual-state.js";
import {
  factoryGraphUnknownWorkerType,
  factoryGraphWorkerIconClassName,
  factoryGraphWorkerIconKind,
} from "./worker-icon.js";

/** The portion of a Factory place needed by the original semantic node views. */
export interface FactoryGraphPlaceRef {
  kind?: string;
  place_id: string;
  state_value?: string | null;
  type_id?: string | null;
}

export interface FactoryGraphWorkerNodeData extends Record<string, unknown> {
  activeFlow: boolean;
  expanded?: boolean;
  focused?: boolean;
  factoryGraphNodeId?: string;
  handles: FactoryGraphNodeHandle[];
  interactionOverlay?: FactoryGraphNodeInteractionOverlay;
  kind: "worker";
  locale?: string;
  muted: boolean;
  onSelectWorker?: (workerName: string) => void;
  place: FactoryGraphPlaceRef;
  runnerId?: string | null;
  resizeControls?: FactoryGraphNodeResizeControlsProps;
  selectedWorker: boolean;
  validationError?: boolean;
  workerType?: string | null;
}

export type FactoryGraphWorkerNode = Node<FactoryGraphWorkerNodeData, "worker">;

export interface FactoryGraphWorkTypeNodeData extends Record<string, unknown> {
  activeFlow: boolean;
  expanded?: boolean;
  focused?: boolean;
  factoryGraphNodeId?: string;
  handles: FactoryGraphNodeHandle[];
  interactionOverlay?: FactoryGraphNodeInteractionOverlay;
  isDefaultWorkType?: boolean;
  kind: "work-type";
  locale?: string;
  muted: boolean;
  onSelectWorkType?: (workTypeName: string) => void;
  place: FactoryGraphPlaceRef;
  resizeControls?: FactoryGraphNodeResizeControlsProps;
  selectedWorkType?: boolean;
  validationError?: boolean;
  validationMessage?: string;
}

export type FactoryGraphWorkTypeNode = Node<
  FactoryGraphWorkTypeNodeData,
  "workType"
>;

export interface FactoryGraphResourceNodeData extends Record<string, unknown> {
  activeFlow: boolean;
  expanded?: boolean;
  focused?: boolean;
  factoryGraphNodeId?: string;
  handles: FactoryGraphNodeHandle[];
  interactionOverlay?: FactoryGraphNodeInteractionOverlay;
  kind: "resource";
  locale?: string;
  muted: boolean;
  onSelectResource?: (resourceName: string) => void;
  place: FactoryGraphPlaceRef;
  resizeControls?: FactoryGraphNodeResizeControlsProps;
  selectedResource: boolean;
  tokenCount: number;
  validationError?: boolean;
}

export type FactoryGraphResourceNode = Node<
  FactoryGraphResourceNodeData,
  "resource"
>;

/** Original Factory worker node, with host-owned worker selection. */
export function FactoryGraphWorkerNodeView({
  data,
  selected: reactFlowSelected,
}: NodeProps<FactoryGraphWorkerNode>) {
  const workerName = resolveWorkerName(data);
  const unknownWorkerType = factoryGraphUnknownWorkerType(data.workerType);
  const label = unknownWorkerType
    ? `worker:${workerName} (${unknownWorkerType})`
    : `worker:${workerName}`;
  const workerLabel = semanticLabel("worker", data.locale);
  const workerKindLabel = unknownWorkerType ?? workerLabel;
  const workerIconKind = factoryGraphWorkerIconKind(
    data.workerType,
    data.runnerId,
  );
  const selectable = data.onSelectWorker !== undefined;
  const selected = data.selectedWorker || reactFlowSelected;
  const visualState = resolveFactoryGraphVisualState({
    activeFlow: data.activeFlow,
    family: "worker",
    focused: data.focused,
    muted: data.muted,
    selected,
    validation: data.validationError,
  });
  const content = (
    <span
      aria-label={label}
      className="flex h-full min-h-0 min-w-0 items-center gap-1.5 overflow-hidden"
      data-factory-entity-semantic-icon
      data-worker-label-zone
      role="img"
      title={label}
    >
      <span className="sr-only">{label}</span>
      <GraphSemanticIcon
        className={classNames(
          "h-3.5 w-3.5 shrink-0",
          factoryGraphWorkerIconClassName(
            visualState,
            unknownWorkerType ? "text-on-surface-variant" : "text-info",
          ),
        )}
        kind={workerIconKind}
        label={workerLabel}
      />
      <span className="grid min-w-0 gap-px overflow-hidden">
        <span
          className={factoryGraphNodeWrappedTextClassName(
            classNames(
              "block overflow-hidden text-[0.62rem] font-bold uppercase leading-none",
              unknownWorkerType ? "text-on-surface-variant" : "text-info",
            ),
          )}
          data-worker-kind-label
        >
          {workerKindLabel}
        </span>
        <strong
          className={factoryGraphNodeWrappedTextClassName(
            "block font-mono text-[0.8rem] font-bold leading-tight text-on-surface",
          )}
          data-factory-entity-title
          title={workerName}
        >
          {workerName}
        </strong>
      </span>
    </span>
  );
  return (
    <FactoryGraphNodeShell
      className={classNames(
        factoryGraphNodeSurfaceClassName(
          unknownWorkerType ? "neutral" : "info",
        ),
        "justify-center text-left text-on-surface",
        factoryGraphNodeHoverClassName({
          activeFlow: data.activeFlow,
          muted: data.muted,
          selected,
        }),
      )}
      handles={data.handles}
      interactionOverlay={data.interactionOverlay}
      nodeType="worker"
      resizeControls={data.resizeControls}
      visualState={{
        activeFlow: data.activeFlow,
        focused: data.focused,
        muted: data.muted,
        selected,
        validation: data.validationError,
      }}
    >
      {selectable ? (
        <GraphNodeButton
          aria-label={selectLabel(
            "worker",
            unknownWorkerType
              ? `${workerName} (${unknownWorkerType})`
              : workerName,
            data.locale,
          )}
          aria-pressed={selected}
          className="grid h-full min-h-0 min-w-0 place-content-center gap-0.5 overflow-hidden"
          data-selected-worker={selected ? "true" : undefined}
          onClick={(event) => {
            event.stopPropagation();
            data.onSelectWorker?.(workerName);
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

/** Original Factory work-type node, with host-owned selection and validation. */
export function FactoryGraphWorkTypeNodeView({
  data,
  selected: reactFlowSelected,
}: NodeProps<FactoryGraphWorkTypeNode>) {
  const name = workTypeName(data.place);
  const label = `work-type:${name}`;
  const workTypeLabel = semanticLabel("work-type", data.locale);
  const selectable = data.onSelectWorkType !== undefined;
  const selected = (data.selectedWorkType ?? false) || reactFlowSelected;
  const visualState = resolveFactoryGraphVisualState({
    activeFlow: data.activeFlow,
    family: "work-type",
    focused: data.focused,
    muted: data.muted,
    selected,
    validation: data.validationError,
  });
  const content = (
    <span
      aria-hidden={selectable ? true : undefined}
      className="flex min-w-0 items-center gap-1.5 overflow-hidden"
      data-work-type-label-zone
      {...(selectable ? {} : { "aria-label": label, role: "img" as const })}
      title={data.validationMessage ?? label}
    >
      {selectable ? null : <span className="sr-only">{label}</span>}
      <GraphSemanticIcon
        className={classNames(
          "h-3.5 w-3.5 shrink-0",
          factoryGraphNodeVisualIconClassName(visualState, "text-info"),
        )}
        kind="work-type"
        label={workTypeLabel}
      />
      <span className="grid min-w-0 gap-px overflow-hidden">
        <span className="flex min-w-0 items-start gap-1 overflow-hidden">
          <span
            className={factoryGraphNodeWrappedTextClassName(
              "block overflow-hidden text-[0.62rem] font-bold uppercase leading-none text-info",
            )}
          >
            {workTypeLabel}
          </span>
          {data.isDefaultWorkType ? (
            <FactoryGraphNodeBadge
              className="max-w-full shrink"
              role="status"
              tone="info"
              weight="label"
            >
              {defaultWorkTypeLabel(data.locale)}
            </FactoryGraphNodeBadge>
          ) : null}
        </span>
        <strong
          className={factoryGraphNodeWrappedTextClassName(
            "block font-mono text-[0.8rem] font-bold leading-tight text-on-surface",
          )}
        >
          {name}
        </strong>
      </span>
    </span>
  );
  return (
    <FactoryGraphNodeShell
      className={classNames(
        factoryGraphNodeSurfaceClassName("info"),
        "justify-center border-dashed text-left text-on-surface",
        factoryGraphNodeHoverClassName({
          activeFlow: data.activeFlow,
          muted: data.muted,
          selected,
          validationError: data.validationError,
        }),
      )}
      handles={data.handles}
      interactionOverlay={data.interactionOverlay}
      nodeType="workType"
      resizeControls={data.resizeControls}
      visualState={{
        activeFlow: data.activeFlow,
        focused: data.focused,
        muted: data.muted,
        selected,
        validation: data.validationError,
      }}
    >
      {selectable ? (
        <GraphNodeButton
          aria-invalid={data.validationError ? true : undefined}
          aria-label={selectLabel("work type", name, data.locale)}
          aria-pressed={selected}
          className="grid min-w-0 gap-0.5 overflow-hidden"
          data-selected-work-type={selected ? "true" : undefined}
          onClick={(event) => {
            event.stopPropagation();
            data.onSelectWorkType?.(name);
          }}
        >
          {content}
        </GraphNodeButton>
      ) : (
        <div className="grid min-w-0 gap-0.5 overflow-hidden">{content}</div>
      )}
    </FactoryGraphNodeShell>
  );
}

/** Original Factory resource node, with host-owned resource selection. */
export function FactoryGraphResourceNodeView({
  data,
  selected: reactFlowSelected,
}: NodeProps<FactoryGraphResourceNode>) {
  const label = resourceName(data.place);
  const resourceLabel = semanticLabel("resource", data.locale);
  const selectable = data.onSelectResource !== undefined;
  const selected = data.selectedResource || reactFlowSelected;
  const visualState = resolveFactoryGraphVisualState({
    activeFlow: data.activeFlow,
    family: "resource",
    focused: data.focused,
    muted: data.muted,
    selected,
    validation: data.validationError,
  });
  const content = (
    <FactoryGraphResourceNodeContent
      label={label}
      locale={data.locale}
      place={data.place}
      resourceLabel={resourceLabel}
      tokenCount={data.tokenCount}
      visualState={visualState}
    />
  );
  return (
    <FactoryGraphNodeShell
      className={classNames(
        factoryGraphNodeSurfaceClassName("resource"),
        "justify-center text-left text-on-surface",
        factoryGraphNodeHoverClassName({
          activeFlow: data.activeFlow,
          muted: data.muted,
          selected,
        }),
      )}
      handles={data.handles}
      interactionOverlay={data.interactionOverlay}
      nodeType="resource"
      resizeControls={data.resizeControls}
      visualState={{
        activeFlow: data.activeFlow,
        focused: data.focused,
        muted: data.muted,
        selected,
        validation: data.validationError,
      }}
    >
      {selectable ? (
        <GraphNodeButton
          aria-label={selectLabel("resource", label, data.locale)}
          aria-pressed={selected}
          className="flex min-w-0 w-full flex-col overflow-hidden"
          data-selected-resource={selected ? "true" : undefined}
          onClick={(event) => {
            event.stopPropagation();
            data.onSelectResource?.(label);
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

function FactoryGraphResourceNodeContent({
  label,
  locale,
  place,
  resourceLabel,
  tokenCount,
  visualState,
}: {
  label: string;
  locale?: string;
  place: FactoryGraphPlaceRef;
  resourceLabel: string;
  tokenCount: number;
  visualState: ReturnType<typeof resolveFactoryGraphVisualState>;
}) {
  return (
    <div className="flex min-w-0 w-full flex-col overflow-hidden">
      <span
        aria-label={label}
        className="grid min-h-6 min-w-0 grid-cols-[auto_minmax(0,1fr)] items-start gap-1.5 overflow-hidden"
        data-resource-label-zone
        role="img"
      >
        <span
          className="flex min-h-4 shrink-0 items-center"
          title={resourceLabel}
        >
          <GraphSemanticIcon
            className={classNames(
              "h-3.5 w-3.5",
              factoryGraphNodeVisualIconClassName(visualState, "text-success"),
            )}
            kind="resource"
            label={resourceLabel}
          />
        </span>
        <span className="flex min-w-0 overflow-hidden" title={label}>
          <span
            className={factoryGraphNodeWrappedTextClassName(
              "block overflow-hidden font-mono text-[0.76rem] font-bold leading-[0.82rem] text-on-surface",
            )}
            data-resource-name
            title={label}
          >
            {label}
          </span>
        </span>
      </span>
      <span
        className="flex min-h-5 w-full shrink-0 items-center justify-start overflow-hidden"
        data-resource-token-zone
        title={label}
      >
        <FactoryGraphNodeBadge
          aria-label={tokenCountLabel(place, tokenCount, locale)}
          className="w-fit"
          data-resource-token-count
          role="status"
        >
          {tokenCount}
        </FactoryGraphNodeBadge>
      </span>
    </div>
  );
}

export function FactoryGraphNodeBadge({
  children,
  className,
  tone = "neutral",
  weight = "body",
  ...rest
}: ComponentPropsWithoutRef<"span"> & {
  children: ReactNode;
  tone?: "danger" | "info" | "neutral" | "success" | "warning";
  weight?: "body" | "label";
}) {
  const toneClass = {
    danger:
      "border-af-danger-border bg-error-container text-on-error-container",
    info: "border-af-info-border bg-info-container text-on-info-container",
    neutral: "border-outline bg-surface-container-low text-on-surface-variant",
    success:
      "border-af-success-border bg-success-container text-on-success-container",
    warning:
      "border-af-warning-border bg-warning-container text-on-warning-container",
  }[tone];
  return (
    <span
      className={classNames(
        "inline-flex min-h-6 w-fit items-center justify-center gap-1 rounded-full border px-2 py-0.5 font-semibold leading-none",
        toneClass,
        weight === "body"
          ? "font-mono text-[0.68rem]"
          : "text-[0.65rem] font-semibold uppercase tracking-[0.08em]",
        className,
      )}
      {...rest}
    >
      {children}
    </span>
  );
}

function resolveWorkerName(data: FactoryGraphWorkerNodeData): string {
  return (
    data.place.state_value ??
    data.factoryGraphNodeId?.replace(/^worker:/, "") ??
    data.place.place_id.replace(/^place:worker:/, "").replace(/^worker:/, "")
  );
}
function workTypeName(place: FactoryGraphPlaceRef): string {
  return typeof place.state_value === "string" &&
    place.state_value.trim().length > 0
    ? place.state_value
    : place.place_id.replace(/^work-type:/, "");
}
function resourceName(place: FactoryGraphPlaceRef): string {
  return typeof place.type_id === "string" && place.type_id.trim().length > 0
    ? place.type_id
    : place.place_id.replace(/:available$/, "");
}
function classNames(
  ...values: Array<string | false | null | undefined>
): string {
  return values.filter(Boolean).join(" ");
}
function semanticLabel(
  kind: "resource" | "worker" | "work-type",
  locale?: string,
): string {
  const chinese = locale === "zh-CN";
  return kind === "resource"
    ? chinese
      ? "资源"
      : "Resource"
    : kind === "worker"
      ? chinese
        ? "工作者"
        : "Worker"
      : chinese
        ? "工作类型"
        : "Work type";
}
function defaultWorkTypeLabel(locale?: string): string {
  return locale === "zh-CN" ? "默认工作类型" : "Default work type";
}
function selectLabel(kind: string, name: string, locale?: string): string {
  return locale === "zh-CN"
    ? `选择 ${name} ${kind === "work type" ? "工作类型" : kind === "worker" ? "工作者" : "资源"}`
    : `Select ${name} ${kind}`;
}
function tokenCountLabel(
  _place: FactoryGraphPlaceRef,
  count: number,
  locale?: string,
): string {
  return locale === "zh-CN"
    ? `${count} 个资源令牌`
    : `${count} resource tokens`;
}
