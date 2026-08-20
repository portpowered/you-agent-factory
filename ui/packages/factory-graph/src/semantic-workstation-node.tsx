import type { Node, NodeProps } from "@xyflow/react";
import { GraphNodeButton } from "@you-agent-factory/components/graphs";
import type { FactoryGraphNodeInteractionOverlay } from "./node-interaction-overlay.js";
import type { FactoryGraphNodeResizeControlsProps } from "./node-resize-controls.js";
import {
  type FactoryGraphNodeHandle,
  FactoryGraphNodeShell,
  type FactoryGraphZAxisIncompleteHints,
} from "./semantic-node-shell.js";
import {
  factoryGraphNodeHoverClassName,
  factoryGraphNodeSurfaceClassName,
  factoryGraphNodeWrappedTextClassName,
} from "./semantic-node-style.js";
import { FactoryGraphWorkProgressMarker } from "./semantic-place-nodes.js";
import {
  factoryGraphActiveItemsLabel as activeItemsLabel,
  factoryGraphClassNames as classNames,
  factoryGraphDurationText as durationText,
  type FactoryGraphWorkItemRef,
  type FactoryGraphWorkstationRef,
  factoryGraphGraphDuration as graphDuration,
  factoryGraphSelectWorkstationLabel as selectWorkstationLabel,
  type FactoryGraphWorkstationPresentation as WorkstationPresentation,
  factoryGraphWorkItemLabel as workItemLabel,
  factoryGraphWorkItemLabelClassName as workItemLabelClassName,
  factoryGraphWorkstationPresentation as workstationPresentation,
  factoryGraphWorkstationTitleClassName as workstationTitleClassName,
} from "./semantic-workstation-presentation.js";
import { resolveFactoryGraphVisualState } from "./visual-state.js";
import { factoryGraphWorkProgressMode } from "./work-progress-presentation.js";
import type { FactoryGraphWorkstationSemantics } from "./workstation-semantics.js";

export type {
  FactoryGraphWorkItemRef,
  FactoryGraphWorkstationRef,
} from "./semantic-workstation-presentation.js";
export interface FactoryGraphActiveExecution {
  dispatch_id: string;
  started_at: string;
  work_items?: FactoryGraphWorkItemRef[];
}
export interface FactoryGraphWorkstationNodeData
  extends Record<string, unknown> {
  active: boolean;
  activeFlow: boolean;
  expanded?: boolean;
  focused?: boolean;
  executions: FactoryGraphActiveExecution[];
  factoryGraphNodeId?: string;
  handles: FactoryGraphNodeHandle[];
  interactionOverlay?: FactoryGraphNodeInteractionOverlay;
  kind?: "workstation";
  locale?: string;
  muted: boolean;
  now: number;
  progressOutcomeRouteWorkstation?: unknown;
  runtimeStatus?: string | null;
  selectedWorkID: string | null;
  selectedWorkstation: boolean;
  resizeControls?: FactoryGraphNodeResizeControlsProps;
  summaryOnly?: boolean;
  workstation: FactoryGraphWorkstationRef;
  workstationSemantics?: FactoryGraphWorkstationSemantics;
  zAxisIncompleteHints?: FactoryGraphZAxisIncompleteHints | null;
  onSelectWorkstation?: (nodeId: string) => void;
  onSelectWorkID?: (
    workID: string,
    hint?: { dispatchID?: string; nodeID?: string },
  ) => void;
  validationError?: boolean;
}
export type FactoryGraphWorkstationNode = Node<
  FactoryGraphWorkstationNodeData,
  "workstation"
>;

const WORKSTATION_WORK_ITEM_MODE_MAXIMUM = 2;
const WORKSTATION_HEADER_CLASS_NAME =
  "flex min-w-0 w-full flex-wrap items-start justify-between gap-1 overflow-hidden";

