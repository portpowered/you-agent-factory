import { formatWorkItemLabel } from "../../../components/ui/formatters";
import {
  formatDashboardPlaceLabel,
  getDashboardPlaceLabelParts,
} from "../../../components/ui/place-labels";
import {
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_LABEL_CLASS,
} from "../../../components/ui/dashboard-typography";
import {
  DETAIL_COPY_CLASS,
  WIDGET_SUBTITLE_CLASS,
} from "../../../components/dashboard/widget-board";
import { SelectionDetailLayout } from "./current-selection-detail-layout";
import {
  emptyStatePlaceMessage,
  isTerminalOrFailedPlace,
} from "./detail-card-shared";
import { useCurrentSelectionDetailMessages } from "./current-selection-locale";
import type {
  StateNodeDetailCardProps,
  StatePositionWorkListItemProps,
  StatePositionWorkListProps,
} from "../detail-card-types";

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
  const visibleWorkItems = usesRetainedWorkItems
    ? terminalHistoryWorkItems
    : currentWorkItems;
  const messages = useCurrentSelectionDetailMessages();

  return (
    <SelectionDetailLayout widgetId={widgetId}>
      <div className="mt-0 grid gap-1" title={placeLabel}>
        <span className={DASHBOARD_SUPPORTING_LABEL_CLASS}>
          {placeLabelParts.workType}
        </span>
        <span className={WIDGET_SUBTITLE_CLASS}>
          {placeLabelParts.stateValue}
        </span>
      </div>
      <dl>
        <div>
          <dt>{messages.workTypeLabel}</dt>
          <dd>{placeLabelParts.workType}</dd>
        </div>
        <div>
          <dt>{messages.stateLabel}</dt>
          <dd>{placeLabelParts.stateValue}</dd>
        </div>
        <div>
          <dt>{messages.stateNodeIdLabel}</dt>
          <dd>{placeLabel}</dd>
        </div>
        <div>
          <dt>{messages.countLabel}</dt>
          <dd>{tokenCount}</dd>
        </div>
      </dl>
      <section className="mt-4 grid gap-2.5 [&_h4]:m-0">
        <h4 className={DASHBOARD_SECTION_HEADING_CLASS}>
          {messages.currentWorkHeading}
        </h4>
        {visibleWorkItems.length > 0 ? (
          <StatePositionWorkList
            failedWorkDetailsByWorkID={failedWorkDetailsByWorkID}
            messages={messages}
            onSelectWorkItem={onSelectWorkItem}
            workItems={visibleWorkItems}
          />
        ) : (
          <p className={DETAIL_COPY_CLASS}>
            {emptyStatePlaceMessage(
              messages,
              usesRetainedWorkItems,
              tokenCount,
            )}
          </p>
        )}
      </section>
    </SelectionDetailLayout>
  );
}

function StatePositionWorkList({
  failedWorkDetailsByWorkID,
  messages,
  onSelectWorkItem,
  workItems,
}: StatePositionWorkListProps) {
  return (
    <ul className="m-0 grid list-none gap-2.5 p-0">
      {workItems.map((workItem) => (
        <StatePositionWorkListItem
          failureDetail={failedWorkDetailsByWorkID?.[workItem.work_id]}
          key={workItem.work_id}
          messages={messages}
          onSelectWorkItem={onSelectWorkItem}
          workItem={workItem}
        />
      ))}
    </ul>
  );
}

function StatePositionWorkListItem({
  failureDetail,
  messages,
  onSelectWorkItem,
  workItem,
}: StatePositionWorkListItemProps) {
  const workLabel = formatWorkItemLabel(workItem);
  const content = (
    <>
      <strong className="min-w-0 [overflow-wrap:anywhere]">{workLabel}</strong>
      <dl
        className={`m-0 grid gap-1.5 [&_dd]:m-0 [&_div]:grid [&_div]:min-w-0 [&_div]:grid-cols-[5rem_minmax(0,1fr)] [&_div]:gap-2 ${DASHBOARD_BODY_TEXT_CLASS}`}
      >
        <div>
          <dt>{messages.workIdLabel}</dt>
          <dd className="[overflow-wrap:anywhere]">{workItem.work_id}</dd>
        </div>
        <div>
          <dt>{messages.workTypeLabel}</dt>
          <dd className="[overflow-wrap:anywhere]">
            {workItem.work_type_id || messages.workTypeUnavailable}
          </dd>
        </div>
        {workItem.trace_id ? (
          <div>
            <dt>{messages.traceIdLabel}</dt>
            <dd className="[overflow-wrap:anywhere]">{workItem.trace_id}</dd>
          </div>
        ) : null}
        {failureDetail?.failure_reason ? (
          <div>
            <dt>{messages.failureReasonLabel}</dt>
            <dd className="[overflow-wrap:anywhere]">
              {failureDetail.failure_reason}
            </dd>
          </div>
        ) : null}
        {failureDetail?.failure_message ? (
          <div>
            <dt>{messages.failureMessageLabel}</dt>
            <dd className="[overflow-wrap:anywhere]">
              {failureDetail.failure_message}
            </dd>
          </div>
        ) : null}
      </dl>
    </>
  );

  if (onSelectWorkItem) {
    return (
      <li>
        <button
          aria-label={messages.selectWorkItemLabel(workLabel)}
          className="grid w-full min-w-0 cursor-pointer gap-2 rounded-lg border border-af-overlay/8 bg-af-overlay/4 px-3 py-2 text-left outline-af-accent transition hover:bg-af-overlay/8 focus-visible:outline-2 focus-visible:outline-offset-2"
          onClick={() => onSelectWorkItem(workItem)}
          type="button"
        >
          {content}
        </button>
      </li>
    );
  }

  return (
    <li className="grid min-w-0 gap-2 rounded-lg border border-af-overlay/8 bg-af-overlay/4 px-3 py-2 text-sm">
      {content}
    </li>
  );
}
