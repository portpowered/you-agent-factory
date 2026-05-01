import { useEffect, useMemo, useState } from "react";

import { AgentBentoCard } from "../../components/dashboard/bento";
import { cx } from "../../components/dashboard/classnames";
import {
  formatDurationMillis,
  formatTraceOutcome,
  formatTypedWorkItemLabel,
} from "../../components/dashboard/formatters";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SUPPORTING_CODE_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
  DASHBOARD_SUPPORTING_LABELS_CLASS,
} from "../../components/dashboard/typography";
import {
  DASHBOARD_WIDGET_CLASS,
  DETAIL_CARD_CLASS,
  DETAIL_CARD_WIDE_CLASS,
  DETAIL_COPY_CLASS,
  EMPTY_STATE_CLASS,
  EMPTY_STATE_COMPACT_CLASS,
} from "../../components/dashboard/widget-board";
import { Button } from "../../components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "../../components/ui/collapsible";
import type {
  DashboardTrace,
  DashboardWorkItemRef,
} from "../../api/dashboard/types";
import { TraceRelationFlow } from "./trace-relation-flow";
import { TraceWorkstationPath } from "./trace-workstation-path";

const TRACE_EXPANDER_HEADER_CLASS =
  "flex items-center justify-between gap-3 rounded-lg border border-af-overlay/8 bg-af-overlay/4 px-3 py-2";
const TRACE_EXPANDER_TOGGLE_CLASS = "min-h-9 shrink-0 px-[0.65rem] py-[0.45rem]";
const TRACE_WORK_ITEM_BUTTON_CLASS = cx(
  "h-auto min-h-0 justify-start border-af-accent/35 bg-af-accent/10 px-[0.65rem] py-[0.35rem] text-left text-af-accent",
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
  onSelectWorkID?: (workID: string) => void;
  state: TraceGridState;
  title?: string;
  widgetId?: string;
}

export function TraceGridBentoCard({
  className = "",
  onSelectWorkID,
  state,
  title = "Trace drill-down",
}: TraceGridBentoCardProps) {
  const cardClassName = cx(
    DASHBOARD_WIDGET_CLASS,
    DETAIL_CARD_CLASS,
    DETAIL_CARD_WIDE_CLASS,
    "h-full min-h-0 overflow-hidden",
    className,
  );

  return (
    <AgentBentoCard className={cardClassName} title={title}>
      <p className={DETAIL_COPY_CLASS}>
        Resolves from selected-tick factory event history.
      </p>
      {renderTraceState(state, onSelectWorkID)}
    </AgentBentoCard>
  );
}

function renderTraceState(
  state: TraceGridState,
  onSelectWorkID?: (workID: string) => void,
) {
  switch (state.status) {
    case "idle":
      return (
        <div className={cx(EMPTY_STATE_CLASS, EMPTY_STATE_COMPACT_CLASS)}>
          <h3>No trace selected</h3>
          <p>{state.message}</p>
        </div>
      );
    case "loading":
      return (
        <div className={cx(EMPTY_STATE_CLASS, EMPTY_STATE_COMPACT_CLASS)}>
          <h3>Loading trace</h3>
          <p>Reconstructing dispatch history for {state.workID}.</p>
        </div>
      );
    case "empty":
      return (
        <div className={cx(EMPTY_STATE_CLASS, EMPTY_STATE_COMPACT_CLASS)}>
          <h3>Trace history unavailable</h3>
          <p>No retained dispatch history is currently available for this work item.</p>
        </div>
      );
    case "error":
      return (
        <div className={cx(EMPTY_STATE_CLASS, EMPTY_STATE_COMPACT_CLASS)}>
          <h3>Trace lookup failed</h3>
          <p>{state.message}</p>
        </div>
      );
    case "ready":
      return <TraceGrid onSelectWorkID={onSelectWorkID} trace={state.trace} />;
  }
}

interface TraceGridProps {
  onSelectWorkID?: (workID: string) => void;
  trace: DashboardTrace;
}

