import { formatWorkItemLabel } from "../../components/ui/formatters";
import {
  formatDashboardPlaceLabel,
  getDashboardPlaceLabelParts,
} from "../../components/ui/place-labels";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
} from "../../components/dashboard/typography";
import { DETAIL_COPY_CLASS, WIDGET_SUBTITLE_CLASS } from "../../components/dashboard/widget-board";
import { SelectionDetailLayout } from "./current-selection-detail-layout";
import {
  emptyStatePlaceMessage,
  isTerminalOrFailedPlace,
} from "./detail-card-shared";
import type {
  StateNodeDetailCardProps,
  StatePositionWorkListItemProps,
  StatePositionWorkListProps,
} from "./detail-card-types";

const STATE_PLACE_COUNT_LABEL = "Count";
const STATE_PLACE_CURRENT_WORK_HEADING = "Current work";

export function StateNodeDetailCard({
  currentWorkItems,
  failedWorkDetailsByWorkID,
  onSelectWorkItem,
  place,
  terminalHistoryWorkItems = [],
  tokenCount,
  widgetId = "current-selection",
}: StateNodeDetailCardProps) {
  const placeLabel = formatDashboardPlaceLabel(place);
  const placeLabelParts = getDashboardPlaceLabelParts(place);
  const usesRetainedWorkItems = isTerminalOrFailedPlace(place);
  const visibleWorkItems = usesRetainedWorkItems ? terminalHistoryWorkItems : currentWorkItems;

  return (
    <SelectionDetailLayout widgetId={widgetId}>
      <div className="mt-0 grid gap-[0.18rem]" title={placeLabel}>
        <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>{placeLabelParts.workType}</span>
        <span className={WIDGET_SUBTITLE_CLASS}>{placeLabelParts.stateValue}</span>
      </div>
      <dl>
        <div>
          <dt>Work type</dt>
          <dd>{placeLabelParts.workType}</dd>
        </div>
        <div>
          <dt>State</dt>
          <dd>{placeLabelParts.stateValue}</dd>
        </div>
        <div>
          <dt>State node ID</dt>
          <dd>{placeLabel}</dd>
        </div>
        <div>
          <dt>{STATE_PLACE_COUNT_LABEL}</dt>
          <dd>{tokenCount}</dd>
        </div>
      </dl>
      <section className="mt-4 grid gap-[0.65rem] [&_h4]:m-0">
        <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>{STATE_PLACE_CURRENT_WORK_HEADING}</h4>
        {visibleWorkItems.length > 0 ? (
          <StatePositionWorkList
            failedWorkDetailsByWorkID={failedWorkDetailsByWorkID}
            onSelectWorkItem={onSelectWorkItem}
            workItems={visibleWorkItems}
          />
        ) : (
          <p className={DETAIL_COPY_CLASS}>
            {emptyStatePlaceMessage(usesRetainedWorkItems, tokenCount)}
          </p>
        )}
      </section>
    </SelectionDetailLayout>
  );
}

function StatePositionWorkList({
  failedWorkDetailsByWorkID,
  onSelectWorkItem,
  workItems,
}: StatePositionWorkListProps) {
  return (
    <ul className="m-0 grid list-none gap-[0.65rem] p-0">
      {workItems.map((workItem) => (
        <StatePositionWorkListItem
          failureDetail={failedWorkDetailsByWorkID?.[workItem.work_id]}
          key={workItem.work_id}
          onSelectWorkItem={onSelectWorkItem}
          workItem={workItem}
        />
      ))}
    </ul>
  );
}

function StatePositionWorkListItem({
  failureDetail,
  onSelectWorkItem,
  workItem,
}: StatePositionWorkListItemProps) {
  const workLabel = formatWorkItemLabel(workItem);
  const content = (
    <>
      <strong className="min-w-0 [overflow-wrap:anywhere]">{workLabel}</strong>
      <dl
        className={`m-0 grid gap-[0.35rem] [&_dd]:m-0 [&_div]:grid [&_div]:min-w-0 [&_div]:grid-cols-[5rem_minmax(0,1fr)] [&_div]:gap-2 ${DASHBOARD_BODY_TEXT_CLASS}`}
      >
        <div>
          <dt>Work ID</dt>
          <dd className="[overflow-wrap:anywhere]">{workItem.work_id}</dd>
        </div>
        <div>
          <dt>Work type</dt>
          <dd className="[overflow-wrap:anywhere]">{workItem.work_type_id || "Unknown"}</dd>
        </div>
        {workItem.trace_id ? (
          <div>
            <dt>Trace ID</dt>
            <dd className="[overflow-wrap:anywhere]">{workItem.trace_id}</dd>
          </div>
        ) : null}
        {failureDetail?.failure_reason ? (
          <div>
            <dt>Failure reason</dt>
            <dd className="[overflow-wrap:anywhere]">{failureDetail.failure_reason}</dd>
          </div>
        ) : null}
        {failureDetail?.failure_message ? (
          <div>
            <dt>Failure message</dt>
            <dd className="[overflow-wrap:anywhere]">{failureDetail.failure_message}</dd>
          </div>
        ) : null}
      </dl>
    </>
  );

  if (onSelectWorkItem) {
    return (
      <li>
        <button
          aria-label={`Select work item ${workLabel}`}
          className="grid w-full min-w-0 cursor-pointer gap-[0.45rem] rounded-lg border border-af-overlay/8 bg-af-overlay/4 px-3 py-2 text-left outline-af-accent transition hover:bg-af-overlay/8 focus-visible:outline-2 focus-visible:outline-offset-2"
          onClick={() => onSelectWorkItem(workItem)}
          type="button"
        >
          {content}
        </button>
      </li>
    );
  }

  return (
    <li className="grid min-w-0 gap-[0.45rem] rounded-lg border border-af-overlay/8 bg-af-overlay/4 px-3 py-2 text-sm">
      {content}
    </li>
  );
}