/** Original Factory workstation presentation, with host-owned selection callbacks. */
export function FactoryGraphWorkstationNodeView({
  data,
  selected: reactFlowSelected,
}: NodeProps<FactoryGraphWorkstationNode>) {
  const presentation = workstationPresentation(
    data.workstationSemantics,
    data.locale,
  );
  const title =
    data.workstation.workstation_name ||
    data.workstation.transition_id ||
    data.workstation.node_id;
  const isExpanded = data.expanded === true;
  const entries = data.executions.flatMap((execution) =>
    (execution.work_items ?? []).map((workItem) => ({ execution, workItem })),
  );
  const selected = data.selectedWorkstation || reactFlowSelected;
  const visualState = resolveFactoryGraphVisualState({
    family: "workstation",
    focused: data.focused,
    lifecycle: data.active ? "PROCESSING" : undefined,
    muted: data.muted,
    runtimeStatus: data.runtimeStatus,
    selected,
    validation: data.validationError,
  });
  const className = classNames(
    factoryGraphNodeSurfaceClassName("workstation"),
    "min-w-0 w-full justify-start overflow-hidden border-2",
    factoryGraphNodeHoverClassName({ muted: data.muted, selected }, "primary"),
    "border-info-border",
    presentation.borderClassName,
    data.selectedWorkID !== null &&
      "border-info-border shadow-af-info-selected",
  );
  return (
    <FactoryGraphNodeShell
      className={className}
      handles={data.handles}
      interactionOverlay={data.interactionOverlay}
      nodeType="workstation"
      resizeControls={
        data.resizeControls
          ? { ...data.resizeControls, isVisible: true }
          : undefined
      }
      visualState={{
        focused: data.focused,
        lifecycle: data.active ? "PROCESSING" : undefined,
        muted: data.muted,
        runtimeStatus: data.runtimeStatus,
        selected,
        validation: data.validationError,
      }}
      zAxisIncompleteHints={data.zAxisIncompleteHints}
    >
      {data.summaryOnly ? (
        <Summary
          data={data}
          isExpanded={isExpanded}
          presentation={presentation}
          title={title}
          visualState={visualState}
        />
      ) : (
        <ActiveContent
          data={data}
          entries={entries}
          isExpanded={isExpanded}
          presentation={presentation}
          title={title}
          visualState={visualState}
        />
      )}
    </FactoryGraphNodeShell>
  );
}

function Summary({
  data,
  isExpanded,
  presentation,
  title,
  visualState,
}: {
  data: FactoryGraphWorkstationNodeData;
  isExpanded: boolean;
  presentation: WorkstationPresentation;
  title: string;
  visualState: ReturnType<typeof resolveFactoryGraphVisualState>;
}) {
  const header = (
    <Header
      presentation={presentation}
      showAuxiliaryDetails={isExpanded}
      title={title}
    />
  );

  return (
    <div
      className="grid min-w-0 gap-0.5"
      data-workstation-control-role={presentation.controlRole}
      data-workstation-density="compact"
      data-workstation-runtime-type={presentation.runtimeType}
      data-workstation-scheduling-behavior={presentation.schedulingBehavior}
    >
      {data.onSelectWorkstation ? (
        <GraphNodeButton
          aria-label={selectWorkstationLabel(title, data.locale)}
          aria-pressed={visualState.selection}
          className={WORKSTATION_HEADER_CLASS_NAME}
          data-selected-workstation={visualState.selection ? "true" : undefined}
          onClick={(event) => {
            event.stopPropagation();
            data.onSelectWorkstation?.(data.workstation.node_id);
          }}
          title={title}
        >
          {header}
        </GraphNodeButton>
      ) : (
        <div className={WORKSTATION_HEADER_CLASS_NAME} title={title}>
          {header}
        </div>
      )}
      <FactoryGraphWorkstationGuardedControlCard
        locale={data.locale}
        presentation={presentation}
      />
    </div>
  );
}

function ActiveContent({
  data,
  entries,
  isExpanded,
  presentation,
  title,
  visualState,
}: {
  data: FactoryGraphWorkstationNodeData;
  entries: Array<{
    execution: FactoryGraphActiveExecution;
    workItem: FactoryGraphWorkItemRef;
  }>;
  isExpanded: boolean;
  presentation: WorkstationPresentation;
  title: string;
  visualState: ReturnType<typeof resolveFactoryGraphVisualState>;
}) {
  const progressMode = factoryGraphWorkProgressMode(
    entries.length,
    WORKSTATION_WORK_ITEM_MODE_MAXIMUM,
  );
  const visible = progressMode === "items" ? entries : [];
  const header = (
    <Header
      presentation={presentation}
      showAuxiliaryDetails={isExpanded}
      title={title}
    />
  );
  return (
    <div
      className="grid h-full min-w-0 grid-rows-[auto_auto_1fr_auto]"
      data-active={data.active ? "true" : undefined}
      data-selected-work={data.selectedWorkID !== null ? "true" : undefined}
      data-selected-workstation={visualState.selection ? "true" : undefined}
      data-workstation-control-role={presentation.controlRole}
      data-workstation-density="compact"
      data-workstation-runtime-type={presentation.runtimeType}
      data-workstation-scheduling-behavior={presentation.schedulingBehavior}
    >
      {data.onSelectWorkstation ? (
        <GraphNodeButton
          aria-label={selectWorkstationLabel(title, data.locale)}
          aria-pressed={visualState.selection}
          className={WORKSTATION_HEADER_CLASS_NAME}
          onClick={(event) => {
            event.stopPropagation();
            data.onSelectWorkstation?.(data.workstation.node_id);
          }}
          title={title}
        >
          {header}
        </GraphNodeButton>
      ) : (
        <div className={WORKSTATION_HEADER_CLASS_NAME} title={title}>
          {header}
        </div>
      )}
      <FactoryGraphWorkstationGuardedControlCard
        locale={data.locale}
        presentation={presentation}
      />
      <ul className="mt-1 grid min-h-0 min-w-0 list-none content-start gap-0.5 overflow-hidden p-0">
        {visible.map(({ execution, workItem }) => (
          <WorkItem
            data={data}
            execution={execution}
            key={`${execution.dispatch_id}:${workItem.work_id}`}
            workItem={workItem}
          />
        ))}
      </ul>
      <Overflow
        total={entries.length}
        visible={visible.length}
        locale={data.locale}
      />
    </div>
  );
}

