import {
  formatLocalDateTime,
  formatWorkItemLabel,
} from "../../../../components/ui/formatters";
import { formatDashboardPlaceLabel } from "../../../../components/ui/place-labels";
import {
  DASHBOARD_BODY_CODE_CLASS,
  DASHBOARD_BODY_TEXT_CLASS,
  DASHBOARD_SECTION_HEADING_CLASS,
  DASHBOARD_SUPPORTING_TEXT_CLASS,
} from "../../../../components/ui/dashboard-typography";
import {
  DETAIL_COPY_CLASS,
  WIDGET_SUBTITLE_CLASS,
} from "../../../../components/ui/widget-frame";
import {
  emptyStatePlaceMessage,
  isTerminalOrFailedPlace,
} from "../../base/components/detail-card-shared";
import { SelectionDetailLayout } from "../../base/components/current-selection-detail-layout";
import {
  useCurrentSelectionDetailMessages,
  useCurrentSelectionLocale,
} from "../../base/components/current-selection-locale";
import type {
  StateNodeDetailCardProps,
  StatePositionWorkListItemProps,
  StatePositionWorkListProps,
} from "../lib/detail-card-types";

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
  const usesRetainedWorkItems = isTerminalOrFailedPlace(place);
  const visibleWorkItems = usesRetainedWorkItems
    ? terminalHistoryWorkItems
    : currentWorkItems;
  const messages = useCurrentSelectionDetailMessages();
  const summaryLabel = formatStateSelectionSummary(
    place.type_id,
    place.state_value,
  );

  return (
    <SelectionDetailLayout widgetId={widgetId}>
      <div className="mt-0 grid gap-1" title={placeLabel}>
        <p className={WIDGET_SUBTITLE_CLASS}>{summaryLabel || placeLabel}</p>
      </div>
      <dl>
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
  const locale = useCurrentSelectionLocale();
  const workLabel = formatWorkItemLabel(workItem);
  const workID = workItem.work_id?.trim();
  const startedAt = resolveStartedAt(workItem);
  const hasFailureReason = Boolean(failureDetail?.failure_reason);
  const hasFailureMessage = Boolean(failureDetail?.failure_message);
  const content = (
    <>
      <strong className="min-w-0 [overflow-wrap:anywhere]">{workLabel}</strong>
      {workID ? (
        <code className={`${DASHBOARD_BODY_CODE_CLASS} ${DASHBOARD_SUPPORTING_TEXT_CLASS}`}>
          {workID}
        </code>
      ) : null}
      {startedAt ? (
        <time
          className={DASHBOARD_BODY_TEXT_CLASS}
          dateTime={startedAt}
          title={startedAt}
        >
          {messages.startedAtLabel}{" "}
          {formatLocalDateTime(startedAt, messages.timestampUnavailable, locale)}
        </time>
      ) : null}
      {hasFailureReason || hasFailureMessage ? (
        <dl
          className={`m-0 grid gap-1.5 [&_dd]:m-0 [&_div]:grid [&_div]:min-w-0 [&_div]:grid-cols-[7rem_minmax(0,1fr)] [&_div]:gap-2 ${DASHBOARD_BODY_TEXT_CLASS}`}
        >
          {hasFailureReason ? (
            <div>
              <dt>{messages.failureReasonLabel}</dt>
              <dd className="[overflow-wrap:anywhere]">
                {failureDetail?.failure_reason}
              </dd>
            </div>
          ) : null}
          {hasFailureMessage ? (
            <div>
              <dt>{messages.failureMessageLabel}</dt>
              <dd className="[overflow-wrap:anywhere]">
                {failureDetail?.failure_message}
              </dd>
            </div>
          ) : null}
        </dl>
      ) : null}
    </>
  );

  if (onSelectWorkItem) {
    return (
      <li>
        <button
          aria-label={messages.selectWorkItemLabel(workLabel)}
          className="grid w-full min-w-0 cursor-pointer gap-2 rounded-lg border border-af-border bg-af-surface-subtle px-3 py-2 text-left outline-af-focus-ring transition hover:bg-af-overlay focus-visible:outline-2 focus-visible:outline-offset-2"
          onClick={() => onSelectWorkItem(workItem)}
          type="button"
        >
          {content}
        </button>
      </li>
    );
  }

  return (
    <li className="grid min-w-0 gap-2 rounded-lg border border-af-border bg-af-surface-subtle px-3 py-2 text-sm">
      {content}
    </li>
  );
}

function formatStateSelectionSummary(workType?: string, stateValue?: string): string {
  if (workType && stateValue) {
    return `${workType}: ${stateValue}`;
  }

  return workType ?? stateValue ?? "";
}

function resolveStartedAt(
  workItem: StatePositionWorkListItemProps["workItem"],
): string | null {
  const startedAt = workItem.startedAt ?? workItem.started_at;

  if (!startedAt || Number.isNaN(Date.parse(startedAt))) {
    return null;
  }

  return startedAt;
}
