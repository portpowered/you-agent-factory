import type { ReactNode } from "react";

import { useEffect, useMemo, useState } from "react";
import type {
  DashboardTrace,
  DashboardWorkItemRef,
} from "../../../api/dashboard/types";
import {
  DASHBOARD_WIDGET_CLASS,
  DETAIL_CARD_CLASS,
  DETAIL_CARD_WIDE_CLASS,
  EMPTY_STATE_CLASS,
  EMPTY_STATE_COMPACT_CLASS,
} from "../../../components/ui/widget-frame";
import { Button } from "../../../components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "../../../components/ui/collapsible";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_CODE_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_LABELS_CLASS,
} from "../../../components/ui/dashboard-typography";
import {
  formatDurationMillis,
  formatTraceOutcome,
  formatTypedWorkItemLabel,
} from "../../../components/ui/formatters";
import { Skeleton } from "../../../components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "../../../components/ui/table";
import { cn } from "../../../lib/cn";
import { AgentBentoCard } from "../../bento/components/agent-bento";
import { getTraceDrilldownMessages } from "../messages/trace-drilldown";
import { TraceRelationFlow } from "./trace-relation-flow";
import { TraceWorkstationPath } from "./trace-workstation-path";

const TRACE_EXPANDER_HEADER_CLASS =
  "flex items-center justify-between gap-3  py-sm rounded-lg ";
const TRACE_EXPANDER_TOGGLE_CLASS = "min-h-9 shrink-0 px-2.5 py-2";
const TRACE_LOADING_SKELETON_CLASS = "h-4 w-full max-w-48";
// tailwind-exception: intrinsic-sizing
const TRACE_GRID_TABLE_CLASS = "min-w-[640px]";
const TRACE_WORK_ITEM_BUTTON_CLASS = cn(
  "h-auto min-h-0 justify-start border-af-accent-border bg-af-accent-surface px-2.5 py-1.5 text-left text-af-accent",
  DASHBOARD_SUPPORTING_CODE_CLASS,
);

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
  state: TraceGridState;
  title?: string;
  widgetId?: string;
}

export function TraceGridBentoCard({
  className = "",
  headerAction,
  locale,
  onSelectWorkID,
  state,
  title,
}: TraceGridBentoCardProps) {
  const messages = getTraceDrilldownMessages(locale);
  const cardClassName = cn(
    DASHBOARD_WIDGET_CLASS,
    DETAIL_CARD_CLASS,
    DETAIL_CARD_WIDE_CLASS,
    "h-full min-h-0 overflow-hidden",
    className,
  );

  return (
    <AgentBentoCard
      className={cardClassName}
      headerAction={headerAction}
      title={title ?? messages.title}
    >
      {renderTraceState(state, locale, onSelectWorkID)}
    </AgentBentoCard>
  );
}

function renderTraceState(
  state: TraceGridState,
  locale?: string,
  onSelectWorkID?: (workID: string) => void,
) {
  const messages = getTraceDrilldownMessages(locale);

  switch (state.status) {
    case "idle":
      return (
        <div className={cn(EMPTY_STATE_CLASS, EMPTY_STATE_COMPACT_CLASS)}>
          <h3>{messages.idleTitle}</h3>
          <p>{messages.idleMessage}</p>
        </div>
      );
    case "loading":
      return (
        <div className={cn(EMPTY_STATE_CLASS, EMPTY_STATE_COMPACT_CLASS)}>
          <h3>{messages.loadingTitle}</h3>
          <p>{messages.loadingMessage(state.workID)}</p>
          <div aria-hidden="true" className="grid gap-2 pt-2">
            <Skeleton className={TRACE_LOADING_SKELETON_CLASS} />
            <Skeleton className="h-24 w-full" />
            <Skeleton className={TRACE_LOADING_SKELETON_CLASS} />
          </div>
        </div>
      );
    case "empty":
      return (
        <div className={cn(EMPTY_STATE_CLASS, EMPTY_STATE_COMPACT_CLASS)}>
          <h3>{messages.emptyTitle}</h3>
          <p>{messages.emptyMessage}</p>
        </div>
      );
    case "error":
      return (
        <div className={cn(EMPTY_STATE_CLASS, EMPTY_STATE_COMPACT_CLASS)}>
          <h3>{messages.errorTitle}</h3>
          <p>{state.message}</p>
        </div>
      );
    case "ready":
      return (
        <TraceGrid
          locale={locale}
          onSelectWorkID={onSelectWorkID}
          trace={state.trace}
        />
      );
  }
}

