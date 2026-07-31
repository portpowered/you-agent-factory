import type { Node, NodeProps } from "@xyflow/react";
import { GraphNodeButton } from "@you-agent-factory/components/graphs";

import { GraphSemanticIcon } from "./semantic-icon.js";
import {
  FactoryGraphNodeShell,
  type FactoryGraphNodeHandle,
  type FactoryGraphZAxisIncompleteHints,
} from "./semantic-node-shell.js";
import {
  factoryGraphNodeHoverClassName,
  factoryGraphNodeSurfaceClassName,
} from "./semantic-node-style.js";
import { FactoryGraphWorkProgressMarker } from "./semantic-place-nodes.js";
import {
  factoryGraphActiveItemsLabel as activeItemsLabel,
  factoryGraphClassNames as classNames,
  factoryGraphDurationText as durationText,
  factoryGraphGraphDuration as graphDuration,
  factoryGraphSelectExhaustionLabel as selectExhaustionLabel,
  factoryGraphSelectWorkstationLabel as selectWorkstationLabel,
  factoryGraphWorkItemLabel as workItemLabel,
  factoryGraphWorkItemLabelClassName as workItemLabelClassName,
  factoryGraphWorkstationPresentation as workstationPresentation,
  factoryGraphWorkstationTitleClassName as workstationTitleClassName,
  type FactoryGraphWorkItemRef,
  type FactoryGraphWorkstationPresentation as WorkstationPresentation,
  type FactoryGraphWorkstationRef,
} from "./semantic-workstation-presentation.js";

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
  executions: FactoryGraphActiveExecution[];
  factoryGraphNodeId?: string;
  handles: FactoryGraphNodeHandle[];
  kind?: "workstation";
  locale?: string;
  muted: boolean;
  now: number;
  progressOutcomeRouteWorkstation?: unknown;
  selectedWorkID: string | null;
  selectedWorkstation: boolean;
  summaryOnly?: boolean;
  workstation: FactoryGraphWorkstationRef;
  zAxisIncompleteHints?: FactoryGraphZAxisIncompleteHints | null;
  onSelectWorkstation?: (nodeId: string) => void;
  onSelectWorkID?: (
    workID: string,
    hint?: { dispatchID?: string; nodeID?: string },
  ) => void;
}
export type FactoryGraphWorkstationNode = Node<
  FactoryGraphWorkstationNodeData,
  "workstation"
>;

const VISIBLE_WORK_ITEM_LIMIT = 3;

/** Original Factory workstation presentation, with host-owned selection callbacks. */
export function FactoryGraphWorkstationNodeView({
  data,
}: NodeProps<FactoryGraphWorkstationNode>) {
  const presentation = workstationPresentation(data.workstation, data.locale);
  const exhaustion = presentation.semanticKind === "exhaustion";
  const title =
    data.workstation.workstation_name ||
    data.workstation.transition_id ||
    data.workstation.node_id;
  const entries = data.executions.flatMap((execution) =>
    (execution.work_items ?? []).map((workItem) => ({ execution, workItem })),
  );
  const className = classNames(
    factoryGraphNodeSurfaceClassName("workstation"),
    "min-w-0 w-full justify-start overflow-hidden border-2",
    factoryGraphNodeHoverClassName(
      { muted: data.muted, selected: data.selectedWorkstation },
      "primary",
    ),
    exhaustion ? "border-dashed border-af-danger-border" : "border-info-border",
    !exhaustion && presentation.borderClassName,
    !exhaustion &&
      data.active &&
      !data.selectedWorkstation &&
      "border-af-success-border shadow-af-success-chip",
    !exhaustion &&
      data.activeFlow &&
      !data.selectedWorkstation &&
      "agent-flow-node--active ring-2 ring-af-success-border",
    data.selectedWorkstation && "border-primary shadow-af-accent-selected",
    !exhaustion &&
      data.selectedWorkID !== null &&
      "border-info-border shadow-af-info-selected",
    data.muted && "opacity-[0.45]",
  );
  return (
    <FactoryGraphNodeShell
      className={className}
      handles={data.handles}
      nodeType="workstation"
      zAxisIncompleteHints={data.zAxisIncompleteHints}
    >
      {exhaustion ? (
        <Exhaustion data={data} presentation={presentation} title={title} />
      ) : data.summaryOnly ? (
        <Summary data={data} presentation={presentation} title={title} />
      ) : (
        <ActiveContent
          data={data}
          entries={entries}
          presentation={presentation}
          title={title}
        />
      )}
    </FactoryGraphNodeShell>
  );
}

function Summary({
  data,
  presentation,
  title,
}: {
  data: FactoryGraphWorkstationNodeData;
  presentation: WorkstationPresentation;
  title: string;
}) {
  return (
    <GraphNodeButton
      aria-label={
        data.onSelectWorkstation
          ? selectWorkstationLabel(title, data.locale)
          : undefined
      }
      aria-pressed={
        data.onSelectWorkstation ? data.selectedWorkstation : undefined
      }
      className="flex min-w-0 w-full items-center justify-between gap-2 overflow-hidden"
      data-selected-workstation={data.selectedWorkstation ? "true" : undefined}
      data-workstation-kind={presentation.semanticKind}
      disabled={data.onSelectWorkstation === undefined}
      onClick={
        data.onSelectWorkstation
          ? (event) => {
              event.stopPropagation();
              data.onSelectWorkstation?.(data.workstation.node_id);
            }
          : undefined
      }
      title={title}
    >
      <Header presentation={presentation} title={title} />
    </GraphNodeButton>
  );
}

