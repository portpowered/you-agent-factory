import {
  DescriptionList,
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@you-agent-factory/components/data-display";
import { Button, Code, Label } from "@you-agent-factory/components/primitives";
import {
  WidgetEmptyState,
  WidgetEmptyStateText,
  WidgetEmptyStateTitle,
} from "@you-agent-factory/components/recipes";
import type { HTMLAttributes, ReactNode } from "react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type {
  DashboardTrace,
  DashboardTraceDispatch,
  DashboardWorkItemRef,
} from "../../../api/dashboard/types";
import {
  Collapsible,
  CollapsibleContent,
} from "../../../components/ui/collapsible";
import { ExpandablePanelTrigger } from "../../../components/ui/expandable-panel-trigger";
import {
  formatDurationMillis,
  formatTraceOutcome,
  formatTypedWorkItemLabel,
} from "../../../components/ui/formatters";
import { Skeleton } from "../../../components/ui/skeleton";
import { DashboardWidgetFrame } from "../../bento/components/dashboard-widget-frame/dashboard-widget-frame";
import {
  type TraceSelectionIdentity,
  traceSelectionForDispatch,
  traceSelectionIdentitiesByWorkID,
  traceSelectionIdentitiesForDispatch,
  traceSelectionKey,
  traceSelectionMatches,
} from "../lib/trace-selection";
import { getTraceDrilldownMessages } from "../messages/trace-drilldown";
import { TraceRelationFlow } from "./trace-relation-flow";
import { TraceWorkstationPath } from "./trace-workstation-path";

export type TraceGridState =
  | { status: "idle"; message: string }
  | { status: "loading"; workID: string }
  | { status: "empty"; workID: string }
  | { status: "error"; message: string }
  | { status: "ready"; trace: DashboardTrace };

export interface TraceGridBentoCardProps {
  className?: string;
  headerAction?: ReactNode;
  locale?: string;
  onSelectWorkID?: (workID: string) => void;
  onSelectTraceSelection?: (selection: TraceSelectionIdentity) => void;
  selectedTraceSelection?: TraceSelectionIdentity | null;
  state: TraceGridState;
  title?: string;
  widgetId?: string;
}

export function TraceGridBentoCard({
  className = "",
  headerAction,
  locale,
  onSelectWorkID,
  onSelectTraceSelection,
  selectedTraceSelection,
  state,
  title,
  widgetId = "trace-drilldown",
}: TraceGridBentoCardProps) {
  const messages = getTraceDrilldownMessages(locale);

  return (
    <DashboardWidgetFrame
      bodyScroll
      bodyProps={
        { "data-trace-card-body": "" } as HTMLAttributes<HTMLDivElement>
      }
      className={className}
      headerAction={headerAction}
      title={title ?? messages.title}
      wide
      widgetId={widgetId}
    >
      {renderTraceState(
        state,
        locale,
        onSelectWorkID,
        onSelectTraceSelection,
        selectedTraceSelection,
      )}
    </DashboardWidgetFrame>
  );
}

function renderTraceState(
  state: TraceGridState,
  locale?: string,
  onSelectWorkID?: (workID: string) => void,
  onSelectTraceSelection?: (selection: TraceSelectionIdentity) => void,
  selectedTraceSelection?: TraceSelectionIdentity | null,
) {
  const messages = getTraceDrilldownMessages(locale);

  switch (state.status) {
    case "idle":
      return (
        <div>
          <WidgetEmptyStateTitle as="h2">
            {messages.idleTitle}
          </WidgetEmptyStateTitle>
        </div>
      );
    case "loading":
      return (
        <div>
          <WidgetEmptyStateTitle>{messages.loadingTitle}</WidgetEmptyStateTitle>
          <WidgetEmptyStateText>
            {messages.loadingMessage(state.workID)}
          </WidgetEmptyStateText>
          <div aria-hidden="true" className="grid gap-2 pt-2">
            <Skeleton className="h-4 w-full max-w-48" />
            <Skeleton className="h-24 w-full" />
            <Skeleton className="h-4 w-full max-w-48" />
          </div>
        </div>
      );
    case "empty":
      return (
        <div>
          <WidgetEmptyStateTitle>{messages.emptyTitle}</WidgetEmptyStateTitle>
          <WidgetEmptyStateText>{messages.emptyMessage}</WidgetEmptyStateText>
        </div>
      );
    case "error":
      return (
        <div>
          <WidgetEmptyStateTitle>{messages.errorTitle}</WidgetEmptyStateTitle>
          <WidgetEmptyStateText>{state.message}</WidgetEmptyStateText>
        </div>
      );
    case "ready":
      return (
        <TraceGrid
          locale={locale}
          onSelectWorkID={onSelectWorkID}
          onSelectTraceSelection={onSelectTraceSelection}
          selectedTraceSelection={selectedTraceSelection}
          trace={state.trace}
        />
      );
  }
}

