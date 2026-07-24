import {
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@you-agent-factory/components/data-display";
import { Button } from "@you-agent-factory/components/primitives";
import {
  WidgetEmptyState,
  WidgetEmptyStateText,
  WidgetEmptyStateTitle,
} from "@you-agent-factory/components/recipes";
import type { HTMLAttributes, ReactNode } from "react";
import { useEffect, useMemo, useState } from "react";
import type {
  DashboardTrace,
  DashboardWorkItemRef,
} from "../../../api/dashboard/types";
import {
  Code,
  DescriptionList,
  ExpandablePanelTrigger,
  Label,
} from "../../../components/ui";
import {
  Collapsible,
  CollapsibleContent,
} from "../../../components/ui/collapsible";
import {
  formatDurationMillis,
  formatTraceOutcome,
  formatTypedWorkItemLabel,
} from "../../../components/ui/formatters";
import { Skeleton } from "../../../components/ui/skeleton";
import { DashboardWidgetFrame } from "../../bento/public";
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
      {renderTraceState(state, locale, onSelectWorkID)}
    </DashboardWidgetFrame>
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
    <div className="grid min-w-0 w-full gap-3">
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
                relations={trace.relations}
              />
            ) : (
              messages.noBatchRelations
            )}
          </dd>
        </div>
      </DescriptionList>

      {trace.dispatches.length > 0 ? (
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
              <TableHead scope="col">
                {messages.workstationColumnLabel}
              </TableHead>
              <TableHead scope="col">{messages.outcomeColumnLabel}</TableHead>
              <TableHead scope="col">
                {messages.inputItemsColumnLabel}
              </TableHead>
              <TableHead scope="col">
                {messages.outputItemsColumnLabel}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {trace.dispatches.map((dispatch) => (
              <TableRow key={dispatch.dispatch_id}>
                <TableCell className="align-top" scope="row">
                  {dispatch.dispatch_id}
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
                      onSelectWorkID={onSelectWorkID}
                      workItems={dispatch.input_items}
                    />
                  ) : (
                    <span>{messages.noInputItems}</span>
                  )}
                </TableCell>
                <TableCell className="align-top">
                  {dispatch.output_items && dispatch.output_items.length > 0 ? (
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
      ) : (
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
              className="h-auto min-h-0 justify-start px-2.5 py-1.5 text-left"
              onClick={() => onSelectWorkID(workItem.work_id)}
              size="sm"
              title={workItem.work_id}
              tone="outline"
            >
              {formatTypedWorkItemLabel(workItem)}
            </Button>
          ) : (
            <Code size="supporting">{formatTypedWorkItemLabel(workItem)}</Code>
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