function Exhaustion({
  data,
  presentation,
  title,
}: {
  data: FactoryGraphWorkstationNodeData;
  presentation: WorkstationPresentation;
  title: string;
}) {
  const header = <Header presentation={presentation} title={title} compact />;
  if (!data.onSelectWorkstation)
    return (
      <div
        className="flex h-full min-w-0 w-full items-center gap-2 overflow-hidden"
        data-selected-workstation={
          data.selectedWorkstation ? "true" : undefined
        }
        data-workstation-kind={presentation.semanticKind}
        title={title}
      >
        {header}
      </div>
    );
  return (
    <GraphNodeButton
      aria-label={selectExhaustionLabel(title, data.locale)}
      aria-pressed={data.selectedWorkstation}
      className="flex h-full min-w-0 w-full items-center gap-2 overflow-hidden"
      data-selected-workstation={data.selectedWorkstation ? "true" : undefined}
      data-workstation-kind={presentation.semanticKind}
      onClick={(event) => {
        event.stopPropagation();
        data.onSelectWorkstation?.(data.workstation.node_id);
      }}
      title={title}
    >
      {header}
    </GraphNodeButton>
  );
}

function ActiveContent({
  data,
  entries,
  presentation,
  title,
}: {
  data: FactoryGraphWorkstationNodeData;
  entries: Array<{
    execution: FactoryGraphActiveExecution;
    workItem: FactoryGraphWorkItemRef;
  }>;
  presentation: WorkstationPresentation;
  title: string;
}) {
  const visible = entries.slice(0, VISIBLE_WORK_ITEM_LIMIT);
  const header = <Header presentation={presentation} title={title} />;
  return (
    <div
      className="grid h-full min-w-0 grid-rows-[auto_1fr_auto]"
      data-active={data.active ? "true" : undefined}
      data-selected-work={data.selectedWorkID !== null ? "true" : undefined}
      data-selected-workstation={data.selectedWorkstation ? "true" : undefined}
      data-workstation-kind={presentation.semanticKind}
    >
      {data.onSelectWorkstation ? (
        <GraphNodeButton
          aria-label={selectWorkstationLabel(title, data.locale)}
          aria-pressed={data.selectedWorkstation}
          className="flex min-w-0 w-full items-center justify-between gap-2 overflow-hidden"
          onClick={(event) => {
            event.stopPropagation();
            data.onSelectWorkstation?.(data.workstation.node_id);
          }}
          title={title}
        >
          {header}
        </GraphNodeButton>
      ) : (
        <div
          className="flex min-w-0 w-full items-center justify-between gap-2 overflow-hidden"
          title={title}
        >
          {header}
        </div>
      )}
      <ul className="mt-2 grid min-w-0 list-none content-start gap-1 p-0">
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
      <span className={workItemLabelClassName(label)} data-active-work-label>
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
  title,
}: {
  compact?: boolean;
  presentation: WorkstationPresentation;
  title: string;
}) {
  return (
    <>
      <span
        className={
          compact
            ? "flex min-h-4 items-center"
            : "flex min-h-5 shrink-0 items-center"
        }
        data-workstation-semantic-icon
        title={presentation.label}
      >
        <GraphSemanticIcon
          className={classNames("h-4 w-4", presentation.className)}
          kind={presentation.iconKind}
          label={presentation.label}
        />
      </span>
      <span
        className={
          compact
            ? "block min-w-0 truncate whitespace-nowrap font-mono text-[0.74rem] font-bold leading-tight text-on-surface"
            : workstationTitleClassName(title)
        }
        data-workstation-title
      >
        {title}
      </span>
    </>
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
  const remaining = Math.max(0, total - visible);
  if (!remaining) return null;
  if (remaining > 10)
    return (
      <FactoryGraphWorkProgressMarker
        ariaLabel={activeItemsLabel(total, locale)}
        className="mt-2 flex min-h-7 w-full rounded-lg px-3 py-1 text-[0.9rem]"
        count={total}
        data-workstation-work-progress="numeric"
        kind="numeric"
      />
    );
  return (
    <FactoryGraphWorkProgressMarker
      ariaLabel={activeItemsLabel(total, locale)}
      className="mt-2 flex min-h-7 gap-1 rounded-lg px-2"
      data-workstation-work-progress="dots"
      dotClassName="h-1.5 w-1.5"
      dotCount={remaining}
      dotDataAttribute="data-workstation-work-progress-dot"
      kind="dots"
      suffix={
        <span className="ml-1 font-mono text-[0.68rem] font-bold text-success">
          +{remaining}
        </span>
      }
    />
  );
}