interface TraceGridProps {
  locale?: string;
  onSelectWorkID?: (workID: string) => void;
  onSelectTraceSelection?: (selection: TraceSelectionIdentity) => void;
  selectedTraceSelection?: TraceSelectionIdentity | null;
  trace: DashboardTrace;
}

type TraceSelectionSource = "graph" | "table" | "text";

interface PendingTraceFocus {
  selection: TraceSelectionIdentity;
  source: TraceSelectionSource;
}

// biome-ignore lint/complexity/noExcessiveLinesPerFunction: trace card keeps the canonical metadata, relation, table, and graph surfaces in one selection-aware render path.
function TraceGrid({
  locale,
  onSelectWorkID,
  onSelectTraceSelection,
  selectedTraceSelection: controlledSelection,
  trace,
}: TraceGridProps) {
  const messages = getTraceDrilldownMessages(locale);
  const workItems = useMemo(() => resolveTraceWorkItems(trace), [trace]);
  const workItemsByWorkId = useMemo(
    () => new Map(workItems.map((workItem) => [workItem.work_id, workItem])),
    [workItems],
  );
  const selectionIdentitiesByWorkID = useMemo(
    () => traceSelectionIdentitiesByWorkID(trace.dispatches),
    [trace.dispatches],
  );
  const [workItemsExpanded, setWorkItemsExpanded] = useState(false);
  const [localSelection, setLocalSelection] =
    useState<TraceSelectionIdentity | null>(null);
  const [pendingFocus, setPendingFocus] = useState<PendingTraceFocus | null>(
    null,
  );
  const gridRef = useRef<HTMLDivElement>(null);
  const selectedTraceSelection =
    controlledSelection === undefined ? localSelection : controlledSelection;
  const workItemsID = `trace-work-items-${trace.trace_id || "selected"}`;

  useEffect(() => {
    setWorkItemsExpanded(false);
  }, []);

  const selectTraceSelection = useCallback(
    (selection: TraceSelectionIdentity, source: TraceSelectionSource) => {
      setLocalSelection(selection);
      setPendingFocus({ selection, source });
      onSelectTraceSelection?.(selection);
      if (selection.work_id) {
        onSelectWorkID?.(selection.work_id);
      }
    },
    [onSelectTraceSelection, onSelectWorkID],
  );

  useEffect(() => {
    const grid = gridRef.current;
    if (!pendingFocus || !grid) {
      return;
    }

    const targetKey = traceSelectionKey(pendingFocus.selection);
    const targetSurfaces =
      pendingFocus.source === "table"
        ? ["graph", "text"]
        : pendingFocus.source === "graph"
          ? ["table", "text"]
          : ["graph", "table"];
    const target = targetSurfaces
      .flatMap((surface) => [
        ...grid.querySelectorAll<HTMLElement>(
          `[data-trace-selection-surface="${surface}"]`,
        ),
      ])
      .find((element) =>
        traceSelectionKeysFromElement(element).includes(targetKey),
      );
    const focusTarget = target?.matches("button")
      ? target
      : target?.querySelector<HTMLElement>("button");

    if (focusTarget) {
      focusTarget.focus();
      focusTarget.scrollIntoView?.({ block: "nearest", inline: "nearest" });
    }

    setPendingFocus(null);
  }, [pendingFocus]);

  return (
    <div className="grid min-w-0 w-full gap-3" ref={gridRef}>
      <DescriptionList className="gap-3 [&_div:first-child]:border-t-0 [&_div:first-child]:pt-0 [&_div]:border-t [&_div]:border-outline [&_div]:pt-3 [&_dt]:mb-1">
        <div>
          <Label as="dt">{messages.traceIdLabel}</Label>
          <dd className="[overflow-wrap:anywhere]">
            {trace.trace_id || messages.unavailableValue}
          </dd>
        </div>
        <div>
          <Label as="dt">{messages.dispatchFlowLabel}</Label>
          <dd>
            <TraceWorkstationPath
              dispatches={trace.dispatches}
              locale={locale}
              onSelectTraceSelection={(selection) =>
                selectTraceSelection(selection, "graph")
              }
              selectedTraceSelection={selectedTraceSelection}
            />
          </dd>
        </div>
        <div>
          <Label as="dt">{messages.dispatchCountLabel}</Label>
          <dd>{trace.dispatches.length}</dd>
        </div>
        <div>
          <dd>
            {workItems.length > 0 ? (
              <Collapsible
                className="grid gap-2.5"
                onOpenChange={setWorkItemsExpanded}
                open={workItemsExpanded}
              >
                <section
                  aria-labelledby={`${workItemsID}-heading`}
                  className="grid gap-2.5"
                >
                  <div className="flex items-center justify-between gap-3  py-sm rounded-lg ">
                    <Label
                      as="h3"
                      className="m-0"
                      id={`${workItemsID}-heading`}
                    >
                      {messages.workItemsSummary(workItems.length)}
                    </Label>
                    <ExpandablePanelTrigger
                      controlsID={workItemsID}
                      expanded={workItemsExpanded}
                      onClick={() =>
                        setWorkItemsExpanded((current) => !current)
                      }
                      variant="outline"
                      className="min-h-9 shrink-0 px-2.5 py-2"
                    >
                      {messages.workItemsExpandLabel(workItemsExpanded)}
                    </ExpandablePanelTrigger>
                  </div>
                  <CollapsibleContent id={workItemsID}>
                    <SelectableWorkList
                      onSelectWorkID={onSelectWorkID}
                      workItems={workItems}
                    />
                  </CollapsibleContent>
                </section>
              </Collapsible>
            ) : (
              messages.unavailableValue
            )}
          </dd>
        </div>
        <div>
          <Label as="dt">{messages.requestIdsLabel}</Label>
          <dd className="[overflow-wrap:anywhere]">
            {trace.request_ids?.join(", ") || messages.unavailableValue}
          </dd>
        </div>
        <div>
          <Label as="dt">{messages.batchRelationsLabel}</Label>
          <dd>
            {trace.relations && trace.relations.length > 0 ? (
              <TraceRelationFlow
                locale={locale}
                onSelectWorkID={onSelectWorkID}
                onSelectTraceSelection={(selection) =>
                  selectTraceSelection(selection, "text")
                }
                relations={trace.relations}
                selectedTraceSelection={selectedTraceSelection}
                selectedWorkID={selectedTraceSelection?.work_id}
                selectionIdentitiesByWorkID={selectionIdentitiesByWorkID}
                workItemsByWorkId={workItemsByWorkId}
              />
            ) : (
              messages.noBatchRelations
            )}
          </dd>
        </div>
      </DescriptionList>

      {trace.dispatches.length > 0 ? (
        <TraceDispatchTable
          dispatches={trace.dispatches}
          locale={locale}
          onSelectTraceSelection={(selection) =>
            selectTraceSelection(selection, "table")
          }
          selectedTraceSelection={selectedTraceSelection}
        />
      ) : trace.relations && trace.relations.length > 0 ? null : (
        <WidgetEmptyState compact>
          <WidgetEmptyStateTitle>
            {messages.noTraceHistoryTitle}
          </WidgetEmptyStateTitle>
          <WidgetEmptyStateText>
            {messages.noTraceHistoryMessage}
          </WidgetEmptyStateText>
        </WidgetEmptyState>
      )}
    </div>
  );
}

