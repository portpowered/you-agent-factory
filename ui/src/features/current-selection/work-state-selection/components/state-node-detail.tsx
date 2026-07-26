import { WidgetDetailCopy } from "@you-agent-factory/components/recipes";
import { DescriptionList } from "@you-agent-factory/components/data-display";
import { Code, Text } from "@you-agent-factory/components/primitives";
import { DashboardActionButton } from "../../../../components/ui/dashboard-action-button";
import {
  formatLocalDateTime,
  formatWorkItemLabel,
} from "../../../../components/ui/formatters";
import { formatDashboardPlaceLabel } from "../../../../components/ui/place-labels";
import {
  emptyStatePlaceMessage,
  isTerminalOrFailedPlace,
} from "../../base/components/detail-card/detail-card-shared";
import { SelectionDetailLayout } from "../../base/components/layout/current-selection-detail-layout";
import {
  useCurrentSelectionDetailMessages,
  useCurrentSelectionLocale,
  useCurrentSelectionWorkstationDetailMessages,
} from "../../base/components/presentation/current-selection-locale";
import { CurrentSelectionWorkRow } from "../../base/components/current-selection-work-row";
import { CurrentSelectionExpandableSection } from "../../base/components/detail/current-selection-expandable-section";
import { CurrentSelectionBodyLayout } from "../../base/components/layout/current-selection-body-layout";
import { CurrentSelectionExecutionPill } from "../../base/components/presentation/current-selection-pill";
import { CurrentSelectionSupportingText } from "../../base/components/presentation/current-selection-supporting-text";
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
  const title = summaryLabel || placeLabel;

  return (
    <SelectionDetailLayout headerAction={headerAction} widgetId={widgetId}>
      <CurrentSelectionBodyLayout title={title}>
        <CurrentSelectionExpandableSection
          defaultExpanded
          title={workStateMessages.summaryHeading}
          toggleLabel={(expanded) =>
            expanded
              ? workStateMessages.collapseAction
              : workStateMessages.expandAction
          }
        >
          <DescriptionList>
            {place.type_id ? (
              <div>
                <dt>{messages.workTypeLabel}</dt>
                <dd>{place.type_id}</dd>
              </div>
            ) : null}
            {place.state_value ? (
              <div>
                <dt>{messages.stateLabel}</dt>
                <dd>{place.state_value}</dd>
              </div>
            ) : null}
            <div>
              <dt>{messages.stateNodeIdLabel}</dt>
              <dd>{place.place_id}</dd>
            </div>
            <div>
              <dt>{messages.countLabel}</dt>
              <dd>{tokenCount}</dd>
            </div>
          </DescriptionList>
        </CurrentSelectionExpandableSection>
        {editableConfigurationState ? (
          <WorkStateEditableConfigurationSection
            messages={workStateMessages}
            saveState={saveState}
            state={editableConfigurationState}
          />
        ) : null}
        <CurrentSelectionExpandableSection
          defaultExpanded
          title={messages.currentWorkHeading}
          toggleLabel={(expanded) =>
            expanded
              ? workStateMessages.collapseAction
              : workStateMessages.expandAction
          }
        >
          {visibleWorkItems.length > 0 ? (
            <StatePositionWorkList
              failedWorkDetailsByWorkID={failedWorkDetailsByWorkID}
              messages={messages}
              onSelectWorkItem={onSelectWorkItem}
              workItems={visibleWorkItems}
            />
          ) : (
            <WidgetDetailCopy>
              {emptyStatePlaceMessage(
                messages,
                usesRetainedWorkItems,
                tokenCount,
              )}
            </WidgetDetailCopy>
          )}
        </CurrentSelectionExpandableSection>
      </CurrentSelectionBodyLayout>
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
  const workstationMessages = useCurrentSelectionWorkstationDetailMessages();
  const workLabel = formatWorkItemLabel(workItem);
  const workID = workItem.work_id?.trim();
  const startedAt = resolveStartedAt(workItem);
  const hasFailureReason = Boolean(failureDetail?.failure_reason);
  const hasFailureMessage = Boolean(failureDetail?.failure_message);
  const action = onSelectWorkItem ? (
    <DashboardActionButton
      aria-label={messages.selectWorkItemLabel(workLabel)}
      onClick={() => onSelectWorkItem(workItem)}
      type="button"
    >
      {workstationMessages.openWorkItemAction}
    </DashboardActionButton>
  ) : null;
  const status = startedAt ? (
    <CurrentSelectionExecutionPill>
      <Text as="time" dateTime={startedAt} title={startedAt}>
        {messages.startedAtLabel}{" "}
        {formatLocalDateTime(startedAt, messages.timestampUnavailable, locale)}
      </Text>
    </CurrentSelectionExecutionPill>
  ) : undefined;
  const supportingContent = (
    <>
      {workID ? (
        <CurrentSelectionSupportingText tone="status">
          <Code className="text-on-surface-variant" size="supporting">
            {workID}
          </Code>
        </CurrentSelectionSupportingText>
      ) : null}
      {hasFailureReason || hasFailureMessage ? (
        <DescriptionList className="[&_div]:grid-cols-[7rem_minmax(0,1fr)]">
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
        </DescriptionList>
      ) : null}
    </>
  );

  return (
    <CurrentSelectionWorkRow
      actions={action}
      status={status}
      supportingContent={supportingContent}
      title={workLabel}
    />
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