function TraceGrid({ onSelectWorkID, trace }: TraceGridProps) {
  const workItems = useMemo(() => resolveTraceWorkItems(trace), [trace]);
  const [workItemsExpanded, setWorkItemsExpanded] = useState(false);
  const workItemsID = `trace-work-items-${trace.trace_id || "selected"}`;

  useEffect(() => {
    setWorkItemsExpanded(false);
  }, [trace.trace_id]);

  return (
    <div className="grid min-w-0 w-full gap-[0.8rem]">
      <dl
        className={cx(
          "m-0 grid gap-[0.8rem] [&_dd]:m-0 [&_div:first-child]:border-t-0 [&_div:first-child]:pt-0 [&_div]:border-t [&_div]:border-af-overlay/6 [&_div]:pt-3 [&_dt]:mb-1",
          DASHBOARD_SUPPORTING_LABELS_CLASS,
          DASHBOARD_BODY_TEXT_CLASS,
        )}
      >
        <div>
          <dt className={DASHBOARD_SUPPORTING_LABEL_CLASS}>Trace ID</dt>
          <dd>{trace.trace_id || "Unavailable"}</dd>
        </div>
        <div>
          <dt className={DASHBOARD_SUPPORTING_LABEL_CLASS}>Dispatch flow</dt>
          <dd>
            <TraceWorkstationPath dispatches={trace.dispatches} />
          </dd>
        </div>
        <div>
          <dt className={DASHBOARD_SUPPORTING_LABEL_CLASS}>Dispatch count</dt>
          <dd>{trace.dispatches.length}</dd>
        </div>
        <div>
          <dt className={DASHBOARD_SUPPORTING_LABEL_CLASS}>Work items</dt>
          <dd>
            {workItems.length > 0 ? (
              <Collapsible
                className="grid gap-[0.65rem]"
                onOpenChange={setWorkItemsExpanded}
                open={workItemsExpanded}
              >
                <section aria-labelledby={`${workItemsID}-heading`} className="grid gap-[0.65rem]">
                  <div className={TRACE_EXPANDER_HEADER_CLASS}>
                    <h3
                      className={DASHBOARD_SUPPORTING_LABEL_CLASS}
                      id={`${workItemsID}-heading`}
                    >
                      {workItems.length} work item{workItems.length === 1 ? "" : "s"}
                    </h3>
                    <CollapsibleTrigger asChild>
                      <Button
                        aria-controls={workItemsID}
                        aria-expanded={workItemsExpanded}
                        className={cx(
                          TRACE_EXPANDER_TOGGLE_CLASS,
                          DASHBOARD_SUPPORTING_LABEL_CLASS,
                        )}
                        size="sm"
                        tone="secondary"
                      >
                        {workItemsExpanded ? "Collapse" : "Expand"}
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
              "Unavailable"
            )}
          </dd>
        </div>
        <div>
          <dt className={DASHBOARD_SUPPORTING_LABEL_CLASS}>Request IDs</dt>
          <dd>{trace.request_ids?.join(", ") || "Unavailable"}</dd>
        </div>
        <div>
          <dt className={DASHBOARD_SUPPORTING_LABEL_CLASS}>Batch relations</dt>
          <dd>
            {trace.relations && trace.relations.length > 0 ? (
              <TraceRelationFlow
                onSelectWorkID={onSelectWorkID}
                relations={trace.relations}
              />
            ) : (
              "None"
            )}
          </dd>
        </div>
      </dl>

      {trace.dispatches.length > 0 ? (
        <div className="min-w-0 overflow-x-auto">
          <table
            className={cx(
              "w-full min-w-[860px] border-collapse [&_td]:border-t [&_td]:border-af-overlay/8 [&_td]:p-[0.7rem] [&_td]:text-left [&_td]:align-top [&_th]:border-t [&_th]:border-af-overlay/8 [&_th]:p-[0.7rem] [&_th]:text-left [&_th]:align-top",
              DASHBOARD_BODY_TEXT_CLASS,
            )}
          >
            <caption
              className={cx(
                "mb-2 text-left",
                DASHBOARD_SUPPORTING_LABEL_CLASS,
              )}
            >
              Trace dispatch grid
            </caption>
            <thead>
              <tr>
                <th className={DASHBOARD_SUPPORTING_LABEL_CLASS} scope="col">Dispatch</th>
                <th className={DASHBOARD_SUPPORTING_LABEL_CLASS} scope="col">Workstation</th>
                <th className={DASHBOARD_SUPPORTING_LABEL_CLASS} scope="col">Outcome</th>
                <th className={DASHBOARD_SUPPORTING_LABEL_CLASS} scope="col">Input items</th>
                <th className={DASHBOARD_SUPPORTING_LABEL_CLASS} scope="col">Output items</th>
              </tr>
            </thead>
            <tbody>
              {trace.dispatches.map((dispatch) => (
                <tr key={dispatch.dispatch_id}>
                  <th scope="row">
                    <span
                      className={cx(
                        "inline-flex rounded-full bg-af-info/15 px-2 py-[0.18rem] text-af-info-ink",
                        DASHBOARD_SUPPORTING_CODE_CLASS,
                      )}
                    >
                      {dispatch.dispatch_id}
                    </span>
                  </th>
                  <td>{dispatch.workstation_name || dispatch.transition_id}</td>
                  <td>
                    {formatTraceOutcome(dispatch.outcome)} ·{" "}
                    {formatDurationMillis(dispatch.duration_millis)}
                  </td>
                  <td>
                    {dispatch.input_items && dispatch.input_items.length > 0 ? (
                      <SelectableWorkList
                        onSelectWorkID={onSelectWorkID}
                        workItems={dispatch.input_items}
                      />
                    ) : (
                      <span>No input items recorded.</span>
                    )}
                  </td>
                  <td>
                    {dispatch.output_items && dispatch.output_items.length > 0 ? (
                      <SelectableWorkList
                        onSelectWorkID={onSelectWorkID}
                        workItems={dispatch.output_items}
                      />
                    ) : (
                      <span>No output items recorded.</span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <div className={cx(EMPTY_STATE_CLASS, EMPTY_STATE_COMPACT_CLASS)}>
          <h3>Trace history unavailable</h3>
          <p>No retained dispatch history is currently available for this work item.</p>
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
    <ul className="m-0 grid gap-[0.35rem] p-0">
      {workItems.map((workItem) => (
        <li className="list-none" key={workItem.work_id}>
          {onSelectWorkID ? (
            <Button
              className={TRACE_WORK_ITEM_BUTTON_CLASS}
              onClick={() => onSelectWorkID(workItem.work_id)}
              size="sm"
              title={workItem.work_id}
              tone="secondary"
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

  return [...itemsByID.values()].sort((left, right) => left.work_id.localeCompare(right.work_id));
}