function TraceDispatchTable({
  dispatches,
  locale,
  onSelectTraceSelection,
  selectedTraceSelection,
}: {
  dispatches: DashboardTraceDispatch[];
  locale?: string;
  onSelectTraceSelection: (selection: TraceSelectionIdentity) => void;
  selectedTraceSelection: TraceSelectionIdentity | null;
}) {
  const messages = getTraceDrilldownMessages(locale);

  return (
    <Table
      className="min-w-2xl"
      containerClassName="min-w-0 overscroll-x-contain"
      containerProps={
        {
          "data-trace-dispatch-table": "",
        } as HTMLAttributes<HTMLDivElement>
      }
    >
      <TableCaption className="mb-2 text-left">
        {messages.tableCaption}
      </TableCaption>
      <TableHeader>
        <TableRow>
          <TableHead scope="col">{messages.dispatchColumnLabel}</TableHead>
          <TableHead scope="col">{messages.workstationColumnLabel}</TableHead>
          <TableHead scope="col">{messages.outcomeColumnLabel}</TableHead>
          <TableHead scope="col">{messages.inputItemsColumnLabel}</TableHead>
          <TableHead scope="col">{messages.outputItemsColumnLabel}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {dispatches.map((dispatch) => {
          const dispatchSelections =
            traceSelectionIdentitiesForDispatch(dispatch);
          const isSelected = dispatchSelections.some((selection) =>
            traceSelectionMatches(selection, selectedTraceSelection),
          );
          const primarySelection = traceSelectionForDispatch(dispatch);
          const primarySelectionKey = traceSelectionKey(primarySelection);
          const selectLabel = messages.selectDispatchLabel(
            dispatch.dispatch_id,
            primarySelection.work_id,
            primarySelection.attempt,
          );

          return (
            <TableRow
              aria-selected={isSelected}
              data-trace-dispatch-row
              data-trace-selection-key={primarySelectionKey}
              key={primarySelectionKey}
            >
              <TableCell className="align-top" scope="row">
                <Button
                  aria-label={selectLabel}
                  aria-pressed={isSelected}
                  className="h-auto min-h-0 justify-start px-2.5 py-1.5 text-left"
                  data-trace-selection-key={primarySelectionKey}
                  data-trace-selection-surface="table"
                  onClick={() => onSelectTraceSelection(primarySelection)}
                  size="sm"
                  title={selectLabel}
                  tone="outline"
                >
                  {dispatch.dispatch_id}
                </Button>
              </TableCell>
              <TableCell className="align-top">
                {dispatch.workstation_name || dispatch.transition_id}
              </TableCell>
              <TableCell className="align-top">
                {formatTraceOutcome(dispatch.outcome)} ·{" "}
                {formatDurationMillis(dispatch.duration_millis)}
              </TableCell>
              <TableCell className="align-top">
                {dispatch.input_items && dispatch.input_items.length > 0 ? (
                  <SelectableWorkList
                    dispatch={dispatch}
                    onSelectTraceSelection={onSelectTraceSelection}
                    selectedTraceSelection={selectedTraceSelection}
                    workItems={dispatch.input_items}
                  />
                ) : (
                  <span>{messages.noInputItems}</span>
                )}
              </TableCell>
              <TableCell className="align-top">
                {dispatch.output_items && dispatch.output_items.length > 0 ? (
                  <SelectableWorkList
                    dispatch={dispatch}
                    onSelectTraceSelection={onSelectTraceSelection}
                    selectedTraceSelection={selectedTraceSelection}
                    workItems={dispatch.output_items}
                  />
                ) : (
                  <span>{messages.noOutputItems}</span>
                )}
              </TableCell>
            </TableRow>
          );
        })}
      </TableBody>
    </Table>
  );
}

