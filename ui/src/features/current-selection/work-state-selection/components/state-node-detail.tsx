import {
  DashboardCode,
  DashboardDescriptionList,
  DashboardText,
  SurfacePanel,
} from "../../../../components/ui";
import {
  formatLocalDateTime,
  formatWorkItemLabel,
} from "../../../../components/ui/formatters";
import { formatDashboardPlaceLabel } from "../../../../components/ui/place-labels";
import {
  DetailCopy,
  WidgetSubtitle,
} from "../../../../components/ui/widget-frame";
import { SelectionDetailLayout } from "../../base/components/current-selection-detail-layout";
import {
  useCurrentSelectionDetailMessages,
  useCurrentSelectionLocale,
} from "../../base/components/current-selection-locale";
import {
  emptyStatePlaceMessage,
  isTerminalOrFailedPlace,
} from "../../base/components/detail-card-shared";
import {
  CurrentSelectionContentSection,
  CurrentSelectionSelectableButton,
} from "../../base/public";
import type {
  StateNodeDetailCardProps,
  StatePositionWorkListItemProps,
  StatePositionWorkListProps,
} from "../lib/detail-card-types";
import { getWorkStateDetailMessages } from "../messages/work-state-detail";
import { WorkStateEditableConfigurationSection } from "./work-state-editable-configuration-section";

export function StateNodeDetailCard({
  currentWorkItems,
  editableConfigurationState,
  failedWorkDetailsByWorkID,
  headerAction,
  locale,
  onSelectWorkItem,
  place,
  saveState,
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
  const workStateMessages = getWorkStateDetailMessages(locale);
  const summaryLabel = formatStateSelectionSummary(
    place.type_id,
    place.state_value,
  );
  const showRuntimeSummary = editableConfigurationState?.status !== "ready";

  return (
    <SelectionDetailLayout headerAction={headerAction} widgetId={widgetId}>
      {showRuntimeSummary ? (
        <div className="mt-0 grid gap-1" title={placeLabel}>
          <WidgetSubtitle>{summaryLabel || placeLabel}</WidgetSubtitle>
        </div>
      ) : null}
      {editableConfigurationState ? (
        <WorkStateEditableConfigurationSection
          messages={workStateMessages}
          saveState={saveState}
          state={editableConfigurationState}
        />
      ) : null}
      <dl>
        <div>
          <dt>{messages.countLabel}</dt>
          <dd>{tokenCount}</dd>
        </div>
      </dl>
      <CurrentSelectionContentSection title={messages.currentWorkHeading}>
        {visibleWorkItems.length > 0 ? (
          <StatePositionWorkList
            failedWorkDetailsByWorkID={failedWorkDetailsByWorkID}
            messages={messages}
            onSelectWorkItem={onSelectWorkItem}
            workItems={visibleWorkItems}
          />
        ) : (
          <DetailCopy>
            {emptyStatePlaceMessage(
              messages,
              usesRetainedWorkItems,
              tokenCount,
            )}
          </DetailCopy>
        )}
      </CurrentSelectionContentSection>
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
        <DashboardCode className="text-on-surface-variant" size="supporting">
          {workID}
        </DashboardCode>
      ) : null}
      {startedAt ? (
        <DashboardText as="time" dateTime={startedAt} title={startedAt}>
          {messages.startedAtLabel}{" "}
          {formatLocalDateTime(
            startedAt,
            messages.timestampUnavailable,
            locale,
          )}
        </DashboardText>
      ) : null}
      {hasFailureReason || hasFailureMessage ? (
        <DashboardDescriptionList className="[&_div]:grid-cols-[7rem_minmax(0,1fr)]">
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
        </DashboardDescriptionList>
      ) : null}
    </>
  );

  if (onSelectWorkItem) {
    return (
      <li>
        <CurrentSelectionSelectableButton
          aria-label={messages.selectWorkItemLabel(workLabel)}
          className="min-w-0 gap-2"
          onClick={() => onSelectWorkItem(workItem)}
          variant="card"
        >
          {content}
        </CurrentSelectionSelectableButton>
      </li>
    );
  }

  return (
    <SurfacePanel asChild className="grid min-w-0 gap-2 text-sm" radius="lg">
      <li>{content}</li>
    </SurfacePanel>
  );
}

function formatStateSelectionSummary(
  workType?: string,
  stateValue?: string,
): string {
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