function WorkItem({
  data,
  execution,
  workItem,
}: {
  data: FactoryGraphWorkstationNodeData;
  execution: FactoryGraphActiveExecution;
  workItem: FactoryGraphWorkItemRef;
}) {
  const selected = data.selectedWorkID === workItem.work_id;
  const label = workItemLabel(workItem);
  const duration = graphDuration(execution.started_at, data.now, data.locale);
  const durationTitle = durationText(
    execution.started_at,
    data.now,
    data.locale,
  );
  const content = (
    <>
      <span
        className={classNames(
          workItemLabelClassName(label),
          "!block !whitespace-nowrap truncate",
        )}
        data-active-work-label
      >
        {label}
      </span>
      <span
        className="shrink-0 whitespace-nowrap text-right font-mono text-[0.72rem] text-on-surface-subtle"
        data-active-work-duration
      >
        {duration}
      </span>
    </>
  );
  const className = classNames(
    "grid min-w-0 w-full grid-cols-[minmax(0,1fr)_auto] items-center gap-1 overflow-hidden rounded-lg border border-outline bg-surface px-1.5 py-1 text-[0.74rem]",
    selected && "border-info-border bg-info-container shadow-af-info-chip",
  );
  return (
    <li>
      {data.onSelectWorkID ? (
        <GraphNodeButton
          aria-pressed={selected}
          className={className}
          data-selected={selected ? "true" : undefined}
          onClick={(event) => {
            event.stopPropagation();
            data.onSelectWorkID?.(workItem.work_id, {
              dispatchID: execution.dispatch_id,
              nodeID: data.workstation.node_id,
            });
          }}
          title={`${label} - ${durationTitle}`}
        >
          {content}
        </GraphNodeButton>
      ) : (
        <div
          className={className}
          data-selected={selected ? "true" : undefined}
          title={`${label} - ${durationTitle}`}
        >
          {content}
        </div>
      )}
    </li>
  );
}

function Header({
  compact = false,
  presentation,
  showAuxiliaryDetails = false,
  title,
}: {
  compact?: boolean;
  presentation: WorkstationPresentation;
  showAuxiliaryDetails?: boolean;
  title: string;
}) {
  return (
    <>
      <span
        className={
          compact
            ? factoryGraphNodeWrappedTextClassName(
                "block font-mono text-[0.74rem] font-bold leading-tight text-on-surface",
              )
            : workstationTitleClassName(title)
        }
        data-factory-entity-title
        data-workstation-title
        title={showAuxiliaryDetails ? undefined : presentation.label}
      >
        {title}
      </span>
      {showAuxiliaryDetails ? (
        <span
          className={factoryGraphNodeWrappedTextClassName(
            "text-[0.62rem] font-semibold leading-tight text-on-surface-subtle",
          )}
          data-workstation-runtime-label
          title={presentation.label}
        >
          {presentation.label}
        </span>
      ) : null}
      {showAuxiliaryDetails && presentation.schedulingLabel ? (
        <span
          className="min-w-0 max-w-full shrink truncate whitespace-nowrap rounded-sm border border-outline-variant bg-surface px-1.5 py-0.5 text-[0.62rem] font-semibold leading-none text-on-surface-subtle"
          data-workstation-scheduling-label
          title={presentation.schedulingLabel}
        >
          {presentation.schedulingLabel}
        </span>
      ) : null}
    </>
  );
}

export function FactoryGraphWorkstationGuardedControlCard({
  locale,
  presentation,
}: {
  locale?: string;
  presentation: WorkstationPresentation;
}) {
  const control = presentation.guardedControl;
  if (presentation.controlRole !== "LOOP_BREAKER" || !control) return null;

  const label = locale === "zh-CN" ? "断路器" : "Breaker";
  return (
    <span
      className="min-w-0 max-w-full shrink truncate whitespace-nowrap rounded-sm border border-af-warning-border bg-warning-container px-1.5 py-0.5 text-[0.62rem] font-semibold leading-none text-on-warning-container"
      data-workstation-control-role={presentation.controlRole}
      data-workstation-guard-card
      data-workstation-guard-type={control.guardType}
      title={label}
    >
      {label}
    </span>
  );
}

function Overflow({
  locale,
  total,
  visible,
}: {
  locale?: string;
  total: number;
  visible: number;
}) {
  if (total <= visible) return null;
  return (
    <FactoryGraphWorkProgressMarker
      ariaLabel={activeItemsLabel(total, locale)}
      className="mt-1 flex min-h-7 w-full rounded-lg px-3 py-1 text-base"
      count={total}
      data-workstation-work-progress="numeric"
      kind="numeric"
    />
  );
}