function SelectableWorkList({
  dispatch,
  onSelectWorkID,
  onSelectTraceSelection,
  selectedTraceSelection,
  workItems,
}: {
  dispatch?: DashboardTraceDispatch;
  onSelectWorkID?: (workID: string) => void;
  onSelectTraceSelection?: (selection: TraceSelectionIdentity) => void;
  selectedTraceSelection?: TraceSelectionIdentity | null;
  workItems: DashboardWorkItemRef[];
}) {
  return (
    <ul className="m-0 grid gap-1.5 p-0">
      {workItems.map((workItem) => {
        const selection = dispatch
          ? traceSelectionForDispatch(dispatch, workItem.work_id)
          : undefined;
        const isSelected = traceSelectionMatches(
          selection,
          selectedTraceSelection,
        );

        return (
          <li
            className="list-none"
            key={selection ? traceSelectionKey(selection) : workItem.work_id}
          >
            {onSelectWorkID || onSelectTraceSelection ? (
              <Button
                aria-pressed={selection ? isSelected : undefined}
                className="h-auto min-h-0 justify-start px-2.5 py-1.5 text-left"
                data-trace-selection-key={
                  selection ? traceSelectionKey(selection) : undefined
                }
                data-trace-selection-surface={selection ? "table" : undefined}
                onClick={() => {
                  if (selection && onSelectTraceSelection) {
                    onSelectTraceSelection(selection);
                    return;
                  }
                  onSelectWorkID?.(workItem.work_id);
                }}
                size="sm"
                title={workItem.work_id}
                tone="outline"
              >
                {formatTypedWorkItemLabel(workItem)}
              </Button>
            ) : (
              <Code size="supporting">
                {formatTypedWorkItemLabel(workItem)}
              </Code>
            )}
          </li>
        );
      })}
    </ul>
  );
}

function traceSelectionKeysFromElement(element: HTMLElement): string[] {
  const serializedKeys = element.getAttribute("data-trace-selection-keys");
  if (serializedKeys) {
    try {
      const keys = JSON.parse(serializedKeys) as unknown;
      if (Array.isArray(keys)) {
        return keys.filter((key): key is string => typeof key === "string");
      }
    } catch {
      return [];
    }
  }

  const key = element.getAttribute("data-trace-selection-key");
  return key ? [key] : [];
}

function resolveTraceWorkItems(trace: DashboardTrace): DashboardWorkItemRef[] {
  if (trace.work_items && trace.work_items.length > 0) {
    return trace.work_items;
  }

  const itemsByID = new Map<string, DashboardWorkItemRef>();

  for (const dispatch of trace.dispatches) {
    for (const workItem of dispatch.input_items ?? []) {
      itemsByID.set(workItem.work_id, workItem);
    }
    for (const workItem of dispatch.output_items ?? []) {
      itemsByID.set(workItem.work_id, workItem);
    }
  }

  return [...itemsByID.values()].sort((left, right) =>
    left.work_id.localeCompare(right.work_id),
  );
}