interface TraceGridProps {
  locale?: string;
  onSelectWorkID?: (workID: string) => void;
  trace: DashboardTrace;
}

function TraceGrid({ locale, onSelectWorkID, trace }: TraceGridProps) {
  const messages = getTraceDrilldownMessages(locale);
  const workItems = useMemo(() => resolveTraceWorkItems(trace), [trace]);
  const [workItemsExpanded, setWorkItemsExpanded] = useState(false);
  const workItemsID = `trace-work-items-${trace.trace_id || "selected"}`;

  useEffect(() => {
    setWorkItemsExpanded(false);
  }, []);

  return (
    <div className="grid min-w-0 w-full gap-3" style={{ overflowX: "hidden" }}>
      <dl
        className={cn(
          "m-0 grid gap-3 [&_dd]:m-0 [&_div:first-child]:border-t-0 [&_div:first-child]:pt-0 [&_div]:border-t [&_div]:border-af-border [&_div]:pt-3 [&_dt]:mb-1",
          DASHBOARD_SUPPORTING_LABELS_CLASS,
          DASHBOARD_BODY_TEXT_CLASS,
        )}
      >
        <div>
          <dt className={DASHBOARD_SUPPORTING_LABEL_CLASS}>
            {messages.traceIdLabel}
          </dt>
          <dd className="[overflow-wrap:anywhere]">
            {trace.trace_id || messages.unavailableValue}
          </dd>
        </div>
        <div>
          <dt className={DASHBOARD_SUPPORTING_LABEL_CLASS}>
            {messages.dispatchFlowLabel}
          </dt>
          <dd>
            <TraceWorkstationPath
              dispatches={trace.dispatches}
              locale={locale}
            />
          </dd>
        </div>
        <div>
          <dt className={DASHBOARD_SUPPORTING_LABEL_CLASS}>
            {messages.dispatchCountLabel}
          </dt>
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
                  <div className={TRACE_EXPANDER_HEADER_CLASS}>
                    <h3
                      className={DASHBOARD_SUPPORTING_LABEL_CLASS}
                      id={`${workItemsID}-heading`}
                    >
                      {messages.workItemsSummary(workItems.length)}
                    </h3>
                    <CollapsibleTrigger asChild>
                      <Button
                        aria-controls={workItemsID}
                        aria-expanded={workItemsExpanded}
                        className={cn(
                          TRACE_EXPANDER_TOGGLE_CLASS,
                          DASHBOARD_SUPPORTING_LABEL_CLASS,
                        )}
                        size="sm"
                        tone="outline"
                      >
                        {messages.workItemsExpandLabel(workItemsExpanded)}
                      </Button>
                    </CollapsibleTrigger>
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
          <dt className={DASHBOARD_SUPPORTING_LABEL_CLASS}>
            {messages.requestIdsLabel}
          </dt>
          <dd className="[overflow-wrap:anywhere]">
            {trace.request_ids?.join(", ") || messages.unavailableValue}
          </dd>
        </div>
        <div>
          <dt className={DASHBOARD_SUPPORTING_LABEL_CLASS}>
            {messages.batchRelationsLabel}
          </dt>
          <dd>
            {trace.relations && trace.relations.length > 0 ? (
              <TraceRelationFlow
                locale={locale}
                onSelectWorkID={onSelectWorkID}
                relations={trace.relations}
              />
            ) : (
              messages.noBatchRelations
            )}
          </dd>
        </div>
      </dl>

      {trace.dispatches.length > 0 ? (
        <div className="min-w-0 overflow-x-auto overscroll-x-contain">
          <Table
            className={cn(TRACE_GRID_TABLE_CLASS, DASHBOARD_BODY_TEXT_CLASS)}
          >
            <TableCaption
              className={cn("mb-2 text-left", DASHBOARD_SUPPORTING_LABEL_CLASS)}
            >
              {messages.tableCaption}
            </TableCaption>
            <TableHeader>
              <TableRow>
                <TableHead
                  className={DASHBOARD_SUPPORTING_LABEL_CLASS}
                  scope="col"
                >
                  {messages.dispatchColumnLabel}
                </TableHead>
                <TableHead
                  className={DASHBOARD_SUPPORTING_LABEL_CLASS}
                  scope="col"
                >
                  {messages.workstationColumnLabel}
                </TableHead>
                <TableHead
                  className={DASHBOARD_SUPPORTING_LABEL_CLASS}
                  scope="col"
                >
                  {messages.outcomeColumnLabel}
                </TableHead>
                <TableHead
                  className={DASHBOARD_SUPPORTING_LABEL_CLASS}
                  scope="col"
                >
                  {messages.inputItemsColumnLabel}
                </TableHead>
                <TableHead
                  className={DASHBOARD_SUPPORTING_LABEL_CLASS}
                  scope="col"
                >
                  {messages.outputItemsColumnLabel}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {trace.dispatches.map((dispatch) => (
                <TableRow key={dispatch.dispatch_id}>
                  <TableHead className="align-top" scope="row">
                    <span
                      className={cn(
                        "inline-flex rounded-full border border-af-info-border bg-af-info-surface px-2 py-0.5 text-af-info-text",
                        DASHBOARD_SUPPORTING_CODE_CLASS,
                      )}
                    >
                      {dispatch.dispatch_id}
                    </span>
                  </TableHead>
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
                        onSelectWorkID={onSelectWorkID}
                        workItems={dispatch.input_items}
                      />
                    ) : (
                      <span>{messages.noInputItems}</span>
                    )}
                  </TableCell>
                  <TableCell className="align-top">
                    {dispatch.output_items &&
                    dispatch.output_items.length > 0 ? (
                      <SelectableWorkList
                        onSelectWorkID={onSelectWorkID}
                        workItems={dispatch.output_items}
                      />
                    ) : (
                      <span>{messages.noOutputItems}</span>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      ) : (
        <div className={cn(EMPTY_STATE_CLASS, EMPTY_STATE_COMPACT_CLASS)}>
          <h3>{messages.noTraceHistoryTitle}</h3>
          <p>{messages.noTraceHistoryMessage}</p>
        </div>
      )}
    </div>
  );
}

function SelectableWorkList({
  onSelectWorkID,
  workItems,
}: {
  onSelectWorkID?: (workID: string) => void;
  workItems: DashboardWorkItemRef[];
}) {
  return (
    <ul className="m-0 grid gap-1.5 p-0">
      {workItems.map((workItem) => (
        <li className="list-none" key={workItem.work_id}>
          {onSelectWorkID ? (
            <Button
              className={TRACE_WORK_ITEM_BUTTON_CLASS}
              onClick={() => onSelectWorkID(workItem.work_id)}
              size="sm"
              title={workItem.work_id}
              tone="outline"
            >
              {formatTypedWorkItemLabel(workItem)}
            </Button>
          ) : (
            <code className={DASHBOARD_SUPPORTING_CODE_CLASS}>
              {formatTypedWorkItemLabel(workItem)}
            </code>
          )}
        </li>
      ))}
    </ul>
  );
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
